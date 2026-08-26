package workspace

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func applyCodexWorkspaceTitles(workspaces []Workspace) {
	paths := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if !workspace.IsMain {
			paths = append(paths, workspace.Path)
		}
	}
	if len(paths) == 0 {
		return
	}

	titles := codexWorkspaceTitles(paths)
	for i := range workspaces {
		if title := strings.TrimSpace(titles[workspaces[i].Path]); title != "" {
			workspaces[i].Name = title
		}
	}
}

func codexWorkspaceTitles(workspacePaths []string) map[string]string {
	titles := make(map[string]string)
	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		return titles
	}
	stateDatabase := latestCodexStateDatabase(codexHomeDir())
	if stateDatabase == "" {
		return titles
	}

	quotedPaths := make([]string, 0, len(workspacePaths))
	for _, workspacePath := range workspacePaths {
		quotedPaths = append(quotedPaths, "'"+strings.ReplaceAll(workspacePath, "'", "''")+"'")
	}
	query := `
		SELECT hex(cwd) || '|' || hex(COALESCE(NULLIF(TRIM(name), ''), NULLIF(TRIM(title), '')))
		FROM threads
		WHERE archived = 0
		  AND source = 'vscode'
		  AND cwd IN (` + strings.Join(quotedPaths, ",") + `)
		ORDER BY cwd, recency_at_ms DESC, updated_at_ms DESC;
	`
	out, err := exec.Command(sqlitePath, "-batch", "-cmd", ".timeout 1000", stateDatabase, query).Output()
	if err != nil {
		return titles
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) != 2 {
			continue
		}
		pathBytes, pathErr := hex.DecodeString(parts[0])
		titleBytes, titleErr := hex.DecodeString(parts[1])
		if pathErr != nil || titleErr != nil {
			continue
		}
		workspacePath := string(pathBytes)
		if _, exists := titles[workspacePath]; !exists {
			titles[workspacePath] = string(titleBytes)
		}
	}

	return titles
}

func codexHomeDir() string {
	if configured := strings.TrimSpace(os.Getenv("CODEX_HOME")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func latestCodexStateDatabase(codexDir string) string {
	paths, _ := filepath.Glob(filepath.Join(codexDir, "state_*.sqlite"))
	sort.Slice(paths, func(i, j int) bool {
		return codexStateDatabaseVersion(paths[i]) > codexStateDatabaseVersion(paths[j])
	})
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func codexStateDatabaseVersion(path string) int {
	name := filepath.Base(path)
	version := strings.TrimSuffix(strings.TrimPrefix(name, "state_"), ".sqlite")
	parsed, _ := strconv.Atoi(version)
	return parsed
}
