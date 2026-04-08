import { useEffect, useRef } from 'react';
import { createTerminal, OrionTerminal, TERMINAL_BASE_FONT_SIZE } from '../lib/terminal';
import { useStore, zoomFactorFor } from '../store';
import '@xterm/xterm/css/xterm.css';

interface TerminalProps {
  terminalId: string;
  visible: boolean;
}

// Refuse to resize below this width — prevents tmux scrollback from
// getting frozen at a tiny wrap when the container briefly shrinks
// during layout transitions (e.g. opening a diff viewer).
const MIN_COLS = 40;
const MIN_ROWS = 5;

function safeFit(term: OrionTerminal | null, container: HTMLElement | null) {
  if (!term || !container) return;
  // Skip if container is hidden or too small to produce a usable size.
  const rect = container.getBoundingClientRect();
  if (rect.width < 100 || rect.height < 40) return;
  try {
    const dims = term.fitAddon.proposeDimensions();
    if (!dims || !dims.cols || !dims.rows) return;
    if (dims.cols < MIN_COLS || dims.rows < MIN_ROWS) return;
    term.fitAddon.fit();
  } catch {}
}

export default function Terminal({ terminalId, visible }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<OrionTerminal | null>(null);
  const zoomLevel = useStore((s) => s.zoomLevel);

  useEffect(() => {
    if (!containerRef.current) return;

    const initialSize = Math.round(TERMINAL_BASE_FONT_SIZE * zoomFactorFor(useStore.getState().zoomLevel));
    const orionTerm = createTerminal(containerRef.current, terminalId, initialSize);
    termRef.current = orionTerm;

    // Focus when first created
    orionTerm.terminal.focus();

    return () => {
      orionTerm.dispose();
      termRef.current = null;
    };
  }, [terminalId]);

  // Handle visibility and resize
  useEffect(() => {
    if (visible && termRef.current) {
      // Small delay to let layout settle before fitting
      const timer = setTimeout(() => {
        safeFit(termRef.current, containerRef.current);
        termRef.current?.terminal.focus();
      }, 50);
      return () => clearTimeout(timer);
    }
  }, [visible]);

  // React to zoom changes
  useEffect(() => {
    if (!termRef.current) return;
    const newSize = Math.round(TERMINAL_BASE_FONT_SIZE * zoomFactorFor(zoomLevel));
    termRef.current.terminal.options.fontSize = newSize;
    safeFit(termRef.current, containerRef.current);
  }, [zoomLevel]);

  // Handle window resize
  useEffect(() => {
    const handleResize = () => {
      if (visible) safeFit(termRef.current, containerRef.current);
    };

    const observer = new ResizeObserver(handleResize);
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }

    return () => observer.disconnect();
  }, [visible]);

  return (
    <div
      ref={containerRef}
      className="terminal-wrapper"
      style={{ display: visible ? 'block' : 'none' }}
    />
  );
}
