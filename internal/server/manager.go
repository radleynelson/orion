package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"orion/internal/config"
	"orion/internal/port"
)

// ServerStatus represents the state of a server for a workspace.
type ServerStatus struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Running bool   `json:"running"`
	TmuxSession string `json:"tmuxSession"`
}

// Manager handles server process lifecycle.
type Manager struct {
	ctx      context.Context
	portReg  *port.Registry
}

// NewManager creates a new server manager.
func NewManager(portReg *port.Registry) *Manager {
	return &Manager{
		portReg: portReg,
	}
}

// SetContext stores the Wails runtime context.
func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// AllocatePorts pre-allocates ports for a workspace and writes .orion/env.sh
// so that agents and shells know the ports before servers are started.
func (m *Manager) AllocatePorts(repoRoot string, workspacePath string, isMain bool) error {
	cfg := config.Load(repoRoot)
	if len(cfg.Servers) == 0 {
		return nil
	}

	wsID := filepath.Base(workspacePath)

	// Check if already allocated
	existing := m.portReg.GetAllocation(wsID)
	if existing != nil {
		// Already allocated, just ensure env file exists
		redisDB := 1 // main default
		if !isMain {
			if db, ok := m.portReg.GetRedisDB(wsID); ok {
				redisDB = db
			} else {
				db, _ := m.portReg.AllocateRedisDB(wsID)
				redisDB = db
			}
		}
		writeEnvFile(workspacePath, existing, cfg, redisDB)
		return nil
	}

	var alloc port.Allocation
	redisDB := 1 // main uses DB 1
	if isMain {
		alloc = make(port.Allocation)
		for name, srv := range cfg.Servers {
			if srv.DefaultPort > 0 {
				alloc[name] = srv.DefaultPort
			}
		}
	} else {
		var portServers []string
		for name, srv := range cfg.Servers {
			if srv.DefaultPort > 0 {
				portServers = append(portServers, name)
			}
		}
		var err error
		alloc, err = m.portReg.AllocateForWorkspace(wsID, portServers)
		if err != nil {
			return err
		}
		db, err := m.portReg.AllocateRedisDB(wsID)
		if err == nil {
			redisDB = db
		}
	}

	writeEnvFile(workspacePath, alloc, cfg, redisDB)
	return nil
}

// StartServers starts all configured servers for a workspace on isolated ports.
// If isMain is true, uses the default_port from config instead of random ports.
// Returns the list of server statuses with assigned ports.
func (m *Manager) StartServers(repoRoot string, workspacePath string, isMain bool) ([]ServerStatus, error) {
	cfg := config.Load(repoRoot)
	if len(cfg.Servers) == 0 {
		return nil, fmt.Errorf("no servers configured in .orion.toml")
	}

	wsID := filepath.Base(workspacePath)

	var alloc port.Allocation

	if isMain {
		// Main branch uses default ports from config
		alloc = make(port.Allocation)
		for name, srv := range cfg.Servers {
			if srv.DefaultPort > 0 {
				alloc[name] = srv.DefaultPort
			}
		}
	} else {
		// Worktrees get random isolated ports
		var portServers []string
		for name, srv := range cfg.Servers {
			if srv.DefaultPort > 0 {
				portServers = append(portServers, name)
			}
		}
		var err error
		alloc, err = m.portReg.AllocateForWorkspace(wsID, portServers)
		if err != nil {
			return nil, fmt.Errorf("port allocation failed: %w", err)
		}
	}

	// Allocate Redis DB
	redisDB := 1 // main uses DB 1
	if !isMain {
		db, err := m.portReg.AllocateRedisDB(wsID)
		if err == nil {
			redisDB = db
		}
	}

	// Write .orion/env.sh so agents and shells know the ports
	writeEnvFile(workspacePath, alloc, cfg, redisDB)

	var statuses []ServerStatus

	for name, srv := range cfg.Servers {
		tmuxName := fmt.Sprintf("orion-srv-%s-%s", wsID, name)

		// Kill existing process on this port if needed
		if assignedPort, ok := alloc[name]; ok {
			killProcessOnPort(assignedPort)
		}

		// Kill existing tmux session
		if hasSession(tmuxName) {
			killSession(tmuxName)
		}

		// Clean up stale lock files (e.g., Next.js .next/dev/lock)
		cleanStaleLocks(workspacePath, srv.Dir)

		// Determine working directory
		workDir := workspacePath
		if srv.Dir != "" {
			workDir = filepath.Join(workspacePath, srv.Dir)
		}

		// Create tmux session
		if err := createTmuxSession(tmuxName, workDir); err != nil {
			statuses = append(statuses, ServerStatus{
				Name: name, Running: false, TmuxSession: tmuxName,
			})
			continue
		}

		// Build environment command prefix
		envParts := buildEnvString(name, srv, alloc, cfg, redisDB)

		// Send the command
		fullCmd := envParts + srv.Command
		if err := sendKeys(tmuxName, fullCmd); err != nil {
			statuses = append(statuses, ServerStatus{
				Name: name, Running: false, TmuxSession: tmuxName,
			})
			continue
		}

		assignedPort := alloc[name]
		statuses = append(statuses, ServerStatus{
			Name:        name,
			Port:        assignedPort,
			Running:     true,
			TmuxSession: tmuxName,
		})
	}

	return statuses, nil
}

// StopServers stops all servers for a workspace.
func (m *Manager) StopServers(workspacePath string) error {
	wsID := filepath.Base(workspacePath)

	// Find and kill all server tmux sessions for this workspace
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	prefix := fmt.Sprintf("orion-srv-%s-", wsID)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			killSession(line)
		}
	}

	// Ports and env file stay — they belong to the workspace, not the server lifecycle.
	// Only released when the workspace/worktree is deleted.

	return nil
}

// GetServerStatuses returns the current status of all servers for a workspace.
func (m *Manager) GetServerStatuses(repoRoot string, workspacePath string) []ServerStatus {
	cfg := config.Load(repoRoot)
	wsID := filepath.Base(workspacePath)
	alloc := m.portReg.GetAllocation(wsID)

	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	var statuses []ServerStatus
	for _, name := range names {
		tmuxName := fmt.Sprintf("orion-srv-%s-%s", wsID, name)
		assignedPort := 0
		if alloc != nil {
			assignedPort = alloc[name]
		}
		statuses = append(statuses, ServerStatus{
			Name:        name,
			Port:        assignedPort,
			Running:     hasSession(tmuxName),
			TmuxSession: tmuxName,
		})
	}
	return statuses
}

// GetPortAllocations returns port allocations for all workspaces.
func (m *Manager) GetPortAllocations() map[string]port.Allocation {
	return m.portReg.GetAllAllocations()
}

// --- helpers ---

func buildEnvString(serverName string, srv config.ServerConfig, alloc port.Allocation, cfg *config.OrionConfig, redisDB int) string {
	var parts []string

	// Set the port env var
	if srv.PortEnv != "" {
		if p, ok := alloc[serverName]; ok {
			parts = append(parts, fmt.Sprintf("%s=%d", srv.PortEnv, p))
		}
	}

	// Redis DB
	if redisDB > 0 {
		parts = append(parts, fmt.Sprintf("REDIS_DB=%d", redisDB))
	}

	// Resolve cross-server env vars with template syntax
	for key, val := range srv.Env {
		resolved := resolveTemplate(val, alloc)
		parts = append(parts, fmt.Sprintf("%s=%s", key, resolved))
	}

	if len(parts) == 0 {
		return ""
	}
	// Use export so vars persist across && chains in the command
	var exports []string
	for _, p := range parts {
		exports = append(exports, "export "+p)
	}
	return strings.Join(exports, " && ") + " && "
}

// resolveTemplate replaces {{serverName.port}} with actual port values.
func resolveTemplate(tmpl string, alloc port.Allocation) string {
	result := tmpl
	for name, p := range alloc {
		placeholder := fmt.Sprintf("{{%s.port}}", name)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%d", p))
	}
	return result
}

func killProcessOnPort(port int) {
	cmd := exec.Command("lsof", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return
	}
	for _, pid := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid = strings.TrimSpace(pid)
		if pid != "" {
			exec.Command("kill", "-9", pid).Run()
		}
	}
}

// cleanStaleLocks removes lock files that can prevent servers from starting.
func cleanStaleLocks(workspacePath, serverDir string) {
	dir := workspacePath
	if serverDir != "" {
		dir = filepath.Join(workspacePath, serverDir)
	}
	// Next.js lock
	os.Remove(filepath.Join(dir, ".next", "dev", "lock"))
	// Vite lock
	os.Remove(filepath.Join(dir, "node_modules", ".vite", "deps", "_lock"))
	// Rails tmp/pids
	os.Remove(filepath.Join(dir, "tmp", "pids", "server.pid"))
}

func hasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

func createTmuxSession(name, workDir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	exec.Command("tmux", "set-option", "-t", name, "history-limit", "50000").Run()
	exec.Command("tmux", "set-option", "-t", name, "mouse", "on").Run()
	exec.Command("tmux", "set-option", "-t", name, "status", "off").Run()
	return nil
}

func sendKeys(name, keys string) error {
	return exec.Command("tmux", "send-keys", "-t", name, keys, "Enter").Run()
}

func killSession(name string) error {
	if !hasSession(name) {
		return nil
	}
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

// writeEnvFile creates .orion/env.sh in the workspace with all port assignments.
// This file is auto-sourced by agent and shell sessions so they know the ports.
func writeEnvFile(workspacePath string, alloc port.Allocation, cfg *config.OrionConfig, redisDB int) {
	dir := filepath.Join(workspacePath, ".orion")
	os.MkdirAll(dir, 0755)
	envPath := filepath.Join(dir, "env.sh")

	var lines []string
	lines = append(lines, "# Auto-generated by Orion — do not edit")
	lines = append(lines, "# Sourced automatically in all Orion terminal sessions")
	lines = append(lines, "")

	// Sort server names for consistent output
	var names []string
	for name := range alloc {
		names = append(names, name)
	}
	sort.Strings(names)

	// Port assignments
	for _, name := range names {
		p := alloc[name]
		upper := strings.ToUpper(name)
		lines = append(lines, fmt.Sprintf("export %s_PORT=%d", upper, p))
		lines = append(lines, fmt.Sprintf("export %s_URL=http://localhost:%d", upper, p))
	}

	// Redis DB assignment
	if redisDB > 0 {
		lines = append(lines, fmt.Sprintf("export REDIS_DB=%d", redisDB))
	}

	// Resolved cross-server env vars from config
	for srvName, srv := range cfg.Servers {
		for key, val := range srv.Env {
			resolved := resolveTemplate(val, alloc)
			lines = append(lines, fmt.Sprintf("export %s=%s", key, resolved))
			_ = srvName // just iterating
		}
	}

	lines = append(lines, "")
	os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0644)

	// Also add .orion/ to .gitignore if not already there
	ensureGitignore(workspacePath, ".orion/")
}

func removeEnvFile(workspacePath string) {
	envPath := filepath.Join(workspacePath, ".orion", "env.sh")
	os.Remove(envPath)
}

func ensureGitignore(workspacePath, pattern string) {
	gitignorePath := filepath.Join(workspacePath, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err == nil {
		if strings.Contains(string(data), pattern) {
			return // already ignored
		}
	}
	// Append to .gitignore
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(data) > 0 && data[len(data)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(pattern + "\n")
}
