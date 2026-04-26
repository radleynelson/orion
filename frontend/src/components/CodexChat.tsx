import { useEffect, useMemo, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import {
  AnswerClaudeChatRequest,
  AnswerCodexChatRequest,
  ApproveClaudePlan,
  ApproveCodexPlan,
  GetClaudeChatMessages,
  GetCodexChatMessages,
  OpenChatAttachmentDialog,
  SendClaudeChatMessage,
  SendCodexChatMessage,
} from "../../wailsjs/go/main/App";
import { useStore } from "../store";
import AgentSigil from "./AgentSigil";

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
  resultText?: string;
  resultDetails?: string;
  toolStatus?: "running" | "complete";
  permissionState?: "waiting" | "submitted" | "answered";
  answerText?: string;
  planState?: "waiting" | "approved";
};

type LiveActivityItem = {
  id: string;
  kind: "status" | "tool" | "reasoning" | "plan" | "stream" | "question";
  label: string;
  value?: string;
};

type SessionMetadata = {
  provider?: string;
  viewMode?: string;
  model?: string;
  reasoningEffort?: string;
  approvalPolicy?: string;
  sandboxMode?: string;
  permissionMode?: string;
  collaborationMode?: string;
  threadId?: string;
};

interface CodexChatProps {
  sessionId: string;
  visible: boolean;
  kind?: ChatKind;
}

type ChatKind = "codex" | "claude";

type ChatConfig = {
  displayName: string;
  avatar: string;
  sigil: string;
  eventPrefix: string;
  getMessages: (sessionID: string) => Promise<any>;
  sendMessage: (
    sessionID: string,
    text: string,
    attachments: ChatAttachment[],
  ) => Promise<void>;
  answerRequest: (
    sessionID: string,
    toolUseID: string,
    result: string,
  ) => Promise<void>;
  approvePlan?: (sessionID: string) => Promise<void>;
  emptyText: string;
};

const CHAT_CONFIG: Record<ChatKind, ChatConfig> = {
  codex: {
    displayName: "Codex",
    avatar: "C",
    sigil: "codex",
    eventPrefix: "codex-chat",
    getMessages: GetCodexChatMessages,
    sendMessage: SendCodexChatMessage,
    answerRequest: AnswerCodexChatRequest,
    approvePlan: ApproveCodexPlan,
    emptyText: "Ask Codex to inspect, edit, or explain this workspace.",
  },
  claude: {
    displayName: "Claude",
    avatar: "◆",
    sigil: "claude",
    eventPrefix: "claude-chat",
    getMessages: GetClaudeChatMessages,
    sendMessage: SendClaudeChatMessage,
    answerRequest: AnswerClaudeChatRequest,
    approvePlan: ApproveClaudePlan,
    emptyText: "Ask Claude to inspect, edit, or explain this workspace.",
  },
};

export default function CodexChat({
  sessionId,
  visible,
  kind = "codex",
}: CodexChatProps) {
  const config = CHAT_CONFIG[kind];
  const setCodeReviewVisible = useStore((s) => s.setCodeReviewVisible);
  const setCodeReviewBase = useStore((s) => s.setCodeReviewBase);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [submittedAnswers, setSubmittedAnswers] = useState<
    Record<string, string>
  >({});
  const [answerStates, setAnswerStates] = useState<
    Record<string, "submitting" | "submitted" | "error">
  >({});
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
        console.error(
          `Failed to load ${config.displayName} chat messages:`,
          err,
        );
      }
    })();

    const cancel = EventsOn(
      `${config.eventPrefix}:message:${sessionId}`,
      (msg: ChatMessage) => {
        setMessages((prev) => [...prev, msg]);
      },
    );

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

  const rows = useMemo(
    () => mergeRows(messages, config.displayName),
    [messages, config.displayName],
  );
  const metadata = useMemo(() => sessionMetadata(messages), [messages]);
  const lastStatusMessage = [...messages]
    .reverse()
    .find((m) => m.type === "status");
  const lastStatus = lastStatusMessage?.status || "idle";
  const isRunning = lastStatus === "running";
  const liveActivity = useMemo(
    () =>
      liveActivityItems(
        messages,
        rows,
        lastStatus,
        config.displayName,
        lastStatusMessage?.text,
      ),
    [messages, rows, lastStatus, lastStatusMessage?.text, config.displayName],
  );

  const send = async () => {
    const text = input.trim();
    if ((!text && attachments.length === 0) || sending) return;
    const nextAttachments = attachments;
    setInput("");
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
          type: "error",
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
      console.error("Failed to attach image:", err);
    } finally {
      inputRef.current?.focus();
    }
  };

  const answer = async (toolUseId: string) => {
    const text = (answers[toolUseId] || "").trim();
    if (!text) return;
    setAnswerStates((prev) => ({ ...prev, [toolUseId]: "submitting" }));
    try {
      await config.answerRequest(sessionId, toolUseId, text);
      setSubmittedAnswers((prev) => ({ ...prev, [toolUseId]: text }));
      setAnswers((prev) => {
        const next = { ...prev };
        delete next[toolUseId];
        return next;
      });
      setAnswerStates((prev) => ({ ...prev, [toolUseId]: "submitted" }));
    } catch (err) {
      console.error(
        `Failed to answer ${config.displayName} chat request:`,
        err,
      );
      setAnswerStates((prev) => ({ ...prev, [toolUseId]: "error" }));
      setMessages((prev) => [
        ...prev,
        {
          id: `local-answer-error-${Date.now()}`,
          sessionId,
          type: "error",
          text: err instanceof Error ? err.message : String(err),
          createdAt: new Date().toISOString(),
        },
      ]);
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
          type: "error",
          text: err instanceof Error ? err.message : String(err),
          createdAt: new Date().toISOString(),
        },
      ]);
    } finally {
      setApprovingPlanId(null);
    }
  };

  const reviewDiff = () => {
    setCodeReviewBase("uncommitted");
    setCodeReviewVisible(true);
  };

  return (
    <div className="codex-chat">
      <div className="codex-chat-header">
        <div className="codex-chat-header-main">
          <div className="codex-chat-heading">
            <AgentSigil id={config.sigil} size={34} strong />
            <div>
              <div className="codex-chat-title">{config.displayName} Chat</div>
              <div className="codex-chat-subtitle">
                {statusLabel(
                  lastStatus,
                  config.displayName,
                  lastStatusMessage?.text,
                )}
              </div>
            </div>
          </div>
          <div className={`codex-chat-status codex-chat-status-${lastStatus}`}>
            {statusLabelShort(lastStatus)}
          </div>
        </div>
        {metadata && <SessionModeStrip metadata={metadata} kind={kind} />}
        {liveActivity.length > 0 && <LiveActivityStrip items={liveActivity} />}
      </div>

      <div ref={scrollerRef} className="codex-chat-messages">
        <div className="codex-chat-thread">
          {rows.length === 0 && (
            <div className="codex-chat-empty">{config.emptyText}</div>
          )}
          {rows.map((msg) =>
            renderRow(
              msg,
              answers,
              answerStates,
              submittedAnswers,
              setAnswers,
              answer,
              config.displayName,
              config.sigil,
              setExpandedPlan,
              reviewDiff,
              config.approvePlan ? approvePlan : undefined,
              approvingPlanId === msg.id,
            ),
          )}
        </div>
      </div>

      {expandedPlan && (
        <PlanOverlay
          plan={expandedPlan}
          assistantName={config.displayName}
          onClose={() => setExpandedPlan(null)}
          onReviewDiff={() => {
            reviewDiff();
            setExpandedPlan(null);
          }}
          onApprove={
            config.approvePlan ? () => approvePlan(expandedPlan) : undefined
          }
          approving={approvingPlanId === expandedPlan.id}
        />
      )}

      <div className="codex-chat-input">
        {isRunning && (
          <WorkingIndicator
            text={lastStatusMessage?.text || `${config.displayName} is working`}
          />
        )}
        {attachments.length > 0 && (
          <div className="codex-chat-attachment-tray">
            {attachments.map((attachment, index) => (
              <AttachmentChip
                key={attachment.id || attachment.path || index}
                attachment={attachment}
                onRemove={() =>
                  setAttachments((prev) => prev.filter((_, i) => i !== index))
                }
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
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                send();
              }
            }}
            rows={3}
          />
          <button
            onClick={send}
            disabled={sending || (!input.trim() && attachments.length === 0)}
          >
            {sending ? "Sending" : "Send"}
          </button>
        </div>
      </div>
    </div>
  );
}

function mergeRows(messages: ChatMessage[], assistantName: string): ChatRow[] {
  const rows: ChatRow[] = [];
  for (const msg of messages) {
    if (msg.type === "status") continue;
    if (msg.type === "permission_submitted") {
      updatePermissionRow(rows, msg, "submitted");
      continue;
    }
    if (msg.type === "permission_resolved") {
      updatePermissionRow(rows, msg, "answered");
      continue;
    }
    if (msg.type === "plan_resolved") {
      updatePlanRow(rows);
      continue;
    }
    if (shouldHideMessage(msg)) continue;
    if (msg.type === "stream_delta") {
      const last = rows[rows.length - 1];
      if (last?.type === "assistant" && last.merged) {
        last.text = `${last.text || ""}${msg.text || ""}`;
      } else {
        rows.push({
          ...msg,
          id: `assistant-stream-${msg.id}`,
          type: "assistant",
          role: "assistant",
          merged: true,
        });
      }
      continue;
    }
    if (msg.type === "thinking_delta") {
      const last = rows[rows.length - 1];
      if (last?.type === "thinking_delta") {
        last.text = `${last.text || ""}${msg.text || ""}`;
      } else {
        rows.push({ ...msg });
      }
      continue;
    }
    if (msg.type === "tool_result") {
      const toolIndex = findOpenToolRow(rows, msg);
      if (toolIndex >= 0) {
        rows[toolIndex] = {
          ...rows[toolIndex],
          resultText: msg.text,
          resultDetails: msg.details,
          toolStatus: "complete",
        };
        continue;
      }
    }
    if (msg.type === "tool") {
      rows.push({ ...msg, toolStatus: "running" });
      continue;
    }
    if (msg.type === "permission_request") {
      rows.push({ ...msg, permissionState: "waiting" });
      continue;
    }
    if (msg.type === "plan") {
      rows.push({ ...msg, planState: "waiting" });
      continue;
    }
    rows.push({ ...msg });
  }
  return rows;
}

function updatePermissionRow(
  rows: ChatRow[],
  update: ChatMessage,
  state: "submitted" | "answered",
) {
  let fallbackIndex = -1;
  for (let i = rows.length - 1; i >= 0; i--) {
    const row = rows[i];
    if (row.type !== "permission_request") continue;
    if (fallbackIndex < 0 && row.permissionState !== "answered") {
      fallbackIndex = i;
    }
    if (update.toolUseId && row.toolUseId !== update.toolUseId) continue;
    const answerText =
      update.text && !isGenericPermissionAnswer(update.text)
        ? update.text
        : row.answerText;
    rows[i] = {
      ...row,
      permissionState: state,
      answerText,
    };
    return;
  }
  if (fallbackIndex >= 0) {
    const row = rows[fallbackIndex];
    const answerText =
      update.text && !isGenericPermissionAnswer(update.text)
        ? update.text
        : row.answerText;
    rows[fallbackIndex] = {
      ...row,
      permissionState: state,
      answerText,
    };
  }
}

function isGenericPermissionAnswer(text: string): boolean {
  const normalized = text.trim().toLowerCase();
  return normalized === "answered" || normalized === "answer submitted";
}

function updatePlanRow(rows: ChatRow[]) {
  for (let i = rows.length - 1; i >= 0; i--) {
    const row = rows[i];
    if (row.type !== "plan") continue;
    rows[i] = { ...row, planState: "approved" };
    return;
  }
}

function findOpenToolRow(rows: ChatRow[], result: ChatMessage): number {
  for (let i = rows.length - 1; i >= 0; i--) {
    const row = rows[i];
    if (row.type !== "tool") continue;
    if (row.resultText || row.resultDetails) continue;
    if (result.toolUseId && row.toolUseId === result.toolUseId) return i;
    if (!result.toolUseId && row.toolName === result.toolName) return i;
  }
  return -1;
}

function sessionMetadata(messages: ChatMessage[]): SessionMetadata | null {
  const planApproved = messages.some((msg) => msg.type === "plan_resolved");
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg.type !== "system" || !msg.details) continue;
    try {
      const parsed = JSON.parse(msg.details) as SessionMetadata;
      if (
        parsed.model ||
        parsed.reasoningEffort ||
        parsed.approvalPolicy ||
        parsed.sandboxMode ||
        parsed.permissionMode ||
        parsed.collaborationMode ||
        parsed.threadId
      ) {
        if (
          planApproved &&
          (parsed.permissionMode === "plan" || parsed.collaborationMode === "plan")
        ) {
          return { ...parsed, permissionMode: "approved" };
        }
        return parsed;
      }
    } catch {}
  }
  const threadId = [...messages]
    .reverse()
    .find((msg) => msg.threadId)?.threadId;
  return threadId ? { threadId } : null;
}

function liveActivityItems(
  messages: ChatMessage[],
  rows: ChatRow[],
  lastStatus: string,
  assistantName: string,
  statusText?: string,
): LiveActivityItem[] {
  const items: LiveActivityItem[] = [];
  if (lastStatus === "starting") {
    items.push({
      id: "status-starting",
      kind: "status",
      label: "Starting",
      value: assistantName,
    });
  } else if (lastStatus === "running") {
    items.push({
      id: "status-running",
      kind: "status",
      label: "Working",
      value: compactActivityValue(statusText || `${assistantName} is working`),
    });
  } else if (lastStatus === "waiting_input") {
    items.push({
      id: "status-waiting",
      kind: "question",
      label: "Waiting",
      value: "needs your input",
    });
  }

  const runningTools = rows.filter(
    (row) => row.type === "tool" && row.toolStatus === "running",
  );
  const latestTool = runningTools[runningTools.length - 1];
  if (latestTool) {
    items.push({
      id: `tool-${latestTool.id}`,
      kind: "tool",
      label: "Using",
      value: compactActivityValue(
        latestTool.toolName || latestTool.text || "tool",
      ),
    });
    if (runningTools.length > 1) {
      items.push({
        id: "tool-count",
        kind: "tool",
        label: `${runningTools.length} tools`,
        value: "active",
      });
    }
  }

  const lastVisibleRow = rows[rows.length - 1];
  const lastReasoning = [...rows]
    .reverse()
    .find((row) => row.type === "thinking_delta");
  if (
    lastReasoning &&
    (lastStatus === "running" || lastVisibleRow?.id === lastReasoning.id)
  ) {
    items.push({
      id: `reasoning-${lastReasoning.id}`,
      kind: "reasoning",
      label: "Reasoning",
      value: reasoningSummary(lastReasoning.text),
    });
  }

  if (
    lastStatus === "running" &&
    lastVisibleRow?.type === "assistant" &&
    lastVisibleRow.merged
  ) {
    items.push({
      id: `stream-${lastVisibleRow.id}`,
      kind: "stream",
      label: "Streaming",
      value: "answer",
    });
  }

  const planState = openPlanMessage(messages);
  if (planState) {
    items.push({
      id: `plan-${planState.id}`,
      kind: "plan",
      label: "Plan ready",
      value: compactActivityValue(
        planState.text || planTitle(planState.details || ""),
      ),
    });
  }

  return uniqueActivityItems(items).slice(0, 4);
}

function uniqueActivityItems(items: LiveActivityItem[]): LiveActivityItem[] {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = `${item.kind}:${item.label}:${item.value || ""}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function openPlanMessage(messages: ChatMessage[]): ChatMessage | null {
  let lastPlan: ChatMessage | null = null;
  let lastPlanIndex = -1;
  let resolvedIndex = -1;
  messages.forEach((msg, index) => {
    if (msg.type === "plan") {
      lastPlan = msg;
      lastPlanIndex = index;
    } else if (msg.type === "plan_resolved") {
      resolvedIndex = index;
    }
  });
  return lastPlan && lastPlanIndex > resolvedIndex ? lastPlan : null;
}

function compactActivityValue(value: string): string {
  const trimmed = value.replace(/\s+/g, " ").trim();
  return trimmed.length > 48 ? `${trimmed.slice(0, 45)}...` : trimmed;
}

function reasoningSummary(value?: string): string {
  if (!value?.trim()) return "thinking";
  const firstLine = value
    .split("\n")
    .map((line) => line.trim())
    .find(Boolean);
  return compactActivityValue(firstLine || "thinking");
}

function reasoningTitle(value?: string): string {
  const summary = reasoningSummary(value);
  return summary === "thinking" ? "Thinking through the plan" : summary;
}

function renderRow(
  msg: ChatRow,
  answers: Record<string, string>,
  answerStates: Record<string, "submitting" | "submitted" | "error">,
  submittedAnswers: Record<string, string>,
  setAnswers: Dispatch<SetStateAction<Record<string, string>>>,
  answer: (toolUseId: string) => Promise<void>,
  assistantName: string,
  agentId: string,
  expandPlan: (msg: ChatMessage) => void,
  reviewDiff: () => void,
  approvePlan?: (msg: ChatMessage) => Promise<void>,
  approvingPlan?: boolean,
) {
  if (msg.type === "plan") {
    return (
      <PlanCard
        key={msg.id}
        plan={msg}
        assistantName={assistantName}
        agentId={agentId}
        onReview={() => expandPlan(msg)}
        onReviewDiff={reviewDiff}
        onApprove={approvePlan ? () => approvePlan(msg) : undefined}
        approving={approvingPlan}
      />
    );
  }
  if (msg.type === "loading") {
    return (
      <LoadingRow
        key={msg.id}
        msg={msg}
        assistantName={assistantName}
        agentId={agentId}
      />
    );
  }
  if (msg.type === "tool") {
    return <ToolActivityCard key={msg.id} msg={msg} agentId={agentId} />;
  }
  if (msg.type === "thinking_delta") {
    return <ReasoningCard key={msg.id} msg={msg} agentId={agentId} />;
  }
  if (msg.type === "permission_request") {
    return (
      <PermissionCard
        key={msg.id}
        msg={msg}
        answers={answers}
        answerStates={answerStates}
        submittedAnswers={submittedAnswers}
        setAnswers={setAnswers}
        answer={answer}
        assistantName={assistantName}
        agentId={agentId}
      />
    );
  }

  const kind = rowKind(msg.type);
  if (kind === "activity") {
    return (
      <div
        key={msg.id}
        className={`codex-chat-activity codex-chat-activity-${activityClass(msg.type)}`}
      >
        <div className="codex-chat-activity-line">
          <span className="codex-chat-activity-dot" />
          <span className="codex-chat-activity-label">
            {activityLabel(msg)}
          </span>
          {msg.text && (
            <span className="codex-chat-activity-text">{msg.text}</span>
          )}
        </div>
        {msg.details && detailsBlock(msg.details)}
      </div>
    );
  }

  const isUser = msg.type === "user";
  return (
    <div
      key={msg.id}
      className={`codex-chat-message ${isUser ? "codex-chat-message-user" : "codex-chat-message-assistant"}`}
    >
      {!isUser && (
        <AgentSigil
          id={agentId}
          size={24}
          className="codex-chat-avatar-sigil"
        />
      )}
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">
          {rowLabel(msg, assistantName)}
        </div>
        <div className="codex-chat-bubble">
          {msg.attachments?.length ? (
            <div className="codex-chat-attachments">
              {msg.attachments.map((attachment, index) => (
                <AttachmentPill
                  key={attachment.id || attachment.path || index}
                  attachment={attachment}
                />
              ))}
            </div>
          ) : null}
          {msg.text && <div className="codex-chat-text">{msg.text}</div>}
          {msg.details && detailsBlock(msg.details)}
        </div>
      </div>
    </div>
  );
}

function SessionModeStrip({
  metadata,
  kind,
}: {
  metadata: SessionMetadata;
  kind: ChatKind;
}) {
  const items = [
    { label: "model", value: metadata.model },
    { label: "reasoning", value: metadata.reasoningEffort },
    { label: "approvals", value: approvalLabel(metadata.approvalPolicy, kind) },
    { label: "sandbox", value: sandboxLabel(metadata.sandboxMode) },
    {
      label: "mode",
      value: modeLabel(metadata.permissionMode || metadata.collaborationMode),
    },
  ].filter((item) => item.value);

  if (items.length === 0) return null;

  return (
    <div className="codex-chat-mode-strip">
      {items.map((item) => (
        <div className="codex-chat-mode-pill" key={item.label}>
          <span>{item.label}</span>
          <strong>{item.value}</strong>
        </div>
      ))}
    </div>
  );
}

function LiveActivityStrip({ items }: { items: LiveActivityItem[] }) {
  return (
    <div className="codex-chat-live-strip" aria-label="Live chat activity">
      {items.map((item) => (
        <div
          className={`codex-chat-live-pill codex-chat-live-pill-${item.kind}`}
          key={item.id}
        >
          <span className="codex-chat-live-dot" />
          <span className="codex-chat-live-label">{item.label}</span>
          {item.value && (
            <span className="codex-chat-live-value">{item.value}</span>
          )}
        </div>
      ))}
    </div>
  );
}

function approvalLabel(
  value: string | undefined,
  kind: ChatKind,
): string | undefined {
  if (!value) return kind === "claude" ? undefined : "full access";
  if (value === "never") return "full access";
  if (value === "on-request") return "ask first";
  return value.replaceAll("_", " ");
}

function sandboxLabel(value: string | undefined): string | undefined {
  if (!value) return undefined;
  if (value === "danger-full-access") return "workspace + network";
  return value.replaceAll("-", " ");
}

function modeLabel(value: string | undefined): string | undefined {
  if (!value) return undefined;
  if (value === "bypassPermissions") return "full access";
  if (value === "acceptEdits") return "accept edits";
  if (value === "dontAsk") return "don't ask";
  return value.replaceAll("_", " ").replaceAll("-", " ");
}

function AttachmentChip({
  attachment,
  onRemove,
}: {
  attachment: ChatAttachment;
  onRemove: () => void;
}) {
  return (
    <div className="codex-chat-attachment-chip">
      <span className="codex-chat-attachment-icon">img</span>
      <span className="codex-chat-attachment-name">
        {attachmentName(attachment)}
      </span>
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Remove ${attachmentName(attachment)}`}
      >
        ×
      </button>
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
    return parts[parts.length - 1] || "Image";
  }
  return "Image";
}

function WorkingIndicator({ text }: { text: string }) {
  return (
    <div className="codex-chat-working-indicator">
      <span className="codex-chat-typing-dots" aria-hidden="true">
        <span />
        <span />
        <span />
      </span>
      <span>{text}</span>
    </div>
  );
}

function LoadingRow({
  msg,
  assistantName,
  agentId,
}: {
  msg: ChatRow;
  assistantName: string;
  agentId: string;
}) {
  return (
    <div
      className="codex-chat-message codex-chat-message-assistant codex-chat-message-loading"
      key={msg.id}
    >
      <AgentSigil id={agentId} size={24} className="codex-chat-avatar-sigil" />
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">{assistantName}</div>
        <div className="codex-chat-loading-bubble">
          <span className="codex-chat-loading-text">
            {msg.text || `${assistantName} is thinking`}
          </span>
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

function ToolActivityCard({ msg, agentId }: { msg: ChatRow; agentId: string }) {
  const complete = msg.toolStatus === "complete";
  const output = msg.resultText || msg.resultDetails || "";
  const commandLike = toolLooksLikeCommand(msg.toolName);

  return (
    <div className="codex-chat-message codex-chat-message-assistant codex-chat-message-tool">
      <AgentSigil id={agentId} size={24} className="codex-chat-avatar-sigil" />
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">
          {complete ? "Tool finished" : "Tool running"}
        </div>
        <div className="codex-tool-card">
          <div className="codex-tool-card-header">
            <div className="codex-tool-title">
              <span className="codex-tool-icon">{toolIcon(msg.toolName)}</span>
              <div>
                <div className="codex-tool-name">{msg.toolName || "Tool"}</div>
                {msg.toolUseId && (
                  <div className="codex-tool-id">{shortID(msg.toolUseId)}</div>
                )}
              </div>
            </div>
            <span
              className={`codex-tool-status ${complete ? "complete" : "running"}`}
            >
              {complete ? "complete" : "running"}
            </span>
          </div>
          {msg.text && (
            <pre
              className={commandLike ? "codex-tool-command" : "codex-tool-text"}
            >
              {msg.text}
            </pre>
          )}
          {msg.details &&
            detailsBlock(msg.details, commandLike ? "Input" : "Details")}
          {output && (
            <details className="codex-tool-output" open={!commandLike}>
              <summary>{commandLike ? "Output" : "Result"}</summary>
              <pre>{formatDetails(output)}</pre>
            </details>
          )}
        </div>
      </div>
    </div>
  );
}

function ReasoningCard({ msg, agentId }: { msg: ChatRow; agentId: string }) {
  return (
    <div className="codex-chat-message codex-chat-message-assistant codex-chat-message-reasoning">
      <AgentSigil id={agentId} size={24} className="codex-chat-avatar-sigil" />
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">Reasoning</div>
        <details className="codex-reasoning-card">
          <summary>
            <span className="codex-reasoning-dot" />
            <span>{reasoningTitle(msg.text)}</span>
          </summary>
          <div>{msg.text || "Reasoning in progress."}</div>
        </details>
      </div>
    </div>
  );
}

function PermissionCard({
  msg,
  answers,
  answerStates,
  submittedAnswers,
  setAnswers,
  answer,
  assistantName,
  agentId,
}: {
  msg: ChatRow;
  answers: Record<string, string>;
  answerStates: Record<string, "submitting" | "submitted" | "error">;
  submittedAnswers: Record<string, string>;
  setAnswers: Dispatch<SetStateAction<Record<string, string>>>;
  answer: (toolUseId: string) => Promise<void>;
  assistantName: string;
  agentId: string;
}) {
  const prompt = permissionPrompt(msg);
  const toolUseId = msg.toolUseId || "";
  const localState = toolUseId ? answerStates[toolUseId] : undefined;
  const state =
    msg.permissionState === "answered"
      ? "answered"
      : localState || msg.permissionState || "waiting";
  const waiting = state === "waiting" || state === "error";
  const disabled =
    state === "submitting" || state === "submitted" || state === "answered";
  const resolved = state === "submitted" || state === "answered";
  const answerText = toolUseId
    ? msg.answerText || submittedAnswers[toolUseId] || msg.resultText || ""
    : msg.answerText || msg.resultText || "";
  const statusLabel = permissionStatusLabel(state);
  const questionText = msg.text || prompt;
  return (
    <div
      className={`codex-chat-message codex-chat-message-assistant codex-chat-message-permission codex-chat-message-permission-${state}`}
    >
      <AgentSigil id={agentId} size={24} className="codex-chat-avatar-sigil" />
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">
          {resolved
            ? `${assistantName} answered`
            : `${assistantName} needs input`}
        </div>
        <div className="codex-permission-card">
          <div className="codex-permission-header">
            <span className="codex-permission-icon">
              {resolved ? "✓" : "?"}
            </span>
            <div>
              <div className="codex-permission-title">
                {msg.toolName || "Question"}
              </div>
              {!resolved && (
                <div className="codex-permission-subtitle">
                  {questionText || "The session is waiting for your answer."}
                </div>
              )}
            </div>
            <span
              className={`codex-permission-state codex-permission-state-${state}`}
            >
              {statusLabel}
            </span>
          </div>
          {waiting && <div className="codex-permission-prompt">{prompt}</div>}
          {resolved && (
            <div className="codex-permission-resolved">
              {questionText && (
                <div className="codex-permission-qa">
                  <div className="codex-permission-qa-label">Question</div>
                  <div className="codex-permission-qa-text">{questionText}</div>
                </div>
              )}
              <div className="codex-permission-qa">
                <div className="codex-permission-qa-label">Your answer</div>
                <div className="codex-permission-qa-text codex-permission-qa-answer">
                  {answerText ||
                    (state === "answered"
                      ? "Answer delivered."
                      : "Waiting for the session to continue.")}
                </div>
              </div>
            </div>
          )}
          {toolUseId && waiting && (
            <div className="codex-chat-answer">
              <textarea
                value={answers[toolUseId] || ""}
                onChange={(e) =>
                  setAnswers((prev) => ({
                    ...prev,
                    [toolUseId]: e.target.value,
                  }))
                }
                placeholder={`Answer ${assistantName}...`}
                disabled={disabled}
                rows={2}
              />
              <button
                onClick={() => answer(toolUseId)}
                disabled={disabled || !(answers[toolUseId] || "").trim()}
              >
                {state === "error" ? "Retry Answer" : "Send Answer"}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function PlanCard({
  plan,
  assistantName,
  agentId,
  onReview,
  onReviewDiff,
  onApprove,
  approving,
}: {
  plan: ChatRow;
  assistantName: string;
  agentId: string;
  onReview: () => void;
  onReviewDiff: () => void;
  onApprove?: () => void;
  approving?: boolean;
}) {
  const markdown = plan.details || plan.text || "";
  const insights = planInsights(markdown);
  const sections = planSections(markdown);
  const approved = plan.planState === "approved";
  return (
    <div
      className="codex-chat-message codex-chat-message-assistant codex-chat-message-plan"
      key={plan.id}
    >
      <AgentSigil id={agentId} size={24} className="codex-chat-avatar-sigil" />
      <div className="codex-chat-message-stack">
        <div className="codex-chat-message-meta">
          {assistantName} has a plan
        </div>
        <div className="codex-plan-card">
          <div className="codex-plan-card-header">
            <div>
              <div className="codex-plan-kicker">
                Plan · {approved ? "approved" : "waiting for you"}
              </div>
              <div className="codex-plan-title">
                {plan.text || planTitle(markdown)}
              </div>
            </div>
            <div className="codex-plan-badges">
              <span className="codex-plan-badge">
                {approved ? "Approved" : "Plan"}
              </span>
              {insights.sections > 0 && (
                <span className="codex-plan-badge muted">
                  {insights.sections} sections
                </span>
              )}
              {insights.steps > 0 && (
                <span className="codex-plan-badge muted">
                  {insights.steps} steps
                </span>
              )}
            </div>
          </div>
          {sections.length > 0 && (
            <div className="codex-plan-section-row">
              {sections.slice(0, 4).map((section) => (
                <span key={section}>{section}</span>
              ))}
            </div>
          )}
          <PlanPreview markdown={markdown} />
          <div className="codex-plan-actions">
            <button type="button" onClick={onReview}>
              Review plan
            </button>
            <button type="button" onClick={onReviewDiff}>
              Review diff
            </button>
            {approved ? (
              <span className="codex-plan-approved">Plan approved</span>
            ) : (
              onApprove && (
                <button
                  type="button"
                  className="codex-plan-primary"
                  onClick={onApprove}
                  disabled={approving}
                >
                  {approving ? "Approving" : "Approve & run"}
                </button>
              )
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
  onReviewDiff,
  onApprove,
  approving,
}: {
  plan: ChatMessage;
  assistantName: string;
  onClose: () => void;
  onReviewDiff: () => void;
  onApprove?: () => void;
  approving?: boolean;
}) {
  const markdown = plan.details || plan.text || "";
  const insights = planInsights(markdown);
  return (
    <div
      className="codex-plan-overlay"
      role="dialog"
      aria-modal="true"
      aria-label={`${assistantName} plan`}
    >
      <div className="codex-plan-panel">
        <div className="codex-plan-panel-header">
          <div>
            <div className="codex-plan-kicker">{assistantName} plan</div>
            <div className="codex-plan-panel-title">
              {plan.text || planTitle(markdown)}
            </div>
            <div className="codex-plan-panel-meta">
              <span>{insights.sections} sections</span>
              <span>{insights.steps} steps</span>
              {plan.planPath && <span>{plan.planPath}</span>}
            </div>
          </div>
          <button type="button" className="codex-plan-close" onClick={onClose}>
            Minimize
          </button>
        </div>
        <pre className="codex-plan-markdown">{markdown}</pre>
        <div className="codex-plan-panel-footer">
          <button type="button" onClick={onClose}>
            Back to chat
          </button>
          <button type="button" onClick={onReviewDiff}>
            Review diff
          </button>
          {onApprove && (
            <button
              type="button"
              className="codex-plan-primary"
              onClick={onApprove}
              disabled={approving}
            >
              {approving ? "Approving" : "Approve & run"}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function detailsBlock(details: string, label = "Details") {
  return (
    <details className="codex-chat-disclosure">
      <summary>{label}</summary>
      <pre className="codex-chat-details">{formatDetails(details)}</pre>
    </details>
  );
}

function shouldHideMessage(msg: ChatMessage): boolean {
  if (msg.type === "permission_resolved") return true;
  if (msg.type === "permission_submitted") return true;
  if (msg.type === "plan_resolved") return true;
  if (msg.type === "result") {
    const value = (msg.subtype || msg.text || "").toLowerCase();
    return (
      value === "" ||
      value === "completed" ||
      value === "success" ||
      value === "ok"
    );
  }
  if (msg.type === "system") {
    const value = (msg.text || "").toLowerCase();
    return (
      value.includes("codex chat ready") || value.includes("claude chat ready")
    );
  }
  return false;
}

function rowKind(type: string): "message" | "activity" {
  switch (type) {
    case "user":
    case "assistant":
    case "permission_request":
    case "loading":
    case "plan":
    case "tool":
    case "thinking_delta":
      return "message";
    default:
      return "activity";
  }
}

function activityClass(type: string): string {
  switch (type) {
    case "tool":
    case "tool_result":
      return "tool";
    case "thinking_delta":
      return "thinking";
    case "error":
      return "error";
    default:
      return "system";
  }
}

function activityLabel(msg: ChatMessage): string {
  switch (msg.type) {
    case "tool":
      return msg.toolName ? `Using ${msg.toolName}` : "Using tool";
    case "tool_result":
      return msg.toolName ? `${msg.toolName} finished` : "Tool finished";
    case "thinking_delta":
      return "Thinking";
    case "result":
      return msg.subtype || "Finished";
    case "error":
      return "Error";
    case "system":
      return "System";
    default:
      return msg.type;
  }
}

function rowLabel(msg: ChatMessage, assistantName: string): string {
  switch (msg.type) {
    case "user":
      return "You";
    case "assistant":
      return assistantName;
    case "tool":
      return msg.toolName || "Tool";
    case "tool_result":
      return `${msg.toolName || "Tool"} result`;
    case "permission_request":
      return msg.toolName || "Question";
    case "plan":
      return "Plan";
    case "thinking_delta":
      return "Thinking";
    case "result":
      return msg.subtype || "Result";
    case "error":
      return "Error";
    case "system":
      return "System";
    default:
      return msg.type;
  }
}

function statusLabel(
  status: string,
  assistantName: string,
  text?: string,
): string {
  switch (status) {
    case "starting":
      return `Starting ${assistantName}`;
    case "running":
      return text || `${assistantName} is working`;
    case "waiting_input":
      return "Waiting for your answer";
    case "stopped":
      return "Session stopped";
    default:
      return "Ready";
  }
}

function statusLabelShort(status: string): string {
  switch (status) {
    case "waiting_input":
      return "needs input";
    case "running":
      return "running";
    case "starting":
      return "starting";
    case "stopped":
      return "stopped";
    default:
      return "ready";
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
  const line = markdown
    .split("\n")
    .map((item) => item.trim())
    .find(Boolean);
  return line ? line.replace(/^#+\s*/, "") : "Plan ready";
}

function planPreview(markdown: string): string {
  const lines = markdown
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith("#"));
  return (
    lines.slice(0, 4).join("\n") || "Review the plan before changes start."
  );
}

function PlanPreview({ markdown }: { markdown: string }) {
  const lines = planPreview(markdown).split("\n").filter(Boolean);
  return (
    <div className="codex-plan-preview-list">
      {lines.map((line, index) => (
        <div className="codex-plan-preview-line" key={`${line}-${index}`}>
          <span>{index + 1}</span>
          <p>{line.replace(/^[-*]\s*/, "").replace(/^\d+\.\s*/, "")}</p>
        </div>
      ))}
    </div>
  );
}

function planInsights(markdown: string): { sections: number; steps: number } {
  const lines = markdown
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  return {
    sections: lines.filter((line) => /^#{1,4}\s+/.test(line)).length,
    steps: lines.filter((line) => /^([-*]|\d+\.)\s+/.test(line)).length,
  };
}

function planSections(markdown: string): string[] {
  return markdown
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => /^#{1,4}\s+/.test(line))
    .map((line) => line.replace(/^#{1,4}\s+/, "").trim())
    .filter(Boolean);
}

function toolLooksLikeCommand(toolName?: string): boolean {
  const value = (toolName || "").toLowerCase();
  return (
    value.includes("bash") ||
    value.includes("command") ||
    value.includes("shell")
  );
}

function toolIcon(toolName?: string): string {
  const value = (toolName || "").toLowerCase();
  if (value.includes("bash") || value.includes("command")) return "$";
  if (value.includes("file")) return "±";
  if (value.includes("web")) return "⌕";
  if (value.includes("mcp")) return "◆";
  return "∴";
}

function permissionPrompt(msg: ChatMessage): string {
  if (msg.details) return msg.details;
  if (msg.text) return msg.text;
  return "Choose how Orion should continue.";
}

function permissionStatusLabel(
  state: "waiting" | "submitting" | "submitted" | "answered" | "error",
): string {
  switch (state) {
    case "submitting":
      return "sending";
    case "submitted":
      return "submitted";
    case "answered":
      return "answered";
    case "error":
      return "retry needed";
    default:
      return "waiting";
  }
}

function shortID(id: string): string {
  return id.length > 12 ? `${id.slice(0, 6)}…${id.slice(-4)}` : id;
}
