export type InlineToken =
  | { kind: "text"; text: string }
  | { kind: "bold"; text: string }
  | { kind: "code"; text: string }
  | { kind: "link"; label: string; href: string; filePath?: string };

export type MarkdownBlock =
  | { kind: "heading"; level: 1 | 2 | 3 | 4; tokens: InlineToken[] }
  | { kind: "codeblock"; lang: string; code: string }
  | { kind: "list"; ordered: boolean; items: InlineToken[][] }
  | { kind: "acceptance"; passed: boolean; tokens: InlineToken[] }
  | { kind: "hr" }
  | { kind: "paragraph"; tokens: InlineToken[] };

function extractWorkspaceRelativePath(href: string, label: string): string | undefined {
  const cleanLabel = label.replace(/:L?\d+(?:-L?\d+)?$/i, "").replace(/#L\d+(?:-L\d+)?$/i, "").trim();
  if (href.startsWith("file://")) {
    if (cleanLabel && !cleanLabel.startsWith("http") && (cleanLabel.includes("/") || cleanLabel.includes("."))) {
      return cleanLabel.replace(/^\/+/, "");
    }
  }
  return undefined;
}

export function parseInlineTokens(input: string): InlineToken[] {
  const tokens: InlineToken[] = [];
  const pattern = /(\*\*([^*]+)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)]+)\))/g;
  let lastIdx = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(input)) !== null) {
    if (match.index > lastIdx) {
      tokens.push({ kind: "text", text: input.slice(lastIdx, match.index) });
    }
    if (match[2] !== undefined) {
      tokens.push({ kind: "bold", text: match[2] });
    } else if (match[3] !== undefined) {
      tokens.push({ kind: "code", text: match[3] });
    } else if (match[4] !== undefined && match[5] !== undefined) {
      const label = match[4];
      const href = match[5];
      tokens.push({
        kind: "link",
        label,
        href,
        filePath: extractWorkspaceRelativePath(href, label)
      });
    }
    lastIdx = pattern.lastIndex;
  }

  if (lastIdx < input.length) {
    tokens.push({ kind: "text", text: input.slice(lastIdx) });
  }
  return tokens.length > 0 ? tokens : [{ kind: "text", text: input }];
}

export function parseMarkdownBlocks(raw: string): MarkdownBlock[] {
  if (!raw || !raw.trim()) return [];
  const lines = raw.replace(/\r\n/g, "\n").split("\n");
  const blocks: MarkdownBlock[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    if (!trimmed) {
      i++;
      continue;
    }

    const fenceMatch = /^```([A-Za-z0-9_-]*)\s*$/.exec(trimmed);
    if (fenceMatch) {
      const lang = fenceMatch[1] || "";
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !/^```\s*$/.test(lines[i].trim())) {
        codeLines.push(lines[i]);
        i++;
      }
      if (i < lines.length) i++;
      blocks.push({ kind: "codeblock", lang, code: codeLines.join("\n") });
      continue;
    }

    if (/^---+$/.test(trimmed) || /^\*\*\*+$/.test(trimmed)) {
      blocks.push({ kind: "hr" });
      i++;
      continue;
    }

    const headingMatch = /^(#{1,4})\s+(.+)$/.exec(trimmed);
    if (headingMatch) {
      const level = Math.min(4, Math.max(1, headingMatch[1].length)) as 1 | 2 | 3 | 4;
      blocks.push({ kind: "heading", level, tokens: parseInlineTokens(headingMatch[2].trim()) });
      i++;
      continue;
    }

    if (trimmed.startsWith("Acceptance PASS:") || trimmed.startsWith("Acceptance FAIL:")) {
      const passed = trimmed.startsWith("Acceptance PASS:");
      const body = trimmed.replace(/^Acceptance (?:PASS|FAIL):\s*/, "");
      blocks.push({ kind: "acceptance", passed, tokens: parseInlineTokens(body) });
      i++;
      continue;
    }

    if (/^[-*•]\s+/.test(trimmed)) {
      const items: InlineToken[][] = [];
      while (i < lines.length && /^[-*•]\s+/.test(lines[i].trim())) {
        const itemText = lines[i].trim().replace(/^[-*•]\s+/, "");
        items.push(parseInlineTokens(itemText));
        i++;
      }
      blocks.push({ kind: "list", ordered: false, items });
      continue;
    }

    if (/^\d+\.\s+/.test(trimmed)) {
      const items: InlineToken[][] = [];
      while (i < lines.length && /^\d+\.\s+/.test(lines[i].trim())) {
        const itemText = lines[i].trim().replace(/^\d+\.\s+/, "");
        items.push(parseInlineTokens(itemText));
        i++;
      }
      blocks.push({ kind: "list", ordered: true, items });
      continue;
    }

    blocks.push({ kind: "paragraph", tokens: parseInlineTokens(trimmed) });
    i++;
  }

  return blocks;
}
