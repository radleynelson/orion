import { useCallback, useEffect, useMemo, useState } from 'react';
import { useStore, Tab } from '../store';
import { server, workspace } from '../../wailsjs/go/models';
import AgentSigil from './AgentSigil';

interface WorkspaceDetailPanelProps {
  workspace: workspace.Workspace;
  serverStatuses: server.ServerStatus[];
  envVars: Record<string, string>;
  onStartServers: (workspacePath: string, isMain: boolean) => Promise<unknown>;
  onStopServers: (workspacePath: string) => Promise<unknown>;
  onNewSession: () => void;
}

export default function WorkspaceDetailPanel({
  workspace,
  serverStatuses,
  envVars,
  onStartServers,
  onStopServers,
  onNewSession,
}: WorkspaceDetailPanelProps) {
  const { tabs, activeTabId, setActiveWorkspace, setActiveTab } = useStore();
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [envVisible, setEnvVisible] = useState(false);

  const liveTabs = useMemo(() => {
    return tabs.filter((tab) =>
      tab.workspacePath === workspace.path &&
      tab.tabType !== 'server' &&
      tab.tabType !== 'diagnostics' &&
      tab.tabType !== 'editor',
    );
  }, [tabs, workspace.path]);

  const runningServers = useMemo(() => serverStatuses.filter((status) => status.running), [serverStatuses]);
  const envEntries = useMemo(() => Object.entries(envVars), [envVars]);

  useEffect(() => {
    setEnvVisible(false);
  }, [workspace.path]);

  const openTab = useCallback((tab: Tab) => {
    setActiveWorkspace(tab.workspacePath);
    setActiveTab(tab.id);
  }, [setActiveTab, setActiveWorkspace]);

  const runBusy = useCallback(async (key: string, action: () => Promise<unknown>) => {
    setBusyAction(key);
    try {
      await action();
    } catch (err) {
      console.error(`Workspace action failed: ${key}`, err);
    } finally {
      setBusyAction(null);
    }
  }, []);

  const serverActionRunning = busyAction === 'servers';

  return (
    <div className="workspace-detail workspace-detail-minimal">
      <section className="workspace-min-section">
        <div className="workspace-min-label">
          <span>Sessions</span>
          <button type="button" onClick={onNewSession} title="New session (Cmd+T)">⌘T</button>
        </div>
        <div className="workspace-min-list">
          {liveTabs.map((tab) => (
            <SessionRow key={tab.id} tab={tab} active={tab.id === activeTabId} onOpen={() => openTab(tab)} />
          ))}
          {liveTabs.length === 0 && (
            <div className="workspace-min-row workspace-min-empty">
              <span className="workspace-min-session-icon">·</span>
              <span>No active sessions</span>
            </div>
          )}
          <button type="button" className="workspace-min-row workspace-min-new" onClick={onNewSession}>
            <span className="workspace-min-plus">+</span>
            <span>new session</span>
          </button>
        </div>
      </section>

      {serverStatuses.length > 0 && (
        <section className="workspace-min-section">
          <div className="workspace-min-label">
            <span>Servers</span>
            <button
              type="button"
              className={runningServers.length > 0 ? 'stop' : 'start'}
              onClick={() => runBusy('servers', () => runningServers.length > 0 ? onStopServers(workspace.path) : onStartServers(workspace.path, workspace.isMain))}
              disabled={serverActionRunning}
            >
              {serverActionRunning ? '...' : runningServers.length > 0 ? 'stop' : 'start'}
            </button>
          </div>
          <div className="workspace-min-list">
            {[...serverStatuses].sort(serverSort).map((status) => (
              <div key={status.name} className="workspace-min-row workspace-min-server">
                <span className={`server-dot ${status.running ? 'running' : 'stopped'}`}>●</span>
                <span>{status.name}</span>
                {status.port > 0 && <code>:{status.port}</code>}
              </div>
            ))}
          </div>
        </section>
      )}

      {envEntries.length > 0 && (
        <section className="workspace-min-section">
          <button type="button" className="workspace-min-label workspace-min-env-toggle" onClick={() => setEnvVisible((visible) => !visible)}>
            <span>{envVisible ? '▾' : '▸'} Env · {envEntries.length}</span>
            <b>+</b>
          </button>
          {envVisible && (
            <div className="workspace-min-list">
              {envEntries.map(([key, value]) => (
                <button key={key} type="button" className="workspace-min-row workspace-min-env" onClick={() => navigator.clipboard.writeText(value)} title="Click to copy">
                  <span>{key}</span>
                  <code>{maskValue(value)}</code>
                </button>
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}

function SessionRow({ tab, active, onOpen }: { tab: Tab; active: boolean; onOpen: () => void }) {
  return (
    <button type="button" className={`workspace-min-row workspace-min-session ${active ? 'active' : ''}`} onClick={onOpen}>
      <AgentSigil id={tab.icon || tab.provider || tab.tabType} size={17} />
      <span>{sessionTitle(tab)}</span>
      <em>{sessionKind(tab)}</em>
    </button>
  );
}

function sessionTitle(tab: Tab): string {
  if (tab.tabType === 'shell') return tab.label?.replace(/^Shell \d+$/, 'zsh') || 'zsh';
  if (tab.tabType === 'claude' || tab.tabType === 'claude-chat') return tab.label || 'Claude';
  if (tab.tabType === 'codex' || tab.tabType === 'codex-chat') return tab.label || 'Codex';
  return tab.label;
}

function sessionKind(tab: Tab): string {
  if (tab.tabType === 'claude-chat' || tab.tabType === 'codex-chat') return 'chat';
  return 'tmux';
}

function serverSort(a: server.ServerStatus, b: server.ServerStatus) {
  const order: Record<string, number> = { frontend: 0, backend: 1, sidekiq: 2 };
  return (order[a.name] ?? 99) - (order[b.name] ?? 99);
}

function maskValue(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length <= 14) return trimmed;
  return `${trimmed.slice(0, 7)}...${trimmed.slice(-4)}`;
}
