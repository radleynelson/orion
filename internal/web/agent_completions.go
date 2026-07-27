package web

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type agentCompletion struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"`
	Provider    string `json:"provider"`
	Scope       string `json:"scope"`
	Source      string `json:"source,omitempty"`
	InsertText  string `json:"insertText"`
}

func (s *Server) handleAgentCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))
	workspacePath := strings.TrimSpace(r.URL.Query().Get("workspace"))
	items := listAgentCompletions(provider, workspacePath)
	writeJSON(w, items)
}

func listAgentCompletions(provider string, workspacePath string) []agentCompletion {
	home, _ := os.UserHomeDir()
	var items []agentCompletion

	includeCodex := provider == "" || provider == "codex" || provider == "codex-chat"
	includeClaude := provider == "" || provider == "claude" || provider == "claude-chat"

	if workspacePath != "" {
		if includeCodex {
			items = append(items, readSkillRoots("codex", "workspace", filepath.Join(workspacePath, ".codex", "skills"))...)
		}
		if includeClaude {
			items = append(items, readSkillRoots("claude", "workspace", filepath.Join(workspacePath, ".claude", "skills"))...)
			items = append(items, readCommandDir("claude", "workspace", filepath.Join(workspacePath, ".claude", "commands"))...)
		}
	}

	if home != "" {
		if includeCodex {
			items = append(items, readSkillRoots("codex", "global", filepath.Join(home, ".codex", "skills"))...)
			items = append(items, readSkillRoots("codex", "system", filepath.Join(home, ".codex", "skills", ".system"))...)
			items = append(items, readPluginSkills("codex", filepath.Join(home, ".codex", "plugins", "cache"))...)
		}
		if includeClaude {
			items = append(items, readSkillRoots("claude", "global", filepath.Join(home, ".claude", "skills"))...)
			items = append(items, readCommandDir("claude", "global", filepath.Join(home, ".claude", "commands"))...)
			items = append(items, readPluginSkills("claude", filepath.Join(home, ".claude", "plugins"))...)
		}
	}

	return sortedUniqueCompletions(items)
}

func readSkillRoots(provider string, scope string, root string) []agentCompletion {
	var items []agentCompletion
	entries, err := os.ReadDir(root)
	if err != nil {
		return items
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		if item, ok := readSkillFile(provider, scope, path); ok {
			items = append(items, item)
		}
	}
	return items
}

func readPluginSkills(provider string, root string) []agentCompletion {
	var items []agentCompletion
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		if !strings.Contains(path, string(filepath.Separator)+"skills"+string(filepath.Separator)) {
			return nil
		}
		if item, ok := readSkillFile(provider, "plugin", path); ok {
			items = append(items, item)
		}
		return nil
	})
	return items
}

func readSkillFile(provider string, scope string, path string) (agentCompletion, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return agentCompletion{}, false
	}
	meta, body := parseMarkdownFrontmatter(string(data))
	rawName := firstNonEmpty(meta["name"], filepath.Base(filepath.Dir(path)))
	title := skillTitle(rawName)
	description := firstNonEmpty(meta["description"], firstMarkdownParagraph(body))
	return agentCompletion{
		ID:          completionID(provider, "skill", scope, rawName, path),
		Title:       title,
		Description: compactCompletionDescription(description),
		Kind:        "skill",
		Provider:    provider,
		Scope:       scope,
		Source:      completionSource(path),
		InsertText:  "$" + title,
	}, true
}

func readCommandDir(provider string, scope string, root string) []agentCompletion {
	var items []agentCompletion
	entries, err := os.ReadDir(root)
	if err != nil {
		return items
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		meta, body := parseMarkdownFrontmatter(string(data))
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		title := skillTitle(firstNonEmpty(meta["name"], name))
		description := firstNonEmpty(meta["description"], firstMarkdownParagraph(body))
		items = append(items, agentCompletion{
			ID:          completionID(provider, "slash-command", scope, name, path),
			Title:       title,
			Description: compactCompletionDescription(description),
			Kind:        "slash-command",
			Provider:    provider,
			Scope:       scope,
			Source:      completionSource(path),
			InsertText:  "/" + name,
		})
	}
	return items
}

func sortedUniqueCompletions(items []agentCompletion) []agentCompletion {
	scopeRank := map[string]int{"workspace": 0, "global": 1, "system": 2, "plugin": 3}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Provider != items[j].Provider {
			return items[i].Provider < items[j].Provider
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if scopeRank[items[i].Scope] != scopeRank[items[j].Scope] {
			return scopeRank[items[i].Scope] < scopeRank[items[j].Scope]
		}
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})

	seen := map[string]bool{}
	out := make([]agentCompletion, 0, len(items))
	for _, item := range items {
		key := item.Provider + "|" + item.Kind + "|" + strings.ToLower(item.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func parseMarkdownFrontmatter(text string) (map[string]string, string) {
	meta := map[string]string{}
	trimmed := strings.TrimPrefix(text, "\ufeff")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return meta, text
	}
	lines := strings.Split(trimmed, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
		line := strings.TrimSpace(lines[i])
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if end == -1 {
		return meta, text
	}
	return meta, strings.Join(lines[end+1:], "\n")
}

func firstMarkdownParagraph(body string) string {
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "#") || strings.HasPrefix(block, "```") {
			continue
		}
		return strings.Join(strings.Fields(block), " ")
	}
	return ""
}

func compactCompletionDescription(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 180 {
		return value
	}
	return strings.TrimSpace(value[:177]) + "..."
}

func skillTitle(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, "$")
	if value == "" {
		return "Skill"
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '/'
	})
	for i, part := range parts {
		lower := strings.ToLower(part)
		switch lower {
		case "ios":
			parts[i] = "iOS"
		case "macos":
			parts[i] = "macOS"
		case "api":
			parts[i] = "API"
		case "cli":
			parts[i] = "CLI"
		case "mcp":
			parts[i] = "MCP"
		default:
			if len(part) > 1 {
				parts[i] = strings.ToUpper(part[:1]) + part[1:]
			} else {
				parts[i] = strings.ToUpper(part)
			}
		}
	}
	return strings.Join(parts, " ")
}

func completionID(provider string, kind string, scope string, name string, path string) string {
	return provider + ":" + kind + ":" + scope + ":" + firstNonEmpty(strings.TrimSpace(name), filepath.Base(filepath.Dir(path)))
}

func completionSource(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if rel, relErr := filepath.Rel(home, path); relErr == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	return path
}
