package tmuxutil

import "strings"

// WrapInitialCommand turns a command into a tmux shell-command. This avoids
// feeding startup commands through pane stdin, where interactive shell startup
// prompts can consume the first byte before the shell prompt is ready.
func WrapInitialCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}

	script := strings.Join([]string{
		"set +e",
		command,
		"orion_status=$?",
		`printf '\n[orion] command exited with status %d. Returning to shell.\n' "$orion_status"`,
		`exec "${SHELL:-/bin/zsh}" -l`,
	}, "\n")

	return `exec "${SHELL:-/bin/zsh}" -lc ` + shellQuote(script)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
