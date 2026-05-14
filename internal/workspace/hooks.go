package workspace

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orion/internal/applog"
	"orion/internal/config"
)

const (
	hookWorktreeCreated  = "worktree_created"
	hookWorktreeDeleting = "worktree_deleting"
)

type hookContext struct {
	Name             string
	Branch           string
	BaseRef          string
	WorkspacePath    string
	RepoRoot         string
	MainWorktreePath string
}

type hookExecutionError struct {
	Event      string
	Command    string
	ExitStatus int
	OutputTail string
	LogPath    string
	Err        error
}

func (e *hookExecutionError) Error() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("hook %s failed", e.Event))
	if e.ExitStatus >= 0 {
		parts = append(parts, fmt.Sprintf("exit status %d", e.ExitStatus))
	} else if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	parts = append(parts, fmt.Sprintf("command: %s", e.Command))
	if e.LogPath != "" {
		parts = append(parts, fmt.Sprintf("log: %s", e.LogPath))
	}
	if e.OutputTail != "" {
		parts = append(parts, fmt.Sprintf("output:\n%s", e.OutputTail))
	}
	return strings.Join(parts, "\n")
}

func runHook(event string, hook config.HookConfig, hookCtx hookContext, defaultBlocking bool) error {
	command := strings.TrimSpace(hook.Command)
	if command == "" {
		return nil
	}

	rendered := renderHookCommand(command, hookCtx)
	result := executeHook(event, rendered, hookCtx)
	if result.err == nil {
		return nil
	}

	hookErr := &hookExecutionError{
		Event:      event,
		Command:    rendered,
		ExitStatus: result.exitStatus,
		OutputTail: hookOutputTail(result.stdout, result.stderr, 4000),
		LogPath:    result.logPath,
		Err:        result.err,
	}

	if hook.IsBlocking(defaultBlocking) {
		return hookErr
	}

	applog.Warnf("%v", hookErr)
	return nil
}

type hookResult struct {
	stdout     string
	stderr     string
	logPath    string
	exitStatus int
	err        error
}

func executeHook(event string, command string, hookCtx hookContext) hookResult {
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = hookCtx.WorkspacePath
	cmd.Env = hookEnv(os.Environ(), hookCtx)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := hookResult{
		stdout:     stdout.String(),
		stderr:     stderr.String(),
		exitStatus: 0,
		err:        err,
	}
	if err != nil {
		result.exitStatus = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.exitStatus = exitErr.ExitCode()
		}
	}

	if logPath, logErr := writeHookLog(event, hookCtx, command, result); logErr == nil {
		result.logPath = logPath
	} else {
		applog.Warnf("failed to write %s hook log for %s: %v", event, hookCtx.WorkspacePath, logErr)
	}

	return result
}

func renderHookCommand(command string, hookCtx hookContext) string {
	replacements := map[string]string{
		"{{name}}":               shellQuote(hookCtx.Name),
		"{{branch}}":             shellQuote(hookCtx.Branch),
		"{{base_ref}}":           shellQuote(hookCtx.BaseRef),
		"{{workspace_path}}":     shellQuote(hookCtx.WorkspacePath),
		"{{repo_root}}":          shellQuote(hookCtx.RepoRoot),
		"{{main_worktree_path}}": shellQuote(hookCtx.MainWorktreePath),
	}
	for placeholder, value := range replacements {
		command = strings.ReplaceAll(command, placeholder, value)
	}
	return command
}

func hookEnv(base []string, hookCtx hookContext) []string {
	values := map[string]string{
		"ORION_WORKSPACE_NAME":     hookCtx.Name,
		"ORION_BRANCH":             hookCtx.Branch,
		"ORION_BASE_REF":           hookCtx.BaseRef,
		"ORION_WORKSPACE_PATH":     hookCtx.WorkspacePath,
		"ORION_REPO_ROOT":          hookCtx.RepoRoot,
		"ORION_MAIN_WORKTREE_PATH": hookCtx.MainWorktreePath,
	}
	keys := []string{
		"ORION_WORKSPACE_NAME",
		"ORION_BRANCH",
		"ORION_BASE_REF",
		"ORION_WORKSPACE_PATH",
		"ORION_REPO_ROOT",
		"ORION_MAIN_WORKTREE_PATH",
	}

	env := make([]string, 0, len(base)+len(keys))
	for _, entry := range base {
		keep := true
		for _, key := range keys {
			if strings.HasPrefix(entry, key+"=") {
				keep = false
				break
			}
		}
		if keep {
			env = append(env, entry)
		}
	}
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func writeHookLog(event string, hookCtx hookContext, command string, result hookResult) (string, error) {
	logRoot := hookCtx.WorkspacePath
	if event == hookWorktreeDeleting && hookCtx.MainWorktreePath != "" {
		logRoot = hookCtx.MainWorktreePath
	}

	dir := filepath.Join(logRoot, ".orion", "hooks")
	if event == hookWorktreeDeleting {
		dir = filepath.Join(dir, sanitize(filepath.Base(hookCtx.WorkspacePath)))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	ensureWorkspaceGitignore(logRoot, ".orion/")

	logPath := filepath.Join(dir, fmt.Sprintf("%s-%d.log", event, time.Now().UnixNano()))
	var lines []string
	lines = append(lines, "event: "+event)
	lines = append(lines, "command: "+command)
	lines = append(lines, fmt.Sprintf("exit_status: %d", result.exitStatus))
	if result.err != nil {
		lines = append(lines, "error: "+result.err.Error())
	}
	lines = append(lines, "", "stdout:", result.stdout, "", "stderr:", result.stderr, "")
	return logPath, os.WriteFile(logPath, []byte(strings.Join(lines, "\n")), 0644)
}

func hookOutputTail(stdout string, stderr string, limit int) string {
	var combined []string
	if strings.TrimSpace(stdout) != "" {
		combined = append(combined, "stdout:\n"+strings.TrimRight(stdout, "\n"))
	}
	if strings.TrimSpace(stderr) != "" {
		combined = append(combined, "stderr:\n"+strings.TrimRight(stderr, "\n"))
	}
	text := strings.Join(combined, "\n\n")
	if len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}

func workspaceNameFromPath(repoName string, workspacePath string) string {
	name := filepath.Base(workspacePath)
	prefix := repoName + "-"
	return strings.TrimPrefix(name, prefix)
}

func getWorktreeBranch(workspacePath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workspacePath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return ""
	}
	return branch
}

func ensureWorkspaceGitignore(workspacePath, pattern string) {
	gitignorePath := filepath.Join(workspacePath, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err == nil && strings.Contains(string(data), pattern) {
		return
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(data) > 0 && data[len(data)-1] != '\n' {
		_, _ = f.WriteString("\n")
	}
	_, _ = f.WriteString(pattern + "\n")
}
