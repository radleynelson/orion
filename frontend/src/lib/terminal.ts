import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { EventsOn, EventsEmit, BrowserOpenURL } from '../../wailsjs/runtime/runtime';

// Warp-inspired dark theme for xterm.js
const THEME = {
  background: '#1e1e1e',
  foreground: '#d4d4d4',
  cursor: '#d4d4d4',
  cursorAccent: '#1e1e1e',
  selectionBackground: 'rgba(108, 182, 255, 0.3)',
  selectionForeground: undefined,
  black: '#1e1e1e',
  red: '#ff7b72',
  green: '#7ee787',
  yellow: '#d29922',
  blue: '#6cb6ff',
  magenta: '#d2a8ff',
  cyan: '#76e3ea',
  white: '#d4d4d4',
  brightBlack: '#5a5a5a',
  brightRed: '#ffa198',
  brightGreen: '#90ee90',
  brightYellow: '#e3b341',
  brightBlue: '#79c0ff',
  brightMagenta: '#d8b4fe',
  brightCyan: '#a5f3fc',
  brightWhite: '#ffffff',
};

export const TERMINAL_BASE_FONT_SIZE = 13;

export interface OrionTerminal {
  terminal: Terminal;
  fitAddon: FitAddon;
  dispose: () => void;
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

  // Clickable links — Cmd+click opens in default browser
  terminal.loadAddon(new WebLinksAddon((e, url) => {
    if (e.metaKey) BrowserOpenURL(url);
  }));

  fitAddon.fit();

  // Helper to send a raw escape sequence to the PTY
  const sendSeq = (seq: string) => {
    const bytes = new TextEncoder().encode(seq);
    const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
    EventsEmit('terminal:input', terminalId, btoa(binary));
  };

  const keyCaptureHandler = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !e.isComposing) {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      sendSeq('\x1b[13;2u');
    }
  };
  container.addEventListener('keydown', keyCaptureHandler, { capture: true });

  // Handle keyboard shortcuts that the Wails webview doesn't route natively
  terminal.attachCustomKeyEventHandler((e: KeyboardEvent) => {
    if (e.type !== 'keydown') return true;

    // Fallback for Shift+Enter if the DOM capture listener misses it.
    if (e.key === 'Enter' && e.shiftKey) {
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
      const lines = selection.split('\n').map((line: string) => line.trimEnd());
      // Join lines that are likely wrapped (previous line is full-width
      // or doesn't end with a natural break, next line doesn't start with space)
      const joined: string[] = [];
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        const prevLine = joined.length > 0 ? joined[joined.length - 1] : '';
        // Join with previous line if:
        // - previous line exists and is close to terminal width (wrapped)
        // - OR previous line doesn't end with a natural sentence/command break
        // - AND current line doesn't start with whitespace (indented = new line)
        if (joined.length > 0 && line.length > 0 && !line.startsWith(' ') && !line.startsWith('\t')) {
          const prevLen = prevLine.length;
          const nearFullWidth = prevLen >= terminal.cols - 2;
          const endsWithContinuation = /[^.\s:;,)}\]>]$/.test(prevLine);
          if (nearFullWidth || (endsWithContinuation && prevLen > 20)) {
            joined[joined.length - 1] = prevLine + line;
            continue;
          }
        }
        joined.push(line);
      }
      e.clipboardData?.setData('text/plain', joined.join('\n'));
    }
  };
  container.addEventListener('copy', copyHandler);

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
    container.removeEventListener('copy', copyHandler);
    onDataDispose.dispose();
    onResizeDispose.dispose();
    cancelOutput();
    cancelExit();
    terminal.dispose();
  };

  return { terminal, fitAddon, dispose };
}
