package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SessionInfo represents a recovered tmux session matched to a workspace.
type SessionInfo struct {
	TmuxName      string `json:"tmuxName"`
	Type          string `json:"type"`
	Label         string `json:"label"`
	WorkspacePath string `json:"workspacePath"`
}

// SavedTab represents a tab that can be restored on next launch.
type SavedTab struct {
	Label         string `json:"label"`
	TabType       string `json:"tabType"`
	TmuxSession   string `json:"tmuxSession"`
	WorkspacePath string `json:"workspacePath"`
}

// --- Global State (shared across instances) ---

// GlobalState tracks recent projects and the last opened project.
type GlobalState struct {
	LastProject    string   `json:"lastProject"`
	RecentProjects []string `json:"recentProjects"`
	filePath       string
}

func NewGlobalState() *GlobalState {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".orion")
	os.MkdirAll(dir, 0755)

	g := &GlobalState{filePath: filepath.Join(dir, "global.json")}
	g.load()

	// Migrate from old state.json if global.json doesn't exist yet
	oldPath := filepath.Join(dir, "state.json")
	if g.LastProject == "" {
		var old struct {
			LastProject string `json:"lastProject"`
		}
		if data, err := os.ReadFile(oldPath); err == nil {
			json.Unmarshal(data, &old)
			if old.LastProject != "" {
				g.LastProject = old.LastProject
				g.AddRecentProject(old.LastProject)
				g.save()
			}
		}
	}

	return g
}

func (g *GlobalState) GetLastProject() string {
	return g.LastProject
}

func (g *GlobalState) SetLastProject(root string) {
	g.LastProject = root
	g.AddRecentProject(root)
	g.save()
}

func (g *GlobalState) GetRecentProjects() []string {
	return g.RecentProjects
}

func (g *GlobalState) AddRecentProject(root string) {
	// Remove if already exists, add to front
	var filtered []string
	for _, p := range g.RecentProjects {
		if p != root {
			filtered = append(filtered, p)
		}
	}
	g.RecentProjects = append([]string{root}, filtered...)
	// Keep max 20
	if len(g.RecentProjects) > 20 {
		g.RecentProjects = g.RecentProjects[:20]
	}
	g.save()
}

func (g *GlobalState) load() {
	data, err := os.ReadFile(g.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, g)
}

func (g *GlobalState) save() {
	data, _ := json.MarshalIndent(g, "", "  ")
	os.WriteFile(g.filePath, data, 0644)
}

// --- Per-Project State ---

// ProjectState stores state for a single project (saved tabs, etc.)
type ProjectState struct {
	SavedTabs []SavedTab `json:"savedTabs,omitempty"`
	filePath  string
}

func projectHash(root string) string {
	h := sha256.Sum256([]byte(root))
	return fmt.Sprintf("%x", h[:8])
}

func NewProjectState(projectRoot string) *ProjectState {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".orion", "projects", projectHash(projectRoot))
	os.MkdirAll(dir, 0755)

	ps := &ProjectState{filePath: filepath.Join(dir, "state.json")}
	ps.load()

	// Migrate from old global state.json if this project has no saved tabs
	if len(ps.SavedTabs) == 0 {
		migrateOldTabs(projectRoot, ps)
	}

	return ps
}

func (ps *ProjectState) SaveTabs(tabs []SavedTab) {
	ps.SavedTabs = tabs
	ps.save()
}

func (ps *ProjectState) GetSavedTabs() []SavedTab {
	return ps.SavedTabs
}

func (ps *ProjectState) load() {
	data, err := os.ReadFile(ps.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, ps)
}

func (ps *ProjectState) save() {
	data, _ := json.MarshalIndent(ps, "", "  ")
	os.WriteFile(ps.filePath, data, 0644)
}

// migrateOldTabs reads the old global state.json and migrates tabs for this project.
func migrateOldTabs(projectRoot string, ps *ProjectState) {
	home, _ := os.UserHomeDir()
	oldPath := filepath.Join(home, ".orion", "state.json")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return
	}

	var old struct {
		SavedTabs []SavedTab `json:"savedTabs"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return
	}

	// Filter tabs belonging to this project (workspace paths under the project root)
	seen := make(map[string]bool) // dedup by tmux session
	var migrated []SavedTab
	for _, tab := range old.SavedTabs {
		if strings.HasPrefix(tab.WorkspacePath, filepath.Dir(projectRoot)) && !seen[tab.TmuxSession] {
			seen[tab.TmuxSession] = true
			migrated = append(migrated, tab)
		}
	}

	if len(migrated) > 0 {
		ps.SavedTabs = migrated
		ps.save()
	}
}

// --- AppState wraps both global and project state ---

type AppState struct {
	global  *GlobalState
	project *ProjectState
}

func NewAppState() *AppState {
	return &AppState{
		global: NewGlobalState(),
	}
}

func (a *AppState) SetProject(root string) {
	a.global.SetLastProject(root)
	a.project = NewProjectState(root)
}

func (a *AppState) GetLastProject() string {
	return a.global.GetLastProject()
}

func (a *AppState) GetRecentProjects() []string {
	return a.global.GetRecentProjects()
}

func (a *AppState) SaveTabs(tabs []SavedTab) {
	if a.project != nil {
		a.project.SaveTabs(tabs)
	}
}

func (a *AppState) GetSavedTabs() []SavedTab {
	if a.project != nil {
		return a.project.GetSavedTabs()
	}
	return nil
}

// --- Session Recovery (unchanged) ---

func RecoverSessions(repoName string, workspacePaths []string) []SessionInfo {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	type wsEntry struct {
		basename string
		path     string
	}
	var wsEntries []wsEntry
	for _, p := range workspacePaths {
		wsEntries = append(wsEntries, wsEntry{basename: filepath.Base(p), path: p})
	}
	for i := 0; i < len(wsEntries); i++ {
		for j := i + 1; j < len(wsEntries); j++ {
			if len(wsEntries[j].basename) > len(wsEntries[i].basename) {
				wsEntries[i], wsEntries[j] = wsEntries[j], wsEntries[i]
			}
		}
	}

	prefix := "orion-"
	srvPrefix := "orion-srv-"
	shellPrefix := "orion-shell-"

	var sessions []SessionInfo
	shellCount := make(map[string]int)

	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || !strings.HasPrefix(name, prefix) {
			continue
		}

		if strings.HasPrefix(name, shellPrefix) {
			dirCmd := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_current_path}")
			dirOut, err := dirCmd.Output()
			if err != nil {
				continue
			}
			sessionDir := strings.TrimSpace(string(dirOut))
			for _, ws := range wsEntries {
				if strings.HasPrefix(sessionDir, ws.path) {
					shellCount[ws.path]++
					label := "Shell"
					if shellCount[ws.path] > 1 {
						label = fmt.Sprintf("Shell %d", shellCount[ws.path])
					}
					sessions = append(sessions, SessionInfo{
						TmuxName:      name,
						Type:          "shell",
						Label:         label,
						WorkspacePath: ws.path,
					})
					break
				}
			}
			continue
		}

		if strings.HasPrefix(name, srvPrefix) {
			rest := strings.TrimPrefix(name, srvPrefix)
			for _, ws := range wsEntries {
				if strings.HasPrefix(rest, ws.basename+"-") {
					serverName := strings.TrimPrefix(rest, ws.basename+"-")
					sessions = append(sessions, SessionInfo{
						TmuxName:      name,
						Type:          "server",
						Label:         capitalize(serverName),
						WorkspacePath: ws.path,
					})
					break
				}
			}
			continue
		}

		rest := strings.TrimPrefix(name, prefix)
		repoPrefix := repoName + "-"
		if !strings.HasPrefix(rest, repoPrefix) {
			continue
		}
		afterRepo := strings.TrimPrefix(rest, repoPrefix)

		matched := false
		for _, ws := range wsEntries {
			if afterRepo == ws.basename {
				sessions = append(sessions, SessionInfo{
					TmuxName:      name,
					Type:          "shell",
					Label:         "Shell",
					WorkspacePath: ws.path,
				})
				matched = true
				break
			}
			if strings.HasPrefix(afterRepo, ws.basename+"-") {
				suffix := strings.TrimPrefix(afterRepo, ws.basename+"-")
				label := "Shell " + suffix
				sessions = append(sessions, SessionInfo{
					TmuxName:      name,
					Type:          "shell",
					Label:         label,
					WorkspacePath: ws.path,
				})
				matched = true
				break
			}
		}
		_ = matched
	}

	return sessions
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
