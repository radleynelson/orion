package claudechat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTranscriptExtractsPlanMessage(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(dir, "session.jsonl")
	content := strings.Join([]string{
		`{"type":"user","uuid":"user-1","timestamp":"2026-04-19T22:00:00Z","cwd":"` + workspace + `","sessionId":"session-1","message":{"role":"user","content":"Make a plan"}}`,
		`{"type":"assistant","uuid":"assistant-1","timestamp":"2026-04-19T22:00:01Z","cwd":"` + workspace + `","sessionId":"session-1","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-write","name":"Write","input":{"file_path":"/Users/rad_nelson/.claude/plans/example.md","content":"# Plan: Demo\n\nStep one"}}]}}`,
		`{"type":"assistant","uuid":"assistant-2","timestamp":"2026-04-19T22:00:02Z","cwd":"` + workspace + `","sessionId":"session-1","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-search","name":"ToolSearch","input":{"query":"select:ExitPlanMode","max_results":1}}]}}`,
		`{"type":"assistant","uuid":"assistant-3","timestamp":"2026-04-19T22:00:03Z","cwd":"` + workspace + `","sessionId":"session-1","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-plan","name":"ExitPlanMode","input":{"plan":"# Plan: Demo\n\nStep one\n\nStep two","planFilePath":"/Users/rad_nelson/.claude/plans/example.md"}}]}}`,
	}, "\n")
	if err := os.WriteFile(transcript, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, threadID, err := parseTranscript(transcript, workspace, "tmux-1")
	if err != nil {
		t.Fatal(err)
	}
	if threadID != "session-1" {
		t.Fatalf("threadID = %q, want session-1", threadID)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want user plus plan; messages=%+v", len(messages), messages)
	}
	plan := messages[1]
	if plan.Type != "plan" {
		t.Fatalf("plan Type = %q, want plan", plan.Type)
	}
	if plan.Text != "Plan: Demo" {
		t.Fatalf("plan Text = %q, want Plan: Demo", plan.Text)
	}
	if !strings.Contains(plan.Details, "Step two") {
		t.Fatalf("plan Details = %q, want full plan markdown", plan.Details)
	}
	if plan.PlanPath != "/Users/rad_nelson/.claude/plans/example.md" {
		t.Fatalf("plan PlanPath = %q", plan.PlanPath)
	}
}

func TestTranscriptMatchingPromptSelectsPromptTranscript(t *testing.T) {
	workspace, oldTranscript, targetTranscript, baseTime := writeTranscriptFixture(t)
	hint := transcriptHint{Text: "unique prompt for this tmux", After: baseTime.Add(-time.Second)}

	got := transcriptMatchingPrompt(workspace, []transcriptHint{hint})
	if got != targetTranscript {
		t.Fatalf("transcriptMatchingPrompt = %q, want %q; old=%q", got, targetTranscript, oldTranscript)
	}
}

func TestFirstTranscriptForWorkspaceSelectsEarliestAfterAttachTime(t *testing.T) {
	workspace, firstTranscript, _, baseTime := writeTranscriptFixture(t)

	got := firstTranscriptForWorkspace(workspace, baseTime.Add(-time.Second))
	if got != firstTranscript {
		t.Fatalf("firstTranscriptForWorkspace = %q, want %q", got, firstTranscript)
	}
}

func TestParseClaudeResumeIDs(t *testing.T) {
	command := "claude --dangerously-skip-permissions --resume 17a79ca2-813f-4be3-970b-7316d5055b82 --effort xhigh"
	got := parseClaudeResumeIDs(command)
	if len(got) != 1 || got[0] != "17a79ca2-813f-4be3-970b-7316d5055b82" {
		t.Fatalf("parseClaudeResumeIDs = %#v", got)
	}

	got = parseClaudeResumeIDs("claude --resume=abc123")
	if len(got) != 1 || got[0] != "abc123" {
		t.Fatalf("parseClaudeResumeIDs with equals = %#v", got)
	}
}

func writeTranscriptFixture(t *testing.T) (workspace string, firstTranscript string, targetTranscript string, baseTime time.Time) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace = filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspace))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	baseTime = time.Date(2026, 4, 19, 22, 0, 0, 0, time.UTC)
	firstTranscript = filepath.Join(dir, "first.jsonl")
	targetTranscript = filepath.Join(dir, "target.jsonl")
	writeTranscript(t, firstTranscript, workspace, "first-session", "older prompt", baseTime)
	writeTranscript(t, targetTranscript, workspace, "target-session", "unique prompt for this tmux", baseTime.Add(2*time.Second))
	if err := os.Chtimes(firstTranscript, baseTime, baseTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(targetTranscript, baseTime.Add(2*time.Second), baseTime.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	return workspace, firstTranscript, targetTranscript, baseTime
}

func writeTranscript(t *testing.T, path string, workspace string, sessionID string, prompt string, timestamp time.Time) {
	t.Helper()
	line := `{"type":"user","uuid":"` + sessionID + `-user","timestamp":"` + timestamp.Format(time.RFC3339Nano) + `","cwd":"` + workspace + `","sessionId":"` + sessionID + `","message":{"role":"user","content":"` + prompt + `"}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
