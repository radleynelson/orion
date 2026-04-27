package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"orion/internal/chatattachments"
	claudechat "orion/internal/claudesdk"
	"orion/internal/codexchat"
	oriongit "orion/internal/git"
	"orion/internal/server"
	"orion/internal/state"
	"orion/internal/terminal"
	"orion/internal/workspace"
)

// AppAPI defines the methods the web server needs from the main App.
// This avoids circular imports with the main package.
// AgentType represents an available agent type from config.
type AgentType struct {
	Name              string `json:"name"`
	Label             string `json:"label"`
	Provider          string `json:"provider,omitempty"`
	Icon              string `json:"icon,omitempty"`
	Model             string `json:"model,omitempty"`
	ReasoningEffort   string `json:"reasoningEffort,omitempty"`
	ApprovalPolicy    string `json:"approvalPolicy,omitempty"`
	SandboxMode       string `json:"sandboxMode,omitempty"`
	PermissionMode    string `json:"permissionMode,omitempty"`
	CollaborationMode string `json:"collaborationMode,omitempty"`
	ChatCapable       bool   `json:"chatCapable"`
}

type AppAPI interface {
	GetRecentProjects() []string
	GetProjectInfo(path string) (*workspace.ProjectInfo, error)
	ListWorkspaces(repoRoot string) ([]workspace.Workspace, error)
	CreateWorkspaceFrom(repoRoot string, name string, baseRef string) (*workspace.Workspace, error)
	RecoverSessions(repoName string, workspacePaths []string) []state.SessionInfo
	GetSavedTabs() []state.SavedTab
	LaunchShell(repoRoot string, workspacePath string) (string, error)
	LaunchAgent(repoRoot string, workspacePath string, agentType string) (string, error)
	ConvertChatToTerminal(repoRoot string, workspacePath string, sessionID string, chatKind string) (string, error)
	ConvertChatToTerminalWithOptions(repoRoot string, workspacePath string, sessionID string, chatKind string, model string, reasoningEffort string, permissionMode string, collaborationMode string) (string, error)
	LaunchClaudeChat(repoRoot string, workspacePath string) (*claudechat.SessionInfo, error)
	LaunchClaudeChatWithOptions(repoRoot string, workspacePath string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, permissionMode string) (*claudechat.SessionInfo, error)
	ResumeClaudeChat(repoRoot string, workspacePath string, threadID string) (*claudechat.SessionInfo, error)
	ResumeClaudeChatWithOptions(repoRoot string, workspacePath string, threadID string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, permissionMode string) (*claudechat.SessionInfo, error)
	ConvertTerminalToClaudeChat(repoRoot string, workspacePath string, tmuxSession string) (*claudechat.SessionInfo, error)
	ConvertTerminalToClaudeChatWithOptions(repoRoot string, workspacePath string, tmuxSession string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, permissionMode string) (*claudechat.SessionInfo, error)
	LaunchCodexChatWithOptions(repoRoot string, workspacePath string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, collaborationMode string) (*codexchat.SessionInfo, error)
	ResumeCodexChatWithOptions(repoRoot string, workspacePath string, threadID string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, collaborationMode string) (*codexchat.SessionInfo, error)
	ConvertTerminalToCodexChatWithOptions(repoRoot string, workspacePath string, tmuxSession string, model string, reasoningEffort string, approvalPolicy string, sandboxMode string, collaborationMode string) (*codexchat.SessionInfo, error)
	ListClaudeChatSessions(workspacePaths []string) []state.SessionInfo
	ListCodexChatSessions(workspacePaths []string) []state.SessionInfo
	StartServers(repoRoot string, workspacePath string, isMain bool) ([]server.ServerStatus, error)
	StopServers(workspacePath string) error
	GetServerStatuses(repoRoot string, workspacePath string) []server.ServerStatus
	GetAgentNames(repoRoot string) []AgentType
	GetChangedFilesAgainst(workspacePath string, base string) ([]oriongit.ChangedFile, error)
	GetUnifiedDiff(workspacePath string, base string, filePath string) (string, error)
	EmitSessionCreated(tmuxSession string, sessionType string, label string, workspacePath string)
	EmitSessionCreatedInfo(session state.SessionInfo)
	EmitSessionKilled(sessionID string)
}

// Server is the embedded HTTP/WebSocket server for the mobile companion PWA.
type Server struct {
	app       AppAPI
	termMgr   *terminal.Manager
	codexMgr  *codexchat.Manager
	claudeMgr *claudechat.Manager
	httpSrv   *http.Server
	token     string
	port      int

	upgrader websocket.Upgrader

	// Voice mode: connected iOS clients listening for Claude responses
	voiceClients   []*websocket.Conn
	voiceClientsMu sync.Mutex

	// Track active web terminal connections for zombie cleanup
	activeWebTerminals   map[string]bool
	activeWebTerminalsMu sync.Mutex
}

// NewServer creates a new web server instance.
func NewServer(app AppAPI, termMgr *terminal.Manager, codexMgr *codexchat.Manager, claudeMgr *claudechat.Manager) *Server {
	token := loadOrCreateToken()
	return &Server{
		app:       app,
		termMgr:   termMgr,
		codexMgr:  codexMgr,
		claudeMgr: claudeMgr,
		token:     token,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		activeWebTerminals: make(map[string]bool),
	}
}

// Start begins listening on the given port. Blocks until stopped.
func (s *Server) Start(port int) error {
	s.port = port

	// Clean up stale orion-web-* grouped sessions from previous runs
	s.cleanupStaleWebSessions()

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/projects", s.authMiddleware(s.handleProjects))
	mux.HandleFunc("/api/projects/info", s.authMiddleware(s.handleProjectInfo))
	mux.HandleFunc("/api/workspaces", s.authMiddleware(s.handleWorkspaces))
	mux.HandleFunc("/api/sessions", s.authMiddleware(s.handleSessions))
	mux.HandleFunc("/api/terminal", s.authMiddleware(s.handleTerminal))
	mux.HandleFunc("/api/shell", s.authMiddleware(s.handleShell))
	mux.HandleFunc("/api/convert-chat-to-terminal", s.authMiddleware(s.handleConvertChatToTerminal))
	mux.HandleFunc("/api/claude-chat", s.authMiddleware(s.handleClaudeChat))
	mux.HandleFunc("/api/claude-chat/message", s.authMiddleware(s.handleClaudeChatMessage))
	mux.HandleFunc("/api/claude-chat/answer", s.authMiddleware(s.handleClaudeChatAnswer))
	mux.HandleFunc("/api/codex-chat/history", s.authMiddleware(s.handleCodexChatHistory))
	mux.HandleFunc("/api/codex-chat", s.authMiddleware(s.handleCodexChat))
	mux.HandleFunc("/api/codex-chat/message", s.authMiddleware(s.handleCodexChatMessage))
	mux.HandleFunc("/api/codex-chat/answer", s.authMiddleware(s.handleCodexChatAnswer))
	mux.HandleFunc("/api/agents", s.authMiddleware(s.handleAgents))
	mux.HandleFunc("/api/agent", s.authMiddleware(s.handleLaunchAgent))
	mux.HandleFunc("/api/servers", s.authMiddleware(s.handleServers))
	mux.HandleFunc("/api/servers/start", s.authMiddleware(s.handleServersStart))
	mux.HandleFunc("/api/servers/stop", s.authMiddleware(s.handleServersStop))
	mux.HandleFunc("/api/git/changes", s.authMiddleware(s.handleGitChanges))
	mux.HandleFunc("/api/git/diff", s.authMiddleware(s.handleGitDiff))
	mux.HandleFunc("/api/kill-session", s.authMiddleware(s.handleKillSession))

	// Voice mode routes
	mux.HandleFunc("/api/voice/response", s.authMiddleware(s.handleVoiceResponse))
	mux.HandleFunc("/api/config", s.authMiddleware(s.handleConfig))
	mux.HandleFunc("/ws/voice", s.handleVoiceWS)

	// WebSocket route
	mux.HandleFunc("/ws/terminal/", s.handleTerminalWS)
	mux.HandleFunc("/ws/claude-chat/", s.handleClaudeChatWS)
	mux.HandleFunc("/ws/codex-chat/", s.handleCodexChatWS)

	// Static PWA files
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to load static assets: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticContent)))

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start periodic zombie web session cleanup (every 5 minutes)
	go s.zombieCleanupLoop()

	// Print connection info
	log.Printf("[Orion Mobile] Listening on port %d", port)
	log.Printf("[Orion Mobile] Token: %s", s.token)
	for _, ip := range getLocalIPs() {
		log.Printf("[Orion Mobile] Connect: http://%s:%d/?token=%s", ip, port, s.token)
	}

	if err := s.httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the web server.
func (s *Server) Stop() {
	s.cleanupStaleWebSessions()
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(ctx)
	}
}

// cleanupStaleWebSessions kills any orion-web-* tmux sessions left over from
// previous phone connections that weren't properly closed.
func (s *Server) cleanupStaleWebSessions() {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, "orion-web-") {
			exec.Command("tmux", "kill-session", "-t", name).Run()
			count++
		}
	}
	if count > 0 {
		log.Printf("[Orion Mobile] Cleaned up %d stale web sessions", count)
	}
}

// zombieCleanupLoop periodically checks for orion-web-* tmux sessions that are
// no longer associated with an active WebSocket connection and kills them.
func (s *Server) zombieCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
		if err != nil {
			continue
		}
		s.activeWebTerminalsMu.Lock()
		count := 0
		for _, line := range strings.Split(string(out), "\n") {
			name := strings.TrimSpace(line)
			if strings.HasPrefix(name, "orion-web-") && !s.activeWebTerminals[name] {
				exec.Command("tmux", "kill-session", "-t", name).Run()
				count++
			}
		}
		s.activeWebTerminalsMu.Unlock()
		if count > 0 {
			log.Printf("[Orion Mobile] Zombie cleanup: killed %d stale web sessions", count)
		}
	}
}

// GetConnectionURL returns the URL for connecting from a phone.
func (s *Server) GetConnectionURL() string {
	ips := getLocalIPs()
	if len(ips) == 0 {
		return fmt.Sprintf("http://localhost:%d/?token=%s", s.port, s.token)
	}
	return fmt.Sprintf("http://%s:%d/?token=%s", ips[0], s.port, s.token)
}

// GetToken returns the auth token.
func (s *Server) GetToken() string {
	return s.token
}

// --- Auth ---

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token {
			next(w, r)
			return
		}
		// Check query param
		if r.URL.Query().Get("token") == s.token {
			next(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

// --- REST Handlers ---

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.app.GetRecentProjects())
}

func (s *Server) handleProjectInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		http.Error(w, "root parameter required", http.StatusBadRequest)
		return
	}
	info, err := s.app.GetProjectInfo(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, info)
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		root := r.URL.Query().Get("root")
		if root == "" {
			http.Error(w, "root parameter required", http.StatusBadRequest)
			return
		}
		workspaces, err := s.app.ListWorkspaces(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, workspaces)
	case http.MethodPost:
		var req struct {
			Root    string `json:"root"`
			Name    string `json:"name"`
			BaseRef string `json:"baseRef"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Root) == "" || strings.TrimSpace(req.Name) == "" {
			http.Error(w, "root and name required", http.StatusBadRequest)
			return
		}
		workspace, err := s.app.CreateWorkspaceFrom(req.Root, req.Name, req.BaseRef)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, workspace)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := r.URL.Query().Get("repo")
	wsParam := r.URL.Query().Get("workspaces")
	if repo == "" {
		http.Error(w, "repo parameter required", http.StatusBadRequest)
		return
	}
	var paths []string
	if wsParam != "" {
		paths = strings.Split(wsParam, ",")
	}
	// Build the session list from saved tabs (primary source — same as desktop)
	// then supplement with RecoverSessions for any tmux sessions not in saved tabs.
	savedTabs := s.app.GetSavedTabs()

	// Get list of live tmux sessions for validation
	liveSessions := make(map[string]bool)
	if out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			name := strings.TrimSpace(line)
			if name != "" {
				liveSessions[name] = true
			}
		}
	}

	// Build workspace path set for filtering
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}

	// Primary: saved tabs that are still alive in tmux and match requested workspaces
	var sessions []state.SessionInfo
	seen := make(map[string]bool)
	for _, t := range savedTabs {
		if len(pathSet) > 0 && !pathSet[t.WorkspacePath] {
			continue // not in the requested workspace set
		}
		if t.TabType == codexchat.SessionType || t.TabType == claudechat.SessionType {
			continue // chat saved tabs are restore metadata; mobile should only show live chat managers
		}
		if !liveSessions[t.TmuxSession] {
			continue // tmux session is gone
		}
		if strings.HasPrefix(t.TmuxSession, "orion-web-") {
			continue // skip phone companion sessions
		}
		if seen[t.TmuxSession] {
			continue
		}
		sessions = append(sessions, state.SessionInfo{
			TmuxName:         t.TmuxSession,
			Type:             t.TabType,
			Label:            t.Label,
			WorkspacePath:    t.WorkspacePath,
			Provider:         t.Provider,
			Icon:             t.Icon,
			ViewMode:         t.ViewMode,
			RuntimeSessionID: t.RuntimeSessionID,
			ThreadID:         t.ThreadID,
			Model:            t.Model,
			ReasoningEffort:  t.ReasoningEffort,
			ApprovalPolicy:   t.ApprovalPolicy,
			SandboxMode:      t.SandboxMode,
		})
		seen[t.TmuxSession] = true
	}

	upsertSession := func(sess state.SessionInfo) {
		for i := range sessions {
			if sessions[i].TmuxName == sess.TmuxName || (sessions[i].ThreadID != "" && sessions[i].ThreadID == sess.ThreadID) {
				sessions[i] = reconcileSessionInfo(sessions[i], sess)
				seen[sess.TmuxName] = true
				return
			}
		}
		sessions = append(sessions, sess)
		seen[sess.TmuxName] = true
	}

	// Supplement and reconcile with live tmux recovery. Recovery is allowed to
	// correct stale saved tabs, for example a tab saved as shell while Claude is
	// now the live process inside that tmux session.
	for _, sess := range s.app.RecoverSessions(repo, paths) {
		upsertSession(sess)
	}
	for _, sess := range s.app.ListCodexChatSessions(paths) {
		upsertSession(sess)
	}
	for _, sess := range s.app.ListClaudeChatSessions(paths) {
		upsertSession(sess)
	}

	writeJSON(w, sessions)
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			TmuxSession string `json:"tmuxSession"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.TmuxSession == "" {
			http.Error(w, "tmuxSession required", http.StatusBadRequest)
			return
		}
		// Generate a unique terminal ID for the web client
		id := fmt.Sprintf("web-%d", time.Now().UnixNano())
		// The actual PTY creation happens when the WebSocket connects
		writeJSON(w, map[string]string{
			"terminalId":  id,
			"tmuxSession": req.TmuxSession,
		})

	case http.MethodDelete:
		// Extract ID from path: /api/terminal/<id>
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 {
			http.Error(w, "terminal ID required", http.StatusBadRequest)
			return
		}
		id := parts[len(parts)-1]
		if err := s.termMgr.Close(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "closed"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleShell(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RepoRoot      string `json:"repoRoot"`
		WorkspacePath string `json:"workspacePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepoRoot == "" || req.WorkspacePath == "" {
		http.Error(w, "repoRoot and workspacePath required", http.StatusBadRequest)
		return
	}
	tmuxSession, err := s.app.LaunchShell(req.RepoRoot, req.WorkspacePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.app.EmitSessionCreated(tmuxSession, "shell", "Shell", req.WorkspacePath)
	writeJSON(w, map[string]string{"tmuxSession": tmuxSession})
}

func (s *Server) handleConvertChatToTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RepoRoot          string `json:"repoRoot"`
		WorkspacePath     string `json:"workspacePath"`
		SessionID         string `json:"sessionId"`
		ChatKind          string `json:"chatKind"`
		Model             string `json:"model"`
		ReasoningEffort   string `json:"reasoningEffort"`
		PermissionMode    string `json:"permissionMode"`
		CollaborationMode string `json:"collaborationMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepoRoot == "" || req.WorkspacePath == "" || req.SessionID == "" || req.ChatKind == "" {
		http.Error(w, "repoRoot, workspacePath, sessionId, and chatKind required", http.StatusBadRequest)
		return
	}
	tmuxSession, err := s.app.ConvertChatToTerminalWithOptions(
		req.RepoRoot,
		req.WorkspacePath,
		req.SessionID,
		req.ChatKind,
		req.Model,
		req.ReasoningEffort,
		req.PermissionMode,
		req.CollaborationMode,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	label := "Codex"
	if req.ChatKind == "claude" {
		label = "Claude"
	}
	s.app.EmitSessionCreated(tmuxSession, req.ChatKind, label, req.WorkspacePath)
	writeJSON(w, map[string]string{"tmuxSession": tmuxSession})
}

func (s *Server) handleClaudeChat(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		wsParam := r.URL.Query().Get("workspaces")
		var paths []string
		if wsParam != "" {
			paths = strings.Split(wsParam, ",")
		}
		writeJSON(w, s.claudeMgr.List(paths))
	case http.MethodPost:
		var req struct {
			RepoRoot        string `json:"repoRoot"`
			WorkspacePath   string `json:"workspacePath"`
			ThreadID        string `json:"threadId"`
			TmuxSession     string `json:"tmuxSession"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoningEffort"`
			ApprovalPolicy  string `json:"approvalPolicy"`
			SandboxMode     string `json:"sandboxMode"`
			PermissionMode  string `json:"permissionMode"`
			Icon            string `json:"icon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.WorkspacePath == "" {
			http.Error(w, "workspacePath required", http.StatusBadRequest)
			return
		}
		if req.RepoRoot == "" {
			http.Error(w, "repoRoot required", http.StatusBadRequest)
			return
		}
		var (
			info *claudechat.SessionInfo
			err  error
		)
		switch {
		case strings.TrimSpace(req.ThreadID) != "":
			info, err = s.app.ResumeClaudeChatWithOptions(
				req.RepoRoot,
				req.WorkspacePath,
				req.ThreadID,
				req.Model,
				req.ReasoningEffort,
				req.ApprovalPolicy,
				req.SandboxMode,
				req.PermissionMode,
			)
		case strings.TrimSpace(req.TmuxSession) != "":
			info, err = s.app.ConvertTerminalToClaudeChatWithOptions(
				req.RepoRoot,
				req.WorkspacePath,
				req.TmuxSession,
				req.Model,
				req.ReasoningEffort,
				req.ApprovalPolicy,
				req.SandboxMode,
				req.PermissionMode,
			)
		default:
			info, err = s.app.LaunchClaudeChatWithOptions(
				req.RepoRoot,
				req.WorkspacePath,
				req.Model,
				req.ReasoningEffort,
				req.ApprovalPolicy,
				req.SandboxMode,
				req.PermissionMode,
			)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		info.Icon = firstNonEmpty(req.Icon, info.Icon, "claude")
		s.claudeMgr.SetIcon(info.ID, info.Icon)
		s.app.EmitSessionCreatedInfo(state.SessionInfo{
			TmuxName:         info.ID,
			Type:             info.Type,
			Label:            info.Label,
			WorkspacePath:    info.WorkspacePath,
			Provider:         "claude",
			Icon:             info.Icon,
			ViewMode:         "chat",
			RuntimeSessionID: info.ID,
			ThreadID:         info.ThreadID,
			Model:            info.Model,
			ReasoningEffort:  info.ReasoningEffort,
			ApprovalPolicy:   info.ApprovalPolicy,
			SandboxMode:      info.SandboxMode,
			PermissionMode:   info.PermissionMode,
		})
		writeJSON(w, info)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleClaudeChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID   string                  `json:"sessionId"`
		Text        string                  `json:"text"`
		Attachments []chatattachments.Input `json:"attachments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	session, ok := s.claudeMgr.Get(req.SessionID)
	if !ok {
		http.Error(w, "claude chat session not found", http.StatusNotFound)
		return
	}
	attachments, err := chatattachments.Resolve(req.SessionID, req.Attachments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := session.Send(req.Text, attachments); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

func (s *Server) handleClaudeChatAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"sessionId"`
		ToolUseID string `json:"toolUseId"`
		Result    string `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	session, ok := s.claudeMgr.Get(req.SessionID)
	if !ok {
		http.Error(w, "claude chat session not found", http.StatusNotFound)
		return
	}
	if err := session.Answer(req.ToolUseID, req.Result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "answered"})
}

func (s *Server) handleCodexChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, codexchat.ListHistory(r.URL.Query().Get("workspace"), limit))
}

func (s *Server) handleCodexChat(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		wsParam := r.URL.Query().Get("workspaces")
		var paths []string
		if wsParam != "" {
			paths = strings.Split(wsParam, ",")
		}
		writeJSON(w, s.codexMgr.List(paths))
	case http.MethodPost:
		var req struct {
			RepoRoot          string `json:"repoRoot"`
			WorkspacePath     string `json:"workspacePath"`
			ThreadID          string `json:"threadId"`
			TmuxSession       string `json:"tmuxSession"`
			Model             string `json:"model"`
			ReasoningEffort   string `json:"reasoningEffort"`
			ApprovalPolicy    string `json:"approvalPolicy"`
			SandboxMode       string `json:"sandboxMode"`
			CollaborationMode string `json:"collaborationMode"`
			Icon              string `json:"icon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.WorkspacePath == "" {
			http.Error(w, "workspacePath required", http.StatusBadRequest)
			return
		}
		var (
			info *codexchat.SessionInfo
			err  error
		)
		switch {
		case strings.TrimSpace(req.ThreadID) != "":
			info, err = s.app.ResumeCodexChatWithOptions(
				req.RepoRoot,
				req.WorkspacePath,
				req.ThreadID,
				req.Model,
				req.ReasoningEffort,
				req.ApprovalPolicy,
				req.SandboxMode,
				req.CollaborationMode,
			)
		case strings.TrimSpace(req.TmuxSession) != "":
			info, err = s.app.ConvertTerminalToCodexChatWithOptions(
				req.RepoRoot,
				req.WorkspacePath,
				req.TmuxSession,
				req.Model,
				req.ReasoningEffort,
				req.ApprovalPolicy,
				req.SandboxMode,
				req.CollaborationMode,
			)
		default:
			info, err = s.app.LaunchCodexChatWithOptions(
				req.RepoRoot,
				req.WorkspacePath,
				req.Model,
				req.ReasoningEffort,
				req.ApprovalPolicy,
				req.SandboxMode,
				req.CollaborationMode,
			)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		info.Icon = firstNonEmpty(req.Icon, info.Icon, codexchat.Provider)
		s.codexMgr.SetIcon(info.ID, info.Icon)
		s.app.EmitSessionCreatedInfo(state.SessionInfo{
			TmuxName:          info.ID,
			Type:              codexchat.SessionType,
			Label:             info.Label,
			WorkspacePath:     info.WorkspacePath,
			Provider:          codexchat.Provider,
			Icon:              info.Icon,
			ViewMode:          codexchat.ViewModeChat,
			RuntimeSessionID:  info.ID,
			ThreadID:          info.ThreadID,
			Model:             info.Model,
			ReasoningEffort:   info.ReasoningEffort,
			ApprovalPolicy:    info.ApprovalPolicy,
			SandboxMode:       info.SandboxMode,
			CollaborationMode: info.CollaborationMode,
		})
		writeJSON(w, info)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCodexChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID   string                  `json:"sessionId"`
		Text        string                  `json:"text"`
		Attachments []chatattachments.Input `json:"attachments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	session, ok := s.codexMgr.Get(req.SessionID)
	if !ok {
		http.Error(w, "codex chat session not found", http.StatusNotFound)
		return
	}
	attachments, err := chatattachments.Resolve(req.SessionID, req.Attachments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := session.Send(req.Text, attachments); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "sent"})
}

func (s *Server) handleCodexChatAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"sessionId"`
		ToolUseID string `json:"toolUseId"`
		Result    string `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	session, ok := s.codexMgr.Get(req.SessionID)
	if !ok {
		http.Error(w, "codex chat session not found", http.StatusNotFound)
		return
	}
	if err := session.Answer(req.ToolUseID, req.Result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "answered"})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		http.Error(w, "root parameter required", http.StatusBadRequest)
		return
	}
	agents := s.app.GetAgentNames(root)
	if agents == nil {
		agents = []AgentType{}
	}
	writeJSON(w, agents)
}

func (s *Server) handleGitChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspacePath := r.URL.Query().Get("workspace")
	if workspacePath == "" {
		http.Error(w, "workspace parameter required", http.StatusBadRequest)
		return
	}
	files, err := s.app.GetChangedFilesAgainst(workspacePath, r.URL.Query().Get("base"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []oriongit.ChangedFile{}
	}
	writeJSON(w, files)
}

func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workspacePath := r.URL.Query().Get("workspace")
	filePath := r.URL.Query().Get("file")
	if workspacePath == "" || filePath == "" {
		http.Error(w, "workspace and file parameters required", http.StatusBadRequest)
		return
	}
	diff, err := s.app.GetUnifiedDiff(workspacePath, r.URL.Query().Get("base"), filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"diff": diff})
}

func (s *Server) handleLaunchAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RepoRoot      string `json:"repoRoot"`
		WorkspacePath string `json:"workspacePath"`
		AgentType     string `json:"agentType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	tmuxSession, err := s.app.LaunchAgent(req.RepoRoot, req.WorkspacePath, req.AgentType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sessionType := req.AgentType
	label := req.AgentType
	for _, agent := range s.app.GetAgentNames(req.RepoRoot) {
		if agent.Name != req.AgentType {
			continue
		}
		label = agent.Label
		if strings.TrimSpace(agent.Provider) != "" {
			sessionType = agent.Provider
		}
		break
	}
	if label == "" {
		label = "Agent"
	}
	s.app.EmitSessionCreated(tmuxSession, sessionType, label, req.WorkspacePath)
	writeJSON(w, map[string]string{"tmuxSession": tmuxSession})
}

func (s *Server) handleServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := r.URL.Query().Get("root")
	wsPath := r.URL.Query().Get("workspace")
	if root == "" || wsPath == "" {
		http.Error(w, "root and workspace required", http.StatusBadRequest)
		return
	}
	statuses := s.app.GetServerStatuses(root, wsPath)
	if statuses == nil {
		statuses = []server.ServerStatus{}
	}
	writeJSON(w, statuses)
}

func (s *Server) handleServersStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RepoRoot      string `json:"repoRoot"`
		WorkspacePath string `json:"workspacePath"`
		IsMain        bool   `json:"isMain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	statuses, err := s.app.StartServers(req.RepoRoot, req.WorkspacePath, req.IsMain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, statuses)
}

func (s *Server) handleServersStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspacePath string `json:"workspacePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.app.StopServers(req.WorkspacePath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TmuxSession string `json:"tmuxSession"`
		SessionID   string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	target := firstNonEmpty(req.SessionID, req.TmuxSession)
	if target == "" {
		http.Error(w, "sessionId required", http.StatusBadRequest)
		return
	}
	if stopped := s.stopClaudeChatTarget(target); stopped != "" {
		s.app.EmitSessionKilled(stopped)
		writeJSON(w, map[string]string{"status": "killed"})
		return
	}
	if stopped := s.stopCodexChatTarget(target); stopped != "" {
		s.app.EmitSessionKilled(stopped)
		writeJSON(w, map[string]string{"status": "killed"})
		return
	}
	exec.Command("tmux", "kill-session", "-t", target).Run()
	s.app.EmitSessionKilled(target)
	writeJSON(w, map[string]string{"status": "killed"})
}

func (s *Server) stopClaudeChatTarget(target string) string {
	if target == "" {
		return ""
	}
	if _, ok := s.claudeMgr.Get(target); ok {
		_ = s.claudeMgr.Stop(target)
		return target
	}
	for _, info := range s.claudeMgr.List(nil) {
		if info.ID == target || info.RuntimeSessionID == target || (info.ThreadID != "" && info.ThreadID == target) {
			_ = s.claudeMgr.Stop(info.ID)
			return target
		}
	}
	return ""
}

func (s *Server) stopCodexChatTarget(target string) string {
	if target == "" {
		return ""
	}
	if _, ok := s.codexMgr.Get(target); ok {
		_ = s.codexMgr.Stop(target)
		return target
	}
	for _, info := range s.codexMgr.List(nil) {
		if info.ID == target || info.RuntimeSessionID == target || (info.ThreadID != "" && info.ThreadID == target) {
			_ = s.codexMgr.Stop(info.ID)
			return target
		}
	}
	return ""
}

// --- Config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	home, _ := os.UserHomeDir()
	apiKey, _ := os.ReadFile(filepath.Join(home, ".orion", "openai-api-key"))
	writeJSON(w, map[string]string{
		"openaiApiKey": strings.TrimSpace(string(apiKey)),
	})
}

// --- Voice Mode ---

// handleVoiceResponse receives Claude's response text from the Stop hook
// and broadcasts it to all connected voice WebSocket clients.
func (s *Server) handleVoiceResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text    string `json:"text"`
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if req.Session != "" {
		s.claudeMgr.SyncTmux(req.Session)
	}

	msg, _ := json.Marshal(map[string]string{
		"type":    "voice",
		"text":    req.Text,
		"session": req.Session,
	})

	s.voiceClientsMu.Lock()
	var alive []*websocket.Conn
	for _, c := range s.voiceClients {
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			c.Close()
		} else {
			alive = append(alive, c)
		}
	}
	s.voiceClients = alive
	s.voiceClientsMu.Unlock()

	log.Printf("[Orion Voice] Broadcast to %d client(s): %d chars", len(alive), len(req.Text))
	writeJSON(w, map[string]string{"status": "ok", "clients": fmt.Sprintf("%d", len(alive))})
}

// handleVoiceWS upgrades to a WebSocket for streaming voice messages to the iOS app.
func (s *Server) handleVoiceWS(w http.ResponseWriter, r *http.Request) {
	// Auth check
	token := r.URL.Query().Get("token")
	auth := r.Header.Get("Authorization")
	if token != s.token && !(strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Orion Voice] WebSocket upgrade failed: %v", err)
		return
	}

	// WebSocket heartbeat: set initial read deadline and pong handler
	conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})

	// Send ping frames every 15 seconds
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	s.voiceClientsMu.Lock()
	s.voiceClients = append(s.voiceClients, conn)
	count := len(s.voiceClients)
	s.voiceClientsMu.Unlock()

	log.Printf("[Orion Voice] Client connected (%d total)", count)

	// Keep the connection alive by reading (and discarding) client messages.
	// The client may send control messages like {"type":"ping"} or voice mode toggles.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		// Reset read deadline on every received message
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))

		// Handle application-level ping from iOS client
		var parsed struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(msg, &parsed) == nil && parsed.Type == "ping" {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`))
		}
	}

	close(pingDone)

	// Remove from voice clients on disconnect
	s.voiceClientsMu.Lock()
	for i, c := range s.voiceClients {
		if c == conn {
			s.voiceClients = append(s.voiceClients[:i], s.voiceClients[i+1:]...)
			break
		}
	}
	count = len(s.voiceClients)
	s.voiceClientsMu.Unlock()

	log.Printf("[Orion Voice] Client disconnected (%d remaining)", count)
}

// --- WebSocket Terminal Handler ---

type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	// Auth check
	token := r.URL.Query().Get("token")
	auth := r.Header.Get("Authorization")
	if token != s.token && !(strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract terminal ID and tmux session from path: /ws/terminal/<id>?tmux=<session>
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/ws/terminal/"), "/")
	terminalID := parts[0]
	tmuxSession := r.URL.Query().Get("tmux")

	if terminalID == "" || tmuxSession == "" {
		http.Error(w, "terminalId and tmux query param required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Orion Mobile] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// WebSocket heartbeat: set initial read deadline and pong handler
	conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})

	// Send ping frames every 15 seconds
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()
	defer close(pingDone)

	var wsMu sync.Mutex
	writeWS := func(msg wsMessage) {
		wsMu.Lock()
		defer wsMu.Unlock()
		conn.WriteJSON(msg)
	}
	closeWS := func(code int, text string) {
		wsMu.Lock()
		defer wsMu.Unlock()
		deadline := time.Now().Add(2 * time.Second)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), deadline)
		_ = conn.Close()
	}
	groupedName := "orion-web-" + terminalID

	// Create a grouped tmux session with output callback
	onOutput := func(data []byte) {
		if data == nil {
			if tmuxSessionExists(tmuxSession) {
				log.Printf("[Orion Mobile] grouped terminal detached: terminal=%s grouped=%s tmux=%s", terminalID, groupedName, tmuxSession)
				closeWS(websocket.CloseGoingAway, "terminal detached")
				return
			}
			log.Printf("[Orion Mobile] terminal exited: terminal=%s grouped=%s tmux=%s", terminalID, groupedName, tmuxSession)
			writeWS(wsMessage{Type: "exit"})
			return
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		writeWS(wsMessage{Type: "output", Data: encoded})
	}

	if err := s.termMgr.CreateGroupedAttached(terminalID, tmuxSession, onOutput); err != nil {
		log.Printf("[Orion Mobile] Failed to create grouped terminal: %v", err)
		conn.WriteJSON(wsMessage{Type: "error", Data: err.Error()})
		return
	}

	// Track active web terminal for zombie cleanup
	s.activeWebTerminalsMu.Lock()
	s.activeWebTerminals[groupedName] = true
	s.activeWebTerminalsMu.Unlock()

	// Clean up on disconnect
	defer func() {
		s.termMgr.Close(terminalID)
		s.activeWebTerminalsMu.Lock()
		delete(s.activeWebTerminals, groupedName)
		s.activeWebTerminalsMu.Unlock()
	}()

	firstResize := true

	// Read messages from the client
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		// Reset read deadline on every received message
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))

		switch msg.Type {
		case "input":
			s.termMgr.Write(terminalID, msg.Data)
		case "ping":
			writeWS(wsMessage{Type: "pong"})
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				s.termMgr.Resize(terminalID, msg.Cols, msg.Rows)
			}
			// After first resize, force tmux to redraw at the phone's size
			if firstResize {
				firstResize = false
				go func() {
					time.Sleep(150 * time.Millisecond)
					// Force tmux to refresh the client at the new size
					exec.Command("tmux", "refresh-client", "-t", groupedName).Run()
				}()
			}
		case "scroll":
			direction := msg.Data
			lines := msg.Cols
			if lines <= 0 {
				lines = 3
			}
			scrollCmd := "scroll-up"
			if direction == "down" {
				scrollCmd = "scroll-down"
			}
			args := []string{"copy-mode", "-t", groupedName}
			for i := 0; i < lines; i++ {
				args = append(args, ";", "send-keys", "-t", groupedName, "-X", scrollCmd)
			}
			out, err := exec.Command("tmux", args...).CombinedOutput()
			if err != nil {
				log.Printf("[Orion Mobile] scroll failed: target=%s dir=%s err=%v out=%q", groupedName, direction, err, string(out))
			} else {
				log.Printf("[Orion Mobile] scroll OK: target=%s dir=%s lines=%d", groupedName, direction, lines)
			}
		case "cancel-copy-mode":
			// Use tmux's official cancel command — bypasses the PTY entirely
			// so it works regardless of any sub-mode (search, goto-line, etc.)
			out, err := exec.Command("tmux", "send-keys", "-t", groupedName, "-X", "cancel").CombinedOutput()
			if err != nil {
				log.Printf("[Orion Mobile] cancel-copy-mode failed: target=%s err=%v out=%q", groupedName, err, string(out))
			}
		}
	}
}

type codexChatWSMessage struct {
	Type        string                  `json:"type"`
	Text        string                  `json:"text,omitempty"`
	ToolUseID   string                  `json:"toolUseId,omitempty"`
	Action      string                  `json:"action,omitempty"`
	Attachments []chatattachments.Input `json:"attachments,omitempty"`
}

func (s *Server) handleClaudeChatWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	auth := r.Header.Get("Authorization")
	if token != s.token && !(strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/ws/claude-chat/")
	if sessionID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	session, ok := s.claudeMgr.Get(sessionID)
	if !ok {
		workspacePath := firstNonEmpty(r.URL.Query().Get("workspacePath"), r.URL.Query().Get("workspace"))
		if workspacePath == "" {
			workspacePath = tmuxCurrentPath(sessionID)
		}
		var (
			info *claudechat.SessionInfo
			err  error
		)
		if strings.HasPrefix(sessionID, "orion-") {
			info, err = s.claudeMgr.Attach(sessionID, workspacePath, "Claude")
		} else {
			info, err = s.claudeMgr.Resume(workspacePath, "Claude Chat", sessionID)
		}
		if err != nil {
			http.Error(w, "claude chat session not found", http.StatusNotFound)
			return
		}
		session, ok = s.claudeMgr.Get(info.ID)
		if !ok {
			http.Error(w, "claude chat session not found", http.StatusNotFound)
			return
		}
	}
	runtimeSessionID := session.Info().ID

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Orion Claude Chat] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})

	var wsMu sync.Mutex
	writeJSONMessage := func(msg interface{}) bool {
		wsMu.Lock()
		defer wsMu.Unlock()
		if err := conn.WriteJSON(msg); err != nil {
			return false
		}
		return true
	}

	for _, msg := range session.Messages() {
		if !writeJSONMessage(msg) {
			return
		}
	}

	updates, unsubscribe := session.Subscribe()
	defer unsubscribe()

	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				wsMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				wsMu.Unlock()
				if err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()
	defer close(pingDone)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			var msg codexChatWSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			conn.SetReadDeadline(time.Now().Add(45 * time.Second))
			switch msg.Type {
			case "input":
				attachments, err := chatattachments.Resolve(runtimeSessionID, msg.Attachments)
				if err != nil {
					writeJSONMessage(claudechat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
					continue
				}
				if err := session.Send(msg.Text, attachments); err != nil {
					writeJSONMessage(claudechat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			case "answer":
				if err := session.Answer(msg.ToolUseID, msg.Text); err != nil {
					writeJSONMessage(claudechat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			case "plan_action":
				if msg.Action != "approve" {
					writeJSONMessage(claudechat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      "unsupported plan action",
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
					continue
				}
				if err := session.ApprovePlan(); err != nil {
					writeJSONMessage(claudechat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			case "ping":
				writeJSONMessage(codexChatWSMessage{Type: "pong"})
			}
		}
	}()

	for {
		select {
		case msg, ok := <-updates:
			if !ok {
				return
			}
			if !writeJSONMessage(msg) {
				return
			}
		case <-readDone:
			return
		}
	}
}

func (s *Server) handleCodexChatWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	auth := r.Header.Get("Authorization")
	if token != s.token && !(strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/ws/codex-chat/")
	if sessionID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	session, ok := s.codexMgr.Get(sessionID)
	if !ok {
		workspacePath := firstNonEmpty(r.URL.Query().Get("workspacePath"), r.URL.Query().Get("workspace"))
		if workspacePath == "" {
			http.Error(w, "codex chat session not found", http.StatusNotFound)
			return
		}
		info, err := s.codexMgr.StartWithOptions(codexchat.StartOptions{
			WorkspacePath: workspacePath,
			Label:         "Codex Chat",
			ThreadID:      sessionID,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		session, ok = s.codexMgr.Get(info.ID)
		if !ok {
			http.Error(w, "codex chat session not found", http.StatusNotFound)
			return
		}
	}
	runtimeSessionID := session.Info().ID

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Orion Codex Chat] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})

	var wsMu sync.Mutex
	writeJSONMessage := func(msg interface{}) bool {
		wsMu.Lock()
		defer wsMu.Unlock()
		if err := conn.WriteJSON(msg); err != nil {
			return false
		}
		return true
	}

	for _, msg := range session.Messages() {
		if !writeJSONMessage(msg) {
			return
		}
	}

	updates, unsubscribe := session.Subscribe()
	defer unsubscribe()

	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				wsMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				wsMu.Unlock()
				if err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()
	defer close(pingDone)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			var msg codexChatWSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			conn.SetReadDeadline(time.Now().Add(45 * time.Second))
			switch msg.Type {
			case "input":
				attachments, err := chatattachments.Resolve(runtimeSessionID, msg.Attachments)
				if err != nil {
					writeJSONMessage(codexchat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
					continue
				}
				if err := session.Send(msg.Text, attachments); err != nil {
					writeJSONMessage(codexchat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			case "answer":
				if err := session.Answer(msg.ToolUseID, msg.Text); err != nil {
					writeJSONMessage(codexchat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			case "plan_action":
				if msg.Action != "approve" {
					writeJSONMessage(codexchat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      "unsupported plan action",
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
					continue
				}
				if err := session.ApprovePlan(); err != nil {
					writeJSONMessage(codexchat.Message{
						ID:        "msg-" + fmt.Sprintf("%d", time.Now().UnixNano()),
						SessionID: runtimeSessionID,
						Type:      "error",
						Text:      err.Error(),
						CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					})
				}
			case "ping":
				writeJSONMessage(codexChatWSMessage{Type: "pong"})
			}
		}
	}()

	for {
		select {
		case msg, ok := <-updates:
			if !ok {
				return
			}
			if !writeJSONMessage(msg) {
				return
			}
		case <-readDone:
			return
		}
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func reconcileSessionInfo(existing state.SessionInfo, incoming state.SessionInfo) state.SessionInfo {
	if incoming.Type == codexchat.SessionType || incoming.Type == claudechat.SessionType {
		return incoming
	}
	if isTerminalAgent(incoming.Type) {
		out := existing
		typeChanged := out.Type != "" && out.Type != incoming.Type
		out.TmuxName = firstNonEmpty(incoming.TmuxName, out.TmuxName)
		out.Type = incoming.Type
		out.Label = firstNonEmpty(incoming.Label, labelForTerminalType(incoming.Type), out.Label)
		out.WorkspacePath = firstNonEmpty(out.WorkspacePath, incoming.WorkspacePath)
		out.Provider = firstNonEmpty(incoming.Provider, incoming.Type, out.Provider)
		out.Icon = firstNonEmpty(out.Icon, incoming.Icon)
		out.ViewMode = firstNonEmpty(out.ViewMode, incoming.ViewMode, "terminal")
		out.RuntimeSessionID = firstNonEmpty(out.RuntimeSessionID, incoming.RuntimeSessionID, incoming.TmuxName)
		if typeChanged {
			out.ThreadID = ""
			out.Model = ""
			out.ReasoningEffort = ""
			out.ApprovalPolicy = ""
			out.SandboxMode = ""
			out.PermissionMode = ""
		} else {
			out.ThreadID = firstNonEmpty(out.ThreadID, incoming.ThreadID)
			out.Model = firstNonEmpty(out.Model, incoming.Model)
			out.ReasoningEffort = firstNonEmpty(out.ReasoningEffort, incoming.ReasoningEffort)
			out.ApprovalPolicy = firstNonEmpty(out.ApprovalPolicy, incoming.ApprovalPolicy)
			out.SandboxMode = firstNonEmpty(out.SandboxMode, incoming.SandboxMode)
			out.PermissionMode = firstNonEmpty(out.PermissionMode, incoming.PermissionMode)
		}
		return out
	}
	if existing.Type == "" || existing.Type == "shell" {
		return incoming
	}
	return existing
}

func isTerminalAgent(sessionType string) bool {
	return sessionType == "claude" || sessionType == "codex"
}

func labelForTerminalType(sessionType string) string {
	switch sessionType {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	default:
		return "Shell"
	}
}

func tmuxSessionExists(name string) bool {
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

func loadOrCreateToken() string {
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".orion", "web-token")

	// Try to load existing token
	if data, err := os.ReadFile(tokenPath); err == nil {
		token := strings.TrimSpace(string(data))
		if len(token) >= 32 {
			return token
		}
	}

	// Generate new token
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	// Save it
	os.MkdirAll(filepath.Dir(tokenPath), 0755)
	os.WriteFile(tokenPath, []byte(token), 0600)

	return token
}

func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}
