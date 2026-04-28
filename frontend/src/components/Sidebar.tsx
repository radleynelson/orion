import { useCallback, useState, useEffect } from 'react';
import { useStore, generateId, PaneLeaf, sortWorkspaces } from '../store';
import { server, main } from '../../wailsjs/go/models';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import {
  ListWorkspaces,
  CreateWorkspaceFrom,
  DeleteWorkspace,
  LaunchAgent,
  LaunchClaudeChat,
  LaunchCodexChatWithOptions,
  SendClaudeChatMessage,
  SendCodexChatMessage,
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
import OrionMark from './OrionMark';
import WorkspaceDetailPanel from './WorkspaceDetailPanel';
import AgentSigil from './AgentSigil';

interface SidebarProps {
  onNewSession: () => void;
}

type CodexLaunchOptions = {
  model: string;
  reasoningEffort: string;
  approvalPolicy: string;
  sandboxMode: string;
  collaborationMode: string;
};

type NewWorkspaceDraft = {
  name: string;
  baseRef: string;
  startWith: 'codex-chat' | 'claude-chat' | 'codex' | 'claude' | 'shell' | 'none';
  prompt: string;
  codexOptions: CodexLaunchOptions;
};

const DEFAULT_CODEX_OPTIONS: CodexLaunchOptions = {
  model: '',
  reasoningEffort: 'xhigh',
  approvalPolicy: 'never',
  sandboxMode: 'danger-full-access',
  collaborationMode: 'default',
};

const DEFAULT_WORKSPACE_DRAFT: NewWorkspaceDraft = {
  name: '',
  baseRef: '',
  startWith: 'codex-chat',
  prompt: '',
  codexOptions: DEFAULT_CODEX_OPTIONS,
};

function agentProvider(agent?: main.AgentTypeInfo): 'claude' | 'codex' | undefined {
  const provider = (agent?.provider || agent?.name || '').toLowerCase();
  return provider === 'claude' || provider === 'codex' ? provider : undefined;
}

function agentIcon(agent?: main.AgentTypeInfo): string | undefined {
  return agent?.icon || agentProvider(agent);
}

function codexOptionsForAgent(agent?: main.AgentTypeInfo): CodexLaunchOptions {
  return {
    model: agent?.model || DEFAULT_CODEX_OPTIONS.model,
    reasoningEffort: agent?.reasoningEffort || DEFAULT_CODEX_OPTIONS.reasoningEffort,
    approvalPolicy: agent?.approvalPolicy || DEFAULT_CODEX_OPTIONS.approvalPolicy,
    sandboxMode: agent?.sandboxMode || DEFAULT_CODEX_OPTIONS.sandboxMode,
    collaborationMode: agent?.collaborationMode || DEFAULT_CODEX_OPTIONS.collaborationMode,
  };
}

const CODEX_MODELS = [
  { value: '', label: 'Default' },
  { value: 'gpt-5.5', label: 'GPT-5.5' },
  { value: 'gpt-5.4', label: 'GPT-5.4' },
  { value: 'gpt-5.4-mini', label: 'GPT-5.4 Mini' },
  { value: 'gpt-5.3-codex', label: 'GPT-5.3 Codex' },
  { value: 'gpt-5.3-codex-spark', label: 'GPT-5.3 Codex Spark' },
  { value: 'gpt-5.2', label: 'GPT-5.2' },
];

const REASONING_EFFORTS = [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'Extra high' },
];

export default function Sidebar({ onNewSession }: SidebarProps) {
  const {
    project,
    setProject,
    workspaces,
    activeWorkspacePath,
    setWorkspaces,
    setActiveWorkspace,
    addTab,
    addServerTab,
    tabs,
    workspaceActive,
    setWorkspaceActive,
  } = useStore();

  const [creating, setCreating] = useState(false);
  const [newWorkspaceDraft, setNewWorkspaceDraft] = useState<NewWorkspaceDraft>(DEFAULT_WORKSPACE_DRAFT);
  const [creatingWorkspace, setCreatingWorkspace] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [deletingPath, setDeletingPath] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<{ path: string; message: string } | null>(null);
  const [serverStatuses, setServerStatuses] = useState<Record<string, server.ServerStatus[]>>({});
  const [agentTypes, setAgentTypes] = useState<main.AgentTypeInfo[]>([]);
  const [sidebarVisible, setSidebarVisible] = useState(true);
  const [envVars, setEnvVars] = useState<Record<string, string>>({});
  const [inspectorEnvVars, setInspectorEnvVars] = useState<Record<string, string>>({});
  const [inspector, setInspector] = useState<{ path: string; top: number; left: number } | null>(null);

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

  useEffect(() => {
    if (!inspector?.path) {
      setInspectorEnvVars({});
      return;
    }
    (async () => {
      try {
        const env = await GetWorkspaceEnv(inspector.path);
        setInspectorEnvVars(env || {});
      } catch {
        setInspectorEnvVars({});
      }
    })();
  }, [inspector?.path, serverStatuses]);

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
      } catch {}
    };
    const handleServerChange = () => fetchAll();
    fetchAll();
    const interval = setInterval(fetchAll, 5000);
    window.addEventListener('orion:servers-changed', handleServerChange);
    return () => {
      clearInterval(interval);
      window.removeEventListener('orion:servers-changed', handleServerChange);
    };
  }, [project, workspaces]);

  // Recompute activity tier whenever tabs or server statuses change.
  // Tier drives icon color AND the Cmd+Up/Down cycle order. Uses live tab
  // state as the source of truth for "has agent" so closing a Claude tab
  // clears the yellow indicator immediately (ws.hasAgent is a stale snapshot).
  // 0 = servers running (green), 1 = agent only (yellow), 2 = inactive (grey)
  useEffect(() => {
    for (const ws of workspaces) {
      const statuses = serverStatuses[ws.path] || [];
      const hasServers = statuses.some((s) => s.running);
      const hasAgent = tabs.some(
        (t) => t.workspacePath === ws.path && (t.tabType === 'claude' || t.tabType === 'codex'),
      );
      const tier = hasServers ? 0 : hasAgent ? 1 : 2;
      setWorkspaceActive(ws.path, tier);
    }
  }, [tabs, serverStatuses, workspaces, setWorkspaceActive]);

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
        const baseRefs = workspaceBaseRefs(project?.mainBranch, workspaces);
        setNewWorkspaceDraft({
          ...DEFAULT_WORKSPACE_DRAFT,
          baseRef: baseRefs[0] || project?.mainBranch || 'main',
          codexOptions: { ...DEFAULT_CODEX_OPTIONS },
        });
        setCreateError(null);
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

  useEffect(() => {
    if (!project) return;
    let refreshTimer: number | null = null;
    const scheduleRefresh = () => {
      if (refreshTimer !== null) {
        window.clearTimeout(refreshTimer);
      }
      refreshTimer = window.setTimeout(() => {
        refreshTimer = null;
        refreshWorkspaces();
      }, 200);
    };
    const cancel = EventsOn('git:files-changed', scheduleRefresh);
    return () => {
      if (refreshTimer !== null) {
        window.clearTimeout(refreshTimer);
      }
      cancel();
    };
  }, [project, refreshWorkspaces]);

  const openNewWorkspace = useCallback(() => {
    const baseRefs = workspaceBaseRefs(project?.mainBranch, workspaces);
    setNewWorkspaceDraft({
      ...DEFAULT_WORKSPACE_DRAFT,
      baseRef: baseRefs[0] || project?.mainBranch || 'main',
      codexOptions: { ...DEFAULT_CODEX_OPTIONS },
    });
    setCreateError(null);
    setCreating(true);
  }, [project?.mainBranch, workspaces]);

  useEffect(() => {
    const handler = () => openNewWorkspace();
    window.addEventListener('orion:new-workspace', handler);
    return () => window.removeEventListener('orion:new-workspace', handler);
  }, [openNewWorkspace]);

  useEffect(() => {
    const closeInspector = () => setInspector(null);
    window.addEventListener('orion:close-workspace-inspector', closeInspector);
    return () => window.removeEventListener('orion:close-workspace-inspector', closeInspector);
  }, []);

  const updateNewWorkspaceDraft = useCallback(<K extends keyof NewWorkspaceDraft>(key: K, value: NewWorkspaceDraft[K]) => {
    setNewWorkspaceDraft((current) => ({ ...current, [key]: value }));
  }, []);

  const updateNewWorkspaceCodexOption = useCallback((key: keyof CodexLaunchOptions, value: string) => {
    setNewWorkspaceDraft((current) => ({
      ...current,
      codexOptions: { ...current.codexOptions, [key]: value },
    }));
  }, []);

  const handleDelete = useCallback(async (path: string) => {
    if (!project) return;
    setDeletingPath(path);
    setDeleteError(null);
    try {
      const state = useStore.getState();
      const wsTabs = state.tabs.filter((t) => t.workspacePath === path);
      const wsServerTabs = state.serverTabs.filter((t) => t.workspacePath === path);
      for (const tab of [...wsTabs, ...wsServerTabs]) {
        const termIds = state.getAllTerminalIds(tab);
        for (const termId of termIds) {
          try { await CloseTerminal(termId); } catch (e) { console.error('CloseTerminal failed during delete', { termId, e }); }
        }
      }
      // DeleteWorkspace also runs StopServers backend-side, but call it here
      // so the UI stops polling immediately and any per-server cleanup runs.
      try { await StopServers(path); } catch (e) { console.error('StopServers failed during delete', { path, e }); }
      await DeleteWorkspace(project.root, path);
      // Drop the zustand entries pointing at the now-deleted workspace.
      for (const tab of wsTabs) state.removeTab(tab.id);
      for (const tab of wsServerTabs) state.removeServerTab(tab.id);
      setConfirmDelete(null);
      await refreshWorkspaces();
    } catch (err) {
      console.error('Failed to delete workspace:', err);
      const message = err instanceof Error ? err.message : String(err);
      setDeleteError({ path, message });
    } finally {
      setDeletingPath(null);
    }
  }, [project, refreshWorkspaces]);

  const handleLaunchAgent = useCallback(async (wsPath: string, agentName: string) => {
    if (!project) {
      console.error('LaunchAgent skipped: project is null', { wsPath, agentName });
      return;
    }
    try {
      const tmuxSession = await LaunchAgent(project.root, wsPath, agentName);
      const termId = generateId('term');
      await CreateAttachedTerminal(termId, tmuxSession);
      const agent = agentTypes.find((a) => a.name === agentName);
      const provider = agentProvider(agent);
      const icon = agentIcon(agent);
      addTab({
        id: generateId('tab'),
        label: agent?.label || agentName,
        rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
        tabType: provider || 'shell',
        workspacePath: wsPath,
        icon,
        provider,
        viewMode: 'terminal',
        runtimeSessionId: tmuxSession,
        model: agent?.model,
        reasoningEffort: agent?.reasoningEffort,
        approvalPolicy: agent?.approvalPolicy,
        sandboxMode: agent?.sandboxMode,
        permissionMode: agent?.permissionMode,
        collaborationMode: agent?.collaborationMode,
      });
    } catch (err) {
      console.error('Failed to launch agent:', err);
    }
  }, [project, agentTypes, addTab]);

  const handleLaunchCodexChat = useCallback(async (wsPath: string, options?: CodexLaunchOptions) => {
    if (!project) {
      console.error('LaunchCodexChat skipped: project is null', { wsPath });
      return;
    }
    const selected = options || codexOptionsForAgent(agentTypes.find((agent) => agentProvider(agent) === 'codex'));
    try {
      const session = await LaunchCodexChatWithOptions(
        project.root,
        wsPath,
        selected.model,
        selected.reasoningEffort,
        selected.approvalPolicy,
        selected.sandboxMode,
        selected.collaborationMode,
      );
      addTab({
        id: generateId('tab'),
        label: session?.label || 'Codex Chat',
        rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'codex' } as PaneLeaf,
        tabType: 'codex-chat',
        workspacePath: wsPath,
        icon: 'codex',
        provider: 'codex',
        viewMode: 'chat',
        runtimeSessionId: session.id,
        threadId: session.threadId,
        model: session.model,
        reasoningEffort: session.reasoningEffort,
        approvalPolicy: session.approvalPolicy,
        sandboxMode: session.sandboxMode,
        collaborationMode: session.collaborationMode,
      });
      return session;
    } catch (err) {
      console.error('Failed to launch Codex chat:', err);
      throw err;
    }
  }, [project, agentTypes, addTab]);

  const handleLaunchClaudeChat = useCallback(async (wsPath: string) => {
    if (!project) {
      console.error('LaunchClaudeChat skipped: project is null', { wsPath });
      return;
    }
    try {
      const session = await LaunchClaudeChat(project.root, wsPath);
      addTab({
        id: generateId('tab'),
        label: session?.label || 'Claude Chat',
        rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'claude' } as PaneLeaf,
        tabType: 'claude-chat',
        workspacePath: wsPath,
        icon: 'claude',
        provider: 'claude',
        viewMode: 'chat',
        runtimeSessionId: session.id,
        threadId: session.threadId,
        model: session.model,
        reasoningEffort: session.reasoningEffort,
        approvalPolicy: session.approvalPolicy,
        sandboxMode: session.sandboxMode,
        permissionMode: session.permissionMode,
      });
      return session;
    } catch (err) {
      console.error('Failed to launch Claude chat:', err);
      throw err;
    }
  }, [project, addTab]);

  const handleLaunchShell = useCallback(async (wsPath: string) => {
    if (!project) {
      console.error('LaunchShell skipped: project is null', { wsPath });
      return;
    }
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

  const handleCreate = useCallback(async () => {
    if (!project || !newWorkspaceDraft.name.trim()) return;
    setCreatingWorkspace(true);
    setCreateError(null);
    try {
      const ws = await CreateWorkspaceFrom(project.root, normalizedWorkspaceName(newWorkspaceDraft.name), newWorkspaceDraft.baseRef);
      setCreating(false);
      await refreshWorkspaces();
      if (ws?.path) {
        setActiveWorkspace(ws.path);
        AllocatePorts(project.root, ws.path, false).catch(() => {});
        const prompt = newWorkspaceDraft.prompt.trim();
        switch (newWorkspaceDraft.startWith) {
          case 'codex-chat': {
            const session = await handleLaunchCodexChat(ws.path, newWorkspaceDraft.codexOptions);
            if (session?.id && prompt) await SendCodexChatMessage(session.id, prompt, []);
            break;
          }
          case 'claude-chat': {
            const session = await handleLaunchClaudeChat(ws.path);
            if (session?.id && prompt) await SendClaudeChatMessage(session.id, prompt, []);
            break;
          }
          case 'codex':
            await handleLaunchAgent(ws.path, 'codex');
            break;
          case 'claude':
            await handleLaunchAgent(ws.path, 'claude');
            break;
          case 'shell':
            await handleLaunchShell(ws.path);
            break;
          default:
            break;
        }
      }
    } catch (err) {
      console.error('Failed to create workspace:', err);
      setCreateError(err instanceof Error ? err.message : String(err));
    } finally {
      setCreatingWorkspace(false);
    }
  }, [project, newWorkspaceDraft, refreshWorkspaces, setActiveWorkspace, handleLaunchCodexChat, handleLaunchClaudeChat, handleLaunchAgent, handleLaunchShell]);

  const handleStartServers = useCallback(async (wsPath: string, isMain: boolean) => {
    if (!project) return;
    try {
      const statuses = await StartServers(project.root, wsPath, isMain);
      setServerStatuses((prev) => ({ ...prev, [wsPath]: statuses }));
      window.dispatchEvent(new Event('orion:servers-changed'));
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
  }, [project, addServerTab]);

  const handleStopServers = useCallback(async (wsPath: string) => {
    if (!project) return;
    try {
      await StopServers(wsPath);
      setServerStatuses((prev) => ({ ...prev, [wsPath]: [] }));
      window.dispatchEvent(new Event('orion:servers-changed'));
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

  const baseRefs = workspaceBaseRefs(project.mainBranch, workspaces);
  const normalizedName = normalizedWorkspaceName(newWorkspaceDraft.name);
  const previewPath = normalizedName ? `${project.root}-${normalizedName}` : `${project.root}-new-worktree`;
  const createDisabled = creatingWorkspace || !normalizedName;
  const inspectedWorkspace = inspector ? workspaces.find((ws) => ws.path === inspector.path) : undefined;

  return (
    <div className="sidebar">
      {/* Project name */}
      <div className="sidebar-section">
        <div className="sidebar-brand">
          <OrionMark size={30} />
          <div className="sidebar-brand-text">
            <div className="sidebar-brand-title">{project.name}</div>
            <div className="sidebar-brand-subtitle">{project.root}</div>
          </div>
          <span
            style={{ cursor: 'pointer', color: 'var(--text-dim)', fontSize: 'var(--font-size-xs)' }}
            onClick={handleOpenProject}
            title="Switch project"
          >
            ▾
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
              onClick={openNewWorkspace}
              title="New workspace"
            >
              +
            </span>
          </span>
        </div>

        {sortWorkspaces(workspaces, workspaceActive).map((ws) => {
          const wsStatuses = serverStatuses[ws.path] || [];
          const wsHasServers = wsStatuses.some((s) => s.running);
          const wsAgentTabs = tabs.filter((t) =>
            t.workspacePath === ws.path &&
            (t.tabType === 'claude' || t.tabType === 'codex' || t.tabType === 'claude-chat' || t.tabType === 'codex-chat'),
          );
          const wsHasAgent = wsAgentTabs.length > 0;
          const active = ws.path === activeWorkspacePath;

          return (
            <div key={ws.path} className="sidebar-workspace-row">
              <div
                className={`sidebar-item ${active ? 'active' : ''}`}
                onClick={() => {
                  setActiveWorkspace(ws.path);
                  // Pre-allocate ports so agents/shells know them immediately
                  if (project) AllocatePorts(project.root, ws.path, ws.isMain).catch(() => {});
                }}
              >
                <span className={`icon ${wsHasServers ? '' : wsHasAgent ? 'agent-only' : 'inactive'}`}>
                  {ws.isMain ? '◉' : wsHasAgent || wsHasServers ? '●' : '○'}
                </span>
                <span className="label">{ws.isMain ? 'main' : (project ? ws.name.replace(project.name + '-', '') : ws.name)}</span>
                <WorkspaceActivityBadges tabs={wsAgentTabs} />
                <button
                  type="button"
                  className={`workspace-info-button ${inspector?.path === ws.path ? 'active' : ''}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    const rect = e.currentTarget.getBoundingClientRect();
                    setInspector((current) =>
                      current?.path === ws.path
                        ? null
                        : { path: ws.path, top: Math.max(72, rect.top - 26), left: rect.right + 10 },
                    );
                  }}
                  title="Workspace details"
                >
                  i
                </button>
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
              {deleteError?.path === ws.path && (
                <div
                  style={{
                    margin: '4px 8px 8px 28px',
                    padding: '6px 8px',
                    fontSize: 'var(--font-size-xs)',
                    color: '#ff8a80',
                    background: 'rgba(255, 138, 128, 0.08)',
                    border: '1px solid rgba(255, 138, 128, 0.25)',
                    borderRadius: '4px',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                  }}
                  onClick={() => setDeleteError(null)}
                  title="Click to dismiss"
                >
                  {deleteError.message}
                </div>
              )}
            </div>
          );
        })}

        {inspector && inspectedWorkspace && (
          <>
            <div className="workspace-inspector-dismiss" onMouseDown={() => setInspector(null)} />
            <div
              className="workspace-inspector-popover"
              style={{ top: inspector.top, left: inspector.left }}
              onMouseDown={(e) => e.stopPropagation()}
            >
              <div className="workspace-inspector-header">
                <span className={`icon ${(serverStatuses[inspectedWorkspace.path] || []).some((s) => s.running) ? '' : 'inactive'}`} />
                <div>
                  <strong>{inspectedWorkspace.isMain ? 'main' : (project ? inspectedWorkspace.name.replace(project.name + '-', '') : inspectedWorkspace.name)}</strong>
                  <code>{inspectedWorkspace.branch || inspectedWorkspace.name}</code>
                </div>
                <button type="button" onClick={() => setInspector(null)} title="Close">×</button>
              </div>
              <WorkspaceDetailPanel
                workspace={inspectedWorkspace}
                serverStatuses={serverStatuses[inspectedWorkspace.path] || []}
                envVars={inspectedWorkspace.path === activeWorkspacePath ? envVars : inspectorEnvVars}
                onStartServers={handleStartServers}
                onStopServers={handleStopServers}
                onNewSession={() => {
                  setActiveWorkspace(inspectedWorkspace.path);
                  onNewSession();
                }}
              />
            </div>
          </>
        )}

        {creating && (
          <div className="workspace-create-overlay" onMouseDown={() => !creatingWorkspace && setCreating(false)}>
            <div className="workspace-create-sheet" onMouseDown={(e) => e.stopPropagation()}>
              <div className="workspace-create-header">
                <button type="button" onClick={() => setCreating(false)} disabled={creatingWorkspace}>Cancel</button>
                <div>
                  <div>New worktree</div>
                  <span>{project.name}</span>
                </div>
                <button type="button" className="workspace-create-primary" onClick={handleCreate} disabled={createDisabled}>
                  {creatingWorkspace ? 'Creating' : 'Create'}
                </button>
              </div>

              <div className="workspace-create-body">
                <label className="workspace-create-field workspace-create-wide">
                  <span>Name</span>
                  <input
                    autoFocus
                    autoCapitalize="off"
                    autoCorrect="off"
                    spellCheck={false}
                    placeholder="fix-stripe-webhook"
                    value={newWorkspaceDraft.name}
                    onChange={(e) => updateNewWorkspaceDraft('name', e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !createDisabled) handleCreate();
                      if (e.key === 'Escape') setCreating(false);
                    }}
                  />
                  <small>{previewPath}</small>
                </label>

                <label className="workspace-create-field">
                  <span>Branch from</span>
                  <select value={newWorkspaceDraft.baseRef || baseRefs[0]} onChange={(e) => updateNewWorkspaceDraft('baseRef', e.target.value)}>
                    {baseRefs.map((ref) => <option key={ref} value={ref}>{ref}</option>)}
                  </select>
                </label>

                <label className="workspace-create-field">
                  <span>Start with</span>
                  <select value={newWorkspaceDraft.startWith} onChange={(e) => updateNewWorkspaceDraft('startWith', e.target.value as NewWorkspaceDraft['startWith'])}>
                    <option value="codex-chat">Codex Chat</option>
                    <option value="claude-chat">Claude Chat</option>
                    <option value="codex">Codex CLI</option>
                    <option value="claude">Claude CLI</option>
                    <option value="shell">Shell</option>
                    <option value="none">Nothing</option>
                  </select>
                </label>

                {newWorkspaceDraft.startWith === 'codex-chat' && (
                  <div className="workspace-create-codex">
                    <label>
                      <span>Model</span>
                      <select value={newWorkspaceDraft.codexOptions.model} onChange={(e) => updateNewWorkspaceCodexOption('model', e.target.value)}>
                        {CODEX_MODELS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                    <label>
                      <span>Reasoning</span>
                      <select value={newWorkspaceDraft.codexOptions.reasoningEffort} onChange={(e) => updateNewWorkspaceCodexOption('reasoningEffort', e.target.value)}>
                        {REASONING_EFFORTS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                  </div>
                )}

                <label className="workspace-create-field workspace-create-wide">
                  <span>First prompt</span>
                  <textarea
                    placeholder="Trace the failing checkout webhook and propose a fix."
                    value={newWorkspaceDraft.prompt}
                    onChange={(e) => updateNewWorkspaceDraft('prompt', e.target.value)}
                    disabled={newWorkspaceDraft.startWith !== 'codex-chat' && newWorkspaceDraft.startWith !== 'claude-chat'}
                  />
                  <small>Sent automatically when the new worktree starts with a chat session.</small>
                </label>

                {createError && <div className="workspace-create-error">{createError}</div>}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function WorkspaceActivityBadges({ tabs }: { tabs: { tabType: string; provider?: string; icon?: string }[] }) {
  const ids = Array.from(new Set(tabs.map((tab) => {
    if (tab.icon) return tab.icon;
    if (tab.provider) return tab.provider;
    if (tab.tabType === 'claude-chat') return 'claude';
    if (tab.tabType === 'codex-chat') return 'codex';
    return tab.tabType;
  }))).slice(0, 3);
  if (ids.length === 0) return null;
  return (
    <span className="workspace-activity-badges">
      {ids.map((id) => <AgentSigil key={id} id={id} size={15} />)}
    </span>
  );
}

function workspaceBaseRefs(mainBranch: string | undefined, workspaces: { branch?: string }[]): string[] {
  const refs = new Set<string>();
  if (mainBranch) refs.add(mainBranch);
  for (const ws of workspaces) {
    const branch = (ws.branch || '').trim();
    if (branch && branch !== '(detached)') refs.add(branch);
  }
  if (refs.size === 0) refs.add('main');
  return Array.from(refs);
}

function normalizedWorkspaceName(name: string): string {
  return name.trim().toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '');
}
