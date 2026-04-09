import { useState, useEffect, useCallback, useRef } from 'react';
import { useStore } from '../store';
import {
  GetChangedFilesAgainst,
  GetUnifiedDiff,
  DiscardFileChanges,
  DiscardAllChanges,
  WatchWorkspace,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { git } from '../../wailsjs/go/models';
import { parseUnifiedDiff, ParsedDiff } from '../lib/diffParser';
import { getLanguageFromPath } from '../lib/languages';

interface FileEntry {
  file: git.ChangedFile;
  diff: ParsedDiff | null;
  rawDiff: string;
  collapsed: boolean;
  viewed: boolean;
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
  const [selectedIndex, setSelectedIndex] = useState(0);
  const reqId = useRef(0);
  const fileRefs = useRef<Map<number, HTMLDivElement>>(new Map());
  // Track raw diffs for viewed files so we can detect when they change
  const viewedDiffs = useRef<Map<string, string>>(new Map());

  const baseArg = codeReviewBase === 'main' ? (project?.mainBranch || 'main') : '';

  const refresh = useCallback(async (clear?: boolean) => {
    if (!activeWorkspacePath) {
      setEntries([]);
      return;
    }
    const myReq = ++reqId.current;
    if (clear) setEntries([]);
    setLoading(true);
    try {
      const files = (await GetChangedFilesAgainst(activeWorkspacePath, baseArg)) || [];
      if (myReq !== reqId.current) return;

      // Fetch diffs in parallel
      const results = await Promise.all(
        files.map(async (f) => {
          try {
            const raw = await GetUnifiedDiff(activeWorkspacePath, baseArg, f.path);
            return { raw: raw || '', parsed: parseUnifiedDiff(raw || '') };
          } catch {
            return { raw: '', parsed: parseUnifiedDiff('') };
          }
        })
      );
      if (myReq !== reqId.current) return;

      setEntries((prev) => {
        const prevByPath = new Map(prev.map((e) => [e.file.path, e]));
        return files.map((f, i) => {
          const { raw, parsed } = results[i];
          const savedDiff = viewedDiffs.current.get(f.path);
          const wasViewed = savedDiff !== undefined;
          const diffChanged = wasViewed && savedDiff !== raw;
          if (diffChanged) {
            viewedDiffs.current.delete(f.path);
          }
          const viewed = wasViewed && !diffChanged;
          // Preserve user's collapsed state for non-viewed files across refreshes
          const prevEntry = prevByPath.get(f.path);
          const collapsed = viewed || (prevEntry ? prevEntry.collapsed : false);
          return { file: f, diff: parsed, rawDiff: raw, collapsed, viewed };
        });
      });
    } catch {
      if (myReq === reqId.current) setEntries([]);
    } finally {
      if (myReq === reqId.current) setLoading(false);
    }
  }, [activeWorkspacePath, baseArg]);

  // Clear viewed state and entries when workspace or base changes
  const prevContext = useRef({ workspace: activeWorkspacePath, base: baseArg });
  useEffect(() => {
    const changed = prevContext.current.workspace !== activeWorkspacePath ||
                    prevContext.current.base !== baseArg;
    prevContext.current = { workspace: activeWorkspacePath, base: baseArg };
    if (changed) {
      viewedDiffs.current.clear();
    }
    refresh(changed);
  }, [refresh, activeWorkspacePath, baseArg]);

  // Watch workspace for file changes and auto-refresh
  useEffect(() => {
    if (!activeWorkspacePath) return;
    WatchWorkspace(activeWorkspacePath).catch(() => {});
    const cancel = EventsOn('git:files-changed', () => {
      refresh();
    });
    return cancel;
  }, [activeWorkspacePath, refresh]);

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

  const toggleViewed = (path: string) => {
    setEntries((prev) =>
      prev.map((e) => {
        if (e.file.path !== path) return e;
        const newViewed = !e.viewed;
        if (newViewed) {
          viewedDiffs.current.set(path, e.rawDiff);
          return { ...e, viewed: true, collapsed: true };
        } else {
          viewedDiffs.current.delete(path);
          return { ...e, viewed: false, collapsed: false };
        }
      })
    );
  };

  // Clamp selected index when entries change
  useEffect(() => {
    setSelectedIndex((i) => (entries.length === 0 ? 0 : Math.min(i, entries.length - 1)));
  }, [entries.length]);

  // Scroll selected file into view
  useEffect(() => {
    const el = fileRefs.current.get(selectedIndex);
    if (el) el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }, [selectedIndex]);

  // Keyboard navigation: v = mark viewed + next, j/↓ = next, k/↑ = prev
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      // Don't capture when typing in inputs
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      // Don't capture when terminal is focused (xterm captures its own keys)
      const active = document.activeElement;
      if (active && active.closest('.xterm')) return;

      if (e.key === 'v' && !e.metaKey && !e.ctrlKey && !e.altKey) {
        e.preventDefault();
        setEntries((prev) => {
          if (prev.length === 0) return prev;
          const idx = Math.min(selectedIndex, prev.length - 1);
          const entry = prev[idx];
          if (entry.viewed) {
            // Unmark viewed
            viewedDiffs.current.delete(entry.file.path);
            return prev.map((ent, i) =>
              i === idx ? { ...ent, viewed: false, collapsed: false } : ent
            );
          }
          // Mark viewed
          viewedDiffs.current.set(entry.file.path, entry.rawDiff);
          const updated = prev.map((ent, i) =>
            i === idx ? { ...ent, viewed: true, collapsed: true } : ent
          );
          // Move to next unviewed file
          let next = idx + 1;
          while (next < updated.length && updated[next].viewed) next++;
          if (next >= updated.length) {
            // Wrap: find first unviewed
            next = updated.findIndex((ent) => !ent.viewed);
            if (next === -1) next = idx; // all viewed, stay put
          }
          setSelectedIndex(next);
          return updated;
        });
      }
      if (e.key === 'j' || e.key === 'ArrowDown') {
        if (!e.metaKey && !e.ctrlKey && !e.altKey) {
          e.preventDefault();
          setSelectedIndex((i) => Math.min(i + 1, entries.length - 1));
        }
      }
      if (e.key === 'k' || e.key === 'ArrowUp') {
        if (!e.metaKey && !e.ctrlKey && !e.altKey) {
          e.preventDefault();
          setSelectedIndex((i) => Math.max(i - 1, 0));
        }
      }
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [selectedIndex, entries.length]);

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
          onChange={(e) => { setCodeReviewBase(e.target.value as 'uncommitted' | 'main'); e.target.blur(); }}
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
        <button className="cr-icon-btn" onClick={() => refresh()} title="Refresh">
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
        {entries.map(({ file, diff, collapsed, viewed }, idx) => (
          <div
            className={`cr-file-card ${viewed ? 'cr-file-viewed' : ''} ${idx === selectedIndex ? 'cr-file-selected' : ''}`}
            key={file.path}
            ref={(el) => { if (el) fileRefs.current.set(idx, el); else fileRefs.current.delete(idx); }}
            onClick={() => setSelectedIndex(idx)}
          >
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
              <span
                className="cr-copy-icon"
                onClick={(e) => {
                  e.stopPropagation();
                  navigator.clipboard.writeText(file.path);
                  const el = e.currentTarget;
                  el.textContent = '✓';
                  setTimeout(() => { el.textContent = '⎘'; }, 800);
                }}
                title="Copy path"
              >⎘</span>
              <span style={{ flex: 1 }} />
              <span
                className={`cr-viewed-check ${viewed ? 'checked' : ''}`}
                onClick={(e) => { e.stopPropagation(); toggleViewed(file.path); }}
                title={viewed ? 'Mark as unviewed' : 'Mark as viewed'}
              >
                {viewed ? '✓ Viewed' : 'Viewed'}
              </span>
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
