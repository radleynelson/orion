package main

import (
	"context"

	"orion/internal/terminal"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct bound to Wails.
type App struct {
	ctx      context.Context
	termMgr  *terminal.Manager
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{
		termMgr: terminal.NewManager(),
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.termMgr.SetContext(ctx)

	// Listen for terminal input events from the frontend
	wailsRuntime.EventsOn(ctx, "terminal:input", func(optionalData ...interface{}) {
		if len(optionalData) < 2 {
			return
		}
		id, ok1 := optionalData[0].(string)
		data, ok2 := optionalData[1].(string)
		if ok1 && ok2 {
			a.termMgr.Write(id, data)
		}
	})

	// Listen for terminal resize events
	wailsRuntime.EventsOn(ctx, "terminal:resize", func(optionalData ...interface{}) {
		if len(optionalData) < 3 {
			return
		}
		id, ok1 := optionalData[0].(string)
		cols, ok2 := optionalData[1].(float64)
		rows, ok3 := optionalData[2].(float64)
		if ok1 && ok2 && ok3 {
			a.termMgr.Resize(id, int(cols), int(rows))
		}
	})
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	a.termMgr.CloseAll()
}

// CreateTerminal creates a new terminal session.
func (a *App) CreateTerminal(id string) error {
	return a.termMgr.Create(id)
}

// CreateAttachedTerminal creates a terminal attached to a tmux session.
func (a *App) CreateAttachedTerminal(id string, tmuxSession string) error {
	return a.termMgr.CreateAttached(id, tmuxSession)
}

// CloseTerminal closes a terminal session.
func (a *App) CloseTerminal(id string) error {
	return a.termMgr.Close(id)
}

// ListTerminals returns all active terminal IDs.
func (a *App) ListTerminals() []string {
	return a.termMgr.List()
}
