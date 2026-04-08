package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orion/internal/config"
	"orion/internal/files"
	"orion/internal/git"
	"orion/internal/port"
	"orion/internal/server"
	"orion/internal/state"
	"orion/internal/terminal"
	"orion/internal/workspace"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct bound to Wails.
type App struct {
	ctx      context.Context
	termMgr  *terminal.Manager
	wsMgr    *workspace.Manager
	srvMgr   *server.Manager
	portReg  *port.Registry
	appState *state.AppState
	filesMgr *files.Manager
	gitMgr   *git.Manager
}

// NewApp creates a new App instance.
func NewApp() *App {
	portReg := port.NewRegistry()
	return &App{
		termMgr:  terminal.NewManager(),
		wsMgr:    workspace.NewManager(),
		srvMgr:   server.NewManager(portReg),
		portReg:  portReg,
		appState: state.NewAppState(),
		filesMgr: files.NewManager(),
		gitMgr:   git.NewManager(),
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
	a.wsMgr.SetContext(ctx)
	a.srvMgr.SetContext(ctx)
	a.filesMgr.SetContext(ctx)
	a.gitMgr.SetContext(ctx)

	// Clear macOS saved application state to prevent stale WKWebView restoration
	home, _ := os.UserHomeDir()
	os.RemoveAll(filepath.Join(home, "Library", "Saved Application State", "com.wails.Orion.savedState"))

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
	// Detach from PTYs but keep tmux sessions alive for recovery on next launch
	a.termMgr.DetachAll()
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
		if tab.TmuxSession != "" {
			cmd := exec.Command("tmux", "has-session", "-t", tab.TmuxSession)
			if cmd.Run() == nil {
				alive = append(alive, tab)
			}
		}
	}
	return alive
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

func (a *App) GetChangedFiles(workspacePath string) ([]git.ChangedFile, error) {
	return a.gitMgr.GetChangedFiles(workspacePath)
}

func (a *App) GetFileDiff(workspacePath string, filePath string) (*git.FileDiff, error) {
	return a.gitMgr.GetFileDiff(workspacePath, filePath)
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
