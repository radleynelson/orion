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
- **Dynamic agent buttons** — Define custom agents in config; they appear as sidebar buttons
- **Keyboard-driven** — Cmd+T, Cmd+W, Cmd+1-9, Cmd+\, Cmd+Shift+B

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

Create a `.orion.toml` in your project root to configure Orion for that repo.

### Full Example

```toml
# Files to copy into new worktrees
[credentials]
copy = [
  "backend/.env",
  "frontend/.env.local",
  "backend/config/credentials/*.key",  # glob patterns supported
]

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

# Agent definitions — each becomes a sidebar button
[agents.claude]
command = "claude --dangerously-skip-permissions"

[agents.codex]
command = "codex --dangerously-bypass-approvals-and-sandbox"

# Custom agents
[agents.reviewer]
command = "claude --dangerously-skip-permissions --prompt 'Review code changes for bugs'"

[agents.tests]
command = "./scripts/watch-tests.sh"
```

### Backward Compatibility

If no `.orion.toml` exists, Orion falls back to `.radconfig` (simple list of files to copy, one per line).

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
| `Cmd+[` | Focus previous pane |
| `Cmd+]` | Focus next pane |
| `Cmd+Shift+[` | Swap pane with previous |
| `Cmd+Shift+]` | Swap pane with next |
| `Cmd+1-9` | Switch to tab N |
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
