import { useEffect, useRef, useState } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import { GetFileDiff } from '../../wailsjs/go/main/App';
import { useStore, zoomFactorFor, BASE_FONT_SIZE } from '../store';
import type { editor } from 'monaco-editor';

interface DiffViewerProps {
  filePath: string;
  visible: boolean;
}

export default function DiffViewer({ filePath, visible }: DiffViewerProps) {
  const { project } = useStore();
  const [original, setOriginal] = useState<string>('');
  const [modified, setModified] = useState<string>('');
  const [language, setLanguage] = useState<string>('plaintext');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const editorRef = useRef<editor.IStandaloneDiffEditor | null>(null);
  const zoomLevel = useStore((s) => s.zoomLevel);
  const fontSize = Math.round(BASE_FONT_SIZE * zoomFactorFor(zoomLevel));

  useEffect(() => {
    const ed = editorRef.current;
    if (!ed) return;
    ed.getOriginalEditor().updateOptions({ fontSize });
    ed.getModifiedEditor().updateOptions({ fontSize });
  }, [fontSize]);

  useEffect(() => {
    if (!project) return;
    (async () => {
      try {
        setLoading(true);
        const diff = await GetFileDiff(project.root, filePath);
        setOriginal(diff.originalContent);
        setModified(diff.modifiedContent);
        setLanguage(diff.language);
        setError(null);
      } catch (err: any) {
        setError(err?.message || 'Failed to load diff');
      } finally {
        setLoading(false);
      }
    })();
  }, [filePath, project]);

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

  if (loading) {
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
        Loading diff...
      </div>
    );
  }

  return (
    <DiffEditor
      original={original}
      modified={modified}
      language={language}
      theme="orion-dark"
      onMount={(ed) => { editorRef.current = ed; }}
      options={{
        readOnly: true,
        renderSideBySide: true,
        minimap: { enabled: false },
        fontSize,
        fontFamily: "'JetBrains Mono', 'Menlo', 'Monaco', monospace",
        scrollBeyondLastLine: false,
        automaticLayout: true,
        originalEditable: false,
        scrollbar: {
          verticalScrollbarSize: 6,
          horizontalScrollbarSize: 6,
        },
      }}
    />
  );
}
