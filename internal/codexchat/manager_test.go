package codexchat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRawIDValuePreservesServerRequestIDType(t *testing.T) {
	numeric := rawIDValue(json.RawMessage(`0`))
	if _, ok := numeric.(int64); !ok {
		t.Fatalf("numeric rawIDValue type = %T, want int64", numeric)
	}
	if numeric != int64(0) {
		t.Fatalf("numeric rawIDValue = %#v, want int64(0)", numeric)
	}

	stringID := rawIDValue(json.RawMessage(`"0"`))
	if _, ok := stringID.(string); !ok {
		t.Fatalf("string rawIDValue type = %T, want string", stringID)
	}
	if stringID != "0" {
		t.Fatalf("string rawIDValue = %#v, want %q", stringID, "0")
	}
}

func TestPlanItemSwitchesCollaborationModeAndKeepsWaitingStatus(t *testing.T) {
	session := &Session{
		manager:           NewManager(),
		id:                "codex-chat-test",
		threadID:          "thread-test",
		status:            "running",
		collaborationMode: defaultCollabMode,
		subscribers:       make(map[chan Message]struct{}),
		pendingInputs:     make(map[string]pendingInput),
	}

	session.processItemCompleted(map[string]any{
		"id":   "plan-item",
		"type": "plan",
		"text": "# Plan\n\n- Check README\n- Report back",
	})
	if session.collaborationMode != "plan" {
		t.Fatalf("collaborationMode = %q, want plan", session.collaborationMode)
	}
	if session.status != "waiting_input" {
		t.Fatalf("status after plan = %q, want waiting_input", session.status)
	}

	session.handleTurnCompleted(map[string]any{})
	if session.status != "waiting_input" {
		t.Fatalf("status after turn completed = %q, want waiting_input", session.status)
	}
}

func TestFormatPlanDetailsFormatsCodexPlanPayload(t *testing.T) {
	got := formatPlanDetails(map[string]any{
		"explanation": "QA-only plan.",
		"plan": []any{
			map[string]any{"status": "pending", "step": "Inspect README.md."},
			map[string]any{"status": "pending", "step": "Verify the diff."},
		},
	})
	want := "### Summary\nQA-only plan.\n\n### Steps\n1. [pending] Inspect README.md.\n2. [pending] Verify the diff."
	if got != want {
		t.Fatalf("formatPlanDetails = %q, want %q", got, want)
	}
}

func TestTmuxAttachedSessionTailsTranscriptAndSendsInput(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	workspace := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("HOME", t.TempDir())

	inputPath := filepath.Join(workspace, "tmux-input.txt")
	tmuxName := "orion-codex-test-" + shortID()
	tmuxCommand := fmt.Sprintf(`IFS= read -r line; printf "%%s" "$line" > %s; sleep 60`, shellQuote(inputPath))
	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", workspace, "sh", "-lc", tmuxCommand)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not create tmux session: %v %s", err, strings.TrimSpace(string(out)))
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	threadID := "thread-tail-send"
	decoyPath := filepath.Join(codexHome, "sessions", "2026", "05", "15", "decoy.jsonl")
	writeTranscriptLines(t, decoyPath,
		`{"type":"session_meta","timestamp":"2026-05-15T09:59:00Z","payload":{"id":"thread-decoy","cwd":"`+workspace+`","model":"gpt-5.5"}}`,
		`{"type":"event_msg","timestamp":"2026-05-15T09:59:01Z","payload":{"type":"user_message","message":"wrong transcript","images":[],"local_images":[]}}`,
	)
	transcriptPath := filepath.Join(codexHome, "sessions", "2026", "05", "15", "tail-send.jsonl")
	writeTranscriptLines(t, transcriptPath,
		`{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"`+threadID+`","cwd":"`+workspace+`","model":"gpt-5.5"}}`,
		`{"type":"event_msg","timestamp":"2026-05-15T10:00:01Z","payload":{"type":"user_message","message":"initial prompt","images":[],"local_images":[]}}`,
	)
	setTmuxOption(tmuxName, "@orion_transcript_path", transcriptPath)

	manager := NewManager()
	info, err := manager.AttachSince(tmuxName, workspace, "Codex", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("AttachSince: %v", err)
	}
	session, ok := manager.Get(info.ID)
	if !ok {
		t.Fatalf("attached session %q not found", info.ID)
	}
	defer session.Detach()

	if !hasMessage(session.Messages(), "user", "", "initial prompt") {
		t.Fatalf("initial transcript message not loaded: %#v", session.Messages())
	}
	if hasMessage(session.Messages(), "user", "", "wrong transcript") {
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
	appendTranscriptLines(t, transcriptPath,
		`{"type":"event_msg","timestamp":"2026-05-15T10:00:02Z","payload":{"type":"task_started"}}`,
		`{"type":"response_item","timestamp":"2026-05-15T10:00:03Z","payload":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"Thinking through the command."}]}}`,
		`{"type":"response_item","timestamp":"2026-05-15T10:00:04Z","payload":{"type":"function_call","call_id":"call_tail","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}"}}`,
		`{"type":"response_item","timestamp":"2026-05-15T10:00:05Z","payload":{"type":"function_call_output","call_id":"call_tail","output":"`+workspace+`"}}`,
		`{"type":"event_msg","timestamp":"2026-05-15T10:00:06Z","payload":{"type":"agent_message","message":"Done tailing.","phase":"final_answer"}}`,
	)
	if err := session.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	requireUpdate(t, updates, "status", "", "")
	requireUpdate(t, updates, "thinking_delta", "", "Thinking through the command.")
	requireUpdate(t, updates, "tool", "Bash", "pwd")
	requireUpdate(t, updates, "tool_result", "Bash", workspace)
	requireUpdate(t, updates, "assistant", "", "Done tailing.")

	if err := session.Send("hello from mobile", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForFileContent(t, inputPath, "hello from mobile")
}

func writeTranscriptLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTranscriptLines(t *testing.T, path string, lines ...string) {
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

func requireUpdate(t *testing.T, updates <-chan Message, typ string, toolName string, contains string) {
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
			return
		case <-deadline:
			t.Fatalf("missing update type=%q tool=%q contains=%q", typ, toolName, contains)
		}
	}
}

func hasMessage(messages []Message, typ string, toolName string, contains string) bool {
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

func waitForFileContent(t *testing.T, path string, want string) {
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
