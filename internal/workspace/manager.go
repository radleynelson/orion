package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orion/internal/config"
	"orion/internal/notify"
	"orion/internal/tmuxutil"
	"orion/internal/workspacekey"
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
	if _, err := config.EnsureDefaultFile(root); err != nil {
		return nil, fmt.Errorf("create default %s: %w", config.FileName, err)
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
				applyWorkspaceMetadata(current)
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
		applyWorkspaceMetadata(current)
		current.IsMain = isFirst
		workspaces = append(workspaces, *current)
	}

	// Check for running tmux sessions
	repoName := filepath.Base(repoRoot)
	for i := range workspaces {
		baseName := sessionName(repoName, workspacekey.ID(workspaces[i].Path), 0)
		if hasSession(baseName) {
			workspaces[i].HasAgent = true
		}
	}

	return workspaces, nil
}

// CreateWorkspace creates a new worktree and copies credential files.
func (m *Manager) CreateWorkspace(repoRoot, name string) (*Workspace, error) {
	return m.CreateWorkspaceFrom(repoRoot, name, "")
}

// CreateWorkspaceFrom creates a new worktree from a caller-selected base ref.
func (m *Manager) CreateWorkspaceFrom(repoRoot, name string, baseRef string) (*Workspace, error) {
	repoName := filepath.Base(repoRoot)
	baseBranch := strings.TrimSpace(baseRef)
	if baseBranch == "" {
		baseBranch = getMainBranch(repoRoot)
	}

	mainPath := getMainWorktreePath(repoRoot)

	// Load config once for branch prefix, worktrees dir, and credential copying
	var cfg *config.OrionConfig
	if mainPath != "" {
		cfg = config.Load(mainPath)
	}

	// Resolve worktrees parent directory. Defaults to repo's sibling.
	// Config can override with `worktrees_dir` (supports ~, $VARS, and
	// relative paths resolved against the repo root).
	parentDir := filepath.Dir(repoRoot)
	if cfg != nil && cfg.WorktreesDir != "" {
		resolved := os.ExpandEnv(cfg.WorktreesDir)
		if strings.HasPrefix(resolved, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				resolved = filepath.Join(home, strings.TrimPrefix(resolved, "~"))
			}
		}
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(repoRoot, resolved)
		}
		if err := os.MkdirAll(resolved, 0755); err != nil {
			return nil, fmt.Errorf("failed to create worktrees dir: %w", err)
		}
		parentDir = resolved
	}
	// Apply branch prefix from config (e.g. "mckay" → branch "mckay/name")
	branchName := name
	if cfg != nil && cfg.BranchPrefix != "" {
		branchName = cfg.BranchPrefix + "/" + name
	}

	layout := ""
	if cfg != nil {
		layout = strings.ToLower(strings.TrimSpace(cfg.WorktreeLayout))
	}
	workspaceID := repoName + "-" + name
	displayName := workspaceID
	worktreePath := filepath.Join(parentDir, displayName)
	managedBy := "orion"
	gitArgs := []string{"worktree", "add", "-b", branchName, worktreePath, baseBranch}
	if layout == "codex" {
		shortID, err := uniqueWorktreeID(parentDir)
		if err != nil {
			return nil, err
		}
		workspaceID = repoName + "-" + shortID
		displayName = repoName + "-" + name
		worktreePath = filepath.Join(parentDir, shortID, repoName)
		if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create managed worktree parent: %w", err)
		}
		gitArgs = []string{"worktree", "add", "--detach", worktreePath, baseBranch}
	}

	m.emitWorkspaceCreateProgress(worktreePath, "Creating git worktree")
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		if layout == "codex" {
			_ = os.Remove(filepath.Dir(worktreePath))
		}
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}

	metadata := workspacekey.Metadata{
		ID:               workspaceID,
		Name:             displayName,
		EnvironmentName:  name,
		Branch:           branchName,
		BaseRef:          baseBranch,
		MainWorktreePath: mainPath,
		ManagedBy:        managedBy,
	}
	if err := workspacekey.Save(worktreePath, metadata); err != nil {
		return nil, fmt.Errorf("write workspace metadata: %w", err)
	}
	if err := m.prepareWorkspace(repoRoot, worktreePath, cfg, &metadata); err != nil {
		return nil, err
	}

	m.emitWorkspaceCreateProgress(worktreePath, "Ready")
	resultBranch := branchName
	if layout == "codex" {
		resultBranch = "(detached)"
	}
	return &Workspace{
		Name:   displayName,
		Path:   worktreePath,
		Branch: resultBranch,
	}, nil
}

// AdoptWorkspace provisions an existing non-main git worktree with the same
// credentials, hooks, and setup script used for an Orion-created workspace.
// It is idempotent so Codex may call it whenever an environment is opened.
func (m *Manager) AdoptWorkspace(path string) (*Workspace, error) {
	worktreePath, err := getRepoRoot(path)
	if err != nil {
		return nil, err
	}
	mainPath := getMainWorktreePath(worktreePath)
	if mainPath == "" {
		return nil, fmt.Errorf("cannot find main worktree for %s", worktreePath)
	}
	if filepath.Clean(mainPath) == filepath.Clean(worktreePath) {
		return nil, fmt.Errorf("the main worktree does not need adoption")
	}

	cfg := config.Load(mainPath)
	metadata, ok := workspacekey.Load(worktreePath)
	if !ok {
		repoName := filepath.Base(mainPath)
		shortID := filepath.Base(filepath.Dir(worktreePath))
		if shortID == "." || shortID == string(filepath.Separator) || shortID == repoName {
			shortID = shortHash(worktreePath)
		}
		envName := "codex-" + sanitize(shortID)
		branchName := envName
		if cfg.BranchPrefix != "" {
			branchName = cfg.BranchPrefix + "/" + envName
		}
		metadata = workspacekey.Metadata{
			ID:               repoName + "-" + sanitize(shortID),
			Name:             repoName + "-" + envName,
			EnvironmentName:  envName,
			Branch:           branchName,
			BaseRef:          getMainBranch(mainPath),
			MainWorktreePath: mainPath,
			ManagedBy:        "codex",
		}
		if err := workspacekey.Save(worktreePath, metadata); err != nil {
			return nil, fmt.Errorf("write workspace metadata: %w", err)
		}
	}
	if err := m.prepareWorkspace(mainPath, worktreePath, cfg, &metadata); err != nil {
		return nil, err
	}

	return &Workspace{
		Name:   metadata.Name,
		Path:   worktreePath,
		Branch: displayBranch(worktreePath),
	}, nil
}

// CleanupAdoptedWorkspace runs Orion's deletion hook without removing the Git
// worktree. Codex owns removal of worktrees that it created.
func (m *Manager) CleanupAdoptedWorkspace(path string) error {
	worktreePath, err := getRepoRoot(path)
	if err != nil {
		// Codex may remove the checkout immediately after invoking cleanup. If
		// metadata is still reachable at the supplied path, use it directly.
		worktreePath = filepath.Clean(path)
	}
	metadata, ok := workspacekey.Load(worktreePath)
	if !ok || !metadata.Provisioned {
		return nil
	}
	mainPath := metadata.MainWorktreePath
	if mainPath == "" {
		mainPath = getMainWorktreePath(worktreePath)
	}
	cfg := config.Load(mainPath)
	if err := runHook(hookWorktreeDeleting, cfg.Hooks.WorktreeDeleting, hookContextForMetadata(worktreePath, mainPath, metadata), false); err != nil {
		return err
	}
	metadata.Provisioned = false
	metadata.ProvisionedAt = ""
	return workspacekey.Save(worktreePath, metadata)
}

// DeleteWorkspace removes a worktree and kills its tmux sessions.
func (m *Manager) DeleteWorkspace(repoRoot, path string) error {
	name := workspacekey.ID(path)
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

	mainPath := getMainWorktreePath(repoRoot)
	if mainPath == "" {
		mainPath = repoRoot
	}
	cfg := config.Load(mainPath)
	hookCtx := hookContext{
		Name:             workspaceNameFromPath(repoName, path),
		Branch:           getWorktreeBranch(path),
		WorkspacePath:    path,
		RepoRoot:         repoRoot,
		MainWorktreePath: mainPath,
	}
	if metadata, ok := workspacekey.Load(path); ok {
		hookCtx = hookContextForMetadata(path, mainPath, metadata)
	}
	if err := runHook(hookWorktreeDeleting, cfg.Hooks.WorktreeDeleting, hookCtx, false); err != nil {
		return err
	}

	cmd := exec.Command("git", "worktree", "remove", path, "--force")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// If the worktree directory is already gone (manually deleted, moved, or its
	// .git file is missing), `git worktree remove` refuses with a validation
	// error. In that case, prune the stale admin entry and clean up the dir if
	// any leftover exists.
	msg := strings.TrimSpace(string(out))
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) || strings.Contains(msg, "validation failed") {
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = repoRoot
		if pruneOut, pruneErr := pruneCmd.CombinedOutput(); pruneErr != nil {
			return fmt.Errorf("worktree remove failed: %s; prune also failed: %s", msg, strings.TrimSpace(string(pruneOut)))
		}
		// Best-effort cleanup of any leftover directory contents.
		_ = os.RemoveAll(path)
		return nil
	}

	return fmt.Errorf("%s", msg)
}

// GetConfig returns the .orion.toml config for a repo.
func (m *Manager) GetConfig(repoRoot string) *config.OrionConfig {
	return config.Load(repoRoot)
}

// --- tmux helpers ---

// LaunchAgent creates a tmux session and sends the agent command.
// Returns the tmux session name.
func (m *Manager) LaunchAgent(repoRoot string, workspacePath string, agentType string) (string, error) {
	cfg := config.Load(repoRoot)

	// Determine command from config or defaults
	var agentCmd string
	var agentProvider string
	var agentLabel string
	var agentIcon string
	var agentInitialPrompt string
	if agentCfg, ok := cfg.Agents[agentType]; ok {
		agentCmd = agentCfg.Command
		agentProvider = agentCfg.Provider
		agentLabel = agentCfg.Label
		agentIcon = agentCfg.Icon
		agentInitialPrompt = strings.TrimSpace(agentCfg.InitialPrompt)
	} else {
		switch agentType {
		case "claude":
			agentCmd = "claude --dangerously-skip-permissions"
			agentProvider = "claude"
			agentLabel = "Claude"
		case "codex":
			agentCmd = "codex --dangerously-bypass-approvals-and-sandbox"
			agentProvider = "codex"
			agentLabel = "Codex"
		}
	}

	sessionType := normalizeSessionType(agentProvider)
	if sessionType == "" {
		sessionType = normalizeSessionType(agentType)
	}
	if sessionType == "" {
		sessionType = sessionTypeForCommand(agentCmd)
	}
	if sessionType == "claude" && !strings.Contains(agentCmd, "--resume") && !strings.Contains(agentCmd, "--session-id") {
		if sessionID := randomUUID(); sessionID != "" {
			agentCmd = strings.TrimSpace(agentCmd + " --session-id " + shellQuote(sessionID))
		}
	}

	if agentInitialPrompt != "" {
		agentCmd = strings.TrimSpace(agentCmd + " " + shellQuote(agentInitialPrompt))
	}

	// Install notification hooks for Claude so Orion sees Stop/Notification events.
	if sessionType == "claude" {
		if scriptPath, err := notify.InstallHookScript(); err == nil {
			notify.InstallWorkspaceHooks(workspacePath, scriptPath)
		}
	}

	label := strings.TrimSpace(agentLabel)
	if label == "" {
		label = labelForType(sessionType)
	}
	return m.launchCommand(repoRoot, workspacePath, agentCmd, sessionType, label, agentIcon)
}

// LaunchCommand creates a tmux session and sends a specific command.
func (m *Manager) LaunchCommand(repoRoot string, workspacePath string, command string) (string, error) {
	sessionType := sessionTypeForCommand(command)
	return m.launchCommand(repoRoot, workspacePath, command, sessionType, labelForType(sessionType), "")
}

func (m *Manager) launchCommand(repoRoot string, workspacePath string, command string, sessionType string, label string, icon string) (string, error) {
	repoName := filepath.Base(repoRoot)
	wsName := workspacekey.ID(workspacePath)

	idx := nextSessionIndex(repoName, wsName)
	tmuxName := sessionName(repoName, wsName, idx)

	if !hasSession(tmuxName) {
		if err := createTmuxSession(tmuxName, workspacePath, initialCommand(workspacePath, command)); err != nil {
			return "", err
		}
	}
	markTmuxSession(tmuxName, sessionType, label, workspacePath, command, icon)

	return tmuxName, nil
}

// LaunchShell creates a bare tmux session (no agent command).
func (m *Manager) LaunchShell(repoRoot string, workspacePath string) (string, error) {
	repoName := filepath.Base(repoRoot)
	wsName := workspacekey.ID(workspacePath)

	idx := nextSessionIndex(repoName, wsName)
	tmuxName := sessionName(repoName, wsName, idx)

	if !hasSession(tmuxName) {
		if err := createTmuxSession(tmuxName, workspacePath, ""); err != nil {
			return "", err
		}
	}
	markTmuxSession(tmuxName, "shell", "Shell", workspacePath, "", "")

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

// MainWorktreePath returns the first worktree registered for the repository.
func MainWorktreePath(path string) string {
	return getMainWorktreePath(path)
}

func (m *Manager) prepareWorkspace(repoRoot, worktreePath string, cfg *config.OrionConfig, metadata *workspacekey.Metadata) error {
	if metadata.Provisioned {
		return nil
	}
	mainPath := metadata.MainWorktreePath
	if mainPath == "" {
		mainPath = getMainWorktreePath(repoRoot)
		metadata.MainWorktreePath = mainPath
	}
	if cfg != nil {
		m.emitWorkspaceCreateProgress(worktreePath, "Copying credentials")
		copyCredentialFiles(mainPath, worktreePath, cfg.Credentials.Copy)
		if strings.TrimSpace(cfg.Hooks.WorktreeCreated.Command) != "" {
			m.emitWorkspaceCreateProgress(worktreePath, "Running setup hook")
		}
		if err := runHook(hookWorktreeCreated, cfg.Hooks.WorktreeCreated, hookContextForMetadata(worktreePath, mainPath, *metadata), true); err != nil {
			return err
		}
	}
	if mainPath != "" {
		setupScript := filepath.Join(mainPath, ".worktree-setup.sh")
		if _, err := os.Stat(setupScript); err == nil {
			setupCmd := exec.Command("bash", setupScript)
			setupCmd.Dir = worktreePath
			_ = setupCmd.Run()
		}
	}
	metadata.Provisioned = true
	if err := workspacekey.Save(worktreePath, *metadata); err != nil {
		return fmt.Errorf("mark workspace provisioned: %w", err)
	}
	return nil
}

func hookContextForMetadata(worktreePath, mainPath string, metadata workspacekey.Metadata) hookContext {
	return hookContext{
		Name:             metadata.EnvironmentName,
		Branch:           metadata.Branch,
		BaseRef:          metadata.BaseRef,
		WorkspacePath:    worktreePath,
		RepoRoot:         mainPath,
		MainWorktreePath: mainPath,
	}
}

func applyWorkspaceMetadata(workspace *Workspace) {
	if metadata, ok := workspacekey.Load(workspace.Path); ok && strings.TrimSpace(metadata.Name) != "" {
		workspace.Name = metadata.Name
	}
}

func displayBranch(worktreePath string) string {
	branch := getWorktreeBranch(worktreePath)
	if branch == "" {
		return "(detached)"
	}
	return branch
}

func uniqueWorktreeID(parentDir string) (string, error) {
	for i := 0; i < 64; i++ {
		var bytes [2]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return "", fmt.Errorf("generate managed worktree id: %w", err)
		}
		id := hex.EncodeToString(bytes[:])
		if _, err := os.Stat(filepath.Join(parentDir, id)); os.IsNotExist(err) {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not allocate a unique managed worktree id")
}

func shortHash(value string) string {
	var bytes [2]byte
	copy(bytes[:], []byte(value))
	for i := range value {
		bytes[i%len(bytes)] ^= value[i]
	}
	return hex.EncodeToString(bytes[:])
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

func createTmuxSession(name, workDir, initialCommand string) error {
	tmuxutil.ConfigureExtendedKeys()
	args := []string{"new-session", "-d", "-s", name, "-c", workDir}
	if wrapped := tmuxutil.WrapInitialCommand(initialCommand); wrapped != "" {
		args = append(args, wrapped)
	}
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	exec.Command("tmux", "set-option", "-t", name, "history-limit", "50000").Run()
	exec.Command("tmux", "set-option", "-t", name, "mouse", "on").Run()
	exec.Command("tmux", "set-option", "-t", name, "status", "off").Run()
	exec.Command("tmux", "set-option", "-t", name, "set-clipboard", "on").Run()
	tmuxutil.ConfigureSessionExtendedKeys(name)
	exec.Command("tmux", "bind-key", "-T", "copy-mode", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", "pbcopy").Run()
	exec.Command("tmux", "bind-key", "-T", "copy-mode-vi", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", "pbcopy").Run()
	return nil
}

func initialCommand(workspacePath string, command string) string {
	command = strings.TrimSpace(command)
	envFile := filepath.Join(workspacePath, ".orion", "env.sh")
	if _, err := os.Stat(envFile); err != nil {
		return command
	}
	if command == "" {
		return ". " + shellQuote(envFile)
	}
	return ". " + shellQuote(envFile) + "\n" + command
}

// createTmuxSessionForAgent creates a tmux session with mouse OFF so that
// TUI apps like Claude Code and Codex handle their own mouse/scroll events.

func markTmuxSession(name string, sessionType string, label string, workspacePath string, command string, icon string) {
	sessionType = normalizeSessionType(sessionType)
	if sessionType == "" {
		sessionType = "shell"
	}
	if strings.TrimSpace(label) == "" {
		label = labelForType(sessionType)
	}
	setTmuxOption(name, "@orion_type", sessionType)
	if sessionType == "claude" || sessionType == "codex" {
		setTmuxOption(name, "@orion_provider", sessionType)
	}
	setTmuxOption(name, "@orion_label", label)
	setTmuxOption(name, "@orion_icon", strings.TrimSpace(icon))
	setTmuxOption(name, "@orion_workspace", workspacePath)
	setTmuxOption(name, "@orion_started_at_unix_nano", fmt.Sprintf("%d", time.Now().UnixNano()))
	if command != "" {
		setTmuxOption(name, "@orion_command", command)
		if threadID := threadIDForResumeCommand(sessionType, command); threadID != "" {
			setTmuxOption(name, "@orion_thread_id", threadID)
		}
	}
}

func setTmuxOption(session string, option string, value string) {
	if strings.TrimSpace(session) == "" || strings.TrimSpace(option) == "" {
		return
	}
	exec.Command("tmux", "set-option", "-t", session, option, value).Run()
}

func sessionTypeForCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "shell"
	}
	if sessionType := normalizeSessionType(filepath.Base(fields[0])); sessionType != "" {
		return sessionType
	}
	return "shell"
}

func threadIDForResumeCommand(sessionType string, command string) string {
	sessionType = normalizeSessionType(sessionType)
	switch sessionType {
	case "claude":
		return firstFlagValue(command, "--resume", "-r", "--session-id")
	case "codex":
		for _, id := range codexResumeIDs(command) {
			if strings.TrimSpace(id) != "" {
				return id
			}
		}
	}
	return ""
}

func firstFlagValue(command string, flags ...string) string {
	fields := strings.Fields(command)
	for i, field := range fields {
		for _, flag := range flags {
			switch {
			case field == flag:
				if i+1 < len(fields) {
					return unquoteShellField(fields[i+1])
				}
			case strings.HasPrefix(field, flag+"="):
				return unquoteShellField(strings.TrimPrefix(field, flag+"="))
			}
		}
	}
	return ""
}

func codexResumeIDs(command string) []string {
	if !strings.Contains(command, "codex") {
		return nil
	}
	fields := strings.Fields(command)
	var ids []string
	for i, field := range fields {
		switch {
		case field == "resume":
			if id := codexResumePositionalID(fields[i+1:]); id != "" {
				ids = append(ids, id)
			}
		case field == "--resume" || field == "-r":
			if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
				ids = append(ids, unquoteShellField(fields[i+1]))
			}
		case strings.HasPrefix(field, "--resume="):
			ids = append(ids, unquoteShellField(strings.TrimPrefix(field, "--resume=")))
		}
	}
	return ids
}

func codexResumePositionalID(fields []string) string {
	for i := len(fields) - 1; i >= 0; i-- {
		field := strings.TrimSpace(fields[i])
		if field == "" || strings.HasPrefix(field, "-") {
			continue
		}
		if i > 0 && codexResumeFlagConsumesValue(fields[i-1]) {
			continue
		}
		return unquoteShellField(field)
	}
	return ""
}

func codexResumeFlagConsumesValue(field string) bool {
	switch strings.TrimSpace(field) {
	case "-m", "--model", "-c", "--config", "-s", "--sandbox", "-C", "--cd", "--profile":
		return true
	default:
		return false
	}
}

func unquoteShellField(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func normalizeSessionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "shell":
		return "shell"
	default:
		return ""
	}
}

func labelForType(sessionType string) string {
	switch sessionType {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	default:
		return "Shell"
	}
}

func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:32])
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`!*?[]{}()<>|&;") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
