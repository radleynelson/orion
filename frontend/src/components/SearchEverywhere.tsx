import { useState, useEffect, useRef, useCallback } from 'react';
import { useStore } from '../store';
import { SearchFiles } from '../../wailsjs/go/main/App';
import { getLanguageFromPath } from '../lib/languages';
import { files } from '../../wailsjs/go/models';

interface SearchEverywhereProps {
  visible: boolean;
  onClose: () => void;
}

export default function SearchEverywhere({ visible, onClose }: SearchEverywhereProps) {
  const { activeWorkspacePath, openFile } = useStore();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<files.SearchResult[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // Focus input when modal opens
  useEffect(() => {
    if (visible) {
      setQuery('');
      setResults([]);
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [visible]);

  // Search as user types
  useEffect(() => {
    if (!visible || !activeWorkspacePath || !query.trim()) {
      setResults([]);
      return;
    }
    const timer = setTimeout(async () => {
      try {
        const r = await SearchFiles(activeWorkspacePath, query.trim());
        setResults(r || []);
        setSelectedIndex(0);
      } catch {}
    }, 100); // debounce
    return () => clearTimeout(timer);
  }, [query, activeWorkspacePath, visible]);

  const handleSelect = useCallback((result: files.SearchResult) => {
    const fullPath = activeWorkspacePath + '/' + result.path;
    const language = getLanguageFromPath(result.path);
    openFile(fullPath, language);
    onClose();
  }, [activeWorkspacePath, openFile, onClose]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((i) => Math.min(i + 1, results.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter' && results.length > 0) {
      e.preventDefault();
      handleSelect(results[selectedIndex]);
    }
  }, [results, selectedIndex, handleSelect, onClose]);

  if (!visible) return null;

  return (
    <div className="search-overlay" onClick={onClose}>
      <div className="search-modal" onClick={(e) => e.stopPropagation()}>
        <input
          ref={inputRef}
          className="search-input"
          placeholder="Search files by name..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
        />
        <div className="search-results">
          {results.map((r, i) => (
            <div
              key={r.path}
              className={`search-result ${i === selectedIndex ? 'selected' : ''}`}
              onClick={() => handleSelect(r)}
              onMouseEnter={() => setSelectedIndex(i)}
            >
              <span className="search-result-name">{r.name}</span>
              <span className="search-result-path">{r.path}</span>
            </div>
          ))}
          {query && results.length === 0 && (
            <div className="search-result" style={{ color: 'var(--text-dim)', cursor: 'default' }}>
              No files found
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
