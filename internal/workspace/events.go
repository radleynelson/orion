package workspace

import wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

func (m *Manager) emitWorkspaceCreateProgress(workspacePath string, stage string) {
	if m == nil || m.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(m.ctx, "workspace:create-progress", map[string]string{
		"workspacePath": workspacePath,
		"stage":         stage,
	})
}
