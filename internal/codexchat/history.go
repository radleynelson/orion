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

	"orion/internal/chatattachments"
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
	case "system", "user", "assistant", "tool", "tool_result", "permission_request", "permission_submitted", "permission_resolved", "plan", "plan_resolved", "task_list", "error":
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
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return "", fmt.Errorf("workspacePath required")
	}
	if path := validStampedTranscriptPath(tmuxOption(tmuxSession, "@orion_transcript_path"), workspacePath); path != "" {
		if meta, ok := readSessionMeta(path); ok && strings.TrimSpace(meta.ID) != "" {
			return strings.TrimSpace(meta.ID), nil
		}
	}
	var processThreadIDs []string
	for _, command := range descendantProcessCommands(tmuxPanePID(tmuxSession)) {
		for _, id := range ParseResumeIDs(command) {
			processThreadIDs = append(processThreadIDs, id)
		}
	}
	if id := strings.TrimSpace(tmuxOption(tmuxSession, "@orion_thread_id")); id != "" {
		if ValidThreadForWorkspace(id, workspacePath) {
			return id, nil
		}
	}
	for _, id := range uniqueStrings(processThreadIDs) {
		if ValidThreadForWorkspace(id, workspacePath) {
			return id, nil
		}
	}
	if threadID := threadIDForTmuxHistoryWindow(tmuxSession, workspacePath); threadID != "" {
		return threadID, nil
	}
	return uniqueHistoryThreadIDForWorkspace(workspacePath)
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

func threadIDForTmuxHistoryWindow(tmuxSession string, workspacePath string) string {
	start, end := codexHistoryWindowForTmux(tmuxSession, workspacePath)
	threadID, ok := historyThreadIDForWorkspaceWindow(workspacePath, start, end)
	if !ok {
		return ""
	}
	setTmuxOption(tmuxSession, "@orion_thread_id", threadID)
	return threadID
}

func codexHistoryWindowForTmux(tmuxSession string, workspacePath string) (time.Time, time.Time) {
	start := tmuxSessionStartTime(tmuxSession)
	if start.IsZero() {
		return time.Time{}, time.Time{}
	}
	var end time.Time
	for _, session := range tmuxSessionNames() {
		if session == tmuxSession {
			continue
		}
		if tmuxOption(session, "@orion_type") != "codex" {
			continue
		}
		if !samePath(tmuxOption(session, "@orion_workspace"), workspacePath) {
			continue
		}
		otherStart := tmuxSessionStartTime(session)
		if otherStart.IsZero() || !otherStart.After(start) {
			continue
		}
		if end.IsZero() || otherStart.Before(end) {
			end = otherStart
		}
	}
	return start, end
}

func historyThreadIDForWorkspaceWindow(workspacePath string, start time.Time, end time.Time) (string, bool) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" || start.IsZero() {
		return "", false
	}
	var candidates []string
	for _, file := range codexSessionFiles() {
		meta, startedAt, ok := readSessionMetaWithTimestamp(file.path, file.modTime)
		if !ok || strings.TrimSpace(meta.ID) == "" || !samePath(meta.CWD, workspacePath) {
			continue
		}
		if startedAt.IsZero() || startedAt.Before(start) {
			continue
		}
		if !end.IsZero() && !startedAt.Before(end) {
			continue
		}
		if len(loadHistoryFromFile(file.path, meta.ID, meta.CWD)) == 0 {
			continue
		}
		candidates = append(candidates, meta.ID)
	}
	candidates = uniqueStrings(candidates)
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
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
			MessageCount:  historyVisibleMessageCount(messages),
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
	messages, _, err := parseCodexTranscript(path, workspacePath, "")
	if err != nil {
		return nil
	}
	if threadID != "" {
		for i := range messages {
			if messages[i].ThreadID == "" {
				messages[i].ThreadID = threadID
			}
		}
	}
	if len(messages) > 300 {
		messages = messages[len(messages)-300:]
	}
	return messages
}

func parseCodexTranscript(path string, workspacePath string, sessionID string) ([]Message, codexSessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, codexSessionMeta{}, err
	}
	defer f.Close()

	var out []Message
	seen := map[string]bool{}
	chatContentSeen := map[string]bool{}
	completedTools := map[string]bool{}
	toolNames := map[string]string{}
	meta := codexSessionMeta{}
	currentCollaborationMode := ""
	appendMessage := func(msg Message, keyParts ...string) {
		if msg.Type == "" {
			return
		}
		if msg.CreatedAt == "" {
			msg.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if msg.SessionID == "" {
			msg.SessionID = sessionID
		}
		if msg.ThreadID == "" {
			msg.ThreadID = meta.ID
		}
		key := strings.Join(keyParts, "\x00")
		if key == "" {
			key = msg.ID
		}
		if key == "" {
			key = msg.Type + "\x00" + msg.ToolUseID + "\x00" + msg.ToolName + "\x00" + msg.Text
		}
		if seen[key] {
			return
		}
		seen[key] = true
		if msg.ID == "" {
			msg.ID = "codex-" + shortHash(path+"\x00"+key)
		}
		out = append(out, msg)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry codexLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if len(entry.Payload) == 0 || string(entry.Payload) == "null" {
			continue
		}

		createdAt := historyTimestamp(entry.Timestamp)
		lineKey := fmt.Sprintf("%s:%d", path, lineNo)
		switch entry.Type {
		case "session_meta":
			var sessionMeta codexSessionMeta
			if err := json.Unmarshal(entry.Payload, &sessionMeta); err == nil {
				meta.ID = firstNonEmptyString(meta.ID, sessionMeta.ID)
				meta.CWD = firstNonEmptyString(meta.CWD, sessionMeta.CWD)
				meta.Model = firstNonEmptyString(meta.Model, sessionMeta.Model)
				meta.ReasoningEffort = firstNonEmptyString(meta.ReasoningEffort, sessionMeta.ReasoningEffort)
				meta.CollaborationMode = firstNonEmptyString(meta.CollaborationMode, sessionMeta.CollaborationMode)
			}
		case "turn_context":
			var turn codexTurnContextPayload
			if err := json.Unmarshal(entry.Payload, &turn); err == nil {
				previousCollaborationMode := currentCollaborationMode
				nextCollaborationMode := firstNonEmptyString(turn.CollaborationMode.Mode, meta.CollaborationMode)
				meta.Model = firstNonEmptyString(meta.Model, turn.CollaborationMode.Settings.Model, turn.Model)
				meta.ReasoningEffort = firstNonEmptyString(meta.ReasoningEffort, turn.CollaborationMode.Settings.ReasoningEffort, turn.Effort)
				if nextCollaborationMode != "" {
					meta.CollaborationMode = nextCollaborationMode
				}
				currentCollaborationMode = nextCollaborationMode
				if normalizeCodexKind(previousCollaborationMode) == "plan" && normalizeCodexKind(nextCollaborationMode) != "plan" {
					appendMessage(Message{
						ID:        "codex-" + shortHash(lineKey+":plan_resolved"),
						Type:      "plan_resolved",
						Text:      "Plan approved",
						CreatedAt: createdAt,
					}, "plan_resolved", lineKey)
				}
			}
		case "event_msg":
			var event codexEventPayload
			if err := json.Unmarshal(entry.Payload, &event); err != nil {
				continue
			}
			raw := map[string]any{}
			_ = json.Unmarshal(entry.Payload, &raw)
			parseCodexEventMessage(event, raw, createdAt, lineKey, currentCollaborationMode, appendMessage, completedTools, chatContentSeen)
		case "response_item":
			var item map[string]any
			if err := json.Unmarshal(entry.Payload, &item); err != nil {
				continue
			}
			parseCodexResponseItem(item, createdAt, lineKey, currentCollaborationMode, appendMessage, completedTools, toolNames, chatContentSeen)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, meta, err
	}
	if workspacePath != "" && meta.CWD != "" && !samePath(meta.CWD, workspacePath) {
		return nil, meta, nil
	}
	return out, meta, nil
}

func parseCodexEventMessage(payload codexEventPayload, raw map[string]any, createdAt string, lineKey string, collaborationMode string, appendMessage func(Message, ...string), completedTools map[string]bool, chatContentSeen map[string]bool) {
	switch payload.Type {
	case "user_message":
		text := strings.TrimSpace(payload.Message)
		imageCount := len(payload.Images) + len(payload.LocalImages)
		if text == "" && imageCount > 0 {
			text = attachmentOnlyText(imageCount)
		}
		if text == "" {
			return
		}
		rememberCodexChatContent(chatContentSeen, "user", text)
		appendMessage(Message{
			ID:        "codex-" + shortHash(lineKey+":user"),
			Type:      "user",
			Role:      "user",
			Text:      text,
			CreatedAt: createdAt,
		}, "user", lineKey)
	case "agent_message":
		text := strings.TrimSpace(firstNonEmptyString(payload.Message, payload.LastMessage))
		if text == "" {
			return
		}
		if normalizeCodexKind(collaborationMode) == "plan" && isPlanLikeText(text) {
			rememberCodexChatContent(chatContentSeen, "plan", text)
			appendMessage(Message{
				ID:        "codex-" + shortHash(lineKey+":plan"),
				Type:      "plan",
				Role:      "assistant",
				Text:      "Plan ready",
				Details:   formatPlanDetails(text),
				Status:    "waiting_approval",
				CreatedAt: createdAt,
			}, "plan", lineKey)
			return
		}
		rememberCodexChatContent(chatContentSeen, "assistant", text)
		appendMessage(Message{
			ID:        "codex-" + shortHash(lineKey+":assistant"),
			Type:      "assistant",
			Role:      "assistant",
			Text:      text,
			CreatedAt: createdAt,
		}, "assistant", lineKey)
	case "agent_reasoning":
		text := strings.TrimSpace(firstNonEmptyString(payload.Message, payload.LastMessage))
		if text == "" {
			return
		}
		appendMessage(Message{
			ID:        "codex-" + shortHash(lineKey+":reasoning"),
			Type:      "thinking_delta",
			Text:      text,
			CreatedAt: createdAt,
		}, "thinking_delta", lineKey)
	case "task_started":
		appendMessage(Message{
			ID:        "codex-" + shortHash(lineKey+":task_started"),
			Type:      "status",
			Status:    "running",
			Text:      "Codex is thinking",
			CreatedAt: createdAt,
		}, "status", "running", createdAt)
	case "task_complete":
		appendMessage(Message{
			ID:        "codex-" + shortHash(lineKey+":task_complete"),
			Type:      "result",
			Subtype:   "completed",
			Text:      "completed",
			CreatedAt: createdAt,
		}, "result", "completed", createdAt)
		appendMessage(Message{
			ID:        "codex-" + shortHash(lineKey+":task_idle"),
			Type:      "status",
			Status:    "idle",
			CreatedAt: createdAt,
		}, "status", "idle", createdAt)
	case "exec_command_start", "exec_command_begin":
		toolUseID := firstNonEmptyString(stringFrom(raw, "call_id", "callId", "id"), lineKey)
		command := firstNonEmptyString(stringFrom(raw, "command", "cmd"), commandText(raw["command"]))
		appendMessage(Message{
			ID:        "codex-" + toolUseID + ":tool",
			Type:      "tool",
			ToolUseID: toolUseID,
			ToolName:  "Bash",
			Text:      firstNonEmptyString(command, "Running command"),
			Details:   compactAny(raw),
			CreatedAt: createdAt,
		}, "tool", toolUseID)
	case "exec_command_end":
		toolUseID := firstNonEmptyString(stringFrom(raw, "call_id", "callId", "id"), lineKey)
		if completedTools[toolUseID] {
			return
		}
		output := firstNonEmptyString(
			stringFrom(raw, "aggregated_output", "aggregatedOutput", "output", "stdout", "stderr"),
			codexCombinedOutput(raw),
			"Command completed",
		)
		completedTools[toolUseID] = true
		appendMessage(Message{
			ID:        "codex-" + toolUseID + ":tool_result",
			Type:      "tool_result",
			ToolUseID: toolUseID,
			ToolName:  "Bash",
			Text:      output,
			Details:   compactAny(raw),
			CreatedAt: createdAt,
		}, "tool_result", toolUseID)
	case "web_search_end":
		toolUseID := firstNonEmptyString(stringFrom(raw, "call_id", "callId", "id"), lineKey)
		query := firstNonEmptyString(stringFrom(raw, "query", "action"), payload.Message, "Search complete")
		appendMessage(Message{
			ID:        "codex-" + toolUseID + ":websearch_result",
			Type:      "tool_result",
			ToolUseID: toolUseID,
			ToolName:  "WebSearch",
			Text:      query,
			Details:   compactAny(raw),
			CreatedAt: createdAt,
		}, "tool_result", toolUseID)
	}
}

func parseCodexResponseItem(item map[string]any, createdAt string, lineKey string, collaborationMode string, appendMessage func(Message, ...string), completedTools map[string]bool, toolNames map[string]string, chatContentSeen map[string]bool) {
	itemType := normalizeCodexKind(stringFrom(item, "type"))
	itemID := firstNonEmptyString(stringFrom(item, "id"), stringFrom(item, "call_id"), lineKey)
	switch itemType {
	case "functioncall":
		rawName := stringFrom(item, "name")
		args := codexArgsMap(item["arguments"])
		if normalizeCodexKind(rawName) == "updateplan" {
			appendMessage(Message{
				ID:        "codex-" + itemID + ":task_list",
				Type:      "task_list",
				ToolUseID: itemID,
				ToolName:  "update_plan",
				Text:      "Tasks updated",
				Details:   formatPlanDetails(args),
				CreatedAt: createdAt,
			}, "task_list", itemID)
			return
		}
		toolName := codexFunctionToolName(rawName)
		toolNames[itemID] = toolName
		text := codexFunctionCallText(toolName, args)
		if toolName == "AskUserQuestion" {
			appendMessage(Message{
				ID:        "codex-" + itemID + ":permission_request",
				Type:      "permission_request",
				ToolUseID: itemID,
				ToolName:  toolName,
				Text:      text,
				Details:   compactAny(args),
				CreatedAt: createdAt,
			}, "permission_request", itemID)
			return
		}
		appendMessage(Message{
			ID:        "codex-" + itemID + ":tool",
			Type:      "tool",
			ToolUseID: itemID,
			ToolName:  toolName,
			Text:      text,
			Details:   compactAny(args),
			CreatedAt: createdAt,
		}, "tool", itemID)
	case "functioncalloutput":
		toolName := firstNonEmptyString(toolNames[itemID], "Tool")
		text := codexFunctionOutputText(item)
		if text == "" {
			text = "Tool completed"
		}
		completedTools[itemID] = true
		if toolName == "AskUserQuestion" {
			appendMessage(Message{
				ID:        "codex-" + itemID + ":permission_resolved",
				Type:      "permission_resolved",
				ToolUseID: itemID,
				ToolName:  toolName,
				Text:      "Answered",
				Details:   text,
				CreatedAt: createdAt,
			}, "permission_resolved", itemID)
			return
		}
		appendMessage(Message{
			ID:        "codex-" + itemID + ":tool_result",
			Type:      "tool_result",
			ToolUseID: itemID,
			ToolName:  toolName,
			Text:      text,
			CreatedAt: createdAt,
		}, "tool_result", itemID)
	case "reasoning":
		if text := codexReasoningText(item); text != "" {
			appendMessage(Message{
				ID:        "codex-" + itemID + ":reasoning",
				Type:      "thinking_delta",
				Text:      text,
				CreatedAt: createdAt,
			}, "thinking_delta", itemID, text)
		}
	case "websearchcall":
		query := firstNonEmptyString(stringFrom(item, "query"), stringFrom(item, "action"))
		appendMessage(Message{
			ID:        "codex-" + itemID + ":websearch",
			Type:      "tool",
			ToolUseID: itemID,
			ToolName:  "WebSearch",
			Text:      firstNonEmptyString(query, "Searching the web"),
			Details:   compactAny(item),
			CreatedAt: createdAt,
		}, "tool", itemID)
	case "message":
		text := codexMessageText(item)
		if text == "" {
			return
		}
		role := stringFrom(item, "role")
		if role == "" {
			role = "assistant"
		}
		if normalizeCodexKind(collaborationMode) == "plan" && normalizeCodexKind(role) == "assistant" && isPlanLikeText(text) {
			if codexChatContentSeen(chatContentSeen, "plan", text) {
				return
			}
			rememberCodexChatContent(chatContentSeen, "plan", text)
			appendMessage(Message{
				ID:        "codex-" + itemID + ":plan",
				Type:      "plan",
				Role:      "assistant",
				Text:      "Plan ready",
				Details:   formatPlanDetails(text),
				Status:    "waiting_approval",
				CreatedAt: createdAt,
			}, "plan", itemID, text)
			return
		}
		if normalizeCodexKind(role) != "user" && normalizeCodexKind(role) != "assistant" {
			return
		}
		if codexChatContentSeen(chatContentSeen, role, text) {
			return
		}
		rememberCodexChatContent(chatContentSeen, role, text)
		appendMessage(Message{
			ID:        "codex-" + itemID + ":" + normalizeCodexKind(role),
			Type:      normalizeCodexKind(role),
			Role:      normalizeCodexKind(role),
			Text:      text,
			CreatedAt: createdAt,
		}, role, text)
	}
}

func codexArgsMap(raw any) map[string]any {
	switch value := raw.(type) {
	case map[string]any:
		return value
	case string:
		var parsed map[string]any
		if json.Unmarshal([]byte(value), &parsed) == nil {
			return parsed
		}
		return map[string]any{"arguments": value}
	default:
		if raw == nil {
			return nil
		}
		return map[string]any{"arguments": raw}
	}
}

func codexFunctionToolName(name string) string {
	switch normalizeCodexKind(name) {
	case "execcommand", "shellcommand", "bash", "terminalcommand":
		return "Bash"
	case "applypatch", "filechange", "edit":
		return "FileChange"
	case "websearch", "websearchcall":
		return "WebSearch"
	case "requestuserinput", "askuserquestion":
		return "AskUserQuestion"
	default:
		if strings.TrimSpace(name) == "" {
			return "Tool"
		}
		return name
	}
}

func codexFunctionCallText(toolName string, args map[string]any) string {
	switch toolName {
	case "Bash":
		return firstNonEmptyString(
			stringFrom(args, "cmd", "command", "script"),
			commandText(args["command"]),
		)
	case "FileChange":
		return "Editing files"
	case "WebSearch":
		return firstNonEmptyString(stringFrom(args, "query"), "Searching the web")
	case "AskUserQuestion":
		return firstNonEmptyString(stringFrom(args, "question", "prompt"), "Codex needs input")
	default:
		return toolName
	}
}

func codexFunctionOutputText(item map[string]any) string {
	output := stringFrom(item, "output", "aggregated_output", "aggregatedOutput", "stdout")
	if output != "" {
		return output
	}
	if raw := item["output"]; raw != nil {
		return compactAny(raw)
	}
	return ""
}

func codexCombinedOutput(raw map[string]any) string {
	var parts []string
	for _, key := range []string{"stdout", "stderr"} {
		if text := strings.TrimSpace(stringFrom(raw, key)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func codexReasoningText(item map[string]any) string {
	var parts []string
	if text := stringFrom(item, "text", "summary_text"); text != "" {
		parts = append(parts, text)
	}
	if summary, ok := item["summary"].([]any); ok {
		for _, raw := range summary {
			switch value := raw.(type) {
			case string:
				if strings.TrimSpace(value) != "" {
					parts = append(parts, value)
				}
			case map[string]any:
				if text := stringFrom(value, "text", "summary", "content"); text != "" {
					parts = append(parts, text)
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func codexMessageText(item map[string]any) string {
	if text := stringFrom(item, "text", "message"); text != "" {
		return text
	}
	content, ok := item["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, raw := range content {
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				parts = append(parts, value)
			}
		case map[string]any:
			if normalizeCodexKind(stringFrom(value, "type")) != "" && normalizeCodexKind(stringFrom(value, "type")) != "text" && normalizeCodexKind(stringFrom(value, "type")) != "outputtext" {
				continue
			}
			if text := stringFrom(value, "text"); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func normalizeCodexKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func rememberCodexChatContent(seen map[string]bool, role string, text string) {
	if seen == nil {
		return
	}
	key := codexChatContentKey(role, text)
	if key != "" {
		seen[key] = true
	}
}

func codexChatContentSeen(seen map[string]bool, role string, text string) bool {
	if seen == nil {
		return false
	}
	return seen[codexChatContentKey(role, text)]
}

func codexChatContentKey(role string, text string) string {
	role = normalizeCodexKind(role)
	text = normalizePromptText(text)
	if role == "" || text == "" {
		return ""
	}
	return role + "\x00" + text
}

func isPlanLikeText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "\"plan\"") && strings.Contains(lower, "\"step\"") {
		return true
	}
	if strings.Contains(lower, "### steps") || strings.Contains(lower, "## plan") || strings.HasPrefix(lower, "plan:") {
		return true
	}
	stepLines := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			stepLines++
		}
		if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && strings.Contains(trimmed[:minInt(len(trimmed), 4)], ".") {
			stepLines++
		}
	}
	return stepLines >= 2
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func readSessionMeta(path string) (codexSessionMeta, bool) {
	meta, _, ok := readSessionMetaWithTimestamp(path, time.Time{})
	return meta, ok
}

func readSessionMetaWithTimestamp(path string, fallback time.Time) (codexSessionMeta, time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}, time.Time{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)
	var meta codexSessionMeta
	var startedAt time.Time
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
				return codexSessionMeta{}, time.Time{}, false
			}
			meta.ID = firstNonEmptyString(meta.ID, sessionMeta.ID)
			meta.CWD = firstNonEmptyString(meta.CWD, sessionMeta.CWD)
			meta.Model = firstNonEmptyString(meta.Model, sessionMeta.Model)
			meta.ReasoningEffort = firstNonEmptyString(meta.ReasoningEffort, sessionMeta.ReasoningEffort)
			meta.CollaborationMode = firstNonEmptyString(meta.CollaborationMode, sessionMeta.CollaborationMode)
			if startedAt.IsZero() {
				startedAt = parseCodexTimestamp(entry.Timestamp)
			}
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
	if startedAt.IsZero() {
		startedAt = fallback
	}
	return meta, startedAt, meta.ID != ""
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

func setTmuxOption(name string, option string, value string) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(option) == "" {
		return
	}
	_ = exec.Command("tmux", "set-option", "-t", name, option, value).Run()
}

func tmuxSessionNames() []string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func tmuxSessionStartTime(name string) time.Time {
	if value := tmuxOption(name, "@orion_started_at_unix_nano"); value != "" {
		if nanos, err := strconv.ParseInt(value, 10, 64); err == nil && nanos > 0 {
			return time.Unix(0, nanos)
		}
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", name, "#{session_created}").Output()
	if err != nil {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

func parseCodexTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
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

func mergeTranscriptOptions(s *Session, meta codexSessionMeta) {
	if strings.TrimSpace(meta.CWD) != "" {
		s.workspacePath = meta.CWD
	}
	if strings.TrimSpace(meta.Model) != "" {
		s.model = meta.Model
	}
	if strings.TrimSpace(meta.ReasoningEffort) != "" {
		s.reasoningEffort = meta.ReasoningEffort
	}
	if strings.TrimSpace(meta.CollaborationMode) != "" {
		s.collaborationMode = meta.CollaborationMode
	}
	if strings.TrimSpace(s.reasoningEffort) == "" {
		s.reasoningEffort = defaultReasoningEffort
	}
	if strings.TrimSpace(s.approvalPolicy) == "" {
		s.approvalPolicy = defaultApprovalPolicy
	}
	if strings.TrimSpace(s.sandboxMode) == "" {
		s.sandboxMode = defaultSandboxMode
	}
	if strings.TrimSpace(s.collaborationMode) == "" {
		s.collaborationMode = defaultCollabMode
	}
}

type codexTranscriptCandidate struct {
	path      string
	modTime   time.Time
	startedAt time.Time
}

func latestTranscriptForWorkspace(workspacePath string, notBefore time.Time) string {
	candidates := transcriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].path
}

func validStampedTranscriptPath(path string, workspacePath string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	if workspacePath == "" {
		return path
	}
	meta, ok := readSessionMeta(path)
	if ok && meta.CWD != "" && !samePath(meta.CWD, workspacePath) {
		return ""
	}
	return path
}

func stampTmuxTranscript(tmuxSession string, workspacePath string, threadID string, transcriptPath string) {
	if strings.TrimSpace(tmuxSession) == "" {
		return
	}
	setTmuxOption(tmuxSession, "@orion_type", Provider)
	setTmuxOption(tmuxSession, "@orion_provider", Provider)
	if strings.TrimSpace(workspacePath) != "" {
		setTmuxOption(tmuxSession, "@orion_workspace", workspacePath)
	}
	if strings.TrimSpace(threadID) != "" {
		setTmuxOption(tmuxSession, "@orion_thread_id", threadID)
	}
	if strings.TrimSpace(transcriptPath) != "" {
		setTmuxOption(tmuxSession, "@orion_transcript_path", transcriptPath)
	}
}

func firstTranscriptForWorkspace(workspacePath string, notBefore time.Time) string {
	candidates := transcriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].startedAt.Before(candidates[j].startedAt)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].path
}

func transcriptMatchingPrompt(workspacePath string, hints []transcriptHint) string {
	if len(hints) == 0 {
		return ""
	}
	notBefore := hints[0].After
	for _, hint := range hints[1:] {
		if hint.After.Before(notBefore) {
			notBefore = hint.After
		}
	}
	candidates := transcriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	for _, candidate := range candidates {
		for _, hint := range hints {
			if transcriptHasUserText(candidate.path, workspacePath, hint.Text, hint.After) {
				return candidate.path
			}
		}
	}
	return ""
}

func transcriptCandidatesForWorkspace(workspacePath string, notBefore time.Time) []codexTranscriptCandidate {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return nil
	}
	var candidates []codexTranscriptCandidate
	for _, file := range codexSessionFiles() {
		meta, startedAt, ok := readSessionMetaWithTimestamp(file.path, file.modTime)
		if !ok || !samePath(meta.CWD, workspacePath) {
			continue
		}
		if startedAt.IsZero() {
			startedAt = file.modTime
		}
		if !notBefore.IsZero() && file.modTime.Before(notBefore) && startedAt.Before(notBefore) {
			continue
		}
		candidates = append(candidates, codexTranscriptCandidate{path: file.path, modTime: file.modTime, startedAt: startedAt})
	}
	return candidates
}

func transcriptHasUserText(path string, workspacePath string, text string, after time.Time) bool {
	text = normalizePromptText(text)
	if text == "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry codexLogEntry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if !after.IsZero() && entry.Timestamp != "" {
			if ts := parseCodexTimestamp(entry.Timestamp); !ts.IsZero() && ts.Before(after) {
				continue
			}
		}
		switch entry.Type {
		case "session_meta":
			var meta codexSessionMeta
			if json.Unmarshal(entry.Payload, &meta) == nil && workspacePath != "" && meta.CWD != "" && !samePath(meta.CWD, workspacePath) {
				return false
			}
		case "event_msg":
			var event codexEventPayload
			if json.Unmarshal(entry.Payload, &event) != nil || event.Type != "user_message" {
				continue
			}
			if promptTextMatches(event.Message, text) {
				return true
			}
		case "response_item":
			var item map[string]any
			if json.Unmarshal(entry.Payload, &item) != nil {
				continue
			}
			if normalizeCodexKind(stringFrom(item, "type")) != "message" || normalizeCodexKind(stringFrom(item, "role")) != "user" {
				continue
			}
			if promptTextMatches(codexMessageText(item), text) {
				return true
			}
		}
	}
	return false
}

func normalizePromptText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func promptTextMatches(candidate string, normalizedPrompt string) bool {
	candidate = normalizePromptText(candidate)
	if candidate == "" || normalizedPrompt == "" {
		return false
	}
	return candidate == normalizedPrompt || strings.Contains(candidate, normalizedPrompt) || strings.Contains(normalizedPrompt, candidate)
}

func hasTmuxSession(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

func tmuxCurrentPath(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sendTextToTmux(tmuxSession string, text string) error {
	bufferName := "orion-codex-chat-" + shortID()
	load := exec.Command("tmux", "load-buffer", "-b", bufferName, "-")
	load.Stdin = strings.NewReader(text)
	if out, err := load.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux load-buffer failed: %v %s", err, strings.TrimSpace(string(out)))
	}
	defer exec.Command("tmux", "delete-buffer", "-b", bufferName).Run()

	if out, err := exec.Command("tmux", "paste-buffer", "-t", tmuxSession, "-b", bufferName).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux paste-buffer failed: %v %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("tmux", "send-keys", "-t", tmuxSession, "Enter").CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys failed: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func codexPromptWithAttachments(text string, attachments []chatattachments.Attachment) string {
	text = strings.TrimSpace(text)
	if len(attachments) == 0 {
		return text
	}
	var builder strings.Builder
	if text != "" {
		builder.WriteString(text)
		builder.WriteString("\n\n")
	} else {
		builder.WriteString("Please inspect the attached image")
		if len(attachments) > 1 {
			builder.WriteString("s")
		}
		builder.WriteString(".\n\n")
	}
	builder.WriteString("Attached image")
	if len(attachments) > 1 {
		builder.WriteString("s")
	}
	builder.WriteString(" on this machine:\n")
	for i, attachment := range attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = filepath.Base(attachment.Path)
		}
		builder.WriteString(fmt.Sprintf("%d. %s", i+1, attachment.Path))
		if name != "" {
			builder.WriteString(" (")
			builder.WriteString(name)
			builder.WriteString(")")
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
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
		switch messages[i].Type {
		case "assistant", "user", "plan":
		default:
			continue
		}
		text := strings.Join(strings.Fields(messages[i].Text), " ")
		if messages[i].Type == "plan" {
			text = strings.Join(strings.Fields(firstNonEmptyString(messages[i].Details, messages[i].Text)), " ")
		}
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

func historyVisibleMessageCount(messages []Message) int {
	count := 0
	for _, msg := range messages {
		switch msg.Type {
		case "assistant", "user", "plan":
			count++
		}
	}
	return count
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
