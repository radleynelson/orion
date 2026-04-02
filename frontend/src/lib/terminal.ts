import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { EventsOn, EventsEmit } from '../../wailsjs/runtime/runtime';

// Tokyo Night theme for xterm.js
const THEME = {
  background: '#13131e',
  foreground: '#c0caf5',
  cursor: '#c0caf5',
  cursorAccent: '#13131e',
  selectionBackground: 'rgba(122, 162, 247, 0.3)',
  selectionForeground: undefined,
  black: '#15161e',
  red: '#f7768e',
  green: '#9ece6a',
  yellow: '#e0af68',
  blue: '#7aa2f7',
  magenta: '#bb9af7',
  cyan: '#7dcfff',
  white: '#a9b1d6',
  brightBlack: '#414868',
  brightRed: '#f7768e',
  brightGreen: '#9ece6a',
  brightYellow: '#e0af68',
  brightBlue: '#7aa2f7',
  brightMagenta: '#bb9af7',
  brightCyan: '#7dcfff',
  brightWhite: '#c0caf5',
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
    scrollback: 10000,
    allowProposedApi: true,
    macOptionIsMeta: true,
    macOptionClickForcesSelection: true,
  });

  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

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

  // Wire up input: terminal -> Go backend
  const onDataDispose = terminal.onData((data) => {
    const encoded = btoa(data);
    EventsEmit('terminal:input', terminalId, encoded);
  });

  // Wire up output: Go backend -> terminal
  const cancelOutput = EventsOn(`terminal:output:${terminalId}`, (encoded: string) => {
    try {
      const decoded = atob(encoded);
      terminal.write(decoded);
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
    onDataDispose.dispose();
    onResizeDispose.dispose();
    cancelOutput();
    cancelExit();
    terminal.dispose();
  };

  return { terminal, fitAddon, dispose };
}
