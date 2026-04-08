import { useEffect, useCallback, useState, DragEvent } from 'react';
import './App.css';
import SplitPane from './components/SplitPane';
import Sidebar from './components/Sidebar';
import ActivityBar from './components/ActivityBar';
import FileExplorer from './components/FileExplorer';
import GitPanel from './components/GitPanel';
import GlobalSearch from './components/GlobalSearch';
import SearchEverywhere from './components/SearchEverywhere';
import { useStore, generateId, Tab, PaneLeaf } from './store';
import { configureMonacoTheme } from './lib/monacoTheme';
import { EventsOn } from '../wailsjs/runtime/runtime';
import {
  CreateTerminalInDir,
  CreateAttachedTerminal,
  CloseTerminal,
  SaveTabs,
  GetLastProject,
  GetProjectInfo,
  SetActiveProject,
  ListWorkspaces,
  NewWindow,
  GetAgentTypes,
  GetSavedTabs,
  GetTmuxSession,
} from '../wailsjs/go/main/App';

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
    renameTab,
    focusedPaneId,
    getAllTerminalIds,
    serverTabs,
    activeServerTabId,
    serverPaneVisible,
    serverPaneHeight,
    setActiveServerTab,
    removeServerTab,
    setServerPaneVisible,
    setServerPaneHeight,
    sidebarMode,
    setSidebarMode,
  } = useStore();

  // Initialize Monaco theme once
  useEffect(() => { configureMonacoTheme(); }, []);

  // Menu events are registered after callbacks are defined (see below)

  // Initialize app on mount — load last project and restore saved tabs
  // Lives here (not in Sidebar) because App never unmounts, preventing duplicate tabs
  useEffect(() => {
    const init = async () => {
      try {
        const lastRoot = await GetLastProject();
        if (!lastRoot) return;

        const info = await GetProjectInfo(lastRoot);
        await SetActiveProject(info.root); // load per-project state
        setProject({ name: info.name, root: info.root, mainBranch: info.mainBranch });
        const ws = await ListWorkspaces(info.root);
        setWorkspaces(ws);

        const mainWs = ws.find((w: any) => w.isMain);
        if (mainWs) {
          setActiveWorkspace(mainWs.path);
        }

        const savedTabs = await GetSavedTabs();
        if (savedTabs && savedTabs.length > 0) {
          for (const saved of savedTabs) {
            const termId = generateId('term');
            try {
              await CreateAttachedTerminal(termId, saved.tmuxSession);
              addTab({
                id: generateId('tab'),
                label: saved.label,
                rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
                tabType: saved.tabType as 'shell' | 'claude' | 'codex' | 'server',
                workspacePath: saved.workspacePath,
              });
            } catch {}
          }
        } else if (mainWs) {
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
    };
    init();
  }, []);

  const activeTabs = tabs.filter((t) => t.workspacePath === activeWorkspacePath);
  const activeServerTabs = serverTabs.filter((t) => t.workspacePath === activeWorkspacePath);
  const activeTab = tabs.find((t) => t.id === activeTabId);
  const [dragOverTabId, setDragOverTabId] = useState<string | null>(null);
  const [renamingTabId, setRenamingTabId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [searchEverywhereVisible, setSearchEverywhereVisible] = useState(false);

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

  const handleCloseTab = useCallback(async (tabId: string) => {
    const tab = tabs.find((t) => t.id === tabId);
    if (!tab) return;
    const termIds = getAllTerminalIds(tab);
    for (const termId of termIds) {
      try {
        await CloseTerminal(termId);
      } catch {}
    }
    removeTab(tabId);
  }, [tabs, removeTab, getAllTerminalIds]);

  // Persist tabs to disk whenever they change (for recovery on restart)
  useEffect(() => {
    if (tabs.length === 0) return;
    (async () => {
      const savedTabs = [];
      for (const tab of tabs) {
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
            });
          }
        }
      }
      if (savedTabs.length > 0) {
        await SaveTabs(savedTabs);
      }
    })();
  }, [tabs]);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd+T: new shell tab
      if (e.metaKey && !e.shiftKey && e.key === 't') {
        e.preventDefault();
        createNewShell();
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
      // Cmd+Up/Down: cycle through workspaces
      if (e.metaKey && !e.shiftKey && e.key === 'ArrowUp') {
        e.preventDefault();
        const currentIdx = workspaces.findIndex((w) => w.path === activeWorkspacePath);
        const prevIdx = (currentIdx - 1 + workspaces.length) % workspaces.length;
        if (workspaces[prevIdx]) setActiveWorkspace(workspaces[prevIdx].path);
      }
      if (e.metaKey && !e.shiftKey && e.key === 'ArrowDown') {
        e.preventDefault();
        const currentIdx = workspaces.findIndex((w) => w.path === activeWorkspacePath);
        const nextIdx = (currentIdx + 1) % workspaces.length;
        if (workspaces[nextIdx]) setActiveWorkspace(workspaces[nextIdx].path);
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
      // Cmd+Shift+F: global search
      if (e.metaKey && e.shiftKey && e.key === 'F') {
        e.preventDefault();
        setSidebarMode(sidebarMode === 'search' ? null : 'search');
      }
      // Cmd+Shift+E: file explorer
      if (e.metaKey && e.shiftKey && e.key === 'E') {
        e.preventDefault();
        setSidebarMode(sidebarMode === 'files' ? null : 'files');
      }
      // Cmd+Shift+G: git panel
      if (e.metaKey && e.shiftKey && e.key === 'G') {
        e.preventDefault();
        setSidebarMode(sidebarMode === 'git' ? null : 'git');
      }
      // Cmd+B: toggle sidebar
      if (e.metaKey && !e.shiftKey && e.key === 'b') {
        e.preventDefault();
        setSidebarMode(sidebarMode ? null : 'workspaces');
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTabs, activeTabId, createNewShell, handleClosePane, handleSplit, navigatePane, setActiveTab]);

  // Listen for native menu bar events from Go
  useEffect(() => {
    const cancels = [
      EventsOn('menu:open-project', async () => {
        try {
          const { OpenProjectDialog } = await import('../wailsjs/go/main/App');
          const info = await OpenProjectDialog();
          if (info) {
            await SetActiveProject(info.root);
            setProject({ name: info.name, root: info.root, mainBranch: info.mainBranch });
            const ws = await ListWorkspaces(info.root);
            setWorkspaces(ws);
          }
        } catch {}
      }),
      EventsOn('menu:new-terminal', () => createNewShell()),
      EventsOn('menu:close-tab', () => handleClosePane()),
      EventsOn('menu:toggle-sidebar', () => setSidebarMode(sidebarMode ? null : 'workspaces')),
      EventsOn('menu:show-files', () => setSidebarMode('files')),
      EventsOn('menu:show-search', () => setSidebarMode('search')),
      EventsOn('menu:show-git', () => setSidebarMode('git')),
      EventsOn('menu:show-workspaces', () => setSidebarMode('workspaces')),
      EventsOn('menu:split-right', () => handleSplit('vertical')),
      EventsOn('menu:split-down', () => handleSplit('horizontal')),
      EventsOn('menu:next-pane', () => navigatePane('next')),
      EventsOn('menu:prev-pane', () => navigatePane('prev')),
    ];
    return () => cancels.forEach((c) => c());
  }, [sidebarMode, createNewShell, handleClosePane, handleSplit, navigatePane, setSidebarMode]);

  const activeWorkspace = workspaces.find((w) => w.path === activeWorkspacePath);

  // Count total panes in active tab
  const countPanes = (tab: Tab | undefined): number => {
    if (!tab) return 0;
    return getAllTerminalIds(tab).length;
  };
  const paneCount = countPanes(activeTab);

  return (
    <div className="app">
      <div className="titlebar">
        <span className="titlebar-title">orion</span>
      </div>

      <div className="content">
        <ActivityBar />
        {sidebarMode && (
          <div className="sidebar-container">
            {sidebarMode === 'workspaces' && <Sidebar />}
            {sidebarMode === 'files' && <FileExplorer />}
            {sidebarMode === 'git' && <GitPanel />}
            {sidebarMode === 'search' && <GlobalSearch />}
          </div>
        )}

        <div className="terminal-area">
          {/* Tab bar */}
          <div className="tab-bar">
            {activeTabs.map((tab) => (
              <div
                key={tab.id}
                className={`tab ${tab.id === activeTabId ? 'active' : ''} ${dragOverTabId === tab.id ? 'tab-drop-target' : ''}`}
                onClick={() => setActiveTab(tab.id)}
                draggable
                onDragStart={(e) => {
                  e.dataTransfer.setData('text/plain', tab.id);
                  e.dataTransfer.effectAllowed = 'move';
                }}
                onDragOver={(e) => {
                  e.preventDefault();
                  e.dataTransfer.dropEffect = 'move';
                  setDragOverTabId(tab.id);
                }}
                onDragLeave={() => setDragOverTabId(null)}
                onDrop={(e) => {
                  e.preventDefault();
                  setDragOverTabId(null);
                  const sourceTabId = e.dataTransfer.getData('text/plain');
                  if (sourceTabId && sourceTabId !== tab.id) {
                    mergeTabInto(sourceTabId, tab.id);
                  }
                }}
              >
                <span className="tab-icon">
                  {tab.tabType === 'claude' ? '◆' :
                   tab.tabType === 'codex' ? '◇' :
                   tab.tabType === 'server' ? '▸' :
                   tab.tabType === 'editor' ? '◈' :
                   tab.tabType === 'diff' ? '⟷' : '›'}
                </span>
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
            <div className="tab-add" onClick={createNewShell} title="New shell (⌘T)">
              +
            </div>
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
                        <span className="tab-icon">▸</span>
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
                    {serverTabs.map((tab) => (
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
      </div>

      {/* Search Everywhere modal (double-tap Shift) */}
      <SearchEverywhere
        visible={searchEverywhereVisible}
        onClose={() => setSearchEverywhereVisible(false)}
      />


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
          <span style={{ color: 'var(--text-muted)' }}>⌘D split  ⌘[] panes  drag tabs to merge</span>
        </div>
      </div>
    </div>
  );
}

export default App;
