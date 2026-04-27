import { useStore } from '../store';

type SidebarMode = 'workspaces' | 'files' | 'search';

interface ActivityBarProps {
  onOpenDiagnostics: () => void;
  diagnosticsActive: boolean;
}

export default function ActivityBar({ onOpenDiagnostics, diagnosticsActive }: ActivityBarProps) {
  const { sidebarMode, setSidebarMode } = useStore();

  const toggle = (mode: SidebarMode) => {
    if (sidebarMode === mode) {
      setSidebarMode(null);
    } else {
      setSidebarMode(mode);
    }
  };

  return (
    <div className="activity-bar">
      <div
        className={`activity-bar-icon ${sidebarMode === 'workspaces' ? 'active' : ''}`}
        onClick={() => toggle('workspaces')}
        title="Workspaces"
      >
        <ActivityIcon name="workspaces" />
      </div>
      <div
        className={`activity-bar-icon ${sidebarMode === 'files' ? 'active' : ''}`}
        onClick={() => toggle('files')}
        title="File Explorer (⌘⇧E)"
      >
        <ActivityIcon name="files" />
      </div>
      <div
        className={`activity-bar-icon ${sidebarMode === 'search' ? 'active' : ''}`}
        onClick={() => toggle('search')}
        title="Search (⌘⇧F)"
      >
        <ActivityIcon name="search" />
      </div>
      <div
        className={`activity-bar-icon ${diagnosticsActive ? 'active' : ''}`}
        onClick={onOpenDiagnostics}
        title="Diagnostics"
      >
        <ActivityIcon name="diagnostics" />
      </div>
    </div>
  );
}

function ActivityIcon({ name }: { name: 'workspaces' | 'files' | 'search' | 'diagnostics' }) {
  const props = {
    width: 19,
    height: 19,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.8,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    'aria-hidden': true,
  };

  switch (name) {
    case 'workspaces':
      return (
        <svg {...props}>
          <rect x="5" y="5" width="6" height="14" rx="1.2" />
          <rect x="13" y="5" width="6" height="14" rx="1.2" />
        </svg>
      );
    case 'files':
      return (
        <svg {...props}>
          <path d="M6 6 H18" />
          <path d="M6 10 H18" />
          <path d="M6 14 H18" />
          <path d="M6 18 H18" />
        </svg>
      );
    case 'search':
      return (
        <svg {...props}>
          <circle cx="10.5" cy="10.5" r="5.2" />
          <path d="M14.5 14.5 L19 19" />
        </svg>
      );
    case 'diagnostics':
      return (
        <svg {...props}>
          <circle cx="12" cy="12" r="8" />
          <circle cx="12" cy="12" r="4.2" />
        </svg>
      );
  }
}
