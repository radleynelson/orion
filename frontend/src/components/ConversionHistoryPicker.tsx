import { useCallback, useEffect, useMemo, useState } from 'react';
import AgentSigil from './AgentSigil';

export type ConversionHistoryCandidate = {
  threadId: string;
  updatedAt: string;
  messageCount: number;
  preview?: string;
  model?: string;
};

interface Props {
  visible: boolean;
  kind: 'claude' | 'codex';
  workspacePath: string;
  error?: string;
  candidates: ConversionHistoryCandidate[];
  busy?: boolean;
  onClose: () => void;
  onPick: (threadId: string) => void;
}

export default function ConversionHistoryPicker({
  visible,
  kind,
  workspacePath,
  error,
  candidates,
  busy = false,
  onClose,
  onPick,
}: Props) {
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    if (!visible) return;
    setSelectedIndex(0);
  }, [visible, candidates.length]);

  const selected = candidates[selectedIndex];
  const title = kind === 'claude' ? 'Choose Claude History' : 'Choose Codex History';
  const workspaceName = useMemo(() => workspacePath.split('/').filter(Boolean).pop() || workspacePath, [workspacePath]);

  const pickSelected = useCallback(() => {
    if (!selected || busy) return;
    onPick(selected.threadId);
  }, [busy, onPick, selected]);

  useEffect(() => {
    if (!visible) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        setSelectedIndex((index) => candidates.length === 0 ? 0 : (index + 1) % candidates.length);
        return;
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault();
        setSelectedIndex((index) => candidates.length === 0 ? 0 : (index - 1 + candidates.length) % candidates.length);
        return;
      }
      if (event.key === 'Enter') {
        event.preventDefault();
        pickSelected();
      }
    };
    window.addEventListener('keydown', onKey, { capture: true });
    return () => window.removeEventListener('keydown', onKey, { capture: true });
  }, [candidates.length, onClose, pickSelected, visible]);

  if (!visible) return null;

  return (
    <div className="search-overlay conversion-picker-overlay" onClick={onClose}>
      <div
        className="conversion-picker"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="conversion-picker-header">
          <div className="conversion-picker-title-row">
            <AgentSigil id={kind} size={26} strong />
            <div className="conversion-picker-heading">
              <div className="conversion-picker-title">{title}</div>
              <div className="conversion-picker-subtitle">{workspaceName}</div>
            </div>
          </div>
          <button type="button" className="conversion-picker-close" onClick={onClose} aria-label="Close">×</button>
        </div>

        {error && <div className="conversion-picker-error">{error}</div>}

        <div className="conversion-picker-list" role="listbox" aria-label="Saved histories">
          {candidates.map((candidate, index) => (
            <button
              key={candidate.threadId}
              type="button"
              className={`conversion-history-row ${index === selectedIndex ? 'selected' : ''}`}
              role="option"
              aria-selected={index === selectedIndex}
              disabled={busy}
              onMouseEnter={() => setSelectedIndex(index)}
              onClick={() => onPick(candidate.threadId)}
            >
              <span className="conversion-history-dot" />
              <span className="conversion-history-copy">
                <span className="conversion-history-title">{candidate.preview || shortThread(candidate.threadId)}</span>
                <span className="conversion-history-meta">
                  <span>{shortThread(candidate.threadId)}</span>
                  <span>{candidate.messageCount} messages</span>
                  {candidate.model && <span>{candidate.model}</span>}
                  <span>{formatUpdated(candidate.updatedAt)}</span>
                </span>
              </span>
            </button>
          ))}
        </div>

        <div className="conversion-picker-actions">
          <button type="button" className="conversion-picker-button secondary" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="button" className="conversion-picker-button primary" onClick={pickSelected} disabled={!selected || busy}>
            {busy ? 'Resuming...' : 'Resume selected'}
          </button>
        </div>
      </div>
    </div>
  );
}

function shortThread(threadId: string) {
  const id = threadId.trim();
  if (id.length <= 12) return id;
  return `...${id.slice(-8)}`;
}

function formatUpdated(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return 'recent';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(parsed);
}
