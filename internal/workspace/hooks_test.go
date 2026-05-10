package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orion/internal/config"
)

func TestRenderHookCommandShellQuotesBranchNames(t *testing.T) {
	hookCtx := hookContext{
		Name:             "feature",
		Branch:           "radley/feature's-fix",
		BaseRef:          "main",
		WorkspacePath:    "/tmp/slant-feature",
		RepoRoot:         "/tmp/slant",
		MainWorktreePath: "/tmp/slant",
	}

	got := renderHookCommand("bin/orion-neon-db provision --branch {{branch}} --path {{workspace_path}}", hookCtx)
	if !strings.Contains(got, "--branch 'radley/feature'\"'\"'s-fix'") {
		t.Fatalf("rendered command did not shell-quote branch:\n%s", got)
	}
	if !strings.Contains(got, "--path /tmp/slant-feature") {
		t.Fatalf("rendered command did not shell-quote path:\n%s", got)
	}
}

func TestHookEnvOverridesOrionValues(t *testing.T) {
	env := hookEnv([]string{"PATH=/bin", "ORION_BRANCH=old"}, hookContext{
		Name:             "feature",
		Branch:           "radley/feature",
		BaseRef:          "main",
		WorkspacePath:    "/tmp/slant-feature",
		RepoRoot:         "/tmp/slant",
		MainWorktreePath: "/tmp/slant",
	})

	joined := "\n" + strings.Join(env, "\n") + "\n"
	if strings.Contains(joined, "\nORION_BRANCH=old\n") {
		t.Fatalf("old ORION_BRANCH was not removed: %#v", env)
	}
	if !strings.Contains(joined, "\nORION_BRANCH=radley/feature\n") {
		t.Fatalf("new ORION_BRANCH missing: %#v", env)
	}
	if !strings.Contains(joined, "\nORION_WORKSPACE_PATH=/tmp/slant-feature\n") {
		t.Fatalf("ORION_WORKSPACE_PATH missing: %#v", env)
	}
}

func TestCreateWorkspaceRunsHookAfterCredentialsCopy(t *testing.T) {
	repo := initHookTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "credential.txt"), []byte("secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := `branch_prefix = "radley"

[credentials]
copy = ["credential.txt"]

[hooks.worktree_created]
command = "test -f credential.txt && mkdir -p .orion && echo \"$ORION_BRANCH\" > .orion/hook-ran"
`
	if err := os.WriteFile(filepath.Join(repo, config.FileName), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := NewManager().CreateWorkspaceFrom(repo, "feature", "")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(ws.Path, ".orion", "hook-ran"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "radley/feature" {
		t.Fatalf("hook branch = %q, want radley/feature", got)
	}
	if _, err := os.Stat(filepath.Join(ws.Path, "credential.txt")); err != nil {
		t.Fatalf("credential was not copied before hook: %v", err)
	}
}

func TestCreateWorkspaceBlockingHookFailureReturnsErrorAndLeavesWorktree(t *testing.T) {
	repo := initHookTestRepo(t)
	cfg := `[hooks.worktree_created]
command = "echo setup failed >&2; exit 7"
`
	if err := os.WriteFile(filepath.Join(repo, config.FileName), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := NewManager().CreateWorkspaceFrom(repo, "bad-hook", "")
	if err == nil {
		t.Fatal("CreateWorkspaceFrom succeeded, want hook failure")
	}
	if ws != nil {
		t.Fatalf("workspace = %#v, want nil on failure", ws)
	}
	if !strings.Contains(err.Error(), "exit status 7") || !strings.Contains(err.Error(), "setup failed") {
		t.Fatalf("error missing hook details:\n%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-bad-hook")); statErr != nil {
		t.Fatalf("failed hook should leave worktree for inspection: %v", statErr)
	}
}

func TestDeleteWorkspaceRunsHookBeforeRemovingWorktree(t *testing.T) {
	repo := initHookTestRepo(t)
	cfg := `[hooks.worktree_deleting]
command = "test -d \"$ORION_WORKSPACE_PATH\" && echo \"$ORION_BRANCH\" > \"$ORION_MAIN_WORKTREE_PATH/delete-hook-ran\""
blocking = true
`
	if err := os.WriteFile(filepath.Join(repo, config.FileName), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager()
	ws, err := mgr.CreateWorkspaceFrom(repo, "delete-me", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace(repo, ws.Path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(repo, "delete-hook-ran"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "delete-me" {
		t.Fatalf("delete hook branch = %q, want delete-me", got)
	}
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after delete: %v", err)
	}
}

func TestNonBlockingDeleteHookContinuesDeletion(t *testing.T) {
	repo := initHookTestRepo(t)
	cfg := `[hooks.worktree_deleting]
command = "echo cleanup failed >&2; exit 9"
blocking = false
`
	if err := os.WriteFile(filepath.Join(repo, config.FileName), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager()
	ws, err := mgr.CreateWorkspaceFrom(repo, "delete-anyway", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteWorkspace(repo, ws.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after nonblocking hook failure: %v", err)
	}
}

func initHookTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, string(out))
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=Orion Test", "-c", "user.email=orion@example.test", "commit", "-m", "initial")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
