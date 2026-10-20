export interface DiffLine {
  kind: "hunk" | "add" | "del" | "ctx";
  text: string;
  oldLine?: number;
  newLine?: number;
}

export interface FileDiff {
  file: string;
  added: number;
  deleted: number;
  lines: DiffLine[];
  rawLinesCount: number;
}

export const MAX_HIGHLIGHT_LINES = 3000;

const EXT_TO_LANG: Record<string, string> = {
  go: "go",
  ts: "typescript",
  tsx: "typescript",
  mts: "typescript",
  cts: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  svelte: "svelte",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  md: "markdown",
  sh: "shellscript",
  bash: "shellscript",
  py: "python",
  html: "html",
  htm: "html",
  css: "css",
  sql: "sql"
};

export function detectLanguage(filePath: string): string | null {
  const base = (filePath || "").split("/").pop() || "";
  const idx = base.lastIndexOf(".");
  if (idx < 0) return null;
  const ext = base.slice(idx + 1).toLowerCase();
  return EXT_TO_LANG[ext] || null;
}

export function parseUnifiedDiff(rawDiff: string, fallbackFiles: string[] = []): FileDiff[] {
  const out: FileDiff[] = [];
  const byFile = new Map<string, FileDiff>();

  if (rawDiff && rawDiff.trim()) {
    const lines = rawDiff.replace(/\r\n/g, "\n").split("\n");
    let current: FileDiff | null = null;
    let oldNo = 1;
    let newNo = 1;

    const ensureFile = (name: string): FileDiff => {
      const clean = name.replace(/^[ab]\//, "").trim();
      let existing = byFile.get(clean);
      if (!existing) {
        existing = { file: clean, added: 0, deleted: 0, lines: [], rawLinesCount: 0 };
        byFile.set(clean, existing);
        out.push(existing);
      }
      return existing;
    };

    for (const line of lines) {
      if (line.startsWith("diff --git ")) {
        const m = /^diff --git a\/(.+?) b\/(.+)$/.exec(line);
        current = ensureFile(m ? m[2] : line.slice(11));
        continue;
      }
      if (line.startsWith("--- ")) {
        continue;
      }
      if (line.startsWith("+++ ")) {
        const target = line.slice(4).trim();
        if (target !== "/dev/null") {
          current = ensureFile(target);
        }
        continue;
      }
      if (line.startsWith("index ") || line.startsWith("new file mode") || line.startsWith("deleted file mode")) {
        continue;
      }
      if (!current) {
        if (fallbackFiles.length > 0) {
          current = ensureFile(fallbackFiles[0]);
        } else {
          current = ensureFile("changes.patch");
        }
      }
      if (line.startsWith("@@")) {
        const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
        if (m) {
          oldNo = parseInt(m[1], 10);
          newNo = parseInt(m[2], 10);
        }
        current.lines.push({ kind: "hunk", text: line });
        current.rawLinesCount++;
      } else if (line.startsWith("+")) {
        current.added++;
        current.rawLinesCount++;
        current.lines.push({ kind: "add", text: line.slice(1), newLine: newNo++ });
      } else if (line.startsWith("-")) {
        current.deleted++;
        current.rawLinesCount++;
        current.lines.push({ kind: "del", text: line.slice(1), oldLine: oldNo++ });
      } else if (line.startsWith(" ") || line === "") {
        if (line === "" && current.lines.length === 0) continue;
        current.rawLinesCount++;
        current.lines.push({
          kind: "ctx",
          text: line.startsWith(" ") ? line.slice(1) : line,
          oldLine: oldNo++,
          newLine: newNo++
        });
      }
    }
  }

  for (const f of fallbackFiles) {
    const clean = f.replace(/\\/g, "/");
    if (!byFile.has(clean)) {
      const entry: FileDiff = { file: clean, added: 0, deleted: 0, lines: [], rawLinesCount: 0 };
      byFile.set(clean, entry);
      out.push(entry);
    }
  }
  return out;
}

export interface HighlightToken {
  content: string;
  color?: string;
}

let highlighterPromise: Promise<any> | null = null;
const loadedLangs = new Set<string>();

const LANG_IMPORTERS: Record<string, () => Promise<any>> = {
  go: () => import("shiki/langs/go.mjs"),
  typescript: () => import("shiki/langs/typescript.mjs"),
  javascript: () => import("shiki/langs/javascript.mjs"),
  svelte: () => import("shiki/langs/svelte.mjs"),
  json: () => import("shiki/langs/json.mjs"),
  yaml: () => import("shiki/langs/yaml.mjs"),
  markdown: () => import("shiki/langs/markdown.mjs"),
  shellscript: () => import("shiki/langs/shellscript.mjs"),
  python: () => import("shiki/langs/python.mjs"),
  html: () => import("shiki/langs/html.mjs"),
  css: () => import("shiki/langs/css.mjs"),
  sql: () => import("shiki/langs/sql.mjs")
};

async function getHighlighter() {
  if (!highlighterPromise) {
    highlighterPromise = (async () => {
      const [{ createHighlighterCore }, { createJavaScriptRegexEngine }, githubLight, githubDark] =
        await Promise.all([
          import("shiki/core"),
          import("shiki/engine/javascript"),
          import("shiki/themes/github-light.mjs"),
          import("shiki/themes/github-dark.mjs")
        ]);
      return createHighlighterCore({
        themes: [githubLight.default || githubLight, githubDark.default || githubDark],
        langs: [],
        engine: createJavaScriptRegexEngine()
      });
    })();
  }
  return highlighterPromise;
}

export async function tokenizeCodeLines(
  lines: string[],
  filePath: string,
  dark: boolean
): Promise<HighlightToken[][] | null> {
  if (lines.length === 0 || lines.length > MAX_HIGHLIGHT_LINES) {
    return null;
  }
  const lang = detectLanguage(filePath);
  if (!lang || !LANG_IMPORTERS[lang]) {
    return null;
  }
  try {
    const hl = await getHighlighter();
    if (!loadedLangs.has(lang)) {
      const mod = await LANG_IMPORTERS[lang]();
      await hl.loadLanguage(mod.default || mod);
      loadedLangs.add(lang);
    }
    const result = hl.codeToTokens(lines.join("\n"), {
      lang,
      theme: dark ? "github-dark" : "github-light"
    });
    return result.tokens.map((lineTokens: any[]) =>
      lineTokens.map((t: any) => ({ content: t.content, color: t.color }))
    );
  } catch {
    return null;
  }
}
