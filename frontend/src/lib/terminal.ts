import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { EventsOn, EventsEmit, BrowserOpenURL, ClipboardGetText, ClipboardSetText } from '../../wailsjs/runtime/runtime';

// Nocturne dark theme for xterm.js
const THEME = {
  background: '#131316',
  foreground: '#EAEAEC',
  cursor: '#EAEAEC',
  cursorAccent: '#131316',
  selectionBackground: 'rgba(124, 169, 247, 0.3)',
  selectionForeground: undefined,
  black: '#131316',
  red: '#E89180',
  green: '#8ACFA3',
  yellow: '#E6B86B',
  blue: '#7CA9F7',
  magenta: '#B9A3EC',
  cyan: '#9BC5FF',
  white: '#EAEAEC',
  brightBlack: '#6E6E78',
  brightRed: '#F2B6AA',
  brightGreen: '#BDE8CA',
  brightYellow: '#F1D497',
  brightBlue: '#AFCBFA',
  brightMagenta: '#D4C4F4',
  brightCyan: '#C5DCFF',
  brightWhite: '#ffffff',
};

export const TERMINAL_BASE_FONT_SIZE = 13;

export interface OrionTerminal {
  terminal: Terminal;
  fitAddon: FitAddon;
  dispose: () => void;
}

function clipboardTextFromSelection(selection: string, cols: number): string {
  const lines = selection.split('\n').map((line: string) => line.trimEnd());
  const joined: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const prevLine = joined.length > 0 ? joined[joined.length - 1] : '';
    if (joined.length > 0 && line.length > 0 && !line.startsWith(' ') && !line.startsWith('\t')) {
      const prevLen = prevLine.length;
      const nearFullWidth = prevLen >= cols - 2;
      const endsWithContinuation = /[^.\s:;,)}\]>]$/.test(prevLine);
      if (nearFullWidth || (endsWithContinuation && prevLen > 20)) {
        joined[joined.length - 1] = prevLine + line;
        continue;
      }
    }
    joined.push(line);
  }
  return joined.join('\n');
}

async function writeClipboardText(text: string): Promise<void> {
  try {
    const ok = await ClipboardSetText(text);
    if (ok) return;
  } catch {}

  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch {}

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', 'true');
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  textarea.style.top = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    document.execCommand('copy');
  } finally {
    textarea.remove();
  }
}

async function readClipboardText(): Promise<string> {
  try {
    return await ClipboardGetText();
  } catch {}

  return navigator.clipboard.readText();
}

let lastTerminalCopyText = '';
let lastTerminalCopyAt = 0;
let pendingTerminalClipboardWrite: Promise<void> | null = null;

function copyTerminalText(text: string): void {
  lastTerminalCopyText = text;
  lastTerminalCopyAt = Date.now();
  const write = writeClipboardText(text);
  pendingTerminalClipboardWrite = write;
  void write.finally(() => {
    if (pendingTerminalClipboardWrite === write) {
      pendingTerminalClipboardWrite = null;
    }
  });
}

async function readPasteClipboardText(): Promise<string> {
  const terminalCopyAge = Date.now() - lastTerminalCopyAt;
  const hasRecentTerminalCopy = lastTerminalCopyText.length > 0 && terminalCopyAge < 2500;
  if (hasRecentTerminalCopy && pendingTerminalClipboardWrite) {
    try {
      await pendingTerminalClipboardWrite;
    } catch {}
  }

  try {
    const text = await readClipboardText();
    return hasRecentTerminalCopy ? lastTerminalCopyText : text;
  } catch (error) {
    if (hasRecentTerminalCopy) return lastTerminalCopyText;
    throw error;
  }
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tagName = target.tagName.toLowerCase();
  return tagName === 'input' || tagName === 'textarea' || tagName === 'select';
}

export function createTerminal(
  container: HTMLElement,
  terminalId: string,
  fontSize: number = TERMINAL_BASE_FONT_SIZE,
): OrionTerminal {
  const terminal = new Terminal({
    theme: THEME,
    fontFamily: "'JetBrains Mono', 'Menlo', 'Monaco', 'Cascadia Code', monospace",
    fontSize,
    fontWeight: '400',
    fontWeightBold: '600',
    lineHeight: 1.3,
    letterSpacing: 0,
    cursorBlink: true,
    cursorStyle: 'bar',
    cursorWidth: 2,
    scrollback: 0, // tmux handles scrollback
    allowProposedApi: true,
    macOptionIsMeta: true, // true so Option+Arrow does word navigation
    macOptionClickForcesSelection: true, // keeps Option+click text selection working
  });

  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  // Unicode 11 support for proper emoji and wide character rendering
  const unicode11Addon = new Unicode11Addon();
  terminal.loadAddon(unicode11Addon);
  terminal.unicode.activeVersion = '11';

  terminal.open(container);

  // Try WebGL renderer for GPU-accelerated rendering
  try {
    const webglAddon = new WebglAddon();
    webglAddon.onContextLoss(() => {
      webglAddon.dispose();
    });
    terminal.loadAddon(webglAddon);
  } catch (e) {
    console.warn('WebGL addon failed, falling back to canvas renderer');
  }

  // Clickable links — Cmd+click opens in default browser.
  // We use a custom link provider instead of WebLinksAddon so that URLs
  // wrapping across terminal lines are fully clickable (WebLinksAddon
  // only matches within a single line).
  terminal.registerLinkProvider({
    provideLinks(rowNumber: number, callback: (links: Array<{range: {start: {x: number, y: number}, end: {x: number, y: number}}, text: string, activate: (e: MouseEvent, text: string) => void}> | undefined) => void) {
      // Gather the line at rowNumber and any continuation lines that follow
      // (a continuation line is one where the previous line used the full
      // terminal width, suggesting the text wrapped).
      const lines: Array<{text: string, row: number}> = [];
      let row = rowNumber;
      // Walk backwards to find where a wrapped sequence starts.
      // isWrapped on a line means "this line is a continuation of the line above",
      // so we check the CURRENT row's flag to decide whether to include the row above.
      while (row > 1) {
        const currentLine = terminal.buffer.active.getLine(row - 1);
        if (!currentLine || !currentLine.isWrapped) break;
        row--;
      }
      // Collect forward: start line + any continuation lines
      const startRow = row;
      const firstLine = terminal.buffer.active.getLine(startRow - 1);
      if (firstLine) {
        lines.push({ text: firstLine.translateToString(), row: startRow });
      }
      row = startRow + 1;
      while (row <= terminal.buffer.active.length) {
        const nextLine = terminal.buffer.active.getLine(row - 1);
        if (!nextLine || !nextLine.isWrapped) break;
        lines.push({ text: nextLine.translateToString(), row });
        row++;
      }

      const fullText = lines.map(l => l.text).join('');
      // Match URLs in the joined text
      const urlRegex = /https?:\/\/[^\s<>'")\]},;]+/g;
      let match: RegExpExecArray | null;
      const links: Array<{range: {start: {x: number, y: number}, end: {x: number, y: number}}, text: string, activate: (e: MouseEvent, text: string) => void}> = [];

      while ((match = urlRegex.exec(fullText)) !== null) {
        const urlStart = match.index;
        const urlEnd = urlStart + match[0].length;

        // Map character offsets back to row/col positions
        let charsSoFar = 0;
        let startX = 1, startY = startRow, endX = 1, endY = startRow;
        for (const line of lines) {
          const lineLen = line.text.length;
          if (charsSoFar + lineLen > urlStart && startX === 1 && startY === startRow && charsSoFar <= urlStart) {
            startX = urlStart - charsSoFar + 1;
            startY = line.row;
          }
          if (charsSoFar + lineLen >= urlEnd) {
            endX = urlEnd - charsSoFar;
            endY = line.row;
            break;
          }
          charsSoFar += lineLen;
        }

        // Only return links that intersect the requested row
        if (startY <= rowNumber && endY >= rowNumber) {
          links.push({
            range: {
              start: { x: startX, y: startY },
              end: { x: endX, y: endY },
            },
            text: match[0],
            activate: (e: MouseEvent, url: string) => {
              if (e.metaKey) BrowserOpenURL(url);
            },
          });
        }
      }

      callback(links.length > 0 ? links : undefined);
    },
  });

  fitAddon.fit();

  // Helper to send raw text/control sequences to the PTY.
  const sendData = (data: string) => {
    const bytes = new TextEncoder().encode(data);
    const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
    EventsEmit('terminal:input', terminalId, btoa(binary));
  };
  const sendSeq = (seq: string) => sendData(seq);

  const isVisible = () => {
    const rect = container.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0 && getComputedStyle(container).display !== 'none';
  };

  const isTerminalActive = (target?: EventTarget | null) => {
    if (!isVisible()) return false;
    if (target instanceof Node && container.contains(target)) return true;
    if (terminal.hasSelection()) return true;
    const activeElement = container.ownerDocument.activeElement;
    if (activeElement && container.contains(activeElement)) return true;
    return container.closest('.pane-focused') !== null;
  };

  const pasteText = (text: string) => {
    if (!text) return;
    terminal.focus();
    terminal.clearSelection();
    const normalized = text.replace(/\r?\n/g, '\r');
    const payload = terminal.modes.bracketedPasteMode
      ? `\x1b[200~${normalized}\x1b[201~`
      : normalized;
    sendData(payload);
  };

  let lastPasteHandledAt = 0;
  let pasteSuppressedUntil = 0;
  let pasteRequestToken = 0;
  let shiftKeyDown = false;
  const pasteFromSystemClipboard = async () => {
    const token = ++pasteRequestToken;
    pasteSuppressedUntil = Date.now() + 250;
    try {
      const text = await readPasteClipboardText();
      if (token !== pasteRequestToken) return;
      lastPasteHandledAt = Date.now();
      pasteText(text);
    } catch (error) {
      console.warn('Clipboard paste failed:', error);
    }
  };

  const keyCaptureHandler = (e: KeyboardEvent) => {
    if (e.key === 'Shift') {
      shiftKeyDown = true;
      return;
    }

    if (e.metaKey && !e.ctrlKey && !e.altKey && e.key.toLowerCase() === 'c' && isTerminalActive(e.target)) {
      if (isEditableTarget(e.target) && !(e.target instanceof Node && container.contains(e.target))) {
        return;
      }
      const selection = terminal.getSelection();
      if (selection) {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
        copyTerminalText(clipboardTextFromSelection(selection, terminal.cols));
        return;
      }
    }

    if (e.metaKey && !e.ctrlKey && !e.altKey && e.key.toLowerCase() === 'v' && isTerminalActive(e.target)) {
      if (isEditableTarget(e.target) && !(e.target instanceof Node && container.contains(e.target))) {
        return;
      }
      terminal.focus();
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      void pasteFromSystemClipboard();
      return;
    }

    if (
      e.key === 'Enter' &&
      (e.shiftKey || e.getModifierState('Shift') || shiftKeyDown) &&
      !e.metaKey &&
      !e.ctrlKey &&
      !e.altKey &&
      !e.isComposing &&
      isTerminalActive(e.target)
    ) {
      if (isEditableTarget(e.target) && !(e.target instanceof Node && container.contains(e.target))) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      sendSeq('\x1b[13;2u');
    }
  };
  container.addEventListener('keydown', keyCaptureHandler, { capture: true });

  const keyReleaseHandler = (e: KeyboardEvent) => {
    if (e.key === 'Shift') {
      shiftKeyDown = false;
    }
  };
  container.addEventListener('keyup', keyReleaseHandler, { capture: true });

  const blurHandler = () => {
    shiftKeyDown = false;
  };
  window.addEventListener('blur', blurHandler);

  // Handle keyboard shortcuts that the Wails webview doesn't route natively
  terminal.attachCustomKeyEventHandler((e: KeyboardEvent) => {
    if (e.type !== 'keydown') return true;

    if (e.key === 'Shift') {
      shiftKeyDown = true;
      return true;
    }

    if (e.metaKey && !e.ctrlKey && !e.altKey && e.key.toLowerCase() === 'c') {
      const selection = terminal.getSelection();
      if (selection) {
        e.preventDefault();
        e.stopPropagation();
        copyTerminalText(clipboardTextFromSelection(selection, terminal.cols));
        return false;
      }
    }

    if (e.metaKey && !e.ctrlKey && !e.altKey && e.key.toLowerCase() === 'v') {
      terminal.focus();
      void pasteFromSystemClipboard();
      return false;
    }

    // Fallback for Shift+Enter if the DOM capture listener misses it.
    if (e.key === 'Enter' && (e.shiftKey || e.getModifierState('Shift') || shiftKeyDown)) {
      e.preventDefault();
      e.stopPropagation();
      sendSeq('\x1b[13;2u');
      return false;
    }

    // Option+Arrow: word navigation
    if (e.altKey && !e.metaKey && !e.ctrlKey) {
      if (e.key === 'ArrowLeft') { sendSeq('\x1bb'); return false; }  // word backward
      if (e.key === 'ArrowRight') { sendSeq('\x1bf'); return false; } // word forward
      if (e.key === 'Backspace') { sendSeq('\x17'); return false; }   // delete word backward
    }

    return true;
  });

  // Clean up copied text: trim trailing whitespace and join wrapped lines.
  // Prevents random spaces in copied URLs and rejoins text that wraps
  // at the terminal edge.
  const copyHandler = (e: ClipboardEvent) => {
    const selection = terminal.getSelection();
    if (selection) {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      const text = clipboardTextFromSelection(selection, terminal.cols);
      e.clipboardData?.setData('text/plain', text);
      copyTerminalText(text);
    }
  };
  container.addEventListener('copy', copyHandler, { capture: true });

  const pasteHandler = (e: ClipboardEvent) => {
    if (!isTerminalActive(e.target)) return;
    if (isEditableTarget(e.target) && !(e.target instanceof Node && container.contains(e.target))) {
      return;
    }
    if (Date.now() < pasteSuppressedUntil || Date.now() - lastPasteHandledAt < 100) {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      return;
    }
    const text = e.clipboardData?.getData('text/plain') || '';
    if (!text) return;
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    lastPasteHandledAt = Date.now();
    pasteText(text);
  };
  container.addEventListener('paste', pasteHandler, { capture: true });

  const documentKeyCaptureHandler = (e: KeyboardEvent) => {
    keyCaptureHandler(e);
  };
  const documentKeyReleaseHandler = (e: KeyboardEvent) => {
    keyReleaseHandler(e);
  };
  const documentPasteHandler = (e: ClipboardEvent) => {
    if (e.target instanceof Node && container.contains(e.target)) return;
    pasteHandler(e);
  };
  container.ownerDocument.addEventListener('keydown', documentKeyCaptureHandler, { capture: true });
  container.ownerDocument.addEventListener('keyup', documentKeyReleaseHandler, { capture: true });
  container.ownerDocument.addEventListener('paste', documentPasteHandler, { capture: true });

  // Mouse scroll handling — sends SGR mouse sequences so tmux can scroll
  // in both alternate screen (TUI apps) and normal buffer (server logs).
  const el = container;

  let wheelAccumulator = 0;
  let lastWheelTime = 0;
  let lastScrollEmitTime = 0;
  let scrollFlushTimer: ReturnType<typeof setTimeout> | null = null;
  const scrollIdleResetMs = 180;
  const scrollEmitIntervalMs = 48;
  const minScrollPixelsPerEvent = 88;

  const normalizedWheelDelta = (e: WheelEvent, cellHeight: number, viewportHeight: number) => {
    if (e.deltaMode === WheelEvent.DOM_DELTA_LINE) return e.deltaY * cellHeight;
    if (e.deltaMode === WheelEvent.DOM_DELTA_PAGE) return e.deltaY * viewportHeight;
    return e.deltaY;
  };

  const emitScrollEvent = (direction: 1 | -1, col: number, row: number) => {
    const button = direction < 0 ? 64 : 65;
    const seq = `\x1b[<${button};${col};${row}M`;
    const bytes = new TextEncoder().encode(seq);
    const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
    EventsEmit('terminal:input', terminalId, btoa(binary));
  };

  const scheduleScrollFlush = (delay: number, col: number, row: number, threshold: number) => {
    if (scrollFlushTimer) return;
    scrollFlushTimer = setTimeout(() => {
      scrollFlushTimer = null;
      const magnitude = Math.abs(wheelAccumulator);
      if (magnitude < threshold) return;

      const now = Date.now();
      const elapsed = now - lastScrollEmitTime;
      if (elapsed < scrollEmitIntervalMs) {
        scheduleScrollFlush(scrollEmitIntervalMs - elapsed, col, row, threshold);
        return;
      }

      const direction: 1 | -1 = wheelAccumulator > 0 ? 1 : -1;
      emitScrollEvent(direction, col, row);
      wheelAccumulator -= direction * threshold;
      lastScrollEmitTime = now;

      if (Math.abs(wheelAccumulator) >= threshold) {
        scheduleScrollFlush(scrollEmitIntervalMs, col, row, threshold);
      }
    }, delay);
  };

  const wheelHandler = (e: WheelEvent) => {
    e.preventDefault();
    e.stopPropagation();

    const rect = el.getBoundingClientRect();
    const cellWidth = rect.width / terminal.cols;
    const cellHeight = rect.height / terminal.rows;
    const col = Math.min(terminal.cols, Math.max(1, Math.floor((e.clientX - rect.left) / cellWidth) + 1));
    const row = Math.min(terminal.rows, Math.max(1, Math.floor((e.clientY - rect.top) / cellHeight) + 1));

    const delta = normalizedWheelDelta(e, cellHeight, rect.height);
    const now = Date.now();
    if (
      now - lastWheelTime > scrollIdleResetMs ||
      (wheelAccumulator !== 0 && Math.sign(delta) !== Math.sign(wheelAccumulator))
    ) {
      wheelAccumulator = 0;
    }
    lastWheelTime = now;
    wheelAccumulator += delta;

    const threshold = Math.max(minScrollPixelsPerEvent, cellHeight * 5);
    if (Math.abs(wheelAccumulator) >= threshold) {
      scheduleScrollFlush(0, col, row, threshold);
    }
  };
  // Use capture phase so we intercept before xterm.js's internal handlers
  el.addEventListener('wheel', wheelHandler, { passive: false, capture: true });

  // Wire up input: terminal -> Go backend
  const onDataDispose = terminal.onData((data) => {
    const bytes = new TextEncoder().encode(data);
    const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
    const encoded = btoa(binary);
    EventsEmit('terminal:input', terminalId, encoded);
  });

  // Wire up output: Go backend -> terminal
  // Decode base64 to raw bytes, then write as Uint8Array for proper UTF-8
  const cancelOutput = EventsOn(`terminal:output:${terminalId}`, (encoded: string) => {
    try {
      const binary = atob(encoded);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
      }
      terminal.write(bytes);
    } catch {
      terminal.write(encoded);
    }
  });

  // Wire up exit events
  const cancelExit = EventsOn(`terminal:exit:${terminalId}`, () => {
    terminal.write('\r\n\x1b[90m[Process exited]\x1b[0m\r\n');
  });

  // Handle resize
  const onResizeDispose = terminal.onResize(({ cols, rows }) => {
    EventsEmit('terminal:resize', terminalId, cols, rows);
  });

  // Send initial size
  EventsEmit('terminal:resize', terminalId, terminal.cols, terminal.rows);

  const dispose = () => {
    if (scrollFlushTimer) clearTimeout(scrollFlushTimer);
    el.removeEventListener('wheel', wheelHandler, { capture: true } as any);
    container.removeEventListener('keydown', keyCaptureHandler, { capture: true } as any);
    container.removeEventListener('keyup', keyReleaseHandler, { capture: true } as any);
    container.removeEventListener('copy', copyHandler, { capture: true } as any);
    container.removeEventListener('paste', pasteHandler, { capture: true } as any);
    container.ownerDocument.removeEventListener('keydown', documentKeyCaptureHandler, { capture: true } as any);
    container.ownerDocument.removeEventListener('keyup', documentKeyReleaseHandler, { capture: true } as any);
    container.ownerDocument.removeEventListener('paste', documentPasteHandler, { capture: true } as any);
    window.removeEventListener('blur', blurHandler);
    onDataDispose.dispose();
    onResizeDispose.dispose();
    cancelOutput();
    cancelExit();
    terminal.dispose();
  };

  return { terminal, fitAddon, dispose };
}
