import { writable, derived, get } from "svelte/store";
import { getAPI, postAPI, type SessionSummary, type TurnView } from "../api";
import { workspaceStatus } from "./workspace";
import { getSelectionForWorkspace, rememberSelectionForWorkspace, centerView, selectedDiffFile } from "./settings";
import { selectedTaskId, latestPlans, tasks } from "./tasks";

export const sessions = writable<SessionSummary[]>([]);
export const currentSessionId = writable<string>("");
export const sessionTurns = writable<TurnView[]>([]);
export const localPendingTurns = writable<Record<string, TurnView[]>>({});

export interface SessionGroup {
  label: string;
  items: SessionSummary[];
}

export function groupSessionsByDay(list: SessionSummary[], now = new Date()): SessionGroup[] {
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
  if (today.length) groups.push({ label: "วันนี้", items: today });
  if (yesterday.length) groups.push({ label: "เมื่อวาน", items: yesterday });
  if (older.length) groups.push({ label: "ก่อนหน้านี้", items: older });
  return groups;
}

export const groupedSessions = derived(sessions, ($sessions) => groupSessionsByDay($sessions));

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
  const knownInstructions = new Set(serverTurns.map((t) => t.instruction));
  const extra = pending.filter(
    (p) => !(p.task_id && knownTasks.has(p.task_id)) && !knownInstructions.has(p.instruction)
  );
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

export async function createNewConversation(): Promise<string> {
  const res = await postAPI<{ id: string }>("/api/sessions", {});
  if (!res.ok || !res.data?.id) {
    return "";
  }
  const id = res.data.id;
  currentSessionId.set(id);
  sessionTurns.set([]);
  selectedTaskId.set("");
  latestPlans.set([]);
  selectedDiffFile.set("");
  centerView.set("thread");
  const ws = get(workspaceStatus)?.workspace || "";
  rememberSelectionForWorkspace(ws, { session_id: id, task_id: "" });
  await loadSessions();
  return id;
}

export async function ensureActiveSession(): Promise<string> {
  const ws = get(workspaceStatus)?.workspace || "";
  const saved = getSelectionForWorkspace(ws).session_id || get(currentSessionId);
  if (saved) {
    currentSessionId.set(saved);
    await loadSessionDetail(saved);
    return saved;
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
