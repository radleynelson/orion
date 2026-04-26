#!/usr/bin/env node

import { readFile } from 'node:fs/promises';
import path from 'node:path';
import readline from 'node:readline';
import {
  getSessionMessages,
  unstable_v2_createSession,
  unstable_v2_resumeSession,
} from '@anthropic-ai/claude-agent-sdk';

const args = parseArgs(process.argv.slice(2));

const workspacePath = requiredArg('workspace');
const label = args.label || 'Claude Chat';
const model = args.model || 'claude-opus-4-7';
const reasoningEffort = args.effort || 'xhigh';
const approvalPolicy = args['approval-policy'] || 'never';
const sandboxMode = args['sandbox-mode'] || 'danger-full-access';
const permissionMode = args['permission-mode'] || 'bypassPermissions';
const claudePath = args['claude-path'] || 'claude';
const resumeThreadId = args.resume || '';

let session = null;
let threadId = resumeThreadId;
let currentPermissionMode = permissionMode;
let stopRequested = false;
let running = false;
let lastAssistantText = '';
const seen = new Set();
const pendingAnswers = new Map();
let queued = Promise.resolve();
let activeTurn = null;
let queuedTurns = 0;
let shutdownResolve = null;
const shutdownPromise = new Promise((resolve) => {
  shutdownResolve = resolve;
});

main().catch((error) => {
  emit({ type: 'error', error: error instanceof Error ? error.message : String(error) });
  process.exit(1);
});

async function main() {
  session = resumeThreadId
    ? unstable_v2_resumeSession(resumeThreadId, sessionOptions())
    : unstable_v2_createSession(sessionOptions());

  emit({
    type: 'session',
    threadId,
    model,
    reasoningEffort,
    approvalPolicy,
    sandboxMode,
    permissionMode: currentPermissionMode,
    label,
  });

  startInputLoop();

  if (resumeThreadId) {
    await emitHistory(resumeThreadId);
  }

  await shutdownPromise;
}

function sessionOptions() {
  return {
    cwd: workspacePath,
    pathToClaudeCodeExecutable: claudePath,
    model,
    effort: reasoningEffort,
    permissionMode: currentPermissionMode,
    allowDangerouslySkipPermissions: true,
    settingSources: ['user', 'project', 'local'],
    allowedTools: ['Read', 'Grep', 'Glob', 'LS', 'Bash', 'Edit', 'MultiEdit', 'Write', 'NotebookRead', 'NotebookEdit', 'Task', 'WebFetch', 'WebSearch'],
    onElicitation: async (request) => {
      const toolUseId = request.elicitationId || `elicitation-${Date.now()}`;
      emitMessage({
        id: `claude-${toolUseId}:permission`,
        type: 'permission_request',
        toolUseId,
        toolName: request.display_name || request.title || 'Question',
        text: request.message || 'Claude needs input',
        details: compact({
          mode: request.mode,
          title: request.title,
          description: request.description,
          requestedSchema: request.requestedSchema,
        }),
      });
      return await new Promise((resolve) => {
        pendingAnswers.set(toolUseId, { resolve, request });
      });
    },
  };
}

async function consumeStream() {
  try {
    for await (const event of session.stream()) {
      handleStreamEvent(event);
      if (event.type === 'result') {
        running = false;
        emitStatus('idle');
      }
    }
  } catch (error) {
    emit({ type: 'error', error: error instanceof Error ? error.message : String(error) });
  } finally {
    activeTurn = null;
    if (!stopRequested && queuedTurns > 0) {
      queuedTurns -= 1;
      ensureTurnStream();
    }
  }
}

function ensureTurnStream() {
  if (activeTurn) {
    return activeTurn;
  }
  activeTurn = consumeStream();
  return activeTurn;
}

function handleStreamEvent(event) {
  if (event && typeof event.permissionMode === 'string' && event.permissionMode) {
    setLocalPermissionMode(event.permissionMode, 'Claude permission mode changed');
  }
  switch (event.type) {
    case 'system':
      if (event.subtype === 'init') {
        threadId = event.session_id || threadId;
        emitSessionMetadata(event.model || model, currentPermissionMode);
        emitStatus(running ? 'running' : 'idle', running ? 'Claude is thinking' : '');
      } else if (event.subtype === 'status') {
        if (running) {
          emitStatus('running', statusText(event));
        }
      } else if (event.subtype === 'session_state_changed') {
        if (event.state === 'running') {
          emitStatus('running', 'Claude is thinking');
        } else if (event.state === 'requires_action') {
          emitStatus('waiting_input', 'Waiting for your answer');
        } else if (event.state === 'idle') {
          emitStatus('idle');
        }
      }
      return;
    case 'assistant':
      for (const message of normalizeAssistantEvent(event)) {
        if (message.type === 'plan') {
          setLocalPermissionMode('plan', 'Claude entered plan mode');
          emitStatus('waiting_input', 'Waiting for plan approval');
        }
        emitMessage(message);
      }
      return;
    case 'user':
      for (const message of normalizeUserEvent(event)) {
        emitMessage(message);
      }
      return;
    case 'tool_progress':
      emitStatus('running', `Using ${event.tool_name || 'tool'}`);
      return;
    case 'result':
      emitMessage({
        id: `claude-${event.uuid}:result`,
        type: 'result',
        subtype: event.subtype,
        text: event.result || '',
        details: compact({
          stopReason: event.stop_reason,
          terminalReason: event.terminal_reason,
          totalCostUSD: event.total_cost_usd,
          deferredToolUse: event.deferred_tool_use,
        }),
      });
      return;
    default:
      return;
  }
}

function startInputLoop() {
  const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
  rl.on('line', (line) => {
    const text = line.trim();
    if (!text) {
      return;
    }
    let command;
    try {
      command = JSON.parse(text);
    } catch (error) {
      emit({ type: 'error', error: `invalid bridge command: ${error.message}` });
      return;
    }
    queued = queued.then(() => handleCommand(command)).catch((error) => {
      emit({ type: 'error', error: error instanceof Error ? error.message : String(error) });
    });
  });
  rl.on('close', async () => {
    stopRequested = true;
    if (session) {
      session.close();
    }
    emit({ type: 'stopped' });
    shutdownResolve?.();
  });
}

async function handleCommand(command) {
  switch (command.type) {
    case 'input':
      await sendInput(command.text || '', command.attachments || [], true);
      return;
    case 'continue':
      if (command.planApproved) {
        await setSessionPermissionMode('bypassPermissions');
      }
      await sendInput(command.text || '', command.attachments || [], false);
      return;
    case 'answer':
      await answerRequest(command.toolUseId || '', command.text || '');
      return;
    case 'stop':
      stopRequested = true;
      if (session) {
        session.close();
      }
      emit({ type: 'stopped' });
      shutdownResolve?.();
      return;
    default:
      throw new Error(`unsupported bridge command: ${command.type}`);
  }
}

async function sendInput(text, attachments, echoUser) {
  const payload = await buildInputPayload(text, attachments);
  if (activeTurn) {
    queuedTurns += 1;
  } else {
    ensureTurnStream();
  }
  if (echoUser) {
    emitMessage({
      id: `claude-user-${Date.now()}`,
      type: 'user',
      role: 'user',
      text: text.trim() || attachmentOnlyText(attachments.length),
      attachments,
    });
  }
  running = true;
  emitStatus('running', 'Claude is thinking');
  await session.send(payload);
}

async function answerRequest(toolUseId, text) {
  const pending = pendingAnswers.get(toolUseId);
  if (pending) {
    pendingAnswers.delete(toolUseId);
    pending.resolve(guessElicitationResponse(pending.request, text));
    emitMessage({
      id: `claude-${toolUseId}:answered`,
      type: 'permission_resolved',
      toolUseId,
      toolName: 'Question',
      text,
    });
    running = true;
    emitStatus('running', 'Claude is thinking');
    return;
  }
  await sendInput(text, [], true);
}

async function setSessionPermissionMode(mode) {
  if (!mode || currentPermissionMode === mode) {
    return;
  }
  if (session && typeof session.setPermissionMode === 'function') {
    await session.setPermissionMode(mode);
  }
  setLocalPermissionMode(mode, 'Claude permission mode changed');
}

function setLocalPermissionMode(mode, text) {
  if (!mode || currentPermissionMode === mode) {
    return;
  }
  currentPermissionMode = mode;
  emitSessionMetadata(model, currentPermissionMode);
  emitMessage({
    id: `claude-mode-${Date.now()}`,
    type: 'system',
    text,
    details: compact({
      provider: 'claude',
      viewMode: 'chat',
      threadId,
      model,
      reasoningEffort,
      approvalPolicy,
      sandboxMode,
      permissionMode: currentPermissionMode,
    }),
  });
}

function emitSessionMetadata(nextModel = model, nextPermissionMode = currentPermissionMode) {
  emit({
    type: 'session',
    threadId,
    model: nextModel,
    reasoningEffort,
    approvalPolicy,
    sandboxMode,
    permissionMode: nextPermissionMode,
    label,
  });
}

async function emitHistory(sessionId) {
  const history = await getSessionMessages(sessionId, { dir: workspacePath });
  for (const entry of history) {
    const messages = entry.type === 'assistant'
      ? normalizeAssistantEvent(entry)
      : normalizeUserEvent(entry);
    for (const message of messages) {
      emitMessage(message);
    }
  }
}

function normalizeAssistantEvent(event) {
  const content = Array.isArray(event?.message?.content) ? event.message.content : [];
  const messages = [];
  const texts = [];
  for (const block of content) {
    const blockType = normalize(block?.type);
    if (blockType === 'text') {
      const text = String(block.text || '').trim();
      if (text) {
        texts.push(text);
      }
      continue;
    }
    if (blockType === 'thinking') {
      const thinking = String(block.thinking || '').trim();
      if (thinking) {
        messages.push({
          id: `claude-${event.uuid}:thinking`,
          type: 'thinking_delta',
          text: thinking,
        });
      }
      continue;
    }
    if (blockType !== 'tooluse') {
      continue;
    }
    const toolUseId = String(block.id || `tool-${Date.now()}`);
    const toolName = String(block.name || 'Tool');
    const input = block.input && typeof block.input === 'object' ? block.input : {};
    if (normalize(toolName) === 'exitplanmode') {
      const plan = String(input.plan || '').trim();
      if (!plan) {
        continue;
      }
      messages.push({
        id: `claude-${toolUseId}:plan`,
        type: 'plan',
        subtype: 'waiting_approval',
        toolUseId,
        toolName,
        text: planTitle(plan),
        details: plan,
        planPath: String(input.planFilePath || input.plan_path || ''),
        status: 'waiting_approval',
      });
      continue;
    }
    if (normalize(toolName) === 'askuserquestion') {
      messages.push({
        id: `claude-${toolUseId}:question`,
        type: 'permission_request',
        toolUseId,
        toolName,
        text: extractQuestion(input),
        details: compact(input),
      });
      emitStatus('waiting_input', 'Waiting for your answer');
      continue;
    }
    if (isPlanPlumbingTool(toolName, input)) {
      continue;
    }
    messages.push({
      id: `claude-${toolUseId}:tool`,
      type: 'tool',
      toolUseId,
      toolName,
      text: toolName,
      details: compact(input),
    });
    emitStatus('running', `Using ${toolName}`);
  }
  if (texts.length) {
    const text = texts.join('');
    lastAssistantText = text;
    messages.push({
      id: `claude-${event.uuid}:assistant`,
      type: 'assistant',
      role: 'assistant',
      text,
    });
  }
  return messages;
}

function normalizeUserEvent(event) {
  const messages = [];
  const texts = [];
  const attachments = [];
  const rawContent = event?.message?.content;
  if (typeof rawContent === 'string') {
    const text = rawContent.trim();
    if (text) {
      messages.push({
        id: `claude-${event.uuid}:user`,
        type: 'user',
        role: 'user',
        text,
        attachments,
      });
    }
    return messages;
  }
  const content = Array.isArray(rawContent) ? rawContent : [];
  for (const block of content) {
    const blockType = normalize(block?.type);
    if (blockType === 'text') {
      const text = String(block.text || '').trim();
      if (text) {
        texts.push(text);
      }
      continue;
    }
    if (blockType === 'toolresult') {
      const text = summarizeToolResult(block.content);
      messages.push({
        id: `claude-${event.uuid}:${block.tool_use_id || 'tool-result'}`,
        type: 'tool_result',
        toolUseId: String(block.tool_use_id || ''),
        toolName: inferToolName(block.tool_use_id),
        text,
        details: compact(block.content),
      });
      continue;
    }
    if (blockType === 'image') {
      attachments.push({
        name: 'Image',
        path: '',
        mimeType: block.source?.media_type || block.media_type || '',
      });
    }
  }
  if (texts.length || attachments.length) {
    messages.push({
      id: `claude-${event.uuid}:user`,
      type: 'user',
      role: 'user',
      text: texts.join('\n\n'),
      attachments,
    });
  }
  return messages;
}

function extractQuestion(input) {
  return (
    String(input.question || '').trim() ||
    String(input.prompt || '').trim() ||
    String(input.message || '').trim() ||
    'Claude needs input'
  );
}

function summarizeToolResult(content) {
  if (typeof content === 'string') {
    const text = content.trim();
    if (!text) {
      return 'Tool completed';
    }
    return text.length > 240 ? `${text.slice(0, 237)}...` : text;
  }
  return 'Tool completed';
}

function inferToolName() {
  return 'Tool';
}

async function buildInputPayload(text, attachments) {
  const trimmed = text.trim();
  if (!attachments.length) {
    return trimmed;
  }
  const content = [];
  content.push({
    type: 'text',
    text: trimmed || attachmentOnlyText(attachments.length),
  });
  for (const attachment of attachments) {
    const image = await attachmentBlock(attachment);
    content.push(image);
  }
  return {
    role: 'user',
    content,
  };
}

async function attachmentBlock(attachment) {
  const filePath = String(attachment.path || '').trim();
  if (!filePath) {
    throw new Error('attachment path required');
  }
  const data = await readFile(filePath);
  return {
    type: 'image',
    source: {
      type: 'base64',
      media_type: attachment.mimeType || mimeTypeForPath(filePath),
      data: data.toString('base64'),
    },
  };
}

function guessElicitationResponse(request, text) {
  const trimmed = text.trim();
  if (!request?.requestedSchema || typeof request.requestedSchema !== 'object') {
    return { action: 'accept', content: { answer: trimmed } };
  }
  const properties = request.requestedSchema.properties;
  if (!properties || typeof properties !== 'object') {
    return { action: 'accept', content: { answer: trimmed } };
  }
  const keys = Object.keys(properties);
  if (keys.length === 1) {
    return { action: 'accept', content: { [keys[0]]: trimmed } };
  }
  const content = {};
  for (const key of keys) {
    content[key] = trimmed;
  }
  return { action: 'accept', content };
}

function statusText(event) {
  if (event.status === 'requesting') {
    return 'Claude is thinking';
  }
  if (event.status === 'compacting') {
    return 'Claude is compacting context';
  }
  return 'Claude is thinking';
}

function emitStatus(status, text = '') {
  emit({ type: 'status', status, text });
}

function emitMessage(message) {
  if (!message?.id || seen.has(message.id)) {
    return;
  }
  seen.add(message.id);
  emit({ type: 'message', message });
}

function emit(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

function compact(value) {
  if (value == null) {
    return '';
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function normalize(value) {
  return String(value || '').toLowerCase().replaceAll('_', '').replaceAll('-', '');
}

function isPlanPlumbingTool(toolName, input) {
  const normalized = normalize(toolName);
  if (normalized === 'toolsearch') {
    return String(input.query || '').trim() === 'select:ExitPlanMode';
  }
  if (normalized === 'write') {
    const target = String(input.file_path || input.filePath || '');
    return target.includes(`${path.sep}.claude${path.sep}plans${path.sep}`);
  }
  return false;
}

function planTitle(plan) {
  for (const rawLine of String(plan || '').split('\n')) {
    const line = rawLine.trim().replace(/^#+\s*/, '');
    if (line) {
      return line;
    }
  }
  return 'Plan ready';
}

function mimeTypeForPath(filePath) {
  switch (path.extname(filePath).toLowerCase()) {
    case '.png':
      return 'image/png';
    case '.gif':
      return 'image/gif';
    case '.webp':
      return 'image/webp';
    case '.heic':
      return 'image/heic';
    case '.heif':
      return 'image/heif';
    default:
      return 'image/jpeg';
  }
}

function attachmentOnlyText(count) {
  return count === 1 ? 'Please inspect the attached image.' : `Please inspect the ${count} attached images.`;
}

function parseArgs(argv) {
  const parsed = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith('--')) {
      continue;
    }
    const key = arg.slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith('--')) {
      parsed[key] = 'true';
      continue;
    }
    parsed[key] = next;
    i += 1;
  }
  return parsed;
}

function requiredArg(name) {
  const value = String(args[name] || '').trim();
  if (!value) {
    throw new Error(`missing required --${name}`);
  }
  return value;
}
