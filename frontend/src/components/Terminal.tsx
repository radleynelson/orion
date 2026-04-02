import { useEffect, useRef } from 'react';
import { createTerminal, OrionTerminal } from '../lib/terminal';
import '@xterm/xterm/css/xterm.css';

interface TerminalProps {
  terminalId: string;
  visible: boolean;
}

export default function Terminal({ terminalId, visible }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<OrionTerminal | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const orionTerm = createTerminal(containerRef.current, terminalId);
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
        termRef.current?.fitAddon.fit();
        termRef.current?.terminal.focus();
      }, 50);
      return () => clearTimeout(timer);
    }
  }, [visible]);

  // Handle window resize
  useEffect(() => {
    const handleResize = () => {
      if (visible && termRef.current) {
        termRef.current.fitAddon.fit();
      }
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
