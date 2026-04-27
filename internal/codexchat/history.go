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
	ID                string `json:"id"`
	CWD               string `json:"cwd"`
	Model             string `json:"model"`
	ReasoningEffort   string `json:"reasoningEffort"`
	CollaborationMode string `json:"collaborationMode"`
}

type codexTurnContextPayload struct {
	Model             string `json:"model"`
	Effort            string `json:"effort"`
	CollaborationMode struct {
		Mode     string `json:"mode"`
		Settings struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
		} `json:"settings"`
	} `json:"collaboration_mode"`
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
	meta, ok := readSessionMeta(path)
	if !ok || meta.ID != threadID {
		return nil
	}
	if strings.TrimSpace(workspacePath) != "" && !samePath(meta.CWD, workspacePath) {
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
		if msg.Type == "plan" {
			if details := formatPlanDetails(firstNonEmptyString(msg.Details, msg.Text)); details != "" {
				msg.Details = details
				msg.Text = "Plan ready"
			}
		}
		out = append(out, msg)
	}
	if len(out) > 1000 {
		out = out[len(out)-1000:]
	}
	return out
}

func MergeRestoredMessages(cached []Message, history []Message) []Message {
	if len(cached) == 0 {
		return history
	}
	if len(history) == 0 {
		return cached
	}
	out := make([]Message, 0, len(cached)+len(history))
	seenIDs := map[string]bool{}
	seenContent := map[string]bool{}
	appendMessage := func(msg Message) {
		if msg.ID != "" {
			if seenIDs[msg.ID] {
				return
			}
			seenIDs[msg.ID] = true
		}
		if key := messageContentKey(msg); key != "" {
			if seenContent[key] {
				return
			}
			seenContent[key] = true
		}
		out = append(out, msg)
	}
	for _, msg := range cached {
		appendMessage(msg)
	}
	for _, msg := range history {
		appendMessage(msg)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return messageSortTime(out[i]).Before(messageSortTime(out[j]))
	})
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

func messageContentKey(msg Message) string {
	switch msg.Type {
	case "user", "assistant":
		text := strings.Join(strings.Fields(msg.Text), " ")
		if text == "" {
			return ""
		}
		return msg.Type + "\x00" + msg.Role + "\x00" + text
	default:
		return ""
	}
}

func messageSortTime(msg Message) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, msg.CreatedAt); err == nil {
		return parsed
	}
	return time.Time{}
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

func ThreadOptions(threadID string) codexSessionMeta {
	path := FindSessionFile(threadID)
	if path == "" {
		return codexSessionMeta{}
	}
	meta, _ := readSessionMeta(path)
	return meta
}

// ResolveThreadIDForTmux returns the best-known Codex thread ID for a
// tmux-backed terminal. It only returns IDs that can be validated against the
// requested workspace, or a single unambiguous workspace history candidate.
func ResolveThreadIDForTmux(tmuxSession string, workspacePath string) (string, error) {
	var processThreadIDs []string
	for _, command := range descendantProcessCommands(tmuxPanePID(tmuxSession)) {
		for _, id := range ParseResumeIDs(command) {
			processThreadIDs = append(processThreadIDs, id)
		}
	}
	return resolveThreadIDForWorkspace(
		workspacePath,
		tmuxOption(tmuxSession, "@orion_thread_id"),
		processThreadIDs,
	)
}

// ThreadIDForTmux preserves the old string-returning API for callers that only
// need a best-effort ID. Prefer ResolveThreadIDForTmux when surfacing errors.
func ThreadIDForTmux(tmuxSession string, workspacePath string) string {
	threadID, err := ResolveThreadIDForTmux(tmuxSession, workspacePath)
	if err != nil {
		return ""
	}
	return threadID
}

func resolveThreadIDForWorkspace(workspacePath string, tmuxThreadID string, processThreadIDs []string) (string, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return "", fmt.Errorf("workspacePath required")
	}
	if id := strings.TrimSpace(tmuxThreadID); id != "" {
		if ValidThreadForWorkspace(id, workspacePath) {
			return id, nil
		}
	}
	for _, id := range uniqueStrings(processThreadIDs) {
		if ValidThreadForWorkspace(id, workspacePath) {
			return id, nil
		}
	}
	threadID, err := uniqueHistoryThreadIDForWorkspace(workspacePath)
	if err != nil {
		return "", err
	}
	return threadID, nil
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

func uniqueHistoryThreadIDForWorkspace(workspacePath string) (string, error) {
	var candidates []HistoryThread
	for _, thread := range ListHistory(workspacePath, 50) {
		if strings.TrimSpace(thread.ThreadID) == "" || thread.MessageCount == 0 {
			continue
		}
		candidates = append(candidates, thread)
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("could not identify Codex thread for workspace %s", workspacePath)
	case 1:
		return candidates[0].ThreadID, nil
	default:
		return "", fmt.Errorf("multiple Codex transcripts found for workspace %s; choose a session from history instead of converting this terminal automatically", workspacePath)
	}
}

func ValidThreadForWorkspace(threadID string, workspacePath string) bool {
	threadID = strings.TrimSpace(threadID)
	workspacePath = strings.TrimSpace(workspacePath)
	if threadID == "" || workspacePath == "" {
		return false
	}
	path := FindSessionFile(threadID)
	if path == "" {
		return false
	}
	meta, ok := readSessionMeta(path)
	return ok && meta.ID == threadID && samePath(meta.CWD, workspacePath)
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
			if id := resumePositionalID(fields[i+1:]); id != "" {
				ids = append(ids, id)
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

func resumePositionalID(fields []string) string {
	for i := len(fields) - 1; i >= 0; i-- {
		field := strings.TrimSpace(fields[i])
		if field == "" || strings.HasPrefix(field, "-") {
			continue
		}
		if i > 0 && resumeFlagConsumesValue(fields[i-1]) {
			continue
		}
		return field
	}
	return ""
}

func resumeFlagConsumesValue(field string) bool {
	switch strings.TrimSpace(field) {
	case "-m", "--model", "-c", "--config", "-s", "--sandbox", "-C", "--cd", "--profile":
		return true
	default:
		return false
	}
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
	var meta codexSessionMeta
	for i := 0; i < 80 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry codexLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if len(entry.Payload) == 0 {
			continue
		}
		switch entry.Type {
		case "session_meta":
			var sessionMeta codexSessionMeta
			if err := json.Unmarshal(entry.Payload, &sessionMeta); err != nil {
				return codexSessionMeta{}, false
			}
			meta.ID = firstNonEmptyString(meta.ID, sessionMeta.ID)
			meta.CWD = firstNonEmptyString(meta.CWD, sessionMeta.CWD)
			meta.Model = firstNonEmptyString(meta.Model, sessionMeta.Model)
		case "turn_context":
			var turn codexTurnContextPayload
			if err := json.Unmarshal(entry.Payload, &turn); err != nil {
				continue
			}
			meta.Model = firstNonEmptyString(meta.Model, turn.CollaborationMode.Settings.Model, turn.Model)
			meta.ReasoningEffort = firstNonEmptyString(meta.ReasoningEffort, turn.CollaborationMode.Settings.ReasoningEffort, turn.Effort)
			meta.CollaborationMode = firstNonEmptyString(meta.CollaborationMode, turn.CollaborationMode.Mode)
		}
		if meta.ID != "" && meta.CWD != "" && meta.Model != "" && meta.ReasoningEffort != "" && meta.CollaborationMode != "" {
			break
		}
	}
	return meta, meta.ID != ""
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

func tmuxOption(name string, option string) string {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(option) == "" {
		return ""
	}
	out, err := exec.Command("tmux", "show-options", "-v", "-t", name, option).Output()
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

func uniqueStrings(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
