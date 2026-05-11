import { useEffect, useState, useRef } from 'react';
import Editor from '@monaco-editor/react';
import { ReadFileContents, WriteFileContents, FormatFile, RunOnSave } from '../../wailsjs/go/main/App';
import type { editor } from 'monaco-editor';
import * as monaco from 'monaco-editor';
import { useStore, zoomFactorFor, BASE_FONT_SIZE } from '../store';
import { ensureServer, didOpen, didChange, didSave, didClose, getLSPLanguage } from '../lib/lspClient';

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
  const lspInitialized = useRef(false);
  const changeTimer = useRef<ReturnType<typeof setTimeout>>();
  const zoomLevel = useStore((s) => s.zoomLevel);
  const searchInFileQuery = useStore((s) => s.searchInFileQuery);
  const setSearchInFileQuery = useStore((s) => s.setSearchInFileQuery);
  const markDirty = useStore((s) => s.markDirty);
  const markClean = useStore((s) => s.markClean);
  const project = useStore((s) => s.project);
  const activeWorkspacePath = useStore((s) => s.activeWorkspacePath);
  const openFile = useStore((s) => s.openFile);
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

  // Initialize LSP when file is loaded
  useEffect(() => {
    if (content === null || !project || !activeWorkspacePath) return;
    if (lspInitialized.current) return;

    const lspLanguage = getLSPLanguage(filePath);
    if (!lspLanguage) return;

    (async () => {
      const ready = await ensureServer(project.root, lspLanguage, activeWorkspacePath);
      if (ready) {
        lspInitialized.current = true;
        // Create a proper Monaco URI for this file so LSP can find the model
        const uri = monaco.Uri.file(filePath);
        const existingModel = monaco.editor.getModel(uri);
        if (!existingModel) {
          // The model will be created by Monaco Editor component with its own URI,
          // so we just notify LSP about the document
        }
        await didOpen(filePath, lspLanguage, content);
      }
    })();
  }, [content, filePath, project, activeWorkspacePath]);

  // Clean up LSP document on unmount
  useEffect(() => {
    return () => {
      const lspLanguage = getLSPLanguage(filePath);
      if (lspLanguage && lspInitialized.current) {
        void didClose(filePath, lspLanguage);
      }
    };
  }, [filePath]);

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
    let currentContent = editorRef.current.getValue();
    setSaving(true);
    try {
      // Format on save if a formatter is available
      if (project) {
        try {
          const result = await FormatFile(project.root, filePath, currentContent);
          if (result?.formatted && result.content) {
            currentContent = result.content;
            // Update the editor content with formatted version
            const ed = editorRef.current;
            const pos = ed.getPosition();
            ed.setValue(currentContent);
            if (pos) ed.setPosition(pos);
          }
        } catch {
          // Formatting failed — save without formatting
        }
      }

      await WriteFileContents(filePath, currentContent);
      setSavedContent(currentContent);
      setContent(currentContent);
      markClean(filePath);

      // Run on-save hooks
      if (project) {
        RunOnSave(project.root, filePath).catch(() => {});
      }

      // Notify LSP about save
      const lspLanguage = getLSPLanguage(filePath);
      if (lspLanguage && lspInitialized.current) {
        await didSave(filePath, lspLanguage, currentContent);
      }
    } catch (err: any) {
      console.error('Failed to save file:', err);
    } finally {
      setSaving(false);
    }
  };

  const handleEditorMount = (ed: editor.IStandaloneCodeEditor) => {
    editorRef.current = ed;
    if (line) {
      ed.revealLineInCenter(line);
      ed.setPosition({ lineNumber: line, column: 1 });
    }

    // Add Cmd+S keybinding for save (uses ref to avoid stale closure)
    ed.addCommand(
      2048 | 49, // CtrlCmd | KeyS
      () => { void saveRef.current?.(); }
    );

    // Handle go-to-definition: when Monaco resolves a definition to a file,
    // intercept navigation to open it in a new Orion editor tab
    const editorService = (ed as any)._codeEditorService;
    if (editorService) {
      editorService.openCodeEditor = async (input: any) => {
        const targetPath = input?.resource?.path;
        if (targetPath) {
          const ext = '.' + targetPath.split('.').pop()?.toLowerCase();
          const langMap: Record<string, string> = {
            '.ts': 'typescript', '.tsx': 'typescriptreact', '.js': 'javascript', '.jsx': 'javascriptreact',
            '.go': 'go', '.rb': 'ruby', '.css': 'css', '.html': 'html', '.json': 'json',
            '.py': 'python', '.rs': 'rust', '.md': 'markdown', '.yaml': 'yaml', '.yml': 'yaml',
            '.toml': 'toml', '.sh': 'shell', '.scss': 'scss',
          };
          const lang = langMap[ext] || 'plaintext';
          const targetLine = input?.options?.selection?.startLineNumber;
          openFile(targetPath, lang, targetLine);
        }
        return null;
      };
    }

    // Focus immediately so Cmd+F works right away
    ed.focus();
  };

  const handleEditorChange = (value: string | undefined) => {
    if (value === undefined) return;
    setContent(value);
    if (value !== savedContent) {
      markDirty(filePath);
    } else {
      markClean(filePath);
    }

    // Debounce LSP didChange notifications (300ms)
    if (changeTimer.current) clearTimeout(changeTimer.current);
    changeTimer.current = setTimeout(() => {
      const lspLanguage = getLSPLanguage(filePath);
      if (lspLanguage && lspInitialized.current) {
        void didChange(filePath, lspLanguage, value);
      }
    }, 300);
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
        ed.getAction('actions.find')?.run();
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
        glyphMargin: true,
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
