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

func TestSessionEnvelopeDoesNotClearRunningStatus(t *testing.T) {
	session := &Session{
		manager:     NewManager(),
		id:          "claude-chat-test",
		label:       "Claude Chat",
		status:      "running",
		ready:       make(chan struct{}),
		subscribers: make(map[chan Message]struct{}),
	}

	session.handleEnvelope(bridgeEnvelope{
		Type:     "session",
		ThreadID: "thread-1",
		Model:    "claude-opus-4-7",
	})

	if got := session.currentStatus(); got != "running" {
		t.Fatalf("status = %q, want running", got)
	}

	for _, msg := range session.Messages() {
		if msg.Type == "status" && msg.Status == "idle" {
			t.Fatalf("session init emitted idle while turn was running: %#v", msg)
		}
	}
}

func TestValidClaudeSessionForWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
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
	t.Setenv("CLAUDE_CONFIG_DIR", "")
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

func TestResolveThreadIDFallsBackToOnlyClaudeHistoryWhenProcessIDInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	workspacePath := "/Users/rad_nelson/Desktop/code/voltra_mcp"
	validID := "f7b01a91-8d31-4fd3-8f47-16820af1f45d"
	invalidID := "f706d222-3fc1-479f-ba70-a817a2d50e28"
	writeClaudeHistory(t, home, workspacePath, validID, "Help me scaffold this MCP server.")

	got, err := resolveThreadIDForWorkspace(workspacePath, "", []string{invalidID})
	if err != nil {
		t.Fatalf("resolveThreadIDForWorkspace returned error: %v", err)
	}
	if got != validID {
		t.Fatalf("got %q, want %q", got, validID)
	}
}

func TestResolveThreadIDRejectsAmbiguousClaudeHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	workspacePath := "/Users/rad_nelson/Desktop/code/slant"
	writeClaudeHistory(t, home, workspacePath, "first-thread", "First thread.")
	writeClaudeHistory(t, home, workspacePath, "second-thread", "Second thread.")

	got, err := resolveThreadIDForWorkspace(workspacePath, "", []string{"missing-thread"})
	if err == nil {
		t.Fatalf("expected ambiguity error, got thread %q", got)
	}
	if got != "" {
		t.Fatalf("got %q, want empty thread on ambiguity", got)
	}
}

func TestResolveThreadIDUsesValidProcessIDBeforeHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	workspacePath := "/Users/rad_nelson/Desktop/code/slant"
	processID := "204b42f3-4200-4e5a-8079-0878793d6136"
	writeClaudeHistory(t, home, workspacePath, processID, "Process transcript.")
	writeClaudeHistory(t, home, workspacePath, "other-thread", "Other transcript.")

	got, err := resolveThreadIDForWorkspace(workspacePath, "", []string{processID})
	if err != nil {
		t.Fatalf("resolveThreadIDForWorkspace returned error: %v", err)
	}
	if got != processID {
		t.Fatalf("got %q, want %q", got, processID)
	}
}

func writeClaudeHistory(t *testing.T, home string, workspacePath string, sessionID string, text string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspacePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"type":"user","uuid":"` + sessionID + `","cwd":"` + workspacePath + `","sessionId":"` + sessionID + `","timestamp":"2026-04-20T10:00:00Z","message":{"role":"user","content":"` + text + `"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}
