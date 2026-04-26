import { useCallback, useEffect, useMemo, useState } from 'react';
import { useStore, generateId, PaneLeaf, Tab } from '../store';
import { claudesdk, codexchat, main, workspace } from '../../wailsjs/go/models';
import {
  AllocatePorts,
  CreateAttachedTerminal,
  CreateTerminalInDir,
  LaunchAgent,
  LaunchClaudeChat,
  LaunchCodexChatWithOptions,
  ListClaudeChatHistory,
  ListCodexChatHistory,
  ResumeClaudeChat,
  ResumeCodexChatWithOptions,
  SendClaudeChatMessage,
  SendCodexChatMessage,
} from '../../wailsjs/go/main/App';
import AgentSigil from './AgentSigil';
import OrionMark from './OrionMark';

type ProjectInfo = {
  name: string;
  root: string;
  mainBranch: string;
};

type LauncherKind = 'codex-chat' | 'claude-chat' | 'shell' | `agent:${string}`;
type LauncherMode = 'start' | 'resume';
type HistoryProvider = 'codex' | 'claude';

type CodexLaunchOptions = {
  model: string;
  reasoningEffort: string;
  approvalPolicy: string;
  sandboxMode: string;
  collaborationMode: string;
};

type HistoryItem = {
  provider: HistoryProvider;
  threadId: string;
  workspacePath: string;
  model?: string;
  updatedAt: string;
  messageCount: number;
  preview?: string;
};

interface AgentLauncherProps {
  visible: boolean;
  initialKind: LauncherKind;
  project: ProjectInfo | null;
  workspaces: workspace.Workspace[];
  activeWorkspacePath: string | null;
  agentTypes: main.AgentTypeInfo[];
  onClose: () => void;
}

interface AgentDockProps {
  activeWorkspace?: workspace.Workspace;
  activeTabs: Tab[];
  onOpen: (kind?: LauncherKind) => void;
}

const CODEX_MODELS = [
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

const APPROVAL_POLICIES = [
  { value: 'never', label: 'Full access' },
  { value: 'on-request', label: 'Ask first' },
  { value: 'on-failure', label: 'On failure' },
  { value: 'untrusted', label: 'Untrusted' },
];

const SANDBOX_MODES = [
  { value: 'danger-full-access', label: 'Workspace + network' },
  { value: 'workspace-write', label: 'Workspace write' },
  { value: 'read-only', label: 'Read only' },
];

const COLLABORATION_MODES = [
  { value: 'default', label: 'Default' },
  { value: 'plan', label: 'Plan first' },
];

const DEFAULT_CODEX_OPTIONS: CodexLaunchOptions = {
  model: 'gpt-5.4',
  reasoningEffort: 'xhigh',
  approvalPolicy: 'never',
  sandboxMode: 'danger-full-access',
  collaborationMode: 'default',
};

export function AgentDock({ activeWorkspace, activeTabs, onOpen }: AgentDockProps) {
  const agentTabs = activeTabs.filter((tab) =>
    tab.tabType === 'codex-chat' ||
    tab.tabType === 'claude-chat' ||
    tab.tabType === 'codex' ||
    tab.tabType === 'claude',
  );
  const chatCount = activeTabs.filter((tab) => tab.tabType === 'codex-chat' || tab.tabType === 'claude-chat').length;

  return (
    <div className="agent-dock" aria-label="Agent launcher">
      <button type="button" className="agent-dock-main" onClick={() => onOpen()} title="Agent launcher">
        <OrionMark size={22} />
        <span>
          <strong>Agents</strong>
          <small>{workspaceLabel(activeWorkspace)}</small>
        </span>
        <em>{agentTabs.length}</em>
      </button>
      <div className="agent-dock-tools">
        <button type="button" onClick={() => onOpen('codex-chat')} title="Codex Chat">
          <AgentSigil id="codex-chat" size={18} />
        </button>
        <button type="button" onClick={() => onOpen('claude-chat')} title="Claude Chat">
          <AgentSigil id="claude-chat" size={18} />
        </button>
        <button type="button" onClick={() => onOpen('shell')} title="Shell">
          <AgentSigil id="shell" size={18} />
        </button>
        {chatCount > 0 && <span>{chatCount}</span>}
      </div>
    </div>
  );
}

export default function AgentLauncher({
  visible,
  initialKind,
  project,
  workspaces,
  activeWorkspacePath,
  agentTypes,
  onClose,
}: AgentLauncherProps) {
  const { tabs, setActiveWorkspace, setActiveTab, addTab } = useStore();
  const [workspacePath, setWorkspacePath] = useState('');
  const [mode, setMode] = useState<LauncherMode>('start');
  const [selectedKind, setSelectedKind] = useState<LauncherKind>('codex-chat');
  const [codexOptions, setCodexOptions] = useState<CodexLaunchOptions>(DEFAULT_CODEX_OPTIONS);
  const [prompt, setPrompt] = useState('');
  const [query, setQuery] = useState('');
  const [codexHistory, setCodexHistory] = useState<codexchat.HistoryThread[]>([]);
  const [claudeHistory, setClaudeHistory] = useState<claudesdk.HistoryThread[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const activeWorkspace = useMemo(() => workspaces.find((ws) => ws.path === workspacePath), [workspacePath, workspaces]);
  const liveTabs = useMemo(() => tabs.filter((tab) =>
    tab.workspacePath === workspacePath &&
    tab.tabType !== 'editor' &&
    tab.tabType !== 'diagnostics' &&
    tab.tabType !== 'server',
  ), [tabs, workspacePath]);

  const launcherOptions = useMemo(() => {
    return [
      { kind: 'codex-chat' as LauncherKind, label: 'Codex Chat', subtitle: 'rich chat', icon: 'codex-chat' },
      { kind: 'claude-chat' as LauncherKind, label: 'Claude Chat', subtitle: 'SDK chat', icon: 'claude-chat' },
      ...agentTypes.map((agent) => ({
        kind: `agent:${agent.name}` as LauncherKind,
        label: agent.name === 'codex' || agent.name === 'claude' ? `${agent.label} CLI` : agent.label,
        subtitle: 'terminal agent',
        icon: agent.name,
      })),
      { kind: 'shell' as LauncherKind, label: 'Shell', subtitle: 'terminal', icon: 'shell' },
    ];
  }, [agentTypes]);

  const selectedOption = launcherOptions.find((option) => option.kind === selectedKind) || launcherOptions[0];
  const promptEnabled = selectedKind === 'codex-chat' || selectedKind === 'claude-chat';

  const historyItems = useMemo<HistoryItem[]>(() => {
    const codexItems = codexHistory.map((thread) => ({
      provider: 'codex' as const,
      threadId: thread.threadId,
      workspacePath: thread.workspacePath || workspacePath,
      model: thread.model,
      updatedAt: thread.updatedAt,
      messageCount: thread.messageCount,
      preview: thread.preview,
    }));
    const claudeItems = claudeHistory.map((thread) => ({
      provider: 'claude' as const,
      threadId: thread.threadId,
      workspacePath: thread.workspacePath || workspacePath,
      model: thread.model,
      updatedAt: thread.updatedAt,
      messageCount: thread.messageCount,
      preview: thread.preview,
    }));
    const terms = normalize(query).split(' ').filter(Boolean);
    return [...codexItems, ...claudeItems]
      .filter((thread) => thread.threadId)
      .filter((thread) => {
        if (terms.length === 0) return true;
        const haystack = normalize([thread.provider, thread.threadId, thread.model, thread.preview].filter(Boolean).join(' '));
        return terms.every((term) => haystack.includes(term));
      })
      .sort((a, b) => timestamp(b.updatedAt) - timestamp(a.updatedAt))
      .slice(0, 10);
  }, [claudeHistory, codexHistory, query, workspacePath]);

  useEffect(() => {
    if (!visible) return;
    setWorkspacePath(activeWorkspacePath || workspaces.find((ws) => ws.isMain)?.path || workspaces[0]?.path || '');
    setSelectedKind(initialKind || 'codex-chat');
    setMode('start');
    setPrompt('');
    setQuery('');
    setError(null);
  }, [activeWorkspacePath, initialKind, visible, workspaces]);

  useEffect(() => {
    if (!visible || !workspacePath) return;
    let cancelled = false;
    Promise.all([
      ListCodexChatHistory(workspacePath, 24).catch(() => [] as codexchat.HistoryThread[]),
      ListClaudeChatHistory(workspacePath, 24).catch(() => [] as claudesdk.HistoryThread[]),
    ]).then(([codexThreads, claudeThreads]) => {
      if (cancelled) return;
      setCodexHistory(codexThreads || []);
      setClaudeHistory(claudeThreads || []);
    });
    return () => { cancelled = true; };
  }, [visible, workspacePath]);

  useEffect(() => {
    if (!visible) return;
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', handleKey, { capture: true });
    return () => window.removeEventListener('keydown', handleKey, { capture: true });
  }, [onClose, visible]);

  const updateCodexOption = useCallback((key: keyof CodexLaunchOptions, value: string) => {
    setCodexOptions((current) => ({ ...current, [key]: value }));
  }, []);

  const activateTab = useCallback((tab: Tab) => {
    setActiveWorkspace(tab.workspacePath);
    setActiveTab(tab.id);
    onClose();
  }, [onClose, setActiveTab, setActiveWorkspace]);

  const addChatTab = useCallback((provider: 'codex' | 'claude', session: codexchat.SessionInfo | claudesdk.SessionInfo, fallbackThreadId?: string, fallbackModel?: string) => {
    addTab({
      id: generateId('tab'),
      label: session?.label || (provider === 'codex' ? 'Codex Chat' : 'Claude Chat'),
      rootPane: { type: 'chat', id: generateId('pane'), chatSessionId: session.id, chatThreadId: session.threadId || fallbackThreadId, chatKind: provider } as PaneLeaf,
      tabType: provider === 'codex' ? 'codex-chat' : 'claude-chat',
      workspacePath,
      provider,
      viewMode: 'chat',
      runtimeSessionId: session.id,
      threadId: session.threadId || fallbackThreadId,
      model: session.model || fallbackModel,
      reasoningEffort: session.reasoningEffort,
      approvalPolicy: session.approvalPolicy,
      sandboxMode: session.sandboxMode,
      permissionMode: 'permissionMode' in session ? session.permissionMode : undefined,
      collaborationMode: 'collaborationMode' in session ? session.collaborationMode : undefined,
    });
  }, [addTab, workspacePath]);

  const launchSelected = useCallback(async () => {
    if (!project || !workspacePath || !activeWorkspace) return;
    setBusy('launch');
    setError(null);
    try {
      await AllocatePorts(project.root, workspacePath, activeWorkspace.isMain).catch(() => {});
      setActiveWorkspace(workspacePath);
      if (selectedKind === 'codex-chat') {
        const session = await LaunchCodexChatWithOptions(
          project.root,
          workspacePath,
          codexOptions.model,
          codexOptions.reasoningEffort,
          codexOptions.approvalPolicy,
          codexOptions.sandboxMode,
          codexOptions.collaborationMode,
        );
        addChatTab('codex', session);
        if (prompt.trim()) await SendCodexChatMessage(session.id, prompt.trim(), []);
      } else if (selectedKind === 'claude-chat') {
        const session = await LaunchClaudeChat(project.root, workspacePath);
        addChatTab('claude', session);
        if (prompt.trim()) await SendClaudeChatMessage(session.id, prompt.trim(), []);
      } else if (selectedKind === 'shell') {
        const termId = generateId('term');
        await CreateTerminalInDir(termId, workspacePath);
        const shellNum = tabs.filter((tab) => tab.workspacePath === workspacePath && tab.tabType === 'shell').length + 1;
        addTab({
          id: generateId('tab'),
          label: `Shell ${shellNum}`,
          rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
          tabType: 'shell',
          workspacePath,
        });
      } else {
        const agentName = selectedKind.replace(/^agent:/, '');
        const agent = agentTypes.find((item) => item.name === agentName);
        const tmuxSession = await LaunchAgent(project.root, workspacePath, agentName);
        const termId = generateId('term');
        await CreateAttachedTerminal(termId, tmuxSession);
        addTab({
          id: generateId('tab'),
          label: agent?.label || agentName,
          rootPane: { type: 'terminal', id: generateId('pane'), terminalId: termId } as PaneLeaf,
          tabType: (agentName === 'claude' || agentName === 'codex') ? agentName as 'claude' | 'codex' : 'shell',
          workspacePath,
          provider: (agentName === 'claude' || agentName === 'codex') ? agentName as 'claude' | 'codex' : undefined,
          viewMode: 'terminal',
          runtimeSessionId: tmuxSession,
        });
      }
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  }, [activeWorkspace, addChatTab, addTab, agentTypes, codexOptions, onClose, project, prompt, selectedKind, setActiveWorkspace, tabs, workspacePath]);

  const openHistory = useCallback(async (item: HistoryItem) => {
    if (!project || !workspacePath) return;
    const live = tabs.find((tab) =>
      tab.workspacePath === workspacePath &&
      tab.provider === item.provider &&
      tab.threadId === item.threadId,
    );
    if (live) {
      activateTab(live);
      return;
    }
    setBusy(`${item.provider}:${item.threadId}`);
    setError(null);
    try {
      setActiveWorkspace(workspacePath);
      if (item.provider === 'codex') {
        const session = await ResumeCodexChatWithOptions(
          project.root,
          workspacePath,
          item.threadId,
          item.model || codexOptions.model,
          codexOptions.reasoningEffort,
          codexOptions.approvalPolicy,
          codexOptions.sandboxMode,
          codexOptions.collaborationMode,
        );
        addChatTab('codex', session, item.threadId, item.model);
      } else {
        const session = await ResumeClaudeChat(project.root, workspacePath, item.threadId);
        addChatTab('claude', session, item.threadId, item.model);
      }
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  }, [activateTab, addChatTab, codexOptions, onClose, project, setActiveWorkspace, tabs, workspacePath]);

  if (!visible) return null;

  return (
    <div className="search-overlay agent-launcher-overlay" onMouseDown={onClose}>
      <div className="agent-launcher" role="dialog" aria-modal="true" aria-label="Agent launcher" onMouseDown={(event) => event.stopPropagation()}>
        <div className="agent-launcher-header">
          <div className="agent-launcher-title">
            <OrionMark size={30} />
            <div>
              <strong>Agent launcher</strong>
              <span>{workspaceLabel(activeWorkspace)}</span>
            </div>
          </div>
          <div className="agent-launcher-mode" role="tablist" aria-label="Launcher mode">
            <button type="button" className={mode === 'start' ? 'active' : ''} onClick={() => setMode('start')}>Start</button>
            <button type="button" className={mode === 'resume' ? 'active' : ''} onClick={() => setMode('resume')}>Resume</button>
          </div>
        </div>

        <div className="agent-launcher-body">
          <aside className="agent-launcher-sidebar">
            <label className="agent-launcher-field">
              <span>Workspace</span>
              <select value={workspacePath} onChange={(event) => setWorkspacePath(event.target.value)}>
                {workspaces.map((ws) => (
                  <option key={ws.path} value={ws.path}>{workspaceLabel(ws)}</option>
                ))}
              </select>
            </label>

            <div className="agent-launcher-options" role="listbox" aria-label="Agent options">
              {launcherOptions.map((option) => (
                <button
                  key={option.kind}
                  type="button"
                  className={selectedKind === option.kind ? 'active' : ''}
                  onClick={() => {
                    setSelectedKind(option.kind);
                    setMode('start');
                  }}
                >
                  <AgentSigil id={option.icon} size={24} />
                  <span>
                    <strong>{option.label}</strong>
                    <small>{option.subtitle}</small>
                  </span>
                </button>
              ))}
            </div>

            {liveTabs.length > 0 && (
              <div className="agent-launcher-live">
                <span>{liveTabs.length} live</span>
                <div>
                  {liveTabs.slice(0, 5).map((tab) => (
                    <button key={tab.id} type="button" onClick={() => activateTab(tab)}>
                      <AgentSigil id={tab.tabType} size={18} />
                      <strong>{tab.label}</strong>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </aside>

          <main className="agent-launcher-main">
            {mode === 'start' ? (
              <>
                <div className="agent-launcher-start-head">
                  <AgentSigil id={selectedOption?.icon || 'codex-chat'} size={36} strong />
                  <div>
                    <strong>{selectedOption?.label || 'Agent'}</strong>
                    <span>{workspacePath}</span>
                  </div>
                </div>

                {selectedKind === 'codex-chat' && (
                  <div className="agent-launcher-grid">
                    <label>
                      <span>Model</span>
                      <select value={codexOptions.model} onChange={(event) => updateCodexOption('model', event.target.value)}>
                        {CODEX_MODELS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                    <label>
                      <span>Reasoning</span>
                      <select value={codexOptions.reasoningEffort} onChange={(event) => updateCodexOption('reasoningEffort', event.target.value)}>
                        {REASONING_EFFORTS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                    <label>
                      <span>Approvals</span>
                      <select value={codexOptions.approvalPolicy} onChange={(event) => updateCodexOption('approvalPolicy', event.target.value)}>
                        {APPROVAL_POLICIES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                    <label>
                      <span>Sandbox</span>
                      <select value={codexOptions.sandboxMode} onChange={(event) => updateCodexOption('sandboxMode', event.target.value)}>
                        {SANDBOX_MODES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                    <label>
                      <span>Mode</span>
                      <select value={codexOptions.collaborationMode} onChange={(event) => updateCodexOption('collaborationMode', event.target.value)}>
                        {COLLABORATION_MODES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                    </label>
                  </div>
                )}

                {promptEnabled && (
                  <label className="agent-launcher-prompt">
                    <span>Initial prompt</span>
                    <textarea
                      value={prompt}
                      onChange={(event) => setPrompt(event.target.value)}
                      placeholder="Ask the agent to plan or build something..."
                    />
                  </label>
                )}

                <div className="agent-launcher-summary">
                  {selectedKind === 'codex-chat' ? (
                    <>
                      <span>{codexOptions.model}</span>
                      <span>{reasoningLabel(codexOptions.reasoningEffort)}</span>
                      <span>{approvalLabel(codexOptions.approvalPolicy)}</span>
                      <span>{sandboxLabel(codexOptions.sandboxMode)}</span>
                      <span>{codexOptions.collaborationMode === 'plan' ? 'plan first' : 'default'}</span>
                    </>
                  ) : selectedKind === 'claude-chat' ? (
                    <>
                      <span>claude-opus-4-7</span>
                      <span>extra high</span>
                      <span>full access</span>
                      <span>plan mode</span>
                    </>
                  ) : (
                    <span>{selectedOption?.subtitle || 'terminal'}</span>
                  )}
                </div>
              </>
            ) : (
              <div className="agent-launcher-resume">
                <div className="agent-launcher-resume-head">
                  <input
                    autoFocus
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="Search recent Codex and Claude threads..."
                  />
                  <span>{historyItems.length}</span>
                </div>
                <div className="agent-launcher-history">
                  {historyItems.map((item) => {
                    const live = tabs.some((tab) =>
                      tab.workspacePath === workspacePath &&
                      tab.provider === item.provider &&
                      tab.threadId === item.threadId,
                    );
                    const isBusy = busy === `${item.provider}:${item.threadId}`;
                    return (
                      <button key={`${item.provider}:${item.threadId}`} type="button" onClick={() => openHistory(item)} disabled={Boolean(busy)}>
                        <AgentSigil id={`${item.provider}-chat`} size={26} />
                        <span>
                          <strong>
                            {shortThreadLabel(item.threadId)}
                            <small>{relativeTimeLabel(item.updatedAt)}</small>
                          </strong>
                          <small>{item.preview || 'No preview'}</small>
                          <i>{[item.model ? modelLabel(item.model) : '', `${item.messageCount} msg${item.messageCount === 1 ? '' : 's'}`].filter(Boolean).join(' · ')}</i>
                        </span>
                        <em>{isBusy ? 'Opening' : live ? 'Open' : 'Resume'}</em>
                      </button>
                    );
                  })}
                  {historyItems.length === 0 && (
                    <div className="agent-launcher-empty">No recent threads</div>
                  )}
                </div>
              </div>
            )}
          </main>
        </div>

        {error && <div className="agent-launcher-error">{error}</div>}

        <div className="agent-launcher-footer">
          <button type="button" onClick={onClose}>Cancel</button>
          {mode === 'start' ? (
            <button type="button" className="primary" onClick={launchSelected} disabled={!project || !workspacePath || Boolean(busy)}>
              {busy === 'launch' ? 'Starting' : `Start ${selectedOption?.label || 'agent'}`}
            </button>
          ) : (
            <button type="button" className="primary" onClick={() => setMode('start')}>New session</button>
          )}
        </div>
      </div>
    </div>
  );
}

function workspaceLabel(ws?: workspace.Workspace): string {
  if (!ws) return 'No workspace';
  if (ws.isMain) return 'main';
  return ws.branch || ws.name || ws.path;
}

function normalize(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
}

function timestamp(value: string) {
  const parsed = Date.parse(value || '');
  return Number.isFinite(parsed) ? parsed : 0;
}

function shortThreadLabel(threadId: string): string {
  const trimmed = threadId.trim();
  if (trimmed.length <= 12) return trimmed;
  return trimmed.slice(0, 8);
}

function relativeTimeLabel(value: string): string {
  const parsed = timestamp(value);
  if (!parsed) return '';
  const seconds = Math.round((parsed - Date.now()) / 1000);
  const abs = Math.abs(seconds);
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ['year', 31536000],
    ['month', 2592000],
    ['week', 604800],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ];
  for (const [unit, size] of units) {
    if (abs >= size) {
      return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto', style: 'narrow' }).format(Math.round(seconds / size), unit);
    }
  }
  return 'now';
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
  return value === 'xhigh' ? 'extra high' : value;
}

function approvalLabel(value: string): string {
  if (value === 'never') return 'full access';
  return value.replace(/-/g, ' ');
}

function sandboxLabel(value: string): string {
  if (value === 'danger-full-access') return 'workspace + network';
  return value.replace(/-/g, ' ');
}
