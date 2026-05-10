package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Manager watches workspace directories for file changes and emits Wails
// events so the frontend can refresh diffs automatically.
type Manager struct {
	ctx     context.Context
	mu      sync.Mutex
	watcher *fsnotify.Watcher
	done    chan struct{}
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Watch starts watching the given workspace path. Any previous watch is
// stopped first. File change events are debounced and emitted as
// "git:files-changed" Wails events.
//
// We deliberately avoid recursing into the working tree. fsnotify on macOS
// opens a separate file descriptor for every file inside every watched
// directory (kqueue requirement), which exhausts RLIMIT_NOFILE on large
// repos. Instead we watch just .git — index/HEAD updates fire on commits,
// stages, and branch switches, which is what Orion actually cares about for
// "code review" refreshes. Working-tree edits are picked up by the 7s
// `refreshDiffStats` poll in App.tsx.
func (m *Manager) Watch(workspacePath string) error {
	m.Stop()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.watcher = w
	m.done = make(chan struct{})
	m.mu.Unlock()

	gitDir := filepath.Join(workspacePath, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		_ = w.Add(gitDir)
	} else if err == nil && !info.IsDir() {
		// Worktrees use a .git file pointing to the real gitdir.
		if data, rerr := os.ReadFile(gitDir); rerr == nil {
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(line, "gitdir:") {
				realDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
				if !filepath.IsAbs(realDir) {
					realDir = filepath.Join(workspacePath, realDir)
				}
				_ = w.Add(realDir)
			}
		}
	}

	done := m.done

	go func() {
		var debounce *time.Timer
		fire := func() {
			wailsRuntime.EventsEmit(m.ctx, "git:files-changed")
		}

		for {
			select {
			case <-done:
				if debounce != nil {
					debounce.Stop()
				}
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				// Ignore chmod-only events
				if event.Op == fsnotify.Chmod {
					continue
				}
				// Debounce: wait 1.5s after the last event before firing.
				// Long enough to coalesce agent-driven `.git` churn (git status,
				// index refresh) without delaying real edits noticeably.
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(1500*time.Millisecond, fire)
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

// Stop closes the current watcher if any.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.done != nil {
		close(m.done)
		m.done = nil
	}
	if m.watcher != nil {
		_ = m.watcher.Close()
		m.watcher = nil
	}
}
