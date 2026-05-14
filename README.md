# Orion

A workspace manager for agentic coding. Orion wraps git worktrees into isolated development environments with their own servers, ports, and AI agent sessions — all managed from a single terminal-styled desktop app.

Built with Go + React + xterm.js on [Wails](https://wails.io).

## Features

- **Workspace management** — Create, switch, and delete git worktrees from a sidebar
- **Isolated servers** — Each workspace runs its own frontend/backend/workers on unique ports
- **One-click agents** — Launch Claude Code, Codex, or custom agents with a single click
- **Native copy/paste** — xterm.js terminals with full clipboard support (no TUI copy/paste pain)
- **Port isolation** — Main branch gets default ports (3000, 5173), worktrees get random isolated ports
- **Credential copying** — Automatically copies .env files, API keys, and credentials into new worktrees
- **Session persistence** — Close and reopen Orion; running tmux sessions reconnect automatically
- **Browser integration** — One click opens Chrome at the right frontend URL for any workspace
- **Integrated code editor** — Monaco editor with LSP-backed completions, diagnostics, hover, definition navigation, and save-time formatting hooks
- **Dynamic agent buttons** — Define custom agents in config; they appear as sidebar buttons
- **Keyboard-driven** — Cmd+T, Cmd+W, Cmd+B, Cmd+1, Cmd+\, Cmd+Shift+B

## Quick Start

```bash
# Install prerequisites
brew install go node tmux

# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Clone and build
cd orion
wails build

# Launch
open build/bin/Orion.app
```

On first launch, click **"Open project..."** in the sidebar and select a git repository.

## Configuration

Orion uses `.orion.toml` in your project root to configure that repo. If you open a git repository that does not have `.orion.toml`, Orion creates a small default config with Claude and Codex agents enabled at full-permission settings. If the repo has an older `.orion.config.toml`, Orion can still read it, but new projects should use `.orion.toml`.

### Full Example

```toml
# Files to copy into new worktrees
[credentials]
copy = [
  "backend/.env",
  "frontend/.env.local",
  "backend/config/credentials/*.key",  # glob patterns supported
]

# Workspace lifecycle callbacks
[hooks.worktree_created]
command = "./scripts/provision-preview-db.sh --branch {{branch}} --path {{workspace_path}}"
# blocking defaults to true for create hooks

[hooks.worktree_deleting]
command = "./scripts/destroy-preview-db.sh --branch {{branch}}"
blocking = false

# Server definitions
[servers.frontend]
command = "npm install && npm run dev"
dir = "frontend"                        # working directory (relative to worktree root)
default_port = 5173                     # used for main branch
port_env = "PORT"                       # env var injected with assigned port

[servers.backend]
command = "bin/rails server"
dir = "backend"
default_port = 3000
port_env = "PORT"

[servers.sidekiq]
command = "bundle exec sidekiq -C config/sidekiq/all.yml"
dir = "backend"
# no port needed — connects to Redis

# Cross-server environment variables
# {{backend.port}} resolves to the backend's assigned port
[servers.frontend.env]
NEXT_PUBLIC_API_URL = "http://localhost:{{backend.port}}/api/"

# Agent definitions. Claude/Codex agents are provider-aware: Orion can start
# the same configured agent as either a terminal session or a chat session.
[agents.claude]
label = "Claude"
provider = "claude"
command = "claude --dangerously-skip-permissions"
permission_mode = "bypassPermissions"

[agents.codex]
label = "Codex"
provider = "codex"
command = "codex --dangerously-bypass-approvals-and-sandbox"
reasoning_effort = "xhigh"
approval_policy = "never"
sandbox_mode = "danger-full-access"
collaboration_mode = "default"

# Command-only agents are terminal-only unless they set provider = "claude" or "codex".
[agents.reviewer]
label = "Review"
provider = "claude"
icon = "reviewer"
command = "claude --dangerously-skip-permissions --prompt 'Review code changes for bugs'"

[agents.tests]
label = "Tests"
icon = "test"
command = "./scripts/watch-tests.sh"

# Optional LSP override. Without this, Orion uses built-in defaults for
# TypeScript/JavaScript, Go, Ruby, CSS, HTML, and JSON.
[lsp.typescript]
command = "frontend/node_modules/.bin/typescript-language-server --stdio"
extensions = [".ts", ".tsx", ".js", ".jsx"]
```

### Agent Providers

Agents are the source of truth for Claude, Codex, and custom launch behavior. `provider` tells Orion what kind of agent the entry represents:

| Provider | Supported views | Behavior |
|----------|-----------------|----------|
| `claude` | terminal + chat | Terminal uses `command`; chat uses the Claude Code SDK bridge with the same model/reasoning/permission defaults. |
| `codex` | terminal + chat | Terminal uses `command`; chat uses `codex app-server` with the same model/reasoning/approval defaults. |
| unset/custom | terminal only | Orion runs `command` in tmux and does not offer chat conversion. |

Desktop defaults to terminal when creating a configured agent. Mobile defaults Claude/Codex agents to chat. After a session exists, its current view is session state: switching from terminal to chat, or chat to terminal, is explicit and keeps the same provider thread when possible.

If you omit `provider`, Orion infers it only for agents named `claude` or `codex` for backward compatibility. Other command-only agents stay terminal-only.

Model is optional. Leave `model` out to let Claude Code or Codex use their current default model; set it only when you intentionally want to pin a specific model for that project or agent.

Agent icons are optional. If an agent has `provider = "claude"` or `provider = "codex"` and no `icon`, Orion shows the provider icon. Set `icon` to use a custom role icon instead. Current role icons: `reviewer`, `scribe`, `plan`, `test`, `debug`, `deploy`, `ops`, `data`, `design`, `security`, `browser`, `automate`, `branch`, `docs`, `clean`, `shell`, `server`, `editor`, `diagnostics`.

### LSP Support

Orion starts language servers lazily when an editable file is opened in Monaco. The frontend keeps Monaco models on real `file://` URIs, sends `didOpen` / `didChange` / `didSave` / `didClose` notifications through Wails-bound Go methods, and the Go `internal/lsp` manager talks to the language server over stdio. Server responses flow back to Monaco through Wails events.

LSP support can provide diagnostics, completions, hover, go to definition, references, signature help, document symbols, and semantic highlighting when the language server supports those capabilities. In the editor, `Cmd+B` goes to definition and `Cmd+[` / `Cmd+]` navigate editor history.

Built-in defaults cover TypeScript/JavaScript, Go, Ruby, CSS, HTML, and JSON. Orion does not bundle language servers; it resolves project or system tools. For TypeScript, Orion first looks for `frontend/node_modules/.bin/typescript-language-server` in the active worktree, then `node_modules/.bin/typescript-language-server`, then `typescript-language-server` on `PATH`. Installing language servers as project dev dependencies keeps Orion lightweight and makes worktrees behave like the app itself.

You can override a language server in `.orion.toml` with `[lsp.<language>]`. `command` is parsed as an executable plus args, not as a shell command; use a wrapper script when you need shell features.

### Workspace Callbacks

Orion can run shell callbacks around worktree lifecycle events. Use these when a workspace needs matching external resources, such as a preview database, cloud branch, seeded cache, or cleanup job.

```toml
[hooks.worktree_created]
command = "./scripts/provision-preview-db.sh --branch {{branch}} --workspace {{workspace_path}}"

[hooks.worktree_deleting]
command = "./scripts/destroy-preview-db.sh --branch {{branch}}"
blocking = false
```

Available callbacks:

| Callback | Runs | Default blocking behavior |
|----------|------|---------------------------|
| `hooks.worktree_created` | After `git worktree add` and credential copying, before Orion reports the workspace as ready | Blocking. A failure stops workspace creation and leaves the worktree in place for inspection. |
| `hooks.worktree_deleting` | Before `git worktree remove --force` | Non-blocking. A failure is logged and deletion continues unless `blocking = true` is set. |

Each hook runs with `bash -lc` from the workspace directory. The command can use shell-quoted template placeholders:

| Placeholder | Meaning |
|-------------|---------|
| `{{name}}` | Workspace name typed in Orion, without the repo prefix |
| `{{branch}}` | Git branch for the workspace |
| `{{base_ref}}` | Base ref used to create the workspace, when available |
| `{{workspace_path}}` | Full path to the worktree |
| `{{repo_root}}` | Repository root passed to Orion |
| `{{main_worktree_path}}` | Main worktree path |

The same values are also available as environment variables: `ORION_WORKSPACE_NAME`, `ORION_BRANCH`, `ORION_BASE_REF`, `ORION_WORKSPACE_PATH`, `ORION_REPO_ROOT`, and `ORION_MAIN_WORKTREE_PATH`.

Hook output is written under `.orion/hooks/`, and `.orion/` is added to `.gitignore` automatically. Delete-hook logs are written under the main worktree because the target worktree is about to be removed.

### Backward Compatibility

If a repo only has `.radconfig` (simple list of files to copy, one per line), Orion uses that list to seed the generated `.orion.toml` credentials section.

### Port Behavior

| Workspace | Port Strategy |
|-----------|--------------|
| **main** | Uses `default_port` from config (e.g., 5173, 3000) |
| **worktrees** | Random ports from 10000-60000 range |

Port allocations are persisted to `~/.orion/ports.json` so external tools (MCP, browser automation) can discover which workspace is running on which port.

### Environment Sharing

When servers start, Orion writes `.orion/env.sh` in the workspace directory with all port assignments:

```bash
export FRONTEND_PORT=21814
export FRONTEND_URL=http://localhost:21814
export BACKEND_PORT=37792
export BACKEND_URL=http://localhost:37792
export NEXT_PUBLIC_API_URL=http://localhost:37792/api/
```

This file is **automatically sourced** in every new shell and agent session, so Claude Code and Codex always know which ports the servers are running on. The `.orion/` directory is auto-added to `.gitignore`.

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Cmd+T` | New shell tab |
| `Cmd+W` | Close focused pane (closes tab if last pane) |
| `Cmd+D` | Split pane right (vertical) |
| `Cmd+Shift+D` | Split pane down (horizontal) |
| `Cmd+[` | Editor back when Monaco is focused; otherwise focus previous pane |
| `Cmd+]` | Editor forward when Monaco is focused; otherwise focus next pane |
| `Cmd+Shift+[` | Swap pane with previous |
| `Cmd+Shift+]` | Swap pane with next |
| `Cmd+B` | Go to definition in the editor |
| `Cmd+Left/Right` | Cycle tabs |
| `Cmd+1` | Toggle workspace sidebar |
| Drag tab → tab | Merge tabs into split view |
| `Cmd+\` | Toggle sidebar |
| `Cmd+Shift+B` | Open browser for active workspace |

## Architecture

Orion uses tmux under the hood for session resilience. Each terminal tab is an xterm.js instance attached to a tmux session via a Go-managed PTY. This means:

- **Native copy/paste** — xterm.js handles clipboard, no terminal copy-mode needed
- **Session survival** — if Orion crashes, tmux sessions keep running; reopen to reconnect
- **Full terminal** — SSH, vim, everything works as expected
- **No zombies** — closing a tab kills the tmux session and all its processes

```
xterm.js (React) ←→ Wails Events ←→ Go PTY ←→ tmux session ←→ process
```

## Development

```bash
# Run in dev mode (hot reload)
wails dev

# Build production app
wails build

# Regenerate Go→JS bindings after changing Go methods
wails generate module

# Type check frontend
cd frontend && npx tsc --noEmit
```

## Tech Stack

- **Go** — Backend, PTY management, git operations, tmux orchestration
- **React + TypeScript** — Frontend UI
- **xterm.js** — Terminal emulator (WebGL-accelerated)
- **Wails v2** — Desktop app framework (native macOS webview, no Electron)
- **tmux** — Session persistence layer
- **Zustand** — Frontend state management
