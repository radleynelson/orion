import { emitDevEvent } from './runtime';

const PROJECT_ROOT = '/Users/radleynelson/orion';
const MAIN_WORKSPACE = PROJECT_ROOT;

type Workspace = {
  name: string;
  path: string;
  branch: string;
  isMain: boolean;
  hasAgent: boolean;
};

type AgentTypeInfo = {
  name: string;
  command: string;
  label: string;
  provider?: string;
  icon?: string;
  model?: string;
  reasoningEffort?: string;
  approvalPolicy?: string;
  sandboxMode?: string;
  permissionMode?: string;
  collaborationMode?: string;
  chatCapable: boolean;
};

type ServerStatus = {
  name: string;
  port: number;
  running: boolean;
  tmuxSession: string;
};

type ChatAttachment = {
  id?: string;
  name?: string;
  path: string;
  mimeType?: string;
  size?: number;
};

type ChatMessage = {
  id: string;
  sessionId: string;
  threadId?: string;
  type: string;
  subtype?: string;
  role?: string;
  text?: string;
  status?: string;
  toolUseId?: string;
  toolName?: string;
  details?: string;
  attachments?: ChatAttachment[];
  createdAt: string;
};

const terminalSessions = new Map<string, string>();
const serverStatuses = new Map<string, ServerStatus[]>();
const chatMessages = new Map<string, ChatMessage[]>();
const mockFileContents = new Map<string, string>();
const runningLSPServers = new Set<string>();
const openLSPDocuments = new Map<string, { language: string; text: string; version: number }>();

function recordMockLSP(event: string, detail: Record<string, unknown> = {}): void {
  const target = window as unknown as { __orionPreviewLSPLog?: Array<Record<string, unknown>> };
  target.__orionPreviewLSPLog ||= [];
  target.__orionPreviewLSPLog.push({ event, ...detail });
}

let workspaces: Workspace[] = [
  { name: 'orion', path: MAIN_WORKSPACE, branch: 'main', isMain: true, hasAgent: false },
  {
    name: 'orion-browser-preview',
    path: `${PROJECT_ROOT}/.orion-preview/browser-preview`,
    branch: 'browser-preview',
    isMain: false,
    hasAgent: true,
  },
];

const agents: AgentTypeInfo[] = [
  {
    name: 'codex',
    command: 'codex',
    label: 'Codex',
    provider: 'codex',
    icon: 'codex',
    reasoningEffort: 'xhigh',
    approvalPolicy: 'never',
    sandboxMode: 'danger-full-access',
    collaborationMode: 'default',
    chatCapable: true,
  },
  {
    name: 'claude',
    command: 'claude',
    label: 'Claude',
    provider: 'claude',
    icon: 'claude',
    permissionMode: 'bypassPermissions',
    chatCapable: true,
  },
];

function now(): string {
  return new Date().toISOString();
}

function id(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function encodeBase64Payload(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

function terminalBanner(label: string): string {
  return [
    '\x1b[36mOrion browser preview\x1b[0m',
    `Attached to mock session: ${label}`,
    'This Vite mode replaces Wails/tmux calls with browser-safe fixtures.',
    '$ ',
  ].join('\r\n');
}

function emitTerminalBanner(terminalId: string, label: string): void {
  window.setTimeout(() => {
    emitDevEvent(`terminal:output:${terminalId}`, encodeBase64Payload(terminalBanner(label)));
  }, 50);
}

function sessionInfo(kind: 'codex' | 'claude', workspacePath: string) {
  const sessionId = id(`${kind}-chat`);
  const threadId = id(`${kind}-thread`);
  chatMessages.set(sessionId, [
    {
      id: id('msg'),
      sessionId,
      threadId,
      type: 'status',
      role: 'assistant',
      status: 'idle',
      text: `${kind === 'codex' ? 'Codex' : 'Claude'} browser preview session ready.`,
      createdAt: now(),
    },
  ]);
  return {
    id: sessionId,
    type: `${kind}-chat`,
    label: kind === 'codex' ? 'Codex Chat' : 'Claude Chat',
    workspacePath,
    status: 'idle',
    threadId,
    provider: kind,
    icon: kind,
    viewMode: 'chat',
    runtimeSessionId: sessionId,
    model: kind === 'codex' ? 'gpt-5.5' : 'default',
    reasoningEffort: kind === 'codex' ? 'xhigh' : '',
    approvalPolicy: kind === 'codex' ? 'never' : '',
    sandboxMode: kind === 'codex' ? 'danger-full-access' : '',
    permissionMode: kind === 'claude' ? 'bypassPermissions' : '',
    collaborationMode: kind === 'codex' ? 'default' : '',
  };
}

function sendChatEcho(kind: 'codex' | 'claude', sessionId: string, text: string): void {
  const messages = chatMessages.get(sessionId) || [];
  const threadId = messages.find((message) => message.threadId)?.threadId || id(`${kind}-thread`);
  const userMessage: ChatMessage = {
    id: id('msg'),
    sessionId,
    threadId,
    type: 'message',
    role: 'user',
    text,
    createdAt: now(),
  };
  const assistantMessage: ChatMessage = {
    id: id('msg'),
    sessionId,
    threadId,
    type: 'message',
    role: 'assistant',
    text: `Browser preview received: ${text}`,
    createdAt: now(),
  };
  chatMessages.set(sessionId, [...messages, userMessage, assistantMessage]);
  emitDevEvent(`${kind}-chat:message:${sessionId}`, userMessage);
  window.setTimeout(() => {
    emitDevEvent(`${kind}-chat:message:${sessionId}`, assistantMessage);
    emitDevEvent(`${kind}-chat:message`, assistantMessage);
  }, 150);
}

function workspaceForPath(path: string): Workspace {
  return workspaces.find((workspace) => workspace.path === path) || workspaces[0];
}

function rel(path: string): string {
  return path.startsWith(`${MAIN_WORKSPACE}/`) ? path.slice(MAIN_WORKSPACE.length + 1) : path;
}

export async function GetLastProject(): Promise<string> {
  return PROJECT_ROOT;
}

export async function GetProjectInfo(_path: string): Promise<{ name: string; root: string; mainBranch: string }> {
  return { name: 'orion', root: PROJECT_ROOT, mainBranch: 'main' };
}

export async function GetProjectInfoFromCwd(): Promise<{ name: string; root: string; mainBranch: string }> {
  return GetProjectInfo(PROJECT_ROOT);
}

export async function OpenProjectDialog(): Promise<{ name: string; root: string; mainBranch: string }> {
  return GetProjectInfo(PROJECT_ROOT);
}

export async function SetActiveProject(_path: string): Promise<void> {}

export async function ListWorkspaces(_repoRoot: string): Promise<Workspace[]> {
  return workspaces;
}

export async function CreateWorkspaceFrom(
  _repoRoot: string,
  name: string,
  baseRef: string,
): Promise<Workspace> {
  const branch = name || 'browser-preview-workspace';
  const workspace = {
    name: `orion-${branch}`,
    path: `${PROJECT_ROOT}/.orion-preview/${branch}`,
    branch: baseRef || branch,
    isMain: false,
    hasAgent: false,
  };
  workspaces = [...workspaces, workspace];
  return workspace;
}

export async function CreateWorkspace(repoRoot: string, name: string): Promise<Workspace> {
  return CreateWorkspaceFrom(repoRoot, name, 'main');
}

export async function DeleteWorkspace(_repoRoot: string, path: string): Promise<void> {
  workspaces = workspaces.filter((workspace) => workspace.path !== path);
}

export async function GetSavedTabs(): Promise<unknown[]> {
  return [];
}

export async function SaveTabs(_tabs: unknown): Promise<void> {}

export async function RecoverSessions(_repoName: string, _workspacePaths: string[]): Promise<unknown[]> {
  return [];
}

export async function GetAgentTypes(_repoRoot: string): Promise<AgentTypeInfo[]> {
  return agents;
}

export async function GetAgentNames(repoRoot: string): Promise<AgentTypeInfo[]> {
  return GetAgentTypes(repoRoot);
}

export async function CreateTerminalInDir(terminalId: string, dir: string): Promise<void> {
  const session = `orion-preview-${terminalId}`;
  terminalSessions.set(terminalId, session);
  emitTerminalBanner(terminalId, rel(dir));
}

export async function CreateTerminal(terminalId: string): Promise<void> {
  return CreateTerminalInDir(terminalId, MAIN_WORKSPACE);
}

export async function CreateAttachedTerminal(terminalId: string, tmuxSession: string): Promise<void> {
  terminalSessions.set(terminalId, tmuxSession);
  emitTerminalBanner(terminalId, tmuxSession);
}

export async function LaunchShell(_repoRoot: string, workspacePath: string): Promise<string> {
  return `orion-preview-shell-${workspaceForPath(workspacePath).branch}`;
}

export async function LaunchAgent(
  _repoRoot: string,
  workspacePath: string,
  agentType: string,
): Promise<string> {
  workspaceForPath(workspacePath).hasAgent = true;
  return `orion-preview-${agentType}-${workspaceForPath(workspacePath).branch}`;
}

export async function CloseTerminal(terminalId: string): Promise<void> {
  terminalSessions.delete(terminalId);
  emitDevEvent(`terminal:exit:${terminalId}`);
}

export async function DetachTerminal(terminalId: string): Promise<void> {
  terminalSessions.delete(terminalId);
}

export async function GetTmuxSession(terminalId: string): Promise<string> {
  return terminalSessions.get(terminalId) || `orion-preview-${terminalId}`;
}

export async function IsTerminalBusy(_terminalId: string): Promise<boolean> {
  return false;
}

export async function ListTerminals(): Promise<string[]> {
  return [...terminalSessions.keys()];
}

export async function KillSession(_tmuxSession: string): Promise<void> {}

export async function StartServers(
  _repoRoot: string,
  workspacePath: string,
  isMain: boolean,
): Promise<ServerStatus[]> {
  const base = isMain ? 5173 : 12073;
  const statuses = [
    { name: 'frontend', port: base, running: true, tmuxSession: `orion-preview-srv-${base}` },
    { name: 'backend', port: base + 1, running: true, tmuxSession: `orion-preview-api-${base + 1}` },
  ];
  serverStatuses.set(workspacePath, statuses);
  return statuses;
}

export async function StopServers(workspacePath: string): Promise<void> {
  serverStatuses.set(workspacePath, []);
}

export async function GetServerStatuses(
  _repoRoot: string,
  workspacePath: string,
): Promise<ServerStatus[]> {
  return serverStatuses.get(workspacePath) || [];
}

export async function AllocatePorts(
  _repoRoot: string,
  workspacePath: string,
  isMain: boolean,
): Promise<Record<string, number>> {
  const frontend = isMain ? 5173 : 12073;
  serverStatuses.set(workspacePath, [
    { name: 'frontend', port: frontend, running: false, tmuxSession: '' },
    { name: 'backend', port: frontend + 1, running: false, tmuxSession: '' },
  ]);
  return { frontend, backend: frontend + 1 };
}

export async function GetWorkspaceEnv(workspacePath: string): Promise<Record<string, string>> {
  const frontend = workspacePath === MAIN_WORKSPACE ? 5173 : 12073;
  return {
    FRONTEND_PORT: String(frontend),
    FRONTEND_URL: `http://localhost:${frontend}`,
    BACKEND_PORT: String(frontend + 1),
    BACKEND_URL: `http://localhost:${frontend + 1}`,
  };
}

export async function OpenBrowser(_repoRoot: string, workspacePath: string): Promise<void> {
  const env = await GetWorkspaceEnv(workspacePath);
  window.open(env.FRONTEND_URL, '_blank', 'noopener,noreferrer');
}

export async function LaunchCodexChatWithOptions(
  _repoRoot: string,
  workspacePath: string,
  _model: string,
  _reasoningEffort: string,
  _approvalPolicy: string,
  _sandboxMode: string,
  _collaborationMode: string,
) {
  return sessionInfo('codex', workspacePath);
}

export async function LaunchCodexChat(repoRoot: string, workspacePath: string) {
  return LaunchCodexChatWithOptions(repoRoot, workspacePath, '', '', '', '', '');
}

export async function LaunchClaudeChat(_repoRoot: string, workspacePath: string) {
  return sessionInfo('claude', workspacePath);
}

export async function LaunchClaudeChatWithOptions(
  repoRoot: string,
  workspacePath: string,
  _model: string,
  _reasoningEffort: string,
  _approvalPolicy: string,
  _sandboxMode: string,
  _permissionMode: string,
) {
  return LaunchClaudeChat(repoRoot, workspacePath);
}

export async function ResumeCodexChatWithOptions(
  repoRoot: string,
  workspacePath: string,
  _threadId: string,
  model: string,
  reasoningEffort: string,
  approvalPolicy: string,
  sandboxMode: string,
  collaborationMode: string,
) {
  return LaunchCodexChatWithOptions(
    repoRoot,
    workspacePath,
    model,
    reasoningEffort,
    approvalPolicy,
    sandboxMode,
    collaborationMode,
  );
}

export async function ResumeCodexChat(repoRoot: string, workspacePath: string, threadId: string) {
  return ResumeCodexChatWithOptions(repoRoot, workspacePath, threadId, '', '', '', '', '');
}

export async function ResumeClaudeChatWithOptions(
  repoRoot: string,
  workspacePath: string,
  _threadId: string,
  model: string,
  reasoningEffort: string,
  approvalPolicy: string,
  sandboxMode: string,
  permissionMode: string,
) {
  return LaunchClaudeChatWithOptions(
    repoRoot,
    workspacePath,
    model,
    reasoningEffort,
    approvalPolicy,
    sandboxMode,
    permissionMode,
  );
}

export async function ResumeClaudeChat(repoRoot: string, workspacePath: string, threadId: string) {
  return ResumeClaudeChatWithOptions(repoRoot, workspacePath, threadId, '', '', '', '', '');
}

export async function AttachClaudeChat(_tmuxSession: string, workspacePath: string) {
  return sessionInfo('claude', workspacePath);
}

export async function ConvertChatToTerminalWithOptions(
  _repoRoot: string,
  _workspacePath: string,
  sessionID: string,
  chatKind: string,
  _model: string,
  _reasoningEffort: string,
  _permissionMode: string,
  _collaborationMode: string,
): Promise<string> {
  return `orion-preview-${chatKind}-${sessionID}`;
}

export async function ConvertChatToTerminal(
  repoRoot: string,
  workspacePath: string,
  sessionID: string,
  chatKind: string,
): Promise<string> {
  return ConvertChatToTerminalWithOptions(repoRoot, workspacePath, sessionID, chatKind, '', '', '', '');
}

export async function ConvertTerminalToClaudeChatWithOptions(
  _repoRoot: string,
  workspacePath: string,
  _tmuxSession: string,
  _model: string,
  _reasoningEffort: string,
  _approvalPolicy: string,
  _sandboxMode: string,
  _permissionMode: string,
) {
  return sessionInfo('claude', workspacePath);
}

export async function ConvertTerminalToClaudeChat(
  repoRoot: string,
  workspacePath: string,
  tmuxSession: string,
) {
  return ConvertTerminalToClaudeChatWithOptions(repoRoot, workspacePath, tmuxSession, '', '', '', '', '');
}

export async function ConvertTerminalToCodexChatWithOptions(
  _repoRoot: string,
  workspacePath: string,
  _tmuxSession: string,
  _model: string,
  _reasoningEffort: string,
  _approvalPolicy: string,
  _sandboxMode: string,
  _collaborationMode: string,
) {
  return sessionInfo('codex', workspacePath);
}

export async function ConvertTerminalToCodexChat(
  repoRoot: string,
  workspacePath: string,
  tmuxSession: string,
) {
  return ConvertTerminalToCodexChatWithOptions(repoRoot, workspacePath, tmuxSession, '', '', '', '', '');
}

export async function GetCodexChatMessages(sessionID: string): Promise<ChatMessage[]> {
  return chatMessages.get(sessionID) || [];
}

export async function GetClaudeChatMessages(sessionID: string): Promise<ChatMessage[]> {
  return chatMessages.get(sessionID) || [];
}

export async function SendCodexChatMessage(
  sessionID: string,
  text: string,
  _attachments: ChatAttachment[],
): Promise<void> {
  sendChatEcho('codex', sessionID, text);
}

export async function SendClaudeChatMessage(
  sessionID: string,
  text: string,
  _attachments: ChatAttachment[],
): Promise<void> {
  sendChatEcho('claude', sessionID, text);
}

export async function AnswerCodexChatRequest(
  sessionID: string,
  toolUseID: string,
  result: string,
): Promise<void> {
  sendChatEcho('codex', sessionID, `Answer ${toolUseID}: ${result}`);
}

export async function AnswerClaudeChatRequest(
  sessionID: string,
  toolUseID: string,
  result: string,
): Promise<void> {
  sendChatEcho('claude', sessionID, `Answer ${toolUseID}: ${result}`);
}

export async function ApproveCodexPlan(sessionID: string): Promise<void> {
  sendChatEcho('codex', sessionID, 'Plan approved.');
}

export async function ApproveClaudePlan(sessionID: string): Promise<void> {
  sendChatEcho('claude', sessionID, 'Plan approved.');
}

export async function StopCodexChat(_sessionID: string): Promise<void> {}
export async function StopClaudeChat(_sessionID: string): Promise<void> {}

export async function ListCodexChatHistory(_workspacePath: string, _query: string): Promise<unknown[]> {
  return [];
}

export async function ListClaudeChatHistory(_workspacePath: string, _query: string): Promise<unknown[]> {
  return [];
}

export async function ListCodexChatSessions(_workspacePaths: string[]): Promise<unknown[]> {
  return [];
}

export async function ListClaudeChatSessions(_workspacePaths: string[]): Promise<unknown[]> {
  return [];
}

export async function OpenChatAttachmentDialog(): Promise<ChatAttachment[]> {
  return [];
}

export async function GetChangedFiles(_workspacePath: string) {
  return GetChangedFilesAgainst(_workspacePath, '');
}

export async function GetChangedFilesAgainst(_workspacePath: string, _base: string) {
  return [
    { path: 'frontend/src/lib/terminal.ts', status: 'M', statusText: 'modified' },
    { path: 'internal/git/manager.go', status: 'M', statusText: 'modified' },
    { path: 'frontend/src/dev/wails/App.ts', status: '?', statusText: 'untracked' },
  ];
}

export async function GetUnifiedDiff(_workspacePath: string, _base: string, filePath: string): Promise<string> {
  return [
    `diff --git a/${filePath} b/${filePath}`,
    `--- a/${filePath}`,
    `+++ b/${filePath}`,
    '@@ -1,3 +1,4 @@',
    ' context line',
    '-old browser preview behavior',
    '+new browser preview behavior',
    '+mocked Wails bridge',
  ].join('\n');
}

export async function GetGitStatus(_workspacePath: string) {
  return {
    branch: 'browser-preview',
    upstream: 'origin/browser-preview',
    ahead: 1,
    behind: 0,
    hasChanges: true,
    changeCount: 3,
    detached: false,
    canPull: false,
    canPush: true,
  };
}

export async function GitFetch(workspacePath: string) {
  return {
    action: 'fetch',
    output: 'Mock fetch completed.',
    status: await GetGitStatus(workspacePath),
    durationMs: 18,
  };
}

export async function GitPull(workspacePath: string) {
  return {
    action: 'pull',
    output: 'Already up to date.',
    status: await GetGitStatus(workspacePath),
    durationMs: 24,
  };
}

export async function GitPush(workspacePath: string) {
  return {
    action: 'push',
    output: 'Mock push completed.',
    status: await GetGitStatus(workspacePath),
    durationMs: 31,
  };
}

export async function GetFileDiff(_workspacePath: string, filePath: string) {
  return {
    originalContent: `// Original ${filePath}\n`,
    modifiedContent: `// Modified ${filePath}\n// Browser preview fixture\n`,
    language: filePath.endsWith('.go') ? 'go' : filePath.endsWith('.tsx') ? 'typescript' : 'plaintext',
  };
}

export async function DiscardFileChanges(_workspacePath: string, _filePath: string): Promise<void> {}
export async function DiscardAllChanges(_workspacePath: string): Promise<void> {}
export async function WatchWorkspace(_workspacePath: string): Promise<void> {}

export async function ListDirectory(path: string, _depth: number) {
  const rootEntries = [
    { name: 'frontend', path: `${MAIN_WORKSPACE}/frontend`, isDir: true, size: 0 },
    { name: 'internal', path: `${MAIN_WORKSPACE}/internal`, isDir: true, size: 0 },
    { name: 'app.go', path: `${MAIN_WORKSPACE}/app.go`, isDir: false, size: 42000 },
    { name: 'main.go', path: `${MAIN_WORKSPACE}/main.go`, isDir: false, size: 12000 },
  ];
  const frontendEntries = [
    { name: 'src', path: `${MAIN_WORKSPACE}/frontend/src`, isDir: true, size: 0 },
    { name: 'package.json', path: `${MAIN_WORKSPACE}/frontend/package.json`, isDir: false, size: 1200 },
  ];
  const srcEntries = [
    { name: 'App.tsx', path: `${MAIN_WORKSPACE}/frontend/src/App.tsx`, isDir: false, size: 72000 },
    { name: 'components', path: `${MAIN_WORKSPACE}/frontend/src/components`, isDir: true, size: 0 },
    { name: 'dev', path: `${MAIN_WORKSPACE}/frontend/src/dev`, isDir: true, size: 0 },
  ];
  if (path.endsWith('/frontend')) return frontendEntries;
  if (path.endsWith('/frontend/src')) return srcEntries;
  return rootEntries;
}

export async function ReadFileContents(path: string): Promise<string> {
  const existing = mockFileContents.get(path);
  if (existing !== undefined) return existing;

  return [
    `// ${rel(path)}`,
    '// Browser preview fixture content.',
    'export const preview = true;',
    'preview.toString();',
    '',
  ].join('\n');
}

export async function WriteFileContents(path: string, content: string): Promise<void> {
  mockFileContents.set(path, content);
  emitDevEvent('git:files-changed');
}

export async function FormatFile(_repoRoot: string, _filePath: string, content: string): Promise<{ formatted: boolean; content: string; error?: string }> {
  const formatted = content.endsWith('\n') ? content : `${content}\n`;
  return { formatted: formatted !== content, content: formatted };
}

export async function RunOnSave(_repoRoot: string, _filePath: string): Promise<string[]> {
  return [];
}

export async function LintFile(_repoRoot: string, _filePath: string): Promise<{ output: string; error?: string }> {
  return { output: '' };
}

export async function GetFormatOnSaveExtensions(_repoRoot: string): Promise<string[]> {
  return ['.ts', '.tsx', '.js', '.jsx', '.go', '.rb', '.css', '.scss', '.json', '.html'];
}

export async function StartLSP(_repoRoot: string, language: string, _workspacePath: string): Promise<void> {
  runningLSPServers.add(language);
  recordMockLSP('start', { language });
}

export async function StopLSP(language: string): Promise<void> {
  runningLSPServers.delete(language);
  recordMockLSP('stop', { language });
}

export async function IsLSPRunning(language: string): Promise<boolean> {
  return runningLSPServers.has(language);
}

export async function ListLSPServers(): Promise<string[]> {
  return [...runningLSPServers];
}

export async function SendLSPMessage(language: string, message: string): Promise<void> {
  const parsed = JSON.parse(message);
  recordMockLSP('message', { language, method: parsed?.method });
  const doc = parsed?.params?.textDocument;
  if (parsed.method === 'textDocument/didOpen' && doc?.uri) {
    openLSPDocuments.set(doc.uri, {
      language: doc.languageId || language,
      text: doc.text || '',
      version: doc.version || 1,
    });
    emitMockDiagnostics(language, doc.uri, doc.text || '');
  } else if (parsed.method === 'textDocument/didChange' && doc?.uri) {
    const text = parsed.params?.contentChanges?.[0]?.text || '';
    openLSPDocuments.set(doc.uri, {
      language,
      text,
      version: doc.version || 1,
    });
    emitMockDiagnostics(language, doc.uri, text);
  } else if (parsed.method === 'textDocument/didSave' && doc?.uri) {
    const current = openLSPDocuments.get(doc.uri);
    if (current) mockFileContents.set(doc.uri.replace('file://', ''), current.text);
  } else if (parsed.method === 'textDocument/didClose' && doc?.uri) {
    openLSPDocuments.delete(doc.uri);
  }
}

export async function SendLSPRequest(language: string, method: string, paramsJSON: string): Promise<string> {
  runningLSPServers.add(language);
  recordMockLSP('request', { language, method });
  const params = paramsJSON ? JSON.parse(paramsJSON) : {};
  const uri = params?.textDocument?.uri || '';
  const doc = uri ? openLSPDocuments.get(uri) : undefined;

  let result: unknown = null;
  switch (method) {
    case 'initialize':
      result = {
        capabilities: {
          textDocumentSync: { openClose: true, change: 1, save: { includeText: true } },
          completionProvider: { triggerCharacters: ['.', ':', '<', '"', "'", '/', '@', '#'] },
          hoverProvider: true,
          definitionProvider: true,
          referencesProvider: true,
          documentSymbolProvider: true,
          signatureHelpProvider: { triggerCharacters: ['(', ','] },
          semanticTokensProvider: {
            legend: {
              tokenTypes: ['namespace', 'type', 'class', 'enum', 'interface', 'struct', 'typeParameter', 'parameter', 'variable', 'property', 'enumMember', 'event', 'function', 'method', 'macro', 'keyword', 'modifier', 'comment', 'string', 'number', 'regexp', 'operator', 'decorator'],
              tokenModifiers: ['declaration', 'definition', 'readonly', 'static', 'deprecated', 'abstract', 'async', 'modification', 'documentation', 'defaultLibrary'],
            },
            full: true,
          },
        },
      };
      break;
    case 'textDocument/completion':
      result = {
        isIncomplete: false,
        items: [
          { label: 'preview', kind: 6, detail: 'browser-preview const', insertText: 'preview' },
          { label: 'toString', kind: 2, detail: 'mock LSP method', insertText: 'toString()' },
        ],
      };
      break;
    case 'textDocument/hover':
      result = {
        contents: {
          kind: 'markdown',
          value: `Mock ${language} hover for \`${uri.split('/').pop() || 'file'}\``,
        },
      };
      break;
    case 'textDocument/definition':
      result = [{
        uri,
        range: {
          start: { line: 2, character: 13 },
          end: { line: 2, character: 20 },
        },
      }];
      break;
    case 'textDocument/references':
      result = [{
        uri,
        range: {
          start: { line: 2, character: 13 },
          end: { line: 2, character: 20 },
        },
      }];
      break;
    case 'textDocument/documentSymbol':
      result = [{
        name: uri.split('/').pop() || 'PreviewFile',
        kind: 2,
        range: {
          start: { line: 0, character: 0 },
          end: { line: Math.max(0, (doc?.text || '').split('\n').length - 1), character: 0 },
        },
        selectionRange: {
          start: { line: 2, character: 13 },
          end: { line: 2, character: 20 },
        },
        children: [],
      }];
      break;
    case 'textDocument/signatureHelp':
      result = {
        signatures: [{
          label: 'toString(): string',
          documentation: 'Mock browser-preview signature help',
          parameters: [],
        }],
        activeSignature: 0,
        activeParameter: 0,
      };
      break;
    case 'textDocument/semanticTokens/full':
      result = { data: [] };
      break;
  }

  return JSON.stringify({ jsonrpc: '2.0', id: Date.now(), result });
}

function emitMockDiagnostics(language: string, uri: string, text: string): void {
  const diagnostics = text.includes('preview')
    ? [{
        range: {
          start: { line: 1, character: 3 },
          end: { line: 1, character: 18 },
        },
        severity: 3,
        source: 'browser-preview',
        message: 'Mock LSP diagnostics are wired.',
      }]
    : [];

  window.setTimeout(() => {
    recordMockLSP('diagnostics', { language, uri, count: diagnostics.length });
    emitDevEvent(`lsp:message:${language}`, JSON.stringify({
      jsonrpc: '2.0',
      method: 'textDocument/publishDiagnostics',
      params: { uri, diagnostics },
    }));
  }, 25);
}

export async function SearchContents(_workspacePath: string, query: string) {
  return query
    ? [
        { file: 'frontend/src/App.tsx', line: 42, content: `Mock result containing ${query}` },
        { file: 'frontend/src/components/Sidebar.tsx', line: 140, content: `Sidebar also mentions ${query}` },
      ]
    : [];
}

export async function SearchFiles(_workspacePath: string, query: string) {
  return query
    ? [
        { name: 'App.tsx', path: `${MAIN_WORKSPACE}/frontend/src/App.tsx`, isDir: false },
        { name: 'Sidebar.tsx', path: `${MAIN_WORKSPACE}/frontend/src/components/Sidebar.tsx`, isDir: false },
      ]
    : [];
}

export async function RevealInFinder(path: string): Promise<void> {
  console.info(`Reveal in Finder: ${path}`);
}

export async function GetMemorySnapshot() {
  return {
    totals: {
      orionMB: 180,
      webviewMB: 95,
      helpersMB: 12,
      sessionsMB: 64,
      grandMB: 351,
    },
    go: {
      heapAllocMB: 38,
      heapSysMB: 66,
      stackInUseMB: 4,
      sysMB: 112,
      numGC: 12,
      numGoroutine: 48,
    },
    fds: {
      count: 128,
      softLimit: 10240,
      hardLimit: 10240,
      usagePct: 1.25,
      byType: [],
      topEntries: [],
      groupedDirs: [],
      truncated: false,
    },
    sessions: [],
    helpers: [],
  };
}

export async function GetClipboard(): Promise<string> {
  return navigator.clipboard?.readText?.() ?? '';
}

export async function SetClipboard(text: string): Promise<void> {
  await navigator.clipboard?.writeText?.(text);
}

export async function LogClient(level: string, message: string): Promise<void> {
  if (level === 'error') console.error(message);
  else if (level === 'warn') console.warn(message);
  else console.info(message);
}

export async function LogPath(): Promise<string> {
  return 'browser-preview';
}

export async function NewWindow(): Promise<void> {}
export async function NewWindowWithProject(_projectPath: string): Promise<void> {}
export async function EmitSessionCreated(
  _tmuxSession: string,
  _sessionType: string,
  _label: string,
  _workspacePath: string,
): Promise<void> {}
export async function EmitSessionCreatedInfo(_session: unknown): Promise<void> {}
export async function EmitSessionKilled(_sessionID: string): Promise<void> {}
