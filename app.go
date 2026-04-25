package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"orion/internal/chatattachments"
	claudechat "orion/internal/claudesdk"
	"orion/internal/codexchat"
	"orion/internal/config"
	"orion/internal/diag"
	"orion/internal/files"
	"orion/internal/git"
	"orion/internal/notify"
	"orion/internal/port"
	"orion/internal/server"
	"orion/internal/state"
	"orion/internal/terminal"
	"orion/internal/watcher"
	"orion/internal/web"
	"orion/internal/workspace"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct bound to Wails.
type App struct {
	ctx        context.Context
	termMgr    *terminal.Manager
	claudeMgr  *claudechat.Manager
	codexMgr   *codexchat.Manager
	wsMgr      *workspace.Manager
	srvMgr     *server.Manager
	portReg    *port.Registry
	appState   *state.AppState
	filesMgr   *files.Manager
	gitMgr     *git.Manager
	watcherMgr *watcher.Manager
	webSrv     *web.Server
	notifier   *notify.Notifier
	diagMgr    *diag.Manager
}

// NewApp creates a new App instance.
func NewApp() *App {
	portReg := port.NewRegistry()
	return &App{
		termMgr:    terminal.NewManager(),
		claudeMgr:  claudechat.NewManager(),
		codexMgr:   codexchat.NewManager(),
		wsMgr:      workspace.NewManager(),
		srvMgr:     server.NewManager(portReg),
		portReg:    portReg,
		appState:   state.NewAppState(),
		filesMgr:   files.NewManager(),
		gitMgr:     git.NewManager(),
		watcherMgr: watcher.NewManager(),
		notifier:   notify.New(nil),
		diagMgr:    diag.NewManager(),
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Fix PATH for macOS dock launches — the dock uses a minimal PATH that
	// doesn't include /opt/homebrew/bin, /usr/local/bin, etc.
	path := os.Getenv("PATH")
	for _, p := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/homebrew/sbin", "/usr/local/sbin"} {
		if !strings.Contains(path, p) {
			path = p + ":" + path
		}
	}
	os.Setenv("PATH", path)

	a.termMgr.SetContext(ctx)
	a.claudeMgr.SetContext(ctx)
	a.codexMgr.SetContext(ctx)
	a.wsMgr.SetContext(ctx)
	a.srvMgr.SetContext(ctx)
	a.filesMgr.SetContext(ctx)
	a.gitMgr.SetContext(ctx)
	a.watcherMgr.SetContext(ctx)
	a.codexMgr.SetListener(func(sessionID string, message codexchat.Message) {
		wailsRuntime.EventsEmit(ctx, "codex-chat:message:"+sessionID, message)
		wailsRuntime.EventsEmit(ctx, "codex-chat:message", message)
	})
	a.claudeMgr.SetListener(func(sessionID string, message claudechat.Message) {
		wailsRuntime.EventsEmit(ctx, "claude-chat:message:"+sessionID, message)
		wailsRuntime.EventsEmit(ctx, "claude-chat:message", message)
	})
	a.notifier.SetContext(ctx)
	a.diagMgr.SetContext(ctx)
	if err := a.notifier.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "notify: failed to start hook listener: %v\n", err)
	}
	// Install the hook script up front so it's ready the moment a Claude agent
	// launches. Per-workspace settings.local.json is written lazily by
	// LaunchAgent.
	go notify.InstallHookScript()

	// Clear macOS saved application state to prevent stale WKWebView restoration
	home, _ := os.UserHomeDir()
	os.RemoveAll(filepath.Join(home, "Library", "Saved Application State", "com.wails.Orion.savedState"))

	// Start mobile companion web server
	a.webSrv = web.NewServer(a, a.termMgr, a.codexMgr, a.claudeMgr)
	go a.webSrv.Start(mobileServerPort())

	wailsRuntime.EventsOn(ctx, "terminal:input", func(optionalData ...interface{}) {
		if len(optionalData) < 2 {
			return
		}
		id, ok1 := optionalData[0].(string)
		data, ok2 := optionalData[1].(string)
		if ok1 && ok2 {
			a.termMgr.Write(id, data)
		}
	})

	wailsRuntime.EventsOn(ctx, "terminal:resize", func(optionalData ...interface{}) {
		if len(optionalData) < 3 {
			return
		}
		id, ok1 := optionalData[0].(string)
		cols, ok2 := optionalData[1].(float64)
		rows, ok3 := optionalData[2].(float64)
		if ok1 && ok2 && ok3 {
			a.termMgr.Resize(id, int(cols), int(rows))
		}
	})
}

// domReady fires when the frontend DOM is ready.
func (a *App) domReady(ctx context.Context) {
	wailsRuntime.EventsEmit(ctx, "app:ready")
}

func (a *App) shutdown(ctx context.Context) {
	// Stop mobile companion web server
	if a.webSrv != nil {
		a.webSrv.Stop()
	}
	if a.notifier != nil {
		a.notifier.Stop()
	}
	// Detach from PTYs but keep tmux sessions alive for recovery on next launch
	a.termMgr.DetachAll()
	if a.codexMgr != nil {
		for _, session := range a.codexMgr.List(nil) {
			_ = a.codexMgr.Stop(session.ID)
		}
	}
	if a.claudeMgr != nil {
		a.claudeMgr.DetachAll()
	}
	a.watcherMgr.Stop()
}

// --- Terminal methods ---

func (a *App) CreateTerminal(id string) error {
	return a.termMgr.Create(id)
}

func (a *App) CreateTerminalInDir(id string, dir string) error {
	return a.termMgr.CreateInDir(id, dir)
}

func (a *App) CreateAttachedTerminal(id string, tmuxSession string) error {
	return a.termMgr.CreateAttached(id, tmuxSession)
}

func (a *App) CloseTerminal(id string) error {
	return a.termMgr.Close(id)
}

func (a *App) DetachTerminal(id string) error {
	return a.termMgr.Detach(id)
}

func (a *App) GetTmuxSession(terminalId string) string {
	return a.termMgr.GetTmuxSession(terminalId)
}

func (a *App) ListTerminals() []string {
	return a.termMgr.List()
}

// --- Workspace methods ---

func (a *App) GetProjectInfo(path string) (*workspace.ProjectInfo, error) {
	return a.wsMgr.GetProjectInfo(path)
}

func (a *App) GetProjectInfoFromCwd() (*workspace.ProjectInfo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return a.wsMgr.GetProjectInfo(cwd)
}

func (a *App) OpenProjectDialog() (*workspace.ProjectInfo, error) {
	dir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Open Project",
	})
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, fmt.Errorf("no directory selected")
	}
	info, err := a.wsMgr.GetProjectInfo(dir)
	if err != nil {
		return nil, err
	}
	a.appState.SetProject(info.Root)
	return info, nil
}

func (a *App) OpenChatAttachmentDialog() ([]chatattachments.Attachment, error) {
	paths, err := wailsRuntime.OpenMultipleFilesDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Attach Images",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Images (*.png, *.jpg, *.jpeg, *.gif, *.webp, *.heic)", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.heic;*.heif"},
		},
	})
	if err != nil {
		return nil, err
	}
	attachments := make([]chatattachments.Attachment, 0, len(paths))
	for _, path := range paths {
		attachment, err := chatattachments.FromPath(path)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

// SetActiveProject sets the project and loads its per-project state.
func (a *App) SetActiveProject(root string) {
	a.appState.SetProject(root)
}

// NewWindow launches a new Orion instance.
// Passes --new flag so the new instance opens to project picker instead of
// auto-loading the last project (which would steal tmux sessions from this window).
func (a *App) NewWindow() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	appPath := execPath
	if idx := strings.Index(execPath, ".app/"); idx > 0 {
		appPath = execPath[:idx+4]
	}
	cmd := exec.Command("open", "-n", appPath, "--args", "--new")
	return cmd.Start()
}

// NewWindowWithProject launches a new Orion instance for a specific project.
func (a *App) NewWindowWithProject(projectRoot string) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	appPath := execPath
	if idx := strings.Index(execPath, ".app/"); idx > 0 {
		appPath = execPath[:idx+4]
	}
	cmd := exec.Command("open", "-n", appPath, "--args", "--project", projectRoot)
	return cmd.Start()
}

func (a *App) ListWorkspaces(repoRoot string) ([]workspace.Workspace, error) {
	return a.wsMgr.ListWorkspaces(repoRoot)
}

func (a *App) CreateWorkspace(repoRoot string, name string) (*workspace.Workspace, error) {
	return a.wsMgr.CreateWorkspace(repoRoot, name)
}

func (a *App) CreateWorkspaceFrom(repoRoot string, name string, baseRef string) (*workspace.Workspace, error) {
	return a.wsMgr.CreateWorkspaceFrom(repoRoot, name, baseRef)
}

func (a *App) DeleteWorkspace(repoRoot string, path string) error {
	wsID := filepath.Base(path)
	a.portReg.ReleaseWorkspace(wsID)
	a.portReg.ReleaseRedisDB(wsID)
	return a.wsMgr.DeleteWorkspace(repoRoot, path)
}

func (a *App) LaunchAgent(repoRoot string, workspacePath string, agentType string) (string, error) {
	return a.wsMgr.LaunchAgent(repoRoot, workspacePath, agentType)
}

func (a *App) LaunchShell(repoRoot string, workspacePath string) (string, error) {
	return a.wsMgr.LaunchShell(repoRoot, workspacePath)
}

func (a *App) ConvertChatToTerminal(repoRoot string, workspacePath string, sessionID string, chatKind string) (string, error) {
	switch chatKind {
	case "claude":
		var threadID string
		if session, ok := a.claudeMgr.Get(sessionID); ok {
			threadID = strings.TrimSpace(session.Info().ThreadID)
			_ = session.Stop()
		}
		if threadID == "" && !strings.HasPrefix(strings.TrimSpace(sessionID), claudechat.SessionType+"-") {
			threadID = strings.TrimSpace(sessionID)
		}
		if threadID != "" {
			return a.wsMgr.LaunchCommand(repoRoot, workspacePath, "claude --dangerously-skip-permissions --resume "+shellQuote(threadID))
		}
		return a.wsMgr.LaunchAgent(repoRoot, workspacePath, "claude")
	case "codex":
		var threadID string
		if session, ok := a.codexMgr.Get(sessionID); ok {
			threadID = strings.TrimSpace(session.Info().ThreadID)
			_ = session.Stop()
		}
		if threadID != "" {
			return a.wsMgr.LaunchCommand(repoRoot, workspacePath, "codex resume --dangerously-bypass-approvals-and-sandbox --no-alt-screen "+shellQuote(threadID))
		}
		return a.wsMgr.LaunchAgent(repoRoot, workspacePath, "codex")
	default:
		return "", fmt.Errorf("unsupported chat kind: %s", chatKind)
	}
}

// --- Claude chat methods ---

func (a *App) LaunchClaudeChat(repoRoot string, workspacePath string) (*claudechat.SessionInfo, error) {
	return a.claudeMgr.StartWithOptions(claudechat.StartOptions{
		WorkspacePath:    workspacePath,
		Label:            "Claude Chat",
		Model:            "claude-opus-4-7",
		ReasoningEffort:  "xhigh",
		ApprovalPolicy:   "never",
		SandboxMode:      "danger-full-access",
		PermissionMode:   "plan",
		ClaudeExecutable: "claude",
	})
}

func (a *App) ResumeClaudeChat(repoRoot string, workspacePath string, threadID string) (*claudechat.SessionInfo, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("threadId required")
	}
	return a.claudeMgr.StartWithOptions(claudechat.StartOptions{
		WorkspacePath:    workspacePath,
		Label:            "Claude Chat",
		ThreadID:         threadID,
		Model:            "claude-opus-4-7",
		ReasoningEffort:  "xhigh",
		ApprovalPolicy:   "never",
		SandboxMode:      "danger-full-access",
		PermissionMode:   "plan",
		ClaudeExecutable: "claude",
	})
}

func (a *App) AttachClaudeChat(tmuxSession string, workspacePath string) (*claudechat.SessionInfo, error) {
	return a.claudeMgr.Attach(tmuxSession, workspacePath, "Claude")
}

func (a *App) ConvertTerminalToClaudeChat(repoRoot string, workspacePath string, tmuxSession string) (*claudechat.SessionInfo, error) {
	tmuxSession = strings.TrimSpace(tmuxSession)
	if tmuxSession == "" {
		return nil, fmt.Errorf("tmuxSession required")
	}
	threadID := claudechat.ThreadIDForTmux(tmuxSession, workspacePath)
	if threadID == "" {
		return nil, fmt.Errorf("could not identify Claude session for tmux session %s", tmuxSession)
	}
	return a.ResumeClaudeChat(repoRoot, workspacePath, threadID)
}

func (a *App) ListClaudeChatSessions(workspacePaths []string) []state.SessionInfo {
	infos := a.claudeMgr.List(workspacePaths)
	sessions := make([]state.SessionInfo, 0, len(infos))
	for _, info := range infos {
		sessions = append(sessions, state.SessionInfo{
			TmuxName:         info.ID,
			Type:             info.Type,
			Label:            info.Label,
			WorkspacePath:    info.WorkspacePath,
			Provider:         "claude",
			ViewMode:         "chat",
			RuntimeSessionID: info.ID,
			ThreadID:         info.ThreadID,
			Model:            info.Model,
			ReasoningEffort:  info.ReasoningEffort,
			ApprovalPolicy:   info.ApprovalPolicy,
			SandboxMode:      info.SandboxMode,
			PermissionMode:   info.PermissionMode,
		})
	}
	return sessions
}

func (a *App) GetClaudeChatMessages(sessionID string) []claudechat.Message {
	session, ok := a.claudeMgr.Get(sessionID)
	if !ok {
		return nil
	}
	return session.Messages()
}

func (a *App) SendClaudeChatMessage(sessionID string, text string, attachments []chatattachments.Attachment) error {
	session, ok := a.claudeMgr.Get(sessionID)
	if !ok {
		return fmt.Errorf("claude chat session not found: %s", sessionID)
	}
	return session.Send(text, attachments)
}

func (a *App) AnswerClaudeChatRequest(sessionID string, toolUseID string, result string) error {
	session, ok := a.claudeMgr.Get(sessionID)
	if !ok {
		return fmt.Errorf("claude chat session not found: %s", sessionID)
	}
	return session.Answer(toolUseID, result)
}

func (a *App) ApproveClaudePlan(sessionID string) error {
	session, ok := a.claudeMgr.Get(sessionID)
	if !ok {
		return fmt.Errorf("claude chat session not found: %s", sessionID)
	}
	return session.ApprovePlan()
}

func (a *App) StopClaudeChat(sessionID string) error {
	return a.claudeMgr.Stop(sessionID)
}

// --- Codex chat methods ---

func (a *App) LaunchCodexChat(repoRoot string, workspacePath string) (*codexchat.SessionInfo, error) {
	return a.codexMgr.Start(workspacePath, "Codex Chat")
}

func (a *App) LaunchCodexChatWithOptions(repoRoot string, workspacePath string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, collaborationMode string) (*codexchat.SessionInfo, error) {
	return a.codexMgr.StartWithOptions(codexchat.StartOptions{
		WorkspacePath:     workspacePath,
		Label:             "Codex Chat",
		Model:             model,
		ReasoningEffort:   reasoningEffort,
		ApprovalPolicy:    approvalPolicy,
		SandboxMode:       sandboxMode,
		CollaborationMode: collaborationMode,
	})
}

func (a *App) ResumeCodexChat(repoRoot string, workspacePath string, threadID string) (*codexchat.SessionInfo, error) {
	return a.ResumeCodexChatWithOptions(repoRoot, workspacePath, threadID, "", "", "", "", "")
}

func (a *App) ResumeCodexChatWithOptions(repoRoot string, workspacePath string, threadID string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, collaborationMode string) (*codexchat.SessionInfo, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("threadId required")
	}
	return a.codexMgr.StartWithOptions(codexchat.StartOptions{
		WorkspacePath:     workspacePath,
		Label:             "Codex Chat",
		ThreadID:          threadID,
		Model:             model,
		ReasoningEffort:   reasoningEffort,
		ApprovalPolicy:    approvalPolicy,
		SandboxMode:       sandboxMode,
		CollaborationMode: collaborationMode,
	})
}

func (a *App) ConvertTerminalToCodexChat(repoRoot string, workspacePath string, tmuxSession string) (*codexchat.SessionInfo, error) {
	tmuxSession = strings.TrimSpace(tmuxSession)
	if tmuxSession == "" {
		return nil, fmt.Errorf("tmuxSession required")
	}
	threadID := codexchat.ThreadIDForTmux(tmuxSession, workspacePath)
	if threadID == "" {
		return nil, fmt.Errorf("could not identify Codex thread for tmux session %s", tmuxSession)
	}
	return a.ResumeCodexChat(repoRoot, workspacePath, threadID)
}

func (a *App) ListCodexChatSessions(workspacePaths []string) []state.SessionInfo {
	infos := a.codexMgr.List(workspacePaths)
	sessions := make([]state.SessionInfo, 0, len(infos))
	for _, info := range infos {
		sessions = append(sessions, state.SessionInfo{
			TmuxName:          info.ID,
			Type:              codexchat.SessionType,
			Label:             info.Label,
			WorkspacePath:     info.WorkspacePath,
			Provider:          codexchat.Provider,
			ViewMode:          codexchat.ViewModeChat,
			RuntimeSessionID:  info.ID,
			ThreadID:          info.ThreadID,
			Model:             info.Model,
			ReasoningEffort:   info.ReasoningEffort,
			ApprovalPolicy:    info.ApprovalPolicy,
			SandboxMode:       info.SandboxMode,
			CollaborationMode: info.CollaborationMode,
		})
	}
	return sessions
}

func (a *App) GetCodexChatMessages(sessionID string) []codexchat.Message {
	session, ok := a.codexMgr.Get(sessionID)
	if !ok {
		return nil
	}
	return session.Messages()
}

func (a *App) SendCodexChatMessage(sessionID string, text string, attachments []chatattachments.Attachment) error {
	session, ok := a.codexMgr.Get(sessionID)
	if !ok {
		return fmt.Errorf("codex chat session not found: %s", sessionID)
	}
	return session.Send(text, attachments)
}

func (a *App) AnswerCodexChatRequest(sessionID string, toolUseID string, result string) error {
	session, ok := a.codexMgr.Get(sessionID)
	if !ok {
		return fmt.Errorf("codex chat session not found: %s", sessionID)
	}
	return session.Answer(toolUseID, result)
}

func (a *App) StopCodexChat(sessionID string) error {
	return a.codexMgr.Stop(sessionID)
}

func (a *App) GetConfig(repoRoot string) *config.OrionConfig {
	return a.wsMgr.GetConfig(repoRoot)
}

// --- Server methods ---

func (a *App) AllocatePorts(repoRoot string, workspacePath string, isMain bool) error {
	return a.srvMgr.AllocatePorts(repoRoot, workspacePath, isMain)
}

func (a *App) StartServers(repoRoot string, workspacePath string, isMain bool) ([]server.ServerStatus, error) {
	return a.srvMgr.StartServers(repoRoot, workspacePath, isMain)
}

func (a *App) StopServers(workspacePath string) error {
	return a.srvMgr.StopServers(workspacePath)
}

func (a *App) GetServerStatuses(repoRoot string, workspacePath string) []server.ServerStatus {
	return a.srvMgr.GetServerStatuses(repoRoot, workspacePath)
}

// GetWorkspaceEnv returns the env vars from .orion/env.sh for display.
func (a *App) GetWorkspaceEnv(workspacePath string) map[string]string {
	envFile := filepath.Join(workspacePath, ".orion", "env.sh")
	data, err := os.ReadFile(envFile)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			kv := strings.TrimPrefix(line, "export ")
			if idx := strings.Index(kv, "="); idx > 0 {
				result[kv[:idx]] = kv[idx+1:]
			}
		}
	}
	return result
}

func (a *App) OpenBrowser(repoRoot string, workspacePath string) error {
	cfg := config.Load(repoRoot)
	wsID := filepath.Base(workspacePath)
	alloc := a.portReg.GetAllocation(wsID)
	if alloc == nil && filepath.Clean(repoRoot) == filepath.Clean(workspacePath) {
		alloc = make(port.Allocation)
		for name, srv := range cfg.Servers {
			if srv.DefaultPort > 0 {
				alloc[name] = srv.DefaultPort
			}
		}
	}

	if alloc == nil {
		return fmt.Errorf("no servers running for this workspace")
	}

	frontendPort := 0
	if p, ok := alloc["frontend"]; ok {
		frontendPort = p
	} else {
		for name := range cfg.Servers {
			if p, ok := alloc[name]; ok {
				frontendPort = p
				break
			}
		}
	}

	if frontendPort == 0 {
		return fmt.Errorf("no frontend port found")
	}

	url := fmt.Sprintf("http://localhost:%d", frontendPort)
	return exec.Command("open", url).Run()
}

// --- State methods ---

func (a *App) GetLastProject() string {
	// New window flag — open to project picker, don't auto-load
	if os.Getenv("ORION_NEW_WINDOW") == "1" {
		return ""
	}
	// Check CLI flag first (for multi-instance launches)
	if envProject := os.Getenv("ORION_PROJECT"); envProject != "" {
		return envProject
	}
	return a.appState.GetLastProject()
}

func (a *App) GetRecentProjects() []string {
	return a.appState.GetRecentProjects()
}

func (a *App) RecoverSessions(repoName string, workspacePaths []string) []state.SessionInfo {
	return state.RecoverSessions(repoName, workspacePaths)
}

func (a *App) SaveTabs(tabs []state.SavedTab) {
	a.appState.SaveTabs(tabs)
}

func (a *App) GetSavedTabs() []state.SavedTab {
	saved := a.appState.GetSavedTabs()
	if len(saved) == 0 {
		return nil
	}
	var alive []state.SavedTab
	for _, tab := range saved {
		if tab.TabType == codexchat.SessionType && strings.TrimSpace(tab.ThreadID) != "" {
			alive = append(alive, tab)
			continue
		}
		if tab.TmuxSession != "" {
			cmd := exec.Command("tmux", "has-session", "-t", tab.TmuxSession)
			if cmd.Run() == nil {
				alive = append(alive, tab)
			}
		}
	}
	return alive
}

// --- Mobile companion methods ---

func (a *App) GetMobileURL() string {
	if a.webSrv != nil {
		return a.webSrv.GetConnectionURL()
	}
	return ""
}

func (a *App) GetMobileToken() string {
	if a.webSrv != nil {
		return a.webSrv.GetToken()
	}
	return ""
}

func (a *App) EmitSessionCreated(tmuxSession string, sessionType string, label string, workspacePath string) {
	wailsRuntime.EventsEmit(a.ctx, "mobile:session-created", map[string]string{
		"tmuxSession":   tmuxSession,
		"type":          sessionType,
		"label":         label,
		"workspacePath": workspacePath,
	})
}

func (a *App) EmitSessionCreatedInfo(session state.SessionInfo) {
	wailsRuntime.EventsEmit(a.ctx, "mobile:session-created", map[string]string{
		"tmuxSession":       session.TmuxName,
		"type":              session.Type,
		"label":             session.Label,
		"workspacePath":     session.WorkspacePath,
		"provider":          session.Provider,
		"viewMode":          session.ViewMode,
		"runtimeSessionId":  session.RuntimeSessionID,
		"threadId":          session.ThreadID,
		"model":             session.Model,
		"reasoningEffort":   session.ReasoningEffort,
		"approvalPolicy":    session.ApprovalPolicy,
		"sandboxMode":       session.SandboxMode,
		"permissionMode":    session.PermissionMode,
		"collaborationMode": session.CollaborationMode,
	})
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (a *App) GetAgentNames(repoRoot string) []web.AgentType {
	cfg := config.Load(repoRoot)
	var agents []web.AgentType
	for name := range cfg.Agents {
		agents = append(agents, web.AgentType{Name: name, Label: capitalize(name)})
	}
	return agents
}

// --- Agent methods ---

func (a *App) GetAgentTypes(repoRoot string) []AgentTypeInfo {
	cfg := config.Load(repoRoot)
	var agents []AgentTypeInfo
	for name, agent := range cfg.Agents {
		agents = append(agents, AgentTypeInfo{
			Name:    name,
			Command: agent.Command,
			Label:   capitalize(name),
		})
	}
	sortAgents(agents)
	return agents
}

type AgentTypeInfo struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Label   string `json:"label"`
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// --- File & Git methods ---

func (a *App) ListDirectory(dir string, depth int) ([]files.FileEntry, error) {
	return a.filesMgr.ListDirectory(dir, depth)
}

func (a *App) ReadFileContents(path string) (string, error) {
	return a.filesMgr.ReadFileContents(path)
}

// RevealInFinder opens Finder with the file selected.
func (a *App) RevealInFinder(path string) error {
	return exec.Command("open", "-R", path).Run()
}

// WatchWorkspace starts watching a workspace directory for file changes.
// Emits "git:files-changed" events to the frontend when changes are detected.
func (a *App) WatchWorkspace(workspacePath string) error {
	return a.watcherMgr.Watch(workspacePath)
}

func (a *App) GetChangedFiles(workspacePath string) ([]git.ChangedFile, error) {
	return a.gitMgr.GetChangedFiles(workspacePath)
}

func (a *App) GetFileDiff(workspacePath string, filePath string) (*git.FileDiff, error) {
	return a.gitMgr.GetFileDiff(workspacePath, filePath)
}

func (a *App) GetChangedFilesAgainst(workspacePath string, base string) ([]git.ChangedFile, error) {
	return a.gitMgr.GetChangedFilesAgainst(workspacePath, base)
}

func (a *App) GetUnifiedDiff(workspacePath string, base string, filePath string) (string, error) {
	return a.gitMgr.GetUnifiedDiff(workspacePath, base, filePath)
}

func (a *App) DiscardFileChanges(workspacePath string, filePath string) error {
	return a.gitMgr.DiscardFileChanges(workspacePath, filePath)
}

func (a *App) DiscardAllChanges(workspacePath string) error {
	return a.gitMgr.DiscardAllChanges(workspacePath)
}

func (a *App) SearchFiles(root string, query string) ([]files.SearchResult, error) {
	return a.filesMgr.SearchFiles(root, query, 50)
}

func (a *App) SearchContents(root string, query string) ([]files.GrepResult, error) {
	return a.filesMgr.SearchContents(root, query, 100)
}

func (a *App) GetClipboard() string {
	text, _ := wailsRuntime.ClipboardGetText(a.ctx)
	return text
}

func (a *App) SetClipboard(text string) {
	wailsRuntime.ClipboardSetText(a.ctx, text)
}

// --- Diagnostics ---

func (a *App) GetMemorySnapshot() (*diag.MemorySnapshot, error) {
	return a.diagMgr.Snapshot()
}

// KillSession terminates a tmux session by name. Closes any attached Orion
// terminal first so the PTY and tab state are cleaned up, then issues
// tmux kill-session for anything not attached. Refuses non-orion- sessions
// so a buggy UI can't wipe a user's unrelated tmux work.
func (a *App) KillSession(name string) error {
	if !strings.HasPrefix(name, "orion-") {
		return fmt.Errorf("refusing to kill non-orion session: %s", name)
	}
	for _, id := range a.termMgr.List() {
		if a.termMgr.GetTmuxSession(id) == name {
			return a.termMgr.Close(id)
		}
	}
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

func sortAgents(agents []AgentTypeInfo) {
	priority := map[string]int{"claude": 0, "codex": 1}
	for i := 0; i < len(agents); i++ {
		for j := i + 1; j < len(agents); j++ {
			pi, oki := priority[agents[i].Name]
			pj, okj := priority[agents[j].Name]
			if !oki {
				pi = 99
			}
			if !okj {
				pj = 99
			}
			if pj < pi {
				agents[i], agents[j] = agents[j], agents[i]
			}
		}
	}
}

func mobileServerPort() int {
	if raw := strings.TrimSpace(os.Getenv("ORION_MOBILE_PORT")); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port < 65536 {
			return port
		}
	}
	return 9867
}
