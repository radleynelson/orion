// File extension to Monaco language mapping
const languageMap: Record<string, string> = {
  '.js': 'javascript',
  '.jsx': 'javascript',
  '.ts': 'typescript',
  '.tsx': 'typescript',
  '.go': 'go',
  '.py': 'python',
  '.rb': 'ruby',
  '.rs': 'rust',
  '.java': 'java',
  '.json': 'json',
  '.yaml': 'yaml',
  '.yml': 'yaml',
  '.toml': 'toml',
  '.md': 'markdown',
  '.css': 'css',
  '.scss': 'scss',
  '.html': 'html',
  '.xml': 'xml',
  '.sql': 'sql',
  '.sh': 'shell',
  '.bash': 'shell',
  '.zsh': 'shell',
  '.env': 'plaintext',
  '.txt': 'plaintext',
  '.c': 'c',
  '.cpp': 'cpp',
  '.h': 'c',
  '.hpp': 'cpp',
  '.swift': 'swift',
  '.kt': 'kotlin',
  '.php': 'php',
  '.lua': 'lua',
  '.r': 'r',
  '.vue': 'html',
  '.svelte': 'html',
  '.dockerfile': 'dockerfile',
  '.graphql': 'graphql',
  '.prisma': 'prisma',
  '.erb': 'erb',
  '.haml': 'ruby',
  '.slim': 'ruby',
  '.rake': 'ruby',
  '.gemspec': 'ruby',
  '.jbuilder': 'ruby',
};

const specialFiles: Record<string, string> = {
  'dockerfile': 'dockerfile',
  'makefile': 'makefile',
  'gemfile': 'ruby',
  'rakefile': 'ruby',
  '.gitignore': 'plaintext',
  '.dockerignore': 'plaintext',
};

export function getLanguageFromPath(filePath: string): string {
  const basename = filePath.split('/').pop()?.toLowerCase() || '';
  if (specialFiles[basename]) return specialFiles[basename];
  // Check compound extensions first (e.g., .html.erb, .json.jbuilder)
  const parts = basename.split('.');
  if (parts.length > 2) {
    const compoundExt = '.' + parts.slice(-2).join('.');
    if (compoundExt === '.html.erb' || compoundExt === '.text.erb' || compoundExt === '.js.erb') {
      return 'erb';
    }
  }
  const ext = '.' + parts.pop();
  return languageMap[ext] || 'plaintext';
}
