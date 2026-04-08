package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedFile represents a file with uncommitted changes.
type ChangedFile struct {
	Path       string `json:"path"`
	Status     string `json:"status"`     // "M", "A", "D", "R", "?"
	StatusText string `json:"statusText"` // "modified", "added", "deleted", "renamed", "untracked"
}

// FileDiff contains the original and modified content for Monaco DiffEditor.
type FileDiff struct {
	OriginalContent string `json:"originalContent"`
	ModifiedContent string `json:"modifiedContent"`
	Language        string `json:"language"`
}

// Manager handles git operations.
type Manager struct {
	ctx context.Context
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// GetChangedFiles returns all files with uncommitted changes in the workspace.
func (m *Manager) GetChangedFiles(workspacePath string) ([]ChangedFile, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = workspacePath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	var files []ChangedFile
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}

		// Porcelain v1 format: XY filename
		// X = index status, Y = working tree status
		xy := line[:2]
		path := strings.TrimSpace(line[3:])

		// Handle renamed files (format: "R  old -> new")
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			path = parts[1]
		}

		status, statusText := parseStatus(xy)
		files = append(files, ChangedFile{
			Path:       path,
			Status:     status,
			StatusText: statusText,
		})
	}

	return files, nil
}

// GetChangedFilesAgainst returns changed files in the workspace, comparing
// either uncommitted changes (base == "") or HEAD vs a base ref like "main".
func (m *Manager) GetChangedFilesAgainst(workspacePath string, base string) ([]ChangedFile, error) {
	if base == "" {
		return m.GetChangedFiles(workspacePath)
	}

	// Find merge base so we only show changes introduced on this branch.
	mb, err := exec.Command("git", "-C", workspacePath, "merge-base", "HEAD", base).Output()
	if err != nil {
		return nil, fmt.Errorf("git merge-base failed: %w", err)
	}
	mergeBase := strings.TrimSpace(string(mb))

	// git diff --name-status <merge-base> -- (committed changes vs base)
	cmd := exec.Command("git", "-C", workspacePath, "diff", "--name-status", mergeBase)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status failed: %w", err)
	}

	seen := make(map[string]bool)
	var files []ChangedFile
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		statusCode := parts[0]
		path := parts[len(parts)-1]
		if seen[path] {
			continue
		}
		seen[path] = true

		var status, statusText string
		switch statusCode[0] {
		case 'M':
			status, statusText = "M", "modified"
		case 'A':
			status, statusText = "A", "added"
		case 'D':
			status, statusText = "D", "deleted"
		case 'R':
			status, statusText = "R", "renamed"
		default:
			status, statusText = string(statusCode[0]), "changed"
		}
		files = append(files, ChangedFile{Path: path, Status: status, StatusText: statusText})
	}

	// Layer uncommitted changes on top so the review reflects the working tree.
	uncommitted, err := m.GetChangedFiles(workspacePath)
	if err == nil {
		for _, f := range uncommitted {
			if seen[f.Path] {
				continue
			}
			seen[f.Path] = true
			files = append(files, f)
		}
	}

	return files, nil
}

// GetUnifiedDiff returns the unified diff text for a single file. base == ""
// means working tree vs HEAD; otherwise it's working tree vs <base>.
func (m *Manager) GetUnifiedDiff(workspacePath string, base string, filePath string) (string, error) {
	relPath := filePath
	if filepath.IsAbs(filePath) {
		if rel, err := filepath.Rel(workspacePath, filePath); err == nil {
			relPath = rel
		}
	}

	args := []string{"-C", workspacePath, "diff", "--no-color", "--unified=3"}
	if base != "" {
		mb, err := exec.Command("git", "-C", workspacePath, "merge-base", "HEAD", base).Output()
		if err == nil {
			args = append(args, strings.TrimSpace(string(mb)))
		} else {
			args = append(args, base)
		}
	}
	args = append(args, "--", relPath)

	out, err := exec.Command("git", args...).Output()
	if err == nil && len(out) > 0 {
		return string(out), nil
	}

	// Fallback for untracked files: synthesize a diff against /dev/null.
	fullPath := filepath.Join(workspacePath, relPath)
	if data, rerr := os.ReadFile(fullPath); rerr == nil {
		out2, _ := exec.Command("git", "-C", workspacePath, "diff", "--no-color", "--no-index", "--unified=3", "/dev/null", relPath).Output()
		if len(out2) > 0 {
			return string(out2), nil
		}
		// Last resort: build a minimal +line diff manually
		lines := strings.Split(string(data), "\n")
		var b strings.Builder
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", relPath, relPath)
		fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", relPath)
		fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, l := range lines {
			b.WriteString("+" + l + "\n")
		}
		return b.String(), nil
	}

	return "", nil
}

// GetFileDiff returns the original (HEAD) and current content for a file.
// These feed directly into Monaco's DiffEditor.
func (m *Manager) GetFileDiff(workspacePath string, filePath string) (*FileDiff, error) {
	// Get the relative path from workspace root
	relPath := filePath
	if filepath.IsAbs(filePath) {
		rel, err := filepath.Rel(workspacePath, filePath)
		if err != nil {
			relPath = filePath
		} else {
			relPath = rel
		}
	}

	// Get original content from HEAD
	cmd := exec.Command("git", "show", "HEAD:"+relPath)
	cmd.Dir = workspacePath
	originalOut, err := cmd.Output()
	originalContent := ""
	if err == nil {
		originalContent = string(originalOut)
	}
	// If error (e.g., new file), originalContent stays empty

	// Get current working tree content
	fullPath := filepath.Join(workspacePath, relPath)
	modifiedData, err := os.ReadFile(fullPath)
	modifiedContent := ""
	if err == nil {
		modifiedContent = string(modifiedData)
	}
	// If error (e.g., deleted file), modifiedContent stays empty

	language := detectLanguage(relPath)

	return &FileDiff{
		OriginalContent: originalContent,
		ModifiedContent: modifiedContent,
		Language:        language,
	}, nil
}

func parseStatus(xy string) (string, string) {
	// Check working tree status first (Y), then index status (X)
	y := xy[1]
	x := xy[0]

	switch {
	case y == 'M' || x == 'M':
		return "M", "modified"
	case y == 'D' || x == 'D':
		return "D", "deleted"
	case x == 'A':
		return "A", "added"
	case x == 'R':
		return "R", "renamed"
	case xy == "??":
		return "?", "untracked"
	case x == 'U' || y == 'U':
		return "U", "conflict"
	default:
		return string(x), "changed"
	}
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	languages := map[string]string{
		".js":    "javascript",
		".jsx":   "javascript",
		".ts":    "typescript",
		".tsx":   "typescript",
		".go":    "go",
		".py":    "python",
		".rb":    "ruby",
		".rs":    "rust",
		".java":  "java",
		".json":  "json",
		".yaml":  "yaml",
		".yml":   "yaml",
		".toml":  "toml",
		".md":    "markdown",
		".css":   "css",
		".scss":  "scss",
		".html":  "html",
		".xml":   "xml",
		".sql":   "sql",
		".sh":    "shell",
		".bash":  "shell",
		".zsh":   "shell",
		".env":   "plaintext",
		".txt":   "plaintext",
		".vue":   "html",
		".svelte": "html",
		".c":     "c",
		".cpp":   "cpp",
		".h":     "c",
		".hpp":   "cpp",
		".swift": "swift",
		".kt":    "kotlin",
		".php":   "php",
		".lua":   "lua",
		".r":     "r",
		".dockerfile": "dockerfile",
	}

	// Handle special filenames
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case "gemfile", "rakefile":
		return "ruby"
	case ".gitignore", ".dockerignore":
		return "plaintext"
	}

	if lang, ok := languages[ext]; ok {
		return lang
	}
	return "plaintext"
}
