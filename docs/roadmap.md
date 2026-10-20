# APW roadmap — Core CLI first

Updated 2026-10-17 (Asia/Bangkok). This ordering supersedes the original M0 handoff and revision 6 implementation plan. The existing Desktop remains available; its visual redesign is deferred until the CLI release gates pass.

## 1. Core execution and source safety

Implemented: review/preview/execute separation, explicit apply, dirty Git snapshots, hash conflicts, rooted path and Windows junction checks, transaction recovery, cancellation of subprocess trees, strict model output, baseline diagnostics and executable acceptance checks.

Release gate: an actual Go task reaches a reviewable diff, verifies in an isolated worktree, applies only on request, and preserves source on failure, cancellation or conflict. Offline tests and a disposable-repository CLI smoke cover this. Worktree isolation does not sandbox trusted test code.

## 2. CLI reliability and configuration

Current stabilization work:

- Reject malformed configuration without silently replacing it with defaults; write config atomically.
- Use the selected configuration/workspace for daemon, verify, auth, model and shell commands.
- Keep selected models and explicit fallbacks unchanged during discovery refresh.
- Persist request and conversation IDs; retries deduplicate; /new isolates conversation memory. Conversation saves are atomic and corrupt files are preserved with a visible error.
- Support task inspection, plain diff, cancellation, explicit resume and watch/reattach.
- Preserve Thai input across UTF-8 read boundaries and pasted commands; handle bare / and input failures.
- Keep shell command completion/help consistent with actual task commands.

Gate: regression tests, race tests, vet and an actual installed-binary workflow with a local model fixture pass. Include Windows paths with spaces/Thai, config outside the workspace, daemon restart, repeated request ID and repeated apply. Do not describe live model/provider behavior as verified by the offline fixture.

Validation on 2026-10-17: the disposable CLI fixture passed stable request IDs, mismatched retry rejection, watch/reattach, a Thai/spaced Windows workspace, configuration outside the repository, plan, preview/diff, trust gating, resume, verify, explicit repeated apply, piped shell /new isolation and an actual daemon restart. The fixture used a local canned-response HTTP server and sent no requests to real model providers or Telegram. Git commands now use the bounded process runner; CRLF formatting checks compare normalized source while preserving the original bytes.

## 3. Real-provider acceptance and release preparation

Completed on the configured Windows host: live selected Codex/Flash responses, a real Codex-plan/Flash-patch/verify/explicit-apply task with independent tests, actual console Ctrl+C, forced daemon crash and exact-step resume without automatic model replay, source/build metadata, portable pair builder and legacy task export. See [acceptance](acceptance.md) and [release instructions](release.md) for evidence and artifact promotion checks.

Remaining before claiming all supported transports and channels production ready:

- Opt-in real-provider run for each supported transport: API/local, AGY, Claude CLI, Codex CLI.
- Obtain model identity evidence where the provider exposes it; AGY currently accepts an explicit model argument but emits no identity metadata. Verify real quota/reset behavior with the relevant account; offline error classification does not establish account quota accuracy.
- Complete a clean Windows VM install check; the portable/upgrade checks on this host have a narrower scope.
- Retention/cleanup: policy and explicit `apw cleanup [--dry-run]` are implemented (see [task lifecycle](task-lifecycle.md#retention-and-cleanup)); legacy task work stays protected. Remaining: a dry run and cleanup on the real workspace, reviewing the listed protections before removal.
- Verify the existing Telegram channel against the real service before declaring live chat delivery complete. Discord remains a separate future adapter.

Unconfigured lanes require external service access and appropriate credentials. Desktop work remains deferred; passing the selected CLI workflow does not close unrelated transport/channel gates.

## 4. Desktop — final phase, using the supplied screenshot as the layout reference

Implemented on top of the shared `app.API` service and stored task/events (no separate Desktop executor):

- Left sidebar: workspace & trust controls, `APW.md` project instructions card, conversations (`#session-list`), recent tasks (`#task-list`) with one-click reopen, new conversation (`+ Session ใหม่`), and settings/model switcher (`#settings-dialog`).
- Center: conversation history with clickable `task_id` links, deterministic message rendering (no simulated timing or typewriter gauges), live task progress cards with inline `Cancel` / `Resume` / `Apply` / `Inspect Diff` actions, composer, clickable Brain 1/Brain 2 model pills, and current execution mode.
- Right panel: combined affected/modified files with per-file filter chips, per-file diff headers with `+added`/`-deleted` line counts and full-file viewer link, structured verification/acceptance/reflex evidence (`#acceptance-structured`), explicit apply/resume/cancel footer, and workspace conflict/hash-drift banner (`#artifact-conflict-box`).
- Resizable panels (`#left-resizer`, `#right-resizer` with width persistence in `localStorage`), restrained dark theme, Thai text support, keyboard navigation (`Ctrl+B` left sidebar, `Ctrl+J` right pane, `Ctrl+K` composer focus, `Escape` close dialogs), and persisted conversation/task selection (`apw_desktop_session`, `apw_desktop_task`).

Desktop completion gate: create a conversation, review a repository, preview a change, inspect its diff, execute/verify, cancel/resume, apply and reopen the same task without losing history. Source safety and ownership behavior match CLI tests.

## 5. Next-generation autonomous & self-evolving architecture

Implemented on top of the Triad core and SQLite task store:

- **Cross-File AST Symbol & Call Impact Graph (`internal/surgeon/graph.go`, `GET /api/graph`)**: indexes top-level symbol definitions (`func`, `method`, `struct`, `interface`, `type`) and cross-file call edges across the workspace so repair and re-planning steps know which upstream callers are impacted by a file modification.
- **Diagnostic Context Extraction & Iterative Re-Planning (`internal/executor/replan.go`)**: parses compiler/test diagnostic file references (`file.ext:line`) to attach referenced source files and cross-file call impact to `repairWithReflex`, and invokes `replanWithArchitect` (`MaxReplans`) to synthesize recovery subtasks when initial subtasks fail verification.
- **3-Tier Self-Evolving Memory & Procedural Skill Crystallization (`internal/store/autonomy.go`, `POST /api/autonomy/skills`)**: distills completed task outcomes into persistent `episodic_memory`, automatically recalls relevant lessons (`RecallEpisodicContext`) into the planning prompt, and crystallizes verified task patterns into `.apw/skills/<slug>/SKILL.md`.
- **24/7 Autonomous Runtime & Safety Governor (`internal/app/autonomy.go`, `internal/desktop/autonomy.go`)**: background autonomous loop (`StepAutonomy`) that dispatches due recurring schedules (`autonomy_schedules`) and priority-ordered backlog goals (`autonomy_goals`) inside isolated Git worktrees when the worker is idle, guarded by a rate limit (`max_tasks_per_hour`) and consecutive-failure circuit breaker (`autonomy_governor`).


