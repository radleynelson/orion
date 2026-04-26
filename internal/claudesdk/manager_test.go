package claudesdk

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseClaudeSessionIDs(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "session id flag",
			command: "claude --dangerously-skip-permissions --session-id e4f52ec0-3da4-401c-b99c-aa6f4d221d64",
			want:    []string{"e4f52ec0-3da4-401c-b99c-aa6f4d221d64"},
		},
		{
			name:    "resume equals",
			command: "claude --resume=204b42f3-4200-4e5a-8079-0878793d6136",
			want:    []string{"204b42f3-4200-4e5a-8079-0878793d6136"},
		},
		{
			name:    "resume short flag",
			command: "claude -r f19446af-d564-4370-851e-b6bb6aaa3e08",
			want:    []string{"f19446af-d564-4370-851e-b6bb6aaa3e08"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClaudeSessionIDs(tt.command)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestValidClaudeSessionForWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspacePath := "/Users/rad_nelson/Desktop/code/slant"
	sessionID := "204b42f3-4200-4e5a-8079-0878793d6136"
	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspacePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !validClaudeSessionForWorkspace(sessionID, workspacePath) {
		t.Fatalf("expected session to be valid")
	}
	if validClaudeSessionForWorkspace("e4f52ec0-3da4-401c-b99c-aa6f4d221d64", workspacePath) {
		t.Fatalf("missing session should not be valid")
	}
}

func TestLatestSessionIDForWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspacePath := "/Users/rad_nelson/Desktop/code/slant"
	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspacePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(dir, "older.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	if err := os.WriteFile(older, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if got := latestSessionIDForWorkspace(workspacePath); got != "newer" {
		t.Fatalf("got %q, want newer", got)
	}
}
