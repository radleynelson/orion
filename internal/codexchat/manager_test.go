package codexchat

import (
	"encoding/json"
	"testing"
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
