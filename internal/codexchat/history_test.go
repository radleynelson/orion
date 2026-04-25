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
