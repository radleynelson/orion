import { useState, useEffect, useRef, useCallback } from 'react';
import { useStore } from '../store';
import { SearchContents } from '../../wailsjs/go/main/App';
import { getLanguageFromPath } from '../lib/languages';
import { files } from '../../wailsjs/go/models';

export default function GlobalSearch() {
  const { activeWorkspacePath, openFile } = useStore();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<files.GrepResult[]>([]);
  const [searching, setSearching] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setTimeout(() => inputRef.current?.focus(), 50);
  }, []);

  const doSearch = useCallback(async () => {
    if (!activeWorkspacePath || !query.trim()) {
      setResults([]);
      return;
    }
    setSearching(true);
    try {
      const r = await SearchContents(activeWorkspacePath, query.trim());
      setResults(r || []);
    } catch {}
    setSearching(false);
  }, [activeWorkspacePath, query]);

  // Search on Enter or after debounce
  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      return;
    }
    const timer = setTimeout(doSearch, 300);
    return () => clearTimeout(timer);
  }, [query, doSearch]);

  const handleResultClick = useCallback((result: files.GrepResult) => {
    if (!activeWorkspacePath) return;
    const fullPath = activeWorkspacePath + '/' + result.file;
    const language = getLanguageFromPath(result.file);
    openFile(fullPath, language, result.line > 0 ? result.line : undefined);
  }, [activeWorkspacePath, openFile]);

  return (
    <div className="sidebar">
      <div className="sidebar-section">
        <div className="sidebar-label">Search</div>
        <input
          ref={inputRef}
          className="sidebar-input"
          placeholder="Search in files..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') doSearch();
          }}
        />
      </div>
      <div className="search-content-results">
        {searching && (
          <div className="sidebar-item" style={{ color: 'var(--text-dim)' }}>
            Searching...
          </div>
        )}
        {!searching && query && results.length === 0 && (
          <div className="sidebar-item" style={{ color: 'var(--text-dim)' }}>
            No results
          </div>
        )}
        {results.map((r, i) => (
          <div
            key={`${r.file}-${r.line}-${i}`}
            className="search-content-result"
            onClick={() => handleResultClick(r)}
          >
            <div className="search-content-file">
              <span className="search-content-filename">{r.file.split('/').pop()}</span>
              <span className="search-content-filepath">{r.file}</span>
            </div>
            {r.line > 0 && (
              <div className="search-content-line">
                <span className="search-content-linenum">{r.line}</span>
                <span className="search-content-text">{r.content}</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
