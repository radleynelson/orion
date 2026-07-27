package claudesdk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"orion/internal/chatattachments"
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
	Subtype     string          `json:"subtype"`
	Content     json.RawMessage `json:"content"`
	Message     json.RawMessage `json:"message"`
	Permission  string          `json:"permissionMode"`
}

type historyTranscriptMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type claudeTranscriptMeta struct {
	ID             string
	CWD            string
	Model          string
	PermissionMode string
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

func parseClaudeTranscript(path string, workspacePath string, sessionID string) ([]Message, claudeTranscriptMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, claudeTranscriptMeta{}, err
	}
	defer f.Close()

	var messages []Message
	var meta claudeTranscriptMeta
	toolNames := map[string]string{}
	lineNumber := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec historyTranscriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.SessionID != "" {
			meta.ID = rec.SessionID
		}
		if rec.CWD != "" {
			meta.CWD = rec.CWD
		}
		if rec.Permission != "" {
			meta.PermissionMode = rec.Permission
		}
		if rec.Type == "permission-mode" {
			meta.PermissionMode = rec.Permission
			continue
		}
		if rec.IsSidechain {
			continue
		}
		if workspacePath != "" && rec.CWD != "" && !sameClaudeHistoryPath(rec.CWD, workspacePath) {
			continue
		}

		baseID := rec.UUID
		if baseID == "" {
			baseID = fmt.Sprintf("%s:%d", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), lineNumber)
		}
		createdAt := historyTimestamp(rec.Timestamp)

		switch rec.Type {
		case "user":
			var msg historyTranscriptMessage
			if err := json.Unmarshal(rec.Message, &msg); err != nil {
				continue
			}
			for _, parsed := range parseClaudeUserMessage(baseID, createdAt, msg.Content, toolNames) {
				messages = append(messages, withClaudeSession(parsed, sessionID, meta.ID))
			}
		case "assistant":
			var msg historyTranscriptMessage
			if err := json.Unmarshal(rec.Message, &msg); err != nil {
				continue
			}
			if msg.Model != "" {
				meta.Model = msg.Model
			}
			for _, parsed := range parseClaudeAssistantMessage(baseID, createdAt, msg.Content, toolNames) {
				messages = append(messages, withClaudeSession(parsed, sessionID, meta.ID))
			}
		case "system":
			text := strings.TrimSpace(rawJSONString(rec.Content))
			if text != "" {
				messages = append(messages, Message{
					ID:        "claude-" + baseID + ":system",
					SessionID: sessionID,
					ThreadID:  meta.ID,
					Type:      "system",
					Subtype:   rec.Subtype,
					Text:      text,
					CreatedAt: createdAt,
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, meta, err
	}
	return messages, meta, nil
}

func parseClaudeAssistantMessage(baseID string, createdAt string, raw json.RawMessage, toolNames map[string]string) []Message {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		text := strings.TrimSpace(direct)
		if text == "" {
			return nil
		}
		return []Message{{
			ID:        "claude-" + baseID + ":assistant",
			Type:      "assistant",
			Role:      "assistant",
			Text:      text,
			CreatedAt: createdAt,
		}}
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var messages []Message
	var texts []string
	for index, block := range blocks {
		blockType := normalize(stringValue(block["type"]))
		switch blockType {
		case "text":
			if text := strings.TrimSpace(stringValue(block["text"])); text != "" {
				texts = append(texts, text)
			}
		case "thinking":
			if thinking := strings.TrimSpace(stringValue(block["thinking"])); thinking != "" {
				messages = append(messages, Message{
					ID:        fmt.Sprintf("claude-%s:thinking:%d", baseID, index),
					Type:      "thinking_delta",
					Text:      thinking,
					CreatedAt: createdAt,
				})
			}
		case "tooluse":
			toolUseID := strings.TrimSpace(stringValue(block["id"]))
			if toolUseID == "" {
				toolUseID = fmt.Sprintf("%s:tool:%d", baseID, index)
			}
			toolName := strings.TrimSpace(stringValue(block["name"]))
			if toolName == "" {
				toolName = "Tool"
			}
			input := mapValue(block["input"])
			toolNames[toolUseID] = toolName
			switch normalize(toolName) {
			case "exitplanmode":
				plan := strings.TrimSpace(stringValue(input["plan"]))
				if plan == "" {
					continue
				}
				messages = append(messages, Message{
					ID:        "claude-" + toolUseID + ":plan",
					Type:      "plan",
					Subtype:   "waiting_approval",
					ToolUseID: toolUseID,
					ToolName:  toolName,
					Text:      planTitle(plan),
					Details:   plan,
					PlanPath:  firstNonEmpty(stringValue(input["planFilePath"]), stringValue(input["plan_path"])),
					Status:    "waiting_approval",
					CreatedAt: createdAt,
				})
			case "askuserquestion":
				messages = append(messages, Message{
					ID:        "claude-" + toolUseID + ":question",
					Type:      "permission_request",
					ToolUseID: toolUseID,
					ToolName:  toolName,
					Text:      claudeQuestionText(input),
					Details:   compactAny(input),
					CreatedAt: createdAt,
				})
			default:
				if isPlanPlumbingTool(toolName, input) {
					continue
				}
				messages = append(messages, Message{
					ID:        "claude-" + toolUseID + ":tool",
					Type:      "tool",
					ToolUseID: toolUseID,
					ToolName:  toolName,
					Text:      toolName,
					Details:   compactAny(input),
					CreatedAt: createdAt,
				})
			}
		}
	}
	if len(texts) > 0 {
		messages = append(messages, Message{
			ID:        "claude-" + baseID + ":assistant",
			Type:      "assistant",
			Role:      "assistant",
			Text:      strings.Join(texts, ""),
			CreatedAt: createdAt,
		})
	}
	return messages
}

func parseClaudeUserMessage(baseID string, createdAt string, raw json.RawMessage, toolNames map[string]string) []Message {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		text := strings.TrimSpace(direct)
		if text == "" {
			return nil
		}
		return []Message{{
			ID:        "claude-" + baseID + ":user",
			Type:      "user",
			Role:      "user",
			Text:      text,
			CreatedAt: createdAt,
		}}
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var messages []Message
	var texts []string
	var attachments []chatattachments.Attachment
	for index, block := range blocks {
		switch normalize(stringValue(block["type"])) {
		case "text":
			if text := strings.TrimSpace(stringValue(block["text"])); text != "" {
				texts = append(texts, text)
			}
		case "toolresult":
			toolUseID := strings.TrimSpace(stringValue(block["tool_use_id"]))
			toolName := firstNonEmpty(toolNames[toolUseID], "Tool")
			messages = append(messages, Message{
				ID:        fmt.Sprintf("claude-%s:%s:tool_result:%d", baseID, firstNonEmpty(toolUseID, "tool"), index),
				Type:      "tool_result",
				ToolUseID: toolUseID,
				ToolName:  toolName,
				Text:      summarizeClaudeToolResult(block["content"]),
				Details:   compactAny(block["content"]),
				CreatedAt: createdAt,
			})
		case "image":
			mimeType := ""
			if source := mapValue(block["source"]); source != nil {
				mimeType = stringValue(source["media_type"])
			}
			attachments = append(attachments, chatattachments.Attachment{Name: "Image", MIMEType: mimeType})
		}
	}
	if len(texts) > 0 || len(attachments) > 0 {
		messages = append(messages, Message{
			ID:          "claude-" + baseID + ":user",
			Type:        "user",
			Role:        "user",
			Text:        strings.Join(texts, "\n\n"),
			Attachments: attachments,
			CreatedAt:   createdAt,
		})
	}
	return messages
}

func withClaudeSession(msg Message, sessionID string, threadID string) Message {
	out := msg
	out.SessionID = sessionID
	if out.ThreadID == "" {
		out.ThreadID = threadID
	}
	return out
}

func rawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return ""
}

func claudeQuestionText(input map[string]any) string {
	if summary := claudeQuestionSummary(input); summary != "" {
		return summary
	}
	return firstNonEmpty(
		stringValue(input["question"]),
		stringValue(input["prompt"]),
		stringValue(input["message"]),
		"Claude needs input",
	)
}

func claudeQuestionSummary(input map[string]any) string {
	rawQuestions, ok := input["questions"].([]any)
	if !ok {
		return ""
	}
	var lines []string
	for _, rawQuestion := range rawQuestions {
		question, ok := rawQuestion.(map[string]any)
		if !ok {
			continue
		}
		header := strings.TrimSpace(stringValue(question["header"]))
		text := strings.TrimSpace(stringValue(question["question"]))
		switch {
		case header != "" && text != "":
			lines = append(lines, header+": "+text)
		case text != "":
			lines = append(lines, text)
		case header != "":
			lines = append(lines, header)
		}
		rawOptions, _ := question["options"].([]any)
		var labels []string
		for _, rawOption := range rawOptions {
			option, ok := rawOption.(map[string]any)
			if !ok {
				continue
			}
			if label := strings.TrimSpace(stringValue(option["label"])); label != "" {
				labels = append(labels, label)
			}
		}
		if len(labels) > 0 {
			lines = append(lines, "Options: "+strings.Join(labels, ", "))
		}
	}
	return strings.Join(lines, "\n")
}

func summarizeClaudeToolResult(raw any) string {
	switch value := raw.(type) {
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return "Tool completed"
		}
		return compactClaudeText(text, 240)
	case []any:
		var parts []string
		for _, item := range value {
			if block, ok := item.(map[string]any); ok && normalize(stringValue(block["type"])) == "text" {
				if text := strings.TrimSpace(stringValue(block["text"])); text != "" {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			return compactClaudeText(strings.Join(parts, "\n\n"), 240)
		}
	}
	return "Tool completed"
}

func compactClaudeText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit-3] + "..."
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

func claudeTranscriptPath(workspacePath string, threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	path := filepath.Join(userClaudeProjectDir(workspacePath), threadID+".jsonl")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path
}

func validStampedClaudeTranscriptPath(path string, workspacePath string) string {
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
	thread := loadHistoryThread(path, workspacePath)
	if thread.WorkspacePath != "" && !sameClaudeHistoryPath(thread.WorkspacePath, workspacePath) {
		return ""
	}
	return path
}

func stampClaudeTmuxTranscript(tmuxSession string, workspacePath string, threadID string, transcriptPath string) {
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

type claudeTranscriptCandidate struct {
	path      string
	modTime   time.Time
	startedAt time.Time
}

func latestClaudeTranscriptForWorkspace(workspacePath string, notBefore time.Time) string {
	candidates := claudeTranscriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].path
}

func firstClaudeTranscriptForWorkspace(workspacePath string, notBefore time.Time) string {
	candidates := claudeTranscriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].startedAt.Before(candidates[j].startedAt)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].path
}

func claudeTranscriptMatchingPrompt(workspacePath string, hints []claudeTranscriptHint) string {
	if len(hints) == 0 {
		return ""
	}
	notBefore := hints[0].After
	for _, hint := range hints[1:] {
		if hint.After.Before(notBefore) {
			notBefore = hint.After
		}
	}
	candidates := claudeTranscriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	for _, candidate := range candidates {
		for _, hint := range hints {
			if claudeTranscriptHasUserText(candidate.path, workspacePath, hint.Text, hint.After) {
				return candidate.path
			}
		}
	}
	return ""
}

func claudeTranscriptCandidatesForWorkspace(workspacePath string, notBefore time.Time) []claudeTranscriptCandidate {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return nil
	}
	var candidates []claudeTranscriptCandidate
	for _, file := range claudeHistoryFiles(workspacePath) {
		startedAt := firstClaudeTranscriptTimestamp(file.path, file.modTime)
		if !notBefore.IsZero() && file.modTime.Before(notBefore) && startedAt.Before(notBefore) {
			continue
		}
		candidates = append(candidates, claudeTranscriptCandidate{path: file.path, modTime: file.modTime, startedAt: startedAt})
	}
	return candidates
}

func firstClaudeTranscriptTimestamp(path string, fallback time.Time) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec historyTranscriptRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if ts := parseClaudeTimestamp(rec.Timestamp); !ts.IsZero() {
			return ts
		}
	}
	return fallback
}

func claudeTranscriptHasUserText(path string, workspacePath string, text string, after time.Time) bool {
	text = normalizeClaudePromptText(text)
	if text == "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec historyTranscriptRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if !after.IsZero() {
			if ts := parseClaudeTimestamp(rec.Timestamp); !ts.IsZero() && ts.Before(after) {
				continue
			}
		}
		if workspacePath != "" && rec.CWD != "" && !sameClaudeHistoryPath(rec.CWD, workspacePath) {
			return false
		}
		if rec.Type != "user" || len(rec.Message) == 0 || string(rec.Message) == "null" {
			continue
		}
		var msg historyTranscriptMessage
		if json.Unmarshal(rec.Message, &msg) != nil {
			continue
		}
		if claudePromptTextMatches(historyTextFromContent(rec.Type, msg.Content), text) {
			return true
		}
	}
	return false
}

func normalizeClaudePromptText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func claudePromptTextMatches(candidate string, normalizedPrompt string) bool {
	candidate = normalizeClaudePromptText(candidate)
	if candidate == "" || normalizedPrompt == "" {
		return false
	}
	return candidate == normalizedPrompt || strings.Contains(candidate, normalizedPrompt) || strings.Contains(normalizedPrompt, candidate)
}

func parseClaudeTimestamp(value string) time.Time {
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
