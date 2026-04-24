// Package notify turns Claude Code hook events into native macOS notifications.
//
// Wire: Claude Code invokes the installed hook script on Stop/Notification →
// script POSTs the hook JSON to Orion's local hook HTTP listener → Notifier
// dispatches a notification. When terminal-notifier is available, the
// notification carries a click callback that hits /focus so Orion raises its
// window and the frontend navigates to the originating tab.
//
// HTTP (rather than OSC escape sequences) is used because Claude runs inside
// tmux inside our PTY — tmux filters most escape sequences from reaching the
// outer PTY that Orion reads.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// hookPayload mirrors the JSON Claude Code sends on stdin to a hook, plus
// tmux_session appended by our hook script for tab routing.
type hookPayload struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
	Message       string `json:"message"`
	TmuxSession   string `json:"tmux_session"`
}

// Notifier owns the local HTTP listener and dispatches notifications.
type Notifier struct {
	mu              sync.Mutex
	ctx             context.Context
	server          *http.Server
	port            int
	notifierBinPath string // path to terminal-notifier if found, else ""
}

// New constructs a Notifier (HTTP listener is not yet started).
func New(ctx context.Context) *Notifier {
	return &Notifier{ctx: ctx}
}

// SetContext updates the Wails runtime context used for in-app event emission.
func (n *Notifier) SetContext(ctx context.Context) {
	n.mu.Lock()
	n.ctx = ctx
	n.mu.Unlock()
}

// Start binds a loopback listener, records the port to ~/.orion/hook-port, and
// serves the /hook + /focus endpoints.
func (n *Notifier) Start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("hook listener bind: %w", err)
	}
	addr := listener.Addr().(*net.TCPAddr)

	n.mu.Lock()
	n.port = addr.Port
	n.notifierBinPath = locateTerminalNotifier()
	n.mu.Unlock()

	if err := writeHookPort(addr.Port); err != nil {
		listener.Close()
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hook", n.handleHook)
	mux.HandleFunc("/focus", n.handleFocus)
	server := &http.Server{Handler: mux}

	n.mu.Lock()
	n.server = server
	n.mu.Unlock()

	if n.notifierBinPath == "" {
		log.Printf("notify: terminal-notifier not found on PATH — notifications will be display-only. `brew install terminal-notifier` to enable click-to-focus.")
	}

	go server.Serve(listener)
	return nil
}

// Stop shuts down the HTTP listener.
func (n *Notifier) Stop() {
	n.mu.Lock()
	s := n.server
	n.server = nil
	n.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

// Port returns the TCP port the hook listener is bound to (0 if not started).
func (n *Notifier) Port() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.port
}

func (n *Notifier) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var hp hookPayload
	if err := json.Unmarshal(body, &hp); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	log.Printf("notify: %s cwd=%s msg=%q", hp.HookEventName, hp.Cwd, hp.Message)
	n.dispatch(hp)
	w.WriteHeader(http.StatusNoContent)
}

// handleFocus is invoked when the user clicks a notification. It raises the
// Orion window and emits an `agent:focus` event so the frontend can switch
// to the originating tab.
func (n *Notifier) handleFocus(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cwd := q.Get("cwd")
	session := q.Get("session")
	tmuxSession := q.Get("tmux_session")
	log.Printf("notify: focus cwd=%s session=%s tmux=%s", cwd, session, tmuxSession)

	n.mu.Lock()
	ctx := n.ctx
	n.mu.Unlock()
	if ctx != nil {
		runtime.WindowShow(ctx)
		runtime.WindowUnminimise(ctx)
		runtime.EventsEmit(ctx, "agent:focus", map[string]string{
			"cwd":         cwd,
			"session":     session,
			"tmuxSession": tmuxSession,
		})
	}
	// Fallback app activation via AppleScript (brings Orion to front even when
	// it isn't the frontmost app). Best-effort.
	go activateApp()
	w.WriteHeader(http.StatusNoContent)
}

func (n *Notifier) dispatch(hp hookPayload) {
	title, body := formatMessage(hp)
	if body == "" {
		return
	}

	n.mu.Lock()
	ctx := n.ctx
	port := n.port
	bin := n.notifierBinPath
	n.mu.Unlock()

	go dispatchNative(bin, title, body, port, hp.Cwd, hp.SessionID, hp.TmuxSession)

	if ctx != nil {
		runtime.EventsEmit(ctx, "agent:notification", map[string]string{
			"event":       hp.HookEventName,
			"cwd":         hp.Cwd,
			"session":     hp.SessionID,
			"tmuxSession": hp.TmuxSession,
			"title":       title,
			"body":        body,
		})
	}
}

// formatMessage turns a hook payload into a notification title/body. Empty
// body means "ignore this event".
func formatMessage(hp hookPayload) (title, body string) {
	wsName := ""
	if hp.Cwd != "" {
		wsName = filepath.Base(hp.Cwd)
	}
	title = "Orion"
	if wsName != "" {
		title = "Orion — " + wsName
	}
	switch hp.HookEventName {
	case "Stop":
		body = "Claude is ready for input"
	case "Notification":
		body = hp.Message
		if body == "" {
			body = "Claude needs your attention"
		}
	default:
		return title, ""
	}
	return title, body
}

// dispatchNative fires a desktop notification. Uses terminal-notifier when
// available (for click-to-focus) and falls back to osascript (display-only).
func dispatchNative(bin, title, body string, port int, cwd, session, tmuxSession string) {
	if bin != "" {
		focusURL := fmt.Sprintf(
			"http://127.0.0.1:%d/focus?cwd=%s&session=%s&tmux_session=%s",
			port, urlEscape(cwd), urlEscape(session), urlEscape(tmuxSession),
		)
		execCmd := fmt.Sprintf("curl -sS -m 2 %q >/dev/null 2>&1", focusURL)
		cmd := exec.Command(bin,
			"-title", title,
			"-message", body,
			"-sound", "Pop",
			"-group", "orion-"+cwd,
			"-execute", execCmd,
		)
		if err := cmd.Run(); err == nil {
			return
		}
		// fall through to osascript if terminal-notifier failed
	}
	script := "display notification \"" + escapeAS(body) + "\" with title \"" + escapeAS(title) + "\" sound name \"Pop\""
	exec.Command("osascript", "-e", script).Run()
}

// activateApp brings Orion.app to the foreground. terminal-notifier's click
// handler only fires our -execute callback; it doesn't auto-focus Orion.
func activateApp() {
	exec.Command("osascript", "-e", `tell application "Orion" to activate`).Run()
}

func escapeAS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// urlEscape percent-encodes a string for use as a query-string value.
func urlEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// locateTerminalNotifier resolves the path to the terminal-notifier binary.
// Checks PATH first, then common Homebrew locations. Returns "" if absent.
func locateTerminalNotifier() string {
	if p, err := exec.LookPath("terminal-notifier"); err == nil {
		return p
	}
	for _, p := range []string{
		"/opt/homebrew/bin/terminal-notifier",
		"/usr/local/bin/terminal-notifier",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// writeHookPort records the hook listener port so the hook script can find it.
func writeHookPort(port int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".orion")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hook-port"), []byte(fmt.Sprintf("%d\n", port)), 0644)
}
