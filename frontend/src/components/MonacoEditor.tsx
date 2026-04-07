import { useEffect, useState, useRef } from 'react';
import Editor, { type Monaco } from '@monaco-editor/react';
import { ReadFileContents } from '../../wailsjs/go/main/App';
import type { editor } from 'monaco-editor';

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
  };

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
        fontSize: 13,
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
