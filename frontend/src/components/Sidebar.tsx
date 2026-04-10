import { useCallback, useState, useEffect } from 'react';
import { useStore, generateId, PaneLeaf, sortWorkspaces } from '../store';
import { server, main } from '../../wailsjs/go/models';
import {
  ListWorkspaces,
  CreateWorkspace,
  DeleteWorkspace,
  LaunchAgent,
  CreateAttachedTerminal,
  CreateTerminalInDir,
  CloseTerminal,
  OpenProjectDialog,
  StartServers,
  StopServers,
  GetServerStatuses,
  OpenBrowser,
  GetAgentTypes,
  SaveTabs,
  GetTmuxSession,
  GetWorkspaceEnv,
  AllocatePorts,
} from '../../wailsjs/go/main/App';

export default function Sidebar() {
  const {
    project,
    setProject,
    workspaces,
    activeWorkspacePath,
    setWorkspaces,
    setActiveWorkspace,
    addTab,
    addServerTab,
    serverTabs,
    tabs,
    workspaceActive,
    setWorkspaceActive,
  } = useStore();

  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [deletingPath, setDeletingPath] = useState<string | null>(null);
  const [serverStatuses, setServerStatuses] = useState<Record<string, server.ServerStatus[]>>({});
  const [agentTypes, setAgentTypes] = useState<main.AgentTypeInfo[]>([]);
  const [sidebarVisible, setSidebarVisible] = useState(true);
  const [envVars, setEnvVars] = useState<Record<string, string>>({});
  const [envVisible, setEnvVisible] = useState(false);

  // Init is handled by App.tsx (which never unmounts)

  // Load agent types when project changes
  useEffect(() => {
    if (!project) return;
    (async () => {
      try {
        const agents = await GetAgentTypes(project.root);
        setAgentTypes(agents);
      } catch {}
    })();
  }, [project]);

  // Fetch env vars when active workspace changes
  useEffect(() => {
    if (!activeWorkspacePath) return;
    (async () => {
      try {
        const env = await GetWorkspaceEnv(activeWorkspacePath);
        setEnvVars(env || {});
      } catch {}
    })();
  }, [activeWorkspacePath, serverStatuses]);

  // Poll server statuses for ALL workspaces (so indicators are correct on startup)
  useEffect(() => {
    if (!project || workspaces.length === 0) return;
    const fetchAll = async () => {
      try {
        const results = await Promise.all(
          workspaces.map(async (ws) => {
            try {
              const statuses = await GetServerStatuses(project.root, ws.path);
              return [ws.path, statuses || []] as [string, server.ServerStatus[]];
            } catch {
              return [ws.path, [] as server.ServerStatus[]] as [string, server.ServerStatus[]];
            }
          })
        );
        setServerStatuses((prev) => {
          const next = { ...prev };
          for (const [path, statuses] of results) next[path] = statuses;
          return next;
        });
        // Publish active flags so the cycle order in App.tsx matches the sidebar sort
        for (const [path, statuses] of results) {
          const ws = workspaces.find((w) => w.path === path);
          const active = statuses.some((s) => s.running) || !!ws?.hasAgent;
          setWorkspaceActive(path, active);
        }
      } catch {}
    };
    fetchAll();
    const interval = setInterval(fetchAll, 5000);
    return () => clearInterval(interval);
  }, [project, workspaces]);

  // Keyboard shortcut: Cmd+\ to toggle sidebar
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.metaKey && e.key === '\\') {
        e.preventDefault();
        setSidebarVisible((v) => !v);
      }
      // Cmd+Shift+B: open browser
      if (e.metaKey && e.shiftKey && e.key === 'B' && project && activeWorkspacePath) {
        e.preventDefault();
        OpenBrowser(project.root, activeWorkspacePath);
      }
      // Cmd+N: new workspace
      if (e.metaKey && !e.shiftKey && e.key === 'n') {
        e.preventDefault();
        setCreating(true);
      }
      // Cmd+Shift+Backspace: delete active workspace
      if (e.metaKey && e.shiftKey && e.key === 'Backspace' && activeWorkspacePath) {
        e.preventDefault();
        const ws = workspaces.find((w) => w.path === activeWorkspacePath);
        if (ws && !ws.isMain) {
          setConfirmDelete(activeWorkspacePath);
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [project, activeWorkspacePath, workspaces]);

  const refreshWorkspaces = useCallback(async () => {
    if (!project) return;
    try {
      const ws = await ListWorkspaces(project.root);
      setWorkspaces(ws);
    } catch (err) {
      console.error('Failed to list workspaces:', err);
    }
  }, [project, setWorkspaces]);

  const handleCreate = useCallback(async () => {
    if (!project || !newName.trim()) return;
    try {
      const ws = await CreateWorkspace(project.root, newName.trim());
      setNewName('');
      setCreating(false);
      await refreshWorkspaces();
      if (ws?.path) {
        setActiveWorkspace(ws.path);
        AllocatePorts(project.root, ws.path, false).catch(() => {});
      }
    } catch (err) {
      console.error('Failed to create workspace:', err);
    }
  }, [project, newName, refreshWorkspaces, setActiveWorkspace]);

  const handleDelete = useCallback(async (path: string) => {
    if (!project) return;
    setDeletingPath(path);
    try {
      const wsTabs = tabs.filter((t) => t.workspacePath === path);
      for (const tab of wsTabs) {
        const termIds = useStore.getState().getAllTerminalIds(tab);
        for (const termId of termIds) {
          await CloseTerminal(termId);
        }
      }
      await DeleteWorkspace(project.root, path);
      setConfirmDelete(null);
      await refreshWorkspaces();
    } catch (err) {
      console.error('Failed to delete workspace:', err);
    } finally {
      setDeletingPath(null);
    }
  }, [project, tabs, refreshWorkspaces]);

  const handleLaunchAgent = useCallback(async (wsPath: string, agentName: string) => {
    if (!project) return;
    try {
      const tmuxSession = await LaunchAgent(project.root, wsPath, agentName);
      const termId = generateId('term');
      await CreateAttachedTerminal(termId, tmuxSession);
      const agent = agentTypes.find((a) => a.name === agentName);
      addTab({
        id: generateId('tab'),
        label: agent?.label || agentName,
        rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
        tabType: (agentName === 'claude' || agentName === 'codex') ? agentName as 'claude' | 'codex' : 'shell',
        workspacePath: wsPath,
      });
    } catch (err) {
      console.error('Failed to launch agent:', err);
    }
  }, [project, agentTypes, addTab]);

  const handleLaunchShell = useCallback(async (wsPath: string) => {
    if (!project) return;
    try {
      const termId = generateId('term');
      await CreateTerminalInDir(termId, wsPath);
      const shellNum = tabs.filter((t) => t.workspacePath === wsPath && t.tabType === 'shell').length + 1;
      addTab({
        id: generateId('tab'),
        label: `Shell ${shellNum}`,
        rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
        tabType: 'shell',
        workspacePath: wsPath,
      });
    } catch (err) {
      console.error('Failed to launch shell:', err);
    }
  }, [project, tabs, addTab]);

  const handleStartServers = useCallback(async (wsPath: string, isMain: boolean) => {
    if (!project) return;
    try {
      const statuses = await StartServers(project.root, wsPath, isMain);
      setServerStatuses((prev) => ({ ...prev, [wsPath]: statuses }));
      for (const srv of statuses) {
        if (srv.running && srv.tmuxSession) {
          const termId = generateId('term');
          await CreateAttachedTerminal(termId, srv.tmuxSession);
          addServerTab({
            id: generateId('tab'),
            label: srv.name.charAt(0).toUpperCase() + srv.name.slice(1),
            rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
            tabType: 'server',
            workspacePath: wsPath,
          });
        }
      }
    } catch (err) {
      console.error('Failed to start servers:', err);
    }
  }, [project, addTab]);

  const handleStopServers = useCallback(async (wsPath: string) => {
    if (!project) return;
    try {
      await StopServers(wsPath);
      setServerStatuses((prev) => ({ ...prev, [wsPath]: [] }));
      // Clean up server tabs from the bottom pane
      const srvTabs = useStore.getState().serverTabs.filter((t) => t.workspacePath === wsPath);
      for (const tab of srvTabs) {
        const termIds = useStore.getState().getAllTerminalIds(tab);
        for (const termId of termIds) {
          await CloseTerminal(termId);
        }
        useStore.getState().removeServerTab(tab.id);
      }
    } catch (err) {
      console.error('Failed to stop servers:', err);
    }
  }, [project, tabs]);

  const handleOpenBrowser = useCallback(async (wsPath: string) => {
    if (!project) return;
    try {
      await OpenBrowser(project.root, wsPath);
    } catch (err) {
      console.error('Failed to open browser:', err);
    }
  }, [project]);

  const handleOpenProject = useCallback(async () => {
    try {
      const info = await OpenProjectDialog();
      if (!info) return;
      // Delegate to App.loadProject which handles tab restore + tmux recovery.
      const loader = (window as any).__orionLoadProject;
      if (loader) {
        await loader(info);
      } else {
        // Fallback: minimal load (shouldn't happen)
        setProject({ name: info.name, root: info.root, mainBranch: info.mainBranch });
        const ws = await ListWorkspaces(info.root);
        setWorkspaces(ws);
      }
    } catch (err) {
      console.error('Failed to open project:', err);
    }
  }, [setProject, setWorkspaces]);

  if (!sidebarVisible) {
    return null;
  }

  if (!project) {
    return (
      <div className="sidebar">
        <div className="sidebar-section">
          <div className="sidebar-label">Project</div>
          <div className="sidebar-item" onClick={handleOpenProject} style={{ cursor: 'pointer' }}>
            <span className="icon inactive">+</span>
            <span className="label">Open project...</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="sidebar">
      {/* Project name */}
      <div className="sidebar-section">
        <div className="sidebar-label" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{project.name}</span>
          <span
            style={{ cursor: 'pointer', color: 'var(--text-dim)', fontSize: 'var(--font-size-xs)' }}
            onClick={handleOpenProject}
            title="Switch project"
          >
            ↗
          </span>
        </div>
      </div>

      {/* Workspaces */}
      <div className="sidebar-section" style={{ flex: 1 }}>
        <div className="sidebar-label" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>Workspaces</span>
          <span style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <span
              style={{ cursor: 'pointer', color: 'var(--text-dim)', fontSize: 'var(--font-size)' }}
              onClick={refreshWorkspaces}
              title="Refresh workspaces"
            >
              ↻
            </span>
            <span
              style={{ cursor: 'pointer', color: 'var(--text-dim)', fontSize: 'var(--font-size)' }}
              onClick={() => setCreating(true)}
              title="New workspace"
            >
              +
            </span>
          </span>
        </div>

        {sortWorkspaces(workspaces, workspaceActive).map((ws) => {
          const wsStatuses = serverStatuses[ws.path] || [];
          const wsHasServers = wsStatuses.some((s) => s.running);

          return (
            <div key={ws.path}>
              <div
                className={`sidebar-item ${ws.path === activeWorkspacePath ? 'active' : ''}`}
                onClick={() => {
                  setActiveWorkspace(ws.path);
                  // Pre-allocate ports so agents/shells know them immediately
                  if (project) AllocatePorts(project.root, ws.path, ws.isMain).catch(() => {});
                }}
              >
                <span className={`icon ${ws.hasAgent || wsHasServers ? '' : 'inactive'}`}>
                  {ws.isMain ? '◉' : ws.hasAgent || wsHasServers ? '●' : '○'}
                </span>
                <span className="label">{ws.isMain ? 'main' : (project ? ws.name.replace(project.name + '-', '') : ws.name)}</span>
                {!ws.isMain && deletingPath === ws.path && (
                  <span className="ws-delete-spinner" title="Deleting...">⟳</span>
                )}
                {!ws.isMain && deletingPath !== ws.path && (
                  <span
                    className="ws-delete-icon"
                    onClick={(e) => {
                      e.stopPropagation();
                      if (confirmDelete === ws.path) {
                        handleDelete(ws.path);
                      } else {
                        setConfirmDelete(ws.path);
                        setTimeout(() => setConfirmDelete((c) => c === ws.path ? null : c), 4000);
                      }
                    }}
                    title={confirmDelete === ws.path ? 'Click again to confirm' : 'Delete workspace'}
                  >
                    {confirmDelete === ws.path ? '✕?' : '✕'}
                  </span>
                )}
              </div>

              {/* Actions when workspace is selected */}
              {ws.path === activeWorkspacePath && (
                <>
                  {/* Dynamic agent buttons from config */}
                  <div className="sidebar-actions">
                    {agentTypes.map((agent) => (
                      <span
                        key={agent.name}
                        className="sidebar-action"
                        onClick={() => handleLaunchAgent(ws.path, agent.name)}
                        title={agent.command}
                      >
                        + {agent.label}
                      </span>
                    ))}
                    <span className="sidebar-action" onClick={() => handleLaunchShell(ws.path)}>
                      + Shell
                    </span>
                  </div>

                  {/* Server controls */}
                  <div className="sidebar-actions">
                    {!wsHasServers ? (
                      <span className="sidebar-action" onClick={() => handleStartServers(ws.path, ws.isMain)} style={{ color: 'var(--accent-green)' }}>
                        ▶ Start Servers
                      </span>
                    ) : (
                      <>
                        <span className="sidebar-action" onClick={() => handleStopServers(ws.path)} style={{ color: 'var(--accent-red)' }}>
                          ■ Stop
                        </span>
                        <span className="sidebar-action" onClick={() => handleOpenBrowser(ws.path)} style={{ color: 'var(--accent-cyan)' }}>
                          ◎ Browser
                        </span>
                      </>
                    )}
                  </div>

                  {/* Server port display */}
                  {wsStatuses.length > 0 && (
                    <div className="sidebar-servers">
                      {wsStatuses.map((srv) => (
                        <div key={srv.name} className="sidebar-server">
                          <span className={`server-dot ${srv.running ? 'running' : 'stopped'}`}>●</span>
                          <span className="server-name">{srv.name}</span>
                          {srv.port > 0 && <span className="server-port">:{srv.port}</span>}
                        </div>
                      ))}
                    </div>
                  )}

                  {/* Environment variables panel */}
                  {Object.keys(envVars).length > 0 && ws.path === activeWorkspacePath && (
                    <div className="sidebar-env">
                      <div
                        className="sidebar-env-header"
                        onClick={() => setEnvVisible(!envVisible)}
                      >
                        <span>{envVisible ? '▾' : '▸'} Env</span>
                      </div>
                      {envVisible && (
                        <div className="sidebar-env-list">
                          {Object.entries(envVars).map(([key, val]) => (
                            <div
                              key={key}
                              className="sidebar-env-item"
                              onClick={() => navigator.clipboard.writeText(val)}
                              title="Click to copy"
                            >
                              <span className="env-key">{key}</span>
                              <span className="env-val">{val}</span>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </>
              )}

            </div>
          );
        })}

        {creating && (
          <div style={{ padding: '4px 12px' }}>
            <input
              autoFocus
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              className="sidebar-input"
              placeholder="branch-name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleCreate();
                if (e.key === 'Escape') { setCreating(false); setNewName(''); }
              }}
              onBlur={() => { if (!newName.trim()) { setCreating(false); setNewName(''); } }}
            />
          </div>
        )}
      </div>
    </div>
  );
}
