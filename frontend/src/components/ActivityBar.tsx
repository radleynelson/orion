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
        ◫
      </div>
      <div
        className={`activity-bar-icon ${sidebarMode === 'files' ? 'active' : ''}`}
        onClick={() => toggle('files')}
        title="File Explorer (⌘⇧E)"
      >
        ▤
      </div>
      <div
        className={`activity-bar-icon ${sidebarMode === 'search' ? 'active' : ''}`}
        onClick={() => toggle('search')}
        title="Search (⌘⇧F)"
      >
        ⌕
      </div>
      <div
        className={`activity-bar-icon ${diagnosticsActive ? 'active' : ''}`}
        onClick={onOpenDiagnostics}
        title="Diagnostics"
      >
        ◎
      </div>
    </div>
  );
}
