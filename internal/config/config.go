package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	FileName       = ".orion.toml"
	LegacyFileName = ".orion.config.toml"
)

// OrionConfig represents the per-repo .orion.toml configuration.
type OrionConfig struct {
	BranchPrefix string                  `toml:"branch_prefix"`
	WorktreesDir string                  `toml:"worktrees_dir"`
	Credentials  CredentialsConfig       `toml:"credentials"`
	Hooks        HooksConfig             `toml:"hooks"`
	Servers      map[string]ServerConfig `toml:"servers"`
	Agents       map[string]AgentConfig  `toml:"agents"`
}

type CredentialsConfig struct {
	Copy []string `toml:"copy"`
}

type HooksConfig struct {
	WorktreeCreated  HookConfig `toml:"worktree_created"`
	WorktreeDeleting HookConfig `toml:"worktree_deleting"`
}

type HookConfig struct {
	Command  string `toml:"command"`
	Blocking *bool  `toml:"blocking"`
}

func (h HookConfig) IsBlocking(defaultValue bool) bool {
	if h.Blocking == nil {
		return defaultValue
	}
	return *h.Blocking
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
	Icon              string `toml:"icon"`
	Command           string `toml:"command"`
	InitialPrompt     string `toml:"initial_prompt"`
	Model             string `toml:"model"`
	ReasoningEffort   string `toml:"reasoning_effort"`
	ApprovalPolicy    string `toml:"approval_policy"`
	SandboxMode       string `toml:"sandbox_mode"`
	PermissionMode    string `toml:"permission_mode"`
	CollaborationMode string `toml:"collaboration_mode"`
}

// Load reads .orion.toml from a repo root.
// Falls back to legacy .orion.config.toml and .radconfig for backward compatibility.
func Load(repoRoot string) *OrionConfig {
	// Try .orion.toml first
	tomlPath := filepath.Join(repoRoot, FileName)
	if cfg, err := loadTOML(tomlPath); err == nil {
		return cfg
	}

	// Fall back to the old config name if a repo already has one.
	legacyPath := filepath.Join(repoRoot, LegacyFileName)
	if cfg, err := loadTOML(legacyPath); err == nil {
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

func HasProjectConfig(repoRoot string) bool {
	for _, name := range []string{FileName, LegacyFileName} {
		if _, err := os.Stat(filepath.Join(repoRoot, name)); err == nil {
			return true
		}
	}
	return false
}

func EnsureDefaultFile(repoRoot string) (bool, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return false, fmt.Errorf("repo root required")
	}
	if HasProjectConfig(repoRoot) {
		return false, nil
	}
	credentials := defaultCredentialFiles()
	if cfg, err := loadRadConfig(filepath.Join(repoRoot, ".radconfig")); err == nil && len(cfg.Credentials.Copy) > 0 {
		credentials = cfg.Credentials.Copy
	}
	path := filepath.Join(repoRoot, FileName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()
	if _, err := file.WriteString(defaultFileContents(credentials)); err != nil {
		return false, err
	}
	return true, nil
}

func DefaultFileContents() string {
	return defaultFileContents(defaultCredentialFiles())
}

func defaultCredentialFiles() []string {
	return []string{".env", ".env.local", ".env.development", ".env.development.local"}
}

func defaultFileContents(credentials []string) string {
	var credentialLines []string
	for _, credential := range credentials {
		credential = strings.TrimSpace(credential)
		if credential == "" {
			continue
		}
		credentialLines = append(credentialLines, "  "+strconv.Quote(credential)+",")
	}
	if len(credentialLines) == 0 {
		credentialLines = []string{
			`  ".env",`,
			`  ".env.local",`,
			`  ".env.development",`,
			`  ".env.development.local",`,
		}
	}

	return strings.TrimSpace(`# Orion project config.
# Orion generated this file because the project did not have one yet.
# Model is intentionally omitted so Claude/Codex use their current default.

[credentials]
copy = [
`+strings.Join(credentialLines, "\n")+`
]

[agents.claude]
label = "Claude"
provider = "claude"
command = "claude --dangerously-skip-permissions"
permission_mode = "bypassPermissions"

[agents.codex]
label = "Codex"
provider = "codex"
command = "codex --dangerously-bypass-approvals-and-sandbox"
reasoning_effort = "xhigh"
approval_policy = "never"
sandbox_mode = "danger-full-access"
collaboration_mode = "default"

# Optional custom role icon for non-default agents:
# icon = "reviewer" # reviewer, scribe, plan, test, debug, deploy, ops, data, design, security, browser, automate, branch, docs, clean
`) + "\n"
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
			agent.Command = "claude --dangerously-skip-permissions"
		}
		if strings.TrimSpace(agent.PermissionMode) == "" {
			agent.PermissionMode = "bypassPermissions"
		}
	case "codex":
		if strings.TrimSpace(agent.Command) == "" {
			agent.Command = "codex --dangerously-bypass-approvals-and-sandbox"
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
