package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTypeScriptLSPConfigPrefersFrontendLocalServer(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "frontend", "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(binDir, "typescript-language-server")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := defaultTypeScriptLSPConfig("typescript", workspace)
	if cfg.Executable != want {
		t.Fatalf("Executable = %q, want %q", cfg.Executable, want)
	}
	if cfg.WorkDir != workspace {
		t.Fatalf("WorkDir = %q, want %q", cfg.WorkDir, workspace)
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "--stdio" {
		t.Fatalf("Args = %#v, want [--stdio]", cfg.Args)
	}
}

func TestDefaultTypeScriptLSPConfigFallsBackToPathCommand(t *testing.T) {
	workspace := t.TempDir()

	cfg := defaultTypeScriptLSPConfig("typescript", workspace)
	if cfg.Command != "typescript-language-server --stdio" {
		t.Fatalf("Command = %q, want default command", cfg.Command)
	}
	if cfg.Executable != "" {
		t.Fatalf("Executable = %q, want empty", cfg.Executable)
	}
	if cfg.WorkDir != workspace {
		t.Fatalf("WorkDir = %q, want %q", cfg.WorkDir, workspace)
	}
}
