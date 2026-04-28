import { useEffect, useState, useRef, useCallback } from 'react';
import { GetMemorySnapshot, GetTmuxSession, KillSession } from '../../wailsjs/go/main/App';
import { diag } from '../../wailsjs/go/models';
import { useStore, Tab } from '../store';

const REFRESH_MS = 2000;

function fmtMB(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(2)} GB`;
  if (mb >= 10) return `${mb.toFixed(0)} MB`;
  return `${mb.toFixed(1)} MB`;
}

function fmtPct(p: number): string {
  if (p < 0.1) return '';
  return `${p.toFixed(0)}%`;
}

function workspaceName(path: string): string {
  if (!path) return '';
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || path;
}

interface TabBinding {
  tabId: string;
  label: string;
  workspacePath: string;
  isServer: boolean;
}

// Human-readable fallback parse for sessions with no matching Orion tab.
function parseSessionName(name: string): { kind: string; label: string; hint: string } {
  if (name.startsWith('orion-srv-')) {
    const rest = name.slice('orion-srv-'.length);
    const idx = rest.lastIndexOf('-');
    if (idx > 0) {
      const wsID = rest.slice(0, idx);
      const srv = rest.slice(idx + 1);
      return { kind: 'server', label: `${srv}`, hint: wsID };
    }
    return { kind: 'server', label: rest, hint: '' };
  }
  if (name.startsWith('orion-shell-')) return { kind: 'shell', label: 'Shell', hint: '' };
  if (name.startsWith('orion-web-')) return { kind: 'web', label: 'Mobile session', hint: '' };
  // Agent session: orion-<repo>-<ws>[-N]
  const rest = name.slice('orion-'.length);
  return { kind: 'agent', label: rest, hint: '' };
}

const KIND_COLORS: Record<string, string> = {
  server: 'var(--accent-green, #6bcf7f)',
  agent: 'var(--accent-purple, #c57bdb)',
  shell: 'var(--accent-blue)',
  web: 'var(--accent-orange, #e8a547)',
};

function Bar({ value, max, color }: { value: number; max: number; color: string }) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  return (
    <div className="diag-bar">
      <div className="diag-bar-fill" style={{ width: `${pct}%`, background: color }} />
    </div>
  );
}

export default function DiagnosticsPage({ visible }: { visible: boolean }) {
  const [snap, setSnap] = useState<diag.MemorySnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [paused, setPaused] = useState(false);
  const [tmuxToTab, setTmuxToTab] = useState<Map<string, TabBinding>>(new Map());
  const [confirmKill, setConfirmKill] = useState<string | null>(null);
  const [expandedSession, setExpandedSession] = useState<Set<string>>(new Set());
  const inFlight = useRef(false);

  const tabs = useStore((s) => s.tabs);
  const serverTabs = useStore((s) => s.serverTabs);
  const getAllTerminalIds = useStore((s) => s.getAllTerminalIds);

  // Build the tmux session → tab binding map whenever tabs change.
  useEffect(() => {
    let cancelled = false;
    const build = async () => {
      const map = new Map<string, TabBinding>();
      const all: Array<Tab & { isServer: boolean }> = [
        ...tabs.map((t) => ({ ...t, isServer: false })),
        ...serverTabs.map((t) => ({ ...t, isServer: true })),
      ];
      for (const tab of all) {
        const termIds = getAllTerminalIds(tab);
        for (const termId of termIds) {
          try {
            const s = await GetTmuxSession(termId);
            if (s) {
              map.set(s, {
                tabId: tab.id,
                label: tab.label,
                workspacePath: tab.workspacePath,
                isServer: tab.isServer,
              });
            }
          } catch {}
        }
      }
      if (!cancelled) setTmuxToTab(map);
    };
    build();
    return () => { cancelled = true; };
  }, [tabs, serverTabs, getAllTerminalIds]);

  // Poll the backend for a snapshot. Pause when hidden or user paused.
  useEffect(() => {
    if (!visible || paused) return;
    let cancelled = false;
    const tick = async () => {
      if (inFlight.current) return;
      inFlight.current = true;
      try {
        const s = await GetMemorySnapshot();
        if (!cancelled) {
          setSnap(s);
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled) setError(String(e?.message || e));
      } finally {
        inFlight.current = false;
      }
    };
    tick();
    const id = window.setInterval(tick, REFRESH_MS);
    return () => { cancelled = true; window.clearInterval(id); };
  }, [visible, paused]);

  const gotoSession = useCallback((sessionName: string) => {
    const binding = tmuxToTab.get(sessionName);
    if (!binding) return;
    const state = useStore.getState();
    if (binding.workspacePath && binding.workspacePath !== state.activeWorkspacePath) {
      state.setActiveWorkspace(binding.workspacePath);
    }
    if (binding.isServer) {
      state.setActiveServerTab(binding.tabId);
      state.setServerPaneVisible(true);
    } else {
      state.setActiveTab(binding.tabId);
    }
  }, [tmuxToTab]);

  const doKill = useCallback(async (sessionName: string) => {
    setConfirmKill(null);
    try {
      await KillSession(sessionName);
      // If a matching Orion tab existed, remove it. CloseTerminal was called
      // already by the backend if attached, but the React tab state still has it.
      const binding = tmuxToTab.get(sessionName);
      if (binding) {
        const state = useStore.getState();
        if (binding.isServer) state.removeServerTab(binding.tabId);
        else state.removeTab(binding.tabId);
      }
      // Force a fresh snapshot right away so the killed row vanishes.
      try {
        const s = await GetMemorySnapshot();
        setSnap(s);
      } catch {}
    } catch (e) {
      console.error('kill session failed:', e);
    }
  }, [tmuxToTab]);

  const toggleExpand = (name: string) => {
    setExpandedSession((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  };

  if (!snap && !error) {
    return <div className="diag-page"><div className="diag-empty">Loading memory snapshot…</div></div>;
  }
  if (error && !snap) {
    return <div className="diag-page"><div className="diag-empty diag-error">Error: {error}</div></div>;
  }

  const t = snap!.totals;
  const sessions = snap!.sessions || [];
  const webview = snap!.webview || [];
  const helpers = snap!.helpers || [];

  // Pick a shared bar scale across sections so biggest offenders pop.
  const barMax = Math.max(
    snap!.orion.rssMB,
    ...sessions.map((s) => s.totalRSSMB),
    ...webview.map((p) => p.rssMB),
    ...helpers.map((p) => p.rssMB),
    1,
  );

  return (
    <div className="diag-page">
      <div className="diag-page-header">
        <div className="diag-page-title">
          <span className="diag-page-icon">◎</span>
          <span>Diagnostics</span>
        </div>
        <div className="diag-page-total">
          <span className="diag-page-total-label">Total tracked</span>
          <span className="diag-page-total-val">{fmtMB(t.grandMB)}</span>
        </div>
        <div className="diag-page-actions">
          <button
            className={`diag-btn ${paused ? 'diag-btn-paused' : ''}`}
            onClick={() => setPaused(!paused)}
            title={paused ? 'Resume polling' : 'Pause polling'}
          >
            {paused ? '▸ Resume' : '❚❚ Pause'}
          </button>
        </div>
      </div>

      <div className="diag-page-summary">
        <SummaryCard label="Sessions" value={t.sessionsMB} color="var(--accent-purple, #c57bdb)" count={sessions.length} />
        <SummaryCard label="WebView" value={t.webviewMB} color="var(--accent-blue)" count={webview.length} />
        <SummaryCard label="Orion helpers" value={t.helpersMB} color="var(--accent-orange, #e8a547)" count={helpers.length} />
        <SummaryCard label="Orion main" value={t.orionMB} color="var(--text-secondary)" count={1} />
      </div>

      {/* Tmux sessions - top so user can spot hogs fast */}
      <Section title={`Tmux sessions (${sessions.length})`} hint="click a row to jump to its tab · ⨯ to kill">
        {sessions.length === 0 ? (
          <div className="diag-empty">No orion-* tmux sessions running.</div>
        ) : (
          <div className="diag-session-list">
            {sessions.map((s) => {
              const binding = tmuxToTab.get(s.sessionName);
              const parsed = parseSessionName(s.sessionName);
              const kind = binding ? (s.kind || parsed.kind) : parsed.kind;
              const color = KIND_COLORS[kind] || 'var(--accent-blue)';
              const label = binding ? binding.label : parsed.label;
              const wsHint = binding
                ? workspaceName(binding.workspacePath)
                : parsed.hint;
              const expanded = expandedSession.has(s.sessionName);
              const pending = confirmKill === s.sessionName;
              return (
                <div key={s.sessionName} className="diag-session-row">
                  <div
                    className={`diag-session-main ${binding ? 'diag-clickable' : ''}`}
                    onClick={() => binding && gotoSession(s.sessionName)}
                    title={binding ? `Jump to ${label} in ${wsHint} · ${s.sessionName}` : s.sessionName}
                  >
                    <div className="diag-session-info">
                      <div className="diag-session-title-line">
                        <span className="diag-kind-badge" style={{ color, borderColor: color }}>
                          {kind}
                        </span>
                        <span className="diag-session-label">{label}</span>
                        {wsHint && <span className="diag-session-ws">· {wsHint}</span>}
                        {!binding && <span className="diag-session-orphan">orphaned</span>}
                      </div>
                      {!binding && (
                        <div className="diag-session-subname">{s.sessionName}</div>
                      )}
                    </div>
                    <div className="diag-session-meta">
                      <span className="diag-session-procs">{s.processes.length} proc</span>
                      <span className="diag-session-rss">{fmtMB(s.totalRSSMB)}</span>
                    </div>
                  </div>
                  <div className="diag-session-bar">
                    <Bar value={s.totalRSSMB} max={barMax} color={color} />
                  </div>
                  <div className="diag-session-actions">
                    <button
                      className="diag-btn diag-btn-ghost"
                      onClick={(e) => { e.stopPropagation(); toggleExpand(s.sessionName); }}
                    >
                      {expanded ? '▾ Hide procs' : '▸ Show procs'}
                    </button>
                    {pending ? (
                      <>
                        <button
                          className="diag-btn diag-btn-danger"
                          onClick={(e) => { e.stopPropagation(); doKill(s.sessionName); }}
                        >
                          Confirm kill
                        </button>
                        <button
                          className="diag-btn diag-btn-ghost"
                          onClick={(e) => { e.stopPropagation(); setConfirmKill(null); }}
                        >
                          Cancel
                        </button>
                      </>
                    ) : (
                      <button
                        className="diag-btn diag-btn-danger-ghost"
                        onClick={(e) => { e.stopPropagation(); setConfirmKill(s.sessionName); }}
                        title="Kill tmux session (closes the tab too)"
                      >
                        ⨯ Kill
                      </button>
                    )}
                  </div>
                  {expanded && (
                    <div className="diag-session-procs-list">
                      {s.processes.map((p) => (
                        <div key={p.pid} className="diag-proc-row">
                          <span className="diag-proc-name" title={`PID ${p.pid}`}>{p.name}</span>
                          <span className="diag-proc-cpu">{fmtPct(p.cpuPct)}</span>
                          <span className="diag-proc-rss">{fmtMB(p.rssMB)}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </Section>

      {/* Orion main + helpers */}
      <Section title="Orion app">
        <div className="diag-proc-table">
          <ProcRow label="Orion (main)" name={snap!.orion.name} pid={snap!.orion.pid} rssMB={snap!.orion.rssMB} cpu={snap!.orion.cpuPct} max={barMax} color="var(--text-secondary)" />
          {webview.length > 0 && (
            <>
              <div className="diag-subhead">WebView helpers ({webview.length})</div>
              {webview.map((p) => (
                <ProcRow key={p.pid} name={p.name} pid={p.pid} rssMB={p.rssMB} cpu={p.cpuPct} max={barMax} color="var(--accent-blue)" />
              ))}
            </>
          )}
          {helpers.length > 0 && (
            <>
              <div className="diag-subhead">Other helpers ({helpers.length})</div>
              {helpers.slice(0, 30).map((p) => (
                <ProcRow key={p.pid} name={p.name} pid={p.pid} rssMB={p.rssMB} cpu={p.cpuPct} max={barMax} color="var(--accent-orange, #e8a547)" />
              ))}
              {helpers.length > 30 && (
                <div className="diag-truncate">+ {helpers.length - 30} more (smaller)</div>
              )}
            </>
          )}
        </div>
      </Section>

      {/* File descriptors */}
      {snap!.fds && <FDSection fds={snap!.fds} />}

      {/* Go runtime */}
      <Section title="Go runtime">
        <div className="diag-kv diag-kv-wide">
          <span>Heap alloc</span><b>{fmtMB(snap!.go.heapAllocMB)}</b>
          <span>Heap sys</span><b>{fmtMB(snap!.go.heapSysMB)}</b>
          <span>Stack</span><b>{fmtMB(snap!.go.stackInUseMB)}</b>
          <span>Sys</span><b>{fmtMB(snap!.go.sysMB)}</b>
          <span>Goroutines</span><b>{snap!.go.numGoroutine}</b>
          <span>GC runs</span><b>{snap!.go.numGC}</b>
        </div>
      </Section>
    </div>
  );
}

function FDSection({ fds }: { fds: diag.FDStats }) {
  const [showAll, setShowAll] = useState(false);
  const pct = fds.usagePct || 0;
  const barColor = pct > 80 ? 'var(--accent-red, #e26d6d)' : pct > 50 ? 'var(--accent-orange, #e8a547)' : 'var(--accent-green, #6bcf7f)';
  return (
    <Section
      title={`File descriptors (${fds.count.toLocaleString()} / ${fds.softLimit.toLocaleString()})`}
      hint={fds.error ? `lsof error: ${fds.error}` : `${pct.toFixed(2)}% of soft limit`}
    >
      <div className="diag-fd-bar">
        <div className="diag-bar" style={{ flex: 1 }}>
          <div className="diag-bar-fill" style={{ width: `${Math.min(100, pct)}%`, background: barColor }} />
        </div>
        <span className="diag-fd-pct">{pct.toFixed(2)}%</span>
      </div>
      {fds.byType && fds.byType.length > 0 && (
        <div className="diag-kv diag-kv-wide" style={{ marginTop: 10 }}>
          {fds.byType.map((t) => (
            <span key={t.type} style={{ display: 'contents' }}>
              <span title="lsof type code">{t.type}</span>
              <b>{t.count.toLocaleString()}</b>
            </span>
          ))}
        </div>
      )}
      {fds.groupedDirs && fds.groupedDirs.length > 0 && (
        <>
          <div className="diag-subhead">Top directories (regular files / dirs)</div>
          <div className="diag-fd-dirs">
            {fds.groupedDirs.map((d) => (
              <div key={d.dir} className="diag-fd-dir-row">
                <span className="diag-fd-dir-count">{d.count}</span>
                <span className="diag-fd-dir-path" title={d.dir}>{d.dir}</span>
              </div>
            ))}
          </div>
        </>
      )}
      <button
        className="diag-btn diag-btn-ghost"
        style={{ marginTop: 10 }}
        onClick={() => setShowAll((v) => !v)}
      >
        {showAll ? '▾ Hide full list' : `▸ Show full list (${fds.topEntries?.length || 0}${fds.truncated ? '+' : ''})`}
      </button>
      {showAll && fds.topEntries && (
        <div className="diag-fd-list">
          {fds.topEntries.map((e, i) => (
            <div key={`${e.fd}-${i}`} className="diag-fd-row">
              <span className="diag-fd-fd">{e.fd}</span>
              <span className="diag-fd-type">{e.type}</span>
              <span className="diag-fd-name" title={e.name}>{e.name}</span>
            </div>
          ))}
          {fds.truncated && (
            <div className="diag-truncate">List truncated; showing first {fds.topEntries.length} of {fds.count} entries.</div>
          )}
        </div>
      )}
    </Section>
  );
}

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="diag-page-section">
      <div className="diag-page-section-header">
        <span className="diag-page-section-title">{title}</span>
        {hint && <span className="diag-page-section-hint">{hint}</span>}
      </div>
      <div className="diag-page-section-body">{children}</div>
    </div>
  );
}

function SummaryCard({ label, value, color, count }: { label: string; value: number; color: string; count: number }) {
  return (
    <div className="diag-summary-card">
      <div className="diag-summary-label">{label} <span className="diag-summary-count">({count})</span></div>
      <div className="diag-summary-val" style={{ color }}>{fmtMB(value)}</div>
    </div>
  );
}

function ProcRow({ label, name, pid, rssMB, cpu, max, color }: {
  label?: string; name: string; pid: number; rssMB: number; cpu: number; max: number; color: string;
}) {
  return (
    <div className="diag-proc-row-wide">
      <div className="diag-proc-head">
        <span className="diag-proc-title">
          {label && <b>{label} · </b>}
          <span>{name}</span>
          <span className="diag-proc-pid">  pid {pid}</span>
        </span>
        <span className="diag-proc-nums">
          {cpu >= 0.1 && <span className="diag-proc-cpu">{fmtPct(cpu)}</span>}
          <span className="diag-proc-rss">{fmtMB(rssMB)}</span>
        </span>
      </div>
      <Bar value={rssMB} max={max} color={color} />
    </div>
  );
}
