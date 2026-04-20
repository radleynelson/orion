import { useEffect, useMemo, useRef, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import {
	  AnswerClaudeChatRequest,
	  AnswerCodexChatRequest,
	  ApproveClaudePlan,
	  GetClaudeChatMessages,
	  GetCodexChatMessages,
	  OpenChatAttachmentDialog,
	  SendClaudeChatMessage,
	  SendCodexChatMessage,
	} from '../../wailsjs/go/main/App';

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
  planPath?: string;
  attachments?: ChatAttachment[];
  createdAt: string;
};

type ChatRow = ChatMessage & {
  merged?: boolean;
};

interface CodexChatProps {
  sessionId: string;
  visible: boolean;
  kind?: ChatKind;
}

type ChatKind = 'codex' | 'claude';

type ChatConfig = {
  displayName: string;
  avatar: string;
  eventPrefix: string;
  getMessages: (sessionID: string) => Promise<any>;
  sendMessage: (sessionID: string, text: string, attachments: ChatAttachment[]) => Promise<void>;
  answerRequest: (sessionID: string, toolUseID: string, result: string) => Promise<void>;
  approvePlan?: (sessionID: string) => Promise<void>;
  emptyText: string;
};

const CHAT_CONFIG: Record<ChatKind, ChatConfig> = {
  codex: {
    displayName: 'Codex',
    avatar: 'C',
    eventPrefix: 'codex-chat',
    getMessages: GetCodexChatMessages,
    sendMessage: SendCodexChatMessage,
    answerRequest: AnswerCodexChatRequest,
    emptyText: 'Ask Codex to inspect, edit, or explain this workspace.',
  },
  claude: {
    displayName: 'Claude',
    avatar: '◆',
    eventPrefix: 'claude-chat',
    getMessages: GetClaudeChatMessages,
    sendMessage: SendClaudeChatMessage,
    answerRequest: AnswerClaudeChatRequest,
    approvePlan: ApproveClaudePlan,
    emptyText: 'Ask Claude to inspect, edit, or explain this workspace.',
  },
};

export default function CodexChat({ sessionId, visible, kind = 'codex' }: CodexChatProps) {
  const config = CHAT_CONFIG[kind];
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [attachments, setAttachments] = useState<ChatAttachment[]>([]);
  const [expandedPlan, setExpandedPlan] = useState<ChatMessage | null>(null);
  const [approvingPlanId, setApprovingPlanId] = useState<string | null>(null);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const existing = await config.getMessages(sessionId);
        if (!cancelled) setMessages((existing || []) as ChatMessage[]);
      } catch (err) {
        console.error(`Failed to load ${config.displayName} chat messages:`, err);
      }
    })();

    const cancel = EventsOn(`${config.eventPrefix}:message:${sessionId}`, (msg: ChatMessage) => {
      setMessages((prev) => [...prev, msg]);
    });

    return () => {
      cancelled = true;
      cancel();
    };
  }, [sessionId, config]);

  useEffect(() => {
    if (!visible) return;
    const node = scrollerRef.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
  }, [messages, visible]);

  useEffect(() => {
    if (!visible) return;
    inputRef.current?.focus();
  }, [visible, sessionId]);

  const rows = useMemo(() => mergeRows(messages, config.displayName), [messages, config.displayName]);
  const lastStatusMessage = [...messages].reverse().find((m) => m.type === 'status');
  const lastStatus = lastStatusMessage?.status || 'idle';

  const send = async () => {
    const text = input.trim();
    if ((!text && attachments.length === 0) || sending) return;
    const nextAttachments = attachments;
    setInput('');
    setAttachments([]);
    setSending(true);
    try {
      await config.sendMessage(sessionId, text, nextAttachments);
    } catch (err) {
      setInput(text);
      setAttachments(nextAttachments);
      console.error(`Failed to send ${config.displayName} chat message:`, err);
      setMessages((prev) => [
        ...prev,
        {
          id: `local-error-${Date.now()}`,
          sessionId,
          type: 'error',
          text: err instanceof Error ? err.message : String(err),
          createdAt: new Date().toISOString(),
        },
      ]);
    } finally {
      setSending(false);
      inputRef.current?.focus();
    }
  };

  const chooseAttachments = async () => {
    try {
      const selected = (await OpenChatAttachmentDialog()) as ChatAttachment[];
      if (!selected?.length) return;
      setAttachments((prev) => {
        const seen = new Set(prev.map((item) => item.path));
        const merged = [...prev];
        for (const attachment of selected) {
          if (!attachment.path || seen.has(attachment.path)) continue;
          seen.add(attachment.path);
          merged.push(attachment);
        }
        return merged.slice(0, 6);
      });
    } catch (err) {
      console.error('Failed to attach image:', err);
    } finally {
      inputRef.current?.focus();
    }
  };

  const answer = async (toolUseId: string) => {
    const text = (answers[toolUseId] || '').trim();
    if (!text) return;
    setAnswers((prev) => ({ ...prev, [toolUseId]: '' }));
    try {
      await config.answerRequest(sessionId, toolUseId, text);
    } catch (err) {
      console.error(`Failed to answer ${config.displayName} chat request:`, err);
    }
  };

  const approvePlan = async (plan: ChatMessage) => {
    if (!config.approvePlan || approvingPlanId) return;
    setApprovingPlanId(plan.id);
    try {
      await config.approvePlan(sessionId);
      setExpandedPlan(null);
    } catch (err) {
      console.error(`Failed to approve ${config.displayName} plan:`, err);
      setMessages((prev) => [
        ...prev,
        {
          id: `local-plan-error-${Date.now()}`,
          sessionId,
          type: 'error',
          text: err instanceof Error ? err.message : String(err),
          createdAt: new Date().toISOString(),
        },
      ]);
    } finally {
      setApprovingPlanId(null);
    }
  };

  return (
    <div className="codex-chat">
      <div className="codex-chat-header">
          <div>
            <div className="codex-chat-title">{config.displayName} Chat</div>
          <div className="codex-chat-subtitle">{statusLabel(lastStatus, config.displayName, lastStatusMessage?.text)}</div>
        </div>
        <div className={`codex-chat-status codex-chat-status-${lastStatus}`}>{lastStatus}</div>
      </div>

      <div ref={scrollerRef} className="codex-chat-messages">
        <div className="codex-chat-thread">
          {rows.length === 0 && (
            <div className="codex-chat-empty">
              {config.emptyText}
            </div>
          )}
          {rows.map((msg) => renderRow(
            msg,
            answers,
            setAnswers,
            answer,
            config.displayName,
            config.avatar,
            setExpandedPlan,
            config.approvePlan ? approvePlan : undefined,
            approvingPlanId === msg.id,
          ))}
        </div>
      </div>

      {expandedPlan && (
        <PlanOverlay
          plan={expandedPlan}
          assistantName={config.displayName}
          onClose={() => setExpandedPlan(null)}
          onApprove={config.approvePlan ? () => approvePlan(expandedPlan) : undefined}
          approving={approvingPlanId === expandedPlan.id}
        />
      )}

      <div className="codex-chat-input">
        {attachments.length > 0 && (
          <div className="codex-chat-attachment-tray">
            {attachments.map((attachment, index) => (
              <AttachmentChip
                key={attachment.id || attachment.path || index}
                attachment={attachment}
                onRemove={() => setAttachments((prev) => prev.filter((_, i) => i !== index))}
              />
            ))}
          </div>
        )}
        <div className="codex-chat-input-row">
        <button
          type="button"
          className="codex-chat-icon-button"
          onClick={chooseAttachments}
          title="Attach image"
          aria-label="Attach image"
          disabled={sending}
        >
          +
        </button>
        <textarea
          ref={inputRef}
          value={input}
          placeholder={`Message ${config.displayName}...`}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              send();
            }
          }}
          rows={3}
        />
        <button onClick={send} disabled={sending || (!input.trim() && attachments.length === 0)}>
          {sending ? 'Sending' : 'Send'}
        </button>
        </div>
      </div>
    </div>
  );
}

function mergeRows(messages: ChatMessage[], assistantName: string): ChatRow[] {
  const rows: ChatRow[] = [];
  for (const msg of messages) {
    if (msg.type === 'status') continue;
    if (shouldHideMessage(msg)) continue;
    if (msg.type === 'stream_delta') {
      const last = rows[rows.length - 1];
      if (last?.type === 'assistant' && last.merged) {
        last.text = `${last.text || ''}${msg.text || ''}`;
      } else {
        rows.push({ ...msg, id: `assistant-stream-${msg.id}`, type: 'assistant', role: 'assistant', merged: true });
      }
      continue;
    }
    if (msg.type === 'thinking_delta') {
      const last = rows[rows.length - 1];
      if (last?.type === 'thinking_delta') {
        last.text = `${last.text || ''}${msg.text || ''}`;
      } else {
        rows.push({ ...msg });
      }
      continue;
    }
    rows.push({ ...msg });
  }
  const status = [...messages].reverse().find((msg) => msg.type === 'status');
  if (status?.status === 'running') {
    rows.push({
      id: `loading-${status.id}`,
      sessionId: status.sessionId,
      threadId: status.threadId,
      type: 'loading',
      text: status.text || `${assistantName} is thinking`,
      createdAt: status.createdAt,
    });
  }
  return rows;
}

function renderRow(
  msg: ChatRow,
  answers: Record<string, string>,
  setAnswers: Dispatch<SetStateAction<Record<string, string>>>,
  answer: (toolUseId: string) => Promise<void>,
  assistantName: string,
  avatar: string,
  expandPlan: (msg: ChatMessage) => void,
  approvePlan?: (msg: ChatMessage) => Promise<void>,
  approvingPlan?: boolean,
) {
  if (msg.type === 'plan') {
    return (
      <PlanCard
        key={msg.id}
        plan={msg}
        assistantName={assistantName}
        avatar={avatar}
        onReview={() => expandPlan(msg)}
        onApprove={approvePlan ? () => approvePlan(msg) : undefined}
        approving={approvingPlan}
      />
    );
  }
  if (msg.type === 'loading') {
    return <LoadingRow key={msg.id} msg={msg} assistantName={assistantName} avatar={avatar} />;
  }

  const kind = rowKind(msg.type);
  if (kind === 'activity') {
    return (
      <div key={msg.id} className={`codex-chat-activity codex-chat-activity-${activityClass(msg.type)}`}>
        <div className="codex-chat-activity-line">
          <span className="codex-chat-activity-dot" />
          <span className="codex-chat-activity-label">{activityLabel(msg)}</span>
          {msg.text && <span className="codex-chat-activity-text">{msg.text}</span>}
        </div>
        {msg.details && detailsBlock(msg.details)}
      </div>
    );
  }

  const isUser = msg.type === 'user';
  const isPermission = msg.type === 'permission_request';
  return (
    <div
      key={msg.id}
      className={`codex-chat-message ${isUser ? 'codex-chat-message-user' : 'codex-chat-message-assistant'}${isPermission ? ' codex-chat-message-permission' : ''}`}
    >
      {!isUser && <div className="codex-chat-avatar">{avatar}</div>}
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">{rowLabel(msg, assistantName)}</div>
        <div className="codex-chat-bubble">
          {msg.attachments?.length ? (
            <div className="codex-chat-attachments">
              {msg.attachments.map((attachment, index) => (
                <AttachmentPill key={attachment.id || attachment.path || index} attachment={attachment} />
              ))}
            </div>
          ) : null}
          {msg.text && <div className="codex-chat-text">{msg.text}</div>}
          {msg.details && detailsBlock(msg.details)}
          {msg.type === 'permission_request' && msg.toolUseId && (
            <div className="codex-chat-answer">
              <textarea
                value={answers[msg.toolUseId] || ''}
                onChange={(e) => setAnswers((prev) => ({ ...prev, [msg.toolUseId!]: e.target.value }))}
                placeholder={`Answer ${assistantName}...`}
                rows={2}
              />
              <button onClick={() => answer(msg.toolUseId!)}>Send Answer</button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function AttachmentChip({ attachment, onRemove }: { attachment: ChatAttachment; onRemove: () => void }) {
  return (
    <div className="codex-chat-attachment-chip">
      <span className="codex-chat-attachment-icon">img</span>
      <span className="codex-chat-attachment-name">{attachmentName(attachment)}</span>
      <button type="button" onClick={onRemove} aria-label={`Remove ${attachmentName(attachment)}`}>×</button>
    </div>
  );
}

function AttachmentPill({ attachment }: { attachment: ChatAttachment }) {
  return (
    <div className="codex-chat-attachment-pill">
      <span className="codex-chat-attachment-icon">img</span>
      <span>{attachmentName(attachment)}</span>
    </div>
  );
}

function attachmentName(attachment: ChatAttachment): string {
  if (attachment.name) return attachment.name;
  if (attachment.path) {
    const parts = attachment.path.split(/[\\/]/);
    return parts[parts.length - 1] || 'Image';
  }
  return 'Image';
}

function LoadingRow({ msg, assistantName, avatar }: { msg: ChatRow; assistantName: string; avatar: string }) {
  return (
    <div className="codex-chat-message codex-chat-message-assistant codex-chat-message-loading" key={msg.id}>
      <div className="codex-chat-avatar">{avatar}</div>
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">{assistantName}</div>
        <div className="codex-chat-loading-bubble">
          <span className="codex-chat-loading-text">{msg.text || `${assistantName} is thinking`}</span>
          <span className="codex-chat-typing-dots" aria-hidden="true">
            <span />
            <span />
            <span />
          </span>
        </div>
      </div>
    </div>
  );
}

function PlanCard({
  plan,
  assistantName,
  avatar,
  onReview,
  onApprove,
  approving,
}: {
  plan: ChatMessage;
  assistantName: string;
  avatar: string;
  onReview: () => void;
  onApprove?: () => void;
  approving?: boolean;
}) {
  const markdown = plan.details || plan.text || '';
  return (
    <div className="codex-chat-message codex-chat-message-assistant codex-chat-message-plan" key={plan.id}>
      <div className="codex-chat-avatar">{avatar}</div>
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">{assistantName} has a plan</div>
        <div className="codex-plan-card">
          <div className="codex-plan-card-header">
            <div>
              <div className="codex-plan-kicker">Waiting for approval</div>
              <div className="codex-plan-title">{plan.text || planTitle(markdown)}</div>
            </div>
            <span className="codex-plan-badge">Plan</span>
          </div>
          <div className="codex-plan-preview">{planPreview(markdown)}</div>
          <div className="codex-plan-actions">
            <button type="button" onClick={onReview}>Review</button>
            {onApprove && (
              <button type="button" className="codex-plan-primary" onClick={onApprove} disabled={approving}>
                {approving ? 'Approving' : 'Approve and run'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function PlanOverlay({
  plan,
  assistantName,
  onClose,
  onApprove,
  approving,
}: {
  plan: ChatMessage;
  assistantName: string;
  onClose: () => void;
  onApprove?: () => void;
  approving?: boolean;
}) {
  const markdown = plan.details || plan.text || '';
  return (
    <div className="codex-plan-overlay" role="dialog" aria-modal="true" aria-label={`${assistantName} plan`}>
      <div className="codex-plan-panel">
        <div className="codex-plan-panel-header">
          <div>
            <div className="codex-plan-kicker">{assistantName} plan</div>
            <div className="codex-plan-panel-title">{plan.text || planTitle(markdown)}</div>
          </div>
          <button type="button" className="codex-plan-close" onClick={onClose}>Minimize</button>
        </div>
        <pre className="codex-plan-markdown">{markdown}</pre>
        <div className="codex-plan-panel-footer">
          <button type="button" onClick={onClose}>Back to chat</button>
          {onApprove && (
            <button type="button" className="codex-plan-primary" onClick={onApprove} disabled={approving}>
              {approving ? 'Approving' : 'Approve and run'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function detailsBlock(details: string) {
  return (
    <details className="codex-chat-disclosure">
      <summary>Details</summary>
      <pre className="codex-chat-details">{formatDetails(details)}</pre>
    </details>
  );
}

function shouldHideMessage(msg: ChatMessage): boolean {
  if (msg.type === 'permission_resolved') return true;
  if (msg.type === 'plan_resolved') return true;
  if (msg.type === 'result') {
    const value = (msg.subtype || msg.text || '').toLowerCase();
    return value === '' || value === 'completed' || value === 'success' || value === 'ok';
  }
  if (msg.type === 'system') {
    const value = (msg.text || '').toLowerCase();
    return value.includes('codex chat ready') || value.includes('claude chat ready');
  }
  return false;
}

function rowKind(type: string): 'message' | 'activity' {
  switch (type) {
    case 'user':
    case 'assistant':
    case 'permission_request':
    case 'loading':
    case 'plan':
      return 'message';
    default:
      return 'activity';
  }
}

function activityClass(type: string): string {
  switch (type) {
    case 'tool':
    case 'tool_result':
      return 'tool';
    case 'thinking_delta':
      return 'thinking';
    case 'error':
      return 'error';
    default:
      return 'system';
  }
}

function activityLabel(msg: ChatMessage): string {
  switch (msg.type) {
    case 'tool':
      return msg.toolName ? `Using ${msg.toolName}` : 'Using tool';
    case 'tool_result':
      return msg.toolName ? `${msg.toolName} finished` : 'Tool finished';
    case 'thinking_delta':
      return 'Thinking';
    case 'result':
      return msg.subtype || 'Finished';
    case 'error':
      return 'Error';
    case 'system':
      return 'System';
    default:
      return msg.type;
  }
}

function rowLabel(msg: ChatMessage, assistantName: string): string {
  switch (msg.type) {
    case 'user': return 'You';
    case 'assistant': return assistantName;
    case 'tool': return msg.toolName || 'Tool';
    case 'tool_result': return `${msg.toolName || 'Tool'} result`;
    case 'permission_request': return msg.toolName || 'Question';
    case 'plan': return 'Plan';
    case 'thinking_delta': return 'Thinking';
    case 'result': return msg.subtype || 'Result';
    case 'error': return 'Error';
    case 'system': return 'System';
    default: return msg.type;
  }
}

function statusLabel(status: string, assistantName: string, text?: string): string {
  switch (status) {
    case 'starting': return `Starting ${assistantName}`;
    case 'running': return text || `${assistantName} is working`;
    case 'waiting_input': return 'Waiting for your answer';
    case 'stopped': return 'Session stopped';
    default: return 'Ready';
  }
}

function formatDetails(details: string): string {
  try {
    return JSON.stringify(JSON.parse(details), null, 2);
  } catch {
    return details;
  }
}

function planTitle(markdown: string): string {
  const line = markdown.split('\n').map((item) => item.trim()).find(Boolean);
  return line ? line.replace(/^#+\s*/, '') : 'Plan ready';
}

function planPreview(markdown: string): string {
  const lines = markdown
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'));
  return lines.slice(0, 4).join('\n') || 'Review the plan before Claude starts changing files.';
}
