import { useState, useEffect, useCallback } from 'react';
import { useStore } from '../store';
import { GetChangedFiles } from '../../wailsjs/go/main/App';
import { git } from '../../wailsjs/go/models';

export default function GitPanel() {
  const { activeWorkspacePath, openDiff, project } = useStore();
  const [changedFiles, setChangedFiles] = useState<git.ChangedFile[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (!activeWorkspacePath) return;
    setLoading(true);
    try {
      const files = await GetChangedFiles(activeWorkspacePath);
      setChangedFiles(files || []);
    } catch {}
    setLoading(false);
  }, [activeWorkspacePath]);

  // Refresh when panel becomes visible or workspace changes
  useEffect(() => {
    refresh();
  }, [refresh]);

  const statusColor = (status: string) => {
    switch (status) {
      case 'M': return 'var(--accent-orange)';
      case 'A': return 'var(--accent-green)';
      case 'D': return 'var(--accent-red)';
      case '?': return 'var(--text-dim)';
      case 'R': return 'var(--accent-blue)';
      case 'U': return 'var(--accent-yellow)';
      default: return 'var(--text-dim)';
    }
  };

  if (!activeWorkspacePath) {
    return (
      <div className="sidebar">
        <div className="sidebar-section">
          <div className="sidebar-label">Git Changes</div>
          <div className="sidebar-item">
            <span className="label" style={{ color: 'var(--text-dim)' }}>No workspace selected</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="sidebar">
      <div className="sidebar-section">
        <div className="sidebar-label" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>Changes {changedFiles.length > 0 ? `(${changedFiles.length})` : ''}</span>
          <span
            className="sidebar-action"
            onClick={refresh}
            style={{ cursor: 'pointer', fontSize: '12px' }}
          >
            {loading ? '...' : '↻'}
          </span>
        </div>
      </div>
      <div className="git-file-list">
        {changedFiles.length === 0 && !loading && (
          <div className="sidebar-item" style={{ color: 'var(--text-dim)' }}>
            <span className="label">No changes</span>
          </div>
        )}
        {changedFiles.map((file) => (
          <div
            key={file.path}
            className="git-changed-file"
            onClick={() => openDiff(file.path)}
          >
            <span className="git-status-badge" style={{ color: statusColor(file.status) }}>
              {file.status}
            </span>
            <span className="git-file-name">{file.path}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
