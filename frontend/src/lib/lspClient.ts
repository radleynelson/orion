/**
 * LSP Client — bridges Monaco Editor to Go-proxied language servers.
 *
 * Architecture:
 * Monaco Editor ↔ lspClient (this file) ↔ Wails Events ↔ Go LSP Manager ↔ LSP Server
 *
 * This module handles:
 * - Starting/stopping LSP servers via Go backend
 * - Sending textDocument/* notifications (didOpen, didChange, didSave, didClose)
 * - Registering Monaco providers (completion, hover, definition, diagnostics)
 * - Receiving LSP notifications (diagnostics, etc.) and applying them to Monaco
 */
import * as monaco from 'monaco-editor';
import { StartLSP, SendLSPMessage, SendLSPRequest, IsLSPRunning } from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

// Map file extensions to LSP language IDs
const EXTENSION_TO_LSP_LANGUAGE: Record<string, string> = {
  '.ts': 'typescript',
  '.tsx': 'typescriptreact',
  '.js': 'javascript',
  '.jsx': 'javascriptreact',
  '.go': 'go',
  '.rb': 'ruby',
  '.css': 'css',
  '.scss': 'scss',
  '.less': 'less',
  '.html': 'html',
  '.htm': 'html',
  '.json': 'json',
};

// Map LSP language to the server key (multiple languages share one server)
const LSP_LANGUAGE_TO_SERVER: Record<string, string> = {
  'typescript': 'typescript',
  'typescriptreact': 'typescript',
  'javascript': 'typescript',
  'javascriptreact': 'typescript',
  'go': 'go',
  'ruby': 'ruby',
  'css': 'css',
  'scss': 'css',
  'less': 'css',
  'html': 'html',
  'json': 'json',
};

const LSP_LANGUAGE_SELECTORS: monaco.languages.LanguageSelector = Array.from(
  new Set(Object.values(EXTENSION_TO_LSP_LANGUAGE)),
).map((language) => ({ language, exclusive: true }));

interface LSPServerState {
  language: string;
  initialized: boolean;
  openDocuments: Set<string>;
  documentVersions: Map<string, number>;
  eventCancel?: () => void;
  semanticTokensLegend?: { tokenTypes: string[]; tokenModifiers: string[] };
}

const servers = new Map<string, LSPServerState>();
const disposables: monaco.IDisposable[] = [];
let providersRegistered = false;
let monacoApi: typeof monaco = monaco;

export function setMonacoInstance(instance: any): void {
  if (monacoApi === instance) return;
  monacoApi = instance as typeof monaco;
  if (!providersRegistered) return;

  for (const d of disposables) {
    d.dispose();
  }
  disposables.length = 0;
  providersRegistered = false;
  registerProviders();
  providersRegistered = true;
}

/**
 * Get the LSP language for a file path.
 */
export function getLSPLanguage(filePath: string): string | null {
  const ext = '.' + filePath.split('.').pop()?.toLowerCase();
  return EXTENSION_TO_LSP_LANGUAGE[ext] || null;
}

/**
 * Get the LSP server key for a language.
 */
function getServerKey(language: string): string {
  return LSP_LANGUAGE_TO_SERVER[language] || language;
}

/**
 * Ensure an LSP server is running for the given language.
 */
export async function ensureServer(repoRoot: string, language: string, workspacePath: string): Promise<boolean> {
  const serverKey = getServerKey(language);

  if (servers.has(serverKey)) {
    return servers.get(serverKey)!.initialized;
  }

  try {
    const running = await IsLSPRunning(serverKey);
    if (!running) {
      await StartLSP(repoRoot, serverKey, workspacePath);
    }
  } catch (err) {
    console.warn(`Failed to start LSP for ${serverKey}:`, err);
    return false;
  }

  const state: LSPServerState = {
    language: serverKey,
    initialized: false,
    openDocuments: new Set(),
    documentVersions: new Map(),
  };

  servers.set(serverKey, state);

  // Listen for messages from this LSP server
  const eventName = `lsp:message:${serverKey}`;
  const cancel = EventsOn(eventName, (msg: string) => {
    handleServerMessage(serverKey, msg);
  });
  state.eventCancel = cancel;

  // Send initialize request
  try {
    const initResult = await SendLSPRequest(serverKey, 'initialize', JSON.stringify({
      processId: null,
      rootUri: `file://${workspacePath}`,
      capabilities: {
        textDocument: {
          completion: {
            completionItem: {
              snippetSupport: true,
              commitCharactersSupport: true,
              documentationFormat: ['markdown', 'plaintext'],
              resolveSupport: { properties: ['documentation', 'detail', 'additionalTextEdits'] },
            },
            contextSupport: true,
          },
          hover: {
            contentFormat: ['markdown', 'plaintext'],
          },
          definition: {},
          references: {},
          documentSymbol: {
            hierarchicalDocumentSymbolSupport: true,
          },
          publishDiagnostics: {
            relatedInformation: true,
          },
          signatureHelp: {
            signatureInformation: {
              documentationFormat: ['markdown', 'plaintext'],
              parameterInformation: { labelOffsetSupport: true },
            },
          },
          semanticTokens: {
            dynamicRegistration: false,
            tokenTypes: [
              'namespace', 'type', 'class', 'enum', 'interface', 'struct',
              'typeParameter', 'parameter', 'variable', 'property', 'enumMember',
              'event', 'function', 'method', 'macro', 'keyword', 'modifier',
              'comment', 'string', 'number', 'regexp', 'operator', 'decorator',
            ],
            tokenModifiers: [
              'declaration', 'definition', 'readonly', 'static', 'deprecated',
              'abstract', 'async', 'modification', 'documentation', 'defaultLibrary',
            ],
            formats: ['relative'],
            requests: { full: true, range: false },
            multilineTokenSupport: false,
            overlappingTokenSupport: false,
          },
        },
        workspace: {
          workspaceFolders: true,
        },
      },
      workspaceFolders: [{ uri: `file://${workspacePath}`, name: workspacePath.split('/').pop() || '' }],
    }));

    if (initResult) {
      // Parse server capabilities for semantic tokens
      try {
        const parsed = JSON.parse(initResult);
        const caps = parsed?.result?.capabilities;
        if (caps?.semanticTokensProvider?.legend) {
          state.semanticTokensLegend = caps.semanticTokensProvider.legend;
        }
      } catch {}

      // Send initialized notification
      await SendLSPMessage(serverKey, JSON.stringify({
        jsonrpc: '2.0',
        method: 'initialized',
        params: {},
      }));
      state.initialized = true;
    }
  } catch (err) {
    console.warn(`Failed to initialize LSP for ${serverKey}:`, err);
    return false;
  }

  // Register Monaco providers once
  if (!providersRegistered) {
    registerProviders();
    providersRegistered = true;
  }

  return true;
}

/**
 * Notify the LSP server that a document was opened.
 */
export async function didOpen(filePath: string, language: string, content: string): Promise<void> {
  const serverKey = getServerKey(language);
  const state = servers.get(serverKey);
  if (!state?.initialized) return;

  const uri = `file://${filePath}`;
  if (state.openDocuments.has(uri)) return;

  state.openDocuments.add(uri);
  state.documentVersions.set(uri, 1);

  await SendLSPMessage(serverKey, JSON.stringify({
    jsonrpc: '2.0',
    method: 'textDocument/didOpen',
    params: {
      textDocument: {
        uri,
        languageId: language,
        version: 1,
        text: content,
      },
    },
  }));
}

/**
 * Notify the LSP server that a document changed.
 */
export async function didChange(filePath: string, language: string, content: string): Promise<void> {
  const serverKey = getServerKey(language);
  const state = servers.get(serverKey);
  if (!state?.initialized) return;

  const uri = `file://${filePath}`;
  const version = (state.documentVersions.get(uri) || 0) + 1;
  state.documentVersions.set(uri, version);

  await SendLSPMessage(serverKey, JSON.stringify({
    jsonrpc: '2.0',
    method: 'textDocument/didChange',
    params: {
      textDocument: { uri, version },
      contentChanges: [{ text: content }],
    },
  }));
}

/**
 * Notify the LSP server that a document was saved.
 */
export async function didSave(filePath: string, language: string, content: string): Promise<void> {
  const serverKey = getServerKey(language);
  const state = servers.get(serverKey);
  if (!state?.initialized) return;

  await SendLSPMessage(serverKey, JSON.stringify({
    jsonrpc: '2.0',
    method: 'textDocument/didSave',
    params: {
      textDocument: { uri: `file://${filePath}` },
      text: content,
    },
  }));
}

/**
 * Notify the LSP server that a document was closed.
 */
export async function didClose(filePath: string, language: string): Promise<void> {
  const serverKey = getServerKey(language);
  const state = servers.get(serverKey);
  if (!state?.initialized) return;

  const uri = `file://${filePath}`;
  state.openDocuments.delete(uri);
  state.documentVersions.delete(uri);

  await SendLSPMessage(serverKey, JSON.stringify({
    jsonrpc: '2.0',
    method: 'textDocument/didClose',
    params: {
      textDocument: { uri },
    },
  }));
}

/**
 * Handle a message from the LSP server (notifications, diagnostics, etc.).
 */
function handleServerMessage(serverKey: string, rawMsg: string): void {
  try {
    const msg = JSON.parse(rawMsg);

    if (msg.method === 'textDocument/publishDiagnostics') {
      applyDiagnostics(msg.params);
    }
  } catch (err) {
    console.warn('Failed to parse LSP message:', err);
  }
}

/**
 * Apply diagnostics from LSP to Monaco markers.
 */
function applyDiagnostics(params: { uri: string; diagnostics: any[] }): void {
  const filePath = params.uri.replace('file://', '');
  const models = monacoApi.editor.getModels();
  const model = models.find((m) => m.uri.path === filePath);
  if (!model) return;

  const markers: monaco.editor.IMarkerData[] = params.diagnostics.map((d) => ({
    severity: lspSeverityToMonaco(d.severity),
    startLineNumber: (d.range?.start?.line ?? 0) + 1,
    startColumn: (d.range?.start?.character ?? 0) + 1,
    endLineNumber: (d.range?.end?.line ?? 0) + 1,
    endColumn: (d.range?.end?.character ?? 0) + 1,
    message: d.message || '',
    source: d.source || '',
    code: d.code?.toString(),
  }));

  monacoApi.editor.setModelMarkers(model, 'lsp', markers);
}

function lspSeverityToMonaco(severity: number): monaco.MarkerSeverity {
  switch (severity) {
    case 1: return monacoApi.MarkerSeverity.Error;
    case 2: return monacoApi.MarkerSeverity.Warning;
    case 3: return monacoApi.MarkerSeverity.Info;
    case 4: return monacoApi.MarkerSeverity.Hint;
    default: return monacoApi.MarkerSeverity.Info;
  }
}

/**
 * Convert an LSP CompletionItemKind to Monaco CompletionItemKind.
 */
function lspCompletionKindToMonaco(kind: number): monaco.languages.CompletionItemKind {
  // LSP and Monaco kinds are similar but not identical
  const map: Record<number, monaco.languages.CompletionItemKind> = {
    1: monacoApi.languages.CompletionItemKind.Text,
    2: monacoApi.languages.CompletionItemKind.Method,
    3: monacoApi.languages.CompletionItemKind.Function,
    4: monacoApi.languages.CompletionItemKind.Constructor,
    5: monacoApi.languages.CompletionItemKind.Field,
    6: monacoApi.languages.CompletionItemKind.Variable,
    7: monacoApi.languages.CompletionItemKind.Class,
    8: monacoApi.languages.CompletionItemKind.Interface,
    9: monacoApi.languages.CompletionItemKind.Module,
    10: monacoApi.languages.CompletionItemKind.Property,
    11: monacoApi.languages.CompletionItemKind.Unit,
    12: monacoApi.languages.CompletionItemKind.Value,
    13: monacoApi.languages.CompletionItemKind.Enum,
    14: monacoApi.languages.CompletionItemKind.Keyword,
    15: monacoApi.languages.CompletionItemKind.Snippet,
    16: monacoApi.languages.CompletionItemKind.Color,
    17: monacoApi.languages.CompletionItemKind.File,
    18: monacoApi.languages.CompletionItemKind.Reference,
    19: monacoApi.languages.CompletionItemKind.Folder,
    20: monacoApi.languages.CompletionItemKind.EnumMember,
    21: monacoApi.languages.CompletionItemKind.Constant,
    22: monacoApi.languages.CompletionItemKind.Struct,
    23: monacoApi.languages.CompletionItemKind.Event,
    24: monacoApi.languages.CompletionItemKind.Operator,
    25: monacoApi.languages.CompletionItemKind.TypeParameter,
  };
  return map[kind] || monacoApi.languages.CompletionItemKind.Text;
}

/**
 * Register Monaco language providers (completion, hover, definition, references).
 * These providers are language-agnostic — they check if an LSP server is running
 * for the file's language and delegate to it.
 */
function registerProviders(): void {
  // Completion provider
  disposables.push(
    monacoApi.languages.registerCompletionItemProvider(LSP_LANGUAGE_SELECTORS, {
      triggerCharacters: ['.', ':', '<', '"', "'", '/', '@', '#'],
      provideCompletionItems: async (model, position) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return { suggestions: [] };

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized) return { suggestions: [] };

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/completion', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
            position: { line: position.lineNumber - 1, character: position.column - 1 },
          }));

          const parsed = JSON.parse(result);
          const response = parsed.result || parsed;
          const items = Array.isArray(response) ? response : response?.items || [];

          const word = model.getWordUntilPosition(position);
          const range = {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: word.startColumn,
            endColumn: word.endColumn,
          };

          return {
            suggestions: items.map((item: any) => ({
              label: item.label,
              kind: lspCompletionKindToMonaco(item.kind || 1),
              insertText: item.insertText || item.label,
              insertTextRules: item.insertTextFormat === 2
                ? monacoApi.languages.CompletionItemInsertTextRule.InsertAsSnippet
                : undefined,
              detail: item.detail,
              documentation: item.documentation
                ? (typeof item.documentation === 'string'
                  ? item.documentation
                  : { value: item.documentation.value || '' })
                : undefined,
              range,
              sortText: item.sortText,
              filterText: item.filterText,
            })),
          };
        } catch {
          return { suggestions: [] };
        }
      },
    })
  );

  // Hover provider
  disposables.push(
    monacoApi.languages.registerHoverProvider(LSP_LANGUAGE_SELECTORS, {
      provideHover: async (model, position) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return null;

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized) return null;

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/hover', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
            position: { line: position.lineNumber - 1, character: position.column - 1 },
          }));

          const parsed = JSON.parse(result);
          const hover = parsed.result || parsed;
          if (!hover?.contents) return null;

          const contents: monaco.IMarkdownString[] = [];
          if (typeof hover.contents === 'string') {
            contents.push({ value: hover.contents });
          } else if (hover.contents.kind === 'markdown') {
            contents.push({ value: hover.contents.value });
          } else if (hover.contents.value) {
            contents.push({ value: `\`\`\`${hover.contents.language || ''}\n${hover.contents.value}\n\`\`\`` });
          } else if (Array.isArray(hover.contents)) {
            for (const c of hover.contents) {
              if (typeof c === 'string') contents.push({ value: c });
              else if (c.value) contents.push({ value: `\`\`\`${c.language || ''}\n${c.value}\n\`\`\`` });
            }
          }

          return {
            contents,
            range: hover.range ? {
              startLineNumber: hover.range.start.line + 1,
              startColumn: hover.range.start.character + 1,
              endLineNumber: hover.range.end.line + 1,
              endColumn: hover.range.end.character + 1,
            } : undefined,
          };
        } catch {
          return null;
        }
      },
    })
  );

  // Definition provider (go-to-definition)
  disposables.push(
    monacoApi.languages.registerDefinitionProvider(LSP_LANGUAGE_SELECTORS, {
      provideDefinition: async (model, position) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return null;

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized) return null;

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/definition', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
            position: { line: position.lineNumber - 1, character: position.column - 1 },
          }));

          const parsed = JSON.parse(result);
          const def = parsed.result || parsed;
          if (!def) return null;

          const locations = Array.isArray(def) ? def : [def];
          return locations.map((loc: any) => ({
            uri: monacoApi.Uri.parse(loc.uri || loc.targetUri),
            range: {
              startLineNumber: (loc.range?.start?.line ?? loc.targetRange?.start?.line ?? 0) + 1,
              startColumn: (loc.range?.start?.character ?? loc.targetRange?.start?.character ?? 0) + 1,
              endLineNumber: (loc.range?.end?.line ?? loc.targetRange?.end?.line ?? 0) + 1,
              endColumn: (loc.range?.end?.character ?? loc.targetRange?.end?.character ?? 0) + 1,
            },
          }));
        } catch {
          return null;
        }
      },
    })
  );

  // References provider
  disposables.push(
    monacoApi.languages.registerReferenceProvider(LSP_LANGUAGE_SELECTORS, {
      provideReferences: async (model, position, context) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return null;

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized) return null;

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/references', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
            position: { line: position.lineNumber - 1, character: position.column - 1 },
            context: { includeDeclaration: context.includeDeclaration },
          }));

          const parsed = JSON.parse(result);
          const refs = parsed.result || parsed;
          if (!Array.isArray(refs)) return null;

          return refs.map((ref: any) => ({
            uri: monacoApi.Uri.parse(ref.uri),
            range: {
              startLineNumber: (ref.range?.start?.line ?? 0) + 1,
              startColumn: (ref.range?.start?.character ?? 0) + 1,
              endLineNumber: (ref.range?.end?.line ?? 0) + 1,
              endColumn: (ref.range?.end?.character ?? 0) + 1,
            },
          }));
        } catch {
          return null;
        }
      },
    })
  );

  // Signature help provider
  disposables.push(
    monacoApi.languages.registerSignatureHelpProvider(LSP_LANGUAGE_SELECTORS, {
      signatureHelpTriggerCharacters: ['(', ','],
      provideSignatureHelp: async (model, position) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return null;

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized) return null;

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/signatureHelp', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
            position: { line: position.lineNumber - 1, character: position.column - 1 },
          }));

          const parsed = JSON.parse(result);
          const sigHelp = parsed.result || parsed;
          if (!sigHelp?.signatures?.length) return null;

          return {
            value: {
              signatures: sigHelp.signatures.map((sig: any) => ({
                label: sig.label,
                documentation: sig.documentation
                  ? (typeof sig.documentation === 'string'
                    ? sig.documentation
                    : { value: sig.documentation.value || '' })
                  : undefined,
                parameters: (sig.parameters || []).map((p: any) => ({
                  label: p.label,
                  documentation: p.documentation
                    ? (typeof p.documentation === 'string'
                      ? p.documentation
                      : { value: p.documentation.value || '' })
                    : undefined,
                })),
              })),
              activeSignature: sigHelp.activeSignature || 0,
              activeParameter: sigHelp.activeParameter || 0,
            },
            dispose: () => {},
          };
        } catch {
          return null;
        }
      },
    })
  );

  // Document symbol provider (outline)
  disposables.push(
    monacoApi.languages.registerDocumentSymbolProvider(LSP_LANGUAGE_SELECTORS, {
      provideDocumentSymbols: async (model) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return null;

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized) return null;

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/documentSymbol', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
          }));

          const parsed = JSON.parse(result);
          const symbols = parsed.result || parsed;
          if (!Array.isArray(symbols)) return null;

          return symbols.map(convertSymbol);
        } catch {
          return null;
        }
      },
    })
  );

  // Semantic tokens provider (LSP-powered syntax highlighting)
  disposables.push(
    monacoApi.languages.registerDocumentSemanticTokensProvider(LSP_LANGUAGE_SELECTORS, {
      getLegend: () => {
        // Return a merged legend from all servers, or a default one
        const tokenTypes = [
          'namespace', 'type', 'class', 'enum', 'interface', 'struct',
          'typeParameter', 'parameter', 'variable', 'property', 'enumMember',
          'event', 'function', 'method', 'macro', 'keyword', 'modifier',
          'comment', 'string', 'number', 'regexp', 'operator', 'decorator',
        ];
        const tokenModifiers = [
          'declaration', 'definition', 'readonly', 'static', 'deprecated',
          'abstract', 'async', 'modification', 'documentation', 'defaultLibrary',
        ];
        return { tokenTypes, tokenModifiers };
      },
      provideDocumentSemanticTokens: async (model) => {
        const language = getLSPLanguageForModel(model);
        if (!language) return null;

        const serverKey = getServerKey(language);
        const state = servers.get(serverKey);
        if (!state?.initialized || !state.semanticTokensLegend) return null;

        try {
          const result = await SendLSPRequest(serverKey, 'textDocument/semanticTokens/full', JSON.stringify({
            textDocument: { uri: model.uri.toString() },
          }));

          const parsed = JSON.parse(result);
          const tokens = parsed?.result;
          if (!tokens?.data?.length) return null;

          // The server's legend may differ from ours — remap token types
          const serverLegend = state.semanticTokensLegend;
          const ourTypes = [
            'namespace', 'type', 'class', 'enum', 'interface', 'struct',
            'typeParameter', 'parameter', 'variable', 'property', 'enumMember',
            'event', 'function', 'method', 'macro', 'keyword', 'modifier',
            'comment', 'string', 'number', 'regexp', 'operator', 'decorator',
          ];

          const typeMap = new Map<number, number>();
          for (let i = 0; i < serverLegend.tokenTypes.length; i++) {
            const ourIdx = ourTypes.indexOf(serverLegend.tokenTypes[i]);
            if (ourIdx >= 0) typeMap.set(i, ourIdx);
          }

          // Remap the encoded data
          const data = new Uint32Array(tokens.data.length);
          for (let i = 0; i < tokens.data.length; i += 5) {
            data[i] = tokens.data[i];     // deltaLine
            data[i + 1] = tokens.data[i + 1]; // deltaStartChar
            data[i + 2] = tokens.data[i + 2]; // length
            data[i + 3] = typeMap.get(tokens.data[i + 3]) ?? tokens.data[i + 3]; // tokenType
            data[i + 4] = tokens.data[i + 4]; // tokenModifiers
          }

          return { data };
        } catch {
          return null;
        }
      },
      releaseDocumentSemanticTokens: () => {},
    })
  );
}

function convertSymbol(sym: any): any {
  const range = sym.range || sym.location?.range;
  const selRange = sym.selectionRange || range;
  return {
    name: sym.name,
    detail: sym.detail || '',
    kind: sym.kind || 0,
    range: range ? {
      startLineNumber: range.start.line + 1,
      startColumn: range.start.character + 1,
      endLineNumber: range.end.line + 1,
      endColumn: range.end.character + 1,
    } : { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 1 },
    selectionRange: selRange ? {
      startLineNumber: selRange.start.line + 1,
      startColumn: selRange.start.character + 1,
      endLineNumber: selRange.end.line + 1,
      endColumn: selRange.end.character + 1,
    } : { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 1 },
    children: sym.children?.map(convertSymbol) || [],
  };
}

function getLSPLanguageForModel(model: monaco.editor.ITextModel): string | null {
  const path = model.uri.path;
  return getLSPLanguage(path);
}

/**
 * Clean up all LSP resources.
 */
export function dispose(): void {
  for (const d of disposables) {
    d.dispose();
  }
  disposables.length = 0;
  providersRegistered = false;

  for (const [, state] of servers) {
    if (state.eventCancel) state.eventCancel();
  }
  servers.clear();
}
