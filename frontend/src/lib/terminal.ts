import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { EventsOn, EventsEmit } from '../../wailsjs/runtime/runtime';

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

export interface OrionTerminal {
  terminal: Terminal;
  fitAddon: FitAddon;
  dispose: () => void;
}

export function createTerminal(
  container: HTMLElement,
  terminalId: string,
): OrionTerminal {
  const terminal = new Terminal({
    theme: THEME,
    fontFamily: "'JetBrains Mono', 'Menlo', 'Monaco', 'Cascadia Code', monospace",
    fontSize: 13,
    fontWeight: '400',
    fontWeightBold: '600',
    lineHeight: 1.3,
    letterSpacing: 0,
    cursorBlink: true,
    cursorStyle: 'bar',
    cursorWidth: 2,
    scrollback: 0, // tmux handles scrollback; 0 prevents xterm.js from intercepting wheel events
    allowProposedApi: true,
    macOptionIsMeta: false, // false so Option+click works for text selection
    macOptionClickForcesSelection: true,
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

  fitAddon.fit();

  // Handle keyboard shortcuts that the Wails webview doesn't route natively
  terminal.attachCustomKeyEventHandler((e: KeyboardEvent) => {
    if (e.type !== 'keydown') return true;

    // Shift+Enter: send distinct escape sequence so Claude Code can
    // differentiate it from Enter (new line vs submit)
    if (e.key === 'Enter' && e.shiftKey) {
      const seq = '\x1b[13;2u';
      const bytes = new TextEncoder().encode(seq);
      const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
      EventsEmit('terminal:input', terminalId, btoa(binary));
      return false;
    }

    return true;
  });

  // Alternate screen mouse handling for TUI apps (Claude Code, Codex, vim, etc.)
  // - Wheel events: sent as SGR mouse sequences so the app can scroll
  // - Click/drag events: blocked from PTY so text selection works without
  //   the app snapping back to bottom on mousedown
  const el = container;

  let lastScrollTime = 0;
  const scrollThrottleMs = 50; // minimum ms between scroll events

  const wheelHandler = (e: WheelEvent) => {
    const buffer = terminal.buffer.active;
    if (buffer.type === 'alternate') {
      e.preventDefault();
      e.stopPropagation();

      // Throttle scroll events to control speed
      const now = Date.now();
      if (now - lastScrollTime < scrollThrottleMs) return;
      lastScrollTime = now;

      const rect = el.getBoundingClientRect();
      const cellWidth = rect.width / terminal.cols;
      const cellHeight = rect.height / terminal.rows;
      const col = Math.min(terminal.cols, Math.max(1, Math.floor((e.clientX - rect.left) / cellWidth) + 1));
      const row = Math.min(terminal.rows, Math.max(1, Math.floor((e.clientY - rect.top) / cellHeight) + 1));

      const button = e.deltaY < 0 ? 64 : 65;
      const lines = 1; // one line per throttled event
      for (let i = 0; i < lines; i++) {
        const seq = `\x1b[<${button};${col};${row}M`;
        const bytes = new TextEncoder().encode(seq);
        const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
        EventsEmit('terminal:input', terminalId, btoa(binary));
      }
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
    el.removeEventListener('wheel', wheelHandler, { capture: true } as any);
    onDataDispose.dispose();
    onResizeDispose.dispose();
    cancelOutput();
    cancelExit();
    terminal.dispose();
  };

  return { terminal, fitAddon, dispose };
}
