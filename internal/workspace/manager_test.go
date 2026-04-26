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
