package codexchat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadHistoryReadsCodexJSONL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	threadID := "019d9f18-9278-7f90-96bb-78390d0560e1"
	workspace := filepath.Join(t.TempDir(), "orion")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "20")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "rollout-2026-04-20T10-00-00-"+threadID+".jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-04-20T10:00:00Z","payload":{"id":"` + threadID + `","cwd":"` + workspace + `"}}
{"type":"event_msg","timestamp":"2026-04-20T10:00:01Z","payload":{"type":"user_message","message":"Hello","images":[],"local_images":[]}}
{"type":"event_msg","timestamp":"2026-04-20T10:00:02Z","payload":{"type":"agent_message","message":"Hi there","phase":"final_answer"}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	messages := LoadHistory(threadID, workspace)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d: %#v", len(messages), messages)
	}
	if messages[0].Type != "user" || messages[0].Text != "Hello" {
		t.Fatalf("unexpected first message: %#v", messages[0])
	}
	if messages[1].Type != "assistant" || messages[1].Text != "Hi there" {
		t.Fatalf("unexpected second message: %#v", messages[1])
	}
}

func TestParseCodexTranscriptReadsRuntimeItems(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	threadID := "thread-with-runtime-items"
	workspace := filepath.Join(t.TempDir(), "orion")
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "15")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "runtime.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"` + threadID + `","cwd":"` + workspace + `","model":"gpt-5.5"}}
{"type":"turn_context","timestamp":"2026-05-15T10:00:01Z","payload":{"model":"gpt-5.5","effort":"xhigh","collaboration_mode":{"mode":"default","settings":{"model":"gpt-5.5","reasoning_effort":"xhigh"}}}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:02Z","payload":{"type":"task_started"}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:03Z","payload":{"type":"user_message","message":"Run the focused tests","images":[],"local_images":[]}}
{"type":"response_item","timestamp":"2026-05-15T10:00:04Z","payload":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"Need to run the focused package tests."}]}}
{"type":"response_item","timestamp":"2026-05-15T10:00:05Z","payload":{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"go test ./internal/codexchat\",\"workdir\":\"` + workspace + `\"}"}}
{"type":"response_item","timestamp":"2026-05-15T10:00:06Z","payload":{"type":"function_call_output","call_id":"call_1","output":"ok  \torion/internal/codexchat\t0.482s"}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:07Z","payload":{"type":"agent_message","message":"Done.","phase":"final_answer"}}
{"type":"response_item","timestamp":"2026-05-15T10:00:08Z","payload":{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"Done."}]}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:09Z","payload":{"type":"task_complete","last_agent_message":"Done."}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	messages, meta, err := parseCodexTranscript(path, workspace, "orion-test-session")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != threadID || meta.Model != "gpt-5.5" || meta.ReasoningEffort != "xhigh" {
		t.Fatalf("unexpected meta: %#v", meta)
	}
	assertCodexMessage(t, messages, "status", "", "running", "")
	assertCodexMessage(t, messages, "user", "", "", "Run the focused tests")
	assertCodexMessage(t, messages, "thinking_delta", "", "", "Need to run the focused package tests.")
	assertCodexMessage(t, messages, "tool", "Bash", "", "go test ./internal/codexchat")
	assertCodexMessage(t, messages, "tool_result", "Bash", "", "orion/internal/codexchat")
	assertCodexMessage(t, messages, "assistant", "", "", "Done.")
	assertCodexMessage(t, messages, "result", "", "", "completed")

	assistantCount := 0
	for _, msg := range messages {
		if msg.Type == "assistant" && msg.Text == "Done." {
			assistantCount++
		}
	}
	if assistantCount != 1 {
		t.Fatalf("expected duplicate assistant rows to collapse, got %d in %#v", assistantCount, messages)
	}
}

func TestParseCodexTranscriptMapsPlanModeFinalAnswerToPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	threadID := "thread-with-plan"
	workspace := filepath.Join(t.TempDir(), "orion")
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "15")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "plan.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"` + threadID + `","cwd":"` + workspace + `"}}
{"type":"turn_context","timestamp":"2026-05-15T10:00:01Z","payload":{"collaboration_mode":{"mode":"plan","settings":{"reasoning_effort":"high"}}}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:02Z","payload":{"type":"agent_message","message":"## Plan\n- Inspect the transcript format.\n- Wire the parser into mobile chat.","phase":"final_answer"}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	messages, _, err := parseCodexTranscript(path, workspace, "orion-test-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 plan message, got %d: %#v", len(messages), messages)
	}
	if messages[0].Type != "plan" || messages[0].Status != "waiting_approval" || messages[0].Text != "Plan ready" {
		t.Fatalf("unexpected plan message: %#v", messages[0])
	}
	if messages[0].Details == "" || messages[0].Details == "Plan ready" {
		t.Fatalf("expected plan details, got %#v", messages[0])
	}
}

func TestParseCodexTranscriptKeepsRepeatedUserMessages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	workspace := filepath.Join(t.TempDir(), "orion")
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "15")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "repeated.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"thread-repeat","cwd":"` + workspace + `"}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:01Z","payload":{"type":"user_message","message":"try again","images":[],"local_images":[]}}
{"type":"event_msg","timestamp":"2026-05-15T10:00:02Z","payload":{"type":"user_message","message":"try again","images":[],"local_images":[]}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	messages, _, err := parseCodexTranscript(path, workspace, "orion-test-session")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, msg := range messages {
		if msg.Type == "user" && msg.Text == "try again" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected two repeated user messages, got %d in %#v", count, messages)
	}
}

func TestParseCodexTranscriptMapsAskUserQuestionToPermissionRequest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	workspace := filepath.Join(t.TempDir(), "orion")
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "15")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "question.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"thread-question","cwd":"` + workspace + `"}}
{"type":"response_item","timestamp":"2026-05-15T10:00:01Z","payload":{"type":"function_call","call_id":"ask_1","name":"request_user_input","arguments":"{\"question\":\"Which label should I use?\"}"}}
{"type":"response_item","timestamp":"2026-05-15T10:00:02Z","payload":{"type":"function_call_output","call_id":"ask_1","output":"Use Page header"}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	messages, _, err := parseCodexTranscript(path, workspace, "orion-test-session")
	if err != nil {
		t.Fatal(err)
	}
	assertCodexMessage(t, messages, "permission_request", "AskUserQuestion", "", "Which label should I use?")
	assertCodexMessage(t, messages, "permission_resolved", "AskUserQuestion", "", "Use Page header")
}

func TestParseCodexTranscriptMapsUpdatePlanToTaskList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	workspace := filepath.Join(t.TempDir(), "orion")
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "15")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "tasks.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"thread-tasks","cwd":"` + workspace + `"}}
{"type":"response_item","timestamp":"2026-05-15T10:00:01Z","payload":{"type":"function_call","call_id":"plan_1","name":"update_plan","arguments":"{\"plan\":[{\"step\":\"Inspect parser\",\"status\":\"completed\"},{\"step\":\"Render task card\",\"status\":\"in_progress\"},{\"step\":\"Run tests\",\"status\":\"pending\"}],\"explanation\":\"UI pass\"}"}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	messages, _, err := parseCodexTranscript(path, workspace, "orion-test-session")
	if err != nil {
		t.Fatal(err)
	}
	assertCodexMessage(t, messages, "task_list", "update_plan", "", "Render task card")
}

func TestLoadHistoryRejectsWrongWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", t.TempDir())
	threadID := "019d9f18-9278-7f90-96bb-78390d0560e1"
	workspace := filepath.Join(t.TempDir(), "orion")
	otherWorkspace := filepath.Join(t.TempDir(), "waterboy")
	writeCodexHistory(t, home, workspace, threadID, "Hello from Orion.")

	if messages := LoadHistory(threadID, otherWorkspace); len(messages) != 0 {
		t.Fatalf("expected no messages for wrong workspace, got %#v", messages)
	}
}

func assertCodexMessage(t *testing.T, messages []Message, typ string, toolName string, status string, contains string) {
	t.Helper()
	for _, msg := range messages {
		if msg.Type != typ {
			continue
		}
		if toolName != "" && msg.ToolName != toolName {
			continue
		}
		if status != "" && msg.Status != status {
			continue
		}
		if contains != "" && !strings.Contains(msg.Text, contains) && !strings.Contains(msg.Details, contains) {
			continue
		}
		return
	}
	t.Fatalf("missing message type=%q tool=%q status=%q contains=%q in %#v", typ, toolName, status, contains, messages)
}

func TestCachedMessagesPreserveRichRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	threadID := "thread-rich-cache"

	AppendCachedMessage(Message{ID: "status-1", ThreadID: threadID, Type: "status", Status: "running"})
	AppendCachedMessage(Message{ID: "question-1", ThreadID: threadID, Type: "permission_request", ToolUseID: "tool-1", ToolName: "AskUserQuestion", Text: "Where should the label live?"})
	AppendCachedMessage(Message{ID: "answer-1", ThreadID: threadID, Type: "permission_submitted", ToolUseID: "tool-1", ToolName: "AskUserQuestion", Text: "Page header"})

	messages := LoadCachedMessages(threadID, "")
	if len(messages) != 2 {
		t.Fatalf("expected 2 cached messages, got %d: %#v", len(messages), messages)
	}
	if messages[0].SessionID != "" || messages[0].Type != "permission_request" || messages[0].Text != "Where should the label live?" {
		t.Fatalf("unexpected cached question: %#v", messages[0])
	}
	if messages[1].Type != "permission_submitted" || messages[1].Text != "Page header" {
		t.Fatalf("unexpected cached answer: %#v", messages[1])
	}
}

func TestCachedPlanMessagesNormalizeDetails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	threadID := "thread-plan-cache"
	rawPlan := `{"explanation":"QA-only plan.","plan":[{"status":"pending","step":"Inspect README.md."}]}`

	AppendCachedMessage(Message{ID: "plan-1", ThreadID: threadID, Type: "plan", Text: "Plan updated", Details: rawPlan})

	messages := LoadCachedMessages(threadID, "")
	if len(messages) != 1 {
		t.Fatalf("expected 1 cached plan, got %d: %#v", len(messages), messages)
	}
	if messages[0].Text != "Plan ready" {
		t.Fatalf("Text = %q, want Plan ready", messages[0].Text)
	}
	if messages[0].Details != "### Summary\nQA-only plan.\n\n### Steps\n1. [pending] Inspect README.md." {
		t.Fatalf("Details = %q", messages[0].Details)
	}
}

func TestMergeRestoredMessagesIncludesHistoryWhenCacheHasOnlyMetadata(t *testing.T) {
	cached := []Message{
		{ID: "system-1", Type: "system", Text: "Codex chat ready", CreatedAt: "2026-04-20T10:00:03Z"},
	}
	history := []Message{
		{ID: "history-user", Type: "user", Role: "user", Text: "Hello", CreatedAt: "2026-04-20T10:00:01Z"},
		{ID: "history-assistant", Type: "assistant", Role: "assistant", Text: "Hi there", CreatedAt: "2026-04-20T10:00:02Z"},
	}

	messages := MergeRestoredMessages(cached, history)
	if len(messages) != 3 {
		t.Fatalf("expected cached metadata plus history, got %d: %#v", len(messages), messages)
	}
	if messages[0].Type != "user" || messages[1].Type != "assistant" || messages[2].Type != "system" {
		t.Fatalf("unexpected merged order: %#v", messages)
	}
}

func TestLatestThreadIDForWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	workspace := filepath.Join(t.TempDir(), "waterboy")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "20")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldID := "old-thread"
	newID := "new-thread"
	oldPath := filepath.Join(sessionDir, "old.jsonl")
	newPath := filepath.Join(sessionDir, "new.jsonl")
	if err := os.WriteFile(oldPath, []byte(`{"type":"session_meta","payload":{"id":"`+oldID+`","cwd":"`+workspace+`"}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(`{"type":"session_meta","payload":{"id":"`+newID+`","cwd":"`+workspace+`"}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldPath, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}

	if got := LatestThreadIDForWorkspace(workspace); got != newID {
		t.Fatalf("LatestThreadIDForWorkspace = %q, want %q", got, newID)
	}
}

func TestResolveThreadIDFallsBackToOnlyCodexHistoryWhenProcessIDInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", t.TempDir())
	workspace := filepath.Join(t.TempDir(), "slant")
	validID := "019dc827-72fb-7100-9770-33b63986e2ea"
	writeCodexHistory(t, home, workspace, validID, "Plan a README update.")

	got, err := resolveThreadIDForWorkspace(workspace, "", []string{"missing-thread"})
	if err != nil {
		t.Fatalf("resolveThreadIDForWorkspace returned error: %v", err)
	}
	if got != validID {
		t.Fatalf("got %q, want %q", got, validID)
	}
}

func TestResolveThreadIDRejectsAmbiguousCodexHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", t.TempDir())
	workspace := filepath.Join(t.TempDir(), "slant")
	writeCodexHistory(t, home, workspace, "first-thread", "First thread.")
	writeCodexHistory(t, home, workspace, "second-thread", "Second thread.")

	got, err := resolveThreadIDForWorkspace(workspace, "", []string{"missing-thread"})
	if err == nil {
		t.Fatalf("expected ambiguity error, got thread %q", got)
	}
	if got != "" {
		t.Fatalf("got %q, want empty thread on ambiguity", got)
	}
}

func TestResolveThreadIDUsesValidCodexProcessIDBeforeHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", t.TempDir())
	workspace := filepath.Join(t.TempDir(), "slant")
	processID := "019d9f18-9278-7f90-96bb-78390d0560e1"
	writeCodexHistory(t, home, workspace, processID, "Process thread.")
	writeCodexHistory(t, home, workspace, "other-thread", "Other thread.")

	got, err := resolveThreadIDForWorkspace(workspace, "", []string{processID})
	if err != nil {
		t.Fatalf("resolveThreadIDForWorkspace returned error: %v", err)
	}
	if got != processID {
		t.Fatalf("got %q, want %q", got, processID)
	}
}

func TestHistoryThreadIDForWorkspaceWindowUsesSingleTranscriptInWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", t.TempDir())
	workspace := filepath.Join(t.TempDir(), "notetaker")
	start := time.Date(2026, 4, 27, 22, 46, 42, 0, time.UTC)
	end := start.Add(30 * time.Second)

	writeCodexHistoryAt(t, home, workspace, "before-thread", "Before window.", start.Add(-25*time.Second))
	writeCodexHistoryAt(t, home, workspace, "target-thread", "Inside window.", start.Add(1*time.Second))
	writeCodexHistoryAt(t, home, workspace, "after-thread", "After window.", end.Add(1*time.Second))

	got, ok := historyThreadIDForWorkspaceWindow(workspace, start, end)
	if !ok {
		t.Fatalf("expected a single history candidate")
	}
	if got != "target-thread" {
		t.Fatalf("got %q, want target-thread", got)
	}
}

func TestHistoryThreadIDForWorkspaceWindowRejectsAmbiguousWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", t.TempDir())
	workspace := filepath.Join(t.TempDir(), "notetaker")
	start := time.Date(2026, 4, 27, 22, 46, 42, 0, time.UTC)

	writeCodexHistoryAt(t, home, workspace, "first-thread", "First in window.", start.Add(1*time.Second))
	writeCodexHistoryAt(t, home, workspace, "second-thread", "Second in window.", start.Add(2*time.Second))

	got, ok := historyThreadIDForWorkspaceWindow(workspace, start, time.Time{})
	if ok {
		t.Fatalf("expected ambiguous window to be rejected, got %q", got)
	}
	if got != "" {
		t.Fatalf("got %q, want empty thread on ambiguity", got)
	}
}

func TestThreadOptionsReadsTurnContextModelAndEffort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	workspace := filepath.Join(t.TempDir(), "slant")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "20")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	threadID := "thread-with-turn-context"
	path := filepath.Join(sessionDir, "turn-context.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-04-20T10:00:00Z","payload":{"id":"` + threadID + `","cwd":"` + workspace + `"}}
{"type":"turn_context","timestamp":"2026-04-20T10:00:01Z","payload":{"model":"gpt-5.5","effort":"xhigh","collaboration_mode":{"mode":"default","settings":{"model":"gpt-5.5","reasoning_effort":"xhigh"}}}}
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	options := ThreadOptions(threadID)
	if options.Model != "gpt-5.5" {
		t.Fatalf("Model = %q, want gpt-5.5", options.Model)
	}
	if options.ReasoningEffort != "xhigh" {
		t.Fatalf("ReasoningEffort = %q, want xhigh", options.ReasoningEffort)
	}
	if options.CollaborationMode != "default" {
		t.Fatalf("CollaborationMode = %q, want default", options.CollaborationMode)
	}
}

func writeCodexHistory(t *testing.T, home string, workspace string, threadID string, text string) {
	t.Helper()
	writeCodexHistoryAt(t, home, workspace, threadID, text, time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC))
}

func writeCodexHistoryAt(t *testing.T, home string, workspace string, threadID string, text string, timestamp time.Time) {
	t.Helper()
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "20")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	startedAt := timestamp.UTC().Format(time.RFC3339Nano)
	messageAt := timestamp.Add(time.Second).UTC().Format(time.RFC3339Nano)
	raw := `{"type":"session_meta","timestamp":"` + startedAt + `","payload":{"id":"` + threadID + `","cwd":"` + workspace + `","model":"gpt-5.4"}}
{"type":"event_msg","timestamp":"` + messageAt + `","payload":{"type":"user_message","message":"` + text + `","images":[],"local_images":[]}}
`
	if err := os.WriteFile(filepath.Join(sessionDir, threadID+".jsonl"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestListHistoryFiltersWorkspaceAndBuildsPreview(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	workspace := filepath.Join(t.TempDir(), "orion")
	otherWorkspace := filepath.Join(t.TempDir(), "other")
	sessionDir := filepath.Join(home, "sessions", "2026", "04", "20")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	threadID := "thread-orion"
	threadPath := filepath.Join(sessionDir, "orion.jsonl")
	raw := `{"type":"session_meta","timestamp":"2026-04-20T10:00:00Z","payload":{"id":"` + threadID + `","cwd":"` + workspace + `","model":"gpt-5.4"}}
{"type":"event_msg","timestamp":"2026-04-20T10:00:01Z","payload":{"type":"user_message","message":"Please inspect the login bug","images":[],"local_images":[]}}
{"type":"event_msg","timestamp":"2026-04-20T10:00:02Z","payload":{"type":"agent_message","message":"The bug is in auth middleware.","phase":"final_answer"}}
`
	if err := os.WriteFile(threadPath, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(sessionDir, "other.jsonl")
	if err := os.WriteFile(otherPath, []byte(`{"type":"session_meta","payload":{"id":"thread-other","cwd":"`+otherWorkspace+`"}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	history := ListHistory(workspace, 10)
	if len(history) != 1 {
		t.Fatalf("expected 1 history thread, got %d: %#v", len(history), history)
	}
	if history[0].ThreadID != threadID {
		t.Fatalf("ThreadID = %q, want %q", history[0].ThreadID, threadID)
	}
	if history[0].Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want gpt-5.4", history[0].Model)
	}
	if history[0].MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2", history[0].MessageCount)
	}
	if history[0].Preview != "The bug is in auth middleware." {
		t.Fatalf("Preview = %q", history[0].Preview)
	}
}

func TestParseResumeIDs(t *testing.T) {
	command := "codex resume --dangerously-bypass-approvals-and-sandbox --no-alt-screen 019d9f18-9278-7f90-96bb-78390d0560e1"
	got := ParseResumeIDs(command)
	if len(got) != 1 || got[0] != "019d9f18-9278-7f90-96bb-78390d0560e1" {
		t.Fatalf("ParseResumeIDs = %#v", got)
	}

	got = ParseResumeIDs("codex --resume=abc123")
	if len(got) != 1 || got[0] != "abc123" {
		t.Fatalf("ParseResumeIDs with equals = %#v", got)
	}

	command = `codex resume --dangerously-bypass-approvals-and-sandbox --no-alt-screen -m gpt-5.5 -c model_reasoning_effort="xhigh" 019dc827-72fb-7100-9770-33b63986e2ea`
	got = ParseResumeIDs(command)
	if len(got) != 1 || got[0] != "019dc827-72fb-7100-9770-33b63986e2ea" {
		t.Fatalf("ParseResumeIDs with options = %#v", got)
	}
}
