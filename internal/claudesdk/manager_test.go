package claudesdk

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestTranscriptSyncClearsRunningStatusAfterFinalAssistantMessage(t *testing.T) {
	session := &Session{
		manager:     NewManager(),
		id:          "claude-chat-test",
		label:       "Claude Chat",
		status:      "running",
		ready:       make(chan struct{}),
		subscribers: make(map[chan Message]struct{}),
	}

	session.updateStatusAfterTranscriptSync([]Message{
		{ID: "tool-1", Type: "tool", ToolName: "Bash", Text: "pwd"},
		{ID: "result-1", Type: "tool_result", ToolName: "Bash", Text: "/tmp/project"},
		{ID: "answer-1", Type: "assistant", Text: "Done."},
	}, "")

	if got := session.currentStatus(); got != "idle" {
		t.Fatalf("status = %q, want idle", got)
	}
	if !hasClaudeMessage(session.Messages(), "status", "", "") {
		t.Fatalf("expected idle status message, got %#v", session.Messages())
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

func TestTmuxAttachedSessionTailsTranscriptAndSendsInput(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	workspace := t.TempDir()
	claudeConfig := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeConfig)
	t.Setenv("HOME", t.TempDir())

	inputPath := filepath.Join(workspace, "tmux-input.txt")
	tmuxName := "orion-claude-test-" + shortID()
	tmuxCommand := fmt.Sprintf(`IFS= read -r line; printf "%%s" "$line" > %s; sleep 60`, shellQuote(inputPath))
	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", workspace, "sh", "-lc", tmuxCommand)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not create tmux session: %v %s", err, strings.TrimSpace(string(out)))
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	threadID := "thread-tail-send"
	decoyPath := filepath.Join(claudeConfig, "projects", claudeProjectDir(workspace), "thread-decoy.jsonl")
	writeClaudeTranscriptLines(t, decoyPath,
		`{"type":"user","uuid":"user-decoy","cwd":`+quoteJSON(workspace)+`,"sessionId":"thread-decoy","timestamp":"2026-05-15T09:59:00Z","message":{"role":"user","content":[{"type":"text","text":"wrong Claude transcript"}]}}`,
	)
	transcriptPath := filepath.Join(claudeConfig, "projects", claudeProjectDir(workspace), threadID+".jsonl")
	writeClaudeTranscriptLines(t, transcriptPath,
		`{"type":"user","uuid":"user-tail","cwd":`+quoteJSON(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"initial Claude prompt"}]}}`,
	)
	setTmuxOption(tmuxName, "@orion_transcript_path", transcriptPath)

	manager := NewManager()
	info, err := manager.AttachSince(tmuxName, workspace, "Claude", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("AttachSince: %v", err)
	}
	session, ok := manager.Get(info.ID)
	if !ok {
		t.Fatalf("attached session %q not found", info.ID)
	}
	defer session.Detach()

	if !hasClaudeMessage(session.Messages(), "user", "", "initial Claude prompt") {
		t.Fatalf("initial transcript message not loaded: %#v", session.Messages())
	}
	if hasClaudeMessage(session.Messages(), "user", "", "wrong Claude transcript") {
		t.Fatalf("stamped transcript path was not preferred: %#v", session.Messages())
	}
	if got := tmuxOption(tmuxName, "@orion_thread_id"); got != threadID {
		t.Fatalf("@orion_thread_id = %q, want %q", got, threadID)
	}
	if got := tmuxOption(tmuxName, "@orion_transcript_path"); got != transcriptPath {
		t.Fatalf("@orion_transcript_path = %q, want %q", got, transcriptPath)
	}
	if got := tmuxOption(tmuxName, "@orion_provider"); got != Provider {
		t.Fatalf("@orion_provider = %q, want %q", got, Provider)
	}

	updates, unsubscribe := session.Subscribe()
	defer unsubscribe()
	appendClaudeTranscriptLines(t, transcriptPath,
		`{"type":"assistant","uuid":"assistant-tail","cwd":`+quoteJSON(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:01Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"thinking","thinking":"Thinking through the Claude command."},{"type":"tool_use","id":"tool-tail","name":"Bash","input":{"command":"pwd","description":"print cwd"}}]}}`,
		`{"type":"user","uuid":"result-tail","cwd":`+quoteJSON(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-tail","content":`+quoteJSON(workspace)+`,"is_error":false}]}}`,
		`{"type":"assistant","uuid":"answer-tail","cwd":`+quoteJSON(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"Done tailing Claude."}]}}`,
	)
	if err := session.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	requireClaudeUpdate(t, updates, "thinking_delta", "", "Thinking through the Claude command.")
	requireClaudeUpdate(t, updates, "tool", "Bash", "pwd")
	requireClaudeUpdate(t, updates, "tool_result", "Bash", workspace)
	requireClaudeUpdate(t, updates, "assistant", "", "Done tailing Claude.")

	if err := session.Send("hello from Claude mobile", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForClaudeFileContent(t, inputPath, "hello from Claude mobile")
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

func writeClaudeTranscriptLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendClaudeTranscriptLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
}

func hasClaudeMessage(messages []Message, typ string, toolName string, contains string) bool {
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
		return true
	}
	return false
}

func requireClaudeUpdate(t *testing.T, updates <-chan Message, typ string, toolName string, contains string) Message {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-updates:
			if msg.Type != typ {
				continue
			}
			if toolName != "" && msg.ToolName != toolName {
				continue
			}
			if contains != "" && !strings.Contains(msg.Text, contains) && !strings.Contains(msg.Details, contains) {
				continue
			}
			return msg
		case <-deadline:
			t.Fatalf("missing update type=%q tool=%q contains=%q", typ, toolName, contains)
			return Message{}
		}
	}
}

func waitForClaudeFileContent(t *testing.T, path string, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if strings.TrimSpace(string(data)) == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("file %s = %q, want %q", path, strings.TrimSpace(string(data)), want)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
