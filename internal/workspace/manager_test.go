package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orion/internal/config"
)

func TestGetProjectInfoCreatesDefaultOrionConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, string(out))
	}

	info, err := NewManager().GetProjectInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(info.Root)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != wantRoot {
		t.Fatalf("Root = %q, want %q", root, wantRoot)
	}

	data, err := os.ReadFile(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `[agents.claude]`) || !strings.Contains(text, `[agents.codex]`) {
		t.Fatalf("generated config missing default agents:\n%s", text)
	}
	if strings.Contains(text, "model =") {
		t.Fatalf("generated config should not pin models:\n%s", text)
	}
}

func TestThreadIDForResumeCommand(t *testing.T) {
	tests := []struct {
		name        string
		sessionType string
		command     string
		want        string
	}{
		{
			name:        "claude resume flag",
			sessionType: "claude",
			command:     "claude --dangerously-skip-permissions --resume '204b42f3-4200-4e5a-8079-0878793d6136'",
			want:        "204b42f3-4200-4e5a-8079-0878793d6136",
		},
		{
			name:        "claude session id flag",
			sessionType: "claude",
			command:     "claude --session-id=e4f52ec0-3da4-401c-b99c-aa6f4d221d64",
			want:        "e4f52ec0-3da4-401c-b99c-aa6f4d221d64",
		},
		{
			name:        "codex resume positional",
			sessionType: "codex",
			command:     "codex resume --dangerously-bypass-approvals-and-sandbox --no-alt-screen -m gpt-5.5 -c 'model_reasoning_effort=\"xhigh\"' '019dc827-72fb-7100-9770-33b63986e2ea'",
			want:        "019dc827-72fb-7100-9770-33b63986e2ea",
		},
		{
			name:        "codex resume equals",
			sessionType: "codex",
			command:     "codex --resume=abc123",
			want:        "abc123",
		},
		{
			name:        "shell ignores resume-looking command",
			sessionType: "shell",
			command:     "echo --resume abc123",
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := threadIDForResumeCommand(tt.sessionType, tt.command); got != tt.want {
				t.Fatalf("threadIDForResumeCommand = %q, want %q", got, tt.want)
			}
		})
	}
}
