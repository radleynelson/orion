import type { CSSProperties } from 'react';

export type AgentSigilId = 'claude' | 'codex' | 'reviewer' | 'scribe' | 'shell' | 'server' | 'editor' | 'diagnostics';

const AGENT_META: Record<AgentSigilId, { color: string; deep: string; label: string }> = {
  claude: { color: '#B9A3EC', deep: '#7A5FD4', label: 'Claude' },
  codex: { color: '#8ACFA3', deep: '#4FA872', label: 'Codex' },
  reviewer: { color: '#F4B46A', deep: '#C58330', label: 'Reviewer' },
  scribe: { color: '#E89BB4', deep: '#B66585', label: 'Scribe' },
  shell: { color: '#8E93A6', deep: '#5E6376', label: 'Shell' },
  server: { color: '#7CA9F7', deep: '#5A8BE8', label: 'Server' },
  editor: { color: '#E6B86B', deep: '#B98433', label: 'Editor' },
  diagnostics: { color: '#9BC5FF', deep: '#5A8BE8', label: 'Diagnostics' },
};

export function normalizeAgentSigil(input?: string): AgentSigilId {
  const value = (input || '').toLowerCase();
  if (value.includes('claude')) return 'claude';
  if (value.includes('codex')) return 'codex';
  if (value.includes('reviewer')) return 'reviewer';
  if (value.includes('scribe')) return 'scribe';
  if (value.includes('server')) return 'server';
  if (value.includes('editor')) return 'editor';
  if (value.includes('diagnostics')) return 'diagnostics';
  return 'shell';
}

interface Props {
  id?: string;
  size?: number;
  strong?: boolean;
  className?: string;
}

export default function AgentSigil({ id, size = 20, strong = false, className = '' }: Props) {
  const agentId = normalizeAgentSigil(id);
  const agent = AGENT_META[agentId];
  const style = {
    width: size,
    height: size,
    '--agent-color': agent.color,
    '--agent-color-deep': agent.deep,
  } as CSSProperties;

  return (
    <span
      className={`agent-sigil agent-sigil-${agentId}${strong ? ' agent-sigil-strong' : ''}${className ? ` ${className}` : ''}`}
      style={style}
      aria-label={agent.label}
      title={agent.label}
    >
      <SigilGlyph id={agentId} size={Math.round(size * (strong ? 0.58 : 0.68))} />
    </span>
  );
}

function SigilGlyph({ id, size }: { id: AgentSigilId; size: number }) {
  const strokeWidth = size <= 14 ? 2.4 : size <= 22 ? 2 : 1.8;
  const props = {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    'aria-hidden': true,
  };

  switch (id) {
    case 'claude':
      return (
        <svg {...props}>
          <path d="M17 4.5 A 9 9 0 1 0 17 19.5 A 7 7 0 1 1 17 4.5 Z" />
          <circle cx="8.5" cy="12" r="1" fill="currentColor" stroke="none" />
        </svg>
      );
    case 'codex':
      return (
        <svg {...props}>
          <path d="M6 6 L12 12 L6 18" />
          <path d="M13 6 L19 12 L13 18" />
        </svg>
      );
    case 'reviewer':
      return (
        <svg {...props}>
          <circle cx="10" cy="10" r="6" />
          <circle cx="10" cy="10" r="1.6" fill="currentColor" stroke="none" />
          <path d="M14.5 14.5 L19 19" />
        </svg>
      );
    case 'scribe':
      return (
        <svg {...props}>
          <path d="M6 5 L12 5" strokeWidth={strokeWidth * 0.9} />
          <path d="M9 5 L9 19" strokeWidth={strokeWidth * 1.3} />
          <path d="M6 19 L14 19" strokeWidth={strokeWidth * 0.9} />
          <circle cx="17" cy="8" r="1.3" fill="currentColor" stroke="none" />
        </svg>
      );
    case 'server':
      return (
        <svg {...props}>
          <path d="M7 5 H17 L19 12 L17 19 H7 L5 12 Z" />
          <path d="M8.5 12 H15.5" />
        </svg>
      );
    case 'editor':
      return (
        <svg {...props}>
          <path d="M7 5 H17" />
          <path d="M7 10 H15" />
          <path d="M7 15 H13" />
          <path d="M17 14 L19 16 L15 20 L13 20 L13 18 Z" />
        </svg>
      );
    case 'diagnostics':
      return (
        <svg {...props}>
          <circle cx="12" cy="12" r="7" />
          <path d="M12 8 V12" />
          <path d="M12 16 H12.01" />
        </svg>
      );
    case 'shell':
    default:
      return (
        <svg {...props}>
          <path d="M7 7 L13 12 L7 17" />
          <path d="M14 17 L19 17" strokeWidth={strokeWidth * 0.9} />
        </svg>
      );
  }
}
