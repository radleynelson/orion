package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"orion/internal/applog"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ServerConfig describes how to launch a language server.
type ServerConfig struct {
	// Language identifier (e.g. "typescript", "go", "ruby")
	Language string `json:"language"`
	// Command to run (e.g. "typescript-language-server --stdio")
	Command string `json:"command"`
	// Executable and Args allow callers to avoid shell-style parsing for resolved commands.
	Executable string   `json:"executable,omitempty"`
	Args       []string `json:"args,omitempty"`
	// WorkDir is the process working directory, usually the workspace root.
	WorkDir string `json:"workDir,omitempty"`
	// File extensions this server handles
	Extensions []string `json:"extensions"`
	// Root URI for the workspace
	RootURI string `json:"rootUri"`
}

// server tracks a running LSP process.
type server struct {
	cfg     ServerConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	cancel  context.CancelFunc
	mu      sync.Mutex
	nextID  int
	pending map[int]chan json.RawMessage
}

// Manager manages LSP server processes.
type Manager struct {
	ctx     context.Context
	mu      sync.RWMutex
	servers map[string]*server // keyed by language
}

func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]*server),
	}
}

func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// StartServer launches an LSP server for the given config.
func (m *Manager) StartServer(cfg ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.servers[cfg.Language]; ok {
		return fmt.Errorf("server already running for %s", cfg.Language)
	}

	executable, args := cfg.Executable, cfg.Args
	if executable == "" {
		parts := strings.Fields(cfg.Command)
		if len(parts) > 0 {
			executable = parts[0]
			args = parts[1:]
		}
	}
	if executable == "" {
		return fmt.Errorf("empty command for %s", cfg.Language)
	}

	cmdPath, err := resolveExecutable(executable, cfg.WorkDir)
	if err != nil {
		return missingExecutableError(executable, cfg, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	cmd.Env = append(os.Environ(), "NODE_OPTIONS=--max-old-space-size=4096")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start %s: %w", cfg.Command, err)
	}

	srv := &server{
		cfg:     cfg,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		cancel:  cancel,
		pending: make(map[int]chan json.RawMessage),
	}

	m.servers[cfg.Language] = srv

	// Read LSP responses/notifications and forward to frontend
	go m.readLoop(srv)
	go logStderr(cfg.Language, stderr)

	return nil
}

// StopServer stops an LSP server for the given language.
func (m *Manager) StopServer(language string) error {
	m.mu.Lock()
	srv, ok := m.servers[language]
	if ok {
		delete(m.servers, language)
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}

	srv.cancel()
	srv.stdin.Close()
	return srv.cmd.Wait()
}

// StopAll stops all running LSP servers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	servers := make(map[string]*server)
	for k, v := range m.servers {
		servers[k] = v
	}
	m.servers = make(map[string]*server)
	m.mu.Unlock()

	for _, srv := range servers {
		srv.cancel()
		srv.stdin.Close()
		srv.cmd.Wait()
	}
}

// SendMessage sends a JSON-RPC message to the LSP server for the given language.
// The message should be a complete JSON-RPC request or notification.
func (m *Manager) SendMessage(language string, message string) error {
	m.mu.RLock()
	srv, ok := m.servers[language]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no server for %s", language)
	}

	return writeMessage(srv.stdin, []byte(message))
}

// SendRequest sends a JSON-RPC request and returns the response.
func (m *Manager) SendRequest(language string, method string, params interface{}) (string, error) {
	m.mu.RLock()
	srv, ok := m.servers[language]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("no server for %s", language)
	}

	srv.mu.Lock()
	srv.nextID++
	id := srv.nextID
	ch := make(chan json.RawMessage, 1)
	srv.pending[id] = ch
	srv.mu.Unlock()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	if err := writeMessage(srv.stdin, data); err != nil {
		srv.mu.Lock()
		delete(srv.pending, id)
		srv.mu.Unlock()
		return "", err
	}

	select {
	case resp := <-ch:
		return string(resp), nil
	case <-time.After(30 * time.Second):
		srv.mu.Lock()
		delete(srv.pending, id)
		srv.mu.Unlock()
		return "", fmt.Errorf("LSP request %s timed out for %s", method, language)
	}
}

// IsRunning checks if an LSP server is running for the given language.
func (m *Manager) IsRunning(language string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.servers[language]
	return ok
}

// ListRunning returns the languages with active LSP servers.
func (m *Manager) ListRunning() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var langs []string
	for k := range m.servers {
		langs = append(langs, k)
	}
	return langs
}

// readLoop reads LSP messages from stdout and emits them as Wails events.
func (m *Manager) readLoop(srv *server) {
	reader := bufio.NewReader(srv.stdout)
	for {
		msg, err := readMessage(reader)
		if err != nil {
			break
		}

		// Check if this is a response to a pending request
		var envelope struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(msg, &envelope); err == nil && envelope.ID != nil {
			srv.mu.Lock()
			ch, ok := srv.pending[*envelope.ID]
			if ok {
				delete(srv.pending, *envelope.ID)
			}
			srv.mu.Unlock()

			if ok {
				ch <- msg
				continue
			}
		}

		// Forward notifications and server-initiated requests to frontend
		if m.ctx != nil {
			wailsRuntime.EventsEmit(m.ctx, "lsp:message:"+srv.cfg.Language, string(msg))
		}
	}
}

func resolveExecutable(name string, workDir string) (string, error) {
	if filepath.IsAbs(name) || strings.ContainsRune(name, os.PathSeparator) {
		if filepath.IsAbs(name) {
			return executablePath(name)
		}
		if workDir != "" {
			return executablePath(filepath.Join(workDir, name))
		}
		return executablePath(name)
	}

	if workDir != "" {
		if path, err := executablePath(filepath.Join(workDir, "node_modules", ".bin", name)); err == nil {
			return path, nil
		}
	}

	return exec.LookPath(name)
}

func executablePath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if info.Mode()&0111 == 0 {
		return "", fmt.Errorf("%s is not executable", path)
	}
	return path, nil
}

func missingExecutableError(name string, cfg ServerConfig, err error) error {
	switch cfg.Language {
	case "typescript", "javascript", "typescriptreact", "javascriptreact":
		return fmt.Errorf("%s not found for TypeScript LSP. Install typescript-language-server in the project or configure [lsp.typescript].command: %w", name, err)
	}
	return fmt.Errorf("%s not found for %s LSP: %w", name, cfg.Language, err)
}

func logStderr(language string, stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			applog.Warnf("lsp stderr [%s]: %s", language, line)
		}
	}
	if err := scanner.Err(); err != nil {
		applog.Warnf("lsp stderr [%s] read failed: %v", language, err)
	}
}

// writeMessage writes a JSON-RPC message with Content-Length header.
func writeMessage(w io.Writer, data []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, err := io.WriteString(w, header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// readMessage reads a JSON-RPC message with Content-Length header.
func readMessage(reader *bufio.Reader) ([]byte, error) {
	contentLen := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(val)
			if err == nil {
				contentLen = n
			}
		}
	}

	if contentLen < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLen)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
