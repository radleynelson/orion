package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveExecutablePrefersWorkDirNodeModulesBin(t *testing.T) {
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(binDir, "fake-lsp")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveExecutable("fake-lsp", workDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveExecutable() = %q, want %q", got, want)
	}
}

func TestStartServerUsesExplicitExecutableArgsAndWorkDir(t *testing.T) {
	tmp := t.TempDir()
	workDir := filepath.Join(tmp, "workspace")
	if err := os.Mkdir(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(tmp, "fake-lsp")
	cwdFile := filepath.Join(tmp, "cwd.txt")
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd > \"$1\"\nwhile IFS= read -r line; do :; done\n"), 0755); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager()
	err := mgr.StartServer(ServerConfig{
		Language:   "test",
		Executable: script,
		Args:       []string{cwdFile},
		WorkDir:    workDir,
		RootURI:    "file://" + workDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.StopServer("test")

	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(cwdFile)
		if err == nil {
			gotPath := strings.TrimSpace(string(data))
			gotResolved, err := filepath.EvalSymlinks(gotPath)
			if err != nil {
				t.Fatal(err)
			}
			wantResolved, err := filepath.EvalSymlinks(workDir)
			if err != nil {
				t.Fatal(err)
			}
			if gotResolved != wantResolved {
				t.Fatalf("cwd = %q, want %q", gotResolved, wantResolved)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for cwd file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
