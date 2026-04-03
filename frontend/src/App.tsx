import { useEffect, useCallback, useState, DragEvent } from 'react';
import './App.css';
import SplitPane from './components/SplitPane';
import Sidebar from './components/Sidebar';
import { useStore, generateId, Tab, PaneLeaf } from './store';
import {
  CreateTerminalInDir,
  CloseTerminal,
  SaveTabs,
  GetTmuxSession,
} from '../wailsjs/go/main/App';

function App() {
  const {
    workspaces,
    activeWorkspacePath,
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
  } = useStore();

  const activeTabs = tabs.filter((t) => t.workspacePath === activeWorkspacePath);
  const activeTab = tabs.find((t) => t.id === activeTabId);
  const [dragOverTabId, setDragOverTabId] = useState<string | null>(null);
  const [renamingTabId, setRenamingTabId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');

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
      if (e.metaKey && e.shiftKey && e.key === 'D') {
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
      // Cmd+1-9: switch tabs
      if (e.metaKey && !e.shiftKey && e.key >= '1' && e.key <= '9') {
        e.preventDefault();
        const idx = parseInt(e.key) - 1;
        if (idx < activeTabs.length) {
          setActiveTab(activeTabs[idx].id);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTabs, activeTabId, createNewShell, handleClosePane, handleSplit, navigatePane, setActiveTab]);

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
        <Sidebar />

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
                   tab.tabType === 'server' ? '▸' : '›'}
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

          {/* Pane area — render active tab's pane tree */}
          <div className="terminal-container">
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
        </div>
      </div>

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
