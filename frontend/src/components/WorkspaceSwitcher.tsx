import { useState, useEffect, useCallback } from 'react';
import { useStore } from '../store';

interface WorkspaceSwitcherProps {
  visible: boolean;
  onClose: () => void;
}

export default function WorkspaceSwitcher({ visible, onClose }: WorkspaceSwitcherProps) {
  const { workspaces, activeWorkspacePath, setActiveWorkspace } = useStore();
  const [selectedIndex, setSelectedIndex] = useState(0);

  // Set initial selection to current workspace
  useEffect(() => {
    if (visible) {
      const currentIdx = workspaces.findIndex((w) => w.path === activeWorkspacePath);
      // Start on the NEXT workspace (like Cmd+Tab selects the previous app)
      setSelectedIndex(currentIdx >= 0 ? (currentIdx + 1) % workspaces.length : 0);
    }
  }, [visible, workspaces, activeWorkspacePath]);

  const handleSelect = useCallback(() => {
    if (workspaces.length > 0 && selectedIndex < workspaces.length) {
      setActiveWorkspace(workspaces[selectedIndex].path);
    }
    onClose();
  }, [workspaces, selectedIndex, setActiveWorkspace, onClose]);

  useEffect(() => {
    if (!visible) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      } else if (e.key === 'Tab' && e.ctrlKey) {
        e.preventDefault();
        if (e.shiftKey) {
          // Ctrl+Shift+Tab: previous
          setSelectedIndex((i) => (i - 1 + workspaces.length) % workspaces.length);
        } else {
          // Ctrl+Tab: next
          setSelectedIndex((i) => (i + 1) % workspaces.length);
        }
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((i) => (i + 1) % workspaces.length);
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((i) => (i - 1 + workspaces.length) % workspaces.length);
      } else if (e.key === 'Enter') {
        e.preventDefault();
        handleSelect();
      }
    };

    // Select on Ctrl release (like Cmd+Tab behavior)
    const handleKeyUp = (e: KeyboardEvent) => {
      if (e.key === 'Control') {
        handleSelect();
      }
    };

    window.addEventListener('keydown', handleKeyDown, { capture: true });
    window.addEventListener('keyup', handleKeyUp);
    return () => {
      window.removeEventListener('keydown', handleKeyDown, { capture: true });
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [visible, workspaces, handleSelect, onClose]);

  if (!visible || workspaces.length === 0) return null;

  return (
    <div className="search-overlay" onClick={onClose}>
      <div className="switcher-modal" onClick={(e) => e.stopPropagation()}>
        <div className="switcher-title">Switch Workspace</div>
        {workspaces.map((ws, i) => (
          <div
            key={ws.path}
            className={`switcher-item ${i === selectedIndex ? 'selected' : ''} ${ws.path === activeWorkspacePath ? 'current' : ''}`}
            onClick={() => {
              setSelectedIndex(i);
              setActiveWorkspace(ws.path);
              onClose();
            }}
          >
            <span className="switcher-icon">
              {ws.isMain ? '◉' : '○'}
            </span>
            <span className="switcher-name">
              {ws.isMain ? 'main' : ws.branch || ws.name}
            </span>
            {ws.path === activeWorkspacePath && (
              <span className="switcher-current">current</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
