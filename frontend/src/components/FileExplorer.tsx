import { useState, useEffect, useCallback, useRef } from 'react';
import { useStore, PaneLeaf } from '../store';
import { ListDirectory } from '../../wailsjs/go/main/App';
import { getLanguageFromPath } from '../lib/languages';
import { files } from '../../wailsjs/go/models';

// Collect all leaves from a pane tree
function collectLeaves(pane: any): PaneLeaf[] {
  if (pane.type === 'terminal' || pane.type === 'editor') return [pane];
  if (pane.children) return pane.children.flatMap(collectLeaves);
  return [];
}

interface TreeNodeProps {
  entry: files.FileEntry;
  depth: number;
  revealPath: string | null;
  activeFilePath: string | null;
}

function TreeNode({ entry, depth, revealPath, activeFilePath }: TreeNodeProps) {
  const [expanded, setExpanded] = useState(false);
  const [children, setChildren] = useState<files.FileEntry[] | null>(null);
  const { openFile, setSidebarMode } = useStore();
  const nodeRef = useRef<HTMLDivElement>(null);

  // Auto-expand if this directory is an ancestor of the reveal path
  useEffect(() => {
    if (!revealPath || !entry.isDir) return;
    if (revealPath.startsWith(entry.path + '/')) {
      if (!expanded) {
        setExpanded(true);
        if (!children) {
          ListDirectory(entry.path, 0).then(setChildren).catch(() => {});
        }
      }
    }
  }, [revealPath, entry.path, entry.isDir]);

  // Scroll into view if this is the revealed file
  useEffect(() => {
    if (revealPath && entry.path === revealPath && nodeRef.current) {
      nodeRef.current.scrollIntoView({ block: 'center', behavior: 'smooth' });
    }
  }, [revealPath, entry.path]);

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
      setSidebarMode('files');
    }
  }, [entry, expanded, children, openFile, setSidebarMode]);

  const isActive = !entry.isDir && entry.path === activeFilePath;
  const icon = entry.isDir ? (expanded ? '▾' : '▸') : '◇';
  const iconColor = entry.isDir ? 'var(--accent-blue)' : isActive ? 'var(--accent-purple)' : 'var(--text-dim)';

  return (
    <>
      <div
        ref={nodeRef}
        className={`file-tree-node ${isActive ? 'file-tree-active' : ''}`}
        onClick={handleClick}
        style={{ paddingLeft: `${12 + depth * 16}px` }}
      >
        <span className="file-tree-icon" style={{ color: iconColor }}>{icon}</span>
        <span className="file-tree-label">{entry.name}</span>
      </div>
      {expanded && children && children.map((child) => (
        <TreeNode
          key={child.path}
          entry={child}
          depth={depth + 1}
          revealPath={revealPath}
          activeFilePath={activeFilePath}
        />
      ))}
    </>
  );
}

export default function FileExplorer() {
  const { activeWorkspacePath, tabs, activeTabId } = useStore();
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

  // Get the active file path from the current editor tab
  const activeFilePath = (() => {
    const tab = tabs.find((t) => t.id === activeTabId);
    if (!tab) return null;
    const leaves = collectLeaves(tab.rootPane);
    const editorLeaf = leaves.find((l) => l.type === 'editor');
    return editorLeaf?.filePath || null;
  })();

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
          <TreeNode
            key={entry.path}
            entry={entry}
            depth={0}
            revealPath={activeFilePath}
            activeFilePath={activeFilePath}
          />
        ))}
      </div>
    </div>
  );
}
