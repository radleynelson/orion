package tmuxutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestWrapInitialCommandDoesNotUsePaneStdin(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	tmp := t.TempDir()
	zdotdir := filepath.Join(tmp, "zdot")
	if err := os.MkdirAll(zdotdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zdotdir, ".zshrc"), []byte("read -k 1 answer\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmp, "out.txt")
	session := "orion-wrap-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	})

	command := "printf %s " + shellQuote("source") + " > " + shellQuote(outPath)
	wrapped := WrapInitialCommand(command)
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", session, "-e", "ZDOTDIR="+zdotdir, "-c", tmp, wrapped).CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session failed: %v %s", err, string(out))
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(outPath)
		if err == nil {
			if string(data) != "source" {
				t.Fatalf("expected full command output, got %q", string(data))
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("wrapped command did not run")
}
