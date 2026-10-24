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

export interface TelegramChannelConfig {
  enabled: boolean;
  token_env: string;
  token_configured: boolean;
  token_masked: string;
  allow_users: number[];
}

export interface ChannelsPayload {
  telegram: TelegramChannelConfig;
}

export interface SessionSummary {
  id: string;
  updated_at: string;
  turn_count: number;
  first_instruction?: string;
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

export interface SubagentReport {
  id: string;
  role: string;
  action: string;
  target_file?: string;
  status: "running" | "done" | "failed" | string;
  detail?: string;
}

// The report types mirror the Go JSON (executor.ExecutionReport, reflex.VerificationResult,
// executor.CheckResult). Fixtures generated from Go (src/fixtures, internal/desktop/devfixture)
// keep them honest.
export interface ReflexDiagnostic {
  stage: string;
  file?: string;
  line?: number;
  col?: number;
  message: string;
  raw_error?: string;
}

export interface VerificationResult {
  passed: boolean;
  failed_stage?: string;
  diagnostics?: ReflexDiagnostic[];
  raw_output?: string;
  duration?: number;
  sandbox?: string;
}

export interface ReflexAttempt {
  Number: number;
  Result?: VerificationResult;
}

export interface AcceptanceCheckReport {
  criterion: string;
  passed: boolean;
  evidence: string;
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
  subagents?: SubagentReport[];
  reflex_attempts?: ReflexAttempt[];
  baseline?: VerificationResult;
  acceptance?: AcceptanceCheckReport[];
  applied?: boolean;
  worktree_path?: string;
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

export interface QuickScript {
  id: string;
  label: string;
  command: string;
  category: string;
  source: string;
  default_port?: number;
  target_port?: number;
  port_conflict?: boolean;
  hint_url?: string;
  description?: string;
}

export interface TerminalSessionView {
  id: string;
  command: string;
  cwd: string;
  port?: number;
  status: "running" | "exited" | "failed" | "stopped" | string;
  pid?: number;
  exit_code: number;
  started_at: string;
  updated_at: string;
  output: string;
  urls: string[];
}

export interface ActivePortInfo {
  port: number;
  url: string;
  source?: string;
}

export interface TerminalStatePayload {
  workspace: string;
  scripts: QuickScript[];
  sessions: TerminalSessionView[];
  active_ports: ActivePortInfo[];
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

export interface FileTreeEntry {
  path: string;
  size: number;
}

export interface FilesTreePayload {
  workspace: string;
  source: string;
  files: FileTreeEntry[];
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

export interface BoardArchitectureInfo {
  app_type: string;
  license: string;
  core_base: string;
  diff_engine: string;
  plugin_system: string;
  supported_os: string[];
  integration_mode: string;
  recommended_models: string[];
  active_architect: string;
  active_reflex: string;
  trace_access: string;
  read_only_mode: boolean;
  base_branch: string;
  current_branch: string;
  workspace: string;
}

export interface BoardSequenceNode {
  id: string;
  label: string;
  role: string;
  file: string;
  start_line: number;
  end_line: number;
  description: string;
}

export interface BoardSequenceEdge {
  id: string;
  from: string;
  to: string;
  symbol: string;
  label: string;
  file: string;
  line: number;
  step_order: number;
}

export interface BoardERColumn {
  name: string;
  type: string;
  key?: string;
}

export interface BoardERTable {
  id: string;
  name: string;
  file: string;
  line: number;
  description: string;
  columns: BoardERColumn[];
}

export interface BoardERRelation {
  id: string;
  from_table: string;
  to_table: string;
  cardinality: string;
  label: string;
  file: string;
  line: number;
}

export interface BoardTraceNote {
  id: string;
  stage: string;
  title: string;
  summary: string;
  file: string;
  line: number;
  timestamp: string;
}

export interface BoardDiagramCanvas {
  sequence_nodes: BoardSequenceNode[];
  sequence_edges: BoardSequenceEdge[];
  er_tables: BoardERTable[];
  er_relations: BoardERRelation[];
  trace_notes: BoardTraceNote[];
}

export interface BoardPseudocode {
  input: string;
  process: string;
  output: string;
}

export interface BoardStructuralItem {
  id: string;
  file: string;
  symbol: string;
  category: "core" | "test" | "docs" | string;
  category_label: string;
  auto_collapsed: boolean;
  start_line: number;
  end_line: number;
  added: number;
  deleted: number;
  pseudocode: BoardPseudocode;
  code_snippet: string;
}

export interface BoardDecisionItem {
  id: string;
  kind: "unprompted" | "requested" | string;
  pinned_alert: boolean;
  title: string;
  user_requested: string;
  agent_decision: string;
  rationale: string;
  file: string;
  line: number;
  symbol: string;
}

export interface BoardPromptStep {
  step: number;
  title: string;
  description: string;
  example: string;
}

export interface BoardPromptTemplate {
  id: string;
  name: string;
  category: string;
  description: string;
  steps: BoardPromptStep[];
  prompt_text: string;
}

export interface BoardPrivacySpec {
  telemetry_enabled: boolean;
  redacted_fields: string[];
  beta_constraints: string[];
}

export interface AgentBoardPayload {
  architecture: BoardArchitectureInfo;
  diagram: BoardDiagramCanvas;
  structural_diff: BoardStructuralItem[];
  decision_log: BoardDecisionItem[];
  prompt_templates: BoardPromptTemplate[];
  privacy: BoardPrivacySpec;
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
