package codexchat

import (
	"os"
	"path/filepath"
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
}
