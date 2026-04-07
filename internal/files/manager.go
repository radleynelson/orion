package files

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FileEntry represents a file or directory in the workspace.
type FileEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	Children []FileEntry `json:"children,omitempty"`
}

// Manager handles file system operations.
type Manager struct {
	ctx context.Context
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Directories to skip in file listings
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
	"__pycache__":  true,
	".orion":       true,
	".sidecar":     true,
	".rad":         true,
	"tmp":          true,
}

// ListDirectory returns the contents of a directory, sorted (dirs first, then files).
// depth controls recursion: 1 = just this dir's children, 0 = no children.
func (m *Manager) ListDirectory(dir string, depth int) ([]FileEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var result []FileEntry
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files (except notable ones) and known heavy directories
		if strings.HasPrefix(name, ".") && name != ".env" && name != ".env.local" && name != ".orion.toml" && name != ".gitignore" {
			continue
		}
		if entry.IsDir() && skipDirs[name] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fe := FileEntry{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
		}

		// Recursively load children if depth > 0
		if entry.IsDir() && depth > 0 {
			children, err := m.ListDirectory(fe.Path, depth-1)
			if err == nil {
				fe.Children = children
			}
		}

		result = append(result, fe)
	}

	// Sort: directories first, then alphabetical
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

// ReadFileContents returns the contents of a file as a string.
// Refuses files larger than 5MB and detects binary files.
func (m *Manager) ReadFileContents(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}

	if info.Size() > 5*1024*1024 {
		return "", fmt.Errorf("file too large (%d bytes, max 5MB)", info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Check for binary content (null bytes in first 8KB)
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return "", fmt.Errorf("binary file")
		}
	}

	return string(data), nil
}

// SearchResult represents a file matching a search query.
type SearchResult struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// SearchFiles walks the workspace and fuzzy-matches filenames against a query.
// Returns up to maxResults matches, scored by relevance.
func (m *Manager) SearchFiles(root string, query string, maxResults int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	query = strings.ToLower(query)
	var results []scoredResult

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()

		// Skip hidden dirs and known heavy directories
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(name, ".") {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		score := scoreFileMatch(strings.ToLower(name), strings.ToLower(rel), query)
		if score > 0 {
			results = append(results, scoredResult{
				result: SearchResult{Name: name, Path: rel, IsDir: false},
				score:  score,
			})
		}
		return nil
	})

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Limit results
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = r.result
	}
	return out, nil
}

type scoredResult struct {
	result SearchResult
	score  int
}

// scoreFileMatch scores how well a query matches a file.
// It tries multiple strategies and returns the best score.
func scoreFileMatch(nameLower, relPathLower, query string) int {
	bestScore := 0

	// Strategy 1: Match against filename only
	s := fuzzyScore(nameLower, query)
	if s > 0 {
		// Big bonus for filename-only match (user is looking for this specific file)
		bestScore = max(bestScore, s+200)
	}

	// Strategy 2: Match against path with slashes (models/client.rb)
	s = fuzzyScore(relPathLower, query)
	bestScore = max(bestScore, s)

	// Strategy 3: Match against flattened path (modelsclient.rb)
	// Split query into segments that align with path boundaries
	pathFlat := strings.ReplaceAll(relPathLower, "/", "")
	s = fuzzyScore(pathFlat, query)
	if s > 0 {
		// Check if the query aligns with path segment boundaries
		// e.g., "modelsclient.rb" aligns perfectly with "models/" + "client.rb"
		segBonus := pathSegmentBonus(relPathLower, query)
		bestScore = max(bestScore, s+segBonus)
	}

	return bestScore
}

// pathSegmentBonus gives extra score when query chars align with path segment boundaries.
// e.g., query "modelsclient.rb" against path "backend/app/models/client.rb"
// — "models" matches segment "models", "client.rb" matches segment "client.rb" = big bonus
func pathSegmentBonus(relPath, query string) int {
	segments := strings.Split(relPath, "/")
	bonus := 0
	qi := 0

	for _, seg := range segments {
		if qi >= len(query) {
			break
		}
		// Check how many consecutive query chars match this segment from the start
		segMatch := 0
		si := 0
		for si < len(seg) && qi+segMatch < len(query) {
			if seg[si] == query[qi+segMatch] {
				segMatch++
			}
			si++
		}
		if segMatch > 0 {
			qi += segMatch
			// Full segment match is a strong signal
			if segMatch == len(seg) {
				bonus += 100
			} else if segMatch >= 3 {
				bonus += segMatch * 10
			}
		}
	}

	// Bonus if we consumed all query chars through segment matching
	if qi >= len(query) {
		bonus += 50
	}

	return bonus
}

// fuzzyScore returns a score for how well the text matches the query.
// Higher = better match. 0 = no match.
func fuzzyScore(text, query string) int {
	if len(query) == 0 {
		return 0
	}

	// Exact match
	if text == query {
		return 1000
	}

	// Suffix match (e.g., query "client.rb" matches "past_client.rb")
	if strings.HasSuffix(text, query) {
		return 600 + max(0, 100-len(text))
	}

	// Prefix match
	if strings.HasPrefix(text, query) {
		return 500 + max(0, 100-len(text))
	}

	// Contains match
	if strings.Contains(text, query) {
		return 300 + max(0, 100-len(text))
	}

	// Subsequence match (fuzzy)
	qi := 0
	consecutive := 0
	maxConsecutive := 0
	score := 0
	for ti := 0; ti < len(text) && qi < len(query); ti++ {
		if text[ti] == query[qi] {
			qi++
			consecutive++
			if consecutive > maxConsecutive {
				maxConsecutive = consecutive
			}
			score += 10
			// Bonus for matching after separator
			if ti > 0 && (text[ti-1] == '_' || text[ti-1] == '-' || text[ti-1] == '.' || text[ti-1] == '/') {
				score += 25
			}
			if ti == 0 {
				score += 30
			}
		} else {
			consecutive = 0
		}
	}

	if qi < len(query) {
		return 0
	}

	score += maxConsecutive * 20
	// Strongly prefer shorter filenames
	score += max(0, 80-len(text))

	return score
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GrepResult represents a content search match.
type GrepResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// SearchContents searches file contents using ripgrep (or fallback grep).
// Returns up to maxResults matches.
func (m *Manager) SearchContents(root string, query string, maxResults int) ([]GrepResult, error) {
	if query == "" {
		return nil, nil
	}

	// Try ripgrep first, fall back to grep
	var cmd *exec.Cmd
	rgPath, err := exec.LookPath("rg")
	if err == nil {
		cmd = exec.Command(rgPath, "--json", "--max-count", "3", "--max-filesize", "1M",
			"-g", "!node_modules", "-g", "!.git", "-g", "!vendor", "-g", "!dist",
			"-g", "!build", "-g", "!.next",
			query, root)
	} else {
		cmd = exec.Command("grep", "-rn", "--include=*.{go,ts,tsx,js,jsx,rb,py,rs,json,yaml,yml,toml,css,html,md,sh}",
			"-l", query, root)
	}

	out, err := cmd.Output()
	if err != nil {
		// grep/rg returns exit code 1 for no matches
		return nil, nil
	}

	var results []GrepResult

	if rgPath != "" {
		// Parse ripgrep JSON output
		for _, line := range strings.Split(string(out), "\n") {
			if len(results) >= maxResults {
				break
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Simple JSON parsing for ripgrep match lines
			if strings.Contains(line, `"type":"match"`) {
				result := parseRgMatch(line, root)
				if result != nil {
					results = append(results, *result)
				}
			}
		}
	}

	return results, nil
}

func parseRgMatch(jsonLine string, root string) *GrepResult {
	// Quick and dirty JSON extraction for ripgrep match format
	// {"type":"match","data":{"path":{"text":"..."},"lines":{"text":"..."},"line_number":N,...}}

	pathIdx := strings.Index(jsonLine, `"text":"`)
	if pathIdx < 0 {
		return nil
	}
	rest := jsonLine[pathIdx+8:]
	pathEnd := strings.Index(rest, `"`)
	if pathEnd < 0 {
		return nil
	}
	filePath := rest[:pathEnd]

	// Get relative path
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		rel = filePath
	}

	// Extract line number
	lineNum := 0
	lnIdx := strings.Index(jsonLine, `"line_number":`)
	if lnIdx >= 0 {
		numStr := jsonLine[lnIdx+14:]
		numEnd := strings.IndexAny(numStr, ",}")
		if numEnd > 0 {
			fmt.Sscanf(numStr[:numEnd], "%d", &lineNum)
		}
	}

	// Extract line content
	content := ""
	linesIdx := strings.Index(jsonLine, `"lines":{"text":"`)
	if linesIdx >= 0 {
		contentRest := jsonLine[linesIdx+17:]
		contentEnd := strings.Index(contentRest, `"`)
		if contentEnd > 0 && contentEnd < 200 {
			content = strings.TrimSpace(contentRest[:contentEnd])
		}
	}

	return &GrepResult{
		File:    rel,
		Line:    lineNum,
		Content: content,
	}
}
