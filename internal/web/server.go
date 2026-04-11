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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"orion/internal/server"
	"orion/internal/state"
	"orion/internal/terminal"
	"orion/internal/workspace"
)

// AppAPI defines the methods the web server needs from the main App.
// This avoids circular imports with the main package.
// AgentType represents an available agent type from config.
type AgentType struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
}

type AppAPI interface {
	GetRecentProjects() []string
	GetProjectInfo(path string) (*workspace.ProjectInfo, error)
	ListWorkspaces(repoRoot string) ([]workspace.Workspace, error)
	RecoverSessions(repoName string, workspacePaths []string) []state.SessionInfo
	GetSavedTabs() []state.SavedTab
	LaunchShell(repoRoot string, workspacePath string) (string, error)
	LaunchAgent(repoRoot string, workspacePath string, agentType string) (string, error)
	StartServers(repoRoot string, workspacePath string, isMain bool) ([]server.ServerStatus, error)
	StopServers(workspacePath string) error
	GetServerStatuses(repoRoot string, workspacePath string) []server.ServerStatus
	GetAgentNames(repoRoot string) []AgentType
	EmitSessionCreated(tmuxSession string, sessionType string, label string, workspacePath string)
}

// Server is the embedded HTTP/WebSocket server for the mobile companion PWA.
type Server struct {
	app     AppAPI
	termMgr *terminal.Manager
	httpSrv *http.Server
	token   string
	port    int

	upgrader websocket.Upgrader

	// Voice mode: connected iOS clients listening for Claude responses
	voiceClients   []*websocket.Conn
	voiceClientsMu sync.Mutex
}

// NewServer creates a new web server instance.
func NewServer(app AppAPI, termMgr *terminal.Manager) *Server {
	token := loadOrCreateToken()
	return &Server{
		app:     app,
		termMgr: termMgr,
		token:   token,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
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
	mux.HandleFunc("/api/agents", s.authMiddleware(s.handleAgents))
	mux.HandleFunc("/api/agent", s.authMiddleware(s.handleLaunchAgent))
	mux.HandleFunc("/api/servers", s.authMiddleware(s.handleServers))
	mux.HandleFunc("/api/servers/start", s.authMiddleware(s.handleServersStart))
	mux.HandleFunc("/api/servers/stop", s.authMiddleware(s.handleServersStop))
	mux.HandleFunc("/api/kill-session", s.authMiddleware(s.handleKillSession))

	// Voice mode routes
	mux.HandleFunc("/api/voice/response", s.authMiddleware(s.handleVoiceResponse))
	mux.HandleFunc("/api/config", s.authMiddleware(s.handleConfig))
	mux.HandleFunc("/ws/voice", s.handleVoiceWS)

	// WebSocket route
	mux.HandleFunc("/ws/terminal/", s.handleTerminalWS)

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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
		if !liveSessions[t.TmuxSession] {
			continue // tmux session is gone
		}
		if len(pathSet) > 0 && !pathSet[t.WorkspacePath] {
			continue // not in the requested workspace set
		}
		if strings.HasPrefix(t.TmuxSession, "orion-web-") {
			continue // skip phone companion sessions
		}
		sessions = append(sessions, state.SessionInfo{
			TmuxName:      t.TmuxSession,
			Type:          t.TabType,
			Label:         t.Label,
			WorkspacePath: t.WorkspacePath,
		})
		seen[t.TmuxSession] = true
	}

	// Supplement: pick up any sessions that RecoverSessions finds but saved tabs missed
	recovered := s.app.RecoverSessions(repo, paths)
	for _, sess := range recovered {
		if !seen[sess.TmuxName] {
			sessions = append(sessions, sess)
			seen[sess.TmuxName] = true
		}
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
	label := strings.ToUpper(req.AgentType[:1]) + req.AgentType[1:]
	s.app.EmitSessionCreated(tmuxSession, req.AgentType, label, req.WorkspacePath)
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.TmuxSession == "" {
		http.Error(w, "tmuxSession required", http.StatusBadRequest)
		return
	}
	exec.Command("tmux", "kill-session", "-t", req.TmuxSession).Run()
	writeJSON(w, map[string]string{"status": "killed"})
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

	s.voiceClientsMu.Lock()
	s.voiceClients = append(s.voiceClients, conn)
	count := len(s.voiceClients)
	s.voiceClientsMu.Unlock()

	log.Printf("[Orion Voice] Client connected (%d total)", count)

	// Keep the connection alive by reading (and discarding) client messages.
	// The client may send control messages like {"type":"ping"} or voice mode toggles.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

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

	var wsMu sync.Mutex
	writeWS := func(msg wsMessage) {
		wsMu.Lock()
		defer wsMu.Unlock()
		conn.WriteJSON(msg)
	}

	// Create a grouped tmux session with output callback
	onOutput := func(data []byte) {
		if data == nil {
			// Terminal exited
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

	// Clean up on disconnect
	defer s.termMgr.Close(terminalID)

	groupedName := "orion-web-" + terminalID
	firstResize := true

	// Read messages from the client
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		switch msg.Type {
		case "input":
			s.termMgr.Write(terminalID, msg.Data)
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
		}
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
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
