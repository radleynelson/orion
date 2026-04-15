import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
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

  // Helper to send a raw escape sequence to the PTY
  const sendSeq = (seq: string) => {
    const bytes = new TextEncoder().encode(seq);
    const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
    EventsEmit('terminal:input', terminalId, btoa(binary));
  };

  // Handle keyboard shortcuts that the Wails webview doesn't route natively
  terminal.attachCustomKeyEventHandler((e: KeyboardEvent) => {
    if (e.type !== 'keydown') return true;

    // Shift+Enter: send distinct escape sequence so Claude Code can
    // differentiate it from Enter (new line vs submit)
    if (e.key === 'Enter' && e.shiftKey) {
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

  let lastScrollTime = 0;
  const scrollThrottleMs = 16; // minimum ms between scroll events (~60fps)

  const wheelHandler = (e: WheelEvent) => {
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
    const lines = 1; // lines per throttled event
    for (let i = 0; i < lines; i++) {
      const seq = `\x1b[<${button};${col};${row}M`;
      const bytes = new TextEncoder().encode(seq);
      const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
      EventsEmit('terminal:input', terminalId, btoa(binary));
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
    container.removeEventListener('copy', copyHandler);
    onDataDispose.dispose();
    onResizeDispose.dispose();
    cancelOutput();
    cancelExit();
    terminal.dispose();
  };

  return { terminal, fitAddon, dispose };
}
