package claudechat

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"orion/internal/chatattachments"
)

const (
	// SessionType is the UI view type. The underlying process is a normal
	// Claude terminal session, whose session type is TerminalSessionType.
	SessionType         = "claude-chat"
	TerminalSessionType = "claude"
)

type SessionInfo struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Label         string `json:"label"`
	WorkspacePath string `json:"workspacePath"`
	Status        string `json:"status"`
	ThreadID      string `json:"threadId,omitempty"`
}

type Message struct {
	ID          string                       `json:"id"`
	SessionID   string                       `json:"sessionId"`
	ThreadID    string                       `json:"threadId,omitempty"`
	Type        string                       `json:"type"`
	Subtype     string                       `json:"subtype,omitempty"`
	Role        string                       `json:"role,omitempty"`
	Text        string                       `json:"text,omitempty"`
	Status      string                       `json:"status,omitempty"`
	ToolUseID   string                       `json:"toolUseId,omitempty"`
	ToolName    string                       `json:"toolName,omitempty"`
	Details     string                       `json:"details,omitempty"`
	PlanPath    string                       `json:"planPath,omitempty"`
	Attachments []chatattachments.Attachment `json:"attachments,omitempty"`
	CreatedAt   string                       `json:"createdAt"`
}

type Listener func(sessionID string, message Message)

type Manager struct {
	ctx context.Context

	mu       sync.RWMutex
	sessions map[string]*Session
	listener Listener
}

func NewManager() *Manager {
	return &Manager{
		ctx:      context.Background(),
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.ctx = ctx
}

func (m *Manager) SetListener(listener Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listener = listener
}

// Start is kept for older callers, but Claude chat no longer starts a separate
// headless Claude process. Launch a normal Claude terminal and Attach it.
func (m *Manager) Start(workspacePath string, label string) (*SessionInfo, error) {
	return nil, errors.New("claude chat now attaches to a Claude terminal session")
}

func (m *Manager) Attach(tmuxSession string, workspacePath string, label string) (*SessionInfo, error) {
	return m.AttachSince(tmuxSession, workspacePath, label, time.Time{})
}

func (m *Manager) AttachSince(tmuxSession string, workspacePath string, label string, transcriptNotBefore time.Time) (*SessionInfo, error) {
	tmuxSession = strings.TrimSpace(tmuxSession)
	workspacePath = strings.TrimSpace(workspacePath)
	if tmuxSession == "" {
		return nil, errors.New("tmuxSession required")
	}
	if strings.TrimSpace(label) == "" {
		label = "Claude"
	}
	if !hasTmuxSession(tmuxSession) {
		return nil, fmt.Errorf("tmux session not found: %s", tmuxSession)
	}
	if workspacePath == "" {
		workspacePath = tmuxCurrentPath(tmuxSession)
	}

	m.mu.Lock()
	if existing, ok := m.sessions[tmuxSession]; ok {
		existing.mu.Lock()
		if workspacePath != "" {
			existing.workspacePath = workspacePath
		}
		if !transcriptNotBefore.IsZero() {
			existing.transcriptNotBefore = transcriptNotBefore
		}
		existing.label = label
		existing.mu.Unlock()
		info := existing.Info()
		m.mu.Unlock()
		_ = existing.Sync()
		return &info, nil
	}

	ctx, cancel := context.WithCancel(m.ctx)
	session := &Session{
		manager:             m,
		ctx:                 ctx,
		cancel:              cancel,
		id:                  tmuxSession,
		tmuxSession:         tmuxSession,
		label:               label,
		workspacePath:       workspacePath,
		status:              "idle",
		transcriptNotBefore: transcriptNotBefore,
		subscribers:         make(map[chan Message]struct{}),
		seen:                make(map[string]bool),
	}
	m.sessions[tmuxSession] = session
	m.mu.Unlock()

	go session.pollLoop()
	_ = session.Sync()

	info := session.Info()
	return &info, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *Manager) List(workspacePaths []string) []SessionInfo {
	pathSet := make(map[string]bool)
	for _, path := range workspacePaths {
		if path != "" {
			pathSet[path] = true
		}
	}

	m.mu.RLock()
	var infos []SessionInfo
	var stale []string
	for id, session := range m.sessions {
		if session.tmuxSession != "" && !hasTmuxSession(session.tmuxSession) {
			stale = append(stale, id)
			continue
		}
		info := session.Info()
		if len(pathSet) > 0 && !pathSet[info.WorkspacePath] {
			continue
		}
		infos = append(infos, info)
	}
	m.mu.RUnlock()

	if len(stale) > 0 {
		m.mu.Lock()
		for _, id := range stale {
			if session, ok := m.sessions[id]; ok && session.tmuxSession != "" && !hasTmuxSession(session.tmuxSession) {
				session.detach()
				delete(m.sessions, id)
			}
		}
		m.mu.Unlock()
	}

	return infos
}

func (m *Manager) Stop(id string) error {
	session, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("claude chat session not found: %s", id)
	}
	return session.Stop()
}

func (m *Manager) DetachAll() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, session := range sessions {
		session.detach()
	}
}

func (m *Manager) SyncTmux(tmuxSession string) {
	if session, ok := m.Get(tmuxSession); ok {
		_ = session.Sync()
	}
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func (m *Manager) emit(sessionID string, msg Message) {
	m.mu.RLock()
	listener := m.listener
	m.mu.RUnlock()
	if listener != nil {
		listener(sessionID, msg)
	}
}

type Session struct {
	manager *Manager
	ctx     context.Context
	cancel  context.CancelFunc

	mu                  sync.RWMutex
	id                  string
	tmuxSession         string
	label               string
	workspacePath       string
	threadID            string
	status              string
	statusText          string
	transcriptPath      string
	transcriptNotBefore time.Time
	transcriptHints     []transcriptHint

	messagesMu sync.Mutex
	messages   []Message
	seen       map[string]bool

	subscribersMu sync.Mutex
	subscribers   map[chan Message]struct{}
}

type transcriptHint struct {
	Text  string
	After time.Time
}

func (s *Session) Info() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SessionInfo{
		ID:            s.id,
		Type:          TerminalSessionType,
		Label:         s.label,
		WorkspacePath: s.workspacePath,
		Status:        s.status,
		ThreadID:      s.threadID,
	}
}

func (s *Session) Messages() []Message {
	s.messagesMu.Lock()
	defer s.messagesMu.Unlock()
	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out
}

func (s *Session) Subscribe() (<-chan Message, func()) {
	ch := make(chan Message, 128)
	s.subscribersMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscribersMu.Unlock()
	cancel := func() {
		s.subscribersMu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.subscribersMu.Unlock()
	}
	return ch, cancel
}

func (s *Session) Send(text string, attachments []chatattachments.Attachment) error {
	text = strings.TrimSpace(text)
	if text == "" && len(attachments) == 0 {
		return nil
	}
	text = claudePromptWithAttachments(text, attachments)
	s.addTranscriptHint(text, time.Now().Add(-2*time.Second))
	s.setStatusText("running", "Claude is thinking")
	if err := sendTextToTmux(s.tmuxSession, text); err != nil {
		s.emit(Message{Type: "error", Text: err.Error()})
		s.setStatus("idle")
		return err
	}
	return nil
}

func (s *Session) Answer(toolUseID string, result string) error {
	if strings.TrimSpace(result) == "" {
		return nil
	}
	s.emit(Message{Type: "permission_resolved", ToolUseID: toolUseID, ToolName: "Question", Text: "Answered"})
	return s.Send(result, nil)
}

func (s *Session) ApprovePlan() error {
	if strings.TrimSpace(s.tmuxSession) == "" {
		return errors.New("tmux session required")
	}
	s.setStatusText("running", "Claude is starting the plan")
	if out, err := exec.Command("tmux", "send-keys", "-t", s.tmuxSession, "Enter").CombinedOutput(); err != nil {
		s.setStatus("waiting_input")
		return fmt.Errorf("tmux approve plan failed: %v %s", err, strings.TrimSpace(string(out)))
	}
	s.emit(Message{Type: "plan_resolved", Text: "Plan approved"})
	return nil
}

func (s *Session) Stop() error {
	s.detach()
	if s.tmuxSession != "" {
		_ = exec.Command("tmux", "kill-session", "-t", s.tmuxSession).Run()
	}
	s.setStatus("stopped")
	s.manager.remove(s.id)
	return nil
}

func (s *Session) detach() {
	s.cancel()
}

func (s *Session) pollLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.Sync()
			s.emitRunningActivity()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Session) Sync() error {
	s.mu.RLock()
	workspacePath := s.workspacePath
	transcriptPath := s.transcriptPath
	transcriptNotBefore := s.transcriptNotBefore
	s.mu.RUnlock()

	if workspacePath == "" {
		workspacePath = tmuxCurrentPath(s.tmuxSession)
		if workspacePath != "" {
			s.mu.Lock()
			s.workspacePath = workspacePath
			s.mu.Unlock()
		}
	}

	if transcriptPath == "" {
		if path := transcriptForTmuxResume(s.tmuxSession, workspacePath); path != "" {
			transcriptPath = path
		} else if path := transcriptMatchingPrompt(workspacePath, s.transcriptHintsSnapshot()); path != "" {
			transcriptPath = path
			s.clearTranscriptHints()
		} else if !transcriptNotBefore.IsZero() {
			transcriptPath = firstTranscriptForWorkspace(workspacePath, transcriptNotBefore)
		}
	}
	if transcriptPath != "" {
		s.mu.Lock()
		s.transcriptPath = transcriptPath
		s.mu.Unlock()
	}
	if transcriptPath == "" {
		return nil
	}

	messages, threadID, err := parseTranscript(transcriptPath, workspacePath, s.id)
	if err != nil {
		return err
	}
	if threadID != "" {
		s.mu.Lock()
		s.threadID = threadID
		s.mu.Unlock()
	}

	sawAssistant := false
	sawPlan := false
	for _, msg := range messages {
		if s.markSeen(msg.ID) {
			s.emit(msg)
			if msg.Type == "assistant" {
				sawAssistant = true
			}
			if msg.Type == "plan" {
				sawPlan = true
			}
		}
	}
	if sawPlan {
		s.setStatus("waiting_input")
	} else if sawAssistant {
		s.emit(Message{Type: "result", Subtype: "completed"})
		s.setStatus("idle")
	}
	return nil
}

func (s *Session) markSeen(id string) bool {
	s.messagesMu.Lock()
	defer s.messagesMu.Unlock()
	if s.seen[id] {
		return false
	}
	s.seen[id] = true
	return true
}

func (s *Session) addTranscriptHint(text string, after time.Time) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.mu.Lock()
	s.transcriptHints = append(s.transcriptHints, transcriptHint{Text: text, After: after})
	if len(s.transcriptHints) > 8 {
		s.transcriptHints = s.transcriptHints[len(s.transcriptHints)-8:]
	}
	s.mu.Unlock()
}

func (s *Session) transcriptHintsSnapshot() []transcriptHint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]transcriptHint, len(s.transcriptHints))
	copy(out, s.transcriptHints)
	return out
}

func (s *Session) clearTranscriptHints() {
	s.mu.Lock()
	s.transcriptHints = nil
	s.mu.Unlock()
}

func (s *Session) setStatus(status string) {
	s.setStatusText(status, "")
}

func (s *Session) setStatusText(status string, text string) {
	s.mu.Lock()
	changed := s.status != status || s.statusText != text
	s.status = status
	s.statusText = text
	s.mu.Unlock()
	if changed {
		s.emit(Message{Type: "status", Status: status, Text: text})
	}
}

func (s *Session) emitRunningActivity() {
	s.mu.RLock()
	status := s.status
	current := s.statusText
	tmuxSession := s.tmuxSession
	s.mu.RUnlock()
	if status != "running" || tmuxSession == "" {
		return
	}
	text := claudeActivityFromTmux(tmuxSession)
	if text == "" {
		text = "Claude is thinking"
	}
	if text != current {
		s.setStatusText("running", text)
	}
}

func (s *Session) emit(msg Message) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if msg.ID == "" {
		msg.ID = "msg-" + shortID()
	}
	msg.SessionID = s.id
	if msg.ThreadID == "" {
		s.mu.RLock()
		msg.ThreadID = s.threadID
		s.mu.RUnlock()
	}
	if msg.CreatedAt == "" {
		msg.CreatedAt = now
	}

	s.messagesMu.Lock()
	if msg.Type == "status" && msg.Status != "" {
		s.mu.Lock()
		s.status = msg.Status
		s.mu.Unlock()
	}
	s.messages = append(s.messages, msg)
	if len(s.messages) > 1000 {
		s.messages = s.messages[len(s.messages)-1000:]
	}
	s.messagesMu.Unlock()

	s.subscribersMu.Lock()
	for ch := range s.subscribers {
		select {
		case ch <- msg:
		default:
		}
	}
	s.subscribersMu.Unlock()

	s.manager.emit(s.id, msg)
}

type transcriptRecord struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	CWD         string          `json:"cwd"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

type transcriptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func parseTranscript(path string, workspacePath string, sessionID string) ([]Message, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	var messages []Message
	var threadID string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rec transcriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.IsSidechain {
			continue
		}
		if rec.SessionID != "" {
			threadID = rec.SessionID
		}
		if workspacePath != "" && rec.CWD != "" && rec.CWD != workspacePath {
			continue
		}
		if rec.Type != "user" && rec.Type != "assistant" {
			continue
		}
		if len(rec.Message) == 0 || string(rec.Message) == "null" {
			continue
		}

		var msg transcriptMessage
		if err := json.Unmarshal(rec.Message, &msg); err != nil {
			continue
		}
		createdAt := rec.Timestamp
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		baseID := rec.UUID
		if baseID == "" {
			baseID = fmt.Sprintf("%s:%d", path, lineNo)
		}

		switch rec.Type {
		case "user":
			text := userTextFromContent(msg.Content)
			if text == "" {
				continue
			}
			messages = append(messages, Message{
				ID:        "claude-" + baseID + ":user",
				SessionID: sessionID,
				ThreadID:  rec.SessionID,
				Type:      "user",
				Role:      "user",
				Text:      text,
				CreatedAt: createdAt,
			})
		case "assistant":
			tools, text := assistantContent(msg.Content)
			for _, tool := range tools {
				tool.SessionID = sessionID
				tool.ThreadID = rec.SessionID
				tool.CreatedAt = createdAt
				if tool.ID == "" {
					tool.ID = "claude-" + baseID + ":tool:" + shortID()
				}
				messages = append(messages, tool)
			}
			if text != "" {
				messages = append(messages, Message{
					ID:        "claude-" + baseID + ":assistant",
					SessionID: sessionID,
					ThreadID:  rec.SessionID,
					Type:      "assistant",
					Role:      "assistant",
					Text:      text,
					CreatedAt: createdAt,
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, threadID, err
	}
	return messages, threadID, nil
}

func userTextFromContent(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, block := range blocks {
		if normalize(stringValue(block["type"])) != "text" {
			continue
		}
		if value := strings.TrimSpace(stringValue(block["text"])); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func assistantContent(raw json.RawMessage) ([]Message, string) {
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return nil, ""
	}

	var tools []Message
	var texts []string
	for _, block := range blocks {
		switch normalize(stringValue(block["type"])) {
		case "text":
			if text := stringValue(block["text"]); strings.TrimSpace(text) != "" {
				texts = append(texts, text)
			}
		case "tooluse":
			toolUseID := stringValue(block["id"])
			toolName := stringValue(block["name"])
			if toolName == "" {
				toolName = "Tool"
			}
			input := mapValue(block["input"])
			if normalize(toolName) == "exitplanmode" {
				plan := strings.TrimSpace(stringValue(input["plan"]))
				if plan == "" {
					plan = strings.TrimSpace(compactAny(input))
				}
				tools = append(tools, Message{
					ID:        "claude-" + toolUseID + ":plan",
					Type:      "plan",
					Subtype:   "waiting_approval",
					ToolUseID: toolUseID,
					ToolName:  toolName,
					Text:      planTitle(plan),
					Details:   plan,
					PlanPath:  stringValue(input["planFilePath"]),
					Status:    "waiting_approval",
				})
				continue
			}
			if isPlanPlumbingTool(toolName, input) {
				continue
			}
			tools = append(tools, Message{
				ID:        "claude-" + toolUseID + ":tool",
				Type:      "tool",
				ToolUseID: toolUseID,
				ToolName:  toolName,
				Text:      toolName,
				Details:   compactAny(input),
			})
		}
	}
	return tools, strings.TrimSpace(strings.Join(texts, ""))
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

func firstTranscriptForWorkspace(workspacePath string, notBefore time.Time) string {
	candidates := transcriptCandidatesForWorkspace(workspacePath, notBefore)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.Before(candidates[j].modTime)
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

type transcriptCandidate struct {
	path    string
	modTime time.Time
}

func transcriptCandidatesForWorkspace(workspacePath string, notBefore time.Time) []transcriptCandidate {
	if workspacePath == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspacePath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var candidates []transcriptCandidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !notBefore.IsZero() && info.ModTime().Before(notBefore) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if !transcriptHasWorkspace(path, workspacePath) {
			continue
		}
		candidates = append(candidates, transcriptCandidate{path: path, modTime: info.ModTime()})
	}
	return candidates
}

func transcriptHasWorkspace(path string, workspacePath string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for i := 0; i < 80 && scanner.Scan(); i++ {
		var rec transcriptRecord
		if json.Unmarshal(scanner.Bytes(), &rec) == nil && rec.CWD == workspacePath {
			return true
		}
	}
	return false
}

func transcriptHasUserText(path string, workspacePath string, text string, after time.Time) bool {
	text = strings.TrimSpace(text)
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
		var rec transcriptRecord
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		if rec.IsSidechain || rec.Type != "user" {
			continue
		}
		if workspacePath != "" && rec.CWD != "" && rec.CWD != workspacePath {
			continue
		}
		if !after.IsZero() && rec.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil && ts.Before(after) {
				continue
			}
		}
		if len(rec.Message) == 0 || string(rec.Message) == "null" {
			continue
		}
		var msg transcriptMessage
		if json.Unmarshal(rec.Message, &msg) != nil {
			continue
		}
		userText := userTextFromContent(msg.Content)
		if userText == text || strings.Contains(userText, text) {
			return true
		}
	}
	return false
}

func transcriptForTmuxResume(tmuxSession string, workspacePath string) string {
	for _, sessionID := range claudeResumeIDsForTmux(tmuxSession) {
		path := transcriptPathForSessionID(workspacePath, sessionID)
		if path == "" {
			continue
		}
		return path
	}
	return ""
}

func transcriptPathForSessionID(workspacePath string, sessionID string) string {
	workspacePath = strings.TrimSpace(workspacePath)
	sessionID = strings.TrimSpace(sessionID)
	if workspacePath == "" || sessionID == "" || strings.Contains(sessionID, string(filepath.Separator)) {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".claude", "projects", claudeProjectDir(workspacePath), sessionID+".jsonl")
	if info, err := os.Stat(path); err == nil && !info.IsDir() && transcriptHasWorkspace(path, workspacePath) {
		return path
	}
	return ""
}

func claudeResumeIDsForTmux(tmuxSession string) []string {
	panePID := tmuxPanePID(tmuxSession)
	if panePID <= 0 {
		return nil
	}
	commands := descendantProcessCommands(panePID)
	var ids []string
	seen := map[string]bool{}
	for _, command := range commands {
		for _, id := range parseClaudeResumeIDs(command) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func parseClaudeResumeIDs(command string) []string {
	fields := strings.Fields(command)
	var ids []string
	for i, field := range fields {
		switch {
		case field == "--resume" || field == "-r":
			if i+1 < len(fields) {
				ids = append(ids, strings.TrimSpace(fields[i+1]))
			}
		case strings.HasPrefix(field, "--resume="):
			ids = append(ids, strings.TrimSpace(strings.TrimPrefix(field, "--resume=")))
		}
	}
	return ids
}

func tmuxPanePID(name string) int {
	out, err := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_pid}").Output()
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return pid
}

func descendantProcessCommands(rootPID int) []string {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil
	}
	type proc struct {
		pid     int
		ppid    int
		command string
	}
	children := map[int][]proc{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		command := strings.Join(fields[2:], " ")
		children[ppid] = append(children[ppid], proc{pid: pid, ppid: ppid, command: command})
	}

	var commands []string
	queue := append([]proc(nil), children[rootPID]...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		commands = append(commands, current.command)
		queue = append(queue, children[current.pid]...)
	}
	return commands
}

func claudeProjectDir(path string) string {
	dir := strings.ReplaceAll(path, string(filepath.Separator), "-")
	return strings.ReplaceAll(dir, "_", "-")
}

func sendTextToTmux(tmuxSession string, text string) error {
	bufferName := "orion-chat-" + shortID()
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

func claudePromptWithAttachments(text string, attachments []chatattachments.Attachment) string {
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

func hasTmuxSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

func tmuxCurrentPath(name string) string {
	out, err := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func claudeActivityFromTmux(name string) string {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", name, "-S", "-40").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if activity := normalizeClaudeActivityLine(lines[i]); activity != "" {
			return activity
		}
	}
	return ""
}

func normalizeClaudeActivityLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.Join(strings.Fields(line), " ")
	if line == "" {
		return ""
	}
	lower := strings.ToLower(line)
	ignored := []string{
		"esc to interrupt",
		"shift+tab",
		"claude in chrome",
		"remote-control",
		"orion-",
		"tip:",
		"for shortcuts",
		"plan mode on",
		"bypass permissions",
		"auto mode on",
		"accept edits on",
	}
	for _, value := range ignored {
		if strings.Contains(lower, value) {
			return ""
		}
	}
	if strings.HasPrefix(line, "⎿") || strings.HasPrefix(line, "❯") || strings.HasPrefix(line, "⏺") {
		return ""
	}
	hasActivityMarker := strings.Contains(line, "…") ||
		strings.Contains(lower, "thinking with") ||
		strings.Contains(lower, "thinking") ||
		strings.Contains(lower, "running stop hook")
	if !hasActivityMarker {
		return ""
	}
	line = strings.TrimLeft(line, "·✢✳✶✻✽⠂⠐ \t")
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(line), "running stop hook") {
		return "Wrapping up"
	}
	return line
}

func stringValue(raw any) string {
	if value, ok := raw.(string); ok {
		return value
	}
	return ""
}

func mapValue(raw any) map[string]any {
	if value, ok := raw.(map[string]any); ok {
		return value
	}
	return nil
}

func normalize(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func isPlanPlumbingTool(toolName string, input map[string]any) bool {
	switch normalize(toolName) {
	case "toolsearch":
		return strings.TrimSpace(stringValue(input["query"])) == "select:ExitPlanMode"
	case "write":
		path := stringValue(input["file_path"])
		if path == "" {
			path = stringValue(input["filePath"])
		}
		return strings.Contains(path, string(filepath.Separator)+".claude"+string(filepath.Separator)+"plans"+string(filepath.Separator))
	default:
		return false
	}
}

func planTitle(plan string) string {
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "# ")
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "Plan ready"
}

func compactAny(value any) string {
	if value == nil {
		return ""
	}
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(b)
}

func shortID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
