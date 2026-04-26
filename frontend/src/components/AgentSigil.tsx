import type { CSSProperties } from 'react';
import claudeProviderIcon from '../assets/images/provider-icons/claude-provider.png';
import codexProviderIcon from '../assets/images/provider-icons/codex-provider.png';

export type AgentSigilId =
  | 'claude'
  | 'codex'
  | 'reviewer'
  | 'scribe'
  | 'plan'
  | 'test'
  | 'debug'
  | 'deploy'
  | 'ops'
  | 'data'
  | 'design'
  | 'security'
  | 'browser'
  | 'automate'
  | 'branch'
  | 'docs'
  | 'clean'
  | 'shell'
  | 'server'
  | 'editor'
  | 'diagnostics';

const AGENT_META: Record<AgentSigilId, { color: string; deep: string; label: string }> = {
  claude: { color: '#B9A3EC', deep: '#7A5FD4', label: 'Claude' },
  codex: { color: '#8ACFA3', deep: '#4FA872', label: 'Codex' },
  reviewer: { color: '#F4B46A', deep: '#C58330', label: 'Reviewer' },
  scribe: { color: '#E89BB4', deep: '#B66585', label: 'Scribe' },
  plan: { color: '#7CA9F7', deep: '#5A8BE8', label: 'Planner' },
  test: { color: '#9BC5FF', deep: '#5A8BE8', label: 'Test' },
  debug: { color: '#E89BB4', deep: '#B66585', label: 'Debug' },
  deploy: { color: '#8ACFA3', deep: '#4FA872', label: 'Deploy' },
  ops: { color: '#7CA9F7', deep: '#5A8BE8', label: 'Ops' },
  data: { color: '#77D7C8', deep: '#3BA696', label: 'Data' },
  design: { color: '#B9A3EC', deep: '#7A5FD4', label: 'Design' },
  security: { color: '#F4B46A', deep: '#C58330', label: 'Security' },
  browser: { color: '#77D7C8', deep: '#3BA696', label: 'Browser' },
  automate: { color: '#E6B86B', deep: '#B98433', label: 'Automate' },
  branch: { color: '#8ACFA3', deep: '#4FA872', label: 'Branch' },
  docs: { color: '#9BC5FF', deep: '#5A8BE8', label: 'Docs' },
  clean: { color: '#F4B46A', deep: '#C58330', label: 'Clean' },
  shell: { color: '#8E93A6', deep: '#5E6376', label: 'Shell' },
  server: { color: '#7CA9F7', deep: '#5A8BE8', label: 'Server' },
  editor: { color: '#E6B86B', deep: '#B98433', label: 'Editor' },
  diagnostics: { color: '#9BC5FF', deep: '#5A8BE8', label: 'Diagnostics' },
};

export function normalizeAgentSigil(input?: string): AgentSigilId {
  const value = (input || '').toLowerCase().trim();
  if (value.includes('claude')) return 'claude';
  if (value.includes('codex')) return 'codex';
  if (value.includes('review') || value.includes('audit')) return 'reviewer';
  if (value.includes('scribe') || value.includes('write')) return 'scribe';
  if (value.includes('plan')) return 'plan';
  if (value.includes('test') || value.includes('qa')) return 'test';
  if (value.includes('debug') || value.includes('bug')) return 'debug';
  if (value.includes('deploy') || value.includes('release')) return 'deploy';
  if (value.includes('ops')) return 'ops';
  if (value.includes('data') || value.includes('database')) return 'data';
  if (value.includes('design')) return 'design';
  if (value.includes('security') || value.includes('secure')) return 'security';
  if (value.includes('browser') || value.includes('web')) return 'browser';
  if (value.includes('automate') || value.includes('automation')) return 'automate';
  if (value.includes('branch') || value.includes('git')) return 'branch';
  if (value.includes('docs') || value.includes('doc')) return 'docs';
  if (value.includes('clean') || value.includes('refactor')) return 'clean';
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
  const nativeIcon = providerIcon(agentId);
  const style = {
    width: size,
    height: size,
    '--agent-color': agent.color,
    '--agent-color-deep': agent.deep,
  } as CSSProperties;

  return (
    <span
      className={`agent-sigil agent-sigil-${agentId}${nativeIcon ? ' agent-sigil-native' : ''}${strong ? ' agent-sigil-strong' : ''}${className ? ` ${className}` : ''}`}
      style={style}
      aria-label={agent.label}
      title={agent.label}
    >
      {nativeIcon ? (
        <img src={nativeIcon} alt="" draggable={false} />
      ) : (
        <SigilGlyph id={agentId} size={Math.round(size * (strong ? 0.58 : 0.68))} />
      )}
    </span>
  );
}

function providerIcon(id: AgentSigilId): string | null {
  if (id === 'claude') return claudeProviderIcon;
  if (id === 'codex') return codexProviderIcon;
  return null;
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
    case 'plan':
      return (
        <svg {...props}>
          <path d="M7 6 H17" />
          <path d="M7 11 H17" />
          <path d="M7 16 H14" />
          <circle cx="5" cy="6" r="0.9" fill="currentColor" stroke="none" />
          <circle cx="5" cy="11" r="0.9" fill="currentColor" stroke="none" />
          <circle cx="5" cy="16" r="0.9" fill="currentColor" stroke="none" />
        </svg>
      );
    case 'test':
      return (
        <svg {...props}>
          <path d="M9 4 V10 L6 17 A3 3 0 0 0 8.7 20 H15.3 A3 3 0 0 0 18 17 L15 10 V4" />
          <path d="M8 4 H16" />
          <path d="M8 16 H16" />
        </svg>
      );
    case 'debug':
      return (
        <svg {...props}>
          <circle cx="12" cy="13" r="5" />
          <path d="M9 6 L8 4" />
          <path d="M15 6 L16 4" />
          <path d="M7 13 H4" />
          <path d="M20 13 H17" />
          <path d="M8 18 L6 20" />
          <path d="M16 18 L18 20" />
        </svg>
      );
    case 'deploy':
      return (
        <svg {...props}>
          <path d="M12 4 C16 6 18 9 18 13 L14 17 H10 L6 13 C6 9 8 6 12 4 Z" />
          <path d="M9 18 L7 21" />
          <path d="M15 18 L17 21" />
          <circle cx="12" cy="10" r="1.7" />
        </svg>
      );
    case 'ops':
      return (
        <svg {...props}>
          <path d="M7 5 H17 L19 12 L17 19 H7 L5 12 Z" />
          <path d="M8.5 12 H15.5" />
        </svg>
      );
    case 'data':
      return (
        <svg {...props}>
          <ellipse cx="12" cy="6" rx="6" ry="3" />
          <path d="M6 6 V18 C6 19.7 8.7 21 12 21 C15.3 21 18 19.7 18 18 V6" />
          <path d="M6 12 C6 13.7 8.7 15 12 15 C15.3 15 18 13.7 18 12" />
        </svg>
      );
    case 'design':
      return (
        <svg {...props}>
          <path d="M12 4 A8 8 0 1 0 12 20 H14.5 A2 2 0 0 0 16.5 18 A1.7 1.7 0 0 1 18.2 16.3 A2 2 0 0 0 20 14.3 A8 8 0 0 0 12 4 Z" />
          <circle cx="8.6" cy="11" r="1" fill="currentColor" stroke="none" />
          <circle cx="11.4" cy="8.5" r="1" fill="currentColor" stroke="none" />
          <circle cx="14.8" cy="9.5" r="1" fill="currentColor" stroke="none" />
        </svg>
      );
    case 'security':
      return (
        <svg {...props}>
          <path d="M12 4 L18 6.5 V11.5 C18 15.8 15.6 18.7 12 20 C8.4 18.7 6 15.8 6 11.5 V6.5 Z" />
          <path d="M9.5 12 L11.2 13.7 L15 10" />
        </svg>
      );
    case 'browser':
      return (
        <svg {...props}>
          <circle cx="12" cy="12" r="8" />
          <path d="M4.5 12 H19.5" />
          <path d="M12 4 C14.4 6.5 15.5 9.1 15.5 12 C15.5 14.9 14.4 17.5 12 20" />
          <path d="M12 4 C9.6 6.5 8.5 9.1 8.5 12 C8.5 14.9 9.6 17.5 12 20" />
        </svg>
      );
    case 'automate':
      return (
        <svg {...props}>
          <path d="M13 3 L6 13 H11 L10 21 L18 10 H13 Z" />
        </svg>
      );
    case 'branch':
      return (
        <svg {...props}>
          <path d="M7 7 C11 7 10.5 17 17 17" />
          <path d="M7 17 H17" />
          <circle cx="7" cy="7" r="2" />
          <circle cx="7" cy="17" r="2" />
          <circle cx="17" cy="17" r="2" />
        </svg>
      );
    case 'docs':
      return (
        <svg {...props}>
          <path d="M7 4 H15 L18 7 V20 H7 Z" />
          <path d="M15 4 V8 H18" />
          <path d="M9 12 H15" />
          <path d="M9 16 H14" />
        </svg>
      );
    case 'clean':
      return (
        <svg {...props}>
          <path d="M7 14 L14 7 L17 10 L10 17 H7 Z" />
          <path d="M12.5 8.5 L15.5 11.5" />
          <path d="M6 20 H18" />
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
