package state

import "testing"

func TestDedupeSavedTabsUsesTmuxIdentityForTerminalTabs(t *testing.T) {
	tabs := []SavedTab{
		{TabType: "claude", TmuxSession: "orion-demo-main", WorkspacePath: "/repo/main", Label: "Claude"},
		{TabType: "codex", TmuxSession: "orion-demo-main", WorkspacePath: "/repo/main", Label: "Codex"},
	}

	got := dedupeSavedTabs(tabs)
	if len(got) != 1 {
		t.Fatalf("len(dedupeSavedTabs) = %d, want 1", len(got))
	}
	if got[0].TabType != "claude" {
		t.Fatalf("dedupe kept TabType %q, want first tab type claude", got[0].TabType)
	}
}

func TestDedupeSavedTabsKeepsCodexChatsByThread(t *testing.T) {
	tabs := []SavedTab{
		{TabType: "codex-chat", ThreadID: "thread-1", WorkspacePath: "/repo/main", Label: "Codex Chat"},
		{TabType: "codex-chat", ThreadID: "thread-2", WorkspacePath: "/repo/main", Label: "Codex Chat"},
	}

	got := dedupeSavedTabs(tabs)
	if len(got) != 2 {
		t.Fatalf("len(dedupeSavedTabs) = %d, want 2", len(got))
	}
}
