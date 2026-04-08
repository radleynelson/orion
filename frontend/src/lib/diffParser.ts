// Minimal unified-diff parser. Returns hunks of typed lines for inline rendering.

export type DiffLineKind = 'add' | 'del' | 'ctx' | 'hunk';

export interface DiffLine {
  kind: DiffLineKind;
  text: string;
  oldNum?: number; // line number in original file
  newNum?: number; // line number in new file
}

export interface DiffHunk {
  header: string;
  lines: DiffLine[];
}

export interface ParsedDiff {
  hunks: DiffHunk[];
  added: number;
  removed: number;
}

export function parseUnifiedDiff(raw: string): ParsedDiff {
  const result: ParsedDiff = { hunks: [], added: 0, removed: 0 };
  if (!raw) return result;

  const lines = raw.split('\n');
  let current: DiffHunk | null = null;
  let oldNum = 0;
  let newNum = 0;

  for (const line of lines) {
    // Skip file headers; we render the path ourselves.
    if (
      line.startsWith('diff --git') ||
      line.startsWith('index ') ||
      line.startsWith('--- ') ||
      line.startsWith('+++ ') ||
      line.startsWith('new file mode') ||
      line.startsWith('deleted file mode') ||
      line.startsWith('similarity index') ||
      line.startsWith('rename from') ||
      line.startsWith('rename to')
    ) {
      continue;
    }

    if (line.startsWith('@@')) {
      // @@ -oldStart,oldCount +newStart,newCount @@ optional context
      const m = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (m) {
        oldNum = parseInt(m[1], 10);
        newNum = parseInt(m[2], 10);
      }
      current = { header: line, lines: [] };
      result.hunks.push(current);
      continue;
    }

    if (!current) continue;

    if (line.startsWith('+')) {
      current.lines.push({ kind: 'add', text: line.slice(1), newNum });
      newNum++;
      result.added++;
    } else if (line.startsWith('-')) {
      current.lines.push({ kind: 'del', text: line.slice(1), oldNum });
      oldNum++;
      result.removed++;
    } else if (line.startsWith('\\')) {
      // "\ No newline at end of file" — skip
      continue;
    } else {
      // context line (starts with space, or empty)
      const text = line.startsWith(' ') ? line.slice(1) : line;
      current.lines.push({ kind: 'ctx', text, oldNum, newNum });
      oldNum++;
      newNum++;
    }
  }

  return result;
}
