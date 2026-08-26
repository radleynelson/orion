package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orion/internal/config"
	"orion/internal/port"
)

func TestAllocatePortsDoesNotFallBackToMainRedisDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoRoot := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "worktree")
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		t.Fatal(err)
	}
	configText := "[servers.backend]\ncommand = \"true\"\ndefault_port = 3000\nport_env = \"PORT\"\n"
	if err := os.WriteFile(filepath.Join(repoRoot, config.FileName), []byte(configText), 0644); err != nil {
		t.Fatal(err)
	}

	registry := port.NewRegistry()
	for db := 2; db <= 15; db++ {
		if _, err := registry.AllocateRedisDB(fmt.Sprintf("workspace-%d", db)); err != nil {
			t.Fatal(err)
		}
	}
	manager := NewManager(registry)

	err := manager.AllocatePorts(repoRoot, workspacePath, false)
	if err == nil || !strings.Contains(err.Error(), "all Redis DBs (2-15) are in use") {
		t.Fatalf("AllocatePorts error = %v, want Redis pool exhaustion", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(workspacePath, ".orion", "env.sh")); readErr == nil && strings.Contains(string(data), "REDIS_DB=1") {
		t.Fatalf("AllocatePorts wrote unsafe main Redis DB fallback:\n%s", data)
	}
}
