import { writable } from "svelte/store";
import {
  getAPI,
  postAPI,
  type StatusPayload,
  type ModelsPayload,
  type AutonomyPayload,
  type FileSymbols,
  type GraphPayload,
  type FilePayload
} from "../api";

export const workspaceStatus = writable<StatusPayload | null>(null);
export const connectionState = writable<"connecting" | "connected" | "error">("connecting");
export const connectionError = writable<string>("");
export const modelsState = writable<ModelsPayload | null>(null);
export const autonomyState = writable<AutonomyPayload | null>(null);
export const instructionsContent = writable<{ path: string; content: string; truncated: boolean } | null>(null);
export const workspaceSymbols = writable<FileSymbols[]>([]);
export const workspaceGraph = writable<GraphPayload | null>(null);
export const activeFileView = writable<FilePayload | null>(null);
export const inlineWorkspaceError = writable<string>("");

export async function loadStatus(): Promise<StatusPayload | null> {
  const res = await getAPI<StatusPayload>("/api/status");
  if (!res.ok || !res.data) {
    connectionState.update((cur) => (cur === "connected" ? "error" : "connecting"));
    connectionError.set(res.message || "กำลังเชื่อมต่อกับบริการ APW...");
    return null;
  }
  workspaceStatus.set(res.data);
  connectionState.set("connected");
  connectionError.set("");
  return res.data;
}

export async function loadModels(): Promise<ModelsPayload | null> {
  const res = await getAPI<ModelsPayload>("/api/models");
  if (res.ok && res.data) {
    modelsState.set(res.data);
    return res.data;
  }
  return null;
}

export async function saveModelsConfig(patch: {
  architect?: string;
  reflex?: string;
  approval?: string;
  sandbox_mode?: string;
}): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI<ModelsPayload>("/api/models", patch);
  if (!res.ok || !res.data) {
    return { ok: false, message: res.message };
  }
  modelsState.set(res.data);
  await loadStatus();
  return { ok: true, message: "" };
}

export async function toggleWorkspaceTrust(trusted: boolean): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI<{ trusted: boolean }>("/api/trust", { trusted });
  if (!res.ok) {
    inlineWorkspaceError.set(res.message);
    return { ok: false, message: res.message };
  }
  inlineWorkspaceError.set("");
  await loadStatus();
  return { ok: true, message: "" };
}

export async function loadInstructions(): Promise<{ ok: boolean; message: string }> {
  const res = await getAPI<{ path: string; content: string; truncated: boolean }>("/api/instructions");
  if (!res.ok || !res.data) {
    return { ok: false, message: res.message };
  }
  instructionsContent.set(res.data);
  return { ok: true, message: "" };
}

export async function initInstructionsFile(): Promise<{ ok: boolean; path?: string; message: string }> {
  const res = await postAPI<{ path: string; created: boolean }>("/api/instructions/init", {});
  if (!res.ok || !res.data) {
    inlineWorkspaceError.set(res.message);
    return { ok: false, message: res.message };
  }
  inlineWorkspaceError.set("");
  await loadStatus();
  await loadInstructions();
  return { ok: true, path: res.data.path, message: "" };
}

export async function loadAutonomy(): Promise<AutonomyPayload | null> {
  const res = await getAPI<AutonomyPayload>("/api/autonomy");
  if (res.ok && res.data) {
    autonomyState.set(res.data);
    return res.data;
  }
  return null;
}

export async function updateGovernorState(patch: {
  max_tasks_per_hour: number;
  max_consecutive_failures: number;
  paused: boolean;
  pause_reason?: string;
}): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/autonomy/governor", patch);
  if (!res.ok) return { ok: false, message: res.message };
  await loadAutonomy();
  return { ok: true, message: "" };
}

export async function addGoal(title: string, instruction: string, priority = 10): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/autonomy/goals", { title, instruction, priority });
  if (!res.ok) return { ok: false, message: res.message };
  await loadAutonomy();
  return { ok: true, message: "" };
}

export async function deleteGoal(id: string): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/autonomy/goals", { action: "delete", id });
  if (!res.ok) return { ok: false, message: res.message };
  await loadAutonomy();
  return { ok: true, message: "" };
}

export async function addSchedule(
  name: string,
  instruction: string,
  interval_sec: number
): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/autonomy/schedules", {
    name,
    instruction,
    interval_sec,
    mode: "execute"
  });
  if (!res.ok) return { ok: false, message: res.message };
  await loadAutonomy();
  return { ok: true, message: "" };
}

export async function toggleSchedule(id: string, enabled: boolean): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/autonomy/schedules", { action: "toggle", id, enabled });
  if (!res.ok) return { ok: false, message: res.message };
  await loadAutonomy();
  return { ok: true, message: "" };
}

export async function deleteSchedule(id: string): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/autonomy/schedules", { action: "delete", id });
  if (!res.ok) return { ok: false, message: res.message };
  await loadAutonomy();
  return { ok: true, message: "" };
}

export async function createProceduralSkill(payload: {
  slug?: string;
  title?: string;
  instruction?: string;
  task_id?: string;
}): Promise<{ ok: boolean; status: number; message: string; path?: string }> {
  const res = await postAPI<{ path: string; skill_path: string }>("/api/autonomy/skills", payload);
  if (!res.ok) {
    return { ok: false, status: res.status, message: res.message };
  }
  await loadAutonomy();
  await loadStatus();
  return { ok: true, status: res.status, message: "", path: res.data?.skill_path || res.data?.path };
}

export async function fetchWorkspaceFile(path: string, taskId?: string): Promise<{ ok: boolean; message: string }> {
  const q = "/api/file?path=" + encodeURIComponent(path) + (taskId ? "&task=" + encodeURIComponent(taskId) : "");
  const res = await getAPI<FilePayload>(q);
  if (!res.ok || !res.data) {
    return { ok: false, message: res.message };
  }
  activeFileView.set(res.data);
  return { ok: true, message: "" };
}

export async function fetchWorkspaceSymbols(): Promise<{ ok: boolean; message: string }> {
  const res = await getAPI<FileSymbols[]>("/api/symbols");
  if (!res.ok) return { ok: false, message: res.message };
  workspaceSymbols.set(res.data || []);
  return { ok: true, message: "" };
}

export async function fetchWorkspaceGraph(): Promise<{ ok: boolean; message: string }> {
  const res = await getAPI<GraphPayload>("/api/graph");
  if (!res.ok || !res.data) return { ok: false, message: res.message };
  workspaceGraph.set(res.data);
  return { ok: true, message: "" };
}
