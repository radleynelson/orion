package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orion/internal/config"
)

// Workspace represents a git worktree.
type Workspace struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	IsMain   bool   `json:"isMain"`
	HasAgent bool   `json:"hasAgent"`
}

// ProjectInfo contains metadata about the current project.
type ProjectInfo struct {
	Name       string `json:"name"`
	Root       string `json:"root"`
	MainBranch string `json:"mainBranch"`
}

// Manager handles workspace (worktree) operations. Bound to Wails.
type Manager struct {
	ctx context.Context
}

// NewManager creates a new workspace manager.
func NewManager() *Manager {
	return &Manager{}
}

// SetContext stores the Wails runtime context.
func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// --- Project info ---

// GetProjectInfo returns info about the current git project.
func (m *Manager) GetProjectInfo(path string) (*ProjectInfo, error) {
	root, err := getRepoRoot(path)
	if err != nil {
		return nil, err
	}
	return &ProjectInfo{
		Name:       filepath.Base(root),
		Root:       root,
		MainBranch: getMainBranch(root),
	}, nil
}

// --- Worktree operations ---

// ListWorkspaces returns all worktrees for a given repo.
func (m *Manager) ListWorkspaces(repoRoot string) ([]Workspace, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var workspaces []Workspace
	var current *Workspace
	isFirst := true

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != nil {
				current.IsMain = isFirst
				isFirst = false
				workspaces = append(workspaces, *current)
				current = nil
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "worktree "):
			path := strings.TrimPrefix(line, "worktree ")
			current = &Workspace{
				Path: path,
				Name: filepath.Base(path),
			}
		case strings.HasPrefix(line, "branch "):
			if current != nil {
				branch := strings.TrimPrefix(line, "branch ")
				branch = strings.TrimPrefix(branch, "refs/heads/")
				current.Branch = branch
			}
		case line == "bare":
			current = nil
			isFirst = false
		case line == "detached":
			if current != nil {
				current.Branch = "(detached)"
			}
		}
	}
	if current != nil {
		current.IsMain = isFirst
		workspaces = append(workspaces, *current)
	}

	// Check for running tmux sessions
	repoName := filepath.Base(repoRoot)
	for i := range workspaces {
		baseName := sessionName(repoName, workspaces[i].Name, 0)
		if hasSession(baseName) {
			workspaces[i].HasAgent = true
		}
	}

	return workspaces, nil
}

// CreateWorkspace creates a new worktree and copies credential files.
func (m *Manager) CreateWorkspace(repoRoot, name string) (*Workspace, error) {
	parentDir := filepath.Dir(repoRoot)
	repoName := filepath.Base(repoRoot)
	worktreePath := filepath.Join(parentDir, repoName+"-"+name)
	baseBranch := getMainBranch(repoRoot)

	mainPath := getMainWorktreePath(repoRoot)

	// Load config once for branch prefix and credential copying
	var cfg *config.OrionConfig
	if mainPath != "" {
		cfg = config.Load(mainPath)
	}

	// Apply branch prefix from config (e.g. "mckay" → branch "mckay/name")
	branchName := name
	if cfg != nil && cfg.BranchPrefix != "" {
		branchName = cfg.BranchPrefix + "/" + name
	}

	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, baseBranch)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}

	// Copy credential files
	if cfg != nil {
		copyCredentialFiles(mainPath, worktreePath, cfg.Credentials.Copy)
	}

	// Run setup script if exists
	if mainPath != "" {
		setupScript := filepath.Join(mainPath, ".worktree-setup.sh")
		if _, err := os.Stat(setupScript); err == nil {
			setupCmd := exec.Command("bash", setupScript)
			setupCmd.Dir = worktreePath
			setupCmd.Run() // best-effort
		}
	}

	return &Workspace{
		Name:   filepath.Base(worktreePath),
		Path:   worktreePath,
		Branch: branchName,
	}, nil
}

// DeleteWorkspace removes a worktree and kills its tmux sessions.
func (m *Manager) DeleteWorkspace(repoRoot, path string) error {
	name := filepath.Base(path)
	repoName := filepath.Base(repoRoot)
	baseName := sessionName(repoName, name, 0)

	// Kill all sessions for this workspace
	killSession(baseName)
	for i := 1; i <= 9; i++ {
		extra := fmt.Sprintf("%s-%d", baseName, i)
		if !hasSession(extra) {
			break
		}
		killSession(extra)
	}

	cmd := exec.Command("git", "worktree", "remove", path, "--force")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// GetConfig returns the .orion.toml config for a repo.
func (m *Manager) GetConfig(repoRoot string) *config.OrionConfig {
	return config.Load(repoRoot)
}

// --- tmux helpers ---

// LaunchAgent creates a tmux session and sends the agent command.
// Returns the tmux session name.
func (m *Manager) LaunchAgent(repoRoot string, workspacePath string, agentType string) (string, error) {
	repoName := filepath.Base(repoRoot)
	wsName := filepath.Base(workspacePath)

	cfg := config.Load(repoRoot)

	idx := nextSessionIndex(repoName, wsName)
	tmuxName := sessionName(repoName, wsName, idx)

	if !hasSession(tmuxName) {
		if err := createTmuxSession(tmuxName, workspacePath); err != nil {
			return "", err
		}
	}

	// Determine command from config or defaults
	var agentCmd string
	if agentCfg, ok := cfg.Agents[agentType]; ok {
		agentCmd = agentCfg.Command
	} else {
		switch agentType {
		case "claude":
			agentCmd = "claude --dangerously-skip-permissions"
		case "codex":
			agentCmd = "codex --dangerously-bypass-approvals-and-sandbox"
		}
	}

	// Source .orion/env.sh first so the agent has port awareness
	envFile := filepath.Join(workspacePath, ".orion", "env.sh")
	if _, err := os.Stat(envFile); err == nil {
		sendKeys(tmuxName, "source .orion/env.sh")
	}

	if agentCmd != "" {
		if err := sendKeys(tmuxName, agentCmd); err != nil {
			return "", err
		}
	}

	return tmuxName, nil
}

// LaunchShell creates a bare tmux session (no agent command).
func (m *Manager) LaunchShell(repoRoot string, workspacePath string) (string, error) {
	repoName := filepath.Base(repoRoot)
	wsName := filepath.Base(workspacePath)

	idx := nextSessionIndex(repoName, wsName)
	tmuxName := sessionName(repoName, wsName, idx)

	if !hasSession(tmuxName) {
		if err := createTmuxSession(tmuxName, workspacePath); err != nil {
			return "", err
		}
	}

	return tmuxName, nil
}

// --- internal helpers ---

func getRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

func getMainBranch(root string) string {
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", branch)
		cmd.Dir = root
		if err := cmd.Run(); err == nil {
			return branch
		}
	}
	return "main"
}

func getMainWorktreePath(repoRoot string) string {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			return strings.TrimPrefix(line, "worktree ")
		}
	}
	return ""
}

func sanitize(s string) string {
	r := strings.NewReplacer(".", "-", ":", "-", " ", "-", "/", "-")
	return r.Replace(s)
}

func sessionName(repoName, wsName string, index int) string {
	name := fmt.Sprintf("orion-%s-%s", sanitize(repoName), sanitize(wsName))
	if index > 0 {
		name = fmt.Sprintf("%s-%d", name, index)
	}
	return name
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

// createTmuxSessionForAgent creates a tmux session with mouse OFF so that
// TUI apps like Claude Code and Codex handle their own mouse/scroll events.

func sendKeys(name, keys string) error {
	if keys == "" {
		return exec.Command("tmux", "send-keys", "-t", name, "Enter").Run()
	}
	return exec.Command("tmux", "send-keys", "-t", name, keys, "Enter").Run()
}

func killSession(name string) error {
	if !hasSession(name) {
		return nil
	}
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

func nextSessionIndex(repoName, wsName string) int {
	baseName := sessionName(repoName, wsName, 0)
	if !hasSession(baseName) {
		return 0
	}
	for i := 1; i <= 9; i++ {
		name := fmt.Sprintf("%s-%d", baseName, i)
		if !hasSession(name) {
			return i
		}
	}
	return 9
}

func copyCredentialFiles(srcDir, dstDir string, patterns []string) {
	for _, pattern := range patterns {
		// Handle glob patterns (e.g., "backend/config/credentials/*.key")
		srcPattern := filepath.Join(srcDir, pattern)
		matches, err := filepath.Glob(srcPattern)
		if err != nil || len(matches) == 0 {
			// Try as direct file path
			matches = []string{filepath.Join(srcDir, pattern)}
		}

		for _, src := range matches {
			// Compute relative path from srcDir
			rel, err := filepath.Rel(srcDir, src)
			if err != nil {
				continue
			}
			dst := filepath.Join(dstDir, rel)

			// Only copy if source exists
			if _, err := os.Stat(src); err != nil {
				continue
			}

			// Create parent directories
			os.MkdirAll(filepath.Dir(dst), 0755)

			// Don't overwrite existing files
			if _, err := os.Stat(dst); err == nil {
				continue
			}

			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			os.WriteFile(dst, data, 0644)
		}
	}
}
