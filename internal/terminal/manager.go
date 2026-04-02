package terminal

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Terminal represents a single terminal session backed by a PTY.
type Terminal struct {
	ID   string
	pty  *os.File
	cmd  *exec.Cmd
	done chan struct{}
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
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

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

// CreateAttached spawns a terminal that attaches to an existing tmux session.
func (m *Manager) CreateAttached(id, tmuxSession string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.terminals[id]; exists {
		return fmt.Errorf("terminal %s already exists", id)
	}

	cmd := exec.Command("tmux", "attach-session", "-t", tmuxSession)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to attach to tmux session %s: %w", tmuxSession, err)
	}

	t := &Terminal{
		ID:   id,
		pty:  ptmx,
		cmd:  cmd,
		done: make(chan struct{}),
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

// Close terminates a terminal session.
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

	return nil
}

// CloseAll closes all terminal sessions.
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
				if m.ctx != nil {
					runtime.EventsEmit(m.ctx, fmt.Sprintf("terminal:exit:%s", t.ID))
				}
				return
			}
			if n > 0 && m.ctx != nil {
				// Base64 encode for safe JSON transport
				encoded := base64.StdEncoding.EncodeToString(buf[:n])
				runtime.EventsEmit(m.ctx, eventName, encoded)
			}
		}
	}
}
