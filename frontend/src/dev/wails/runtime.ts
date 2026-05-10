type EventCallback = (...data: unknown[]) => void;

const listeners = new Map<string, Set<EventCallback>>();

function callbacksFor(eventName: string): Set<EventCallback> {
  let callbacks = listeners.get(eventName);
  if (!callbacks) {
    callbacks = new Set();
    listeners.set(eventName, callbacks);
  }
  return callbacks;
}

export function emitDevEvent(eventName: string, ...data: unknown[]): void {
  const callbacks = listeners.get(eventName);
  if (!callbacks) return;
  for (const callback of [...callbacks]) {
    callback(...data);
  }
}

function decodeBase64Payload(payload: unknown): string {
  if (typeof payload !== 'string') return '';
  try {
    const binary = atob(payload);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return new TextDecoder().decode(bytes);
  } catch {
    return '';
  }
}

function encodeBase64Payload(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

function maybeEchoTerminalInput(eventName: string, data: unknown[]): void {
  if (eventName !== 'terminal:input') return;
  const terminalId = String(data[0] || '');
  if (!terminalId) return;
  const input = decodeBase64Payload(data[1]);
  if (!input || input === '\u0003') return;

  const printable = input
    .replace(/\x1b\[200~/g, '')
    .replace(/\x1b\[201~/g, '')
    .replace(/\r/g, '\r\n$ ')
    .trim();
  if (!printable) return;

  window.setTimeout(() => {
    emitDevEvent(
      `terminal:output:${terminalId}`,
      encodeBase64Payload(`${printable}\r\n\x1b[90m(browser preview mock)\x1b[0m\r\n$ `),
    );
  }, 25);
}

export function EventsOnMultiple(
  eventName: string,
  callback: EventCallback,
  maxCallbacks: number,
): () => void {
  let calls = 0;
  const wrapped: EventCallback = (...data) => {
    calls += 1;
    callback(...data);
    if (maxCallbacks > 0 && calls >= maxCallbacks) {
      callbacksFor(eventName).delete(wrapped);
    }
  };
  callbacksFor(eventName).add(wrapped);
  return () => callbacksFor(eventName).delete(wrapped);
}

export function EventsOn(eventName: string, callback: EventCallback): () => void {
  return EventsOnMultiple(eventName, callback, -1);
}

export function EventsOnce(eventName: string, callback: EventCallback): () => void {
  return EventsOnMultiple(eventName, callback, 1);
}

export function EventsOff(eventName: string, ...additionalEventNames: string[]): void {
  for (const name of [eventName, ...additionalEventNames]) {
    listeners.delete(name);
  }
}

export function EventsOffAll(): void {
  listeners.clear();
}

export function EventsEmit(eventName: string, ...data: unknown[]): void {
  maybeEchoTerminalInput(eventName, data);
  emitDevEvent(eventName, ...data);
}

export function BrowserOpenURL(url: string): void {
  window.open(url, '_blank', 'noopener,noreferrer');
}

export async function ClipboardGetText(): Promise<string> {
  return navigator.clipboard?.readText?.() ?? '';
}

export async function ClipboardSetText(text: string): Promise<boolean> {
  await navigator.clipboard?.writeText?.(text);
  return true;
}

export function LogPrint(message: string): void { console.log(message); }
export function LogTrace(message: string): void { console.debug(message); }
export function LogDebug(message: string): void { console.debug(message); }
export function LogInfo(message: string): void { console.info(message); }
export function LogWarning(message: string): void { console.warn(message); }
export function LogError(message: string): void { console.error(message); }
export function LogFatal(message: string): void { console.error(message); }
