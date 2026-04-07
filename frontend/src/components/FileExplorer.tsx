import { useState, useEffect, useCallback } from 'react';
import { useStore } from '../store';
import { ListDirectory } from '../../wailsjs/go/main/App';
import { getLanguageFromPath } from '../lib/languages';
import { files } from '../../wailsjs/go/models';

interface TreeNodeProps {
  entry: files.FileEntry;
  depth: number;
}

function TreeNode({ entry, depth }: TreeNodeProps) {
  const [expanded, setExpanded] = useState(false);
  const [children, setChildren] = useState<files.FileEntry[] | null>(null);
  const { openFile } = useStore();

  const handleClick = useCallback(async () => {
    if (entry.isDir) {
      if (!expanded && !children) {
        try {
          const entries = await ListDirectory(entry.path, 0);
          setChildren(entries);
        } catch {}
      }
      setExpanded(!expanded);
    } else {
      const language = getLanguageFromPath(entry.path);
      openFile(entry.path, language);
    }
  }, [entry, expanded, children, openFile]);

  const icon = entry.isDir
    ? (expanded ? '▾' : '▸')
    : '◇';

  const iconColor = entry.isDir ? 'var(--accent-blue)' : 'var(--text-dim)';

  return (
    <>
      <div
        className="file-tree-node"
        onClick={handleClick}
        style={{ paddingLeft: `${12 + depth * 16}px` }}
      >
        <span className="file-tree-icon" style={{ color: iconColor }}>{icon}</span>
        <span className="file-tree-label">{entry.name}</span>
      </div>
      {expanded && children && children.map((child) => (
        <TreeNode key={child.path} entry={child} depth={depth + 1} />
      ))}
    </>
  );
}

export default function FileExplorer() {
  const { activeWorkspacePath } = useStore();
  const [entries, setEntries] = useState<files.FileEntry[]>([]);

  useEffect(() => {
    if (!activeWorkspacePath) return;
    (async () => {
      try {
        const result = await ListDirectory(activeWorkspacePath, 0);
        setEntries(result);
      } catch {}
    })();
  }, [activeWorkspacePath]);

  if (!activeWorkspacePath) {
    return (
      <div className="sidebar">
        <div className="sidebar-section">
          <div className="sidebar-label">Files</div>
          <div className="sidebar-item">
            <span className="label" style={{ color: 'var(--text-dim)' }}>No workspace selected</span>
          </div>
        </div>
      </div>
    );
  }

  const wsName = activeWorkspacePath.split('/').pop() || '';

  return (
    <div className="sidebar">
      <div className="sidebar-section">
        <div className="sidebar-label">{wsName}</div>
      </div>
      <div className="file-tree">
        {entries.map((entry) => (
          <TreeNode key={entry.path} entry={entry} depth={0} />
        ))}
      </div>
    </div>
  );
}
