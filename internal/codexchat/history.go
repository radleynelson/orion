package codexchat

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sessionFile struct {
	path    string
	modTime time.Time
}

type codexLogEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	ID    string `json:"id"`
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

type codexEventPayload struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	LastMessage string `json:"last_agent_message"`
	Phase       string `json:"phase"`
	Images      []any  `json:"images"`
	LocalImages []any  `json:"local_images"`
}

type HistoryThread struct {
	ThreadID      string `json:"threadId"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	Model         string `json:"model,omitempty"`
	UpdatedAt     string `json:"updatedAt"`
	MessageCount  int    `json:"messageCount"`
	Preview       string `json:"preview,omitempty"`
}

// LoadHistory reads Codex's persisted JSONL transcript for a thread and returns
// chat-shaped user and assistant messages. Runtime events remain in memory; this
// is only the durable baseline for restored chat sessions.
func LoadHistory(threadID string, workspacePath string) []Message {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	path := FindSessionFile(threadID)
	if path == "" {
		return nil
	}
	return loadHistoryFromFile(path, threadID, workspacePath)
}

// LoadCachedMessages reads Orion's normalized chat event cache. Codex's own
// history is text-centric, so this preserves rich cards across Orion restarts.
func LoadCachedMessages(threadID string, workspacePath string) []Message {
	path, ok := cachePath(threadID)
	if !ok {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Message
	seen := map[string]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID == "" || seen[msg.ID] || msg.Type == "status" {
			continue
		}
		seen[msg.ID] = true
		msg.SessionID = ""
		if msg.ThreadID == "" {
			msg.ThreadID = threadID
		}
		out = append(out, msg)
	}
	if len(out) > 1000 {
		out = out[len(out)-1000:]
	}
	return out
}

func AppendCachedMessage(msg Message) {
	if !shouldCacheMessage(msg) {
		return
	}
	path, ok := cachePath(msg.ThreadID)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	msg.SessionID = ""
	if data, err := json.Marshal(msg); err == nil {
		_, _ = f.Write(append(data, '\n'))
	}
}

func shouldCacheMessage(msg Message) bool {
	if strings.TrimSpace(msg.ThreadID) == "" || strings.TrimSpace(msg.ID) == "" {
		return false
	}
	switch msg.Type {
	case "system", "user", "assistant", "tool", "tool_result", "permission_request", "permission_submitted", "permission_resolved", "plan", "plan_resolved", "error":
		return true
	default:
		return false
	}
}

func cachePath(threadID string) (string, bool) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".orion", "chat-cache", "codex", shortHash(threadID)+".jsonl"), true
}

func FindSessionFile(threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	files := codexSessionFiles()
	for _, file := range files {
		if strings.Contains(filepath.Base(file.path), threadID) {
			return file.path
		}
		meta, ok := readSessionMeta(file.path)
		if ok && meta.ID == threadID {
			return file.path
		}
	}
	return ""
}

// ThreadIDForTmux returns the best-known Codex thread ID for a tmux-backed
// terminal. It first checks a running "codex resume" command, then falls back to
// the newest Codex transcript for the workspace.
func ThreadIDForTmux(tmuxSession string, workspacePath string) string {
	for _, command := range descendantProcessCommands(tmuxPanePID(tmuxSession)) {
		for _, id := range ParseResumeIDs(command) {
			if id != "" {
				return id
			}
		}
	}
	return LatestThreadIDForWorkspace(workspacePath)
}

func LatestThreadIDForWorkspace(workspacePath string) string {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return ""
	}
	for _, file := range codexSessionFiles() {
		meta, ok := readSessionMeta(file.path)
		if ok && meta.ID != "" && samePath(meta.CWD, workspacePath) {
			return meta.ID
		}
	}
	return ""
}

func ListHistory(workspacePath string, limit int) []HistoryThread {
	workspacePath = strings.TrimSpace(workspacePath)
	if limit <= 0 {
		limit = 20
	}
	var out []HistoryThread
	seen := map[string]bool{}
	for _, file := range codexSessionFiles() {
		meta, ok := readSessionMeta(file.path)
		if !ok || strings.TrimSpace(meta.ID) == "" {
			continue
		}
		if workspacePath != "" && !samePath(meta.CWD, workspacePath) {
			continue
		}
		if seen[meta.ID] {
			continue
		}
		messages := loadHistoryFromFile(file.path, meta.ID, meta.CWD)
		out = append(out, HistoryThread{
			ThreadID:      meta.ID,
			WorkspacePath: meta.CWD,
			Model:         meta.Model,
			UpdatedAt:     file.modTime.UTC().Format(time.RFC3339Nano),
			MessageCount:  len(messages),
			Preview:       historyPreview(messages),
		})
		seen[meta.ID] = true
		if len(out) >= limit {
			break
		}
	}
	return out
}

func ParseResumeIDs(command string) []string {
	if !strings.Contains(command, "codex") {
		return nil
	}
	fields := strings.Fields(command)
	var ids []string
	for i, field := range fields {
		switch {
		case field == "resume":
			for j := i + 1; j < len(fields); j++ {
				if !strings.HasPrefix(fields[j], "-") {
					ids = append(ids, strings.TrimSpace(fields[j]))
					break
				}
			}
		case field == "--resume" || field == "-r":
			if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
				ids = append(ids, strings.TrimSpace(fields[i+1]))
			}
		case strings.HasPrefix(field, "--resume="):
			ids = append(ids, strings.TrimSpace(strings.TrimPrefix(field, "--resume=")))
		}
	}
	return ids
}

func loadHistoryFromFile(path string, threadID string, workspacePath string) []Message {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Message
	seen := map[string]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry codexLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "event_msg" || len(entry.Payload) == 0 {
			continue
		}
		var payload codexEventPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			continue
		}
		role := ""
		text := ""
		switch payload.Type {
		case "user_message":
			role = "user"
			text = payload.Message
			imageCount := len(payload.Images) + len(payload.LocalImages)
			if strings.TrimSpace(text) == "" && imageCount > 0 {
				text = attachmentOnlyText(imageCount)
			}
		case "agent_message":
			role = "assistant"
			text = payload.Message
		}
		text = strings.TrimSpace(text)
		if role == "" || text == "" {
			continue
		}
		key := role + "\x00" + text
		if seen[key] {
			continue
		}
		seen[key] = true
		id := "history-" + shortHash(threadID+"\x00"+entry.Timestamp+"\x00"+key)
		out = append(out, Message{
			ID:        id,
			ThreadID:  threadID,
			Type:      role,
			Role:      role,
			Text:      text,
			CreatedAt: historyTimestamp(entry.Timestamp),
		})
	}
	if len(out) > 300 {
		out = out[len(out)-300:]
	}
	return out
}

func readSessionMeta(path string) (codexSessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)
	for i := 0; i < 20 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry codexLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "session_meta" || len(entry.Payload) == 0 {
			continue
		}
		var meta codexSessionMeta
		if err := json.Unmarshal(entry.Payload, &meta); err != nil {
			return codexSessionMeta{}, false
		}
		return meta, meta.ID != ""
	}
	return codexSessionMeta{}, false
}

func codexSessionFiles() []sessionFile {
	var files []sessionFile
	seen := map[string]bool{}
	for _, root := range codexSessionRoots() {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			if seen[path] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			seen[path] = true
			files = append(files, sessionFile{path: path, modTime: info.ModTime()})
			return nil
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

func codexSessionRoots() []string {
	var roots []string
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		roots = append(roots, filepath.Join(codexHome, "sessions"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, ".codex", "sessions"))
	}
	return roots
}

func tmuxPanePID(tmuxSession string) int {
	tmuxSession = strings.TrimSpace(tmuxSession)
	if tmuxSession == "" {
		return 0
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", tmuxSession, "#{pane_pid}").Output()
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return pid
}

func descendantProcessCommands(rootPID int) []string {
	if rootPID <= 0 {
		return nil
	}
	var commands []string
	queue := []int{rootPID}
	seen := map[int]bool{rootPID: true}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if command := processCommand(pid); command != "" {
			commands = append(commands, command)
		}
		out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			child, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil || child <= 0 || seen[child] {
				continue
			}
			seen[child] = true
			queue = append(queue, child)
		}
	}
	return commands
}

func processCommand(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func samePath(a string, b string) bool {
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

func historyTimestamp(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func historyPreview(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		text := strings.Join(strings.Fields(messages[i].Text), " ")
		if text == "" {
			continue
		}
		if len(text) > 180 {
			return text[:177] + "..."
		}
		return text
	}
	return ""
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func attachmentOnlyTextForHistory(count int) string {
	if count <= 0 {
		return ""
	}
	if count == 1 {
		return "Image attached."
	}
	return fmt.Sprintf("%d images attached.", count)
}
