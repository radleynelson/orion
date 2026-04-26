package claudesdk

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"orion/internal/chatattachments"
)

const (
	SessionType            = "claude-chat"
	Provider               = "claude"
	ViewModeChat           = "chat"
	ViewModeTerminal       = "terminal"
	defaultReasoningEffort = "xhigh"
	defaultApprovalPolicy  = "never"
	defaultSandboxMode     = "danger-full-access"
	defaultPermissionMode  = "bypassPermissions"
)

type SessionInfo struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Label            string `json:"label"`
	WorkspacePath    string `json:"workspacePath"`
	Status           string `json:"status"`
	ThreadID         string `json:"threadId,omitempty"`
	Provider         string `json:"provider,omitempty"`
	ViewMode         string `json:"viewMode,omitempty"`
	RuntimeSessionID string `json:"runtimeSessionId,omitempty"`
	Model            string `json:"model,omitempty"`
	ReasoningEffort  string `json:"reasoningEffort,omitempty"`
	ApprovalPolicy   string `json:"approvalPolicy,omitempty"`
	SandboxMode      string `json:"sandboxMode,omitempty"`
	PermissionMode   string `json:"permissionMode,omitempty"`
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

type StartOptions struct {
	WorkspacePath    string
	Label            string
	ThreadID         string
	Model            string
	ReasoningEffort  string
	ApprovalPolicy   string
	SandboxMode      string
	PermissionMode   string
	ClaudeExecutable string
}

type bridgeEnvelope struct {
	Type            string                       `json:"type"`
	Status          string                       `json:"status,omitempty"`
	Text            string                       `json:"text,omitempty"`
	Error           string                       `json:"error,omitempty"`
	ThreadID        string                       `json:"threadId,omitempty"`
	Model           string                       `json:"model,omitempty"`
	ReasoningEffort string                       `json:"reasoningEffort,omitempty"`
	ApprovalPolicy  string                       `json:"approvalPolicy,omitempty"`
	SandboxMode     string                       `json:"sandboxMode,omitempty"`
	PermissionMode  string                       `json:"permissionMode,omitempty"`
	Message         *Message                     `json:"message,omitempty"`
	Attachments     []chatattachments.Attachment `json:"attachments,omitempty"`
}

type bridgeCommand struct {
	Type         string                       `json:"type"`
	Text         string                       `json:"text,omitempty"`
	ToolUseID    string                       `json:"toolUseId,omitempty"`
	PlanApproved bool                         `json:"planApproved,omitempty"`
	Attachments  []chatattachments.Attachment `json:"attachments,omitempty"`
}

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
	return m.StartWithOptions(StartOptions{WorkspacePath: workspacePath, Label: label})
}

func (m *Manager) Resume(workspacePath string, label string, threadID string) (*SessionInfo, error) {
	return m.StartWithOptions(StartOptions{
		WorkspacePath: workspacePath,
		Label:         label,
		ThreadID:      threadID,
	})
}

func (m *Manager) Attach(tmuxSession string, workspacePath string, label string) (*SessionInfo, error) {
	tmuxSession = strings.TrimSpace(tmuxSession)
	if tmuxSession == "" {
		return nil, errors.New("tmuxSession required")
	}
	if workspacePath == "" {
		workspacePath = tmuxCurrentPath(tmuxSession)
	}
	threadID := ThreadIDForTmux(tmuxSession, workspacePath)
	if threadID == "" {
		return nil, fmt.Errorf("could not identify Claude session for tmux session %s", tmuxSession)
	}
	return m.Resume(workspacePath, label, threadID)
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
		if strings.TrimSpace(path) != "" {
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
		session.cancel()
		if session.stdin != nil {
			_ = session.stdin.Close()
		}
		if session.cmd != nil && session.cmd.Process != nil {
			_ = session.cmd.Process.Kill()
		}
	}
}

func (m *Manager) SyncTmux(string) {}

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

func (m *Manager) StartWithOptions(options StartOptions) (*SessionInfo, error) {
	workspacePath := strings.TrimSpace(options.WorkspacePath)
	if workspacePath == "" {
		return nil, errors.New("workspacePath required")
	}

	label := strings.TrimSpace(options.Label)
	if label == "" {
		label = "Claude Chat"
	}
	model := strings.TrimSpace(options.Model)
	reasoningEffort := strings.TrimSpace(options.ReasoningEffort)
	if reasoningEffort == "" {
		reasoningEffort = defaultReasoningEffort
	}
	approvalPolicy := strings.TrimSpace(options.ApprovalPolicy)
	if approvalPolicy == "" {
		approvalPolicy = defaultApprovalPolicy
	}
	sandboxMode := strings.TrimSpace(options.SandboxMode)
	if sandboxMode == "" {
		sandboxMode = defaultSandboxMode
	}
	permissionMode := strings.TrimSpace(options.PermissionMode)
	if permissionMode == "" {
		permissionMode = defaultPermissionMode
	}
	threadID := strings.TrimSpace(options.ThreadID)
	if threadID != "" && !validClaudeSessionForWorkspace(threadID, workspacePath) {
		return nil, fmt.Errorf("Claude session not found for workspace: %s", threadID)
	}
	claudeExecutable := strings.TrimSpace(options.ClaudeExecutable)
	if claudeExecutable == "" {
		claudeExecutable = "claude"
	}

	script, err := bridgeScriptPath()
	if err != nil {
		return nil, err
	}

	id := "claude-chat-" + shortID()
	ctx, cancel := context.WithCancel(m.ctx)
	args := []string{
		script,
		"--workspace", workspacePath,
		"--label", label,
		"--effort", reasoningEffort,
		"--approval-policy", approvalPolicy,
		"--sandbox-mode", sandboxMode,
		"--permission-mode", permissionMode,
		"--claude-path", claudeExecutable,
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if threadID != "" {
		args = append(args, "--resume", threadID)
	}

	cmd := exec.CommandContext(ctx, "node", args...)
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
		model:           model,
		reasoningEffort: reasoningEffort,
		approvalPolicy:  approvalPolicy,
		sandboxMode:     sandboxMode,
		permissionMode:  permissionMode,
		cmd:             cmd,
		stdin:           stdin,
		ready:           make(chan struct{}),
		subscribers:     make(map[chan Message]struct{}),
		metadataEmitted: false,
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

	select {
	case <-session.ready:
	case <-time.After(30 * time.Second):
		_ = session.Stop()
		return nil, errors.New("timed out waiting for Claude SDK session to initialize")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	info := session.Info()
	return &info, nil
}

type Session struct {
	manager *Manager
	ctx     context.Context
	cancel  context.CancelFunc

	mu sync.RWMutex

	id              string
	label           string
	workspacePath   string
	threadID        string
	status          string
	model           string
	reasoningEffort string
	approvalPolicy  string
	sandboxMode     string
	permissionMode  string
	metadataEmitted bool

	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex

	ready     chan struct{}
	readyOnce sync.Once

	messagesMu sync.Mutex
	messages   []Message

	subscribersMu sync.Mutex
	subscribers   map[chan Message]struct{}
}

func (s *Session) Info() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SessionInfo{
		ID:               s.id,
		Type:             SessionType,
		Label:            s.label,
		WorkspacePath:    s.workspacePath,
		Status:           s.status,
		ThreadID:         s.threadID,
		Provider:         Provider,
		ViewMode:         ViewModeChat,
		RuntimeSessionID: s.id,
		Model:            s.model,
		ReasoningEffort:  s.reasoningEffort,
		ApprovalPolicy:   s.approvalPolicy,
		SandboxMode:      s.sandboxMode,
		PermissionMode:   s.permissionMode,
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
	return s.sendBridgeCommand(bridgeCommand{
		Type:        "input",
		Text:        text,
		Attachments: attachments,
	})
}

func (s *Session) Answer(toolUseID string, result string) error {
	result = strings.TrimSpace(result)
	if result == "" {
		return nil
	}
	return s.sendBridgeCommand(bridgeCommand{
		Type:      "answer",
		ToolUseID: strings.TrimSpace(toolUseID),
		Text:      result,
	})
}

func (s *Session) ApprovePlan() error {
	if err := s.sendBridgeCommand(bridgeCommand{
		Type:         "continue",
		Text:         "Approved. Continue with the plan.",
		PlanApproved: true,
	}); err != nil {
		return err
	}
	s.emit(Message{Type: "plan_resolved", Text: "Plan approved"})
	return nil
}

func (s *Session) Stop() error {
	_ = s.sendBridgeCommand(bridgeCommand{Type: "stop"})
	s.cancel()
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.setStatus("stopped", "Session stopped")
	s.manager.remove(s.id)
	return nil
}

func (s *Session) sendBridgeCommand(command bridgeCommand) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.stdin == nil {
		return errors.New("claude sdk bridge stdin is closed")
	}
	line, err := json.Marshal(command)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if _, err := s.stdin.Write(line); err != nil {
		return err
	}
	if command.Type == "input" || command.Type == "continue" {
		s.setStatus("running", "Claude is thinking")
	}
	return nil
}

func (s *Session) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope bridgeEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			s.emit(Message{Type: "error", Text: "Invalid Claude SDK event: " + err.Error()})
			continue
		}
		s.handleEnvelope(envelope)
	}
	if err := scanner.Err(); err != nil && s.ctx.Err() == nil {
		s.emit(Message{Type: "error", Text: err.Error()})
	}
}

func (s *Session) stderrLoop(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		s.emit(Message{Type: "error", Text: text})
	}
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	if s.ctx.Err() == nil && err != nil {
		s.emit(Message{Type: "error", Text: err.Error()})
	}
	if s.ctx.Err() == nil {
		s.setStatus("stopped", "Session stopped")
		s.manager.remove(s.id)
	}
}

func (s *Session) handleEnvelope(envelope bridgeEnvelope) {
	switch envelope.Type {
	case "session":
		s.mu.Lock()
		if envelope.ThreadID != "" {
			s.threadID = envelope.ThreadID
		}
		if envelope.Model != "" {
			s.model = envelope.Model
		}
		if envelope.ReasoningEffort != "" {
			s.reasoningEffort = envelope.ReasoningEffort
		}
		if envelope.ApprovalPolicy != "" {
			s.approvalPolicy = envelope.ApprovalPolicy
		}
		if envelope.SandboxMode != "" {
			s.sandboxMode = envelope.SandboxMode
		}
		if envelope.PermissionMode != "" {
			s.permissionMode = envelope.PermissionMode
		}
		s.metadataEmitted = true
		s.mu.Unlock()
		s.readyOnce.Do(func() { close(s.ready) })
		s.emitSessionMetadata("Claude chat ready")
		if s.currentStatus() == "running" {
			s.setStatus("running", "Claude is thinking")
		} else {
			s.setStatus("idle", "")
		}
	case "status":
		s.setStatus(firstNonEmpty(envelope.Status, "idle"), envelope.Text)
	case "message":
		if envelope.Message != nil {
			s.emit(*envelope.Message)
		}
	case "error":
		s.emit(Message{Type: "error", Text: firstNonEmpty(envelope.Error, envelope.Text, "Claude SDK error")})
	case "stopped":
		s.setStatus("stopped", "Session stopped")
	default:
		s.emit(Message{Type: "system", Text: envelope.Type, Details: compactAny(envelope)})
	}
}

func (s *Session) currentStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Session) emitSessionMetadata(text string) {
	info := s.Info()
	s.emit(Message{
		Type: "system",
		Text: text,
		Details: compactAny(map[string]any{
			"provider":         Provider,
			"viewMode":         ViewModeChat,
			"runtimeSessionId": info.ID,
			"threadId":         info.ThreadID,
			"model":            info.Model,
			"reasoningEffort":  info.ReasoningEffort,
			"approvalPolicy":   info.ApprovalPolicy,
			"sandboxMode":      info.SandboxMode,
			"permissionMode":   info.PermissionMode,
		}),
	})
}

func (s *Session) setStatus(status string, text string) {
	s.mu.Lock()
	changed := s.status != status
	textChanged := false
	if status == "" {
		status = "idle"
	}
	if s.status != status {
		s.status = status
		changed = true
	}
	s.mu.Unlock()
	if text != "" || changed || textChanged {
		s.emit(Message{Type: "status", Status: status, Text: text})
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
	if len(s.messages) > 1500 {
		s.messages = s.messages[len(s.messages)-1500:]
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

func ThreadIDForTmux(tmuxSession string, workspacePath string) string {
	if workspacePath == "" {
		workspacePath = tmuxCurrentPath(tmuxSession)
	}
	if id := tmuxOption(tmuxSession, "@orion_thread_id"); validClaudeSessionForWorkspace(id, workspacePath) {
		return id
	}
	if ids := claudeSessionIDsForTmux(tmuxSession); len(ids) > 0 {
		for _, id := range ids {
			if validClaudeSessionForWorkspace(id, workspacePath) {
				return id
			}
		}
		return ""
	}
	return latestSessionIDForWorkspace(workspacePath)
}

func bridgeScriptPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not resolve Claude SDK bridge path")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	path := filepath.Join(root, "frontend", "scripts", "claude-sdk-bridge.mjs")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func claudeSessionIDsForTmux(tmuxSession string) []string {
	panePID := tmuxPanePID(tmuxSession)
	if panePID <= 0 {
		return nil
	}
	commands := descendantProcessCommands(panePID)
	var ids []string
	seen := map[string]bool{}
	for _, command := range commands {
		for _, id := range parseClaudeSessionIDs(command) {
			if seen[id] || strings.TrimSpace(id) == "" {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func parseClaudeSessionIDs(command string) []string {
	fields := strings.Fields(command)
	var ids []string
	for i, field := range fields {
		switch {
		case field == "--resume" || field == "-r" || field == "--session-id":
			if i+1 < len(fields) {
				ids = append(ids, strings.TrimSpace(fields[i+1]))
			}
		case strings.HasPrefix(field, "--resume="):
			ids = append(ids, strings.TrimSpace(strings.TrimPrefix(field, "--resume=")))
		case strings.HasPrefix(field, "--session-id="):
			ids = append(ids, strings.TrimSpace(strings.TrimPrefix(field, "--session-id=")))
		}
	}
	return ids
}

func latestSessionIDForWorkspace(workspacePath string) string {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return ""
	}
	dir := userClaudeProjectDir(workspacePath)
	if dir == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type candidate struct {
		id      string
		modTime time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if strings.TrimSpace(id) == "" {
			continue
		}
		candidates = append(candidates, candidate{id: id, modTime: info.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].id
}

func validClaudeSessionForWorkspace(sessionID string, workspacePath string) bool {
	sessionID = strings.TrimSpace(sessionID)
	workspacePath = strings.TrimSpace(workspacePath)
	if sessionID == "" || workspacePath == "" {
		return false
	}
	path := filepath.Join(userClaudeProjectDir(workspacePath), sessionID+".jsonl")
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func userClaudeProjectDir(workspacePath string) string {
	if configDir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); configDir != "" {
		return filepath.Join(configDir, "projects", claudeProjectDir(workspacePath))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", claudeProjectDir(workspacePath))
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

func tmuxCurrentPath(name string) string {
	out, err := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func claudeProjectDir(path string) string {
	dir := strings.ReplaceAll(path, string(filepath.Separator), "-")
	return strings.ReplaceAll(dir, "_", "-")
}

func normalize(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
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
		line = strings.TrimSpace(strings.TrimLeft(line, "# "))
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shortID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
