package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDefaultFileCreatesOrionTomlWithoutModels(t *testing.T) {
	dir := t.TempDir()

	created, err := EnsureDefaultFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}

	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "model =") {
		t.Fatalf("generated config should not hard-code model fields:\n%s", text)
	}
	if !strings.Contains(text, `provider = "claude"`) || !strings.Contains(text, `provider = "codex"`) {
		t.Fatalf("generated config missing default Claude/Codex agents:\n%s", text)
	}

	cfg := Load(dir)
	if got := strings.TrimSpace(cfg.Agents["claude"].Model); got != "" {
		t.Fatalf("Claude model = %q, want empty", got)
	}
	if got := strings.TrimSpace(cfg.Agents["claude"].Command); got != "claude --dangerously-skip-permissions" {
		t.Fatalf("Claude command = %q, want minimal full-permission command", got)
	}
	if got := strings.TrimSpace(cfg.Agents["claude"].ReasoningEffort); got != "" {
		t.Fatalf("Claude reasoning effort = %q, want empty", got)
	}
	if got := strings.TrimSpace(cfg.Agents["claude"].SandboxMode); got != "" {
		t.Fatalf("Claude sandbox mode = %q, want empty", got)
	}
	if got := strings.TrimSpace(cfg.Agents["codex"].Model); got != "" {
		t.Fatalf("Codex model = %q, want empty", got)
	}
}

func TestEnsureDefaultFileDoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, FileName)
	if err := os.WriteFile(existing, []byte("[agents.custom]\ncommand = \"echo custom\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureDefaultFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatalf("created = true, want false")
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "dangerously") {
		t.Fatalf("existing config was overwritten:\n%s", string(data))
	}
}

func TestLoadReadsAgentIcon(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("[agents.review]\nprovider = \"claude\"\nicon = \"reviewer\"\ncommand = \"claude --dangerously-skip-permissions\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load(dir)
	if got := cfg.Agents["review"].Icon; got != "reviewer" {
		t.Fatalf("agent icon = %q, want reviewer", got)
	}
	if got := cfg.Agents["review"].Provider; got != "claude" {
		t.Fatalf("agent provider = %q, want claude", got)
	}
}

func TestEnsureDefaultFileSeedsCredentialsFromRadconfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".radconfig"), []byte("backend/.env\nfrontend/.env.local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureDefaultFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}

	cfg := Load(dir)
	want := []string{"backend/.env", "frontend/.env.local"}
	if len(cfg.Credentials.Copy) != len(want) {
		t.Fatalf("credentials = %#v, want %#v", cfg.Credentials.Copy, want)
	}
	for i := range want {
		if cfg.Credentials.Copy[i] != want[i] {
			t.Fatalf("credentials = %#v, want %#v", cfg.Credentials.Copy, want)
		}
	}
}

func TestLoadReadsLegacyConfigName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LegacyFileName), []byte("[agents.codex]\nprovider = \"codex\"\ncommand = \"codex custom\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load(dir)
	if got := cfg.Agents["codex"].Command; got != "codex custom" {
		t.Fatalf("legacy codex command = %q, want codex custom", got)
	}
}
