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

// ignoredDir returns true for directories that should never be watched.
func ignoredDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "tmp",
		".next", ".turbo", "__pycache__", ".cache", ".orion":
		return true
	}
	return false
}

// Watch starts watching the given workspace path. Any previous watch is
// stopped first. File change events are debounced and emitted as
// "git:files-changed" Wails events.
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

	// Walk the directory tree and add watchable dirs.
	_ = filepath.WalkDir(workspacePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && name != "." && path != workspacePath {
				// Skip hidden dirs except .git (we want to watch .git/index)
				if name == ".git" {
					// Only watch the .git dir itself (for index changes), not subdirs
					_ = w.Add(path)
				}
				return filepath.SkipDir
			}
			if ignoredDir(name) {
				return filepath.SkipDir
			}
			_ = w.Add(path)
		}
		return nil
	})

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
				// If a new directory was created, start watching it too
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						name := filepath.Base(event.Name)
						if !ignoredDir(name) && !strings.HasPrefix(name, ".") {
							_ = w.Add(event.Name)
						}
					}
				}
				// Debounce: wait 500ms after the last event before firing
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, fire)
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
