import { useEffect, useCallback } from 'react';
import './App.css';
import Terminal from './components/Terminal';
import { useStore, generateTabId, generateTerminalId } from './store';
import { CreateTerminal, CloseTerminal } from '../wailsjs/go/main/App';

function App() {
  const { tabs, activeTabId, addTab, removeTab, setActiveTab } = useStore();

  const createNewTab = useCallback(async () => {
    const tabId = generateTabId();
    const terminalId = generateTerminalId();
    const shellNum = tabs.filter(t => t.type === 'shell').length + 1;

    try {
      await CreateTerminal(terminalId);
      addTab({
        id: tabId,
        label: `Shell ${shellNum}`,
        terminalId,
        type: 'shell',
      });
    } catch (err) {
      console.error('Failed to create terminal:', err);
    }
  }, [tabs, addTab]);

  const closeTab = useCallback(async (tabId: string) => {
    const tab = tabs.find(t => t.id === tabId);
    if (tab) {
      try {
        await CloseTerminal(tab.terminalId);
      } catch (err) {
        console.error('Failed to close terminal:', err);
      }
    }
    removeTab(tabId);
  }, [tabs, removeTab]);

  // Create first terminal on mount
  useEffect(() => {
    if (tabs.length === 0) {
      createNewTab();
    }
  }, []);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd+T: new tab
      if (e.metaKey && e.key === 't') {
        e.preventDefault();
        createNewTab();
      }
      // Cmd+W: close tab
      if (e.metaKey && e.key === 'w') {
        e.preventDefault();
        if (activeTabId) {
          closeTab(activeTabId);
        }
      }
      // Cmd+1-9: switch tabs
      if (e.metaKey && e.key >= '1' && e.key <= '9') {
        e.preventDefault();
        const idx = parseInt(e.key) - 1;
        if (idx < tabs.length) {
          setActiveTab(tabs[idx].id);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [tabs, activeTabId, createNewTab, closeTab, setActiveTab]);

  return (
    <div className="app">
      {/* Title bar with drag region */}
      <div className="titlebar">
        <span className="titlebar-title">orion</span>
      </div>

      <div className="content">
        {/* Sidebar - placeholder for Phase 2 */}
        <div className="sidebar">
          <div className="sidebar-section">
            <div className="sidebar-label">Workspaces</div>
            <div className="sidebar-item active">
              <span className="icon">◉</span>
              <span className="label">main</span>
            </div>
          </div>
        </div>

        {/* Terminal area */}
        <div className="terminal-area">
          {/* Tab bar */}
          <div className="tab-bar">
            {tabs.map((tab) => (
              <div
                key={tab.id}
                className={`tab ${tab.id === activeTabId ? 'active' : ''}`}
                onClick={() => setActiveTab(tab.id)}
              >
                <span>{tab.label}</span>
                <span
                  className="close"
                  onClick={(e) => {
                    e.stopPropagation();
                    closeTab(tab.id);
                  }}
                >
                  ×
                </span>
              </div>
            ))}
            <div className="tab-add" onClick={createNewTab}>
              +
            </div>
          </div>

          {/* Terminal instances */}
          <div className="terminal-container">
            {tabs.map((tab) => (
              <Terminal
                key={tab.terminalId}
                terminalId={tab.terminalId}
                visible={tab.id === activeTabId}
              />
            ))}
          </div>
        </div>
      </div>

      {/* Status bar */}
      <div className="status-bar">
        <div className="status-left">
          <span>orion v0.1.0</span>
        </div>
        <div className="status-right">
          <span>{tabs.length} tab{tabs.length !== 1 ? 's' : ''}</span>
        </div>
      </div>
    </div>
  );
}

export default App;
