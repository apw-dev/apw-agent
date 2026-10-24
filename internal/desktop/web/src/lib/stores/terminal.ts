import { writable, get } from "svelte/store";
import {
  getAPI,
  postAPI,
  type QuickScript,
  type TerminalSessionView,
  type ActivePortInfo,
  type TerminalStatePayload
} from "../api";

export const terminalScripts = writable<QuickScript[]>([]);
export const terminalSessions = writable<TerminalSessionView[]>([]);
export const activeWorkspacePorts = writable<ActivePortInfo[]>([]);
export const selectedTerminalSessionId = writable<string>("");
export const terminalDockOpen = writable<boolean>(false);
export const terminalError = writable<string>("");

const SHELL_LANGS = new Set([
  "sh",
  "bash",
  "zsh",
  "shell",
  "powershell",
  "pwsh",
  "ps1",
  "cmd",
  "bat",
  "console",
  "terminal"
]);

const RUNNABLE_PREFIX_RE =
  /^(?:npm|pnpm|yarn|bun|npx|node|deno|go|docker|docker-compose|kubectl|python|python3|pip|pip3|uv|poetry|cargo|rustc|dotnet|mvn|gradle|make|cmake|git|curl|Invoke-RestMethod|Invoke-WebRequest|powershell|pwsh|cmd|vite|next|nuxt|tsc|eslint|vitest|jest|playwright)\b/i;

export function isRunnableCommandSnippet(code: string, lang?: string): boolean {
  const trimmed = (code || "").trim();
  if (!trimmed) return false;
  const normLang = (lang || "").trim().toLowerCase();
  if (normLang && SHELL_LANGS.has(normLang)) {
    return true;
  }
  if (
    normLang &&
    !SHELL_LANGS.has(normLang) &&
    ["json", "yaml", "yml", "html", "css", "sql", "markdown", "md", "diff", "ts", "js", "go", "py", "rust"].includes(
      normLang
    )
  ) {
    return false;
  }
  const lines = trimmed
    .split(/\r?\n/)
    .map((l) => l.trim().replace(/^(?:\$|>|PS[^>]*>)\s*/, ""))
    .filter((l) => l && !l.startsWith("#"));
  if (lines.length === 0 || lines.length > 8) return false;
  return RUNNABLE_PREFIX_RE.test(lines[0]);
}

export function cleanRunnableCommand(code: string): string {
  return (code || "")
    .split(/\r?\n/)
    .map((l) => l.trim().replace(/^(?:\$|>|PS[^>]*>)\s*/, ""))
    .filter((l) => l && !l.startsWith("#"))
    .join(" ; ");
}

export async function loadTerminalState(): Promise<TerminalStatePayload | null> {
  const res = await getAPI<TerminalStatePayload>("/api/terminal");
  if (!res.ok || !res.data) {
    return null;
  }
  terminalScripts.set(res.data.scripts || []);
  const sessions = res.data.sessions || [];
  terminalSessions.set(sessions);
  activeWorkspacePorts.set(res.data.active_ports || []);

  const currentSel = get(selectedTerminalSessionId);
  if (sessions.length > 0 && (!currentSel || !sessions.some((s) => s.id === currentSel))) {
    const running = sessions.find((s) => s.status === "running");
    selectedTerminalSessionId.set((running || sessions[sessions.length - 1]).id);
  }
  return res.data;
}

export async function runCommandInAPW(command: string, hintUrl?: string, port?: number): Promise<boolean> {
  const clean = cleanRunnableCommand(command);
  if (!clean) return false;
  terminalError.set("");
  terminalDockOpen.set(true);
  const res = await postAPI<{ session: TerminalSessionView }>("/api/terminal/run", {
    command: clean,
    mode: "integrated",
    hint_url: hintUrl || "",
    port: port && port > 0 ? port : 0
  });
  if (!res.ok || !res.data?.session) {
    terminalError.set(res.message || "ไม่สามารถรันคำสั่งได้");
    return false;
  }
  const sess = res.data.session;
  selectedTerminalSessionId.set(sess.id);
  await loadTerminalState();
  return true;
}

export async function openCommandInExternalTerminal(command: string, port?: number): Promise<boolean> {
  const clean = cleanRunnableCommand(command);
  if (!clean) return false;
  terminalError.set("");
  const res = await postAPI<{ opened_external: boolean; command: string; port?: number }>("/api/terminal/run", {
    command: clean,
    mode: "external",
    port: port && port > 0 ? port : 0
  });
  if (!res.ok) {
    terminalError.set(res.message || "ไม่สามารถเปิดหน้าต่าง Terminal แยกได้");
    return false;
  }
  return true;
}

export async function stopTerminalSession(id: string): Promise<boolean> {
  if (!id) return false;
  const res = await postAPI<{ session: TerminalSessionView }>("/api/terminal/stop", { id });
  if (!res.ok) {
    terminalError.set(res.message || "หยุดโปรเซสไม่สำเร็จ");
    return false;
  }
  await loadTerminalState();
  return true;
}

export async function clearFinishedTerminalSessions(): Promise<void> {
  await postAPI("/api/terminal/stop", { clear: true });
  await loadTerminalState();
}

export async function copyTextToClipboard(text: string): Promise<boolean> {
  if (!text) return false;
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fallback below
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    document.body.removeChild(ta);
    return true;
  } catch {
    return false;
  }
}
