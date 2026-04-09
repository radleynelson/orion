import { useState, useEffect, useRef, useCallback } from 'react';
import { useStore } from '../store';
import { SearchContents } from '../../wailsjs/go/main/App';
import { getLanguageFromPath } from '../lib/languages';
import { files } from '../../wailsjs/go/models';

export default function GlobalSearch() {
  const { activeWorkspacePath, openFile, sidebarMode, globalSearchQuery, setGlobalSearchQuery, setSearchInFileQuery } = useStore();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<files.GrepResult[]>([]);
  const [searching, setSearching] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Pick up pre-filled query from store (e.g., from selected text + Cmd+Shift+F)
  useEffect(() => {
    if (sidebarMode === 'search' && globalSearchQuery) {
      setQuery(globalSearchQuery);
      setGlobalSearchQuery(''); // clear so it doesn't re-apply
    }
  }, [sidebarMode, globalSearchQuery, setGlobalSearchQuery]);

  // Focus and select input whenever this panel becomes visible
  useEffect(() => {
    if (sidebarMode === 'search') {
      setTimeout(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      }, 50);
    }
  }, [sidebarMode]);

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
    // Trigger find-in-file highlighting with the search query
    if (query.trim()) {
      setSearchInFileQuery(query.trim());
    }
  }, [activeWorkspacePath, openFile, query, setSearchInFileQuery]);

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
