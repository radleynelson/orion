package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"orion/internal/claudesdk"
	"orion/internal/codexchat"
)

func TestClaudeChatWebSocketStreamsAttachedTmuxTranscript(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	workspace := t.TempDir()
	claudeConfig := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeConfig)
	t.Setenv("HOME", t.TempDir())

	inputPath := filepath.Join(workspace, "ws-input.txt")
	tmuxName := fmt.Sprintf("orion-web-claude-test-%d", time.Now().UnixNano())
	tmuxCommand := fmt.Sprintf(`IFS= read -r line; printf "%%s" "$line" > %s; sleep 60`, shellQuote(inputPath))
	cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", workspace, "sh", "-lc", tmuxCommand)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not create tmux session: %v %s", err, strings.TrimSpace(string(out)))
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	threadID := "thread-claude-ws"
	transcriptPath := filepath.Join(claudeConfig, "projects", claudeProjectDirForTest(workspace), threadID+".jsonl")
	writeTranscriptLines(t, transcriptPath,
		`{"type":"user","uuid":"user-ws","cwd":`+quoteJSONForWebTest(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"initial Claude websocket prompt"}]}}`,
	)
	if out, err := exec.Command("tmux", "set-option", "-t", tmuxName, "@orion_thread_id", threadID).CombinedOutput(); err != nil {
		t.Fatalf("set tmux thread option: %v %s", err, strings.TrimSpace(string(out)))
	}

	claudeMgr := claudesdk.NewManager()
	info, err := claudeMgr.AttachSince(tmuxName, workspace, "Claude", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("AttachSince: %v", err)
	}
	session, ok := claudeMgr.Get(info.ID)
	if !ok {
		t.Fatalf("attached session %q not found", info.ID)
	}
	defer session.Detach()

	server := NewServer(nil, nil, codexchat.NewManager(), claudeMgr)
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleClaudeChatWS))
	defer httpServer.Close()

	wsURL := websocketURL(t, httpServer.URL, "/ws/claude-chat/"+url.PathEscape(tmuxName), map[string]string{
		"token":         server.token,
		"workspacePath": workspace,
	})
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	requireClaudeWSMessage(t, conn, "user", "", "initial Claude websocket prompt")

	appendTranscriptLines(t, transcriptPath,
		`{"type":"assistant","uuid":"assistant-ws","cwd":`+quoteJSONForWebTest(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:01Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"tool_use","id":"tool-ws","name":"Bash","input":{"command":"pwd","description":"print cwd"}}]}}`,
		`{"type":"user","uuid":"result-ws","cwd":`+quoteJSONForWebTest(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-ws","content":`+quoteJSONForWebTest(workspace)+`,"is_error":false}]}}`,
		`{"type":"assistant","uuid":"answer-ws","cwd":`+quoteJSONForWebTest(workspace)+`,"sessionId":"`+threadID+`","timestamp":"2026-05-15T10:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"Claude websocket update rendered."}]}}`,
	)
	if err := session.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	requireClaudeWSMessage(t, conn, "tool", "Bash", "pwd")
	requireClaudeWSMessage(t, conn, "tool_result", "Bash", workspace)
	requireClaudeWSMessage(t, conn, "assistant", "", "Claude websocket update rendered.")

	if err := conn.WriteJSON(map[string]any{"type": "input", "text": "hello from Claude websocket"}); err != nil {
		t.Fatalf("write websocket input: %v", err)
	}
	waitForFileContent(t, inputPath, "hello from Claude websocket")
}

func requireClaudeWSMessage(t *testing.T, conn *websocket.Conn, typ string, toolName string, contains string) claudesdk.Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		var msg claudesdk.Message
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
	t.Fatalf("missing Claude websocket message type=%q tool=%q contains=%q", typ, toolName, contains)
	return claudesdk.Message{}
}

func claudeProjectDirForTest(path string) string {
	dir := strings.ReplaceAll(path, string(filepath.Separator), "-")
	return strings.ReplaceAll(dir, "_", "-")
}

func quoteJSONForWebTest(value string) string {
	quoted := strings.ReplaceAll(value, `\`, `\\`)
	quoted = strings.ReplaceAll(quoted, `"`, `\"`)
	return `"` + quoted + `"`
}
