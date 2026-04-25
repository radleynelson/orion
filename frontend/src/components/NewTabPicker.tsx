import { useCallback, useEffect, useState } from 'react';
import { GetAgentTypes } from '../../wailsjs/go/main/App';
import { main } from '../../wailsjs/go/models';
import { useStore } from '../store';
import AgentSigil from './AgentSigil';

export type NewTabChoice = { kind: 'shell' } | { kind: 'agent'; name: string; label: string };

interface Props {
  visible: boolean;
  onClose: () => void;
  onPick: (choice: NewTabChoice) => void;
}

export default function NewTabPicker({ visible, onClose, onPick }: Props) {
  const project = useStore((s) => s.project);
  const [agents, setAgents] = useState<main.AgentTypeInfo[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);

  const options: NewTabChoice[] = [
    ...agents.map((a) => ({ kind: 'agent' as const, name: a.name, label: a.label })),
    { kind: 'shell' as const },
  ];

  useEffect(() => {
    if (!visible || !project) return;
    GetAgentTypes(project.root).then(setAgents).catch(() => setAgents([]));
    setSelectedIndex(0);
  }, [visible, project]);

  const pick = useCallback((choice: NewTabChoice) => {
    onPick(choice);
    onClose();
  }, [onPick, onClose]);

  useEffect(() => {
    if (!visible) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.preventDefault(); onClose(); return; }
      if (e.key === 'ArrowDown') { e.preventDefault(); setSelectedIndex((i) => (i + 1) % options.length); return; }
      if (e.key === 'ArrowUp')   { e.preventDefault(); setSelectedIndex((i) => (i - 1 + options.length) % options.length); return; }
      if (e.key === 'Enter')     { e.preventDefault(); if (options[selectedIndex]) pick(options[selectedIndex]); return; }
      // Number shortcuts 1..N
      const num = parseInt(e.key, 10);
      if (!isNaN(num) && num >= 1 && num <= options.length) {
        e.preventDefault();
        pick(options[num - 1]);
        return;
      }
      // First-letter mnemonics
      const key = e.key.toLowerCase();
      const byLetter = options.findIndex((o) => labelFor(o).toLowerCase().startsWith(key));
      if (byLetter >= 0) { e.preventDefault(); pick(options[byLetter]); }
    };
    window.addEventListener('keydown', onKey, { capture: true });
    return () => window.removeEventListener('keydown', onKey, { capture: true });
  }, [visible, options, selectedIndex, pick, onClose]);

  if (!visible) return null;

  return (
    <div className="search-overlay" onClick={onClose}>
      <div className="switcher-modal new-tab-modal" onClick={(e) => e.stopPropagation()}>
        <div className="switcher-title">New Tab</div>
        {options.map((opt, i) => (
          <div
            key={keyFor(opt)}
            className={`switcher-item ${i === selectedIndex ? 'selected' : ''}`}
            onClick={() => pick(opt)}
          >
            <span className="switcher-icon"><AgentSigil id={idFor(opt)} size={22} /></span>
            <span className="switcher-name">{labelFor(opt)}</span>
            <span className="switcher-current" style={{ opacity: 0.6 }}>{i + 1}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function labelFor(o: NewTabChoice) {
  return o.kind === 'shell' ? 'Shell' : o.label;
}
function keyFor(o: NewTabChoice) {
  return o.kind === 'shell' ? 'shell' : `agent:${o.name}`;
}
function idFor(o: NewTabChoice) {
  return o.kind === 'shell' ? 'shell' : o.name;
}
