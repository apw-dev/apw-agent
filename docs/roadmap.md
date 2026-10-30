# APW Roadmap — Core CLI, Native Desktop & Multi-Agent Runtime

Updated 2026-10-30 (Asia/Bangkok). This ordering supersedes the original M0 handoff and revision 6 implementation plan.

## 1. Core Execution & Source Safety

Implemented: review/preview/execute separation, explicit apply, dirty Git snapshots, hash conflicts, rooted path and Windows junction checks, transaction recovery, cancellation of subprocess trees, strict model output, baseline diagnostics, and executable acceptance checks.

Release gate: an actual Go task reaches a reviewable diff, verifies in an isolated worktree, applies only on request, and preserves source on failure, cancellation, or conflict. Offline tests and a disposable-repository CLI smoke cover this. Worktree isolation does not sandbox trusted test code unless `sandbox` mode (`restricted` or `docker`) is enabled.

## 2. CLI Reliability, Multi-Language Pipelines & Configuration

Implemented and validated:

- Reject malformed configuration without silently replacing it with defaults; write config atomically.
- Use the selected configuration/workspace for daemon, verify, auth, model, and shell commands.
- Automatic model discovery on first launch (`discoverProviders`) while refusing placeholder providers (`apw-builtin`) when no real model is connected; keep selected models and explicit fallbacks unchanged during discovery refresh.
- Persist request and conversation IDs; retries deduplicate; `/new` isolates conversation memory. Conversation saves are atomic and corrupt files are preserved with a visible error.
- Support task inspection, plain diff, cancellation, explicit resume, and watch/reattach.
- Preserve UTF-8/Thai input across read boundaries and pasted commands; handle bare `/` and input failures.
- Built-in verification pipelines across Go (`gofmt`/`go vet`/`go build`/`go test`), Node.js/TypeScript (`npm`/`pnpm`/`bun`/`yarn`), Rust (`cargo`), Python (`ruff`/`pytest`), .NET (`dotnet test`), and static HTML/CSS, plus headless Chromium/Edge browser verification (`internal/browser`) and custom `verify.stages`.
- Direct plain-question answering (`askText`) without creating unnecessary worktree execution plans when the user asks a conversational or analytical question.

Validation: passed on disposable CLI fixtures and `scripts/accept_cli.py` (stable request IDs, mismatched retry rejection, watch/reattach, spaced/Thai Windows paths, configuration outside the repository, plan, preview/diff, trust gating, exact-step crash/resume without model replay, console `Ctrl+C` exit 130, explicit repeated apply, and daemon restart).

## 3. Real-Provider Acceptance, Prompt Caching & Release Packaging

Completed on the configured Windows host (`0.1.0-rc.1` through `0.1.0-rc.15`):

- Live selected Codex/AGY/Claude CLI responses, real multi-model planning and surgical patching, independent hidden test suites (`apw eval`), portable pair builder, and NSIS installer (`APW-Setup-<version>.exe`) with safe user `PATH` handling (never truncating long user `PATH` strings).
- Newly opened folders start untrusted by default (`TestBootstrapWorkspaceDoesNotTrustNewFolders`), requiring explicit trust before running repository build/test commands.
- **Anthropic Prompt Caching & Streaming (`internal/provider/api/anthropic.go`)**: 1-hour cache TTL for long-lived task context and 5-minute ephemeral cache breakpoints (`CachePrefix`) for short-lived subagent loops, SSE streaming for large outputs, exponential backoff with jitter on HTTP 429/5xx, and `Retry-After` header support.
- See [acceptance](acceptance.md) and [release instructions](release.md) for evidence and artifact promotion checks.

Remaining before claiming all supported transports and channels production-ready:

- Opt-in real-provider run for each supported transport where credentials/endpoints are configured: direct Anthropic API (`ANTHROPIC_API_KEY`), local OpenAI-compatible server (e.g., Ollama), AGY, Claude CLI, and Codex CLI.
- Obtain independent model identity and quota/reset evidence per account where the provider exposes it.
- Complete a clean Windows VM installation check.
- Run a non-dry `apw cleanup` on a long-lived workspace after reviewing protected legacy tasks.
- Verify live Telegram channel delivery against the real service with an authorized token and allowlist.

## 4. Desktop — Native Windows Shell (`APW.exe`) & Svelte 5 Frontend

Implemented on top of the shared `app.API` service and stored task/events (no separate Desktop executor), per [the Wails + Svelte 5 design spec](superpowers/specs/2026-10-20-apw-desktop-wails-design.md):

- **Native Windows shell (`cmd/apw-desktop` → `APW.exe`)**:
  - Wails v2.17 native window with in-process `AssetServer.Handler` (no localhost port opened), single-instance lock with workspace forwarding, plus `--new-window` support for independent side-by-side windows.
  - Multi-workspace host (`WorkspaceHost`) keeping opened project servers cached in memory so background tasks continue running across instantaneous project switches.
  - Win32 geometry persistence in `%APPDATA%\APW\desktop.json`, embedded multi-size window/taskbar icons (`WM_SETICON`), Windows 11 Mica backdrop and light/dark caption sync, native folder dialog (`Ctrl+O`), and automatic sibling `APW.exe` launch from `apw desktop`.
- **Svelte 5 + TypeScript + Vite frontend (`internal/desktop/web/`, embedded via `internal/desktop/embed.go`)**:
  - Strict CSP (`script-src 'self'`) with `<meta name="apw-token">` injection, bundled `Noto Sans` & `Noto Sans Thai` fonts, Warm Paper light / Warm Ink dark themes (WCAG 2.1 AA contrast tokens), and instant Bilingual UI (`en` / `th` via `i18n.ts`).
  - Freely resizable left sidebar and right review/file panels with persisted widths:
    - **Multi-Project Sidebar (`Sidebar.svelte`)**: Antigravity-style `Projects` tree across active and recent workspaces with inline sessions, active/running indicators, New Window / Remove Project actions, search filter (`Ctrl+K`), Unified Thread & Recent Tasks, Agent Board, and Autonomy view switches.
    - **Center Thread & Composer (`Thread.svelte`, `Composer.svelte`)**: rich `MarkdownView` (Mermaid diagrams, syntax-highlighted code blocks), inline live `TaskCard`, `ApprovalCard`, `@file` mention autocomplete, model role/failover selector pills, and mode selector (`ask`/`auto`/`plan`/`unverified`).
    - **Agent Board, Model Flow Runner & Prompt Inspector (`AgentBoardView.svelte`, `FlowCanvas.svelte`, `ModelFlowRunner.svelte`, `PromptInspectorModal.svelte`, `RunConsoleDock.svelte`)**: interactive XYFlow canvas for parallel subagent waves, live Architect/Reflex failover route visualization, full outgoing prompt & response inspection (`PromptTrace` with size, latency, model, and stage), and docked terminal/run console.
    - **Right Review & File Tree Panel (`ReviewPanel.svelte`, `FileTreeView.svelte`, `DiffView.svelte`)**: unified/split diff toggle, Shiki syntax highlighting, structured acceptance/verification/reflex evidence, workspace file explorer, conflict banner, and explicit Apply/Discard footer.

Desktop completion gate: covered by Playwright E2E (`internal/desktop/web/e2e/desktop.spec.ts`), frontend contract tests (`src/contract.test.ts` + `TestFrontendFixturesMatchGoTypes`), Vitest unit suites, and Go desktop server tests (`internal/desktop`, `cmd/apw-desktop`).

## 5. Multi-Agent Orchestration, Subagents & Self-Evolving Autonomy

Implemented on top of the Triad core (`Architect` → `Surgeon` → `Reflex`) and SQLite task store:

- **Domain Codebase Auditors & Parallel Subagent Waves (`internal/executor/subagent.go`)**: pre-plan domain auditors (Backend, Frontend, Security/Test) and parallel execution waves for disjoint file mutations, while keeping overlapping patch subtasks sequential for exact-step checkpointing.
- **Model-Requested Read-Only Subagents (`internal/executor/delegate.go`)**: allows the planner and patch/repair steps to delegate targeted codebase investigations (`grep`, `glob`, `symbol`, line-range `read`) to read-only subagents that return `file:line` cited findings.
- **Cross-File AST Symbol & Call Impact Graph (`internal/surgeon/graph.go`, `GET /api/graph`)**: indexes top-level symbol definitions (`func`, `method`, `struct`, `interface`, `type`) and cross-file call edges across the workspace so repair and re-planning steps know which upstream callers are impacted by a file modification.
- **Diagnostic Context Extraction & Iterative Re-Planning (`internal/executor/replan.go`)**: parses compiler/test diagnostic file references (`file.ext:line`) to attach referenced source files and cross-file call impact to `repairWithReflex`, and invokes `replanWithArchitect` (`MaxReplans`) to synthesize recovery subtasks when initial subtasks fail verification.
- **3-Tier Self-Evolving Memory & Procedural Skill Crystallization (`internal/store/autonomy.go`, `POST /api/autonomy/skills`)**: distills completed task outcomes into persistent `episodic_memory`, automatically recalls relevant lessons (`RecallEpisodicContext`) into the planning prompt, and crystallizes verified task patterns into `.apw/skills/<slug>/SKILL.md`.
- **24/7 Autonomous Runtime & Safety Governor (`internal/app/autonomy.go`, `internal/desktop/autonomy.go`)**: background autonomous loop (`StepAutonomy`) that dispatches due recurring schedules (`autonomy_schedules`) and priority-ordered backlog goals (`autonomy_goals`) inside isolated Git worktrees when the worker is idle, guarded by a rate limit (`max_tasks_per_hour`) and consecutive-failure circuit breaker (`autonomy_governor`).
