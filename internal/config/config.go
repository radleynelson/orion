package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// OrionConfig represents the per-repo .orion.toml configuration.
type OrionConfig struct {
	BranchPrefix string                  `toml:"branch_prefix"`
	WorktreesDir string                  `toml:"worktrees_dir"`
	Credentials  CredentialsConfig       `toml:"credentials"`
	Servers      map[string]ServerConfig `toml:"servers"`
	Agents       map[string]AgentConfig  `toml:"agents"`
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
	Label             string `toml:"label"`
	Provider          string `toml:"provider"`
	Command           string `toml:"command"`
	Model             string `toml:"model"`
	ReasoningEffort   string `toml:"reasoning_effort"`
	ApprovalPolicy    string `toml:"approval_policy"`
	SandboxMode       string `toml:"sandbox_mode"`
	PermissionMode    string `toml:"permission_mode"`
	CollaborationMode string `toml:"collaboration_mode"`
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
		Agents: defaultAgents(),
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
		cfg.Agents = defaultAgents()
	} else {
		normalizeAgents(cfg.Agents)
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
		Agents:      defaultAgents(),
	}, nil
}

func defaultAgents() map[string]AgentConfig {
	return map[string]AgentConfig{
		"claude": DefaultAgent("claude"),
		"codex":  DefaultAgent("codex"),
	}
}

func DefaultAgent(provider string) AgentConfig {
	provider = strings.ToLower(strings.TrimSpace(provider))
	return normalizeAgent(provider, AgentConfig{Provider: provider})
}

func normalizeAgents(agents map[string]AgentConfig) {
	for name, agent := range agents {
		agents[name] = normalizeAgent(name, agent)
	}
}

func normalizeAgent(name string, agent AgentConfig) AgentConfig {
	provider := strings.ToLower(strings.TrimSpace(agent.Provider))
	if provider == "" {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "claude":
			provider = "claude"
		case "codex":
			provider = "codex"
		}
	}
	agent.Provider = provider
	if strings.TrimSpace(agent.Label) == "" {
		agent.Label = capitalize(name)
	}

	switch provider {
	case "claude":
		if strings.TrimSpace(agent.Command) == "" {
			agent.Command = "claude --dangerously-skip-permissions --effort xhigh --chrome"
		}
		if strings.TrimSpace(agent.Model) == "" {
			agent.Model = "claude-opus-4-7"
		}
		if strings.TrimSpace(agent.ReasoningEffort) == "" {
			agent.ReasoningEffort = "xhigh"
		}
		if strings.TrimSpace(agent.ApprovalPolicy) == "" {
			agent.ApprovalPolicy = "never"
		}
		if strings.TrimSpace(agent.SandboxMode) == "" {
			agent.SandboxMode = "danger-full-access"
		}
		if strings.TrimSpace(agent.PermissionMode) == "" {
			agent.PermissionMode = "bypassPermissions"
		}
	case "codex":
		if strings.TrimSpace(agent.Command) == "" {
			agent.Command = "codex --dangerously-bypass-approvals-and-sandbox"
		}
		if strings.TrimSpace(agent.Model) == "" {
			agent.Model = "gpt-5.4"
		}
		if strings.TrimSpace(agent.ReasoningEffort) == "" {
			agent.ReasoningEffort = "xhigh"
		}
		if strings.TrimSpace(agent.ApprovalPolicy) == "" {
			agent.ApprovalPolicy = "never"
		}
		if strings.TrimSpace(agent.SandboxMode) == "" {
			agent.SandboxMode = "danger-full-access"
		}
		if strings.TrimSpace(agent.CollaborationMode) == "" {
			agent.CollaborationMode = "default"
		}
	}
	return agent
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}
