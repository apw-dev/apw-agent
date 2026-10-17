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

Start after core CLI gates are closed. Build on the same application service and stored task/events; the Desktop must not implement its own executor.

Planned composition:

- Left sidebar: projects/workspaces, conversations, recent tasks, new conversation and settings.
- Center: conversation history, task progress, a composer, selected models and current execution mode.
- Right panel: changed files, readable diff, verification/acceptance evidence, explicit apply and conflict state.
- Resizable panels, restrained dark theme, Thai text support and keyboard navigation.
- Persist task/conversation selection so refresh/restart returns to the same work.
- Make Review, Preview, Execute, Cancel, Resume and Apply reflect the shared service state; avoid simulated timing or token gauges.

Desktop completion gate: create a conversation, review a repository, preview a change, inspect its diff, execute/verify, cancel/resume, apply and reopen the same task without losing history. Source safety and ownership behavior must match CLI tests.

The supplied screenshot is a layout reference, not a request to reproduce its brand or to start redesign work now.
