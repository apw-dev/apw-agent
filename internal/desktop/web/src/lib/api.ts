export interface StatusPayload {
  version: string;
  brain1_name: string;
  brain1_model?: string;
  brain1_state: string;
  brain1_latency_ms: number;
  brain2_name: string;
  brain2_model?: string;
  brain2_state: string;
  daemon_running: boolean;
  daemon_pid: number;
  skills_loaded: number;
  workspace: string;
  stack: string;
  trusted: boolean;
  sandbox_mode: string;
  instructions_loaded: boolean;
  instructions_path: string;
  instructions_lines: number;
  instructions_size: number;
}

export interface ProviderOption {
  key: string;
  type: string;
  model: string;
}

export interface ModelsPayload {
  architect: string;
  reflex: string;
  approval: string;
  sandbox_mode: string;
  providers: ProviderOption[];
}

export interface SessionSummary {
  id: string;
  updated_at: string;
  turn_count: number;
  last_instruction: string;
}

export interface TurnView {
  index: number;
  task_id?: string;
  timestamp: string;
  instruction: string;
  success: boolean;
  applied: boolean;
  summary: string;
}

export interface SubtaskPlan {
  id: string;
  action: string;
  target_file?: string;
  description: string;
}

export interface ExecutionPlan {
  plan_id: string;
  objective: string;
  affected_files?: string[];
  subtasks?: SubtaskPlan[];
  acceptance_criteria?: string[];
}

export interface SurgeonStepReport {
  subtask_id: string;
  action: string;
  target_file?: string;
  success: boolean;
  message: string;
}

export interface StageOutcome {
  name: string;
  passed: boolean;
  command?: string;
  output?: string;
  duration?: number;
}

export interface ReflexAttempt {
  Number: number;
  Result?: {
    passed: boolean;
    failed_stage?: string;
    duration?: number;
    stages?: StageOutcome[];
    output?: string;
  };
}

export interface AcceptanceCheckReport {
  kind?: string;
  value?: string;
  description?: string;
  passed: boolean;
  detail?: string;
}

export interface TaskReport {
  task_id?: string;
  mode?: string;
  passed?: boolean;
  unverified?: boolean;
  summary?: string;
  diff?: string;
  modified_files?: string[];
  duration?: number;
  plan?: ExecutionPlan;
  steps?: SurgeonStepReport[];
  reflex_attempts?: ReflexAttempt[];
  acceptance_results?: AcceptanceCheckReport[];
}

export interface TaskJob {
  id: string;
  session_id?: string;
  instruction: string;
  mode: string;
  status: string;
  error?: string;
  worktree_path?: string;
  created_at?: string;
  updated_at?: string;
  report?: TaskReport;
}

export interface TaskEvent {
  seq: number;
  stage: string;
  message: string;
  timestamp?: string;
}

export interface ToolApprovalRequest {
  id: string;
  task_id?: string;
  tool_name: string;
  command: string;
  target_path?: string;
  description: string;
  status: string;
  created_at: string;
}

export interface ApprovalsPayload {
  pending: ToolApprovalRequest[];
  always_allowed: string[];
}

export interface SearchSymbol {
  name: string;
  kind: string;
  file: string;
  start_line: number;
  end_line: number;
}

export interface SearchSkill {
  slug: string;
  title?: string;
  description?: string;
}

export interface SearchPayload {
  files: string[];
  symbols: SearchSymbol[];
  skills: SearchSkill[];
}

export interface FilePayload {
  path: string;
  content: string;
  source: string;
  truncated: boolean;
  binary: boolean;
}

export interface SymbolSpan {
  Name: string;
  Kind: string;
  StartLine: number;
  EndLine: number;
}

export interface FileSymbols {
  file: string;
  symbols: SymbolSpan[];
}

export interface CallEdge {
  from_file: string;
  from_symbol: string;
  to_symbol: string;
  line: number;
}

export interface GraphPayload {
  definitions: SearchSymbol[];
  calls: CallEdge[];
}

export interface AutonomyPayload {
  governor: {
    max_tasks_per_hour: number;
    max_consecutive_failures: number;
    consecutive_failures: number;
    paused: boolean;
    pause_reason?: string;
  };
  goals: Array<{
    id: string;
    title: string;
    instruction: string;
    mode: string;
    priority: number;
    status: string;
  }>;
  schedules: Array<{
    id: string;
    name: string;
    instruction: string;
    mode: string;
    interval_sec: number;
    enabled: boolean;
    next_run_at?: string;
  }>;
  episodes: Array<{
    id?: string;
    task_id?: string;
    instruction: string;
    status?: string;
    outcome?: string;
    lesson: string;
  }>;
  skills: string[];
  skills_loaded: number;
}

export interface ApiResult<T> {
  ok: boolean;
  status: number;
  data?: T;
  error?: string;
  message: string;
}

export function getApiToken(): string {
  if (typeof document === "undefined") return "";
  const meta = document.querySelector('meta[name="apw-token"]');
  return meta?.getAttribute("content") || "";
}

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers || {});
  const token = getApiToken();
  if (token) {
    headers.set("Authorization", "Bearer " + token);
  }
  return fetch(path, { ...init, headers });
}

export async function getAPI<T>(path: string): Promise<ApiResult<T>> {
  try {
    const res = await apiFetch(path);
    const text = await res.text();
    let parsed: any = null;
    try {
      parsed = text ? JSON.parse(text) : null;
    } catch {
      parsed = null;
    }
    if (!res.ok) {
      return {
        ok: false,
        status: res.status,
        error: parsed?.error || "http_" + res.status,
        message: parsed?.message || text.trim() || ("HTTP " + res.status)
      };
    }
    return { ok: true, status: res.status, data: parsed as T, message: "" };
  } catch (err: any) {
    return {
      ok: false,
      status: 0,
      error: "network_error",
      message: err?.message || "Failed to connect to APW server"
    };
  }
}

export async function postAPI<T>(path: string, body: unknown = {}): Promise<ApiResult<T>> {
  try {
    const res = await apiFetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {})
    });
    const text = await res.text();
    let parsed: any = null;
    try {
      parsed = text ? JSON.parse(text) : null;
    } catch {
      parsed = null;
    }
    if (!res.ok) {
      return {
        ok: false,
        status: res.status,
        error: parsed?.error || "http_" + res.status,
        message: parsed?.message || text.trim() || ("HTTP " + res.status)
      };
    }
    return { ok: true, status: res.status, data: parsed as T, message: "" };
  } catch (err: any) {
    return {
      ok: false,
      status: 0,
      error: "network_error",
      message: err?.message || "Failed to connect to APW server"
    };
  }
}
