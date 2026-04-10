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

	"github.com/creack/pty"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Terminal represents a single terminal session backed by a PTY.
type Terminal struct {
	ID             string
	pty            *os.File
	cmd            *exec.Cmd
	done           chan struct{}
	tmuxSession    string         // if attached to a tmux session, track it for cleanup
	OutputCallback func([]byte)   // if set, output goes here instead of Wails events
	isGrouped      bool           // true for grouped tmux sessions (web terminals)
}

// Manager manages multiple terminal sessions.
type Manager struct {
	ctx       context.Context
	terminals map[string]*Terminal
	mu        sync.RWMutex
}

// NewManager creates a new terminal manager.
func NewManager() *Manager {
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

// CreateGroupedAttached creates a grouped tmux session linked to an existing session.
// This allows independent window sizing (phone won't shrink the desktop terminal).
// Output goes to the provided callback instead of Wails events.
func (m *Manager) CreateGroupedAttached(id, tmuxSession string, onOutput func([]byte)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.terminals[id]; exists {
		return fmt.Errorf("terminal %s already exists", id)
	}

	// Create a grouped session linked to the target session
	groupedName := "orion-web-" + id
	createCmd := exec.Command("tmux", "new-session", "-d", "-s", groupedName, "-t", tmuxSession)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux grouped session failed: err=%v out=%q", err, strings.TrimSpace(string(out)))
	}
	// window-size latest: whichever device is active determines the terminal size
	exec.Command("tmux", "set-option", "-g", "window-size", "latest").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "aggressive-resize", "on").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "status", "off").Run()
	exec.Command("tmux", "set-option", "-t", groupedName, "mouse", "on").Run()

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
	buf := make([]byte, 4096)
	eventName := fmt.Sprintf("terminal:output:%s", t.ID)

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
				if t.OutputCallback != nil {
					// Send raw bytes to callback (web terminal)
					out := make([]byte, n)
					copy(out, buf[:n])
					t.OutputCallback(out)
				} else if m.ctx != nil {
					// Base64 encode for safe JSON transport (Wails)
					encoded := base64.StdEncoding.EncodeToString(buf[:n])
					runtime.EventsEmit(m.ctx, eventName, encoded)
				}
			}
		}
	}
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
