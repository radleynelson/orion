import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent } from 'react';
import AgentSigil from './AgentSigil';

export interface CommandPaletteItem {
  id: string;
  title: string;
  subtitle?: string;
  group: string;
  icon?: string;
  shortcut?: string;
  keywords?: string[];
  disabled?: boolean;
  run: () => void | Promise<void>;
}

interface CommandPaletteProps {
  visible: boolean;
  commands: CommandPaletteItem[];
  onClose: () => void;
}

export default function CommandPalette({ visible, commands, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const results = useMemo(() => {
    const terms = normalize(query).split(' ').filter(Boolean);
    const filtered = terms.length === 0
      ? commands
      : commands.filter((command) => {
          const haystack = normalize([
            command.title,
            command.subtitle,
            command.group,
            command.shortcut,
            ...(command.keywords || []),
          ].filter(Boolean).join(' '));
          return terms.every((term) => haystack.includes(term));
        });

    return [...filtered].sort((a, b) => scoreCommand(b, terms) - scoreCommand(a, terms));
  }, [commands, query]);

  useEffect(() => {
    if (!visible) return;
    setQuery('');
    setSelectedIndex(0);
    const timer = window.setTimeout(() => inputRef.current?.focus(), 30);
    return () => window.clearTimeout(timer);
  }, [visible]);

  useEffect(() => {
    setSelectedIndex((index) => Math.min(index, Math.max(results.length - 1, 0)));
  }, [results.length]);

  const runCommand = useCallback(async (command?: CommandPaletteItem) => {
    if (!command || command.disabled) return;
    onClose();
    await command.run();
  }, [onClose]);

  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((index) => results.length === 0 ? 0 : (index + 1) % results.length);
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((index) => results.length === 0 ? 0 : (index - 1 + results.length) % results.length);
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      runCommand(results[selectedIndex]);
    }
  }, [onClose, results, selectedIndex, runCommand]);

  if (!visible) return null;

  return (
    <div className="search-overlay command-palette-overlay" onClick={onClose}>
      <div
        className="command-palette"
        role="dialog"
        aria-modal="true"
        aria-label="Command Palette"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="command-palette-header">
          <span className="command-palette-kicker">Orion</span>
          <input
            ref={inputRef}
            className="command-palette-input"
            placeholder="Type a command, workspace, or tab..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
          />
          <span className="command-palette-shortcut">esc</span>
        </div>

        <div className="command-palette-results" role="listbox" aria-label="Commands">
          {results.map((command, index) => (
            <button
              key={command.id}
              type="button"
              className={`command-palette-item ${index === selectedIndex ? 'selected' : ''}`}
              disabled={command.disabled}
              role="option"
              aria-selected={index === selectedIndex}
              onMouseEnter={() => setSelectedIndex(index)}
              onClick={() => runCommand(command)}
            >
              <span className="command-palette-icon">
                <AgentSigil id={command.icon || 'shell'} size={22} />
              </span>
              <span className="command-palette-copy">
                <span className="command-palette-title">{command.title}</span>
                <span className="command-palette-subtitle">
                  <span>{command.group}</span>
                  {command.subtitle && <span>{command.subtitle}</span>}
                </span>
              </span>
              {command.shortcut && <span className="command-palette-key">{command.shortcut}</span>}
            </button>
          ))}
          {results.length === 0 && (
            <div className="command-palette-empty">No commands found</div>
          )}
        </div>
      </div>
    </div>
  );
}

function normalize(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
}

function scoreCommand(command: CommandPaletteItem, terms: string[]) {
  if (terms.length === 0) return 0;
  const title = normalize(command.title);
  const group = normalize(command.group);
  let score = 0;
  for (const term of terms) {
    if (title.startsWith(term)) score += 8;
    if (title.includes(term)) score += 4;
    if (group.includes(term)) score += 2;
    if ((command.keywords || []).some((keyword) => normalize(keyword).includes(term))) score += 3;
  }
  return score;
}
