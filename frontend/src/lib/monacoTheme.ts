import { loader } from '@monaco-editor/react';

export function configureMonacoTheme() {
  loader.init().then((monaco) => {
    // Comprehensive Orion dark theme — GitHub Dark Dimmed palette
    // with extensive token rules for all supported languages
    monaco.editor.defineTheme('orion-dark', {
      base: 'vs-dark',
      inherit: true,
      rules: [
        // === General tokens (fallbacks for all languages) ===
        { token: 'keyword', foreground: 'ff7b72' },
        { token: 'keyword.control', foreground: 'ff7b72' },
        { token: 'keyword.operator', foreground: 'ff7b72' },
        { token: 'string', foreground: 'a5d6ff' },
        { token: 'string.escape', foreground: '79c0ff' },
        { token: 'comment', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'number', foreground: '79c0ff' },
        { token: 'number.float', foreground: '79c0ff' },
        { token: 'number.hex', foreground: '79c0ff' },
        { token: 'type', foreground: '7ee787' },
        { token: 'type.identifier', foreground: '7ee787' },
        { token: 'function', foreground: 'd2a8ff' },
        { token: 'function.declaration', foreground: 'd2a8ff', fontStyle: 'bold' },
        { token: 'variable', foreground: 'ffa657' },
        { token: 'variable.predefined', foreground: '79c0ff' },
        { token: 'constant', foreground: '79c0ff', fontStyle: 'bold' },
        { token: 'operator', foreground: 'ff7b72' },
        { token: 'delimiter', foreground: 'd4d4d4' },
        { token: 'delimiter.bracket', foreground: 'd4d4d4' },
        { token: 'delimiter.parenthesis', foreground: 'd4d4d4' },
        { token: 'delimiter.square', foreground: 'd4d4d4' },
        { token: 'delimiter.curly', foreground: 'd4d4d4' },
        { token: 'tag', foreground: '7ee787' },
        { token: 'attribute.name', foreground: '79c0ff' },
        { token: 'attribute.value', foreground: 'a5d6ff' },
        { token: 'annotation', foreground: 'ffa657' },
        { token: 'regexp', foreground: 'a5d6ff' },
        { token: 'meta', foreground: 'ffa657' },
        { token: 'identifier', foreground: 'd4d4d4' },

        // === TypeScript / JavaScript ===
        { token: 'keyword.ts', foreground: 'ff7b72' },
        { token: 'keyword.tsx', foreground: 'ff7b72' },
        { token: 'type.identifier.ts', foreground: '7ee787' },
        { token: 'type.identifier.tsx', foreground: '7ee787' },

        // === Ruby ===
        { token: 'keyword.ruby', foreground: 'ff7b72' },
        { token: 'keyword.def.ruby', foreground: 'ff7b72' },
        { token: 'keyword.class.ruby', foreground: 'ff7b72' },
        { token: 'keyword.module.ruby', foreground: 'ff7b72' },
        { token: 'type.ruby', foreground: '7ee787' },
        { token: 'class.name.ruby', foreground: '7ee787', fontStyle: 'bold' },
        { token: 'module.name.ruby', foreground: '7ee787', fontStyle: 'bold' },
        { token: 'method.ruby', foreground: 'd2a8ff' },
        { token: 'method.call.ruby', foreground: 'd2a8ff' },
        { token: 'variable.instance.ruby', foreground: 'ffa657' },
        { token: 'variable.class.ruby', foreground: 'ffa657', fontStyle: 'bold' },
        { token: 'variable.global.ruby', foreground: 'ffa657' },
        { token: 'constant.ruby', foreground: '79c0ff', fontStyle: 'bold' },
        { token: 'symbol.ruby', foreground: '79c0ff' },
        { token: 'string.ruby', foreground: 'a5d6ff' },
        { token: 'string.escape.ruby', foreground: '79c0ff' },
        { token: 'string.interpolation.ruby', foreground: 'd4d4d4' },
        { token: 'string.interpolation', foreground: 'ff7b72' },
        { token: 'comment.ruby', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'number.ruby', foreground: '79c0ff' },
        { token: 'regexp.ruby', foreground: 'a5d6ff' },
        { token: 'operator.ruby', foreground: 'ff7b72' },
        { token: 'delimiter.ruby', foreground: 'd4d4d4' },
        { token: 'identifier.ruby', foreground: 'd4d4d4' },

        // === Go ===
        { token: 'keyword.go', foreground: 'ff7b72' },
        { token: 'type.go', foreground: '7ee787' },
        { token: 'comment.go', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'string.go', foreground: 'a5d6ff' },
        { token: 'number.go', foreground: '79c0ff' },
        { token: 'variable.go', foreground: 'd4d4d4' },
        { token: 'function.go', foreground: 'd2a8ff' },
        { token: 'constant.go', foreground: '79c0ff' },

        // === CSS / SCSS ===
        { token: 'tag.css', foreground: '7ee787' },
        { token: 'attribute.name.css', foreground: '79c0ff' },
        { token: 'attribute.value.css', foreground: 'a5d6ff' },
        { token: 'attribute.value.number.css', foreground: '79c0ff' },
        { token: 'attribute.value.unit.css', foreground: '79c0ff' },
        { token: 'attribute.value.hex.css', foreground: '79c0ff' },
        { token: 'keyword.css', foreground: 'ff7b72' },
        { token: 'string.css', foreground: 'a5d6ff' },
        { token: 'variable.css', foreground: 'ffa657' },

        // === HTML ===
        { token: 'tag.html', foreground: '7ee787' },
        { token: 'attribute.name.html', foreground: '79c0ff' },
        { token: 'attribute.value.html', foreground: 'a5d6ff' },
        { token: 'comment.html', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'string.html', foreground: 'a5d6ff' },

        // === JSON ===
        { token: 'string.key.json', foreground: '79c0ff' },
        { token: 'string.value.json', foreground: 'a5d6ff' },
        { token: 'number.json', foreground: '79c0ff' },
        { token: 'keyword.json', foreground: 'ff7b72' },

        // === YAML ===
        { token: 'type.yaml', foreground: '79c0ff' },
        { token: 'string.yaml', foreground: 'a5d6ff' },
        { token: 'number.yaml', foreground: '79c0ff' },
        { token: 'keyword.yaml', foreground: 'ff7b72' },
        { token: 'comment.yaml', foreground: '8b949e', fontStyle: 'italic' },

        // === Markdown ===
        { token: 'keyword.md', foreground: '79c0ff', fontStyle: 'bold' },
        { token: 'string.md', foreground: 'a5d6ff' },
        { token: 'comment.md', foreground: '8b949e' },
        { token: 'string.link.md', foreground: '79c0ff' },
        { token: 'variable.md', foreground: 'ffa657' },

        // === ERB ===
        { token: 'delimiter.erb', foreground: 'ff7b72', fontStyle: 'bold' },
        { token: 'comment.erb', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'method.call.ruby', foreground: 'd2a8ff' },

        // === Semantic token types (from LSP) ===
        { token: 'namespace', foreground: '7ee787' },
        { token: 'class', foreground: '7ee787' },
        { token: 'enum', foreground: '7ee787' },
        { token: 'interface', foreground: '7ee787', fontStyle: 'italic' },
        { token: 'struct', foreground: '7ee787' },
        { token: 'typeParameter', foreground: '7ee787', fontStyle: 'italic' },
        { token: 'parameter', foreground: 'ffa657' },
        { token: 'property', foreground: '79c0ff' },
        { token: 'enumMember', foreground: '79c0ff' },
        { token: 'event', foreground: '79c0ff' },
        { token: 'method', foreground: 'd2a8ff' },
        { token: 'macro', foreground: 'd2a8ff', fontStyle: 'bold' },
        { token: 'label', foreground: 'ffa657' },
        { token: 'decorator', foreground: 'ffa657' },
      ],
      colors: {
        'editor.background': '#1e1e1e',
        'editor.foreground': '#d4d4d4',
        'editor.selectionBackground': 'rgba(108, 182, 255, 0.3)',
        'editor.inactiveSelectionBackground': 'rgba(108, 182, 255, 0.15)',
        'editor.lineHighlightBackground': '#252525',
        'editor.findMatchBackground': 'rgba(255, 123, 114, 0.3)',
        'editor.findMatchHighlightBackground': 'rgba(255, 123, 114, 0.15)',
        'editor.wordHighlightBackground': 'rgba(108, 182, 255, 0.15)',
        'editor.wordHighlightStrongBackground': 'rgba(108, 182, 255, 0.25)',
        'editorGutter.background': '#1e1e1e',
        'editorLineNumber.foreground': '#5a5a5a',
        'editorLineNumber.activeForeground': '#b0b0b0',
        'editorCursor.foreground': '#d4d4d4',
        'editorBracketMatch.background': 'rgba(108, 182, 255, 0.2)',
        'editorBracketMatch.border': '#79c0ff60',
        'editorIndentGuide.background': '#2d2d2d',
        'editorIndentGuide.activeBackground': '#4a4a4a',
        'editorError.foreground': '#ff7b72',
        'editorWarning.foreground': '#ffa657',
        'editorInfo.foreground': '#79c0ff',
        'editorHint.foreground': '#7ee787',
        'scrollbar.shadow': '#00000000',
        'scrollbarSlider.background': '#3d3d3d80',
        'scrollbarSlider.hoverBackground': '#4a4a4a',
        'scrollbarSlider.activeBackground': '#5a5a5a',
        'editorWidget.background': '#252525',
        'editorWidget.border': '#3d3d3d',
        'editorSuggestWidget.background': '#252525',
        'editorSuggestWidget.border': '#3d3d3d',
        'editorSuggestWidget.selectedBackground': '#3d3d3d',
        'editorSuggestWidget.highlightForeground': '79c0ff',
        'editorHoverWidget.background': '#252525',
        'editorHoverWidget.border': '#3d3d3d',
        'peekView.border': '#79c0ff40',
        'peekViewEditor.background': '#1e1e1e',
        'peekViewResult.background': '#252525',
        'peekViewTitle.background': '#252525',
        'diffEditor.insertedTextBackground': '#7ee78720',
        'diffEditor.removedTextBackground': '#ff7b7220',
        'diffEditor.insertedLineBackground': '#7ee78710',
        'diffEditor.removedLineBackground': '#ff7b7210',
        // Bracket pair colorization
        'editorBracketHighlight.foreground1': '#79c0ff',
        'editorBracketHighlight.foreground2': '#d2a8ff',
        'editorBracketHighlight.foreground3': '#7ee787',
        'editorBracketHighlight.foreground4': '#ffa657',
        'editorBracketHighlight.foreground5': '#ff7b72',
        'editorBracketHighlight.foreground6': '#a5d6ff',
      },
    });

    // Override Monaco's built-in Ruby with a better Monarch tokenizer
    monaco.languages.setMonarchTokensProvider('ruby', {
      defaultToken: '',
      tokenPostfix: '.ruby',

      keywords: [
        'BEGIN', 'END', 'alias', 'and', 'begin', 'break', 'case', 'class',
        'def', 'defined?', 'do', 'else', 'elsif', 'end', 'ensure', 'false',
        'for', 'if', 'in', 'module', 'next', 'nil', 'not', 'or',
        'redo', 'rescue', 'retry', 'return', 'self', 'super', 'then', 'true',
        'undef', 'unless', 'until', 'when', 'while', 'yield',
      ],

      railsMethods: [
        'require', 'require_relative', 'include', 'extend', 'prepend',
        'attr_reader', 'attr_writer', 'attr_accessor',
        'public', 'private', 'protected',
        'raise', 'fail', 'throw', 'catch', 'proc', 'lambda',
        'puts', 'print', 'p', 'pp', 'freeze',
        'has_one', 'has_many', 'belongs_to', 'has_and_belongs_to_many',
        'validates', 'validate', 'before_action', 'after_action', 'around_action',
        'before_save', 'after_save', 'before_create', 'after_create',
        'before_update', 'after_update', 'before_destroy', 'after_destroy',
        'before_validation', 'after_validation',
        'scope', 'delegate', 'enum',
        'render', 'redirect_to', 'respond_to',
        'has_secure_password', 'serialize',
        'publish_events_on', 'chats_with', 'liquid_context_key',
      ],

      typeKeywords: [
        'Array', 'Hash', 'String', 'Integer', 'Float', 'Symbol', 'NilClass',
        'TrueClass', 'FalseClass', 'Numeric', 'Comparable', 'Enumerable',
        'Kernel', 'Object', 'BasicObject', 'Class', 'Module', 'Struct',
        'Proc', 'Method', 'IO', 'File', 'Dir', 'Time', 'Date', 'DateTime',
        'Regexp', 'Range', 'Encoding', 'Exception', 'StandardError',
        'RuntimeError', 'ArgumentError', 'TypeError', 'NameError',
        'NoMethodError', 'ActiveRecord', 'ApplicationRecord',
        'ActionController', 'ApplicationController',
      ],

      operators: [
        '=', '>', '<', '!', '~', '?', ':', '==', '<=', '>=', '!=',
        '&&', '||', '++', '--', '+', '-', '*', '/', '&', '|', '^', '%',
        '<<', '>>', '>>>', '+=', '-=', '*=', '/=', '&=', '|=', '^=',
        '%=', '<<=', '>>=', '>>>=', '=>', '<=>', '=~', '!~', '**',
        '..', '...',
      ],

      symbols: /[=><!~?:&|+\-*\/\^%]+/,

      tokenizer: {
        root: [
          [/\s+/, ''],
          [/#.*$/, 'comment'],
          [/=begin/, 'comment', '@blockComment'],
          [/\b(class)\b(\s+)([A-Z]\w*)/, ['keyword.class', '', 'class.name']],
          [/\b(module)\b(\s+)([A-Z]\w*)/, ['keyword.module', '', 'module.name']],
          [/\b(def)\b(\s+)(self\.)(\w+[!?=]?)/, ['keyword.def', '', 'keyword', 'method']],
          [/\b(def)\b(\s+)(\w+[!?=]?)/, ['keyword.def', '', 'method']],
          [/\b[A-Z][A-Z_0-9]{2,}\b/, 'constant'],
          [/\b[A-Z]\w+\b/, 'type'],
          [/@{1,2}[a-zA-Z_]\w*/, 'variable.instance'],
          [/\$[a-zA-Z_]\w*/, 'variable.global'],
          [/:[a-zA-Z_]\w*[!?]?/, 'symbol'],
          [/\b(has_one|has_many|belongs_to|has_and_belongs_to_many|validates|validate|before_action|after_action|around_action|before_save|after_save|before_create|after_create|before_update|after_update|before_destroy|after_destroy|before_validation|after_validation|scope|delegate|enum|render|redirect_to|respond_to|include|extend|prepend|require|require_relative|attr_reader|attr_writer|attr_accessor|raise|fail|puts|print|p|pp|freeze|lambda|proc|publish_events_on|chats_with|liquid_context_key|serialize|has_secure_password)\b/, 'method.call'],
          [/"/, 'string', '@doubleString'],
          [/'/, 'string', '@singleString'],
          [/%[qQwWiI]?[{(\[]/, 'string', '@percentString'],
          [/\/(?=[^/\s])/, 'regexp', '@regexp'],
          [/\b\d[\d_]*\.[\d_]+([eE][+-]?\d+)?\b/, 'number'],
          [/\b0[xX][0-9a-fA-F_]+\b/, 'number'],
          [/\b0[bB][01_]+\b/, 'number'],
          [/\b0[oO]?[0-7_]+\b/, 'number'],
          [/\b\d[\d_]*\b/, 'number'],
          [/\.(\s*)([a-z_]\w*[!?]?)/, ['delimiter', 'method.call']],
          [/\b(BEGIN|END|alias|and|begin|break|case|class|def|defined\?|do|else|elsif|end|ensure|false|for|if|in|module|next|nil|not|or|redo|rescue|retry|return|self|super|then|true|undef|unless|until|when|while|yield)\b/, 'keyword'],
          [/\b(public|private|protected)\b/, 'keyword'],
          [/\b([a-z_]\w*[!?]?)(\s*\()/, ['method.call', 'delimiter']],
          [/@symbols/, 'operator'],
          [/[{}()\[\]]/, 'delimiter'],
          [/[;,.]/, 'delimiter'],
          [/\|/, 'delimiter'],
          [/[a-z_]\w*[!?]?/, 'identifier'],
        ],
        blockComment: [
          [/=end/, 'comment', '@pop'],
          [/.*/, 'comment'],
        ],
        doubleString: [
          [/#\{/, 'string.interpolation', '@interpolation'],
          [/\\[\\nrt"#\$]/, 'string.escape'],
          [/"/, 'string', '@pop'],
          [/[^"\\#]+/, 'string'],
          [/./, 'string'],
        ],
        singleString: [
          [/\\./, 'string.escape'],
          [/'/, 'string', '@pop'],
          [/[^'\\]+/, 'string'],
          [/./, 'string'],
        ],
        percentString: [
          [/#\{/, 'string.interpolation', '@interpolation'],
          [/[}\])]/, 'string', '@pop'],
          [/./, 'string'],
        ],
        interpolation: [
          [/\}/, 'string.interpolation', '@pop'],
          { include: 'root' },
        ],
        regexp: [
          [/\\[\\\/]/, 'regexp'],
          [/\/[imxouesn]*/, 'regexp', '@pop'],
          [/[^/\\]+/, 'regexp'],
          [/./, 'regexp'],
        ],
      },
    } as any);

    // Register ERB language (HTML with embedded Ruby)
    monaco.languages.register({ id: 'erb' });
    monaco.languages.setMonarchTokensProvider('erb', {
      defaultToken: '',
      tokenPostfix: '.erb',
      tokenizer: {
        root: [
          [/<%=/, { token: 'delimiter.erb', next: '@erbOutput' }],
          [/<%/, { token: 'delimiter.erb', next: '@erbCode' }],
          [/<%#/, { token: 'comment.erb', next: '@erbComment' }],
          [/<!--/, 'comment.html', '@htmlComment'],
          [/<\/?[\w-]+/, { token: 'tag.html', next: '@htmlTag' }],
          [/&\w+;/, 'string.html'],
          [/[^<&%]+/, ''],
          [/./, ''],
        ],
        erbOutput: [
          [/%>/, { token: 'delimiter.erb', next: '@pop' }],
          [/#\{/, 'string.interpolation', '@rubyInterp'],
          [/"/, 'string.ruby', '@rubyDoubleString'],
          [/'/, 'string.ruby', '@rubySingleString'],
          [/:[a-zA-Z_]\w*/, 'symbol.ruby'],
          [/@{1,2}[a-zA-Z_]\w*/, 'variable.instance.ruby'],
          [/\b(if|unless|else|elsif|end|do|each|map|select|reject|nil|true|false|self|yield|return)\b/, 'keyword.ruby'],
          [/\b[A-Z]\w+\b/, 'type.ruby'],
          [/\.([a-z_]\w*[!?]?)/, ['delimiter', 'method.call.ruby']],
          [/([a-z_]\w*[!?]?)(\s*\()/, ['method.call.ruby', 'delimiter']],
          [/[{}()\[\]]/, 'delimiter'],
          [/[=><!~?&|+\-*\/\^%]+/, 'operator.ruby'],
          [/\b\d+\b/, 'number.ruby'],
          [/[a-z_]\w*/, 'identifier.ruby'],
          [/\s+/, ''],
        ],
        erbCode: [
          [/%>/, { token: 'delimiter.erb', next: '@pop' }],
          { include: 'erbOutput' },
        ],
        erbComment: [
          [/%>/, { token: 'comment.erb', next: '@pop' }],
          [/./, 'comment.erb'],
        ],
        htmlComment: [
          [/-->/, 'comment.html', '@pop'],
          [/./, 'comment.html'],
        ],
        htmlTag: [
          [/\s+/, ''],
          [/([\w-]+)(\s*=\s*)/, ['attribute.name.html', '']],
          [/"[^"]*"/, 'attribute.value.html'],
          [/'[^']*'/, 'attribute.value.html'],
          [/<%=/, { token: 'delimiter.erb', next: '@erbOutput' }],
          [/<%/, { token: 'delimiter.erb', next: '@erbCode' }],
          [/\/?>/, { token: 'tag.html', next: '@pop' }],
          [/./, ''],
        ],
        rubyDoubleString: [
          [/#\{/, 'string.interpolation', '@rubyInterp'],
          [/"/, 'string.ruby', '@pop'],
          [/[^"#]+/, 'string.ruby'],
          [/./, 'string.ruby'],
        ],
        rubySingleString: [
          [/'/, 'string.ruby', '@pop'],
          [/[^']+/, 'string.ruby'],
        ],
        rubyInterp: [
          [/\}/, 'string.interpolation', '@pop'],
          { include: 'erbOutput' },
        ],
      },
    } as any);

    // Register TOML language with Monarch tokenizer
    monaco.languages.register({ id: 'toml' });
    monaco.languages.setMonarchTokensProvider('toml', {
      defaultToken: '',
      tokenPostfix: '.toml',
      tokenizer: {
        root: [
          [/#.*$/, 'comment'],
          [/\[\[[^\]]+\]\]/, 'keyword'], // array of tables
          [/\[[^\]]+\]/, 'keyword'],     // table header
          [/([\w.-]+)(\s*=)/, ['type', 'operator']],
          [/"""/, 'string', '@multilineString'],
          [/"/, 'string', '@string'],
          [/'''/, 'string', '@multilineLiteralString'],
          [/'/, 'string', '@literalString'],
          [/\b(true|false)\b/, 'keyword'],
          [/\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}/, 'number'],
          [/\b\d+\.?\d*([eE][+-]?\d+)?\b/, 'number'],
          [/\b0[xX][0-9a-fA-F]+\b/, 'number'],
          [/[{}()\[\],]/, 'delimiter'],
          [/\s+/, ''],
        ],
        string: [
          [/\\./, 'string.escape'],
          [/"/, 'string', '@pop'],
          [/[^"\\]+/, 'string'],
        ],
        literalString: [
          [/'/, 'string', '@pop'],
          [/[^']+/, 'string'],
        ],
        multilineString: [
          [/\\./, 'string.escape'],
          [/"""/, 'string', '@pop'],
          [/./, 'string'],
        ],
        multilineLiteralString: [
          [/'''/, 'string', '@pop'],
          [/./, 'string'],
        ],
      },
    } as any);

    // Register Dockerfile language
    monaco.languages.register({ id: 'dockerfile' });
    monaco.languages.setMonarchTokensProvider('dockerfile', {
      defaultToken: '',
      tokenPostfix: '.dockerfile',
      tokenizer: {
        root: [
          [/#.*$/, 'comment'],
          [/\b(FROM|RUN|CMD|LABEL|EXPOSE|ENV|ADD|COPY|ENTRYPOINT|VOLUME|USER|WORKDIR|ARG|ONBUILD|STOPSIGNAL|HEALTHCHECK|SHELL|MAINTAINER)\b/i, 'keyword'],
          [/\b(AS)\b/i, 'keyword'],
          [/\$\{?[\w]+\}?/, 'variable'],
          [/"/, 'string', '@string'],
          [/'/, 'string', '@stringS'],
          [/\d+/, 'number'],
          [/[=:]/, 'operator'],
          [/\\$/, 'delimiter'],
        ],
        string: [
          [/\\./, 'string.escape'],
          [/"/, 'string', '@pop'],
          [/[^"\\]+/, 'string'],
        ],
        stringS: [
          [/'/, 'string', '@pop'],
          [/[^']+/, 'string'],
        ],
      },
    } as any);
  });
}
