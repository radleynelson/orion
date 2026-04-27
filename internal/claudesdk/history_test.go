package claudesdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListHistoryReadsClaudeProjectTranscripts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspace))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	newer := filepath.Join(dir, "new-session.jsonl")
	older := filepath.Join(dir, "old-session.jsonl")
	writeClaudeHistoryFixture(t, older, workspace, "old-session", "older prompt", "older answer", "2026-04-24T01:00:00Z")
	writeClaudeHistoryFixture(t, newer, workspace, "new-session", "Please plan the login fix", "Here is the login plan", "2026-04-25T01:00:00Z")
	oldTime := time.Date(2026, 4, 24, 1, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 4, 25, 1, 0, 0, 0, time.UTC)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	history := ListHistory(workspace, 10)
	if len(history) != 2 {
		t.Fatalf("expected 2 history threads, got %d: %#v", len(history), history)
	}
	if history[0].ThreadID != "new-session" {
		t.Fatalf("ThreadID = %q, want new-session", history[0].ThreadID)
	}
	if history[0].WorkspacePath != workspace {
		t.Fatalf("WorkspacePath = %q, want %q", history[0].WorkspacePath, workspace)
	}
	if history[0].Model != "claude-opus-4-7" {
		t.Fatalf("Model = %q, want claude-opus-4-7", history[0].Model)
	}
	if history[0].MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2", history[0].MessageCount)
	}
	if history[0].Preview != "Here is the login plan" {
		t.Fatalf("Preview = %q", history[0].Preview)
	}
}

func writeClaudeHistoryFixture(t *testing.T, path string, workspace string, sessionID string, userText string, assistantText string, timestamp string) {
	t.Helper()
	content := `{"type":"user","uuid":"user-` + sessionID + `","cwd":` + quoteJSON(workspace) + `,"sessionId":` + quoteJSON(sessionID) + `,"timestamp":"` + timestamp + `","message":{"role":"user","content":[{"type":"text","text":` + quoteJSON(userText) + `}]}}
{"type":"assistant","uuid":"assistant-` + sessionID + `","cwd":` + quoteJSON(workspace) + `,"sessionId":` + quoteJSON(sessionID) + `,"timestamp":"` + timestamp + `","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":` + quoteJSON(assistantText) + `}]}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quoteJSON(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}
