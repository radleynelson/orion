import { useEffect, useState, useRef } from 'react';
import Editor, { type Monaco } from '@monaco-editor/react';
import { ReadFileContents } from '../../wailsjs/go/main/App';
import type { editor } from 'monaco-editor';
import { useStore, zoomFactorFor, BASE_FONT_SIZE } from '../store';

interface MonacoEditorProps {
  filePath: string;
  language: string;
  visible: boolean;
  line?: number;
}

export default function MonacoEditor({ filePath, language, visible, line }: MonacoEditorProps) {
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const zoomLevel = useStore((s) => s.zoomLevel);
  const searchInFileQuery = useStore((s) => s.searchInFileQuery);
  const setSearchInFileQuery = useStore((s) => s.setSearchInFileQuery);
  const fontSize = Math.round(BASE_FONT_SIZE * zoomFactorFor(zoomLevel));

  useEffect(() => {
    editorRef.current?.updateOptions({ fontSize });
  }, [fontSize]);

  useEffect(() => {
    (async () => {
      try {
        const data = await ReadFileContents(filePath);
        setContent(data);
        setError(null);
      } catch (err: any) {
        setError(err?.message || 'Failed to read file');
        setContent(null);
      }
    })();
  }, [filePath]);

  // Scroll to line when it changes
  useEffect(() => {
    if (line && editorRef.current) {
      editorRef.current.revealLineInCenter(line);
      editorRef.current.setPosition({ lineNumber: line, column: 1 });
    }
  }, [line]);

  const handleEditorMount = (editor: editor.IStandaloneCodeEditor) => {
    editorRef.current = editor;
    if (line) {
      editor.revealLineInCenter(line);
      editor.setPosition({ lineNumber: line, column: 1 });
    }
    // Focus immediately so Cmd+F works right away
    editor.focus();
  };

  // Re-focus editor when tab becomes visible
  useEffect(() => {
    if (visible && editorRef.current) {
      setTimeout(() => editorRef.current?.focus(), 50);
    }
  }, [visible]);

  // Trigger find widget when a global search result opens this file
  useEffect(() => {
    if (visible && searchInFileQuery && editorRef.current) {
      const ed = editorRef.current;
      setTimeout(() => {
        ed.focus();
        // Set the search string and open the find widget
        ed.getAction('actions.find')?.run();
        // After the find widget opens, set its value
        setTimeout(() => {
          const findController = (ed as any).getContribution('editor.contrib.findController');
          if (findController) {
            findController.setSearchString(searchInFileQuery);
            findController.highlightFindOptions();
          }
          setSearchInFileQuery('');
        }, 100);
      }, 100);
    }
  }, [visible, searchInFileQuery, setSearchInFileQuery]);

  if (!visible) return <div style={{ display: 'none' }} />;

  if (error) {
    return (
      <div style={{
        width: '100%',
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--text-dim)',
        fontFamily: 'var(--font-mono)',
        fontSize: 'var(--font-size-sm)',
        background: '#1e1e1e',
      }}>
        {error}
      </div>
    );
  }

  if (content === null) {
    return (
      <div style={{
        width: '100%',
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--text-dim)',
        background: '#1e1e1e',
      }}>
        Loading...
      </div>
    );
  }

  return (
    <Editor
      value={content}
      language={language}
      theme="orion-dark"
      onMount={handleEditorMount}
      options={{
        readOnly: true,
        minimap: { enabled: false },
        fontSize,
        fontFamily: "'JetBrains Mono', 'Menlo', 'Monaco', monospace",
        lineNumbers: 'on',
        scrollBeyondLastLine: false,
        automaticLayout: true,
        wordWrap: 'off',
        renderWhitespace: 'none',
        folding: true,
        glyphMargin: false,
        overviewRulerBorder: false,
        hideCursorInOverviewRuler: true,
        scrollbar: {
          verticalScrollbarSize: 6,
          horizontalScrollbarSize: 6,
        },
      }}
    />
  );
}
