# Orion

Orion is a Wails v2 desktop app (Go + React/TypeScript) that manages agentic coding workspaces. It wraps git worktrees, tmux sessions, and xterm.js terminals into a unified GUI with port isolation, credential management, and one-click agent launching.

## Radley's Codex Shared Remote Host

This is a personal, machine-local service on Radley's Mac. It is not an Orion
project dependency and must not be turned into a required setup for coworkers.
Coworkers who opt in should use `scripts/codex-remote install`; the generic
installer creates their own profile and LaunchAgent without copying Radley's
MCP configuration or credentials.

- Codex has one standalone installation at `~/.local/bin/codex`.
- The shared Orion host uses the isolated profile
  `CODEX_HOME=~/.codex-orion-remote` so it does not collide with the ChatGPT
  desktop app's existing Remote host.
- The persistent server is a Codex remote-control daemon listening at
  `~/.codex-orion-remote/app-server-control/app-server-control.sock`.
- `~/Library/LaunchAgents/com.radley.codex-orion-remote.plist` starts the host at
  macOS login. The daemon stays running when Orion and its TUI tabs close.
- The LaunchAgent sets `SoftResourceLimits.NumberOfFiles = 8192` for the daemon.
  Do not remove this process-local limit: macOS otherwise supplies a soft limit
  of 256, which the persistent daemon can exceed while loading skills and
  per-thread MCP processes.
- The LaunchAgent calls `~/.orion/scripts/codex-orion-remote-service`. That
  wrapper loads `BETTERSTACK_API_TOKEN` from the macOS Keychain service
  `com.radley.codex-orion-remote.betterstack` and prepends the Ruby 3.4 rbenv
  shims needed by Review Town. Never place the token directly in the plist,
  wrapper, or Codex config.
- The login LaunchAgent is intentionally one-shot. `launchctl` normally shows
  it as `state = not running` with `last exit code = 0` after it hands off to
  the persistent Codex daemon.
- `~/.orion/scripts/codex-shared-remote` is an idempotent launcher: it starts the
  daemon if needed and attaches the TUI with `codex --remote unix://`.
- Slant exposes that launcher as the private, gitignored
  `[agents.codex_shared_remote]` entry labeled `Codex Shared Remote`.

Use the isolated `CODEX_HOME` for every management command. Starting remote
control against ordinary `~/.codex` collides with the desktop app host and can
return `409 Remote app server already online`.

```bash
# Health/status
CODEX_HOME="$HOME/.codex-orion-remote" codex doctor --json --summary
CODEX_HOME="$HOME/.codex-orion-remote" codex remote-control start --json
launchctl print "gui/$(id -u)/com.radley.codex-orion-remote"

# Restart through the LaunchAgent so its credentials, PATH, and limits apply.
# First confirm no connected clients or active turns would be interrupted.
CODEX_HOME="$HOME/.codex-orion-remote" codex remote-control stop --json
launchctl kickstart "gui/$(id -u)/com.radley.codex-orion-remote"

# Generate a short-lived device pairing code
CODEX_HOME="$HOME/.codex-orion-remote" codex remote-control pair --json

# Stop only when Radley explicitly asks
CODEX_HOME="$HOME/.codex-orion-remote" codex remote-control stop --json
```

Pairing codes expire; completed device pairings persist until revoked or the
account signs out. Never commit pairing codes, `auth.json`, or files from the
isolated profile. The host is unavailable while the Mac is logged out, asleep,
offline, or powered off. The experimental profile currently disables the Slant
and Sentry OAuth MCP entries until they are authenticated separately.

### Shared Codex/Orion worktree lifecycle

Slant intentionally uses Orion's default sibling worktree layout. Orion-created
worktrees therefore have human-readable paths such as `slant-feature-name`,
which keeps Codex `/resume` history easy to distinguish by working directory.
This layout choice is independent of the shared Codex Remote host. Codex-created
worktrees may still use Codex's nested `<worktree-root>/<short-id>/slant`
layout and are adopted by Orion when opened.

Orion is the sole environment owner in both directions:

- Orion creation copies credentials, runs `hooks.worktree_created` (including
  the PostgreSQL database setup), allocates ports/Redis, and writes
  `.orion/env.sh`.
- A Codex-created checkout runs Slant's `.codex/environments/environment.toml`,
  which calls `~/.orion/scripts/orion-worktree`. The script asks Orion's
  authenticated local API to adopt the checkout and runs the identical
  provisioning path. It starts Orion automatically if needed.
- Adoption is idempotent; reopening an environment does not rerun the database
  hook. Codex cleanup asks Orion to stop servers, release runtime allocations,
  and run `hooks.worktree_deleting` before Codex removes its checkout.

The old `~/.codex/scripts/slant-worktree` implementation is superseded and must
not be used for new environments; it duplicated database, port, Redis, and
server management.

## Tech Stack

- **Backend**: Go (Wails v2 framework)
- **Frontend**: React + TypeScript + Vite
- **Terminal**: xterm.js with WebGL renderer + creack/pty for PTY management
- **Session layer**: tmux (managed invisibly — users never interact with tmux directly)
- **Config**: TOML (.orion.toml per-repo) with .radconfig backward compatibility
- **State**: JSON files in ~/.orion/ (ports.json, state.json)

## Architecture

```
User interacts with xterm.js (React)
  ↕ Wails Events (base64-encoded terminal I/O)
Go backend manages PTYs
  ↕ PTY runs `tmux attach -t <session>`
tmux sessions hold the actual processes (servers, agents, shells)
```

Each terminal tab is an xterm.js instance connected to a Go-managed PTY that attaches to a tmux session. This gives native copy/paste, session persistence (tmux survives app crashes), and full terminal capability.

**Closing a tab kills the tmux session** — no zombie processes. Users can always restart servers or resume agent sessions.

## Project Structure

```
orion/
├── main.go                              # Wails entry point, window config, Mac options
├── app.go                               # Main App struct bound to Wails (all frontend-callable methods)
├── internal/
│   ├── terminal/manager.go              # PTY lifecycle, tmux attach, I/O streaming via Wails Events
│   ├── workspace/manager.go             # Git worktree CRUD, credential copying, agent/shell launching
│   ├── config/config.go                 # .orion.toml parser (falls back to .radconfig)
│   ├── port/manager.go                  # Random port allocation, persistence to ~/.orion/ports.json
│   ├── server/manager.go                # Server process lifecycle, port injection, env template resolution
│   └── state/state.go                   # App state persistence, tmux session recovery on restart
├── frontend/
│   ├── src/
│   │   ├── App.tsx                      # Root layout: titlebar + sidebar + terminal area + status bar
│   │   ├── App.css                      # All component styles (Warp-inspired dark theme)
│   │   ├── style.css                    # CSS variables, global resets, scrollbar styling
│   │   ├── components/
│   │   │   ├── Sidebar.tsx              # Workspace list, agent buttons, server controls, project picker
│   │   │   ├── Terminal.tsx             # xterm.js wrapper with resize observer
│   │   │   ├── SplitPane.tsx            # Recursive split pane renderer (terminals + editors)
│   │   │   ├── ActivityBar.tsx          # Far-left icon bar: Workspaces, Files, Search, Git
│   │   │   ├── FileExplorer.tsx         # Lazy-loading file tree sidebar
│   │   │   ├── GitPanel.tsx             # Git changes list with status badges
│   │   │   ├── GlobalSearch.tsx         # Content search sidebar (ripgrep-powered)
│   │   │   ├── SearchEverywhere.tsx     # Fuzzy file finder modal (double-tap Shift)
│   │   │   ├── MonacoEditor.tsx         # Editable Monaco code editor with LSP support
│   │   │   └── DiffViewer.tsx           # Side-by-side git diff viewer (Monaco DiffEditor)
│   │   ├── store/index.ts              # Zustand state: project, workspaces, tabs, panes, sidebar mode
│   │   └── lib/
│   │       ├── terminal.ts             # xterm.js setup, theme, event wiring, Unicode support
│   │       ├── lspClient.ts            # Monaco ↔ Wails ↔ Go LSP client bridge
│   │       ├── monacoTheme.ts          # Orion dark theme + enhanced Ruby tokenizer for Monaco
│   │       └── languages.ts            # File extension → Monaco language mapping
│   └── wailsjs/                        # Auto-generated Go bindings (DO NOT EDIT)
├── internal/
│   ├── terminal/manager.go              # PTY lifecycle, tmux attach, I/O streaming via Wails Events
│   ├── workspace/manager.go             # Git worktree CRUD, credential copying, agent/shell launching
│   ├── config/config.go                 # .orion.toml parser (falls back to .radconfig)
│   ├── port/manager.go                  # Random port allocation, persistence to ~/.orion/ports.json
│   ├── server/manager.go               # Server process lifecycle, port injection, env template resolution
│   ├── lsp/manager.go                  # Language server process lifecycle and JSON-RPC stdio bridge
│   ├── plugin/manager.go               # Formatters, linters, and on-save hooks
│   ├── state/state.go                   # Per-project state persistence, tmux session recovery
│   ├── files/manager.go                 # File listing, reading, fuzzy search, content search (ripgrep)
│   └── git/manager.go                   # Git status, file diff for Monaco DiffEditor
├── build/
│   ├── appicon.png                      # Orion constellation icon (1024x1024)
│   ├── appicon.icns                     # macOS icon bundle
│   └── icon.svg                         # Source SVG for icon
└── wails.json                           # Wails project config
```

## Key Patterns

### Go ↔ Frontend Communication

- **Bound methods** (request/response): Go methods on the App struct are auto-exposed to JS. Frontend calls them like `await CreateTerminal("id")`. Used for CRUD operations.
- **Wails Events** (streaming): Used for terminal I/O. Go emits `terminal:output:<id>` events, frontend emits `terminal:input:<id>`. Data is base64-encoded for binary safety.

### Terminal Data Flow

1. Frontend calls `CreateTerminal(id)` or `CreateAttachedTerminal(id, tmuxSession)`
2. Go spawns PTY (with TERM=xterm-256color, UTF-8 locale)
3. Go goroutine reads PTY output → base64 encodes → emits Wails event
4. Frontend decodes base64 → writes Uint8Array to xterm.js (preserves UTF-8)
5. User types → xterm.js onData → base64 encode → emit input event → Go writes to PTY

### Port Allocation

- Main branch uses `default_port` from .orion.toml (e.g., 5173, 3000)
- Worktrees get random ports from 10000-60000 range
- Ports persisted to `~/.orion/ports.json` (MCP-readable for browser automation)
- Server env vars use `export` to persist across `&&` chains in commands
- Template syntax `{{backend.port}}` resolves cross-server references

### Workspace Environment (.orion/env.sh)

When servers start, Orion writes `<worktree>/.orion/env.sh` with all port assignments:
```bash
# Auto-generated by Orion
export FRONTEND_PORT=21814
export FRONTEND_URL=http://localhost:21814
export BACKEND_PORT=37792
export BACKEND_URL=http://localhost:37792
export NEXT_PUBLIC_API_URL=http://localhost:37792/api/
```

This file is automatically:
- **Sourced into shell sessions** — new shells get all port vars in their environment
- **Sourced before agent commands** — Claude/Codex know the ports without being told
- **Parsed into PTY env** — `CreateInDir` reads the file and injects vars into the process environment
- **Cleaned up on stop** — `StopServers` removes the env file
- **Gitignored** — `.orion/` is auto-added to `.gitignore`

### Session Management

- tmux sessions named: `orion-<repo>-<workspace>[-N]` for agents/shells, `orion-srv-<workspace>-<server>` for servers
- Closing a tab kills the underlying tmux session (no zombies)
- On startup, Orion scans for existing `orion-*` tmux sessions and reattaches (crash recovery)
- Last opened project remembered in `~/.orion/state.json`

### Config Loading (.orion.toml)

Priority: `.orion.toml` > `.radconfig` > built-in defaults. The config defines:
- `[credentials]` — files to copy into new worktrees (supports globs like `*.key`)
- `[servers.*]` — server commands, ports, env vars, working dirs
- `[agents.*]` — agent commands (dynamic buttons generated from this)
- `[lsp.*]` — optional language server command overrides
- `[plugins.formatters.*]`, `[plugins.linters.*]`, and `[[plugins.on_save]]` — editor save-time tooling
- `[hooks.worktree_created]` / `[hooks.worktree_deleting]` — shell callbacks around worktree lifecycle events

### LSP Support

Orion starts language servers lazily from the editable Monaco editor. `frontend/src/lib/lspClient.ts` registers Monaco providers for diagnostics, completions, hover, definition, references, signature help, document symbols, and semantic tokens. It sends `textDocument/didOpen`, `didChange`, `didSave`, and `didClose` notifications through Wails-bound methods on `App`.

The Go side lives in `internal/lsp/manager.go`. It launches the language server as a stdio process, writes JSON-RPC messages with `Content-Length` framing, matches request responses by ID, and emits server notifications back to the frontend as `lsp:message:<language>` Wails events.

Built-in defaults cover TypeScript/JavaScript, Go, Ruby, CSS, HTML, and JSON. TypeScript resolution prefers `<worktree>/frontend/node_modules/.bin/typescript-language-server`, then `<worktree>/node_modules/.bin/typescript-language-server`, then `typescript-language-server` on `PATH`. This keeps Orion lightweight: apps can provide their own language servers as dev dependencies.

Custom `[lsp.<language>]` commands are parsed as an executable plus args, not run through a shell. Use a wrapper script if a server needs shell setup. LSP processes run with the active worktree as `cwd` and `rootUri`, so module resolution follows the worktree being edited.

### Workspace Lifecycle Hooks

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

## Building

```bash
# Prerequisites
brew install go node tmux

# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Development (hot reload)
wails dev

# Production build
wails build

# The app is at build/bin/Orion.app
open build/bin/Orion.app
```

## Common Tasks

### Adding a new Go backend method callable from frontend
1. Add method to `App` struct in `app.go`
2. Run `wails generate module` to regenerate JS bindings
3. Import from `../../wailsjs/go/main/App` in frontend code

### Adding a new internal package
1. Create `internal/<package>/` directory
2. Import in `app.go` and add methods to `App` struct for frontend access
3. Regenerate bindings

### Modifying the terminal theme
- xterm.js theme: `frontend/src/lib/terminal.ts` (THEME constant)
- UI theme: `frontend/src/style.css` (CSS variables)
- Both should stay in sync — the app uses a Warp-inspired warm dark palette

### Updating the app icon
1. Edit `build/icon.svg`
2. Run: `rsvg-convert -w 1024 -h 1024 build/icon.svg -o build/appicon.png`
3. Generate iconset and .icns (see build script pattern in git history)

## Keyboard Shortcuts

- `Cmd+T` — New shell tab
- `Cmd+W` — Close focused pane (closes tab if last pane)
- `Cmd+D` — Split focused pane right (vertical split)
- `Cmd+Shift+D` — Split focused pane down (horizontal split)
- `Cmd+[` — Editor back when Monaco is focused; otherwise navigate to previous pane
- `Cmd+]` — Editor forward when Monaco is focused; otherwise navigate to next pane
- `Cmd+Shift+[` — Swap focused pane with previous
- `Cmd+Shift+]` — Swap focused pane with next
- `Cmd+B` — Go to definition in the editor
- `Cmd+Left/Right` — Cycle tabs
- `Cmd+1` — Toggle workspace sidebar
- `Cmd+\` — Toggle sidebar
- `Cmd+Shift+B` — Open browser for active workspace

## Split Panes

Each tab contains a pane tree — either a single terminal or a recursive split layout. The data model:

- `PaneLeaf` — a single terminal (has `terminalId`)
- `PaneSplit` — horizontal or vertical split with children and sizes

Pane operations: `splitPane()` wraps the focused pane in a split container with a new terminal. `closePane()` removes a pane and collapses single-child splits. `navigatePane()` cycles focus through leaves. `resizePanes()` updates split ratios (drag dividers). `swapPane()` swaps the focused pane with its neighbor. `mergeTabInto()` combines two tabs into one split tab.

The focused pane has a blue border highlight. Clicking a pane focuses it. Dragging a tab onto another tab merges them into a vertical split.

## Dependencies

**Go:** wails/v2, creack/pty, BurntSushi/toml
**npm:** @xterm/xterm, @xterm/addon-fit, @xterm/addon-webgl, @xterm/addon-unicode11, zustand

## Design Philosophy

- **Warp-inspired aesthetic** — dark theme, monospace everything, minimal chrome, terminal glyphs for status icons. The app should feel like a terminal, not a web app.
- **No zombies** — closing a tab kills the process. Simple mental model: no tab = no session.
- **tmux is invisible** — users never interact with tmux directly. It's a persistence layer.
- **Config-driven** — everything customizable via `.orion.toml` per repo (servers, agents, credentials, ports).
- **Main gets defaults, worktrees get isolation** — main branch uses standard ports (3000, 5173), worktrees get random isolated ports.
- **Agents always run with full permissions** — Claude uses `--dangerously-skip-permissions`, Codex uses `--dangerously-bypass-approvals-and-sandbox`.

## Roadmap / Future Features

These are features the user wants to build. Check Claude memory for full details.

- **Session history/restore** — browse and restore past Claude/Codex sessions per worktree
- **New workspace modal** — styled modal dialog instead of inline sidebar input
- **Split panes within tabs** — multiple terminals in one tab (like tmux panes but with native copy/paste). Cmd+D split right, Cmd+Shift+D split down
- **Git diff viewer** — built-in diff display without leaving Orion
- **Voice dictation** — mic button / Wispr Flow integration for voice-driven coding
- **Light/dark mode** — toggle, default to dark
- **Theming/skinning** — custom color palettes and fonts via theme files
- **File viewer/editor + global search (moonshot)** — Monaco/CodeMirror-based file browser, editor, and Cmd+Shift+F search across the worktree
- **Multi-instance + recent projects** — multiple Orion windows for different repos, macOS dock right-click menu with recent projects (like Cursor)
