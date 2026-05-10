import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { GetGitStatus, GitFetch, GitPull, GitPush } from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { git } from '../../wailsjs/go/models';

type GitAction = 'fetch' | 'pull' | 'push' | 'sync';
type GuardKind = 'dirty' | 'conflict' | 'auth';

interface Props {
  workspacePath?: string;
  fallbackBranch?: string;
  onOpenChanges?: () => void;
}

function GitIcon({ name, size = 13 }: { name: 'branch' | 'sync' | 'down' | 'up' | 'warn' | 'check' | 'dot' | 'plus'; size?: number }) {
  const props = {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.8,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    'aria-hidden': true,
  };
  switch (name) {
    case 'branch':
      return <svg {...props}><circle cx="6" cy="5" r="2" /><circle cx="6" cy="19" r="2" /><circle cx="18" cy="7" r="2" /><path d="M6 7v10" /><path d="M18 9c0 4-6 4-6 8" /></svg>;
    case 'sync':
      return <svg {...props}><path d="M4 12a8 8 0 0 1 14-5.3" /><path d="M18 3v4h-4" /><path d="M20 12a8 8 0 0 1-14 5.3" /><path d="M6 21v-4h4" /></svg>;
    case 'down':
      return <svg {...props}><path d="M12 4v14" /><path d="M6 13l6 6 6-6" /></svg>;
    case 'up':
      return <svg {...props}><path d="M12 20V6" /><path d="M6 11l6-6 6 6" /></svg>;
    case 'warn':
      return <svg {...props}><path d="M12 3l10 17H2L12 3z" /><path d="M12 10v5" /><circle cx="12" cy="18" r="0.7" fill="currentColor" /></svg>;
    case 'check':
      return <svg {...props}><path d="M5 12l5 5 9-11" /></svg>;
    case 'dot':
      return <svg {...props} fill="currentColor" stroke="none"><circle cx="12" cy="12" r="4" /></svg>;
    case 'plus':
      return <svg {...props}><path d="M12 5v14M5 12h14" /></svg>;
  }
}

function firstUsefulLine(output: string | undefined, fallback: string): string {
  const line = (output || '').split('\n').map((part) => part.trim()).find(Boolean);
  return line || fallback;
}

function dirtyLabel(status: git.RepositoryStatus | null): string {
  const count = status?.changeCount || 0;
  return `${count} uncommitted change${count === 1 ? '' : 's'}`;
}

function errorMessage(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err);
  return raw.replace(/^Error:\s*/, '');
}

export default function GitStatusControl({ workspacePath, fallbackBranch, onOpenChanges }: Props) {
  const [status, setStatus] = useState<git.RepositoryStatus | null>(null);
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState<GitAction | null>(null);
  const [notice, setNotice] = useState<{ tone: 'ok' | 'error'; text: string } | null>(null);
  const [guard, setGuard] = useState<GuardKind | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);

  const refreshStatus = useCallback(async () => {
    if (!workspacePath) {
      setStatus(null);
      return;
    }
    try {
      setStatus(await GetGitStatus(workspacePath));
    } catch (err) {
      console.error('GetGitStatus failed', err);
      setStatus(null);
    }
  }, [workspacePath]);

  useEffect(() => {
    void refreshStatus();
  }, [refreshStatus]);

  useEffect(() => {
    if (!workspacePath) return;
    const cancelGitEvents = EventsOn('git:files-changed', () => {
      void refreshStatus();
    });
    const refresh = () => void refreshStatus();
    window.addEventListener('orion:git-status-changed', refresh);
    return () => {
      cancelGitEvents();
      window.removeEventListener('orion:git-status-changed', refresh);
    };
  }, [workspacePath, refreshStatus]);

  useEffect(() => {
    if (!open && !guard) return;
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return;
      setOpen(false);
      setGuard(null);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
        setGuard(null);
      }
    };
    window.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [open, guard]);

  const branchLabel = status?.branch || fallbackBranch || 'detached';
  const upstreamLabel = status?.upstream || (status?.detached ? 'detached HEAD' : 'no upstream');
  const hasDirtyTree = !!status?.hasChanges;
  const pullBlockReason = useMemo(() => {
    if (!status) return 'Git status unavailable';
    if (!status.upstream) return 'No upstream branch';
    if (status.hasChanges) return `${dirtyLabel(status)} - stash or commit first`;
    return '';
  }, [status]);
  const pushBlockReason = useMemo(() => {
    if (!status) return 'Git status unavailable';
    if (status.detached || !status.branch) return 'Cannot push detached HEAD';
    if (status.upstream && status.ahead === 0) return 'Nothing to push';
    return '';
  }, [status]);
  const syncBlockReason = pullBlockReason || (status?.detached ? 'Cannot push detached HEAD' : '');

  const openChanges = useCallback(() => {
    onOpenChanges?.();
    setOpen(false);
    setGuard(null);
  }, [onOpenChanges]);

  const runAction = useCallback(async (action: GitAction) => {
    if (!workspacePath || pending) return;
    if ((action === 'pull' || action === 'sync') && hasDirtyTree) {
      setGuard('dirty');
      setNotice(null);
      return;
    }
    setPending(action);
    setNotice(null);
    setGuard(null);
    try {
      let result: git.ActionResult;
      if (action === 'fetch') {
        result = await GitFetch(workspacePath);
        setNotice({ tone: 'ok', text: firstUsefulLine(result.output, 'Fetched latest refs.') });
      } else if (action === 'pull') {
        result = await GitPull(workspacePath);
        setNotice({ tone: 'ok', text: firstUsefulLine(result.output, 'Already up to date.') });
      } else if (action === 'push') {
        result = await GitPush(workspacePath);
        setNotice({ tone: 'ok', text: firstUsefulLine(result.output, 'Pushed current branch.') });
      } else {
        const pulled = await GitPull(workspacePath);
        const nextStatus = pulled.status || status;
        if (nextStatus?.canPush && (!nextStatus.upstream || nextStatus.ahead > 0)) {
          result = await GitPush(workspacePath);
          setNotice({ tone: 'ok', text: firstUsefulLine(result.output, 'Synced branch.') });
        } else {
          result = pulled;
          setNotice({ tone: 'ok', text: firstUsefulLine(pulled.output, 'Already up to date.') });
        }
      }
      setStatus(result.status || null);
      window.dispatchEvent(new CustomEvent('orion:git-status-changed'));
    } catch (err) {
      const text = errorMessage(err);
      setNotice({ tone: 'error', text });
      if (/conflict|would be overwritten|ff-only|non-fast-forward/i.test(text)) setGuard('conflict');
      if (/auth|permission|403|denied/i.test(text)) setGuard('auth');
      await refreshStatus();
    } finally {
      setPending(null);
    }
  }, [workspacePath, pending, hasDirtyTree, status, refreshStatus]);

  const actionRows = [
    { action: 'fetch' as const, icon: 'sync' as const, label: 'Fetch', shortcut: '⌘⇧F', hint: 'Check what changed upstream' },
    { action: 'pull' as const, icon: 'down' as const, label: status && status.behind > 0 ? `Pull (${status.behind})` : 'Pull', shortcut: '⌘⇧P', hint: pullBlockReason || 'Fast-forward only', disabled: !!pullBlockReason },
    { action: 'push' as const, icon: 'up' as const, label: status && status.ahead > 0 ? `Push (${status.ahead})` : 'Push', shortcut: '⌘⇧K', hint: pushBlockReason || (status?.upstream ? 'Push committed changes' : 'Set upstream and push'), disabled: !!pushBlockReason },
    { action: 'sync' as const, icon: 'sync' as const, label: 'Sync', hint: syncBlockReason || 'Pull, then push if needed', disabled: !!syncBlockReason },
  ];

  return (
    <div className="git-status-root" ref={rootRef}>
      {guard && (
        <div className={`git-inline-guard ${guard}`}>
          <GitIcon name="warn" size={15} />
          <div className="git-inline-guard-copy">
            <strong>
              {guard === 'dirty' && `Can't pull - ${dirtyLabel(status)}`}
              {guard === 'conflict' && 'Pull would conflict'}
              {guard === 'auth' && 'Git authentication failed'}
            </strong>
            <span>
              {guard === 'dirty' && 'A clean tree is required for fast-forward pull. Stash or commit first.'}
              {guard === 'conflict' && 'Fetch succeeded, but applying upstream changes needs manual review.'}
              {guard === 'auth' && 'Git could not authenticate with the remote.'}
            </span>
          </div>
          <div className="git-inline-guard-actions">
            {guard === 'dirty' && (
              <button type="button" onClick={openChanges}>Open changes</button>
            )}
            <button type="button" onClick={() => setGuard(null)}>Dismiss</button>
          </div>
        </div>
      )}

      {open && (
        <div className="git-branch-popover" role="menu">
          <div className="git-popover-head">
            <GitIcon name="branch" size={15} />
            <div>
              <strong>{branchLabel}</strong>
              <span>{upstreamLabel} · ↑{status?.ahead || 0} ↓{status?.behind || 0}</span>
            </div>
          </div>
          <div className="git-popover-actions">
            {actionRows.map((row) => (
              <button
                key={row.action}
                type="button"
                className="git-popover-action"
                disabled={row.disabled || pending !== null}
                onClick={() => void runAction(row.action)}
              >
                <span className={pending === row.action ? 'spinning' : ''}><GitIcon name={row.icon} size={14} /></span>
                <span>
                  <strong>{pending === row.action ? `${row.label}…` : row.label}</strong>
                  <em className={row.disabled ? 'blocked' : ''}>{row.hint}</em>
                </span>
                {row.shortcut && <kbd>{row.shortcut}</kbd>}
              </button>
            ))}
          </div>
          <div className="git-popover-actions secondary">
            <button type="button" className="git-popover-action" disabled>
              <GitIcon name="branch" size={14} />
              <span><strong>Switch branch…</strong><em>Coming soon</em></span>
            </button>
            <button type="button" className="git-popover-action" disabled>
              <GitIcon name="plus" size={14} />
              <span><strong>Create branch…</strong><em>Coming soon</em></span>
            </button>
            {status?.hasChanges && (
              <button type="button" className="git-popover-action" onClick={openChanges}>
                <GitIcon name="dot" size={10} />
                <span><strong>Open {dirtyLabel(status)}</strong><em>Review local changes</em></span>
              </button>
            )}
          </div>
          <div className={`git-popover-foot ${notice?.tone || ''}`}>
            {notice ? notice.text : 'Fetch checks remotes without changing your workspace.'}
          </div>
        </div>
      )}

      <button type="button" className="git-status-segment branch" onClick={() => setOpen((value) => !value)} title={workspacePath || 'No workspace selected'}>
        <GitIcon name="branch" size={12} />
        <span>{branchLabel}</span>
        {status?.hasChanges && <em title={dirtyLabel(status)}>●{status.changeCount || ''}</em>}
      </button>
      <button type="button" className="git-status-segment sync" onClick={() => void runAction('fetch')} disabled={!workspacePath || pending !== null} title="Fetch latest remote refs">
        <GitIcon name="down" size={11} />
        <span>{status?.behind || 0}</span>
        <GitIcon name="up" size={11} />
        <span>{status?.ahead || 0}</span>
        <span className={pending === 'fetch' ? 'spinning' : ''}><GitIcon name="sync" size={12} /></span>
      </button>
      {notice && !open && (
        <span className={`git-status-notice ${notice.tone}`} title={notice.text}>
          {notice.text}
        </span>
      )}
    </div>
  );
}
