package tmuxutil

import (
	"os/exec"
	"strings"
)

// ConfigureExtendedKeys makes tmux preserve modified key sequences such as
// Shift+Enter for TUI apps that support CSI-u input.
func ConfigureExtendedKeys() {
	run("set-option", "-g", "extended-keys", "always")
	run("set-option", "-g", "extended-keys-format", "csi-u")
	if out, err := exec.Command("tmux", "show-options", "-g", "terminal-features").CombinedOutput(); err != nil || !strings.Contains(string(out), "xterm*:extkeys") {
		run("set-option", "-ga", "terminal-features", ",xterm*:extkeys")
	}
}

// ConfigureSessionExtendedKeys applies the same settings to an existing
// session/window. Existing tmux sessions do not always inherit later global
// option changes.
func ConfigureSessionExtendedKeys(session string) {
	ConfigureExtendedKeys()
	session = strings.TrimSpace(session)
	if session == "" {
		return
	}
	run("set-window-option", "-t", session, "extended-keys", "always")
	run("set-window-option", "-t", session, "extended-keys-format", "csi-u")
}

func run(args ...string) {
	_ = exec.Command("tmux", args...).Run()
}
