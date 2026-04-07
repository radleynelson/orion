import { loader } from '@monaco-editor/react';

export function configureMonacoTheme() {
  loader.init().then((monaco) => {
    // Theme with comprehensive token rules matching Cursor/GitHub Dark style
    monaco.editor.defineTheme('orion-dark', {
      base: 'vs-dark',
      inherit: true,
      rules: [
        // Ruby tokens
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
        { token: 'comment.ruby', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'number.ruby', foreground: '79c0ff' },
        { token: 'regexp.ruby', foreground: 'a5d6ff' },
        { token: 'operator.ruby', foreground: 'ff7b72' },
        { token: 'delimiter.ruby', foreground: 'd4d4d4' },
        // General fallbacks
        { token: 'keyword', foreground: 'ff7b72' },
        { token: 'string', foreground: 'a5d6ff' },
        { token: 'comment', foreground: '8b949e', fontStyle: 'italic' },
        { token: 'number', foreground: '79c0ff' },
        { token: 'type', foreground: '7ee787' },
        { token: 'function', foreground: 'd2a8ff' },
        { token: 'variable', foreground: 'ffa657' },
        { token: 'constant', foreground: '79c0ff' },
        { token: 'operator', foreground: 'ff7b72' },
        { token: 'tag', foreground: '7ee787' },
        { token: 'attribute.name', foreground: '79c0ff' },
        { token: 'attribute.value', foreground: 'a5d6ff' },
      ],
      colors: {
        'editor.background': '#1e1e1e',
        'editor.foreground': '#d4d4d4',
        'editor.selectionBackground': 'rgba(108, 182, 255, 0.3)',
        'editor.lineHighlightBackground': '#252525',
        'editorGutter.background': '#1e1e1e',
        'editorLineNumber.foreground': '#5a5a5a',
        'editorLineNumber.activeForeground': '#b0b0b0',
        'editorCursor.foreground': '#d4d4d4',
        'scrollbar.shadow': '#00000000',
        'scrollbarSlider.background': '#3d3d3d80',
        'scrollbarSlider.hoverBackground': '#4a4a4a',
        'scrollbarSlider.activeBackground': '#5a5a5a',
        'editorWidget.background': '#252525',
        'editorWidget.border': '#3d3d3d',
        'diffEditor.insertedTextBackground': '#7ee78720',
        'diffEditor.removedTextBackground': '#ff7b7220',
        'diffEditor.insertedLineBackground': '#7ee78710',
        'diffEditor.removedLineBackground': '#ff7b7210',
      },
    });

    // Override Monaco's built-in Ruby with a better Monarch tokenizer
    // Must dispose existing provider first
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
          // Whitespace
          [/\s+/, ''],

          // Comments
          [/#.*$/, 'comment'],
          [/=begin/, 'comment', '@blockComment'],

          // Class definition: class Name < Parent
          [/\b(class)\b(\s+)([A-Z]\w*)/, ['keyword.class', '', 'class.name']],
          // Module definition
          [/\b(module)\b(\s+)([A-Z]\w*)/, ['keyword.module', '', 'module.name']],
          // Method definition
          [/\b(def)\b(\s+)(self\.)(\w+[!?=]?)/, ['keyword.def', '', 'keyword', 'method']],
          [/\b(def)\b(\s+)(\w+[!?=]?)/, ['keyword.def', '', 'method']],

          // Constants (ALL_CAPS)
          [/\b[A-Z][A-Z_0-9]{2,}\b/, 'constant'],

          // Type names (CamelCase)
          [/\b[A-Z]\w+\b/, 'type'],

          // Instance variables
          [/@{1,2}[a-zA-Z_]\w*/, 'variable.instance'],

          // Global variables
          [/\$[a-zA-Z_]\w*/, 'variable.global'],

          // Symbols
          [/:[a-zA-Z_]\w*[!?]?/, 'symbol'],

          // Rails/Ruby method calls (known methods)
          [/\b(has_one|has_many|belongs_to|has_and_belongs_to_many|validates|validate|before_action|after_action|around_action|before_save|after_save|before_create|after_create|before_update|after_update|before_destroy|after_destroy|before_validation|after_validation|scope|delegate|enum|render|redirect_to|respond_to|include|extend|prepend|require|require_relative|attr_reader|attr_writer|attr_accessor|raise|fail|puts|print|p|pp|freeze|lambda|proc|publish_events_on|chats_with|liquid_context_key|serialize|has_secure_password)\b/, 'method.call'],

          // Strings
          [/"/, 'string', '@doubleString'],
          [/'/, 'string', '@singleString'],
          [/%[qQwWiI]?[{(\[]/, 'string', '@percentString'],

          // Regexp
          [/\/(?=[^/\s])/, 'regexp', '@regexp'],

          // Numbers
          [/\b\d[\d_]*\.[\d_]+([eE][+-]?\d+)?\b/, 'number'],
          [/\b0[xX][0-9a-fA-F_]+\b/, 'number'],
          [/\b0[bB][01_]+\b/, 'number'],
          [/\b0[oO]?[0-7_]+\b/, 'number'],
          [/\b\d[\d_]*\b/, 'number'],

          // Method calls after dot
          [/\.(\s*)([a-z_]\w*[!?]?)/, ['delimiter', 'method.call']],

          // Keywords (must be before identifier rules)
          [/\b(BEGIN|END|alias|and|begin|break|case|class|def|defined\?|do|else|elsif|end|ensure|false|for|if|in|module|next|nil|not|or|redo|rescue|retry|return|self|super|then|true|undef|unless|until|when|while|yield)\b/, 'keyword'],

          // Public/private/protected
          [/\b(public|private|protected)\b/, 'keyword'],

          // Standalone method calls: identifier followed by ( or !
          // e.g., update_columns(...), becomes(...), transaction, reload
          [/\b([a-z_]\w*[!?]?)(\s*\()/, ['method.call', 'delimiter']],

          // Operators
          [/@symbols/, 'operator'],

          // Delimiters
          [/[{}()\[\]]/, 'delimiter'],
          [/[;,.]/, 'delimiter'],

          // Block params
          [/\|/, 'delimiter'],

          // Regular identifiers (local variables, etc.)
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
  });
}
