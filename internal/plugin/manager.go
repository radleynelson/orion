package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orion/internal/config"
)

// FormatResult contains the result of formatting a file.
type FormatResult struct {
	Formatted bool   `json:"formatted"`
	Content   string `json:"content"`
	Error     string `json:"error,omitempty"`
}

// LintResult contains the result of linting a file.
type LintResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Manager handles plugin operations (formatting, linting, on-save hooks).
type Manager struct {
	ctx context.Context
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// FormatFile runs the configured formatter for the given file.
// If stdin mode, passes content via stdin and returns stdout.
// Otherwise, runs the command with the file path and re-reads the file.
func (m *Manager) FormatFile(repoRoot string, filePath string, content string) (*FormatResult, error) {
	cfg := config.Load(repoRoot)
	ext := filepath.Ext(filePath)

	// Check explicit formatters first
	for _, formatter := range cfg.Plugins.Formatters {
		if matchesExtension(ext, formatter.Extensions) {
			return m.runFormatter(formatter, filePath, content)
		}
	}

	// Check on-save actions with action=format
	for _, action := range cfg.Plugins.OnSave {
		if action.Action == "format" && matchesExtension(ext, action.Extensions) {
			return m.runFormatter(config.FormatterConfig{
				Command:    action.Command,
				Extensions: action.Extensions,
				Stdin:      true,
			}, filePath, content)
		}
	}

	// Built-in formatters for common languages
	return m.runBuiltinFormatter(filePath, content)
}

// RunOnSave executes on-save hooks for the given file.
func (m *Manager) RunOnSave(repoRoot string, filePath string) ([]string, error) {
	cfg := config.Load(repoRoot)
	ext := filepath.Ext(filePath)

	var outputs []string
	for _, action := range cfg.Plugins.OnSave {
		if action.Action == "run" && matchesExtension(ext, action.Extensions) {
			output, err := m.runCommand(action.Command, filepath.Dir(filePath), filePath, "")
			if err != nil {
				outputs = append(outputs, fmt.Sprintf("error: %s: %v", action.Command, err))
			} else if output != "" {
				outputs = append(outputs, output)
			}
		}
	}

	return outputs, nil
}

// LintFile runs the configured linter for the given file.
func (m *Manager) LintFile(repoRoot string, filePath string) (*LintResult, error) {
	cfg := config.Load(repoRoot)
	ext := filepath.Ext(filePath)

	for _, linter := range cfg.Plugins.Linters {
		if matchesExtension(ext, linter.Extensions) {
			output, err := m.runCommand(linter.Command, filepath.Dir(filePath), filePath, "")
			result := &LintResult{Output: output}
			if err != nil {
				result.Error = err.Error()
			}
			return result, nil
		}
	}

	return &LintResult{}, nil
}

// GetFormatOnSaveExtensions returns file extensions that have formatters configured.
func (m *Manager) GetFormatOnSaveExtensions(repoRoot string) []string {
	cfg := config.Load(repoRoot)
	extSet := make(map[string]bool)

	for _, formatter := range cfg.Plugins.Formatters {
		for _, ext := range formatter.Extensions {
			extSet[ext] = true
		}
	}
	for _, action := range cfg.Plugins.OnSave {
		if action.Action == "format" {
			for _, ext := range action.Extensions {
				extSet[ext] = true
			}
		}
	}

	// Built-in formatters
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".go", ".rb", ".css", ".scss", ".json", ".html"} {
		extSet[ext] = true
	}

	var exts []string
	for ext := range extSet {
		exts = append(exts, ext)
	}
	return exts
}

func (m *Manager) runFormatter(formatter config.FormatterConfig, filePath string, content string) (*FormatResult, error) {
	if formatter.Stdin {
		output, err := m.runCommand(formatter.Command, filepath.Dir(filePath), filePath, content)
		if err != nil {
			return &FormatResult{Formatted: false, Error: err.Error()}, nil
		}
		if output == content {
			return &FormatResult{Formatted: false, Content: content}, nil
		}
		return &FormatResult{Formatted: true, Content: output}, nil
	}

	// Non-stdin mode: write file, run formatter, re-read
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return nil, err
	}

	_, err := m.runCommand(formatter.Command, filepath.Dir(filePath), filePath, "")
	if err != nil {
		return &FormatResult{Formatted: false, Error: err.Error()}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	formatted := string(data)
	return &FormatResult{Formatted: formatted != content, Content: formatted}, nil
}

func (m *Manager) runBuiltinFormatter(filePath string, content string) (*FormatResult, error) {
	ext := filepath.Ext(filePath)
	dir := filepath.Dir(filePath)

	var cmd string
	var stdin bool

	switch ext {
	case ".ts", ".tsx", ".js", ".jsx":
		// Try prettier, then eslint --fix, then deno fmt
		if _, err := exec.LookPath("prettier"); err == nil {
			cmd = "prettier --stdin-filepath " + shellQuote(filePath)
			stdin = true
		} else if _, err := exec.LookPath("deno"); err == nil {
			cmd = "deno fmt --ext " + strings.TrimPrefix(ext, ".") + " -"
			stdin = true
		}
	case ".go":
		if _, err := exec.LookPath("gofmt"); err == nil {
			cmd = "gofmt"
			stdin = true
		}
	case ".rb":
		if _, err := exec.LookPath("rubocop"); err == nil {
			cmd = "rubocop -a --stdin " + shellQuote(filePath)
			stdin = true
		}
	case ".css", ".scss", ".less":
		if _, err := exec.LookPath("prettier"); err == nil {
			cmd = "prettier --stdin-filepath " + shellQuote(filePath)
			stdin = true
		}
	case ".json":
		if _, err := exec.LookPath("prettier"); err == nil {
			cmd = "prettier --stdin-filepath " + shellQuote(filePath)
			stdin = true
		}
	case ".html", ".htm":
		if _, err := exec.LookPath("prettier"); err == nil {
			cmd = "prettier --stdin-filepath " + shellQuote(filePath)
			stdin = true
		}
	}

	if cmd == "" {
		return &FormatResult{Formatted: false, Content: content}, nil
	}

	if stdin {
		output, err := m.runCommand(cmd, dir, filePath, content)
		if err != nil {
			return &FormatResult{Formatted: false, Content: content, Error: err.Error()}, nil
		}
		// Some formatters (rubocop) add inspection output — strip it
		if ext == ".rb" && strings.Contains(output, "====================") {
			parts := strings.SplitN(output, "====================\n", 2)
			if len(parts) == 2 {
				output = parts[1]
			}
		}
		if output == content {
			return &FormatResult{Formatted: false, Content: content}, nil
		}
		return &FormatResult{Formatted: true, Content: output}, nil
	}

	return &FormatResult{Formatted: false, Content: content}, nil
}

func (m *Manager) runCommand(command string, dir string, filePath string, stdinContent string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Replace {{file}} placeholder with actual file path
	expanded := strings.ReplaceAll(command, "{{file}}", filePath)

	cmd := exec.CommandContext(ctx, "sh", "-c", expanded)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"ORION_FILE="+filePath,
		"ORION_DIR="+dir,
	)

	if stdinContent != "" {
		cmd.Stdin = strings.NewReader(stdinContent)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func matchesExtension(ext string, extensions []string) bool {
	ext = strings.ToLower(ext)
	for _, e := range extensions {
		if strings.ToLower(e) == ext || strings.ToLower("."+e) == ext {
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
