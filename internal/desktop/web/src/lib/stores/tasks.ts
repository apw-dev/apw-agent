import { writable, derived, get } from "svelte/store";
import {
  getAPI,
  postAPI,
  type TaskJob,
  type TaskEvent,
  type ToolApprovalRequest,
  type ApprovalsPayload,
  type ExecutionPlan
} from "../api";
import { workspaceStatus } from "./workspace";
import {
  currentSessionId,
  sessionTurns,
  ensureActiveSession,
  loadSessions,
  loadSessionDetail,
  recordOptimisticTurn
} from "./sessions";
import {
  getSelectionForWorkspace,
  rememberSelectionForWorkspace,
  rightCollapsed,
  rightPanelView,
  selectedDiffFile
} from "./settings";

export type ComposerMode = "execute" | "preview" | "review" | "plan" | "unverified";

// Non-terminal core.TaskStatus values: the UI keeps polling these tasks.
export const ACTIVE_TASK_STATUSES = new Set([
  "queued",
  "analyzing",
  "planning",
  "patching",
  "verifying",
  "applying"
]);

// Statuses app.Service.Resume requeues (ready_to_apply is applied, not resumed).
export const RESUMABLE_TASK_STATUSES = new Set(["interrupted", "failed", "cancelled", "preview_ready"]);

export const tasks = writable<TaskJob[]>([]);
export const selectedTaskId = writable<string>("");
export const taskEvents = writable<Record<string, TaskEvent[]>>({});
export const pendingApprovals = writable<ToolApprovalRequest[]>([]);
export const composerMode = writable<ComposerMode>("execute");
export const taskActionError = writable<Record<string, string>>({});
export const conflictBanner = writable<{ taskId: string; message: string } | null>(null);
export const latestPlans = writable<Array<{ id: string; instruction: string; plan: ExecutionPlan }>>([]);

export const selectedTask = derived([tasks, selectedTaskId], ([$tasks, $id]) =>
  $tasks.find((t) => t.id === $id) || null
);

export const runningTasks = derived([tasks, pendingApprovals], ([$tasks, $approvals]) => {
  const approvalTaskIds = new Set($approvals.map((a) => a.task_id).filter(Boolean));
  return $tasks.filter((t) => ACTIVE_TASK_STATUSES.has(t.status) || approvalTaskIds.has(t.id));
});

export interface StepBarState {
  steps: Array<{ label: "Plan" | "Patch" | "Verify" | "Apply"; state: "done" | "active" | "pending" | "failed" }>;
}

/**
 * Maps real task status and events from the API to the 4-step bar (Plan -> Patch -> Verify -> Apply).
 * Never uses simulated timers or fake percentages.
 */
export function mapTaskSteps(job: TaskJob, events: TaskEvent[] = []): StepBarState {
  const st = (job.status || "").toLowerCase();
  const mode = (job.mode || "execute").toLowerCase();
  const stagesSeen = new Set(events.map((e) => (e.stage || "").toLowerCase()));
  // Progress event stages as app.Service records them (planning, step_start, verifying, ...).
  const hasPlan = Boolean(job.report?.plan) || stagesSeen.has("planning");
  const hasSteps = Boolean(job.report?.steps?.length) || stagesSeen.has("step_start");
  const hasVerify =
    Boolean(job.report?.reflex_attempts?.length) || stagesSeen.has("verifying") || stagesSeen.has("baseline");

  if (st === "succeeded") {
    return {
      steps: [
        { label: "Plan", state: "done" },
        { label: "Patch", state: mode === "review" ? "done" : "done" },
        { label: "Verify", state: "done" },
        { label: "Apply", state: "done" }
      ]
    };
  }
  if (st === "ready_to_apply" || st === "preview_ready") {
    return {
      steps: [
        { label: "Plan", state: "done" },
        { label: "Patch", state: "done" },
        { label: "Verify", state: mode === "preview" || mode === "unverified" ? "pending" : "done" },
        { label: "Apply", state: "active" }
      ]
    };
  }
  if (st === "failed" || st === "cancelled" || st === "interrupted" || st === "provider_unavailable") {
    return {
      steps: [
        { label: "Plan", state: hasPlan ? "done" : "failed" },
        { label: "Patch", state: hasSteps ? "done" : hasPlan ? "failed" : "pending" },
        { label: "Verify", state: hasVerify ? "failed" : "pending" },
        { label: "Apply", state: "pending" }
      ]
    };
  }
  // In flight: the core status names the active step.
  if (st === "applying") {
    return {
      steps: [
        { label: "Plan", state: "done" },
        { label: "Patch", state: "done" },
        { label: "Verify", state: "done" },
        { label: "Apply", state: "active" }
      ]
    };
  }
  if (st === "verifying") {
    return {
      steps: [
        { label: "Plan", state: "done" },
        { label: "Patch", state: "done" },
        { label: "Verify", state: "active" },
        { label: "Apply", state: "pending" }
      ]
    };
  }
  if (st === "patching") {
    return {
      steps: [
        { label: "Plan", state: "done" },
        { label: "Patch", state: "active" },
        { label: "Verify", state: "pending" },
        { label: "Apply", state: "pending" }
      ]
    };
  }
  return {
    steps: [
      { label: "Plan", state: "active" },
      { label: "Patch", state: "pending" },
      { label: "Verify", state: "pending" },
      { label: "Apply", state: "pending" }
    ]
  };
}

const watchedIds = new Set<string>();

export async function loadApprovals(): Promise<ToolApprovalRequest[]> {
  const res = await getAPI<ApprovalsPayload>("/api/approvals");
  if (res.ok && res.data) {
    const list = res.data.pending || [];
    pendingApprovals.set(list);
    return list;
  }
  return [];
}

export async function decideApproval(
  id: string,
  decision: "once" | "always" | "deny"
): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI("/api/approvals/" + encodeURIComponent(id) + "/decide", { decision });
  await loadApprovals();
  if (!res.ok) {
    return { ok: false, message: res.message };
  }
  return { ok: true, message: "" };
}

export async function loadTasks(): Promise<TaskJob[]> {
  const res = await getAPI<TaskJob[]>("/v1/tasks");
  if (!res.ok || !Array.isArray(res.data)) {
    return [];
  }
  const list = res.data;
  tasks.set(list);

  const ws = get(workspaceStatus)?.workspace || "";
  const sid = get(currentSessionId);
  const turnIds = new Set(get(sessionTurns).map((t) => t.task_id).filter(Boolean));
  const savedTask = getSelectionForWorkspace(ws).task_id || get(selectedTaskId);
  if (savedTask && list.some((t) => t.id === savedTask)) {
    selectedTaskId.set(savedTask);
  } else if (!get(selectedTaskId) && sid) {
    const sessionTask = list.find((t) => t.session_id === sid || turnIds.has(t.id));
    if (sessionTask) selectedTaskId.set(sessionTask.id);
  } else if (!get(selectedTaskId) && !sid && list.length > 0) {
    selectedTaskId.set(list[0].id);
  }

  for (const job of list.slice(0, 6)) {
    if (ACTIVE_TASK_STATUSES.has(job.status)) {
      void watchTask(job.id);
    }
  }
  return list;
}

export async function selectTask(taskId: string, openReview = false): Promise<TaskJob | null> {
  if (!taskId) return null;
  selectedTaskId.set(taskId);
  selectedDiffFile.set("");
  latestPlans.set([]);
  const known = get(tasks).find((t) => t.id === taskId);
  if (known?.session_id) {
    if (get(currentSessionId) !== known.session_id) {
      currentSessionId.set(known.session_id);
      void loadSessionDetail(known.session_id);
    }
  } else if (known && get(currentSessionId)) {
    const inCurrentTurns = get(sessionTurns).some((t) => t.task_id === taskId);
    if (!inCurrentTurns) {
      currentSessionId.set("");
      sessionTurns.set([]);
    }
  }
  const ws = get(workspaceStatus)?.workspace || "";
  rememberSelectionForWorkspace(ws, { task_id: taskId });
  if (openReview) {
    rightCollapsed.set(false);
    rightPanelView.set("cards");
  }
  const res = await getAPI<TaskJob>("/v1/tasks/" + encodeURIComponent(taskId));
  if (res.ok && res.data) {
    const updated = res.data;
    tasks.update((list) => {
      const idx = list.findIndex((j) => j.id === updated.id);
      if (idx >= 0) {
        const copy = [...list];
        copy[idx] = updated;
        return copy;
      }
      return [updated, ...list];
    });
    if (ACTIVE_TASK_STATUSES.has(updated.status)) {
      void watchTask(updated.id);
    }
    return updated;
  }
  return null;
}

export async function watchTask(taskId: string): Promise<void> {
  if (!taskId || watchedIds.has(taskId)) return;
  watchedIds.add(taskId);
  let cursor = 0;
  const existingEvents = get(taskEvents)[taskId] || [];
  if (existingEvents.length > 0) {
    cursor = existingEvents[existingEvents.length - 1].seq;
  }

  try {
    while (true) {
      const [evRes, jobRes] = await Promise.all([
        getAPI<TaskEvent[]>("/v1/tasks/" + encodeURIComponent(taskId) + "/events?after=" + cursor),
        getAPI<TaskJob>("/v1/tasks/" + encodeURIComponent(taskId))
      ]);
      void loadApprovals();

      if (evRes.ok && Array.isArray(evRes.data) && evRes.data.length > 0) {
        const incoming = evRes.data;
        cursor = incoming[incoming.length - 1].seq;
        taskEvents.update((cur) => ({
          ...cur,
          [taskId]: [...(cur[taskId] || []), ...incoming]
        }));
      }
      if (!jobRes.ok || !jobRes.data) {
        break;
      }
      const job = jobRes.data;
      tasks.update((list) => {
        const idx = list.findIndex((j) => j.id === job.id);
        if (idx >= 0) {
          const next = [...list];
          next[idx] = job;
          return next;
        }
        return [job, ...list];
      });
      if (!ACTIVE_TASK_STATUSES.has(job.status)) {
        await loadSessions();
        const sid = get(currentSessionId);
        if (sid) await loadSessionDetail(sid);
        break;
      }
      await new Promise((r) => setTimeout(r, 400));
    }
  } finally {
    watchedIds.delete(taskId);
  }
}

export async function submitComposerInstruction(
  rawInstruction: string,
  explicitMode?: ComposerMode
): Promise<{ ok: boolean; message: string }> {
  let instruction = rawInstruction.trim();
  if (!instruction) return { ok: false, message: "" };

  let mode: ComposerMode = explicitMode || get(composerMode);
  if (instruction.startsWith("/plan ")) {
    mode = "plan";
    instruction = instruction.slice(6).trim();
  } else if (instruction.startsWith("/review ")) {
    mode = "review";
    instruction = instruction.slice(8).trim();
  } else if (instruction.startsWith("/preview ")) {
    mode = "preview";
    instruction = instruction.slice(9).trim();
  }
  if (!instruction) return { ok: false, message: "" };

  const sessionId = await ensureActiveSession();
  if (mode === "plan") {
    const res = await postAPI<ExecutionPlan>("/api/plan", { instruction });
    if (!res.ok || !res.data) {
      return { ok: false, message: res.message };
    }
    latestPlans.update((cur) => [{ id: res.data!.plan_id || String(Date.now()), instruction, plan: res.data! }, ...cur]);
    recordOptimisticTurn(sessionId, {
      instruction,
      summary: "ExecutionPlan: " + (res.data.objective || res.data.plan_id),
      success: true,
      applied: false
    });
    return { ok: true, message: "" };
  }

  const requestId =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : "req-" + Date.now() + "-" + Math.random().toString(36).slice(2, 8);

  const res = await postAPI<TaskJob>("/api/run", {
    request_id: requestId,
    session_id: sessionId || undefined,
    instruction,
    mode
  });
  if (!res.ok || !res.data) {
    return { ok: false, message: res.message };
  }
  const job = res.data;
  tasks.update((list) => [job, ...list.filter((j) => j.id !== job.id)]);
  selectedTaskId.set(job.id);
  const ws = get(workspaceStatus)?.workspace || "";
  rememberSelectionForWorkspace(ws, { task_id: job.id });
  recordOptimisticTurn(sessionId, {
    task_id: job.id,
    instruction,
    summary: "",
    success: true,
    applied: false
  });
  void watchTask(job.id);
  return { ok: true, message: "" };
}

export async function runProjectVerify(): Promise<{ ok: boolean; message: string }> {
  const res = await postAPI<TaskJob>("/api/verify", {});
  if (!res.ok || !res.data) {
    return { ok: false, message: res.message };
  }
  const job = res.data;
  tasks.update((list) => [job, ...list.filter((j) => j.id !== job.id)]);
  selectedTaskId.set(job.id);
  void watchTask(job.id);
  return { ok: true, message: "" };
}

export async function applyTaskChanges(
  taskId: string,
  approval: "ask" | "auto" = "ask"
): Promise<{ ok: boolean; message: string }> {
  taskActionError.update((m) => ({ ...m, [taskId]: "" }));
  conflictBanner.set(null);
  const pollTimer = setInterval(() => {
    void loadApprovals();
  }, 250);
  try {
    const res = await postAPI<TaskJob>("/api/tasks/" + encodeURIComponent(taskId) + "/apply", { approval });
    if (!res.ok) {
      taskActionError.update((m) => ({ ...m, [taskId]: res.message }));
      if (
        res.status === 409 ||
        res.error === "apply_failed" ||
        res.message.toLowerCase().includes("conflict") ||
        res.message.toLowerCase().includes("drift") ||
        res.message.toLowerCase().includes("changed")
      ) {
        conflictBanner.set({ taskId, message: res.message });
        rightCollapsed.set(false);
        rightPanelView.set("cards");
      }
      return { ok: false, message: res.message };
    }
    if (res.data) {
      const updated = res.data;
      tasks.update((list) => list.map((j) => (j.id === updated.id ? updated : j)));
    }
    await loadTasks();
    await loadSessions();
    const sid = get(currentSessionId);
    if (sid) await loadSessionDetail(sid);
    return { ok: true, message: "" };
  } finally {
    clearInterval(pollTimer);
    void loadApprovals();
  }
}

export async function cancelTaskById(taskId: string): Promise<{ ok: boolean; message: string }> {
  taskActionError.update((m) => ({ ...m, [taskId]: "" }));
  const res = await postAPI<TaskJob>("/v1/tasks/" + encodeURIComponent(taskId) + "/cancel", {});
  if (!res.ok) {
    taskActionError.update((m) => ({ ...m, [taskId]: res.message }));
    return { ok: false, message: res.message };
  }
  await selectTask(taskId);
  return { ok: true, message: "" };
}

export async function resumeTaskById(taskId: string): Promise<{ ok: boolean; message: string }> {
  taskActionError.update((m) => ({ ...m, [taskId]: "" }));
  const res = await postAPI<TaskJob>("/v1/tasks/" + encodeURIComponent(taskId) + "/resume", {});
  if (!res.ok) {
    taskActionError.update((m) => ({ ...m, [taskId]: res.message }));
    return { ok: false, message: res.message };
  }
  const job = res.data;
  if (job) {
    tasks.update((list) => list.map((j) => (j.id === job.id ? job : j)));
    void watchTask(job.id);
  }
  return { ok: true, message: "" };
}
