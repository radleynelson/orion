package terminal

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"orion/internal/tmuxutil"

	"github.com/creack/pty"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Terminal represents a single terminal session backed by a PTY.
type Terminal struct {
	ID             string
	pty            *os.File
	cmd            *exec.Cmd
	done           chan struct{}
	tmuxSession    string       // if attached to a tmux session, track it for cleanup
	OutputCallback func([]byte) // if set, output goes here instead of Wails events
	isGrouped      bool         // true for grouped tmux sessions (web terminals)
}

// Manager manages multiple terminal sessions.
type Manager struct {
	ctx       context.Context
	terminals map[string]*Terminal
	mu        sync.RWMutex
}

// NewManager creates a new terminal manager.
func NewManager() *Manager {
	tmuxutil.ConfigureExtendedKeys()
	return &Manager{
		terminals: make(map[string]*Terminal),
	}
}

// SetContext sets the Wails runtime context for event emission.
func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Create spawns a new terminal session with the user's default shell.
func (m *Manager) Create(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.terminals[id]; exists {
		return fmt.Errorf("terminal %s already exists", id)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	cmd := exec.Command(shell, "-l")
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"LC_CTYPE=en_US.UTF-8",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	t := &Terminal{
		ID:   id,
		pty:  ptmx,
		cmd:  cmd,
		done: make(chan struct{}),
	}
	m.terminals[id] = t

	// Stream output to frontend
	go m.readLoop(t)

	return nil
}

// CreateInDir creates a tmux session in the given directory and attaches to it.
// Uses tmux so the session is recoverable after Orion restarts.
// Automatically sources .orion/env.sh if it exists (for port awareness).
func (m *Manager) CreateInDir(id string, dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tmuxutil.ConfigureExtendedKeys()

	if _, exists := m.terminals[id]; exists {
		return fmt.Errorf("terminal %s already exists", id)
	}

	// Create a tmux session with a name based on the terminal id
	tmuxName := "orion-shell-" + id

	createCmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", dir)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session failed: err=%v out=%q name=%s dir=%s", err, strings.TrimSpace(string(out)), tmuxName, dir)
	}
	exec.Command("tmux", "set-option", "-t", tmuxName, "history-limit", "50000").Run()
	exec.Command("tmux", "set-option", "-t", tmuxName, "mouse", "on").Run()
	exec.Command("tmux", "set-option", "-t", tmuxName, "status", "off").Run()
	exec.Command("tmux", "set-option", "-t", tmuxName, "set-clipboard", "on").Run()
	tmuxutil.ConfigureSessionExtendedKeys(tmuxName)
	exec.Command("tmux", "bind-key", "-T", "copy-mode", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", "pbcopy").Run()
	exec.Command("tmux", "bind-key", "-T", "copy-mode-vi", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", "pbcopy").Run()
	setTmuxMetadata(tmuxName, "shell", "Shell", dir)

	// Source .orion/env.sh if it exists
	envFile := filepath.Join(dir, ".orion", "env.sh")
	if _, err := os.Stat(envFile); err == nil {
		exec.Command("tmux", "send-keys", "-t", tmuxName, "source .orion/env.sh", "Enter").Run()
	}

	// Attach to the tmux session
	cmd := exec.Command("tmux", "attach-session", "-d", "-t", tmuxName)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"LC_CTYPE=en_US.UTF-8",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		// Clean up the tmux session if attach fails
		exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}

	t := &Terminal{
		ID:          id,
		pty:         ptmx,
		cmd:         cmd,
		done:        make(chan struct{}),
		tmuxSession: tmuxName,
	}
	m.terminals[id] = t

	go m.readLoop(t)

	return nil
}

// CreateAttached spawns a terminal that attaches to an existing tmux session.
func (m *Manager) CreateAttached(id, tmuxSession string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tmuxutil.ConfigureSessionExtendedKeys(tmuxSession)

	if _, exists := m.terminals[id]; exists {
		return fmt.Errorf("terminal %s already exists", id)
	}

	cmd := exec.Command("tmux", "attach-session", "-d", "-t", tmuxSession)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"LC_CTYPE=en_US.UTF-8",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to attach to tmux session %s: %w", tmuxSession, err)
	}

	t := &Terminal{
		ID:          id,
		pty:         ptmx,
		cmd:         cmd,
		done:        make(chan struct{}),
		tmuxSession: tmuxSession,
	}
	m.terminals[id] = t

	go m.readLoop(t)

	return nil
}

func setTmuxMetadata(session string, sessionType string, label string, workspacePath string) {
	if strings.TrimSpace(session) == "" {
		return
	}
	exec.Command("tmux", "set-option", "-t", session, "@orion_type", sessionType).Run()
	exec.Command("tmux", "set-option", "-t", session, "@orion_label", label).Run()
	exec.Command("tmux", "set-option", "-t", session, "@orion_workspace", workspacePath).Run()
}

// CreateGroupedAttached creates a grouped tmux session linked to an existing session.
// This allows independent window sizing (phone won't shrink the desktop terminal).
// Output goes to the provided callback instead of Wails events.
func (m *Manager) CreateGroupedAttached(id, tmuxSession string, onOutput func([]byte)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tmuxutil.ConfigureSessionExtendedKeys(tmuxSession)

	if _, exists := m.terminals[id]; exists {
		return fmt.Errorf("terminal %s already exists", id)
	}

	// Create a grouped session linked to the target session
	groupedName := "orion-web-" + id
	// Kill any pre-existing session with the same name (no-op if none exists)
	exec.Command("tmux", "kill-session", "-t", groupedName).Run()
	createCmd := exec.Command("tmux", "new-session", "-d", "-s", groupedName, "-t", tmuxSession)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux grouped session failed: err=%v out=%q", err, strings.TrimSpace(string(out)))
	}
	// Avoid changing global sizing rules for every tmux client on the machine.
	// `aggressive-resize` is explicitly poor for interactive shells and was a
	// major source of redraw churn for the mobile client.
	exec.Command("tmux", "set-option", "-t", groupedName, "aggressive-resize", "off").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "status", "off").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "mouse", "off").Run()
	tmuxutil.ConfigureSessionExtendedKeys(groupedName)
	// Disable terminal features that SwiftTerm may not handle gracefully.
	// These are the sequences most likely to show up as garbage on the iOS client.
	exec.Command("tmux", "set-option", "-t", groupedName, "focus-events", "off").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "set-clipboard", "off").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "set-titles", "off").Run()
	// Tell tmux this client doesn't support some advanced xterm features
	// (XT = xterm title setting, which also enables XTWINOPS query responses).
	exec.Command("tmux", "set-option", "-ga", "terminal-overrides", ",xterm-256color:XT@").Run()
	if onOutput != nil {
		if history, err := capturePaneHistory(groupedName, 2000); err == nil && len(history) > 0 {
			onOutput(history)
		}
	}

	// Attach to the grouped session
	cmd := exec.Command("tmux", "attach-session", "-t", groupedName)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"LC_CTYPE=en_US.UTF-8",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		exec.Command("tmux", "kill-session", "-t", groupedName).Run()
		return fmt.Errorf("failed to attach to grouped tmux session: %w", err)
	}

	t := &Terminal{
		ID:             id,
		pty:            ptmx,
		cmd:            cmd,
		done:           make(chan struct{}),
		tmuxSession:    groupedName,
		OutputCallback: onOutput,
		isGrouped:      true,
	}
	m.terminals[id] = t

	go m.readLoop(t)

	return nil
}

func capturePaneHistory(target string, lines int) ([]byte, error) {
	if lines <= 0 {
		lines = 2000
	}
	args := []string{
		"capture-pane",
		"-p",
		"-e",
		"-N",
		"-S", fmt.Sprintf("-%d", lines),
		"-t", target,
	}
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tmux capture-pane failed: err=%v out=%q", err, strings.TrimSpace(string(out)))
	}
	if len(out) == 0 {
		return nil, nil
	}
	if out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

// Write sends input data to a terminal.
func (m *Manager) Write(id string, data string) error {
	m.mu.RLock()
	t, ok := m.terminals[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("terminal %s not found", id)
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		// If not base64, write raw
		_, writeErr := t.pty.Write([]byte(data))
		return writeErr
	}

	_, err = t.pty.Write(decoded)
	return err
}

// Resize changes the terminal dimensions.
func (m *Manager) Resize(id string, cols, rows int) error {
	m.mu.RLock()
	t, ok := m.terminals[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("terminal %s not found", id)
	}

	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: uint16(rows),
		Col: uint16(cols),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		t.pty.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return fmt.Errorf("failed to resize: %v", errno)
	}

	return nil
}

// Close terminates a terminal session and kills its tmux session if attached.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	t, ok := m.terminals[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("terminal %s not found", id)
	}
	delete(m.terminals, id)
	m.mu.Unlock()

	close(t.done)
	t.pty.Close()
	t.cmd.Process.Signal(syscall.SIGHUP)
	t.cmd.Wait()

	// Kill the underlying tmux session so no zombie processes remain
	if t.tmuxSession != "" {
		exec.Command("tmux", "kill-session", "-t", t.tmuxSession).Run()
	}

	return nil
}

// Detach closes Orion's PTY attachment without killing the underlying tmux
// session. This is used when switching a tmux-backed session to another view.
func (m *Manager) Detach(id string) error {
	m.mu.Lock()
	t, ok := m.terminals[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("terminal %s not found", id)
	}
	delete(m.terminals, id)
	m.mu.Unlock()

	close(t.done)
	t.pty.Close()
	t.cmd.Process.Signal(syscall.SIGHUP)
	t.cmd.Wait()

	return nil
}

// DetachAll detaches from all terminal PTYs without killing tmux sessions.
// Used on app shutdown so sessions survive for recovery on next launch.
func (m *Manager) DetachAll() {
	m.mu.Lock()
	terminals := make([]*Terminal, 0, len(m.terminals))
	for _, t := range m.terminals {
		terminals = append(terminals, t)
	}
	m.terminals = make(map[string]*Terminal)
	m.mu.Unlock()

	for _, t := range terminals {
		close(t.done)
		t.pty.Close()
		t.cmd.Process.Signal(syscall.SIGHUP)
		t.cmd.Wait()
		// Do NOT kill tmux session — leave it alive for recovery
	}
}

// CloseAll closes all terminal sessions and kills their tmux sessions.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.terminals))
	for id := range m.terminals {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.Close(id)
	}
}

// GetTmuxSession returns the tmux session name for a terminal, or empty string.
func (m *Manager) GetTmuxSession(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.terminals[id]; ok {
		return t.tmuxSession
	}
	return ""
}

// IsBusy reports whether the terminal's tmux pane is running something other
// than an idle shell. Returns false for terminals not attached to tmux or when
// the lookup fails (so callers err on the side of letting Cmd+W proceed).
func (m *Manager) IsBusy(id string) bool {
	m.mu.RLock()
	session := ""
	if t, ok := m.terminals[id]; ok {
		session = t.tmuxSession
	}
	m.mu.RUnlock()
	if session == "" {
		return false
	}
	out, err := exec.Command("tmux", "display-message", "-t", session, "-p", "#{pane_current_command}").Output()
	if err != nil {
		return false
	}
	cmd := strings.TrimSpace(string(out))
	if cmd == "" {
		return false
	}
	switch strings.ToLower(cmd) {
	case "zsh", "bash", "sh", "fish", "dash", "tmux", "login", "-zsh", "-bash":
		return false
	}
	return true
}

// List returns IDs of all active terminals.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.terminals))
	for id := range m.terminals {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) readLoop(t *Terminal) {
	// 32KB read buffer — larger reduces the chance of splitting escape
	// sequences across reads, and the OS buffering helps coalesce PTY writes.
	buf := make([]byte, 32768)
	eventName := fmt.Sprintf("terminal:output:%s", t.ID)
	// Partial buffer: holds trailing bytes that look like an incomplete
	// escape sequence. They get prepended to the next read so SwiftTerm
	// never sees a broken sequence split across WebSocket messages.
	var partial []byte

	for {
		select {
		case <-t.done:
			return
		default:
			n, err := t.pty.Read(buf)
			if err != nil {
				// PTY closed or process exited
				if t.OutputCallback != nil {
					t.OutputCallback(nil) // nil signals exit to web handler
				} else if m.ctx != nil {
					runtime.EventsEmit(m.ctx, fmt.Sprintf("terminal:exit:%s", t.ID))
				}
				return
			}
			if n > 0 {
				// Combine any held partial bytes with the new read
				combined := buf[:n]
				if len(partial) > 0 {
					combined = append(partial, combined...)
					partial = nil
				}

				// Find the safe split point — everything before is complete
				// sequences; everything after is a partial trailing escape sequence
				// that must be held.
				splitAt := safeSplitPoint(combined)
				toSend := combined[:splitAt]
				if splitAt < len(combined) {
					// Copy because `combined` overlaps with `buf` which will be reused
					partial = make([]byte, len(combined)-splitAt)
					copy(partial, combined[splitAt:])
				}

				if len(toSend) == 0 {
					continue
				}

				if t.OutputCallback != nil {
					out := make([]byte, len(toSend))
					copy(out, toSend)
					t.OutputCallback(out)
				} else if m.ctx != nil {
					encoded := base64.StdEncoding.EncodeToString(toSend)
					runtime.EventsEmit(m.ctx, eventName, encoded)
				}
			}
		}
	}
}

// safeSplitPoint returns the byte offset up to which the data can be sent
// without cutting a terminal escape sequence in half. Any bytes at or after
// the returned offset form an incomplete escape sequence and should be held
// until more data arrives.
func safeSplitPoint(data []byte) int {
	n := len(data)
	// Scan backwards looking for the last ESC byte (0x1B)
	for i := n - 1; i >= 0; i-- {
		if data[i] != 0x1B {
			continue
		}
		// Found ESC at position i. Is the sequence starting here complete?
		remaining := data[i+1:]
		if len(remaining) == 0 {
			// Lone ESC — incomplete
			return i
		}
		switch remaining[0] {
		case '[': // CSI — terminated by a byte in 0x40-0x7E
			for j := 1; j < len(remaining); j++ {
				b := remaining[j]
				if b >= 0x40 && b <= 0x7E {
					return n // complete
				}
			}
			return i // incomplete
		case ']': // OSC — terminated by BEL (0x07) or ESC \
			for j := 1; j < len(remaining); j++ {
				if remaining[j] == 0x07 {
					return n
				}
				if remaining[j] == 0x1B && j+1 < len(remaining) && remaining[j+1] == '\\' {
					return n
				}
			}
			return i
		case 'P', '_', '^', 'X': // DCS, APC, PM, SOS — terminated by ESC \
			for j := 1; j < len(remaining); j++ {
				if remaining[j] == 0x1B && j+1 < len(remaining) && remaining[j+1] == '\\' {
					return n
				}
			}
			return i
		default:
			// Single-byte escape (e.g., ESC 7, ESC 8, ESC =) — complete with 1 byte
			return n
		}
	}
	// No ESC found — all bytes are safe to send
	return n
}

// appendOrionEnv reads .orion/env.sh from a workspace dir and appends
// the exported variables to the given environment slice.
func appendOrionEnv(workspaceDir string, env []string) []string {
	envFile := filepath.Join(workspaceDir, ".orion", "env.sh")
	f, err := os.Open(envFile)
	if err != nil {
		return env
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse "export KEY=VALUE" lines
		if strings.HasPrefix(line, "export ") {
			kv := strings.TrimPrefix(line, "export ")
			if strings.Contains(kv, "=") {
				env = append(env, kv)
			}
		}
	}
	return env
}
