package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"orion/internal/claudesdk"
	"orion/internal/codexchat"
)

func TestKillSessionIgnoresImmediateChatAttachCleanup(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	workspace := t.TempDir()
	tmuxName := fmt.Sprintf("orion-web-kill-grace-test-%d", time.Now().UnixNano())
	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", workspace, "sh", "-lc", "sleep 60")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not create tmux session: %v %s", err, strings.TrimSpace(string(out)))
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	server := NewServer(nil, nil, codexchat.NewManager(), claudesdk.NewManager())
	server.markRecentChatAttach(tmuxName)

	body, err := json.Marshal(map[string]string{"tmuxSession": tmuxName})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/kill-session", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleKillSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !tmuxSessionExists(tmuxName) {
		t.Fatalf("tmux session %q was killed during chat attach grace window", tmuxName)
	}
	if !strings.Contains(rec.Body.String(), "ignored") {
		t.Fatalf("response = %s, want ignored", rec.Body.String())
	}
}

func TestCodexChatWebSocketStreamsAttachedTmuxTranscript(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	workspace := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("HOME", t.TempDir())

	inputPath := filepath.Join(workspace, "ws-input.txt")
	tmuxName := fmt.Sprintf("orion-web-codex-test-%d", time.Now().UnixNano())
	tmuxCommand := fmt.Sprintf(`IFS= read -r line; printf "%%s" "$line" > %s; sleep 60`, shellQuote(inputPath))
	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", workspace, "sh", "-lc", tmuxCommand)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not create tmux session: %v %s", err, strings.TrimSpace(string(out)))
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	transcriptPath := filepath.Join(codexHome, "sessions", "2026", "05", "15", "websocket.jsonl")
	writeTranscriptLines(t, transcriptPath,
		`{"type":"session_meta","timestamp":"2026-05-15T10:00:00Z","payload":{"id":"thread-ws","cwd":"`+workspace+`"}}`,
		`{"type":"event_msg","timestamp":"2026-05-15T10:00:01Z","payload":{"type":"user_message","message":"initial websocket prompt","images":[],"local_images":[]}}`,
	)

	codexMgr := codexchat.NewManager()
	info, err := codexMgr.AttachSince(tmuxName, workspace, "Codex", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("AttachSince: %v", err)
	}
	session, ok := codexMgr.Get(info.ID)
	if !ok {
		t.Fatalf("attached session %q not found", info.ID)
	}
	defer session.Detach()

	server := NewServer(nil, nil, codexMgr, claudesdk.NewManager())
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleCodexChatWS))
	defer httpServer.Close()

	wsURL := websocketURL(t, httpServer.URL, "/ws/codex-chat/"+url.PathEscape(tmuxName), map[string]string{
		"token":         server.token,
		"workspacePath": workspace,
	})
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	requireWSMessage(t, conn, "user", "", "initial websocket prompt")

	appendTranscriptLines(t, transcriptPath,
		`{"type":"event_msg","timestamp":"2026-05-15T10:00:02Z","payload":{"type":"task_started"}}`,
		`{"type":"response_item","timestamp":"2026-05-15T10:00:03Z","payload":{"type":"function_call","call_id":"call_ws","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}"}}`,
		`{"type":"response_item","timestamp":"2026-05-15T10:00:04Z","payload":{"type":"function_call_output","call_id":"call_ws","output":"`+workspace+`"}}`,
		`{"type":"event_msg","timestamp":"2026-05-15T10:00:05Z","payload":{"type":"agent_message","message":"Websocket update rendered.","phase":"final_answer"}}`,
	)
	if err := session.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	requireWSMessage(t, conn, "status", "", "")
	requireWSMessage(t, conn, "tool", "Bash", "pwd")
	requireWSMessage(t, conn, "tool_result", "Bash", workspace)
	requireWSMessage(t, conn, "assistant", "", "Websocket update rendered.")

	if err := conn.WriteJSON(map[string]any{"type": "input", "text": "hello from websocket"}); err != nil {
		t.Fatalf("write websocket input: %v", err)
	}
	waitForFileContent(t, inputPath, "hello from websocket")
}

func websocketURL(t *testing.T, base string, path string, query map[string]string) string {
	t.Helper()
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Scheme = "ws"
	parsed.Path = path
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func requireWSMessage(t *testing.T, conn *websocket.Conn, typ string, toolName string, contains string) codexchat.Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		var msg codexchat.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("websocket closed while waiting for %s", typ)
			}
			continue
		}
		if msg.Type != typ {
			continue
		}
		if toolName != "" && msg.ToolName != toolName {
			continue
		}
		if contains != "" && !strings.Contains(msg.Text, contains) && !strings.Contains(msg.Details, contains) {
			continue
		}
		return msg
	}
	t.Fatalf("missing websocket message type=%q tool=%q contains=%q", typ, toolName, contains)
	return codexchat.Message{}
}

func writeTranscriptLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTranscriptLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
}

func waitForFileContent(t *testing.T, path string, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if strings.TrimSpace(string(data)) == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("file %s = %q, want %q", path, strings.TrimSpace(string(data)), want)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
