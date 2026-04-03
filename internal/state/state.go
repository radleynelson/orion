package state

import (
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
	Type          string `json:"type"`          // "server", "agent", "shell"
	Label         string `json:"label"`         // Display name for the tab
	WorkspacePath string `json:"workspacePath"` // Full path to matched workspace
}

// SavedTab represents a tab that can be restored on next launch.
type SavedTab struct {
	Label         string `json:"label"`
	TabType       string `json:"tabType"`
	TmuxSession   string `json:"tmuxSession"`
	WorkspacePath string `json:"workspacePath"`
}

// AppState persists across Orion restarts.
type AppState struct {
	LastProject string     `json:"lastProject"`
	SavedTabs   []SavedTab `json:"savedTabs,omitempty"`
	filePath    string
}

func NewAppState() *AppState {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".orion")
	os.MkdirAll(dir, 0755)
	fp := filepath.Join(dir, "state.json")

	s := &AppState{filePath: fp}
	s.load()
	return s
}

func (s *AppState) SetLastProject(root string) {
	s.LastProject = root
	s.save()
}

func (s *AppState) GetLastProject() string {
	return s.LastProject
}

func (s *AppState) SaveTabs(tabs []SavedTab) {
	s.SavedTabs = tabs
	s.save()
}

func (s *AppState) GetSavedTabs() []SavedTab {
	return s.SavedTabs
}

func (s *AppState) ClearSavedTabs() {
	s.SavedTabs = nil
	s.save()
}

// RecoverSessions finds orion-* tmux sessions and matches them to workspaces.
// workspacePaths is a list of actual workspace directory paths (e.g., /Users/.../slant, /Users/.../slant-fix-auth).
// repoName is the base repo name (e.g., "slant").
func RecoverSessions(repoName string, workspacePaths []string) []SessionInfo {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Build a lookup: workspace basename -> full path
	// Sort by length descending so longer (more specific) names match first
	type wsEntry struct {
		basename string
		path     string
	}
	var wsEntries []wsEntry
	for _, p := range workspacePaths {
		wsEntries = append(wsEntries, wsEntry{basename: filepath.Base(p), path: p})
	}
	// Sort longest basename first for greedy matching
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
	shellCount := make(map[string]int) // workspace path -> shell count for labeling

	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || !strings.HasPrefix(name, prefix) {
			continue
		}

		// Shell sessions created by CreateInDir: orion-shell-<id>
		// These need to be matched by checking tmux's working directory
		if strings.HasPrefix(name, shellPrefix) {
			// Get the tmux session's current directory
			dirCmd := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_current_path}")
			dirOut, err := dirCmd.Output()
			if err != nil {
				continue
			}
			sessionDir := strings.TrimSpace(string(dirOut))

			// Match to a workspace
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

		// Server sessions: orion-srv-<workspace-basename>-<servername>
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

		// Agent/shell sessions: orion-<repo>-<workspace-basename>[-<N>]
		rest := strings.TrimPrefix(name, prefix)
		repoPrefix := repoName + "-"
		if !strings.HasPrefix(rest, repoPrefix) {
			continue
		}
		afterRepo := strings.TrimPrefix(rest, repoPrefix)

		// Try matching against workspace basenames (longest first)
		matched := false
		for _, ws := range wsEntries {
			if afterRepo == ws.basename {
				// Exact match: orion-slant-slant (main workspace)
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
				// Numbered session: orion-slant-slant-1
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

		if !matched {
			// Couldn't match to a workspace — skip it
			continue
		}
	}

	return sessions
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (s *AppState) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, s)
}

func (s *AppState) save() {
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(s.filePath, data, 0644)
}
