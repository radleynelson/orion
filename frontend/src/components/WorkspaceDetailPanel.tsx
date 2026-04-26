import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useStore, generateId, PaneLeaf, Tab } from '../store';
import { claudesdk, codexchat, git, main, server, workspace } from '../../wailsjs/go/models';
import {
  GetChangedFilesAgainst,
  ListClaudeChatHistory,
  ListCodexChatHistory,
  ResumeClaudeChat,
  ResumeCodexChatWithOptions,
} from '../../wailsjs/go/main/App';
import AgentSigil from './AgentSigil';
import OrionMark from './OrionMark';

type ProjectInfo = {
  name: string;
  root: string;
  mainBranch: string;
};

type CodexLaunchOptions = {
  model: string;
  reasoningEffort: string;
  approvalPolicy: string;
  sandboxMode: string;
  collaborationMode: string;
};

type HistoryProvider = 'codex' | 'claude';

type HistoryItem = {
  provider: HistoryProvider;
  threadId: string;
  workspacePath: string;
  model?: string;
  updatedAt: string;
  messageCount: number;
  preview?: string;
};

interface WorkspaceDetailPanelProps {
  project: ProjectInfo;
  workspace: workspace.Workspace;
  serverStatuses: server.ServerStatus[];
  envVars: Record<string, string>;
  agentTypes: main.AgentTypeInfo[];
  onLaunchAgent: (workspacePath: string, agentName: string) => Promise<unknown>;
  onLaunchCodexOptions: (workspacePath: string) => void;
  onLaunchClaudeChat: (workspacePath: string) => Promise<unknown>;
  onLaunchShell: (workspacePath: string) => Promise<unknown>;
  onStartServers: (workspacePath: string, isMain: boolean) => Promise<unknown>;
  onStopServers: (workspacePath: string) => Promise<unknown>;
  onOpenBrowser: (workspacePath: string) => Promise<unknown>;
}

const DEFAULT_CODEX_OPTIONS: CodexLaunchOptions = {
  model: 'gpt-5.4',
  reasoningEffort: 'xhigh',
  approvalPolicy: 'never',
  sandboxMode: 'danger-full-access',
  collaborationMode: 'default',
};

export default function WorkspaceDetailPanel({
  project,
  workspace,
  serverStatuses,
  envVars,
  agentTypes,
  onLaunchAgent,
  onLaunchCodexOptions,
  onLaunchClaudeChat,
  onLaunchShell,
  onStartServers,
  onStopServers,
  onOpenBrowser,
}: WorkspaceDetailPanelProps) {
  const {
    tabs,
    activeTabId,
    setActiveWorkspace,
    setActiveTab,
    addTab,
    serverTabs,
    setCodeReviewBase,
    setCodeReviewVisible,
  } = useStore();

  const [codexHistory, setCodexHistory] = useState<codexchat.HistoryThread[]>([]);
  const [claudeHistory, setClaudeHistory] = useState<claudesdk.HistoryThread[]>([]);
  const [changedFiles, setChangedFiles] = useState<git.ChangedFile[]>([]);
  const [loading, setLoading] = useState(false);
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
  const activeServerTabs = useMemo(() => serverTabs.filter((tab) => tab.workspacePath === workspace.path), [serverTabs, workspace.path]);

  const historyItems = useMemo<HistoryItem[]>(() => {
    const codexItems = codexHistory.map((thread) => ({
      provider: 'codex' as const,
      threadId: thread.threadId,
      workspacePath: thread.workspacePath || workspace.path,
      model: thread.model,
      updatedAt: thread.updatedAt,
      messageCount: thread.messageCount,
      preview: thread.preview,
    }));
    const claudeItems = claudeHistory.map((thread) => ({
      provider: 'claude' as const,
      threadId: thread.threadId,
      workspacePath: thread.workspacePath || workspace.path,
      model: thread.model,
      updatedAt: thread.updatedAt,
      messageCount: thread.messageCount,
      preview: thread.preview,
    }));
    return [...codexItems, ...claudeItems]
      .filter((thread) => thread.threadId)
      .sort((a, b) => Date.parse(b.updatedAt || '') - Date.parse(a.updatedAt || ''))
      .slice(0, 8);
  }, [claudeHistory, codexHistory, workspace.path]);

  const refreshDetail = useCallback(async () => {
    setLoading(true);
    try {
      const [codexThreads, claudeThreads, files] = await Promise.all([
        ListCodexChatHistory(workspace.path, 12).catch(() => [] as codexchat.HistoryThread[]),
        ListClaudeChatHistory(workspace.path, 12).catch(() => [] as claudesdk.HistoryThread[]),
        GetChangedFilesAgainst(workspace.path, '').catch(() => [] as git.ChangedFile[]),
      ]);
      setCodexHistory(codexThreads || []);
      setClaudeHistory(claudeThreads || []);
      setChangedFiles(files || []);
    } finally {
      setLoading(false);
    }
  }, [workspace.path]);

  useEffect(() => {
    refreshDetail();
  }, [refreshDetail]);

  const openTab = useCallback((tab: Tab) => {
    setActiveWorkspace(tab.workspacePath);
    setActiveTab(tab.id);
  }, [setActiveTab, setActiveWorkspace]);

  const openLatest = useCallback(() => {
    if (liveTabs.length > 0) {
      openTab(liveTabs[0]);
      return;
    }
    onLaunchCodexOptions(workspace.path);
  }, [liveTabs, onLaunchCodexOptions, openTab, workspace.path]);

  const openDiff = useCallback(() => {
    setActiveWorkspace(workspace.path);
    setCodeReviewBase('uncommitted');
    setCodeReviewVisible(true);
  }, [setActiveWorkspace, setCodeReviewBase, setCodeReviewVisible, workspace.path]);

  const runBusy = useCallback(async (key: string, action: () => Promise<unknown>) => {
    setBusyAction(key);
    try {
      await action();
      await refreshDetail();
    } catch (err) {
      console.error(`Workspace detail action failed: ${key}`, err);
    } finally {
      setBusyAction(null);
    }
  }, [refreshDetail]);

  const liveTabForHistory = useCallback((item: HistoryItem) => {
    return tabs.find((tab) =>
      tab.workspacePath === workspace.path &&
      tab.provider === item.provider &&
      tab.threadId &&
      tab.threadId === item.threadId,
    );
  }, [tabs, workspace.path]);

  const openHistory = useCallback(async (item: HistoryItem) => {
    const live = liveTabForHistory(item);
    if (live) {
      openTab(live);
      return;
    }
    setBusyAction(`${item.provider}:${item.threadId}`);
    try {
      if (item.provider === 'codex') {
        const session = await ResumeCodexChatWithOptions(
          project.root,
          workspace.path,
          item.threadId,
          item.model || DEFAULT_CODEX_OPTIONS.model,
          DEFAULT_CODEX_OPTIONS.reasoningEffort,
          DEFAULT_CODEX_OPTIONS.approvalPolicy,
          DEFAULT_CODEX_OPTIONS.sandboxMode,
          DEFAULT_CODEX_OPTIONS.collaborationMode,
        );
        addTab({
          id: generateId('tab'),
          label: session?.label || 'Codex Chat',
          rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'codex' } as PaneLeaf,
          tabType: 'codex-chat',
          workspacePath: workspace.path,
          provider: 'codex',
          viewMode: 'chat',
          runtimeSessionId: session.id,
          threadId: session.threadId || item.threadId,
          model: session.model || item.model,
          reasoningEffort: session.reasoningEffort,
          approvalPolicy: session.approvalPolicy,
          sandboxMode: session.sandboxMode,
          collaborationMode: session.collaborationMode,
        });
      } else {
        const session = await ResumeClaudeChat(project.root, workspace.path, item.threadId);
        addTab({
          id: generateId('tab'),
          label: session?.label || 'Claude Chat',
          rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId, chatKind: 'claude' } as PaneLeaf,
          tabType: 'claude-chat',
          workspacePath: workspace.path,
          provider: 'claude',
          viewMode: 'chat',
          runtimeSessionId: session.id,
          threadId: session.threadId || item.threadId,
          model: session.model || item.model,
          reasoningEffort: session.reasoningEffort,
          approvalPolicy: session.approvalPolicy,
          sandboxMode: session.sandboxMode,
          permissionMode: session.permissionMode,
        });
      }
      await refreshDetail();
    } catch (err) {
      console.error('Failed to open history thread:', err);
    } finally {
      setBusyAction(null);
    }
  }, [addTab, liveTabForHistory, openTab, project.root, refreshDetail, workspace.path]);

  const title = workspace.isMain ? 'main' : compactWorkspaceName(workspace.name, project.name);
  const subtitle = workspace.branch || workspace.name;
  const serverActionRunning = busyAction === 'servers';

  return (
    <div className="workspace-detail">
      <div className="workspace-detail-header">
        <OrionMark size={30} />
        <div className="workspace-detail-title">
          <div>
            <span>{title}</span>
            {workspace.isMain && <b>MAIN</b>}
          </div>
          <small title={workspace.path}>{subtitle}</small>
        </div>
        <button type="button" onClick={refreshDetail} title="Refresh detail" disabled={loading}>
          {loading ? '⟳' : '↻'}
        </button>
      </div>

      <div className="workspace-detail-grid">
        <SummaryTile value={String(liveTabs.length)} label="sessions" tone={liveTabs.length > 0 ? 'blue' : 'muted'} />
        <SummaryTile value={String(runningServers.length)} label="servers" tone={runningServers.length > 0 ? 'green' : 'muted'} />
        <SummaryTile value={String(changedFiles.length)} label="files" tone={changedFiles.length > 0 ? 'yellow' : 'muted'} />
      </div>

      <div className="workspace-detail-actions">
        <button type="button" onClick={openLatest}>
          <span>{liveTabs.length > 0 ? '↗' : '+'}</span>
          {liveTabs.length > 0 ? 'Open latest' : 'Start Codex'}
        </button>
        <button type="button" onClick={openDiff}>
          <span>△</span>
          Diff
        </button>
        <button type="button" onClick={() => onLaunchCodexOptions(workspace.path)}>
          <AgentSigil id="codex" size={14} />
          Codex Chat
        </button>
        <button type="button" onClick={() => runBusy('claude', () => onLaunchClaudeChat(workspace.path))} disabled={busyAction === 'claude'}>
          <AgentSigil id="claude" size={14} />
          Claude Chat
        </button>
        <button type="button" onClick={() => runBusy('shell', () => onLaunchShell(workspace.path))} disabled={busyAction === 'shell'}>
          <AgentSigil id="shell" size={14} />
          Shell
        </button>
        {serverStatuses.length > 0 && (
          <button
            type="button"
            className={runningServers.length > 0 ? 'danger' : 'success'}
            onClick={() => runBusy('servers', () => runningServers.length > 0 ? onStopServers(workspace.path) : onStartServers(workspace.path, workspace.isMain))}
            disabled={serverActionRunning}
          >
            <span>{runningServers.length > 0 ? '■' : '▶'}</span>
            {serverActionRunning ? 'Working' : runningServers.length > 0 ? 'Stop servers' : 'Start servers'}
          </button>
        )}
        {(runningServers.length > 0 || activeServerTabs.length > 0) && (
          <button type="button" onClick={() => runBusy('browser', () => onOpenBrowser(workspace.path))} disabled={busyAction === 'browser'}>
            <span>◎</span>
            Browser
          </button>
        )}
        {agentTypes.map((agent) => (
          <button key={agent.name} type="button" onClick={() => runBusy(agent.name, () => onLaunchAgent(workspace.path, agent.name))} disabled={busyAction === agent.name}>
            <AgentSigil id={agent.name} size={14} />
            {agent.label}
          </button>
        ))}
      </div>

      {serverStatuses.length > 0 && (
        <div className="workspace-detail-servers">
          {[...serverStatuses].sort(serverSort).map((status) => (
            <div key={status.name}>
              <span className={`server-dot ${status.running ? 'running' : 'stopped'}`}>●</span>
              <span>{status.name}</span>
              {status.port > 0 && <code>:{status.port}</code>}
            </div>
          ))}
        </div>
      )}

      {Object.keys(envVars).length > 0 && (
        <div className="workspace-detail-env">
          <button type="button" onClick={() => setEnvVisible((visible) => !visible)}>
            {envVisible ? '▾' : '▸'} Env
          </button>
          {envVisible && (
            <div>
              {Object.entries(envVars).map(([key, value]) => (
                <button key={key} type="button" onClick={() => navigator.clipboard.writeText(value)} title="Click to copy">
                  <span>{key}</span>
                  <code>{value}</code>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <DetailSection title="Live sessions" count={liveTabs.length}>
        {liveTabs.length === 0 ? (
          <EmptyRow icon="codex-chat" title="No live sessions" subtitle="Start or resume an agent from this worktree." />
        ) : (
          liveTabs.slice(0, 4).map((tab) => (
            <SessionRow key={tab.id} tab={tab} active={tab.id === activeTabId} onOpen={() => openTab(tab)} />
          ))
        )}
      </DetailSection>

      <DetailSection title="Recent threads" count={historyItems.length}>
        {historyItems.length === 0 ? (
          <EmptyRow icon="codex-chat" title="No saved threads" subtitle="Completed Codex and Claude chats for this worktree appear here." />
        ) : (
          historyItems.map((thread) => {
            const live = liveTabForHistory(thread);
            const busy = busyAction === `${thread.provider}:${thread.threadId}`;
            return (
              <HistoryRow
                key={`${thread.provider}:${thread.threadId}`}
                item={thread}
                live={Boolean(live)}
                busy={busy}
                onOpen={() => openHistory(thread)}
              />
            );
          })
        )}
      </DetailSection>
    </div>
  );
}

function SummaryTile({ value, label, tone }: { value: string; label: string; tone: 'blue' | 'green' | 'yellow' | 'muted' }) {
  return (
    <div className={`workspace-detail-tile ${tone}`}>
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function DetailSection({ title, count, children }: { title: string; count: number; children: ReactNode }) {
  return (
    <section className="workspace-detail-section">
      <div className="workspace-detail-section-title">
        <span>{title}</span>
        <b>{count}</b>
      </div>
      <div className="workspace-detail-list">{children}</div>
    </section>
  );
}

function SessionRow({ tab, active, onOpen }: { tab: Tab; active: boolean; onOpen: () => void }) {
  const provider = tab.provider || (tab.tabType === 'claude-chat' || tab.tabType === 'claude' ? 'claude' : tab.tabType === 'codex-chat' || tab.tabType === 'codex' ? 'codex' : 'shell');
  return (
    <button type="button" className={`workspace-detail-row ${active ? 'active' : ''}`} onClick={onOpen}>
      <AgentSigil id={tab.tabType} size={26} />
      <span>
        <strong>{tab.label}</strong>
        <small>
          {[sessionLabel(tab.tabType), tab.model ? modelLabel(tab.model) : '', tab.reasoningEffort ? reasoningLabel(tab.reasoningEffort) : '']
            .filter(Boolean)
            .join(' · ')}
        </small>
      </span>
      <em>{active ? 'Active' : provider === 'shell' ? 'Open' : 'Open'}</em>
    </button>
  );
}

function HistoryRow({ item, live, busy, onOpen }: { item: HistoryItem; live: boolean; busy: boolean; onOpen: () => void }) {
  return (
    <button type="button" className="workspace-detail-row history" onClick={onOpen} disabled={busy}>
      <AgentSigil id={`${item.provider}-chat`} size={26} />
      <span>
        <strong>
          {shortThreadLabel(item.threadId)}
          <small>{relativeTimeLabel(item.updatedAt)}</small>
        </strong>
        <small>{item.preview || 'No preview'}</small>
        <i>
          {[item.model ? modelLabel(item.model) : '', `${item.messageCount} msg${item.messageCount === 1 ? '' : 's'}`]
            .filter(Boolean)
            .join(' · ')}
        </i>
      </span>
      <em>{busy ? 'Opening' : live ? 'Open' : 'Resume'}</em>
    </button>
  );
}

function EmptyRow({ icon, title, subtitle }: { icon: string; title: string; subtitle: string }) {
  return (
    <div className="workspace-detail-row empty">
      <AgentSigil id={icon} size={26} />
      <span>
        <strong>{title}</strong>
        <small>{subtitle}</small>
      </span>
    </div>
  );
}

function serverSort(a: server.ServerStatus, b: server.ServerStatus) {
  const order: Record<string, number> = { frontend: 0, backend: 1, sidekiq: 2 };
  return (order[a.name] ?? 99) - (order[b.name] ?? 99);
}

function compactWorkspaceName(name: string, projectName: string) {
  return name.replace(`${projectName}-`, '');
}

function sessionLabel(type: string): string {
  switch (type) {
    case 'claude':
      return 'Claude CLI';
    case 'claude-chat':
      return 'Claude chat';
    case 'codex-chat':
      return 'Codex chat';
    case 'codex':
      return 'Codex CLI';
    case 'shell':
      return 'Shell';
    default:
      return type.replace(/-/g, ' ');
  }
}

function modelLabel(value: string): string {
  switch (value) {
    case 'gpt-5.4-mini':
      return 'GPT-5.4 Mini';
    case 'gpt-5.3-codex':
      return 'GPT-5.3 Codex';
    case 'gpt-5.3-codex-spark':
      return 'GPT-5.3 Codex Spark';
    default:
      return value.toUpperCase();
  }
}

function reasoningLabel(value: string): string {
  if (value === 'xhigh') return 'Extra high';
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function shortThreadLabel(threadId: string): string {
  const trimmed = threadId.trim();
  if (trimmed.length <= 12) return trimmed;
  return trimmed.slice(0, 8);
}

function relativeTimeLabel(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return '';
  const seconds = Math.round((timestamp - Date.now()) / 1000);
  const abs = Math.abs(seconds);
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ['year', 31536000],
    ['month', 2592000],
    ['week', 604800],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ];
  for (const [unit, unitSeconds] of units) {
    if (abs >= unitSeconds) {
      return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto', style: 'narrow' }).format(Math.round(seconds / unitSeconds), unit);
    }
  }
  return 'now';
}
