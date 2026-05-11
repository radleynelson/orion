import { useEffect, useState, useRef } from 'react';
import Editor from '@monaco-editor/react';
import { ReadFileContents, WriteFileContents } from '../../wailsjs/go/main/App';
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
  const [savedContent, setSavedContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const zoomLevel = useStore((s) => s.zoomLevel);
  const searchInFileQuery = useStore((s) => s.searchInFileQuery);
  const setSearchInFileQuery = useStore((s) => s.setSearchInFileQuery);
  const markDirty = useStore((s) => s.markDirty);
  const markClean = useStore((s) => s.markClean);
  const fontSize = Math.round(BASE_FONT_SIZE * zoomFactorFor(zoomLevel));

  useEffect(() => {
    editorRef.current?.updateOptions({ fontSize });
  }, [fontSize]);

  useEffect(() => {
    (async () => {
      try {
        const data = await ReadFileContents(filePath);
        setContent(data);
        setSavedContent(data);
        setError(null);
        markClean(filePath);
      } catch (err: any) {
        setError(err?.message || 'Failed to read file');
        setContent(null);
        setSavedContent(null);
      }
    })();
  }, [filePath, markClean]);

  // Scroll to line when it changes
  useEffect(() => {
    if (line && editorRef.current) {
      editorRef.current.revealLineInCenter(line);
      editorRef.current.setPosition({ lineNumber: line, column: 1 });
    }
  }, [line]);

  const saveRef = useRef<() => Promise<void>>();
  saveRef.current = async () => {
    if (!editorRef.current || saving) return;
    const currentContent = editorRef.current.getValue();
    setSaving(true);
    try {
      await WriteFileContents(filePath, currentContent);
      setSavedContent(currentContent);
      markClean(filePath);
    } catch (err: any) {
      console.error('Failed to save file:', err);
    } finally {
      setSaving(false);
    }
  };

  const handleEditorMount = (editor: editor.IStandaloneCodeEditor) => {
    editorRef.current = editor;
    if (line) {
      editor.revealLineInCenter(line);
      editor.setPosition({ lineNumber: line, column: 1 });
    }

    // Add Cmd+S keybinding for save (uses ref to avoid stale closure)
    editor.addCommand(
      // Monaco.KeyMod.CtrlCmd | Monaco.KeyCode.KeyS
      2048 | 49, // CtrlCmd = 2048, KeyS = 49
      () => { void saveRef.current?.(); }
    );

    // Focus immediately so Cmd+F works right away
    editor.focus();
  };

  const handleEditorChange = (value: string | undefined) => {
    if (value === undefined) return;
    setContent(value);
    if (value !== savedContent) {
      markDirty(filePath);
    } else {
      markClean(filePath);
    }
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
      onChange={handleEditorChange}
      options={{
        readOnly: false,
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
        tabSize: 2,
        insertSpaces: true,
        bracketPairColorization: { enabled: true },
        guides: { bracketPairs: true },
        suggestOnTriggerCharacters: true,
        quickSuggestions: true,
        parameterHints: { enabled: true },
      }}
    />
  );
}
