package claudesdk

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type historyFile struct {
	path    string
	modTime time.Time
}

type HistoryThread struct {
	ThreadID      string `json:"threadId"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	Model         string `json:"model,omitempty"`
	UpdatedAt     string `json:"updatedAt"`
	MessageCount  int    `json:"messageCount"`
	Preview       string `json:"preview,omitempty"`
}

type historyTranscriptRecord struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	CWD         string          `json:"cwd"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

type historyTranscriptMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

// ListHistory returns Claude Code SDK sessions persisted in ~/.claude for a
// workspace. It is intentionally lightweight: desktop history only needs enough
// metadata to resume a thread and show a useful preview.
func ListHistory(workspacePath string, limit int) []HistoryThread {
	workspacePath = strings.TrimSpace(workspacePath)
	if limit <= 0 {
		limit = 20
	}
	var out []HistoryThread
	seen := map[string]bool{}
	for _, file := range claudeHistoryFiles(workspacePath) {
		thread := loadHistoryThread(file.path, workspacePath)
		if strings.TrimSpace(thread.ThreadID) == "" || thread.MessageCount == 0 || seen[thread.ThreadID] {
			continue
		}
		if thread.UpdatedAt == "" {
			thread.UpdatedAt = file.modTime.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, thread)
		seen[thread.ThreadID] = true
		if len(out) >= limit {
			break
		}
	}
	return out
}

func claudeHistoryFiles(workspacePath string) []historyFile {
	root := userClaudeProjectsRoot()
	if root == "" {
		return nil
	}
	var dirs []string
	if workspacePath != "" {
		dirs = append(dirs, filepath.Join(root, claudeProjectDir(workspacePath)))
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, filepath.Join(root, entry.Name()))
			}
		}
	}

	var files []historyFile
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, historyFile{
				path:    filepath.Join(dir, entry.Name()),
				modTime: info.ModTime(),
			})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

func loadHistoryThread(path string, workspacePath string) HistoryThread {
	f, err := os.Open(path)
	if err != nil {
		return HistoryThread{}
	}
	defer f.Close()

	thread := HistoryThread{
		ThreadID: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
	}
	var lastText string
	var lastTimestamp string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec historyTranscriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.IsSidechain || (rec.Type != "user" && rec.Type != "assistant") || len(rec.Message) == 0 || string(rec.Message) == "null" {
			continue
		}
		if workspacePath != "" && rec.CWD != "" && !sameClaudeHistoryPath(rec.CWD, workspacePath) {
			continue
		}
		var msg historyTranscriptMessage
		if err := json.Unmarshal(rec.Message, &msg); err != nil {
			continue
		}
		text := historyTextFromContent(rec.Type, msg.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if rec.SessionID != "" {
			thread.ThreadID = rec.SessionID
		}
		if rec.CWD != "" {
			thread.WorkspacePath = rec.CWD
		}
		if msg.Model != "" {
			thread.Model = msg.Model
		}
		thread.MessageCount++
		lastText = text
		if rec.Timestamp != "" {
			lastTimestamp = rec.Timestamp
		}
	}
	if thread.WorkspacePath == "" {
		thread.WorkspacePath = workspacePath
	}
	thread.Preview = compactHistoryPreview(lastText)
	thread.UpdatedAt = historyTimestamp(lastTimestamp)
	return thread
}

func historyTextFromContent(recordType string, raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return strings.TrimSpace(direct)
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var texts []string
	for _, block := range blocks {
		switch normalize(stringValue(block["type"])) {
		case "text":
			if text := strings.TrimSpace(stringValue(block["text"])); text != "" {
				texts = append(texts, text)
			}
		case "tooluse":
			if recordType == "assistant" && normalize(stringValue(block["name"])) == "exitplanmode" {
				input := mapValue(block["input"])
				plan := strings.TrimSpace(stringValue(input["plan"]))
				if plan != "" {
					texts = append(texts, planTitle(plan))
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n\n"))
}

func compactHistoryPreview(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 180 {
		return text[:177] + "..."
	}
	return text
}

func historyTimestamp(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func sameClaudeHistoryPath(a string, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if ar, err := filepath.EvalSymlinks(a); err == nil {
		a = ar
	}
	if br, err := filepath.EvalSymlinks(b); err == nil {
		b = br
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return a == b
}
