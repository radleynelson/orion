import { useState, useEffect, useCallback, useRef, useMemo, type ReactNode } from 'react';
import { useStore } from '../store';
import {
  GetChangedFilesAgainst,
  GetUnifiedDiff,
  DiscardFileChanges,
  DiscardAllChanges,
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

type TreeNode =
  | { kind: 'dir'; name: string; path: string; children: TreeNode[]; added: number; removed: number; fileCount: number }
  | { kind: 'file'; name: string; entry: FileEntry; index: number };

function buildTree(entries: FileEntry[]): TreeNode[] {
  type DirMap = { dirs: Map<string, DirMap>; files: { entry: FileEntry; index: number }[] };
  const root: DirMap = { dirs: new Map(), files: [] };
  entries.forEach((entry, index) => {
    const parts = entry.file.path.split('/');
    parts.pop();
    let cur = root;
    for (const part of parts) {
      let next = cur.dirs.get(part);
      if (!next) {
        next = { dirs: new Map(), files: [] };
        cur.dirs.set(part, next);
      }
      cur = next;
    }
    cur.files.push({ entry, index });
  });

  function toNodes(map: DirMap, parentPath: string): TreeNode[] {
    const result: TreeNode[] = [];
    const dirNames = Array.from(map.dirs.keys()).sort();
    for (const name of dirNames) {
      let dirMap = map.dirs.get(name)!;
      let compactName = name;
      let compactPath = parentPath ? parentPath + '/' + name : name;
      while (dirMap.dirs.size === 1 && dirMap.files.length === 0) {
        const [childName, childMap] = Array.from(dirMap.dirs.entries())[0];
        compactName += '/' + childName;
        compactPath += '/' + childName;
        dirMap = childMap;
      }
      const children = toNodes(dirMap, compactPath);
      let added = 0;
      let removed = 0;
      let fileCount = 0;
      for (const ch of children) {
        if (ch.kind === 'file') {
          fileCount++;
          added += ch.entry.diff?.added ?? 0;
          removed += ch.entry.diff?.removed ?? 0;
        } else {
          fileCount += ch.fileCount;
          added += ch.added;
          removed += ch.removed;
        }
      }
      result.push({ kind: 'dir', name: compactName, path: compactPath, children, added, removed, fileCount });
    }
    const files = [...map.files].sort((a, b) => a.entry.file.path.localeCompare(b.entry.file.path));
    for (const { entry, index } of files) {
      const fileName = entry.file.path.split('/').pop() || entry.file.path;
      result.push({ kind: 'file', name: fileName, entry, index });
    }
    return result;
  }

  return toNodes(root, '');
}

function renderHighlighted(text: string, query: string): ReactNode {
  const display = text.length === 0 ? ' ' : text;
  if (!query) return display;
  const lower = display.toLowerCase();
  const q = query.toLowerCase();
  if (!lower.includes(q)) return display;
  const parts: ReactNode[] = [];
  let cursor = 0;
  while (true) {
    const i = lower.indexOf(q, cursor);
    if (i < 0) {
      if (cursor < display.length) parts.push(display.slice(cursor));
      break;
    }
    if (i > cursor) parts.push(display.slice(cursor, i));
    parts.push(<mark key={parts.length} className="cr-match">{display.slice(i, i + q.length)}</mark>);
    cursor = i + q.length;
  }
  return parts;
}

export default function CodeReviewPane() {
  const {
    activeWorkspacePath,
    workspaces,
    project,
    codeReviewBase,
    setCodeReviewBase,
    setCodeReviewVisible,
    openFile,
  } = useStore();
  const activeWorkspace = workspaces.find((w) => w.path === activeWorkspacePath);
  const activeWorkspaceLabel = activeWorkspace
    ? activeWorkspace.isMain
      ? 'main'
      : project
        ? activeWorkspace.name.replace(project.name + '-', '')
        : activeWorkspace.name
    : '(no workspace)';

  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [confirmAll, setConfirmAll] = useState(false);
  const [confirmFile, setConfirmFile] = useState<string | null>(null);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [fileSearch, setFileSearch] = useState('');
  const [contentSearch, setContentSearch] = useState('');
  const [collapsedDirs, setCollapsedDirs] = useState<Set<string>>(new Set());
  const [filesPanelWidth, setFilesPanelWidth] = useState<number>(() => {
    const v = parseFloat(localStorage.getItem('orion.codeReviewFilesWidth') || '');
    return Number.isFinite(v) && v >= 160 && v <= 500 ? v : 240;
  });
  const [filesPanelVisible, setFilesPanelVisible] = useState<boolean>(() => {
    const v = localStorage.getItem('orion.codeReviewFilesVisible');
    return v === null ? true : v === '1';
  });
  const toggleFilesPanel = useCallback(() => {
    setFilesPanelVisible((v) => {
      const next = !v;
      localStorage.setItem('orion.codeReviewFilesVisible', next ? '1' : '0');
      return next;
    });
  }, []);
  const [matchCursor, setMatchCursor] = useState(0);
  const reqId = useRef(0);
  const fileRefs = useRef<Map<number, HTMLDivElement>>(new Map());
  const lineRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  // Track raw diffs for viewed files so we can detect when they change
  const viewedDiffs = useRef<Map<string, string>>(new Map());

  const baseArg = codeReviewBase === 'main' ? (project?.mainBranch || 'main') : '';

  const persistFilesPanelWidth = useCallback((w: number) => {
    const clamped = Math.max(160, Math.min(500, w));
    localStorage.setItem('orion.codeReviewFilesWidth', String(clamped));
    setFilesPanelWidth(clamped);
  }, []);

  const refresh = useCallback(async (clear?: boolean) => {
    if (!activeWorkspacePath) {
      setEntries([]);
      setLoading(true);
      setTimeout(() => setLoading(false), 150);
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
          } catch (err) {
            console.error('GetUnifiedDiff failed', { path: f.path, baseArg, err });
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
    } catch (err) {
      console.error('GetChangedFilesAgainst failed', { activeWorkspacePath, baseArg, err });
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

  // Listen for file-change events emitted by the watcher (started by App.tsx).
  // We deliberately don't call WatchWorkspace here — duplicating the call from
  // multiple components produced overlapping Stop/Watch cycles and orphaned
  // fsnotify file descriptors.
  useEffect(() => {
    if (!activeWorkspacePath) return;
    const cancel = EventsOn('git:files-changed', () => {
      refresh();
    });
    const refreshFromStatusBar = () => refresh();
    window.addEventListener('orion:git-status-changed', refreshFromStatusBar);
    return () => {
      cancel();
      window.removeEventListener('orion:git-status-changed', refreshFromStatusBar);
    };
  }, [activeWorkspacePath, refresh]);

  const fileQuery = fileSearch.trim().toLowerCase();
  const contentQuery = contentSearch.trim().toLowerCase();

  const filteredEntries = useMemo(() => {
    if (!fileQuery && !contentQuery) return entries;
    return entries.filter((e) => {
      if (fileQuery && !e.file.path.toLowerCase().includes(fileQuery)) return false;
      if (contentQuery) {
        if (!e.diff) return false;
        const hit = e.diff.hunks.some((h) =>
          h.lines.some((l) => l.text.toLowerCase().includes(contentQuery))
        );
        if (!hit) return false;
      }
      return true;
    });
  }, [entries, fileQuery, contentQuery]);

  const tree = useMemo(() => buildTree(filteredEntries), [filteredEntries]);

  const matches = useMemo(() => {
    if (!contentQuery) return [] as { fileIdx: number; hunkIdx: number; lineIdx: number }[];
    const out: { fileIdx: number; hunkIdx: number; lineIdx: number }[] = [];
    filteredEntries.forEach((e, fi) => {
      if (!e.diff) return;
      e.diff.hunks.forEach((h, hi) => {
        h.lines.forEach((l, li) => {
          if (l.text.toLowerCase().includes(contentQuery)) {
            out.push({ fileIdx: fi, hunkIdx: hi, lineIdx: li });
          }
        });
      });
    });
    return out;
  }, [filteredEntries, contentQuery]);

  // Reset match cursor whenever the query or matches change identity
  useEffect(() => { setMatchCursor(0); }, [contentQuery]);
  useEffect(() => {
    if (matches.length === 0) {
      setMatchCursor(0);
    } else {
      setMatchCursor((c) => (c >= matches.length ? 0 : c));
    }
  }, [matches.length]);

  const goToMatch = useCallback((delta: number) => {
    if (matches.length === 0) return;
    const next = ((matchCursor + delta) % matches.length + matches.length) % matches.length;
    setMatchCursor(next);
    const m = matches[next];
    setSelectedIndex(m.fileIdx);
    requestAnimationFrame(() => {
      const key = `${m.fileIdx}:${m.hunkIdx}:${m.lineIdx}`;
      const el = lineRefs.current.get(key);
      if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
  }, [matches, matchCursor]);

  const toggleDir = (path: string) => {
    setCollapsedDirs((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path); else next.add(path);
      return next;
    });
  };

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

  // Clamp selected index to filtered list
  useEffect(() => {
    setSelectedIndex((i) => (filteredEntries.length === 0 ? 0 : Math.min(i, filteredEntries.length - 1)));
  }, [filteredEntries.length]);

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
        if (filteredEntries.length === 0) return;
        const idx = Math.min(selectedIndex, filteredEntries.length - 1);
        const target = filteredEntries[idx];
        const newViewed = !target.viewed;
        if (newViewed) {
          viewedDiffs.current.set(target.file.path, target.rawDiff);
        } else {
          viewedDiffs.current.delete(target.file.path);
        }
        setEntries((prev) =>
          prev.map((ent) =>
            ent.file.path === target.file.path
              ? { ...ent, viewed: newViewed, collapsed: newViewed }
              : ent
          )
        );
        if (newViewed) {
          let next = idx + 1;
          while (next < filteredEntries.length && filteredEntries[next].viewed) next++;
          if (next >= filteredEntries.length) {
            next = filteredEntries.findIndex(
              (ent) => !ent.viewed && ent.file.path !== target.file.path
            );
            if (next === -1) next = idx;
          }
          setSelectedIndex(next);
        }
        return;
      }
      if (e.key === 'j' || e.key === 'ArrowDown') {
        if (!e.metaKey && !e.ctrlKey && !e.altKey) {
          e.preventDefault();
          setSelectedIndex((i) => Math.min(i + 1, filteredEntries.length - 1));
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
  }, [selectedIndex, filteredEntries]);

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

  const renderTreeNode = (node: TreeNode, depth: number): ReactNode => {
    const indent = 6 + depth * 12;
    if (node.kind === 'file') {
      const e = node.entry;
      const isSelected = node.index === selectedIndex;
      return (
        <div
          key={`f:${e.file.path}`}
          className={`cr-tree-row cr-tree-file ${isSelected ? 'selected' : ''} ${e.viewed ? 'viewed' : ''}`}
          style={{ paddingLeft: indent }}
          onClick={() => setSelectedIndex(node.index)}
          title={e.file.path}
        >
          <span className="cr-tree-status" style={{ color: statusColor(e.file.status) }}>
            {e.file.status}
          </span>
          <span className="cr-tree-name">{node.name}</span>
          {e.diff && (e.diff.added > 0 || e.diff.removed > 0) && (
            <span className="cr-tree-counts">
              <span className="cr-add">+{e.diff.added}</span>
              <span className="cr-del">−{e.diff.removed}</span>
            </span>
          )}
        </div>
      );
    }
    const isCollapsed = collapsedDirs.has(node.path);
    return (
      <div key={`d:${node.path}`}>
        <div
          className="cr-tree-row cr-tree-dir"
          style={{ paddingLeft: indent }}
          onClick={() => toggleDir(node.path)}
          title={node.path}
        >
          <span className="cr-tree-chevron">{isCollapsed ? '▸' : '▾'}</span>
          <span className="cr-tree-name">{node.name}</span>
          <span className="cr-tree-counts">
            <span className="cr-tree-filecount">{node.fileCount}</span>
          </span>
        </div>
        {!isCollapsed && node.children.map((c) => renderTreeNode(c, depth + 1))}
      </div>
    );
  };

  return (
    <div className="code-review-pane">
      <div className="cr-header">
        <button
          className={`cr-icon-btn cr-files-toggle ${filesPanelVisible ? 'active' : ''}`}
          onClick={toggleFilesPanel}
          title={filesPanelVisible ? 'Hide files panel' : 'Show files panel'}
        >
          ◧
        </button>
        <span className="cr-title">Code Review</span>
        <span className="cr-workspace" title={activeWorkspacePath || 'no active workspace'}>
          {activeWorkspaceLabel}
        </span>
        <select
          className="cr-base-select"
          value={codeReviewBase}
          onChange={(e) => { setCodeReviewBase(e.target.value as 'uncommitted' | 'main'); e.target.blur(); }}
        >
          <option value="uncommitted">Uncommitted changes</option>
          <option value="main">vs {project?.mainBranch || 'main'}</option>
        </select>
        <div className="cr-content-search-wrap">
          <input
            type="text"
            className="cr-search-input"
            placeholder="Search changes…"
            value={contentSearch}
            onChange={(e) => setContentSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                if (matches.length === 0) return;
                goToMatch(e.shiftKey ? -1 : 1);
              }
            }}
            style={contentSearch ? { paddingRight: 64 } : undefined}
          />
          {contentSearch && (
            <span className="cr-search-count" title="Enter / Shift+Enter to navigate">
              {matches.length === 0 ? '0/0' : `${matchCursor + 1}/${matches.length}`}
            </span>
          )}
          {contentSearch && (
            <button
              type="button"
              className="cr-search-clear"
              onClick={() => setContentSearch('')}
              title="Clear search"
            >
              ✕
            </button>
          )}
        </div>
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

      <div className="cr-main">
        {filesPanelVisible && (
        <div className="cr-files-panel" style={{ width: filesPanelWidth }}>
          <div className="cr-files-search">
            <div className="cr-content-search-wrap">
              <input
                type="text"
                className="cr-search-input"
                placeholder="Search files…"
                value={fileSearch}
                onChange={(e) => setFileSearch(e.target.value)}
              />
              {fileSearch && (
                <button
                  type="button"
                  className="cr-search-clear"
                  onClick={() => setFileSearch('')}
                  title="Clear search"
                >
                  ✕
                </button>
              )}
            </div>
          </div>
          <div className="cr-files-tree">
            {tree.length === 0 ? (
              <div className="cr-empty cr-empty-tree">
                {entries.length === 0 ? 'No changes' : 'No matches'}
              </div>
            ) : (
              tree.map((n) => renderTreeNode(n, 0))
            )}
          </div>
        </div>
        )}
        {filesPanelVisible && (
        <div
          className="cr-files-resizer"
          onMouseDown={(e) => {
            e.preventDefault();
            const startX = e.clientX;
            const startW = filesPanelWidth;
            const onMove = (me: MouseEvent) => {
              persistFilesPanelWidth(startW + (me.clientX - startX));
            };
            const onUp = () => {
              document.removeEventListener('mousemove', onMove);
              document.removeEventListener('mouseup', onUp);
              document.body.style.cursor = '';
              document.body.style.userSelect = '';
            };
            document.addEventListener('mousemove', onMove);
            document.addEventListener('mouseup', onUp);
            document.body.style.cursor = 'col-resize';
            document.body.style.userSelect = 'none';
          }}
        />
        )}
        <div className="cr-body">
          {filteredEntries.length === 0 && !loading && (
            <div className="cr-empty">
              {entries.length === 0 ? 'No changes' : 'No matches'}
            </div>
          )}
          {filteredEntries.map(({ file, diff, collapsed, viewed }, idx) => {
            const effectiveCollapsed = collapsed && !contentQuery;
            return (
              <div
                className={`cr-file-card ${viewed ? 'cr-file-viewed' : ''} ${idx === selectedIndex ? 'cr-file-selected' : ''}`}
                key={file.path}
                ref={(el) => { if (el) fileRefs.current.set(idx, el); else fileRefs.current.delete(idx); }}
                onClick={() => setSelectedIndex(idx)}
              >
                <div className="cr-file-header" onClick={() => toggleCollapse(file.path)}>
                  <span className="cr-chevron">{effectiveCollapsed ? '▸' : '▾'}</span>
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
                {!effectiveCollapsed && diff && diff.hunks.length > 0 && (
                  <div className="cr-hunks">
                    {diff.hunks.map((hunk, hi) => (
                      <div className="cr-hunk" key={hi}>
                        <div className="cr-hunk-header">{hunk.header}</div>
                        {hunk.lines.map((line, li) => {
                          const cm = matches[matchCursor];
                          const isCurrent = cm && cm.fileIdx === idx && cm.hunkIdx === hi && cm.lineIdx === li;
                          const lineKey = `${idx}:${hi}:${li}`;
                          return (
                            <div
                              className={`cr-line cr-${line.kind} ${isCurrent ? 'cr-line-current-match' : ''}`}
                              key={li}
                              ref={(el) => {
                                if (el) lineRefs.current.set(lineKey, el);
                                else lineRefs.current.delete(lineKey);
                              }}
                            >
                              <span className="cr-gutter cr-gutter-old">
                                {line.kind === 'add' ? '' : line.oldNum ?? ''}
                              </span>
                              <span className="cr-gutter cr-gutter-new">
                                {line.kind === 'del' ? '' : line.newNum ?? ''}
                              </span>
                              <span className="cr-sign">
                                {line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '}
                              </span>
                              <span className="cr-text">{renderHighlighted(line.text, contentQuery)}</span>
                            </div>
                          );
                        })}
                      </div>
                    ))}
                  </div>
                )}
                {!effectiveCollapsed && diff && diff.hunks.length === 0 && (
                  <div className="cr-empty cr-empty-file">(no textual diff)</div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
