package codexchat

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"orion/internal/chatattachments"
)

const (
	SessionType            = "codex-chat"
	defaultModel           = "gpt-5.4"
	defaultReasoningEffort = "xhigh"
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

func (m *Manager) Start(workspacePath string, label string) (*SessionInfo, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return nil, errors.New("workspacePath required")
	}
	if strings.TrimSpace(label) == "" {
		label = "Codex Chat"
	}

	id := "codex-chat-" + shortID()
	ctx, cancel := context.WithCancel(m.ctx)
	cmd := exec.CommandContext(ctx, "codex", "app-server", "--listen", "stdio://")
	cmd.Dir = workspacePath

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	session := &Session{
		manager:         m,
		ctx:             ctx,
		cancel:          cancel,
		id:              id,
		label:           label,
		workspacePath:   workspacePath,
		status:          "starting",
		cmd:             cmd,
		stdin:           stdin,
		pending:         make(map[string]chan rpcResponse),
		pendingInputs:   make(map[string]pendingInput),
		subscribers:     make(map[chan Message]struct{}),
		agentDeltaItems: make(map[string]bool),
		model:           defaultModel,
		reasoningEffort: defaultReasoningEffort,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	go session.readLoop(stdout)
	go session.stderrLoop(stderr)
	go session.wait()

	session.emit(Message{Type: "status", Status: "starting", Text: "Starting Codex chat"})

	if err := session.bootstrap(); err != nil {
		_ = session.Stop()
		return nil, err
	}

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
	defer m.mu.RUnlock()
	var infos []SessionInfo
	for _, session := range m.sessions {
		info := session.Info()
		if len(pathSet) > 0 && !pathSet[info.WorkspacePath] {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}

func (m *Manager) Stop(id string) error {
	session, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("codex chat session not found: %s", id)
	}
	return session.Stop()
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

	id              string
	label           string
	workspacePath   string
	threadID        string
	status          string
	model           string
	reasoningEffort string

	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex
	sendMu  sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan rpcResponse
	rpcSeq    int

	messagesMu sync.Mutex
	messages   []Message

	subscribersMu sync.Mutex
	subscribers   map[chan Message]struct{}

	pendingInputsMu sync.Mutex
	pendingInputs   map[string]pendingInput

	agentDeltaMu    sync.Mutex
	agentDeltaItems map[string]bool
}

type pendingInput struct {
	requestID   string
	toolUseID   string
	questionIDs []string
}

type rpcResponse struct {
	result map[string]any
	err    error
}

func (s *Session) Info() SessionInfo {
	s.messagesMu.Lock()
	defer s.messagesMu.Unlock()
	return SessionInfo{
		ID:            s.id,
		Type:          SessionType,
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

	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	if s.threadID == "" {
		return errors.New("codex thread is not initialized")
	}

	displayText := text
	if displayText == "" && len(attachments) > 0 {
		displayText = attachmentOnlyText(len(attachments))
	}
	s.emit(Message{Type: "user", Role: "user", Text: displayText, Attachments: attachments})
	s.setStatus("running")

	input := []map[string]any{}
	if text != "" {
		input = append(input, map[string]any{
			"type":          "text",
			"text":          text,
			"text_elements": []any{},
		})
	} else if len(attachments) > 0 {
		input = append(input, map[string]any{
			"type":          "text",
			"text":          attachmentOnlyText(len(attachments)),
			"text_elements": []any{},
		})
	}
	for _, attachment := range attachments {
		input = append(input, map[string]any{
			"type": "localImage",
			"path": attachment.Path,
		})
	}

	params := map[string]any{
		"threadId":       s.threadID,
		"input":          input,
		"approvalPolicy": "never",
		"effort":         s.reasoningEffort,
		"collaborationMode": map[string]any{
			"mode": "default",
			"settings": map[string]any{
				"model":                  s.model,
				"reasoning_effort":       s.reasoningEffort,
				"developer_instructions": nil,
			},
		},
	}

	if _, err := s.request("turn/start", params, 45*time.Second); err != nil {
		s.emit(Message{Type: "error", Text: err.Error()})
		s.setStatus("idle")
		return err
	}
	return nil
}

func (s *Session) Answer(toolUseID string, result string) error {
	toolUseID = strings.TrimSpace(toolUseID)
	if toolUseID == "" {
		return errors.New("toolUseId required")
	}

	s.pendingInputsMu.Lock()
	pending, ok := s.pendingInputs[toolUseID]
	if ok {
		delete(s.pendingInputs, toolUseID)
	}
	s.pendingInputsMu.Unlock()
	if !ok {
		return fmt.Errorf("pending user input not found: %s", toolUseID)
	}

	answers := make(map[string]any)
	for _, id := range pending.questionIDs {
		answers[id] = map[string]any{"answers": []string{result}}
	}
	if len(answers) == 0 {
		answers["answer"] = map[string]any{"answers": []string{result}}
	}

	if err := s.respond(pending.requestID, map[string]any{"answers": answers}); err != nil {
		return err
	}
	s.emit(Message{Type: "permission_resolved", ToolUseID: toolUseID, ToolName: "AskUserQuestion", Text: "Answered"})
	s.setStatus("running")
	return nil
}

func (s *Session) Stop() error {
	s.cancel()
	_ = s.stdin.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.setStatus("stopped")
	s.manager.remove(s.id)
	return nil
}

func (s *Session) bootstrap() error {
	if _, err := s.request("initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "orion",
			"version": "1.0.0",
			"title":   "Orion",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}, 30*time.Second); err != nil {
		return fmt.Errorf("initialize codex app-server: %w", err)
	}

	if err := s.notify("initialized", map[string]any{}); err != nil {
		return err
	}

	response, err := s.request("thread/start", map[string]any{
		"cwd":                    s.workspacePath,
		"approvalPolicy":         "never",
		"sandbox":                "danger-full-access",
		"experimentalRawEvents":  false,
		"persistExtendedHistory": true,
	}, 45*time.Second)
	if err != nil {
		return fmt.Errorf("start codex thread: %w", err)
	}

	if thread, ok := response["thread"].(map[string]any); ok {
		if id, ok := thread["id"].(string); ok {
			s.threadID = id
		}
		if model, ok := thread["model"].(string); ok && strings.TrimSpace(model) != "" {
			s.model = model
		}
	}
	if model, ok := response["model"].(string); ok && strings.TrimSpace(model) != "" {
		s.model = model
	}
	if s.threadID == "" {
		return errors.New("thread/start returned no thread id")
	}

	s.emit(Message{Type: "system", Text: "Codex chat ready", ThreadID: s.threadID})
	s.setStatus("idle")
	return nil
}

func (s *Session) request(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	s.pendingMu.Lock()
	s.rpcSeq++
	id := s.rpcSeq
	idKey := fmt.Sprintf("%d", id)
	ch := make(chan rpcResponse, 1)
	s.pending[idKey] = ch
	s.pendingMu.Unlock()

	if err := s.writeEnvelope(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		s.pendingMu.Lock()
		delete(s.pending, idKey)
		s.pendingMu.Unlock()
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case response := <-ch:
		if response.err != nil {
			return nil, response.err
		}
		return response.result, nil
	case <-timer.C:
		s.pendingMu.Lock()
		delete(s.pending, idKey)
		s.pendingMu.Unlock()
		return nil, fmt.Errorf("%s timed out", method)
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *Session) notify(method string, params map[string]any) error {
	return s.writeEnvelope(map[string]any{"method": method, "params": params})
}

func (s *Session) respond(id string, result map[string]any) error {
	return s.writeEnvelope(map[string]any{"id": id, "result": result})
}

func (s *Session) writeEnvelope(envelope map[string]any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.stdin == nil {
		return errors.New("codex app-server stdin is closed")
	}
	line, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = s.stdin.Write(line)
	return err
}

func (s *Session) readLoop(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			s.handleLine(line)
		}
		if err != nil {
			if err != io.EOF && s.ctx.Err() == nil {
				s.emit(Message{Type: "error", Text: err.Error()})
			}
			return
		}
	}
}

func (s *Session) stderrLoop(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			log.Printf("[Codex Chat %s] %s", s.id, text)
		}
	}
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	if s.ctx.Err() == nil && err != nil {
		s.emit(Message{Type: "error", Text: err.Error()})
	}
	if s.ctx.Err() == nil {
		s.setStatus("stopped")
		s.manager.remove(s.id)
	}
}

func (s *Session) handleLine(line []byte) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(line, &envelope); err != nil {
		s.emit(Message{Type: "error", Text: "Invalid Codex event: " + err.Error()})
		return
	}

	idRaw, hasID := envelope["id"]
	methodRaw, hasMethod := envelope["method"]

	if hasID && hasMethod {
		id := rawID(idRaw)
		method := rawString(methodRaw)
		params := rawParams(envelope["params"])
		s.handleServerRequest(id, method, params)
		return
	}

	if hasID {
		s.handleResponse(rawID(idRaw), envelope["result"], envelope["error"])
		return
	}

	if hasMethod {
		method := rawString(methodRaw)
		params := rawParams(envelope["params"])
		s.handleNotification(method, params)
	}
}

func (s *Session) handleResponse(id string, resultRaw json.RawMessage, errorRaw json.RawMessage) {
	s.pendingMu.Lock()
	ch := s.pending[id]
	delete(s.pending, id)
	s.pendingMu.Unlock()
	if ch == nil {
		return
	}
	if len(errorRaw) > 0 && string(errorRaw) != "null" {
		ch <- rpcResponse{err: errors.New(errorMessage(errorRaw))}
		return
	}
	result := map[string]any{}
	if len(resultRaw) > 0 && string(resultRaw) != "null" {
		_ = json.Unmarshal(resultRaw, &result)
	}
	ch <- rpcResponse{result: result}
}

func (s *Session) handleServerRequest(id string, method string, params map[string]any) {
	switch method {
	case "item/tool/requestUserInput":
		toolUseID := stringFrom(params, "itemId", "approvalId")
		if toolUseID == "" {
			toolUseID = id
		}
		questionIDs, details := summarizeQuestions(params["questions"])
		s.pendingInputsMu.Lock()
		s.pendingInputs[toolUseID] = pendingInput{
			requestID:   id,
			toolUseID:   toolUseID,
			questionIDs: questionIDs,
		}
		s.pendingInputsMu.Unlock()
		s.emit(Message{
			Type:      "permission_request",
			ToolUseID: toolUseID,
			ToolName:  "AskUserQuestion",
			Text:      "Codex needs input",
			Details:   details,
		})
		s.setStatus("waiting_input")
	case "item/permissions/requestApproval":
		permissions, _ := params["permissions"].(map[string]any)
		_ = s.respond(id, map[string]any{
			"scope":       "session",
			"permissions": permissions,
		})
		s.emit(Message{Type: "tool_result", ToolUseID: id, ToolName: "Permissions", Text: "Approved automatically"})
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		_ = s.respond(id, map[string]any{"decision": "accept"})
		s.emit(Message{Type: "tool_result", ToolUseID: id, ToolName: "Approval", Text: "Approved automatically"})
	default:
		if strings.Contains(method, "requestApproval") {
			_ = s.respond(id, map[string]any{"decision": "accept"})
			s.emit(Message{Type: "tool_result", ToolUseID: id, ToolName: "Approval", Text: "Approved automatically"})
			return
		}
		_ = s.respond(id, map[string]any{})
	}
}

func (s *Session) handleNotification(method string, params map[string]any) {
	switch method {
	case "thread/started":
		if thread, ok := params["thread"].(map[string]any); ok {
			if id, ok := thread["id"].(string); ok {
				s.threadID = id
			}
		}
	case "turn/started":
		s.setStatus("running")
	case "turn/completed":
		s.handleTurnCompleted(params)
	case "item/agentMessage/delta":
		delta := stringFrom(params, "delta", "textDelta")
		itemID := stringFrom(params, "itemId")
		if itemID != "" {
			s.agentDeltaMu.Lock()
			s.agentDeltaItems[itemID] = true
			s.agentDeltaMu.Unlock()
		}
		if delta != "" {
			s.emit(Message{Type: "stream_delta", Text: delta})
		}
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta", "item/plan/delta":
		if delta := stringFrom(params, "delta", "textDelta"); delta != "" {
			s.emit(Message{Type: "thinking_delta", Text: delta})
		}
	case "turn/plan/updated":
		if text := compactAny(params); text != "" {
			s.emit(Message{Type: "assistant", Role: "assistant", Text: "Plan updated", Details: text})
		}
	case "item/started":
		if item, ok := params["item"].(map[string]any); ok {
			s.processItemStarted(item)
		}
	case "item/completed":
		if item, ok := params["item"].(map[string]any); ok {
			s.processItemCompleted(item)
		}
	case "serverRequest/resolved":
		toolUseID := stringFrom(params, "itemId", "approvalId", "requestId")
		if toolUseID != "" {
			s.emit(Message{Type: "permission_resolved", ToolUseID: toolUseID, Text: "Resolved"})
		}
	}
}

func (s *Session) handleTurnCompleted(params map[string]any) {
	status := "completed"
	if turn, ok := params["turn"].(map[string]any); ok {
		if raw, ok := turn["status"].(string); ok && raw != "" {
			status = raw
		}
		if status == "failed" {
			if errObj, ok := turn["error"].(map[string]any); ok {
				if msg, ok := errObj["message"].(string); ok && msg != "" {
					s.emit(Message{Type: "result", Subtype: "error", Text: msg})
					s.setStatus("idle")
					return
				}
			}
		}
	}
	s.emit(Message{Type: "result", Subtype: status, Text: status})

	s.pendingInputsMu.Lock()
	hasPendingInput := len(s.pendingInputs) > 0
	s.pendingInputsMu.Unlock()
	if hasPendingInput {
		s.setStatus("waiting_input")
	} else {
		s.setStatus("idle")
	}
}

func (s *Session) processItemStarted(item map[string]any) {
	itemID := stringFrom(item, "id")
	if itemID == "" {
		itemID = "item-" + shortID()
	}
	itemType := normalizeItemType(stringFrom(item, "type"))

	switch itemType {
	case "commandexecution":
		command := commandText(item["command"])
		s.emit(Message{Type: "tool", ToolUseID: itemID, ToolName: "Bash", Text: command})
	case "filechange":
		s.emit(Message{Type: "tool", ToolUseID: itemID, ToolName: "FileChange", Text: "Editing files", Details: compactAny(item["changes"])})
	case "dynamictoolcall":
		tool := stringFrom(item, "tool")
		if tool == "" {
			tool = "DynamicTool"
		}
		s.emit(Message{Type: "tool", ToolUseID: itemID, ToolName: tool, Text: tool, Details: compactAny(item["arguments"])})
	case "mcptoolcall":
		server := stringFrom(item, "server")
		tool := stringFrom(item, "tool")
		toolName := strings.Trim(server+"/"+tool, "/")
		if toolName == "" {
			toolName = "MCP"
		}
		s.emit(Message{Type: "tool", ToolUseID: itemID, ToolName: toolName, Text: toolName, Details: compactAny(item["arguments"])})
	}
}

func (s *Session) processItemCompleted(item map[string]any) {
	itemID := stringFrom(item, "id")
	if itemID == "" {
		itemID = "item-" + shortID()
	}
	itemType := normalizeItemType(stringFrom(item, "type"))

	switch itemType {
	case "agentmessage":
		s.agentDeltaMu.Lock()
		hadDelta := s.agentDeltaItems[itemID]
		delete(s.agentDeltaItems, itemID)
		s.agentDeltaMu.Unlock()
		if !hadDelta {
			if text := extractText(item); text != "" {
				s.emit(Message{Type: "assistant", Role: "assistant", Text: text})
			}
		}
	case "reasoning":
		if text := extractText(item); text != "" {
			s.emit(Message{Type: "thinking_delta", Text: text})
		}
	case "commandexecution":
		output := stringFrom(item, "aggregatedOutput", "output")
		if output == "" {
			output = "Command completed"
		}
		s.emit(Message{Type: "tool_result", ToolUseID: itemID, ToolName: "Bash", Text: output})
	case "filechange":
		s.emit(Message{Type: "tool_result", ToolUseID: itemID, ToolName: "FileChange", Text: "File changes complete", Details: compactAny(item["changes"])})
	case "dynamictoolcall":
		tool := stringFrom(item, "tool")
		if tool == "" {
			tool = "DynamicTool"
		}
		s.emit(Message{Type: "tool_result", ToolUseID: itemID, ToolName: tool, Text: "Tool completed", Details: compactAny(item)})
	case "mcptoolcall":
		server := stringFrom(item, "server")
		tool := stringFrom(item, "tool")
		toolName := strings.Trim(server+"/"+tool, "/")
		if toolName == "" {
			toolName = "MCP"
		}
		result := item["result"]
		if result == nil {
			result = item["error"]
		}
		s.emit(Message{Type: "tool_result", ToolUseID: itemID, ToolName: toolName, Text: "Tool completed", Details: compactAny(result)})
	case "websearch":
		query := stringFrom(item, "query")
		s.emit(Message{Type: "tool_result", ToolUseID: itemID, ToolName: "WebSearch", Text: query})
	case "error":
		text := stringFrom(item, "message")
		if text == "" {
			text = "Codex item failed"
		}
		s.emit(Message{Type: "error", Text: text})
	}
}

func (s *Session) setStatus(status string) {
	s.messagesMu.Lock()
	changed := s.status != status
	s.status = status
	s.messagesMu.Unlock()
	if changed {
		s.emit(Message{Type: "status", Status: status})
	}
}

func (s *Session) emit(msg Message) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if msg.ID == "" {
		msg.ID = "msg-" + shortID()
	}
	msg.SessionID = s.id
	if msg.ThreadID == "" {
		msg.ThreadID = s.threadID
	}
	msg.CreatedAt = now

	s.messagesMu.Lock()
	if msg.Type == "status" && msg.Status != "" {
		s.status = msg.Status
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

func rawID(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return fmt.Sprintf("%d", n)
	}
	return strings.Trim(string(raw), `"`)
}

func rawString(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func rawParams(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var params map[string]any
	if json.Unmarshal(raw, &params) != nil || params == nil {
		return map[string]any{}
	}
	return params
}

func errorMessage(raw json.RawMessage) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		if msg, ok := obj["message"].(string); ok && msg != "" {
			return msg
		}
		return compactAny(obj)
	}
	return string(raw)
}

func stringFrom(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func summarizeQuestions(raw any) ([]string, string) {
	items, _ := raw.([]any)
	var ids []string
	var lines []string
	for _, item := range items {
		q, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := stringFrom(q, "id")
		if id != "" {
			ids = append(ids, id)
		}
		header := stringFrom(q, "header")
		question := stringFrom(q, "question")
		switch {
		case header != "" && question != "":
			lines = append(lines, header+": "+question)
		case question != "":
			lines = append(lines, question)
		case header != "":
			lines = append(lines, header)
		}
		if options, ok := q["options"].([]any); ok && len(options) > 0 {
			var labels []string
			for _, option := range options {
				if opt, ok := option.(map[string]any); ok {
					if label := stringFrom(opt, "label"); label != "" {
						labels = append(labels, label)
					}
				}
			}
			if len(labels) > 0 {
				lines = append(lines, "Options: "+strings.Join(labels, ", "))
			}
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "Codex is waiting for your answer.")
	}
	return ids, strings.Join(lines, "\n")
}

func commandText(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			parts = append(parts, fmt.Sprint(part))
		}
		return strings.Join(parts, " ")
	default:
		return compactAny(raw)
	}
}

func normalizeItemType(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func extractText(item map[string]any) string {
	if text := stringFrom(item, "text", "message"); text != "" {
		return text
	}
	content, ok := item["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, entry := range content {
		part, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if text := stringFrom(part, "text"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
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

func attachmentOnlyText(count int) string {
	if count == 1 {
		return "Please inspect the attached image."
	}
	return fmt.Sprintf("Please inspect the %d attached images.", count)
}

func shortID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
