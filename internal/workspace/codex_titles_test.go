package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orion/internal/workspacekey"
)

func TestApplyCodexWorkspaceTitlesUsesCurrentDesktopTaskName(t *testing.T) {
	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not available")
	}

	codexDir := t.TempDir()
	t.Setenv("CODEX_HOME", codexDir)
	workspacePath := filepath.Join(t.TempDir(), "it's-a-worktree")
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := workspacekey.Save(workspacePath, workspacekey.Metadata{
		Name:      "slant-codex-abcd",
		ManagedBy: "codex",
	}); err != nil {
		t.Fatal(err)
	}

	databasePath := filepath.Join(codexDir, "state_5.sqlite")
	query := `
		CREATE TABLE threads (
			cwd TEXT NOT NULL,
			name TEXT,
			title TEXT NOT NULL,
			archived INTEGER NOT NULL,
			source TEXT NOT NULL,
			recency_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		);
		INSERT INTO threads VALUES (` + sqlString(workspacePath) + `, 'Older task', 'Older prompt', 0, 'vscode', 1, 1);
		INSERT INTO threads VALUES (` + sqlString(workspacePath) + `, 'Current Codex title', 'Current prompt', 0, 'vscode', 3, 3);
		INSERT INTO threads VALUES (` + sqlString(workspacePath) + `, 'Archived task', 'Archived prompt', 1, 'vscode', 4, 4);
		INSERT INTO threads VALUES (` + sqlString(workspacePath) + `, 'Review helper', 'Review prompt', 0, '{"subagent":"review"}', 5, 5);
	`
	if out, err := exec.Command(sqlitePath, databasePath, query).CombinedOutput(); err != nil {
		t.Fatalf("create Codex state fixture: %v\n%s", err, out)
	}

	workspaces := []Workspace{{Name: "slant-codex-abcd", Path: workspacePath}}
	applyCodexWorkspaceTitles(workspaces)
	if got := workspaces[0].Name; got != "Current Codex title" {
		t.Fatalf("workspace name = %q, want %q", got, "Current Codex title")
	}
}

func TestLatestCodexStateDatabaseUsesHighestVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"state_2.sqlite", "state_11.sqlite", "state_5.sqlite"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if got := latestCodexStateDatabase(dir); got != filepath.Join(dir, "state_11.sqlite") {
		t.Fatalf("latestCodexStateDatabase = %q", got)
	}
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
