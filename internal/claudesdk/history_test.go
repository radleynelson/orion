package claudesdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestParseClaudeTranscriptReadsRuntimeItems(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "thread-runtime.jsonl")
	content := `{"type":"user","uuid":"user-1","cwd":` + quoteJSON(workspace) + `,"sessionId":"thread-runtime","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Exercise Claude transcript parsing."}]}}
{"type":"assistant","uuid":"assistant-tool","cwd":` + quoteJSON(workspace) + `,"sessionId":"thread-runtime","timestamp":"2026-05-15T10:00:01Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"thinking","thinking":"Checking transcript states."},{"type":"tool_use","id":"tool-bash","name":"Bash","input":{"command":"pwd","description":"print cwd"}}]}}
{"type":"user","uuid":"tool-result","cwd":` + quoteJSON(workspace) + `,"sessionId":"thread-runtime","timestamp":"2026-05-15T10:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-bash","content":` + quoteJSON(workspace) + `,"is_error":false}]}}
{"type":"assistant","uuid":"assistant-question","cwd":` + quoteJSON(workspace) + `,"sessionId":"thread-runtime","timestamp":"2026-05-15T10:00:03Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-question","name":"AskUserQuestion","input":{"questions":[{"header":"Mode","question":"Which label should I use?","options":[{"label":"A"},{"label":"B"}]}]}}]}}
{"type":"assistant","uuid":"assistant-text","cwd":` + quoteJSON(workspace) + `,"sessionId":"thread-runtime","timestamp":"2026-05-15T10:00:04Z","message":{"role":"assistant","content":[{"type":"text","text":"The renderer should show this answer."}]}}
{"type":"assistant","uuid":"assistant-plan","cwd":` + quoteJSON(workspace) + `,"sessionId":"thread-runtime","timestamp":"2026-05-15T10:00:05Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-plan","name":"ExitPlanMode","input":{"plan":"# Plan ready\n\n- Verify replay.\n- Verify live updates.","planFilePath":"/tmp/plan.md"}}]}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, meta, err := parseClaudeTranscript(path, workspace, "mobile-session")
	if err != nil {
		t.Fatalf("parseClaudeTranscript: %v", err)
	}
	if meta.ID != "thread-runtime" {
		t.Fatalf("meta.ID = %q, want thread-runtime", meta.ID)
	}
	if meta.Model != "claude-opus-4-7" {
		t.Fatalf("meta.Model = %q", meta.Model)
	}
	assertClaudeMessage(t, messages, "user", "", "Exercise Claude transcript parsing.")
	assertClaudeMessage(t, messages, "thinking_delta", "", "Checking transcript states.")
	assertClaudeMessage(t, messages, "tool", "Bash", "pwd")
	assertClaudeMessage(t, messages, "tool_result", "Bash", workspace)
	assertClaudeMessage(t, messages, "permission_request", "AskUserQuestion", "Which label should I use?")
	assertClaudeMessage(t, messages, "assistant", "", "The renderer should show this answer.")
	assertClaudeMessage(t, messages, "plan", "ExitPlanMode", "Plan ready")
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

func assertClaudeMessage(t *testing.T, messages []Message, typ string, toolName string, contains string) {
	t.Helper()
	for _, msg := range messages {
		if msg.Type != typ {
			continue
		}
		if toolName != "" && msg.ToolName != toolName {
			continue
		}
		if contains != "" && !strings.Contains(msg.Text, contains) && !strings.Contains(msg.Details, contains) {
			continue
		}
		return
	}
	t.Fatalf("missing Claude message type=%q tool=%q contains=%q in %#v", typ, toolName, contains, messages)
}

func quoteJSON(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}
