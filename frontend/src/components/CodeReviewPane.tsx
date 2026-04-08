import { useState, useEffect, useCallback, useRef } from 'react';
import { useStore } from '../store';
import {
  GetChangedFilesAgainst,
  GetUnifiedDiff,
  DiscardFileChanges,
  DiscardAllChanges,
} from '../../wailsjs/go/main/App';
import { git } from '../../wailsjs/go/models';
import { parseUnifiedDiff, ParsedDiff } from '../lib/diffParser';
import { getLanguageFromPath } from '../lib/languages';

interface FileEntry {
  file: git.ChangedFile;
  diff: ParsedDiff | null;
  collapsed: boolean;
}

export default function CodeReviewPane() {
  const {
    activeWorkspacePath,
    project,
    codeReviewBase,
    setCodeReviewBase,
    setCodeReviewVisible,
    openFile,
  } = useStore();

  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [confirmAll, setConfirmAll] = useState(false);
  const [confirmFile, setConfirmFile] = useState<string | null>(null);
  const reqId = useRef(0);

  const baseArg = codeReviewBase === 'main' ? (project?.mainBranch || 'main') : '';

  const refresh = useCallback(async () => {
    if (!activeWorkspacePath) {
      setEntries([]);
      return;
    }
    const myReq = ++reqId.current;
    // Clear stale entries from the previous workspace/base immediately so we
    // never show the wrong diff while the new fetch is in flight.
    setEntries([]);
    setLoading(true);
    try {
      const files = (await GetChangedFilesAgainst(activeWorkspacePath, baseArg)) || [];
      if (myReq !== reqId.current) return;

      // Fetch diffs in parallel
      const diffs = await Promise.all(
        files.map(async (f) => {
          try {
            const raw = await GetUnifiedDiff(activeWorkspacePath, baseArg, f.path);
            return parseUnifiedDiff(raw || '');
          } catch {
            return parseUnifiedDiff('');
          }
        })
      );
      if (myReq !== reqId.current) return;

      setEntries(
        files.map((f, i) => ({ file: f, diff: diffs[i], collapsed: false }))
      );
    } catch {
      if (myReq === reqId.current) setEntries([]);
    } finally {
      if (myReq === reqId.current) setLoading(false);
    }
  }, [activeWorkspacePath, baseArg]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const discardFile = async (path: string) => {
    if (!activeWorkspacePath) return;
    if (confirmFile !== path) {
      setConfirmFile(path);
      setConfirmAll(false);
      // auto-cancel after 4s
      setTimeout(() => setConfirmFile((c) => (c === path ? null : c)), 4000);
      return;
    }
    setConfirmFile(null);
    try {
      await DiscardFileChanges(activeWorkspacePath, path);
      await refresh();
    } catch (err) {
      console.error('Discard failed:', err);
    }
  };

  const discardAll = async () => {
    if (!activeWorkspacePath || entries.length === 0) return;
    if (!confirmAll) {
      setConfirmAll(true);
      setConfirmFile(null);
      setTimeout(() => setConfirmAll((c) => c && false), 4000);
      return;
    }
    setConfirmAll(false);
    try {
      await DiscardAllChanges(activeWorkspacePath);
      await refresh();
    } catch (err) {
      console.error('Discard all failed:', err);
    }
  };

  const toggleCollapse = (path: string) => {
    setEntries((prev) =>
      prev.map((e) => (e.file.path === path ? { ...e, collapsed: !e.collapsed } : e))
    );
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'M': return 'var(--accent-orange)';
      case 'A': return 'var(--accent-green)';
      case 'D': return 'var(--accent-red)';
      case '?': return 'var(--text-dim)';
      case 'R': return 'var(--accent-blue)';
      default: return 'var(--text-dim)';
    }
  };

  return (
    <div className="code-review-pane">
      <div className="cr-header">
        <span className="cr-title">Code Review</span>
        <select
          className="cr-base-select"
          value={codeReviewBase}
          onChange={(e) => setCodeReviewBase(e.target.value as 'uncommitted' | 'main')}
        >
          <option value="uncommitted">Uncommitted changes</option>
          <option value="main">vs {project?.mainBranch || 'main'}</option>
        </select>
        <span className="cr-spacer" />
        {codeReviewBase === 'uncommitted' && entries.length > 0 && (
          <button
            className="cr-icon-btn cr-discard-all"
            onClick={discardAll}
            title="Discard all changes"
          >
            {confirmAll ? 'Click again to confirm' : 'Discard all'}
          </button>
        )}
        <button className="cr-icon-btn" onClick={refresh} title="Refresh">
          {loading ? '…' : '↻'}
        </button>
        <button
          className="cr-icon-btn"
          onClick={() => setCodeReviewVisible(false)}
          title="Close (⌘⇧+)"
        >
          ✕
        </button>
      </div>

      <div className="cr-body">
        {entries.length === 0 && !loading && (
          <div className="cr-empty">No changes</div>
        )}
        {entries.map(({ file, diff, collapsed }) => (
          <div className="cr-file-card" key={file.path}>
            <div className="cr-file-header" onClick={() => toggleCollapse(file.path)}>
              <span className="cr-chevron">{collapsed ? '▸' : '▾'}</span>
              <span className="cr-status" style={{ color: statusColor(file.status) }}>
                {file.status}
              </span>
              <span
                className="cr-file-path cr-file-link"
                onClick={(e) => {
                  if (e.metaKey || e.ctrlKey) {
                    e.stopPropagation();
                    if (activeWorkspacePath) {
                      const fullPath = activeWorkspacePath + '/' + file.path;
                      openFile(fullPath, getLanguageFromPath(file.path));
                    }
                  }
                }}
                title="⌘+click to open file"
              >{file.path}</span>
              {codeReviewBase === 'uncommitted' && (
                <button
                  className="cr-discard-file"
                  onClick={(e) => {
                    e.stopPropagation();
                    discardFile(file.path);
                  }}
                  title="Discard changes to this file"
                >
                  {confirmFile === file.path ? 'Click again' : '↶ Discard'}
                </button>
              )}
              {diff && (
                <span className="cr-counts">
                  <span className="cr-add">+{diff.added}</span>{' '}
                  <span className="cr-del">−{diff.removed}</span>
                </span>
              )}
              <span
                className="cr-copy-icon"
                onClick={(e) => {
                  e.stopPropagation();
                  navigator.clipboard.writeText(file.path);
                  const el = e.currentTarget;
                  el.textContent = '✓';
                  setTimeout(() => { el.textContent = '📋'; }, 800);
                }}
                title="Copy path"
              >📋</span>
            </div>
            {!collapsed && diff && diff.hunks.length > 0 && (
              <div className="cr-hunks">
                {diff.hunks.map((hunk, hi) => (
                  <div className="cr-hunk" key={hi}>
                    <div className="cr-hunk-header">{hunk.header}</div>
                    {hunk.lines.map((line, li) => (
                      <div className={`cr-line cr-${line.kind}`} key={li}>
                        <span className="cr-gutter cr-gutter-old">
                          {line.kind === 'add' ? '' : line.oldNum ?? ''}
                        </span>
                        <span className="cr-gutter cr-gutter-new">
                          {line.kind === 'del' ? '' : line.newNum ?? ''}
                        </span>
                        <span className="cr-sign">
                          {line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '}
                        </span>
                        <span className="cr-text">{line.text || ' '}</span>
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            )}
            {!collapsed && diff && diff.hunks.length === 0 && (
              <div className="cr-empty cr-empty-file">(no textual diff)</div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
