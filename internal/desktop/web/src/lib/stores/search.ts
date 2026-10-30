import { writable } from "svelte/store";
import { getAPI, type SearchPayload } from "../api";
import { tNow } from "../i18n";

export interface MentionCandidate {
  insert: string;
  display: string;
  meta: string;
  kind: "slash" | "file" | "symbol" | "skill";
  action?: () => void;
}

export interface ActiveToken {
  kind: "slash" | "mention";
  query: string;
  atStart: number;
  atEnd: number;
}

export const SLASH_COMMANDS: Array<{
  cmd: string;
  label: string;
  labelTh?: string;
  meta: string;
  metaTh?: string;
  immediate?: string;
}> = [
  {
    cmd: "/plan ",
    label: "/plan <instruction>",
    labelTh: "/plan <คำสั่ง>",
    meta: "Create an ExecutionPlan before modifying files",
    metaTh: "วางแผน ExecutionPlan ก่อนแก้ไขจริง"
  },
  {
    cmd: "/review ",
    label: "/review <instruction>",
    labelTh: "/review <คำสั่ง>",
    meta: "Read and analyze code without executing commands",
    metaTh: "อ่านและวิเคราะห์โค้ดโดยไม่รันคำสั่ง"
  },
  {
    cmd: "/preview ",
    label: "/preview <instruction>",
    labelTh: "/preview <คำสั่ง>",
    meta: "Generate Diff in an Isolated Worktree",
    metaTh: "สร้าง Diff ใน Isolated Worktree"
  },
  {
    cmd: "/verify",
    label: "/verify",
    meta: "Run code verification via Reflex Pipeline",
    metaTh: "รันตรวจสอบโค้ดด้วย Reflex Pipeline",
    immediate: "verify"
  },
  {
    cmd: "/files",
    label: "/files",
    meta: "Open Workspace File Tree",
    metaTh: "เปิดดูโครงสร้างไฟล์ในโปรเจกต์ (Workspace File Tree)",
    immediate: "files"
  },
  {
    cmd: "/symbols",
    label: "/symbols",
    meta: "Open Workspace AST Symbols",
    metaTh: "เปิดดู AST Symbols ทั้งโปรเจกต์",
    immediate: "symbols"
  },
  {
    cmd: "/graph",
    label: "/graph",
    meta: "Open Cross-File Call Impact Graph",
    metaTh: "เปิดดู Cross-File Call Impact Graph",
    immediate: "graph"
  },
  {
    cmd: "/board",
    label: "/board",
    meta: "Open Agent Board (Interactive Canvas, Structural Diff & Decision Log)",
    metaTh: "เปิดโมดูล Agent Board (Interactive Canvas, Structural Diff & Decision Log)",
    immediate: "board"
  },
  {
    cmd: "/autonomy",
    label: "/autonomy",
    meta: "Open Autonomy 24/7 & Skills view",
    metaTh: "เปิดมุมมอง Autonomy 24/7 & Skills",
    immediate: "autonomy"
  },
  {
    cmd: "/settings",
    label: "/settings",
    meta: "Configure Theme, Language, Models & Sandbox",
    metaTh: "ตั้งค่าธีม ภาษา โมเดล และ Sandbox",
    immediate: "settings"
  },
  {
    cmd: "/new",
    label: "/new",
    meta: "Start a new conversation (Ctrl+N)",
    metaTh: "เริ่มบทสนทนาใหม่ (Ctrl+N)",
    immediate: "new"
  }
];

export function parseInstructionSkillPills(raw: string): { skills: string[]; cleanText: string } {
  const skills: string[] = [];
  const seen = new Set<string>();
  const text = (raw || "").replace(/(?:@skill:([A-Za-z0-9_-]+)|\[<>\s*([A-Za-z0-9_-]+)\])/gi, (_m, g1, g2) => {
    const slug = (g1 || g2 || "").trim();
    if (slug && !seen.has(slug.toLowerCase())) {
      seen.add(slug.toLowerCase());
      skills.push(slug);
    }
    return "";
  });
  return {
    skills,
    cleanText: text.replace(/^\s+/, "").replace(/\s{2,}/g, " ")
  };
}

export const mentionCandidates = writable<MentionCandidate[]>([]);
export const mentionSelectedIndex = writable<number>(0);

let searchSeq = 0;

export function detectActiveToken(value: string, cursor: number): ActiveToken | null {
  const before = (value || "").slice(0, cursor);
  if (/^\/[A-Za-z0-9_-]*$/.test(before)) {
    return { kind: "slash", query: before.slice(1), atStart: 0, atEnd: cursor };
  }
  const m = /(?:^|\s)@(#?[A-Za-z0-9_:./\\-]*)$/.exec(before);
  if (!m) return null;
  const query = m[1] || "";
  const atStart = cursor - query.length - 1;
  return { kind: "mention", query, atStart, atEnd: cursor };
}

export function clearMentionCandidates() {
  searchSeq++;
  mentionCandidates.set([]);
  mentionSelectedIndex.set(0);
}

/**
 * Queries /api/search?q=<query> and discards stale responses if a newer query was issued.
 */
export async function queryMentionCandidates(
  token: ActiveToken | null,
  onImmediateCommand?: (actionName: string) => void,
  fetcher: (q: string) => Promise<SearchPayload | null> = defaultSearchFetcher
): Promise<MentionCandidate[]> {
  if (!token) {
    clearMentionCandidates();
    return [];
  }
  if (token.kind === "slash") {
    searchSeq++;
    const q = token.query.toLowerCase();
    const list: MentionCandidate[] = SLASH_COMMANDS.filter(
      (c) =>
        c.cmd.toLowerCase().includes(q) ||
        c.meta.toLowerCase().includes(q) ||
        (c.metaTh || "").toLowerCase().includes(q)
    ).map((c) => ({
      insert: c.cmd,
      display: tNow(c.label, c.labelTh || c.label),
      meta: tNow(c.meta, c.metaTh || c.meta),
      kind: "slash",
      action: c.immediate && onImmediateCommand ? () => onImmediateCommand(c.immediate!) : undefined
    }));
    mentionCandidates.set(list);
    mentionSelectedIndex.set(0);
    return list;
  }

  const seq = ++searchSeq;
  const rawQ = token.query;
  const data = await fetcher(rawQ);
  if (seq !== searchSeq || !data) {
    return [];
  }

  const symOnly = rawQ.startsWith("#");
  const skillOnly = rawQ.toLowerCase().startsWith("skill:");
  const q = (symOnly ? rawQ.slice(1) : skillOnly ? rawQ.slice(6) : rawQ).toLowerCase();

  const items: MentionCandidate[] = [];
  if (!symOnly) {
    for (const sk of data.skills || []) {
      const slug = sk.slug || "";
      const desc = sk.description || "";
      if (!q || slug.toLowerCase().includes(q) || desc.toLowerCase().includes(q)) {
        items.push({
          insert: "skill:" + slug,
          display: "@skill:" + slug,
          meta: "skill · " + (desc.slice(0, 42) || ".apw/skills"),
          kind: "skill"
        });
      }
    }
  }
  if (!symOnly && !skillOnly) {
    for (const f of data.files || []) {
      items.push({
        insert: f,
        display: "@" + f,
        meta: "file",
        kind: "file"
      });
    }
  }
  if (!skillOnly) {
    for (const s of data.symbols || []) {
      items.push({
        insert: "#" + s.name,
        display: "@#" + s.name,
        meta: `${s.kind || "sym"} · ${s.file}:L${s.start_line || 1}`,
        kind: "symbol"
      });
    }
  }

  const capped = items.slice(0, 8);
  mentionCandidates.set(capped);
  mentionSelectedIndex.set(0);
  return capped;
}

async function defaultSearchFetcher(q: string): Promise<SearchPayload | null> {
  const res = await getAPI<SearchPayload>("/api/search?q=" + encodeURIComponent(q || ""));
  return res.ok && res.data ? res.data : null;
}
