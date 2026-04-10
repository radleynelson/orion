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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"orion/internal/state"
	"orion/internal/terminal"
	"orion/internal/workspace"
)

// AppAPI defines the methods the web server needs from the main App.
// This avoids circular imports with the main package.
type AppAPI interface {
	GetRecentProjects() []string
	GetProjectInfo(path string) (*workspace.ProjectInfo, error)
	ListWorkspaces(repoRoot string) ([]workspace.Workspace, error)
	RecoverSessions(repoName string, workspacePaths []string) []state.SessionInfo
	LaunchShell(repoRoot string, workspacePath string) (string, error)
	LaunchAgent(repoRoot string, workspacePath string, agentType string) (string, error)
}

// Server is the embedded HTTP/WebSocket server for the mobile companion PWA.
type Server struct {
	app     AppAPI
	termMgr *terminal.Manager
	httpSrv *http.Server
	token   string
	port    int

	upgrader websocket.Upgrader
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
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/projects", s.authMiddleware(s.handleProjects))
	mux.HandleFunc("/api/projects/info", s.authMiddleware(s.handleProjectInfo))
	mux.HandleFunc("/api/workspaces", s.authMiddleware(s.handleWorkspaces))
	mux.HandleFunc("/api/sessions", s.authMiddleware(s.handleSessions))
	mux.HandleFunc("/api/terminal", s.authMiddleware(s.handleTerminal))
	mux.HandleFunc("/api/shell", s.authMiddleware(s.handleShell))

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
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(ctx)
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
	sessions := s.app.RecoverSessions(repo, paths)
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
	writeJSON(w, map[string]string{"tmuxSession": tmuxSession})
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

	// Read messages from the client
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			// Client disconnected
			break
		}

		switch msg.Type {
		case "input":
			s.termMgr.Write(terminalID, msg.Data)
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				s.termMgr.Resize(terminalID, msg.Cols, msg.Rows)
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
