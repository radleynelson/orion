package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// OrionConfig represents the per-repo .orion.toml configuration.
type OrionConfig struct {
	Credentials CredentialsConfig          `toml:"credentials"`
	Servers     map[string]ServerConfig    `toml:"servers"`
	Agents      map[string]AgentConfig     `toml:"agents"`
}

type CredentialsConfig struct {
	Copy []string `toml:"copy"`
}

type ServerConfig struct {
	Command     string            `toml:"command"`
	Dir         string            `toml:"dir"`
	DefaultPort int               `toml:"default_port"`
	PortEnv     string            `toml:"port_env"`
	Env         map[string]string `toml:"env"`
}

type AgentConfig struct {
	Command string `toml:"command"`
}

// Load reads .orion.toml from a repo root.
// Falls back to .radconfig for backward compatibility.
func Load(repoRoot string) *OrionConfig {
	// Try .orion.toml first
	tomlPath := filepath.Join(repoRoot, ".orion.toml")
	if cfg, err := loadTOML(tomlPath); err == nil {
		return cfg
	}

	// Fall back to .radconfig
	radconfigPath := filepath.Join(repoRoot, ".radconfig")
	if cfg, err := loadRadConfig(radconfigPath); err == nil {
		return cfg
	}

	// Default config
	return &OrionConfig{
		Credentials: CredentialsConfig{
			Copy: []string{".env", ".env.local", ".env.development", ".env.development.local"},
		},
		Agents: map[string]AgentConfig{
			"claude": {Command: "claude --dangerously-skip-permissions"},
			"codex":  {Command: "codex --dangerously-bypass-approvals-and-sandbox"},
		},
	}
}

func loadTOML(path string) (*OrionConfig, error) {
	var cfg OrionConfig
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}

	// Set default agents if not specified
	if cfg.Agents == nil {
		cfg.Agents = map[string]AgentConfig{
			"claude": {Command: "claude --dangerously-skip-permissions"},
			"codex":  {Command: "codex --dangerously-bypass-approvals-and-sandbox"},
		}
	}

	return &cfg, nil
}

func loadRadConfig(path string) (*OrionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		files = append(files, line)
	}

	return &OrionConfig{
		Credentials: CredentialsConfig{Copy: files},
		Agents: map[string]AgentConfig{
			"claude": {Command: "claude --dangerously-skip-permissions"},
			"codex":  {Command: "codex --dangerously-bypass-approvals-and-sandbox"},
		},
	}, nil
}
