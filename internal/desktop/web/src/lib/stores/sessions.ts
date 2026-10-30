import { writable, derived, get } from "svelte/store";
import { getAPI, postAPI, type SessionSummary, type TurnView } from "../api";
import { locale, tr, type AppLocale } from "../i18n";
import { workspaceStatus, activeProjectPath, setActiveProjectPath, loadStatus, loadAutonomy } from "./workspace";
import {
  getSelectionForWorkspace,
  rememberSelectionForWorkspace,
  centerView,
  rightPanelView,
  selectedDiffFile,
  recentWorkspaces,
  projectFolders,
  refreshProjectFolders
} from "./settings";
import { openWorkspaceDir } from "../shell";
import {
  selectedTaskId,
  latestPlans,
  conflictBanner,
  tasks,
  ACTIVE_TASK_STATUSES,
  loadTasks,
  loadApprovals
} from "./tasks";

export const sessions = writable<SessionSummary[]>([]);
export const currentSessionId = writable<string>("");
export const sessionTurns = writable<TurnView[]>([]);
export const localPendingTurns = writable<Record<string, TurnView[]>>({});

export interface SessionGroup {
  label: string;
  items: SessionSummary[];
}

export function groupSessionsByDay(
  list: SessionSummary[],
  now = new Date(),
  lang: AppLocale = get(locale)
): SessionGroup[] {
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const yesterdayStart = todayStart - 86_400_000;

  const today: SessionSummary[] = [];
  const yesterday: SessionSummary[] = [];
  const older: SessionSummary[] = [];

  for (const s of list) {
    const t = s.updated_at ? new Date(s.updated_at).getTime() : 0;
    if (t >= todayStart) {
      today.push(s);
    } else if (t >= yesterdayStart) {
      yesterday.push(s);
    } else {
      older.push(s);
    }
  }

  const groups: SessionGroup[] = [];
  if (today.length) groups.push({ label: tr(lang, "Today", "วันนี้"), items: today });
  if (yesterday.length) groups.push({ label: tr(lang, "Yesterday", "เมื่อวาน"), items: yesterday });
  if (older.length) groups.push({ label: tr(lang, "Earlier", "ก่อนหน้านี้"), items: older });
  return groups;
}

export const groupedSessions = derived([sessions, locale], ([$sessions, $locale]) =>
  groupSessionsByDay($sessions, new Date(), $locale)
);

export async function loadSessions(): Promise<SessionSummary[]> {
  const res = await getAPI<SessionSummary[]>("/api/sessions");
  if (res.ok && Array.isArray(res.data)) {
    sessions.set(res.data);
    return res.data;
  }
  return [];
}

function mergeTurns(sessionId: string, serverTurns: TurnView[]): TurnView[] {
  const pending = get(localPendingTurns)[sessionId] || [];
  if (!pending.length) return serverTurns;
  const knownTasks = new Set(serverTurns.map((t) => t.task_id).filter(Boolean));
  const knownInstructions = new Set(serverTurns.filter((t) => !t.task_id).map((t) => t.instruction));
  const extra = pending.filter((p) => {
    if (p.task_id) {
      return !knownTasks.has(p.task_id);
    }
    return !knownInstructions.has(p.instruction);
  });
  return [...serverTurns, ...extra];
}

export async function loadSessionDetail(id: string): Promise<void> {
  if (!id) return;
  const res = await getAPI<{ id: string; turns: TurnView[] }>("/api/sessions/" + encodeURIComponent(id));
  if (res.ok && res.data) {
    sessionTurns.set(mergeTurns(id, res.data.turns || []));
  }
}

export async function selectSession(id: string): Promise<void> {
  if (!id) return;
  currentSessionId.set(id);
  centerView.set("thread");
  const ws = get(workspaceStatus)?.workspace || "";
  rememberSelectionForWorkspace(ws, { session_id: id, task_id: "" });
  await loadSessionDetail(id);
  const turnIds = new Set(get(sessionTurns).map((t) => t.task_id).filter(Boolean));
  const match = get(tasks).find((t) => t.session_id === id || turnIds.has(t.id));
  const nextTaskId = match?.id || "";
  selectedTaskId.set(nextTaskId);
  latestPlans.set([]);
  selectedDiffFile.set("");
  rememberSelectionForWorkspace(ws, { session_id: id, task_id: nextTaskId });
}

let explicitNewSessionId = "";

function normWsPath(p?: string): string {
  return (p || "").replace(/\\/g, "/").replace(/\/+$/, "").toLowerCase();
}

export async function createNewConversation(targetProjectPath?: string): Promise<string> {
  const prevBackendWs = get(workspaceStatus)?.workspace || "";
  const explicitTarget =
    targetProjectPath && targetProjectPath !== "__current__" ? targetProjectPath.trim() : "";
  const latestProject =
    explicitTarget ||
    get(activeProjectPath) ||
    prevBackendWs ||
    get(recentWorkspaces)[0] ||
    get(projectFolders).find((p) => p.active)?.path ||
    get(projectFolders)[0]?.path ||
    "";

  const switchedProject =
    Boolean(latestProject) && normWsPath(latestProject) !== normWsPath(prevBackendWs);

  if (latestProject) {
    setActiveProjectPath(latestProject);
  }

  const fallbackId = "conv-" + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
  explicitNewSessionId = fallbackId;
  currentSessionId.set(fallbackId);
  sessionTurns.set([]);
  selectedTaskId.set("");
  latestPlans.set([]);
  selectedDiffFile.set("");
  conflictBanner.set(null);
  rightPanelView.set("cards");
  centerView.set("thread");
  const ws = latestProject || prevBackendWs;
  rememberSelectionForWorkspace(ws, { session_id: fallbackId, task_id: "" });

  if (typeof document !== "undefined") {
    const input = document.getElementById("task-input") as HTMLTextAreaElement | null;
    input?.focus();
  }

  if (switchedProject) {
    await openWorkspaceDir(latestProject);
  }

  const res = await postAPI<{ id: string }>("/api/sessions", {});
  const id = res.ok && res.data?.id ? res.data.id : fallbackId;
  explicitNewSessionId = id;
  if (get(currentSessionId) === fallbackId && get(selectedTaskId) === "") {
    currentSessionId.set(id);
    const curWs = get(activeProjectPath) || get(workspaceStatus)?.workspace || ws;
    rememberSelectionForWorkspace(curWs, { session_id: id, task_id: "" });
  }
  if (switchedProject) {
    await Promise.all([
      loadStatus(),
      loadSessions(),
      refreshProjectFolders(),
      loadTasks(),
      loadApprovals(),
      loadAutonomy()
    ]);
  } else {
    await Promise.all([loadSessions(), refreshProjectFolders()]);
  }
  return id;
}

export async function deleteSession(id: string): Promise<boolean> {
  if (!id) return false;
  const wasCurrent = get(currentSessionId) === id;
  for (const job of get(tasks)) {
    if (job.session_id === id && ACTIVE_TASK_STATUSES.has(job.status)) {
      void postAPI("/v1/tasks/" + encodeURIComponent(job.id) + "/cancel", {});
    }
  }
  sessions.update((list) => list.filter((s) => s.id !== id));
  localPendingTurns.update((cur) => {
    const next = { ...cur };
    delete next[id];
    return next;
  });
  const res = await postAPI<{ id: string; deleted: boolean }>(
    "/api/sessions/" + encodeURIComponent(id) + "/delete",
    {}
  );
  if (wasCurrent) {
    await createNewConversation();
  } else {
    await loadSessions();
  }
  return res.ok;
}

export async function ensureActiveSession(): Promise<string> {
  const ws = get(workspaceStatus)?.workspace || "";
  const cur = get(currentSessionId);
  const saved = cur || getSelectionForWorkspace(ws).session_id || "";

  // Load existing sessions from the workspace so we reuse active conversation context
  // instead of accidentally creating a blank conversation when one already exists.
  const loaded = await loadSessions();

  if (saved) {
    const existsOnServer = loaded.some((s) => s.id === saved);
    const hasPending = Boolean(get(localPendingTurns)[saved]?.length);
    const isExplicitNew = saved === explicitNewSessionId;
    if (existsOnServer || hasPending || isExplicitNew) {
      currentSessionId.set(saved);
      rememberSelectionForWorkspace(ws, { session_id: saved });
      if (existsOnServer) {
        await loadSessionDetail(saved);
      } else if (!hasPending) {
        sessionTurns.set([]);
      }
      return saved;
    }
  }

  if (loaded.length > 0) {
    const latestId = loaded[0].id;
    currentSessionId.set(latestId);
    rememberSelectionForWorkspace(ws, { session_id: latestId });
    await loadSessionDetail(latestId);
    return latestId;
  }

  return createNewConversation();
}

export function recordOptimisticTurn(sessionId: string, turn: Omit<TurnView, "index" | "timestamp">) {
  if (!sessionId) return;
  const entry: TurnView = {
    ...turn,
    index: get(sessionTurns).length + 1,
    timestamp: new Date().toISOString()
  };
  localPendingTurns.update((cur) => ({
    ...cur,
    [sessionId]: [...(cur[sessionId] || []), entry]
  }));
  if (get(currentSessionId) === sessionId) {
    sessionTurns.update((t) => [...t, entry]);
  }
}
