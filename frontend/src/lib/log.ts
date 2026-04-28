// Frontend logging helpers that mirror console output to the persistent
// ~/.orion/orion.log file via the Go bridge. Production .app builds detach
// stdout/stderr, so this is the only way to see what the webview said.
import { LogClient } from '../../wailsjs/go/main/App';

type Level = 'error' | 'warn' | 'info' | 'debug';

function fmt(args: unknown[]): string {
  return args
    .map((a) => {
      if (a instanceof Error) return `${a.name}: ${a.message}\n${a.stack || ''}`;
      if (typeof a === 'string') return a;
      try {
        return JSON.stringify(a);
      } catch {
        return String(a);
      }
    })
    .join(' ');
}

function send(level: Level, args: unknown[]) {
  const msg = fmt(args);
  LogClient(level, msg).catch(() => {});
}

// Wrap console.error and console.warn so existing call sites get persisted
// without code changes. Call once at app startup.
export function installConsoleBridge() {
  const origError = console.error.bind(console);
  const origWarn = console.warn.bind(console);
  console.error = (...args: unknown[]) => {
    send('error', args);
    origError(...args);
  };
  console.warn = (...args: unknown[]) => {
    send('warn', args);
    origWarn(...args);
  };
  window.addEventListener('error', (e) => {
    send('error', [`window.onerror: ${e.message}`, e.filename, e.lineno, e.error]);
  });
  window.addEventListener('unhandledrejection', (e) => {
    send('error', ['unhandledrejection:', e.reason]);
  });
}

export const log = {
  error: (...args: unknown[]) => send('error', args),
  warn: (...args: unknown[]) => send('warn', args),
  info: (...args: unknown[]) => send('info', args),
  debug: (...args: unknown[]) => send('debug', args),
};
