import { useEffect, useCallback, useMemo, useState, DragEvent } from 'react';
import './App.css';
import SplitPane from './components/SplitPane';
import Sidebar from './components/Sidebar';
import ActivityBar from './components/ActivityBar';
import FileExplorer from './components/FileExplorer';
import GlobalSearch from './components/GlobalSearch';
import CodeReviewPane from './components/CodeReviewPane';
import SearchEverywhere from './components/SearchEverywhere';
import CommandPalette, { CommandPaletteItem } from './components/CommandPalette';
import NewTabPicker, { NewTabChoice } from './components/NewTabPicker';
import ConversionHistoryPicker, { ConversionHistoryCandidate } from './components/ConversionHistoryPicker';
import AgentSigil from './components/AgentSigil';
import OrionMark from './components/OrionMark';
import { useStore, generateId, Tab, Pane, PaneLeaf, zoomFactorFor, sortWorkspaces } from './store';
import { configureMonacoTheme } from './lib/monacoTheme';
import { parseUnifiedDiff } from './lib/diffParser';
import { BrowserOpenURL, EventsOn } from '../wailsjs/runtime/runtime';
import { claudesdk, codexchat, main, server, state } from '../wailsjs/go/models';
import {
  AllocatePorts,
  ConvertChatToTerminalWithOptions,
  AttachClaudeChat,
  ResumeClaudeChatWithOptions,
  CreateTerminalInDir,
  CreateAttachedTerminal,
  CloseTerminal,
  ResumeCodexChatWithOptions,
  ConvertTerminalToClaudeChatWithOptions,
  ConvertTerminalToCodexChatWithOptions,
  ListClaudeChatHistory,
  ListCodexChatHistory,
  ListClaudeChatSessions,
  ListCodexChatSessions,
  SaveTabs,
  GetLastProject,
  GetProjectInfo,
  SetActiveProject,
  ListWorkspaces,
  NewWindow,
  OpenProjectDialog,
  GetAgentTypes,
  GetSavedTabs,
  GetTmuxSession,
  RecoverSessions,
  RevealInFinder,
  StopClaudeChat,
  StopCodexChat,
  LaunchClaudeChat,
  LaunchCodexChatWithOptions,
  LaunchAgent,
  StartServers,
  StopServers,
  GetServerStatuses,
  GetWorkspaceEnv,
  GetChangedFilesAgainst,
  GetUnifiedDiff,
  WatchWorkspace,
  OpenBrowser,
} from '../wailsjs/go/main/App';

const DEFAULT_CODEX_CHAT_OPTIONS = {
  model: '',
  reasoningEffort: 'xhigh',
  approvalPolicy: 'never',
  sandboxMode: 'danger-full-access',
  collaborationMode: 'default',
};

type DiffStats = { files: number; added: number; removed: number; loading: boolean };
type AgentKind = 'claude' | 'codex';
type ConversionPickerState = {
  kind: AgentKind;
  tabId: string;
  workspacePath: string;
  error: string;
  candidates: ConversionHistoryCandidate[];
};

function agentProvider(agent?: main.AgentTypeInfo): 'claude' | 'codex' | undefined {
  const provider = (agent?.provider || agent?.name || '').toLowerCase();
  return provider === 'claude' || provider === 'codex' ? provider : undefined;
}

function agentIcon(agent?: main.AgentTypeInfo): string | undefined {
  return agent?.icon || agentProvider(agent);
}

function codexOptionsForAgent(agent?: main.AgentTypeInfo) {
  return {
    model: agent?.model || DEFAULT_CODEX_CHAT_OPTIONS.model,
    reasoningEffort: agent?.reasoningEffort || DEFAULT_CODEX_CHAT_OPTIONS.reasoningEffort,
    approvalPolicy: agent?.approvalPolicy || DEFAULT_CODEX_CHAT_OPTIONS.approvalPolicy,
    sandboxMode: agent?.sandboxMode || DEFAULT_CODEX_CHAT_OPTIONS.sandboxMode,
    collaborationMode: agent?.collaborationMode || DEFAULT_CODEX_CHAT_OPTIONS.collaborationMode,
  };
}

function App() {
  const {
    project,
    setProject,
    workspaces,
    setWorkspaces,
    activeWorkspacePath,
    setActiveWorkspace,
    tabs,
    activeTabId,
    addTab,
    removeTab,
    setActiveTab,
    splitPane,
    closePane,
    navigatePane,
    swapPane,
    rotateSplit,
    detachPane,
    mergeTabInto,
    reorderTab,
    renameTab,
    focusedPaneId,
    getAllTerminalIds,
    serverTabs,
    activeServerTabId,
    serverPaneVisible,
    serverPaneHeight,
    addServerTab,
    setActiveServerTab,
    removeServerTab,
    setServerPaneVisible,
    setServerPaneHeight,
    sidebarMode,
    setSidebarMode,
    workspaceActive,
    codeReviewVisible,
    codeReviewWidth,
    toggleCodeReview,
    setCodeReviewWidth,
    zoomLevel,
    zoomIn,
    zoomOut,
    zoomReset,
  } = useStore();

  // Refit terminals after code review pane opens/closes
  useEffect(() => {
    // Delay to let the layout fully settle before terminals refit
    const timer = setTimeout(() => {
      window.dispatchEvent(new Event('resize'));
    }, 100);
    return () => clearTimeout(timer);
  }, [codeReviewVisible, codeReviewWidth]);

  // Apply zoom factor to CSS variable
  useEffect(() => {
    document.documentElement.style.setProperty('--zoom', String(zoomFactorFor(zoomLevel)));
  }, [zoomLevel]);

  // Initialize Monaco theme once
  useEffect(() => { configureMonacoTheme(); }, []);

  // Menu events are registered after callbacks are defined (see below)

  // Load a project: set state, restore saved tabs, fall back to tmux scan.
  // Closes any currently-open tabs from a prior project first.
  const loadProject = useCallback(async (info: { name: string; root: string; mainBranch: string }) => {
    try {
      // Close existing tabs (release their PTYs but leave tmux alive)
      const currentTabs = useStore.getState().tabs;
      for (const t of currentTabs) {
        for (const termId of useStore.getState().getAllTerminalIds(t)) {
          try { await CloseTerminal(termId); } catch {}
        }
        useStore.getState().removeTab(t.id);
      }

      await SetActiveProject(info.root);
      setProject({ name: info.name, root: info.root, mainBranch: info.mainBranch });
      const ws = await ListWorkspaces(info.root);
      setWorkspaces(ws);

      const mainWs = ws.find((w: any) => w.isMain);
      if (mainWs) setActiveWorkspace(mainWs.path);

      const savedTabs = (await GetSavedTabs()) || [];
      const restoredSessions = new Set<string>();
      for (const saved of savedTabs) {
        try {
          if (saved.tabType === 'claude-chat') {
            const session = saved.threadId
              ? await ResumeClaudeChatWithOptions(
                  info.root,
                  saved.workspacePath,
                  saved.threadId,
                  saved.model || '',
                  saved.reasoningEffort || '',
                  saved.approvalPolicy || '',
                  saved.sandboxMode || '',
                  saved.permissionMode || '',
                )
              : await AttachClaudeChat(saved.tmuxSession, saved.workspacePath);
            addTab({
              id: generateId('tab'),
              label: saved.label,
              rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'claude' } as PaneLeaf,
              tabType: 'claude-chat',
              workspacePath: saved.workspacePath,
              icon: saved.icon || 'claude',
              provider: 'claude',
              viewMode: 'chat',
              runtimeSessionId: session.id,
              threadId: session.threadId || saved.threadId,
              model: session.model || saved.model,
              reasoningEffort: session.reasoningEffort || saved.reasoningEffort,
              approvalPolicy: session.approvalPolicy || saved.approvalPolicy,
              sandboxMode: session.sandboxMode || saved.sandboxMode,
              permissionMode: session.permissionMode || saved.permissionMode,
            });
            restoredSessions.add(saved.threadId || saved.tmuxSession);
            continue;
          }
          if (saved.tabType === 'codex-chat' && saved.threadId) {
            const session = await ResumeCodexChatWithOptions(
              info.root,
              saved.workspacePath,
              saved.threadId,
              saved.model || '',
              saved.reasoningEffort || '',
              saved.approvalPolicy || '',
              saved.sandboxMode || '',
              saved.collaborationMode || '',
            );
            addTab({
              id: generateId('tab'),
              label: saved.label || 'Codex Chat',
              rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'codex' } as PaneLeaf,
              tabType: 'codex-chat',
              workspacePath: saved.workspacePath,
              icon: saved.icon || 'codex',
              provider: 'codex',
              viewMode: 'chat',
              runtimeSessionId: session.id,
              threadId: session.threadId || saved.threadId,
              model: session.model || saved.model,
              reasoningEffort: session.reasoningEffort || saved.reasoningEffort,
              approvalPolicy: session.approvalPolicy || saved.approvalPolicy,
              sandboxMode: session.sandboxMode || saved.sandboxMode,
              collaborationMode: session.collaborationMode || saved.collaborationMode,
            });
            restoredSessions.add(saved.threadId);
            continue;
          }
          const termId = generateId('term');
          await CreateAttachedTerminal(termId, saved.tmuxSession);
          addTab({
            id: generateId('tab'),
            label: saved.label,
            rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
            tabType: saved.tabType as 'shell' | 'claude' | 'codex' | 'server',
            workspacePath: saved.workspacePath,
            icon: saved.icon,
            provider: saved.provider as 'codex' | 'claude' | undefined,
            viewMode: 'terminal',
            runtimeSessionId: saved.runtimeSessionId,
            threadId: saved.threadId,
            model: saved.model,
            reasoningEffort: saved.reasoningEffort,
            approvalPolicy: saved.approvalPolicy,
            sandboxMode: saved.sandboxMode,
            permissionMode: saved.permissionMode,
            collaborationMode: saved.collaborationMode,
          });
          restoredSessions.add(saved.tmuxSession);
        } catch {}
      }

      // Fallback: scan tmux directly for any orion-* sessions that weren't
      // in savedTabs (e.g. saved-state was stale or never persisted before quit).
      try {
        const recovered = await RecoverSessions(info.name, ws.map((w: any) => w.path));
        for (const sess of (recovered || [])) {
          if (restoredSessions.has(sess.tmuxName)) continue;
          const termId = generateId('term');
          try {
            await CreateAttachedTerminal(termId, sess.tmuxName);
            const tab: Tab = {
              id: generateId('tab'),
              label: sess.label,
              rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
              tabType: (sess.type === 'server' || sess.type === 'claude' || sess.type === 'codex' ? sess.type : 'shell') as 'shell' | 'claude' | 'codex' | 'server',
              workspacePath: sess.workspacePath,
              icon: sess.icon,
              provider: sess.provider as 'codex' | 'claude' | undefined,
              viewMode: 'terminal',
              runtimeSessionId: sess.runtimeSessionId,
              threadId: sess.threadId,
              model: sess.model,
              reasoningEffort: sess.reasoningEffort,
              approvalPolicy: sess.approvalPolicy,
              sandboxMode: sess.sandboxMode,
              permissionMode: sess.permissionMode,
              collaborationMode: sess.collaborationMode,
            };
            if (sess.type === 'server') {
              useStore.getState().addServerTab(tab);
            } else {
              addTab(tab);
            }
            restoredSessions.add(sess.tmuxName);
          } catch {}
        }
      } catch {}

      // Reconcile active tab to belong to the active (main) workspace.
      // Each addTab above sets activeTabId to itself, so end state is the
      // last-added tab, which may belong to a non-main workspace.
      if (mainWs) setActiveWorkspace(mainWs.path);

      if (savedTabs.length === 0 && restoredSessions.size === 0 && mainWs) {
        const termId = generateId('term');
        await CreateTerminalInDir(termId, mainWs.path);
        addTab({
          id: generateId('tab'),
          label: 'Shell 1',
          rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
          tabType: 'shell',
          workspacePath: mainWs.path,
        });
      }
    } catch {}
  }, [addTab, setProject, setWorkspaces, setActiveWorkspace]);

  // Expose loadProject globally so Sidebar can trigger it after picking a project
  useEffect(() => {
    (window as any).__orionLoadProject = loadProject;
    return () => { delete (window as any).__orionLoadProject; };
  }, [loadProject]);

  // Initialize app on mount — load last project and restore saved tabs
  // Lives here (not in Sidebar) because App never unmounts, preventing duplicate tabs
  useEffect(() => {
    (async () => {
      try {
        const lastRoot = await GetLastProject();
        if (!lastRoot) return;
        const info = await GetProjectInfo(lastRoot);
        await loadProject(info);
      } catch {}
    })();
  }, []);

  const activeTabs = tabs.filter((t) => t.workspacePath === activeWorkspacePath);
  const serverOrder: Record<string, number> = { frontend: 0, backend: 1, sidekiq: 2 };
  const activeServerTabs = serverTabs
    .filter((t) => t.workspacePath === activeWorkspacePath)
    .sort((a, b) => (serverOrder[a.label?.toLowerCase() ?? ''] ?? 99) - (serverOrder[b.label?.toLowerCase() ?? ''] ?? 99));
  const activeTab = tabs.find((t) => t.id === activeTabId);
  const [dragOverTabId, setDragOverTabId] = useState<string | null>(null);
  const [dragMerge, setDragMerge] = useState(false);
  const [renamingTabId, setRenamingTabId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [searchEverywhereVisible, setSearchEverywhereVisible] = useState(false);
  const [commandPaletteVisible, setCommandPaletteVisible] = useState(false);
  const [newTabPickerVisible, setNewTabPickerVisible] = useState(false);
  const [agentTypes, setAgentTypes] = useState<main.AgentTypeInfo[]>([]);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; filePath: string } | null>(null);
  const [activeWorkspaceStatuses, setActiveWorkspaceStatuses] = useState<server.ServerStatus[]>([]);
  const [activeWorkspaceEnv, setActiveWorkspaceEnv] = useState<Record<string, string>>({});
  const [diffStats, setDiffStats] = useState<DiffStats>({ files: 0, added: 0, removed: 0, loading: false });
  const [conversionPicker, setConversionPicker] = useState<ConversionPickerState | null>(null);
  const [conversionPickerBusy, setConversionPickerBusy] = useState(false);
  const [conversionNotice, setConversionNotice] = useState<string | null>(null);
  const [toolbarPopover, setToolbarPopover] = useState<'servers' | 'env' | null>(null);
  const [toolbarBusy, setToolbarBusy] = useState<'servers' | null>(null);
  const [sidebarWidth, setSidebarWidth] = useState<number>(() => {
    const v = parseInt(localStorage.getItem('orion.sidebarWidth') || '', 10);
    return isNaN(v) ? 250 : v;
  });
  const [resizingSidebar, setResizingSidebar] = useState(false);
  useEffect(() => {
    localStorage.setItem('orion.sidebarWidth', String(sidebarWidth));
  }, [sidebarWidth]);

  useEffect(() => {
    if (!conversionNotice) return;
    const timer = window.setTimeout(() => setConversionNotice(null), 7000);
    return () => window.clearTimeout(timer);
  }, [conversionNotice]);

  const openCommandPalette = useCallback(() => {
    setSearchEverywhereVisible(false);
    setNewTabPickerVisible(false);
    setCommandPaletteVisible(true);
  }, []);

  useEffect(() => {
    if (!project) {
      setAgentTypes([]);
      return;
    }
    GetAgentTypes(project.root).then(setAgentTypes).catch(() => setAgentTypes([]));
  }, [project]);

  const refreshActiveWorkspaceMeta = useCallback(async () => {
    if (!project || !activeWorkspacePath) {
      setActiveWorkspaceStatuses([]);
      setActiveWorkspaceEnv({});
      return;
    }
    try {
      const [statuses, env] = await Promise.all([
        GetServerStatuses(project.root, activeWorkspacePath),
        GetWorkspaceEnv(activeWorkspacePath),
      ]);
      setActiveWorkspaceStatuses(statuses || []);
      setActiveWorkspaceEnv(env || {});
    } catch {
      setActiveWorkspaceStatuses([]);
      setActiveWorkspaceEnv({});
    }
  }, [activeWorkspacePath, project]);

  useEffect(() => {
    refreshActiveWorkspaceMeta();
    const interval = setInterval(refreshActiveWorkspaceMeta, 5000);
    const handleServerChange = () => refreshActiveWorkspaceMeta();
    window.addEventListener('orion:servers-changed', handleServerChange);
    return () => {
      clearInterval(interval);
      window.removeEventListener('orion:servers-changed', handleServerChange);
    };
  }, [refreshActiveWorkspaceMeta]);

  const refreshDiffStats = useCallback(async () => {
    if (!activeWorkspacePath) {
      setDiffStats({ files: 0, added: 0, removed: 0, loading: false });
      return;
    }
    setDiffStats((current) => ({ ...current, loading: true }));
    try {
      const files = (await GetChangedFilesAgainst(activeWorkspacePath, '')) || [];
      const diffs = await Promise.all(
        files.map(async (file) => {
          try {
            const raw = await GetUnifiedDiff(activeWorkspacePath, '', file.path);
            return parseUnifiedDiff(raw || '');
          } catch {
            return null;
          }
        }),
      );
      const totals = diffs.reduce(
        (acc, diff) => ({
          added: acc.added + (diff?.added || 0),
          removed: acc.removed + (diff?.removed || 0),
        }),
        { added: 0, removed: 0 },
      );
      setDiffStats({ files: files.length, added: totals.added, removed: totals.removed, loading: false });
    } catch {
      setDiffStats({ files: 0, added: 0, removed: 0, loading: false });
    }
  }, [activeWorkspacePath]);

  useEffect(() => {
    refreshDiffStats();
    if (!activeWorkspacePath) return;
    WatchWorkspace(activeWorkspacePath).catch(() => {});
    const cancel = EventsOn('git:files-changed', () => refreshDiffStats());
    const interval = setInterval(refreshDiffStats, 7000);
    return () => {
      cancel();
      clearInterval(interval);
    };
  }, [activeWorkspacePath, refreshDiffStats]);

  // Double-shift detection for Search Everywhere (like JetBrains)
  useEffect(() => {
    let lastShiftTime = 0;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Shift' && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const now = Date.now();
        if (now - lastShiftTime < 400) {
          setSearchEverywhereVisible(true);
          lastShiftTime = 0;
        } else {
          lastShiftTime = now;
        }
      } else {
        lastShiftTime = 0;
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const createNewShell = useCallback(async () => {
    if (!activeWorkspacePath) return;
    const terminalId = generateId('term');
    const shellNum = tabs.filter((t) => t.workspacePath === activeWorkspacePath && t.tabType === 'shell').length + 1;

    try {
      await CreateTerminalInDir(terminalId, activeWorkspacePath);
      const pane: PaneLeaf = { type: 'terminal', id: generateId('pane'), terminalId };
      addTab({
        id: generateId('tab'),
        label: `Shell ${shellNum}`,
        rootPane: pane,
        tabType: 'shell',
        workspacePath: activeWorkspacePath,
      });
    } catch (err) {
      console.error('Failed to create terminal:', err);
    }
  }, [activeWorkspacePath, tabs, addTab]);

  const launchAgentTab = useCallback(async (agent: main.AgentTypeInfo) => {
    if (!project || !activeWorkspacePath) return;
    try {
      const tmuxSession = await LaunchAgent(project.root, activeWorkspacePath, agent.name);
      const termId = generateId('term');
      await CreateAttachedTerminal(termId, tmuxSession);
      const provider = agentProvider(agent);
      const icon = agentIcon(agent);
      addTab({
        id: generateId('tab'),
        label: agent.label,
        rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
        tabType: provider || 'shell',
        workspacePath: activeWorkspacePath,
        icon,
        provider,
        viewMode: 'terminal',
        runtimeSessionId: tmuxSession,
        model: agent.model,
        reasoningEffort: agent.reasoningEffort,
        approvalPolicy: agent.approvalPolicy,
        sandboxMode: agent.sandboxMode,
        permissionMode: agent.permissionMode,
        collaborationMode: agent.collaborationMode,
      });
    } catch (err) {
      console.error('Failed to launch agent:', err);
    }
  }, [project, activeWorkspacePath, addTab]);

  const launchCodexChatTab = useCallback(async () => {
    if (!project || !activeWorkspacePath) return;
    const options = codexOptionsForAgent(agentTypes.find((agent) => agentProvider(agent) === 'codex'));
    try {
      const session = await LaunchCodexChatWithOptions(
        project.root,
        activeWorkspacePath,
        options.model,
        options.reasoningEffort,
        options.approvalPolicy,
        options.sandboxMode,
        options.collaborationMode,
      );
      addTab({
        id: generateId('tab'),
        label: session?.label || 'Codex Chat',
        rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'codex' } as PaneLeaf,
        tabType: 'codex-chat',
        workspacePath: activeWorkspacePath,
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
    } catch (err) {
      console.error('Failed to launch Codex chat:', err);
    }
  }, [project, activeWorkspacePath, agentTypes, addTab]);

  const launchClaudeChatTab = useCallback(async () => {
    if (!project || !activeWorkspacePath) return;
    try {
      const session = await LaunchClaudeChat(project.root, activeWorkspacePath);
      addTab({
        id: generateId('tab'),
        label: session?.label || 'Claude Chat',
        rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'claude' } as PaneLeaf,
        tabType: 'claude-chat',
        workspacePath: activeWorkspacePath,
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
    } catch (err) {
      console.error('Failed to launch Claude chat:', err);
    }
  }, [project, activeWorkspacePath, addTab]);

  const handleNewTabPick = useCallback((choice: NewTabChoice) => {
    if (choice.kind === 'shell') createNewShell();
    else {
      const agent = agentTypes.find((candidate) => candidate.name === choice.name);
      if (agent) launchAgentTab(agent);
    }
  }, [agentTypes, createNewShell, launchAgentTab]);

  // Open a Diagnostics tab globally: if one exists anywhere, switch to its
  // workspace and focus it. Otherwise create a new one in the active workspace.
  const openDiagnostics = useCallback(() => {
    const state = useStore.getState();
    const existing = state.tabs.find((t) => t.tabType === 'diagnostics');
    if (existing) {
      if (existing.workspacePath !== state.activeWorkspacePath) {
        state.setActiveWorkspace(existing.workspacePath);
      }
      state.setActiveTab(existing.id);
      return;
    }
    if (!activeWorkspacePath) return;
    const pane: PaneLeaf = { type: 'diagnostics', id: generateId('pane') };
    addTab({
      id: generateId('tab'),
      label: 'Diagnostics',
      rootPane: pane,
      tabType: 'diagnostics',
      workspacePath: activeWorkspacePath,
    });
  }, [activeWorkspacePath, addTab]);

  const openProjectDialog = useCallback(async () => {
    try {
      const info = await OpenProjectDialog();
      if (info) await loadProject(info);
    } catch (err) {
      console.error('Failed to open project:', err);
    }
  }, [loadProject]);

  const openNewWorkspaceFlow = useCallback(() => {
    setSidebarMode('workspaces');
    window.setTimeout(() => window.dispatchEvent(new Event('orion:new-workspace')), 0);
  }, [setSidebarMode]);

  const openActiveWorkspaceInBrowser = useCallback(async () => {
    if (!project || !activeWorkspacePath) return;
    try {
      await OpenBrowser(project.root, activeWorkspacePath);
    } catch (err) {
      console.error('Failed to open browser:', err);
    }
  }, [project, activeWorkspacePath]);

  const startServersForActiveWorkspace = useCallback(async () => {
    if (!project || !activeWorkspacePath) return;
    const workspace = workspaces.find((ws) => ws.path === activeWorkspacePath);
    setToolbarBusy('servers');
    try {
      const statuses = await StartServers(project.root, activeWorkspacePath, workspace?.isMain || false);
      setActiveWorkspaceStatuses(statuses || []);
      const env = await GetWorkspaceEnv(activeWorkspacePath).catch(() => ({}));
      setActiveWorkspaceEnv(env || {});
      window.dispatchEvent(new Event('orion:servers-changed'));
      const existingServerTabs = useStore.getState().serverTabs;
      for (const srv of statuses || []) {
        if (!srv.running || !srv.tmuxSession) continue;
        const exists = existingServerTabs.some((tab) =>
          tab.workspacePath === activeWorkspacePath &&
          tab.label.toLowerCase() === srv.name.toLowerCase(),
        );
        if (exists) continue;
        const termId = generateId('term');
        await CreateAttachedTerminal(termId, srv.tmuxSession);
        addServerTab({
          id: generateId('tab'),
          label: srv.name.charAt(0).toUpperCase() + srv.name.slice(1),
          rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
          tabType: 'server',
          workspacePath: activeWorkspacePath,
        });
      }
      setServerPaneVisible(true);
    } catch (err) {
      console.error('Failed to start servers:', err);
    } finally {
      setToolbarBusy(null);
    }
  }, [project, activeWorkspacePath, workspaces, addServerTab, setServerPaneVisible]);

  const stopServersForActiveWorkspace = useCallback(async () => {
    if (!activeWorkspacePath) return;
    setToolbarBusy('servers');
    try {
      await StopServers(activeWorkspacePath);
      setActiveWorkspaceStatuses([]);
      setActiveWorkspaceEnv({});
      window.dispatchEvent(new Event('orion:servers-changed'));
      const tabsToClose = useStore.getState().serverTabs.filter((tab) => tab.workspacePath === activeWorkspacePath);
      for (const tab of tabsToClose) {
        for (const termId of useStore.getState().getAllTerminalIds(tab)) {
          try { await CloseTerminal(termId); } catch {}
        }
        removeServerTab(tab.id);
      }
    } catch (err) {
      console.error('Failed to stop servers:', err);
    } finally {
      setToolbarBusy(null);
    }
  }, [activeWorkspacePath, removeServerTab]);

  const diagnosticsActive = !!tabs.find(
    (t) => t.tabType === 'diagnostics' && t.id === activeTabId,
  );

  const handleSplit = useCallback(async (direction: 'horizontal' | 'vertical') => {
    if (!activeWorkspacePath || !focusedPaneId) return;
    const terminalId = generateId('term');
    try {
      await CreateTerminalInDir(terminalId, activeWorkspacePath);
      splitPane(focusedPaneId, direction, terminalId);
    } catch (err) {
      console.error('Failed to create split terminal:', err);
    }
  }, [activeWorkspacePath, focusedPaneId, splitPane]);

  const handleClosePane = useCallback(async () => {
    if (!focusedPaneId) return;
    const terminalId = closePane(focusedPaneId);
    if (terminalId) {
      try {
        await CloseTerminal(terminalId);
      } catch {}
    }
  }, [focusedPaneId, closePane]);

  const getChatSessions = useCallback((pane: Pane, fallbackKind: 'codex' | 'claude' = 'codex'): { id: string; kind: 'codex' | 'claude'; threadId?: string }[] => {
    if (pane.type === 'chat' && pane.chatSessionId) {
      return [{ id: pane.chatSessionId, kind: pane.chatKind || fallbackKind, threadId: pane.chatThreadId }];
    }
    if (!('children' in pane)) return [];
    return pane.children.flatMap((child) => getChatSessions(child, fallbackKind));
  }, []);

  const sessionKeys = useCallback((session: Partial<state.SessionInfo> | any) => {
    return [
      session?.tmuxName,
      session?.tmuxSession,
      session?.runtimeSessionId,
      session?.sessionId,
      session?.threadId,
    ].filter((value): value is string => typeof value === 'string' && value.trim() !== '');
  }, []);

  const tabKeys = useCallback((tab: Tab) => {
    const fallbackKind = tab.tabType === 'claude-chat' ? 'claude' : 'codex';
    return [
      tab.runtimeSessionId,
      tab.threadId,
      ...getChatSessions(tab.rootPane, fallbackKind).flatMap((chat) => [chat.id, chat.threadId]),
    ].filter((value): value is string => typeof value === 'string' && value.trim() !== '');
  }, [getChatSessions]);

  const tabMatchesSession = useCallback((tab: Tab, session: Partial<state.SessionInfo> | any) => {
    if (session?.workspacePath && tab.workspacePath !== session.workspacePath) return false;
    const wanted = new Set(sessionKeys(session));
    return tabKeys(tab).some((key) => wanted.has(key));
  }, [sessionKeys, tabKeys]);

  const addChatSessionTab = useCallback((session: state.SessionInfo) => {
    const chatKind: AgentKind = session.type === 'claude-chat' ? 'claude' : 'codex';
    addTab({
      id: generateId('tab'),
      label: session.label || (chatKind === 'claude' ? 'Claude Chat' : 'Codex Chat'),
      rootPane: {
        type: 'chat',
        id: generateId('pane'),
        chatSessionId: session.runtimeSessionId || session.tmuxName,
        chatThreadId: session.threadId,
        chatKind,
      } as PaneLeaf,
      tabType: session.type as 'codex-chat' | 'claude-chat',
      workspacePath: session.workspacePath,
      icon: session.icon || chatKind,
      provider: chatKind,
      viewMode: 'chat',
      runtimeSessionId: session.runtimeSessionId || session.tmuxName,
      threadId: session.threadId,
      model: session.model,
      reasoningEffort: session.reasoningEffort,
      approvalPolicy: session.approvalPolicy,
      sandboxMode: session.sandboxMode,
      permissionMode: session.permissionMode,
      collaborationMode: session.collaborationMode,
    });
  }, [addTab]);

  const handleCloseTab = useCallback(async (tabId: string) => {
    const tab = tabs.find((t) => t.id === tabId);
    if (!tab) return;
    const termIds = getAllTerminalIds(tab);
    for (const termId of termIds) {
      try {
        await CloseTerminal(termId);
      } catch {}
    }
    const fallbackKind = tab.tabType === 'claude-chat' ? 'claude' : 'codex';
    for (const session of getChatSessions(tab.rootPane, fallbackKind)) {
      try {
        if (session.kind === 'claude') {
          await StopClaudeChat(session.id);
        } else {
          await StopCodexChat(session.id);
        }
      } catch {}
    }
    removeTab(tabId);
  }, [tabs, removeTab, getAllTerminalIds, getChatSessions]);

  const replaceTerminalWithChat = useCallback(async (
    tab: Tab,
    kind: AgentKind,
    session: claudesdk.SessionInfo | codexchat.SessionInfo,
  ) => {
    for (const termId of getAllTerminalIds(tab)) {
      try { await CloseTerminal(termId); } catch {}
    }
    addTab({
      id: generateId('tab'),
      label: chatTabLabel(kind, session?.label),
      rootPane: {
        type: 'chat',
        id: generateId('pane'),
        chatSessionId: session.id,
        chatThreadId: session.threadId,
        chatKind: kind,
      } as PaneLeaf,
      tabType: kind === 'claude' ? 'claude-chat' : 'codex-chat',
      workspacePath: tab.workspacePath,
      icon: tab.icon || kind,
      provider: kind,
      viewMode: 'chat',
      runtimeSessionId: session.id,
      threadId: session.threadId || tab.threadId,
      model: session.model || tab.model,
      reasoningEffort: session.reasoningEffort || tab.reasoningEffort,
      approvalPolicy: session.approvalPolicy || tab.approvalPolicy,
      sandboxMode: session.sandboxMode || tab.sandboxMode,
      permissionMode: kind === 'claude'
        ? (session as claudesdk.SessionInfo).permissionMode || tab.permissionMode
        : tab.permissionMode,
      collaborationMode: kind === 'codex'
        ? (session as codexchat.SessionInfo).collaborationMode || tab.collaborationMode
        : tab.collaborationMode,
    });
    removeTab(tab.id);
  }, [addTab, getAllTerminalIds, removeTab]);

  const openConversionHistoryPicker = useCallback(async (
    kind: AgentKind,
    tab: Tab,
    err: unknown,
  ) => {
    const message = errorMessage(err);
    try {
      const rawHistory = kind === 'claude'
        ? await ListClaudeChatHistory(tab.workspacePath, 25)
        : await ListCodexChatHistory(tab.workspacePath, 25);
      const candidates = (rawHistory || [])
        .filter((thread) => thread.threadId && thread.messageCount > 0)
        .map(normalizeHistoryCandidate);

      if (candidates.length === 0) {
        setConversionNotice(message || `No saved ${kind === 'claude' ? 'Claude' : 'Codex'} history was found for this workspace.`);
        return;
      }

      setConversionPicker({
        kind,
        tabId: tab.id,
        workspacePath: tab.workspacePath,
        error: message,
        candidates,
      });
      setConversionNotice(null);
    } catch (historyErr) {
      setConversionNotice(errorMessage(historyErr) || message || 'Could not load saved chat history.');
    }
  }, []);

  const handlePickConversionHistory = useCallback(async (threadId: string) => {
    if (!conversionPicker || !project) return;
    const tab = useStore.getState().tabs.find((candidate) => candidate.id === conversionPicker.tabId);
    if (!tab) {
      setConversionPicker(null);
      setConversionNotice('The terminal tab is no longer available.');
      return;
    }
    setConversionPickerBusy(true);
    try {
      const session = conversionPicker.kind === 'claude'
        ? await ResumeClaudeChatWithOptions(
            project.root,
            tab.workspacePath,
            threadId,
            tab.model || '',
            tab.reasoningEffort || '',
            tab.approvalPolicy || '',
            tab.sandboxMode || '',
            tab.permissionMode || '',
          )
        : await ResumeCodexChatWithOptions(
            project.root,
            tab.workspacePath,
            threadId,
            tab.model || '',
            tab.reasoningEffort || '',
            tab.approvalPolicy || '',
            tab.sandboxMode || '',
            tab.collaborationMode || '',
          );
      await replaceTerminalWithChat(tab, conversionPicker.kind, session);
      setConversionPicker(null);
    } catch (err) {
      setConversionPicker((current) => current ? { ...current, error: errorMessage(err) || 'Could not resume selected history.' } : current);
    } finally {
      setConversionPickerBusy(false);
    }
  }, [conversionPicker, project, replaceTerminalWithChat]);

  const handleConvertTab = useCallback(async (tabId: string) => {
    const tab = tabs.find((t) => t.id === tabId);
    if (!tab || !project) return;

    if (tab.tabType === 'claude-chat' || tab.tabType === 'codex-chat') {
      const fallbackKind = tab.tabType === 'claude-chat' ? 'claude' : 'codex';
      const chat = getChatSessions(tab.rootPane, fallbackKind)[0];
      if (!chat) return;
      try {
        const tmuxSession = await ConvertChatToTerminalWithOptions(
          project.root,
          tab.workspacePath,
          chat.id,
          chat.kind,
          tab.model || '',
          tab.reasoningEffort || '',
          tab.permissionMode || '',
          tab.collaborationMode || '',
        );
        const termId = generateId('term');
        await CreateAttachedTerminal(termId, tmuxSession);
        addTab({
          id: generateId('tab'),
          label: chat.kind === 'claude' ? 'Claude' : 'Codex',
          rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
          tabType: chat.kind,
          workspacePath: tab.workspacePath,
          provider: chat.kind,
          viewMode: 'terminal',
          threadId: chat.threadId || tab.threadId,
          model: tab.model,
          reasoningEffort: tab.reasoningEffort,
          approvalPolicy: tab.approvalPolicy,
          sandboxMode: tab.sandboxMode,
          permissionMode: tab.permissionMode,
          collaborationMode: tab.collaborationMode,
        });
        removeTab(tab.id);
      } catch (err) {
        console.error('Failed to convert chat to terminal:', err);
      }
      return;
    }

    if (tab.tabType === 'claude' || tab.tabType === 'codex') {
      const kind = tab.tabType;
      try {
        if (kind === 'claude') {
          const termId = getAllTerminalIds(tab)[0];
          if (!termId) return;
          const tmuxSession = await GetTmuxSession(termId);
          if (!tmuxSession) return;
          const session = await ConvertTerminalToClaudeChatWithOptions(
            project.root,
            tab.workspacePath,
            tmuxSession,
            tab.model || '',
            tab.reasoningEffort || '',
            tab.approvalPolicy || '',
            tab.sandboxMode || '',
            tab.permissionMode || '',
          );
          await replaceTerminalWithChat(tab, 'claude', session);
          return;
        }
        const termId = getAllTerminalIds(tab)[0];
        if (!termId) return;
        const tmuxSession = await GetTmuxSession(termId);
        if (!tmuxSession) return;
        const session = await ConvertTerminalToCodexChatWithOptions(
          project.root,
          tab.workspacePath,
          tmuxSession,
          tab.model || '',
          tab.reasoningEffort || '',
          tab.approvalPolicy || '',
          tab.sandboxMode || '',
          tab.collaborationMode || '',
        );
        await replaceTerminalWithChat(tab, 'codex', session);
      } catch (err) {
        console.error('Failed to convert terminal to chat:', err);
        await openConversionHistoryPicker(kind, tab, err);
      }
    }
  }, [tabs, project, getChatSessions, addTab, removeTab, getAllTerminalIds, replaceTerminalWithChat, openConversionHistoryPicker]);

  // Persist tabs to disk whenever they change (for recovery on restart)
  useEffect(() => {
    if (tabs.length === 0) return;
    (async () => {
      const savedTabs = [];
      for (const tab of tabs) {
        if (tab.tabType === 'codex-chat') {
          const chat = getChatSessions(tab.rootPane, 'codex')[0];
          const threadId = chat?.threadId || tab.threadId;
          if (chat?.id || threadId) {
            savedTabs.push({
              label: tab.label,
              tabType: tab.tabType,
              tmuxSession: '',
              workspacePath: tab.workspacePath,
              icon: tab.icon || 'codex',
              provider: 'codex',
              viewMode: 'chat',
              runtimeSessionId: chat?.id || tab.runtimeSessionId || '',
              threadId: threadId || '',
              model: tab.model || '',
              reasoningEffort: tab.reasoningEffort || '',
              approvalPolicy: tab.approvalPolicy || '',
              sandboxMode: tab.sandboxMode || '',
              permissionMode: tab.permissionMode || '',
              collaborationMode: tab.collaborationMode || '',
            });
          }
          continue;
        }
        if (tab.tabType === 'claude-chat') {
          const chat = getChatSessions(tab.rootPane, 'claude')[0];
          if (chat?.id) {
            savedTabs.push({
              label: tab.label,
              tabType: tab.tabType,
              tmuxSession: chat.id,
              workspacePath: tab.workspacePath,
              icon: tab.icon || 'claude',
              provider: 'claude',
              viewMode: 'chat',
              runtimeSessionId: chat.id,
              threadId: chat.threadId || tab.threadId || '',
              model: tab.model || '',
              reasoningEffort: tab.reasoningEffort || '',
              approvalPolicy: tab.approvalPolicy || '',
              sandboxMode: tab.sandboxMode || '',
              permissionMode: tab.permissionMode || '',
              collaborationMode: tab.collaborationMode || '',
            });
          }
          continue;
        }
        // Get all terminal IDs and their tmux sessions
        const termIds = getAllTerminalIds(tab);
        for (const termId of termIds) {
          const tmuxSession = await GetTmuxSession(termId);
          if (tmuxSession) {
            savedTabs.push({
              label: tab.label,
              tabType: tab.tabType,
              tmuxSession,
              workspacePath: tab.workspacePath,
              icon: tab.icon || '',
              provider: tab.provider || (tab.tabType === 'claude' || tab.tabType === 'codex' ? tab.tabType : ''),
              viewMode: 'terminal',
              runtimeSessionId: tab.runtimeSessionId || tmuxSession,
              threadId: tab.threadId || '',
              model: tab.model || '',
              reasoningEffort: tab.reasoningEffort || '',
              approvalPolicy: tab.approvalPolicy || '',
              sandboxMode: tab.sandboxMode || '',
              permissionMode: tab.permissionMode || '',
              collaborationMode: tab.collaborationMode || '',
            });
          }
        }
      }
      if (savedTabs.length > 0) {
        await SaveTabs(savedTabs);
      }
    })();
  }, [tabs, getAllTerminalIds, getChatSessions]);

  const syncLiveChatTabs = useCallback(async () => {
    if (!project || workspaces.length === 0) return;
    const workspacePaths = workspaces.map((w: any) => w.path).filter(Boolean);
    if (workspacePaths.length === 0) return;
    try {
      const [codexSessions, claudeSessions] = await Promise.all([
        ListCodexChatSessions(workspacePaths),
        ListClaudeChatSessions(workspacePaths),
      ]);
      const live = [...(codexSessions || []), ...(claudeSessions || [])];
      const currentTabs = useStore.getState().tabs;

      for (const session of live) {
        if (!currentTabs.some((tab) => tabMatchesSession(tab, session))) {
          addChatSessionTab(session);
        }
      }

      const liveKeys = new Set(live.flatMap((session) => sessionKeys(session)));
      for (const tab of currentTabs) {
        if (tab.tabType !== 'codex-chat' && tab.tabType !== 'claude-chat') continue;
        if (!workspacePaths.includes(tab.workspacePath)) continue;
        if (!tabKeys(tab).some((key) => liveKeys.has(key))) {
          removeTab(tab.id);
        }
      }
    } catch (err) {
      console.debug('Failed to sync live chat tabs:', err);
    }
  }, [project, workspaces, addChatSessionTab, removeTab, sessionKeys, tabKeys, tabMatchesSession]);

  useEffect(() => {
    if (!project || workspaces.length === 0) return;
    syncLiveChatTabs();
    const interval = setInterval(syncLiveChatTabs, 3000);
    return () => clearInterval(interval);
  }, [project, workspaces, syncLiveChatTabs]);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (commandPaletteVisible && e.key === 'Escape') {
        e.preventDefault();
        setCommandPaletteVisible(false);
        return;
      }
      // Cmd+K / Cmd+Shift+P: command palette
      if (e.metaKey && ((!e.shiftKey && e.key.toLowerCase() === 'k') || (e.shiftKey && e.key.toLowerCase() === 'p'))) {
        e.preventDefault();
        e.stopPropagation();
        openCommandPalette();
        return;
      }
      // Cmd+T: open New Session picker (pick shell / claude / codex)
      if (e.metaKey && !e.shiftKey && e.key === 't') {
        e.preventDefault();
        setNewTabPickerVisible(true);
      }
      // Cmd+W: close focused pane (or tab if single pane)
      if (e.metaKey && !e.shiftKey && e.key === 'w') {
        e.preventDefault();
        handleClosePane();
      }
      // Cmd+D: split right (vertical)
      if (e.metaKey && !e.shiftKey && e.key === 'd') {
        e.preventDefault();
        handleSplit('vertical');
      }
      // Cmd+Shift+D: split down (horizontal)
      if (e.metaKey && e.shiftKey && (e.key === 'D' || e.key === 'd')) {
        e.preventDefault();
        handleSplit('horizontal');
      }
      // Cmd+[ : previous pane
      if (e.metaKey && e.key === '[') {
        e.preventDefault();
        navigatePane('prev');
      }
      // Cmd+] : next pane
      if (e.metaKey && e.key === ']') {
        e.preventDefault();
        navigatePane('next');
      }
      // Cmd+Shift+R: rotate split direction (vertical ↔ horizontal)
      if (e.metaKey && e.shiftKey && e.key === 'R') {
        e.preventDefault();
        rotateSplit();
      }
      // Cmd+Shift+T: detach focused pane into its own tab
      if (e.metaKey && e.shiftKey && e.key === 'T') {
        e.preventDefault();
        detachPane();
      }
      // Cmd+Shift+[ : swap pane left
      if (e.metaKey && e.shiftKey && e.key === '{') {
        e.preventDefault();
        swapPane('prev');
      }
      // Cmd+Shift+] : swap pane right
      if (e.metaKey && e.shiftKey && e.key === '}') {
        e.preventDefault();
        swapPane('next');
      }
      // Cmd+Shift+= (i.e. Cmd+Plus) : toggle Code Review pane
      if (e.metaKey && e.shiftKey && (e.key === '+' || e.key === '=')) {
        e.preventDefault();
        toggleCodeReview();
      }
      // Cmd+Up/Down: cycle through workspaces in the same order as the sidebar
      if (e.metaKey && !e.shiftKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
        e.preventDefault();
        const sorted = sortWorkspaces(workspaces, useStore.getState().workspaceActive);
        if (sorted.length === 0) return;
        const currentIdx = sorted.findIndex((w) => w.path === activeWorkspacePath);
        const delta = e.key === 'ArrowUp' ? -1 : 1;
        const nextIdx = (currentIdx + delta + sorted.length) % sorted.length;
        setActiveWorkspace(sorted[nextIdx].path);
      }
      // Cmd+Left/Right: cycle tabs
      if (e.metaKey && !e.shiftKey && e.key === 'ArrowLeft') {
        e.preventDefault();
        const currentIdx = activeTabs.findIndex((t) => t.id === activeTabId);
        const prevIdx = (currentIdx - 1 + activeTabs.length) % activeTabs.length;
        if (activeTabs[prevIdx]) setActiveTab(activeTabs[prevIdx].id);
      }
      if (e.metaKey && !e.shiftKey && e.key === 'ArrowRight') {
        e.preventDefault();
        const currentIdx = activeTabs.findIndex((t) => t.id === activeTabId);
        const nextIdx = (currentIdx + 1) % activeTabs.length;
        if (activeTabs[nextIdx]) setActiveTab(activeTabs[nextIdx].id);
      }
      // Cmd+Shift+N: new Orion window
      if (e.metaKey && e.shiftKey && e.key === 'N') {
        e.preventDefault();
        NewWindow();
      }
      // Cmd+Shift+F: global search (with selection pre-fill)
      if (e.metaKey && e.shiftKey && (e.key === 'F' || e.key === 'f')) {
        e.preventDefault();
        e.stopPropagation();
        // Grab selected text from window or Monaco
        const sel = window.getSelection()?.toString()?.trim() || '';
        if (sel) {
          useStore.getState().setGlobalSearchQuery(sel);
        }
        setSidebarMode(sidebarMode === 'search' ? null : 'search');
      }
      // Cmd+Shift+E: file explorer
      if (e.metaKey && e.shiftKey && e.key === 'E') {
        e.preventDefault();
        setSidebarMode(sidebarMode === 'files' ? null : 'files');
      }
      // Cmd+Shift+G: toggle code review pane
      if (e.metaKey && e.shiftKey && e.key === 'G') {
        e.preventDefault();
        toggleCodeReview();
      }
      // Cmd+= / Cmd++ : zoom in
      if (e.metaKey && !e.shiftKey && (e.key === '=' || e.key === '+')) {
        e.preventDefault();
        zoomIn();
      }
      // Cmd+- : zoom out
      if (e.metaKey && !e.shiftKey && e.key === '-') {
        e.preventDefault();
        zoomOut();
      }
      // Cmd+0 : reset zoom
      if (e.metaKey && !e.shiftKey && e.key === '0') {
        e.preventDefault();
        zoomReset();
      }
      // Cmd+B: toggle sidebar
      if (e.metaKey && !e.shiftKey && e.key === 'b') {
        e.preventDefault();
        setSidebarMode(sidebarMode ? null : 'workspaces');
      }
      // Cmd+J: toggle server pane
      if (e.metaKey && !e.shiftKey && e.key === 'j') {
        e.preventDefault();
        setServerPaneVisible(!serverPaneVisible);
      }
    };

    window.addEventListener('keydown', handleKeyDown, { capture: true });
    return () => window.removeEventListener('keydown', handleKeyDown, { capture: true } as any);
  }, [
    activeTabId,
    activeTabs,
    activeWorkspacePath,
    commandPaletteVisible,
    detachPane,
    handleClosePane,
    handleSplit,
    navigatePane,
    openCommandPalette,
    rotateSplit,
    serverPaneVisible,
    setActiveTab,
    setActiveWorkspace,
    setServerPaneVisible,
    setSidebarMode,
    sidebarMode,
    swapPane,
    toggleCodeReview,
    workspaces,
    zoomIn,
    zoomOut,
    zoomReset,
  ]);

  // Listen for native menu bar events from Go
  useEffect(() => {
    const cancels = [
      EventsOn('menu:open-project', () => openProjectDialog()),
      EventsOn('menu:new-terminal', () => setNewTabPickerVisible(true)),
      EventsOn('menu:close-tab', () => handleClosePane()),
      EventsOn('menu:toggle-sidebar', () => setSidebarMode(sidebarMode ? null : 'workspaces')),
      EventsOn('menu:show-files', () => setSidebarMode('files')),
      EventsOn('menu:show-search', () => setSidebarMode('search')),
      EventsOn('menu:show-git', () => toggleCodeReview()),
      EventsOn('menu:show-workspaces', () => setSidebarMode('workspaces')),
      EventsOn('menu:split-right', () => handleSplit('vertical')),
      EventsOn('menu:split-down', () => handleSplit('horizontal')),
      EventsOn('menu:next-pane', () => navigatePane('next')),
      EventsOn('menu:prev-pane', () => navigatePane('prev')),
      EventsOn('mobile:session-created', async (data: any) => {
        if (!data?.tmuxSession || !data?.workspacePath) return;
        // Only add if this workspace belongs to the current project
        const ws = useStore.getState().workspaces;
        if (!ws.some((w: any) => w.path === data.workspacePath)) return;
        const incoming = { ...data, tmuxName: data.tmuxName || data.tmuxSession } as state.SessionInfo;
        if (useStore.getState().tabs.some((tab) => tabMatchesSession(tab, incoming))) return;
        if (data.type === 'codex-chat' || data.type === 'claude-chat') {
          addChatSessionTab(incoming);
          return;
        }
        const termId = generateId('term');
        try {
          await CreateAttachedTerminal(termId, data.tmuxSession);
          addTab({
            id: generateId('tab'),
            label: data.label || 'Shell',
            rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
            tabType: (data.type === 'claude' || data.type === 'codex') ? data.type : 'shell',
            workspacePath: data.workspacePath,
            icon: data.icon,
            provider: data.provider || (data.type === 'claude' || data.type === 'codex' ? data.type : undefined),
            viewMode: 'terminal',
            runtimeSessionId: data.runtimeSessionId || data.tmuxSession,
            threadId: data.threadId,
          });
        } catch {}
      }),
      EventsOn('mobile:session-killed', async (data: any) => {
        const target = data?.sessionId || data?.tmuxSession;
        if (!target) return;
        const killed = { sessionId: target, tmuxSession: target, threadId: data?.threadId } as any;
        for (const tab of useStore.getState().tabs) {
          const termIds = useStore.getState().getAllTerminalIds(tab);
          let matched = tabMatchesSession(tab, killed);
          if (!matched) {
            for (const termId of termIds) {
              try {
                if (await GetTmuxSession(termId) === target) {
                  matched = true;
                  break;
                }
              } catch {}
            }
          }
          if (!matched) continue;
          for (const termId of termIds) {
            try { await CloseTerminal(termId); } catch {}
          }
          removeTab(tab.id);
        }
      }),
      EventsOn('agent:focus', async (data: any) => {
        const cwd: string | undefined = data?.cwd;
        const targetTmux: string | undefined = data?.tmuxSession;
        if (!cwd) return;
        const state = useStore.getState();
        if (state.workspaces.some((w: any) => w.path === cwd)) {
          state.setActiveWorkspace(cwd);
        }
        const candidates = state.tabs.filter((t: Tab) => t.workspacePath === cwd);
        let target: Tab | undefined;
        if (targetTmux) {
          for (const tab of candidates) {
            const termIds = state.getAllTerminalIds(tab);
            for (const termId of termIds) {
              try {
                const ts = await GetTmuxSession(termId);
                if (ts === targetTmux) { target = tab; break; }
              } catch {}
            }
            if (target) break;
          }
        }
        if (!target) target = candidates.find((t) => t.tabType === 'claude') || candidates[0];
        if (target) useStore.getState().setActiveTab(target.id);
      }),
    ];
    return () => cancels.forEach((c) => c());
  }, [sidebarMode, handleClosePane, handleSplit, navigatePane, setSidebarMode, addTab, addChatSessionTab, removeTab, openProjectDialog, tabMatchesSession, toggleCodeReview]);

  useEffect(() => {
    const syncChatMetadata = (kind: 'claude' | 'codex', msg: any) => {
      const sessionId = msg?.sessionId;
      if (!sessionId) return;
      const details = parseChatMetadata(msg);
      const nextThreadId = details.threadId || msg.threadId;
      if (!nextThreadId && !details.model && !details.reasoningEffort && !details.approvalPolicy && !details.sandboxMode && !details.permissionMode && !details.collaborationMode) {
        return;
      }
      useStore.setState((state) => ({
        tabs: state.tabs.map((tab) => {
          if (tab.tabType !== `${kind}-chat`) return tab;
          if (!chatPaneHasSession(tab.rootPane, sessionId)) return tab;
          return {
            ...tab,
            threadId: nextThreadId || tab.threadId,
            model: details.model || tab.model,
            reasoningEffort: details.reasoningEffort || tab.reasoningEffort,
            approvalPolicy: details.approvalPolicy || tab.approvalPolicy,
            sandboxMode: details.sandboxMode || tab.sandboxMode,
            permissionMode: details.permissionMode || tab.permissionMode,
            collaborationMode: details.collaborationMode || tab.collaborationMode,
            rootPane: updateChatPaneThreadId(tab.rootPane, sessionId, nextThreadId || tab.threadId),
          };
        }),
      }));
    };

    const cancels = [
      EventsOn('claude-chat:message', (msg: any) => syncChatMetadata('claude', msg)),
      EventsOn('codex-chat:message', (msg: any) => syncChatMetadata('codex', msg)),
    ];
    return () => cancels.forEach((cancel) => cancel());
  }, []);

  const activeWorkspace = workspaces.find((w) => w.path === activeWorkspacePath);

  // Count total panes in active tab
  const countPanes = (tab: Tab | undefined): number => {
    if (!tab) return 0;
    return getAllTerminalIds(tab).length;
  };
  const paneCount = countPanes(activeTab);
  const sessionStatus = activeTab ? 'ready' : 'ready';

  useEffect(() => {
    setToolbarPopover(null);
  }, [activeWorkspacePath]);

  const commandPaletteCommands = useMemo<CommandPaletteItem[]>(() => {
    const activeWorkspaceName = activeWorkspace
      ? (activeWorkspace.isMain ? 'main' : activeWorkspace.branch || activeWorkspace.name)
      : 'No workspace selected';
    const hasActiveWorkspace = Boolean(project && activeWorkspacePath);

    const commands: CommandPaletteItem[] = [
      {
        id: 'new-tab',
        title: 'New Session',
        subtitle: 'Pick Shell, Claude, Codex, or another configured agent',
        group: 'Create',
        icon: 'shell',
        shortcut: '⌘T',
        keywords: ['terminal', 'agent', 'picker'],
        disabled: !hasActiveWorkspace,
        run: () => setNewTabPickerVisible(true),
      },
      {
        id: 'new-shell',
        title: 'New Shell',
        subtitle: activeWorkspaceName,
        group: 'Create',
        icon: 'shell',
        shortcut: '⌘T',
        keywords: ['terminal', 'zsh'],
        disabled: !hasActiveWorkspace,
        run: createNewShell,
      },
      {
        id: 'new-codex-chat',
        title: 'Start Codex Chat',
        subtitle: 'Default rich chat with model/reasoning metadata',
        group: 'Agents',
        icon: 'codex',
        keywords: ['chat', 'codex', 'plan', 'assistant'],
        disabled: !hasActiveWorkspace,
        run: launchCodexChatTab,
      },
      {
        id: 'new-claude-chat',
        title: 'Start Claude Chat',
        subtitle: 'Attach Claude Code to the active workspace',
        group: 'Agents',
        icon: 'claude',
        keywords: ['chat', 'claude', 'plan', 'assistant'],
        disabled: !hasActiveWorkspace,
        run: launchClaudeChatTab,
      },
      ...agentTypes.map((agent) => ({
        id: `agent:${agent.name}`,
        title: `Start ${agent.label}`,
        subtitle: activeWorkspaceName,
        group: 'Agents',
        icon: agent.icon || agent.provider || agent.name,
        keywords: ['terminal', 'agent', agent.name, agent.label],
        disabled: !hasActiveWorkspace,
        run: () => launchAgentTab(agent),
      })),
      {
        id: 'new-workspace',
        title: 'New Workspace',
        subtitle: 'Create a worktree and optionally start an agent',
        group: 'Project',
        icon: 'editor',
        shortcut: '⌘N',
        keywords: ['worktree', 'branch'],
        disabled: !project,
        run: openNewWorkspaceFlow,
      },
      {
        id: 'open-project',
        title: 'Open Project',
        subtitle: project?.root || 'Choose a repository',
        group: 'Project',
        icon: 'editor',
        keywords: ['repo', 'switch'],
        run: openProjectDialog,
      },
      {
        id: 'new-window',
        title: 'New Orion Window',
        subtitle: 'Open a separate desktop window',
        group: 'Project',
        icon: 'editor',
        shortcut: '⌘⇧N',
        keywords: ['window'],
        run: () => NewWindow(),
      },
      {
        id: 'start-servers',
        title: 'Start Servers',
        subtitle: activeWorkspaceName,
        group: 'Workspace',
        icon: 'server',
        keywords: ['run', 'ports', 'dev server'],
        disabled: !hasActiveWorkspace,
        run: startServersForActiveWorkspace,
      },
      {
        id: 'stop-servers',
        title: 'Stop Servers',
        subtitle: activeWorkspaceName,
        group: 'Workspace',
        icon: 'server',
        keywords: ['kill', 'ports', 'dev server'],
        disabled: !hasActiveWorkspace,
        run: stopServersForActiveWorkspace,
      },
      {
        id: 'open-browser',
        title: 'Open Workspace Browser',
        subtitle: activeWorkspaceName,
        group: 'Workspace',
        icon: 'server',
        shortcut: '⌘⇧B',
        keywords: ['localhost', 'preview'],
        disabled: !hasActiveWorkspace,
        run: openActiveWorkspaceInBrowser,
      },
      {
        id: 'reveal-workspace',
        title: 'Reveal Workspace in Finder',
        subtitle: activeWorkspacePath || '',
        group: 'Workspace',
        icon: 'editor',
        keywords: ['finder', 'path'],
        disabled: !activeWorkspacePath,
        run: () => { if (activeWorkspacePath) RevealInFinder(activeWorkspacePath); },
      },
      {
        id: 'show-workspaces',
        title: 'Show Workspaces',
        subtitle: 'Open the left workspace dashboard',
        group: 'View',
        icon: 'editor',
        shortcut: '⌘B',
        keywords: ['sidebar'],
        run: () => setSidebarMode('workspaces'),
      },
      {
        id: 'show-files',
        title: 'Show File Explorer',
        subtitle: activeWorkspaceName,
        group: 'View',
        icon: 'editor',
        shortcut: '⌘⇧E',
        keywords: ['files', 'sidebar'],
        disabled: !hasActiveWorkspace,
        run: () => setSidebarMode('files'),
      },
      {
        id: 'search-files',
        title: 'Search Files by Name',
        subtitle: activeWorkspaceName,
        group: 'Search',
        icon: 'editor',
        keywords: ['fuzzy', 'finder'],
        disabled: !hasActiveWorkspace,
        run: () => setSearchEverywhereVisible(true),
      },
      {
        id: 'search-contents',
        title: 'Search Workspace Contents',
        subtitle: activeWorkspaceName,
        group: 'Search',
        icon: 'editor',
        shortcut: '⌘⇧F',
        keywords: ['grep', 'ripgrep'],
        disabled: !hasActiveWorkspace,
        run: () => setSidebarMode('search'),
      },
      {
        id: 'toggle-review',
        title: codeReviewVisible ? 'Hide Code Review' : 'Show Code Review',
        subtitle: 'Diff viewer and review panel',
        group: 'View',
        icon: 'reviewer',
        shortcut: '⌘⇧G',
        keywords: ['diff', 'changes', 'git'],
        run: toggleCodeReview,
      },
      {
        id: 'toggle-server-pane',
        title: serverPaneVisible ? 'Hide Server Pane' : 'Show Server Pane',
        subtitle: `${activeServerTabs.length} server tab${activeServerTabs.length === 1 ? '' : 's'}`,
        group: 'View',
        icon: 'server',
        shortcut: '⌘J',
        keywords: ['bottom panel'],
        disabled: activeServerTabs.length === 0,
        run: () => setServerPaneVisible(!serverPaneVisible),
      },
      {
        id: 'open-diagnostics',
        title: 'Open Diagnostics',
        subtitle: 'Memory and runtime health',
        group: 'View',
        icon: 'diagnostics',
        keywords: ['health', 'debug'],
        run: openDiagnostics,
      },
    ];

    for (const ws of sortWorkspaces(workspaces, workspaceActive)) {
      commands.push({
        id: `workspace:${ws.path}`,
        title: `Switch to ${ws.isMain ? 'main' : ws.branch || ws.name}`,
        subtitle: ws.path,
        group: 'Workspaces',
        icon: 'editor',
        keywords: ['workspace', 'worktree', ws.branch || '', ws.name],
        run: () => {
          setActiveWorkspace(ws.path);
          if (project) AllocatePorts(project.root, ws.path, ws.isMain).catch(() => {});
        },
      });
    }

    for (const tab of tabs) {
      commands.push({
        id: `tab:${tab.id}`,
        title: `Switch to ${tab.label}`,
        subtitle: workspaces.find((ws) => ws.path === tab.workspacePath)?.branch || tab.workspacePath,
        group: 'Tabs',
        icon: tab.icon || tab.provider || tab.tabType,
        keywords: ['tab', tab.tabType, tab.provider || '', tab.threadId || ''],
        run: () => {
          if (tab.workspacePath !== activeWorkspacePath) setActiveWorkspace(tab.workspacePath);
          setActiveTab(tab.id);
        },
      });
    }

    return commands;
  }, [
    activeServerTabs.length,
    activeWorkspace,
    activeWorkspacePath,
    agentTypes,
    codeReviewVisible,
    createNewShell,
    launchAgentTab,
    launchClaudeChatTab,
    launchCodexChatTab,
    openActiveWorkspaceInBrowser,
    openDiagnostics,
    openNewWorkspaceFlow,
    openProjectDialog,
    project,
    serverPaneVisible,
    setActiveTab,
    setActiveWorkspace,
    setServerPaneVisible,
    setSidebarMode,
    startServersForActiveWorkspace,
    stopServersForActiveWorkspace,
    tabs,
    toggleCodeReview,
    workspaceActive,
    workspaces,
  ]);

  return (
    <div className="app">
      <div className="titlebar">
        <div className="titlebar-brand">
          <OrionMark size={24} />
          <span className="titlebar-title">orion</span>
          {activeWorkspace && (
            <span className="titlebar-project">{activeWorkspace.branch || activeWorkspace.name}</span>
          )}
        </div>
      </div>

      <div className="content">
        <ActivityBar onOpenDiagnostics={openDiagnostics} diagnosticsActive={diagnosticsActive} />
        {sidebarMode && (
          <div className="sidebar-container" style={{ width: sidebarWidth }}>
            {sidebarMode === 'workspaces' && <Sidebar onNewSession={() => setNewTabPickerVisible(true)} />}
            {sidebarMode === 'files' && <FileExplorer />}
            {sidebarMode === 'search' && <GlobalSearch />}
            <div
              className={`sidebar-resizer${resizingSidebar ? ' dragging' : ''}`}
              onMouseDown={(e) => {
                e.preventDefault();
                const startX = e.clientX;
                const startW = sidebarWidth;
                setResizingSidebar(true);
                const onMove = (me: MouseEvent) => {
                  const next = Math.max(160, Math.min(800, startW + (me.clientX - startX)));
                  setSidebarWidth(next);
                };
                const onUp = () => {
                  document.removeEventListener('mousemove', onMove);
                  document.removeEventListener('mouseup', onUp);
                  document.body.style.cursor = '';
                  document.body.style.userSelect = '';
                  setResizingSidebar(false);
                };
                document.addEventListener('mousemove', onMove);
                document.addEventListener('mouseup', onUp);
                document.body.style.cursor = 'col-resize';
                document.body.style.userSelect = 'none';
              }}
            />
          </div>
        )}

        <div className="workspace-area">
          <div
            className="terminal-area"
            style={{ width: codeReviewVisible ? `${100 - codeReviewWidth}%` : '100%' }}
          >
          {/* Tab bar */}
          <div className="tab-bar">
            <div className="tab-list">
              {activeTabs.map((tab) => (
                <div
                  key={tab.id}
                  className={`tab ${tab.id === activeTabId ? 'active' : ''} ${dragOverTabId === tab.id ? (dragMerge ? 'tab-drop-target' : 'tab-reorder-target') : ''}`}
                  onClick={() => setActiveTab(tab.id)}
                  onContextMenu={(e) => {
                    if (tab.tabType === 'editor') {
                      e.preventDefault();
                      // Get file path from pane tree
                      const getFilePath = (pane: any): string | null => {
                        if (pane.type === 'editor' && pane.filePath) return pane.filePath;
                        if (pane.children) {
                          for (const c of pane.children) {
                            const r = getFilePath(c);
                            if (r) return r;
                          }
                        }
                        return null;
                      };
                      const fp = getFilePath(tab.rootPane);
                      if (fp) setContextMenu({ x: e.clientX, y: e.clientY, filePath: fp });
                    }
                  }}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData('text/plain', tab.id);
                    e.dataTransfer.effectAllowed = 'move';
                  }}
                  onDragOver={(e) => {
                    e.preventDefault();
                    e.dataTransfer.dropEffect = 'move';
                    setDragOverTabId(tab.id);
                    setDragMerge(e.altKey);
                  }}
                  onDragLeave={() => { setDragOverTabId(null); setDragMerge(false); }}
                  onDrop={(e) => {
                    e.preventDefault();
                    setDragOverTabId(null);
                    setDragMerge(false);
                    const sourceTabId = e.dataTransfer.getData('text/plain');
                    if (sourceTabId && sourceTabId !== tab.id) {
                      if (e.altKey) {
                        mergeTabInto(sourceTabId, tab.id);
                      } else {
                        reorderTab(sourceTabId, tab.id);
                      }
                    }
                  }}
                >
                  <span className="tab-icon"><AgentSigil id={tab.icon || tab.provider || tab.tabType} size={18} /></span>
                  {renamingTabId === tab.id ? (
                    <input
                      autoFocus
                      className="tab-rename-input"
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          if (renameValue.trim()) renameTab(tab.id, renameValue.trim());
                          setRenamingTabId(null);
                        }
                        if (e.key === 'Escape') setRenamingTabId(null);
                        e.stopPropagation();
                      }}
                      onBlur={() => {
                        if (renameValue.trim()) renameTab(tab.id, renameValue.trim());
                        setRenamingTabId(null);
                      }}
                      onClick={(e) => e.stopPropagation()}
                    />
                  ) : (
                    <span
                      onDoubleClick={(e) => {
                        e.stopPropagation();
                        setRenamingTabId(tab.id);
                        setRenameValue(tab.label);
                      }}
                    >
                      {tab.label}
                    </span>
                  )}
                  {(tab.tabType === 'claude' || tab.tabType === 'codex' || tab.tabType === 'claude-chat' || tab.tabType === 'codex-chat') && (
                    <span
                      className="convert"
                      title={tab.tabType === 'claude-chat' || tab.tabType === 'codex-chat' ? 'Convert to terminal' : 'Convert to chat'}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleConvertTab(tab.id);
                      }}
                    >
                      ↔
                    </span>
                  )}
                  <span
                    className="close"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleCloseTab(tab.id);
                    }}
                  >
                    ×
                  </span>
                </div>
              ))}
              <div className="tab-add" onClick={() => setNewTabPickerVisible(true)} title="New session (⌘T)">
                +
              </div>
            </div>
            <WorkspaceToolbar
              serverStatuses={activeWorkspaceStatuses}
              envVars={activeWorkspaceEnv}
              diffStats={diffStats}
              popover={toolbarPopover}
              busy={toolbarBusy === 'servers'}
              sessionStatus={sessionStatus}
              onTogglePopover={(next) => {
                window.dispatchEvent(new Event('orion:close-workspace-inspector'));
                setToolbarPopover((current) => current === next ? null : next);
              }}
              onClosePopover={() => setToolbarPopover(null)}
              onStartServers={startServersForActiveWorkspace}
              onStopServers={stopServersForActiveWorkspace}
              onToggleDiff={() => {
                setToolbarPopover(null);
                toggleCodeReview();
              }}
            />
          </div>

          {/* Main terminal area */}
          <div className="terminal-container" style={{
            height: serverPaneVisible && activeServerTabs.length > 0 ? `${100 - serverPaneHeight}%` : '100%',
          }}>
            {tabs.map((tab) => (
              <div
                key={tab.id}
                style={{
                  display: tab.id === activeTabId ? 'flex' : 'none',
                  width: '100%',
                  height: '100%',
                }}
              >
                <SplitPane pane={tab.rootPane} visible={tab.id === activeTabId} />
              </div>
            ))}
          </div>

          {/* Server bottom pane */}
          {activeServerTabs.length > 0 && (
            <>
              <div
                className="server-pane-divider"
                onMouseDown={(e) => {
                  const startY = e.clientY;
                  const termArea = e.currentTarget.parentElement;
                  if (!termArea) return;
                  const totalHeight = termArea.clientHeight;
                  const startHeight = serverPaneHeight;

                  const onMove = (me: MouseEvent) => {
                    const delta = startY - me.clientY;
                    const deltaPercent = (delta / totalHeight) * 100;
                    setServerPaneHeight(startHeight + deltaPercent);
                  };
                  const onUp = () => {
                    document.removeEventListener('mousemove', onMove);
                    document.removeEventListener('mouseup', onUp);
                    document.body.style.cursor = '';
                    document.body.style.userSelect = '';
                  };
                  document.addEventListener('mousemove', onMove);
                  document.addEventListener('mouseup', onUp);
                  document.body.style.cursor = 'row-resize';
                  document.body.style.userSelect = 'none';
                }}
              />
              <div className="server-pane" style={{
                height: serverPaneVisible ? `${serverPaneHeight}%` : '28px',
              }}>
                <div className="server-pane-header">
                  <div className="tab-bar" style={{ background: 'transparent', borderBottom: 'none' }}>
                    {activeServerTabs.map((tab) => (
                      <div
                        key={tab.id}
                        className={`tab ${tab.id === activeServerTabId ? 'active' : ''}`}
                        onClick={() => {
                          setActiveServerTab(tab.id);
                          if (!serverPaneVisible) setServerPaneVisible(true);
                        }}
                      >
                        <span className="tab-icon"><AgentSigil id="server" size={18} /></span>
                        <span>{tab.label}</span>
                        <span className="close" onClick={(e) => {
                          e.stopPropagation();
                          const termIds = getAllTerminalIds(tab);
                          termIds.forEach((id) => CloseTerminal(id));
                          removeServerTab(tab.id);
                        }}>×</span>
                      </div>
                    ))}
                  </div>
                  <span
                    className="server-pane-toggle"
                    onClick={() => setServerPaneVisible(!serverPaneVisible)}
                  >
                    {serverPaneVisible ? '▾ hide' : '▸ show'}
                  </span>
                </div>
                {serverPaneVisible && (
                  <div className="server-pane-content">
                    {activeServerTabs.map((tab) => (
                      <div
                        key={tab.id}
                        style={{
                          display: tab.id === activeServerTabId ? 'flex' : 'none',
                          width: '100%',
                          height: '100%',
                        }}
                      >
                        <SplitPane pane={tab.rootPane} visible={tab.id === activeServerTabId && serverPaneVisible} />
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          )}
        </div>

        {codeReviewVisible && (
          <>
            <div
              className="code-review-resizer"
              onMouseDown={(e) => {
                e.preventDefault();
                const startX = e.clientX;
                const startW = codeReviewWidth;
                const parent = (e.currentTarget.parentElement as HTMLElement);
                const parentW = parent ? parent.getBoundingClientRect().width : window.innerWidth;
                const onMove = (me: MouseEvent) => {
                  const deltaPct = ((startX - me.clientX) / parentW) * 100;
                  setCodeReviewWidth(startW + deltaPct);
                };
                const onUp = () => {
                  document.removeEventListener('mousemove', onMove);
                  document.removeEventListener('mouseup', onUp);
                  document.body.style.cursor = '';
                  document.body.style.userSelect = '';
                };
                document.addEventListener('mousemove', onMove);
                document.addEventListener('mouseup', onUp);
                document.body.style.cursor = 'col-resize';
                document.body.style.userSelect = 'none';
              }}
            />
            <div className="code-review-container" style={{ width: `${codeReviewWidth}%` }}>
              <CodeReviewPane />
            </div>
          </>
        )}
        </div>
      </div>

      {/* Search Everywhere modal (double-tap Shift) */}
      <SearchEverywhere
        visible={searchEverywhereVisible}
        onClose={() => setSearchEverywhereVisible(false)}
      />

      {/* Command palette (⌘K / ⌘⇧P) */}
      <CommandPalette
        visible={commandPaletteVisible}
        commands={commandPaletteCommands}
        onClose={() => setCommandPaletteVisible(false)}
      />

      {/* New session picker (⌘T) */}
      <NewTabPicker
        visible={newTabPickerVisible}
        onClose={() => setNewTabPickerVisible(false)}
        onPick={handleNewTabPick}
      />

      {/* Terminal-to-chat history picker */}
      {conversionPicker && (
        <ConversionHistoryPicker
          visible={Boolean(conversionPicker)}
          kind={conversionPicker.kind}
          workspacePath={conversionPicker.workspacePath}
          error={conversionPicker.error}
          candidates={conversionPicker.candidates}
          busy={conversionPickerBusy}
          onClose={() => {
            if (!conversionPickerBusy) setConversionPicker(null);
          }}
          onPick={handlePickConversionHistory}
        />
      )}

      {conversionNotice && (
        <div className="conversion-toast" role="status">
          <span>{conversionNotice}</span>
          <button type="button" onClick={() => setConversionNotice(null)} aria-label="Dismiss">×</button>
        </div>
      )}

      {/* Context menu */}
      {contextMenu && (
        <div
          className="context-menu-overlay"
          onClick={() => setContextMenu(null)}
          onContextMenu={(e) => { e.preventDefault(); setContextMenu(null); }}
        >
          <div
            className="context-menu"
            style={{ top: contextMenu.y, left: contextMenu.x }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="context-menu-item" onClick={() => {
              RevealInFinder(contextMenu.filePath);
              setContextMenu(null);
            }}>
              Reveal in Finder
            </div>
            <div className="context-menu-item" onClick={() => {
              navigator.clipboard.writeText(contextMenu.filePath);
              setContextMenu(null);
            }}>
              Copy Full Path
            </div>
            <div className="context-menu-item" onClick={() => {
              const rel = activeWorkspacePath ? contextMenu.filePath.replace(activeWorkspacePath + '/', '') : contextMenu.filePath;
              navigator.clipboard.writeText(rel);
              setContextMenu(null);
            }}>
              Copy Relative Path
            </div>
          </div>
        </div>
      )}

      {/* Status bar */}
      <div className="status-bar">
        <div className="status-left">
          {activeWorkspace && (
            <>
              <span>⎇ {activeWorkspace.branch || activeWorkspace.name}</span>
              <span style={{ color: 'var(--text-muted)' }}>|</span>
              <span>{activeWorkspace.path}</span>
            </>
          )}
        </div>
        <div className="status-right">
          {paneCount > 1 && <span>{paneCount} panes</span>}
          <span>{activeTabs.length} tab{activeTabs.length !== 1 ? 's' : ''}</span>
          <span style={{ color: 'var(--text-muted)' }}>⌘D split  ⌘[] panes  drag to reorder  ⌥drag to merge</span>
        </div>
      </div>
    </div>
  );
}

function WorkspaceToolbar({
  serverStatuses,
  envVars,
  diffStats,
  popover,
  busy,
  sessionStatus,
  onTogglePopover,
  onClosePopover,
  onStartServers,
  onStopServers,
  onToggleDiff,
}: {
  serverStatuses: server.ServerStatus[];
  envVars: Record<string, string>;
  diffStats: DiffStats;
  popover: 'servers' | 'env' | null;
  busy: boolean;
  sessionStatus: string;
  onTogglePopover: (popover: 'servers' | 'env') => void;
  onClosePopover: () => void;
  onStartServers: () => void;
  onStopServers: () => void;
  onToggleDiff: () => void;
}) {
  const runningServers = serverStatuses.filter((status) => status.running);
  const envEntries = Object.entries(envVars);
  const serverAction = runningServers.length > 0 ? onStopServers : onStartServers;
  const browserURL = workspaceBrowserURL(serverStatuses);

  return (
    <div className="workspace-toolbar">
      {serverStatuses.length > 0 && (
        <button type="button" className="workspace-chip" onClick={() => onTogglePopover('servers')}>
          <span className={`chip-dot ${runningServers.length > 0 ? 'running' : ''}`} />
          <span>servers</span>
          <b>{runningServers.length}/{serverStatuses.length}</b>
        </button>
      )}
      {browserURL && (
        <button type="button" className="workspace-icon-button" onClick={() => BrowserOpenURL(browserURL)} title="Open browser">
          <AgentSigil id="browser" size={15} />
        </button>
      )}
      {envEntries.length > 0 && (
        <button type="button" className="workspace-chip" onClick={() => onTogglePopover('env')}>
          <span>env</span>
          <b>{envEntries.length}</b>
        </button>
      )}
      <button type="button" className="workspace-chip diff-chip" onClick={onToggleDiff}>
        <span className="diff-add">+{diffStats.added}</span>
        <span className="diff-del">-{diffStats.removed}</span>
      </button>
      <span className="workspace-ready">{sessionStatus}</span>

      {popover && (
        <>
          <div className="toolbar-popover-dismiss" onMouseDown={onClosePopover} />
          <div className="toolbar-popover">
            {popover === 'servers' ? (
              <>
                <div className="toolbar-popover-header">
                  <span>Servers · {runningServers.length}/{serverStatuses.length}</span>
                  <button type="button" className={runningServers.length > 0 ? 'stop' : 'start'} onClick={serverAction} disabled={busy}>
                    {busy ? '...' : runningServers.length > 0 ? 'stop' : 'start'}
                  </button>
                </div>
                <div className="toolbar-popover-list">
                  {[...serverStatuses].sort(serverStatusSort).map((status) => (
                    <ServerPopoverRow key={status.name} status={status} allStatuses={serverStatuses} onClosePopover={onClosePopover} />
                  ))}
                </div>
              </>
            ) : (
              <>
                <div className="toolbar-popover-header">
                  <span>Env · {envEntries.length}</span>
                </div>
                <div className="toolbar-popover-list">
                  {envEntries.map(([key, value]) => (
                    <button key={key} type="button" className="toolbar-popover-row env-row" onClick={() => navigator.clipboard.writeText(value)}>
                      <span>{key}</span>
                      <code>{maskToolbarValue(value)}</code>
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function ServerPopoverRow({
  status,
  allStatuses,
  onClosePopover,
}: {
  status: server.ServerStatus;
  allStatuses: server.ServerStatus[];
  onClosePopover: () => void;
}) {
  const action = serverBrowserAction(status, allStatuses);

  return (
    <div className="toolbar-popover-row server-row">
      <span className={`server-dot ${status.running ? 'running' : 'stopped'}`}>●</span>
      <span>{status.name}</span>
      {status.port > 0 ? <code>:{status.port}</code> : <code />}
      {action ? (
        <button
          type="button"
          className="server-row-action"
          onClick={() => {
            onClosePopover();
            BrowserOpenURL(action.url);
          }}
          title={action.label}
          aria-label={action.label}
        >
          <AgentSigil id="browser" size={14} />
        </button>
      ) : (
        <span />
      )}
    </div>
  );
}

function serverStatusSort(a: server.ServerStatus, b: server.ServerStatus) {
  const order: Record<string, number> = { frontend: 0, backend: 1, sidekiq: 2 };
  return (order[a.name] ?? 99) - (order[b.name] ?? 99);
}

function workspaceBrowserURL(statuses: server.ServerStatus[]): string | null {
  const status = pickFrontendStatus(statuses) || statuses.find((candidate) => candidate.running && candidate.port > 0);
  return status?.port ? localhostURL(status.port) : null;
}

function serverBrowserAction(status: server.ServerStatus, statuses: server.ServerStatus[]): { label: string; url: string } | null {
  if (!status.running) return null;
  const name = status.name.toLowerCase();
  if (name === 'sidekiq' || name === 'sidekick') {
    const backend = pickBackendStatus(statuses);
    if (!backend?.port) return null;
    return { label: 'Open Sidekiq dashboard', url: `${localhostURL(backend.port)}/sidekiq` };
  }
  if (status.port > 0) {
    const label = name === 'frontend' || name === 'web' ? 'Open browser' : `Open ${status.name}`;
    return { label, url: localhostURL(status.port) };
  }
  return null;
}

function pickFrontendStatus(statuses: server.ServerStatus[]): server.ServerStatus | undefined {
  const preferred = ['frontend', 'web', 'client', 'app'];
  return preferred
    .map((name) => statuses.find((status) => status.running && status.port > 0 && status.name.toLowerCase() === name))
    .find(Boolean);
}

function pickBackendStatus(statuses: server.ServerStatus[]): server.ServerStatus | undefined {
  const preferred = ['backend', 'server', 'api', 'web'];
  return preferred
    .map((name) => statuses.find((status) => status.running && status.port > 0 && status.name.toLowerCase() === name))
    .find(Boolean) || statuses.find((status) => status.running && status.port > 0 && status.name.toLowerCase() !== 'frontend');
}

function localhostURL(port: number): string {
  return `http://localhost:${port}`;
}

function maskToolbarValue(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length <= 14) return trimmed;
  return `${trimmed.slice(0, 7)}...${trimmed.slice(-4)}`;
}

function parseChatMetadata(msg: any): { threadId?: string; model?: string; reasoningEffort?: string; approvalPolicy?: string; sandboxMode?: string; permissionMode?: string; collaborationMode?: string } {
  if (!msg || msg.type !== 'system' || !msg.details) {
    return {};
  }
  try {
    return JSON.parse(msg.details);
  } catch {
    return {};
  }
}

function chatPaneHasSession(pane: Pane, sessionId: string): boolean {
  if (pane.type === 'chat') {
    return pane.chatSessionId === sessionId;
  }
  if (!('children' in pane)) {
    return false;
  }
  return pane.children.some((child) => chatPaneHasSession(child, sessionId));
}

function updateChatPaneThreadId(pane: Pane, sessionId: string, threadId?: string): Pane {
  if (!threadId) {
    return pane;
  }
  if (pane.type === 'chat') {
    if (pane.chatSessionId !== sessionId) {
      return pane;
    }
    return { ...pane, chatThreadId: threadId };
  }
  if (!('children' in pane)) {
    return pane;
  }
  return {
    ...pane,
    children: pane.children.map((child) => updateChatPaneThreadId(child, sessionId, threadId)),
  };
}

function normalizeHistoryCandidate(thread: claudesdk.HistoryThread | codexchat.HistoryThread): ConversionHistoryCandidate {
  return {
    threadId: thread.threadId,
    updatedAt: thread.updatedAt,
    messageCount: thread.messageCount,
    preview: thread.preview,
    model: thread.model,
  };
}

function chatTabLabel(kind: AgentKind, label?: string) {
  const fallback = kind === 'claude' ? 'Claude Chat' : 'Codex Chat';
  const value = (label || fallback).trim() || fallback;
  return value.toLowerCase().endsWith(' chat') ? value : `${value} Chat`;
}

function errorMessage(err: unknown) {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  if (err && typeof err === 'object' && 'message' in err) {
    return String((err as { message?: unknown }).message || '');
  }
  return String(err || '');
}

export default App;
