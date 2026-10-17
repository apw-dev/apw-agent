# Task lifecycle and operations

CLI, Desktop and the daemon's Telegram channel use the same workspace application service and SQLite task store. Install both executables together:

```powershell
go install ./cmd/apw ./cmd/apwd
apw daemon status
apw desktop
```

## Modes and source changes

| Command | Repository commands | Source workspace |
| --- | --- | --- |
| `apw review "<instruction>"` | None | Read for model context |
| `apw preview "<instruction>"` | None; Git creates a snapshot worktree with hooks and checkout filters disabled | No source patch |
| `apw trust` | None | Saves `policy.trusted: true` in the selected config |
| `apw run "<instruction>"` | Baseline and final fmt/vet/build/test in the worktree | Waits for explicit apply |
| `apw verify .` | fmt/vet/build/test in a snapshot worktree; no model repair | No source patch |
| `apw apply <task-id>` | No model call or repository tests | Applies the verified set after conflict checks |

Review and preview still contact the configured model provider. Source context is bounded to 96 KiB and marks truncation; it is not a complete repository audit. The built-in mock provider is for offline workflow checks.

Execute/verify require an explicit trusted workspace and a Git repository with an initial commit. Trust authorizes repository Go code, tests and build tooling to run with the current user's permissions. The worktree isolates ordinary edits; it is not an OS security sandbox for hostile tests.

Provider CLIs run from a separate temporary directory with the selected model, stdin prompts and their restricted tool/sandbox settings. The executor itself enforces mode restrictions. Review never enters mutation or verification code. Preview supports structured source edits but skips repository commands. Go verification uses `-mod=readonly` and sends build output to the null device. Formatting uses Go's formatter to compare source, accepts equivalent CRLF/LF endings without rewriting files, and skips state folders, dependencies, generated files and test fixtures.

## Inspect, continue and apply

```powershell
apw tasks
apw task <task-id>
apw diff <task-id>
apw watch <task-id>
apw cancel <task-id>
apw resume <task-id>
apw apply <task-id>
```

`apw watch <task-id>` follows an existing job without resubmitting it; Ctrl+C detaches. Ctrl+C while `run/review/preview` is following its submitted job requests cancellation. Use `--request-id <stable-id>` with the same instruction, mode and session to retry a submission safely. A changed body with the same ID is rejected.

`--session <id>` selects an owner-bound conversation; omitting it uses the owner's default history. Interactive `/new` starts a distinct conversation and persists that ID with subsequent tasks. Conversation files are saved atomically. Unreadable or corrupt saved context stops the task and is preserved instead of being replaced by empty history. `apw shell --config <path>` uses that configuration for shell tasks.

`model/auth/daemon/verify` accept `--config <path>`. Explicit missing or malformed files fail instead of silently loading defaults. A daemon tied to another config file in the same workspace is rejected; stop it before changing files. Discovery refresh updates the selectable catalog and keeps selected roles/fallbacks.

Add `--config <path>` before the task argument to select a workspace configuration. `apw task` prints status, report, diff and acceptance evidence. A successful execute ends at `ready_to_apply`. Preview ends at `preview_ready` and cannot apply directly: resume it to execute verification first. Resume does not automatically apply.

Patch/create/delete use rooted file access, hash checks and a per-workspace writer lock. Symlink/junction escape attempts and internal state/secret paths are rejected. Snapshots include existing dirty tracked files, staged renames/deletes and untracked files. The diff is relative to that initial snapshot so pre-existing user edits are not presented as generated changes.

Verification rejects unexpected worktree changes outside the executor's recorded edit set. Apply checks the original HEAD and destination hashes, checks that the verified worktree files still match, records a durable journal, and rolls back a failed apply. Recovery refuses to overwrite later user edits. Crash recovery is transactional through a journal; visibility of a multi-file update to unrelated external processes is not atomic. External editors do not participate in APW's process lock.

Planning includes source contents; directives and plans reject incomplete or invalid responses. Every acceptance criterion requires explicit supported checks (`file_exists`, `file_absent`, `contains`, `not_contains`, `go_symbol`, `unchanged`, `only_changes`, `verification`). `unchanged` compares file bytes with the task snapshot; `only_changes` restricts the changed file set; `verification` checks the actual fixed Go verifier result (`all`, `fmt`, `vet`, `build`, `test`). Model-provided commands are not executed as acceptance checks. Passing the Go verification pipeline alone does not satisfy arbitrary functional criteria. Baseline comparisons say which diagnostics were observed before, not observed before, or no longer observed; an early baseline failure may hide later failures.

For a legacy task without a snapshot manifest, use `apw recover --config <path> <legacy-task-id>`. It exports the original plan/checkpoint, dirty source files, hashes and a binary Git diff into a fresh `.apw/recovery` directory. It preserves the old checkout and does not call a model or apply its changes. Review the evidence before preparing a new task.

## Durable state and daemon

State lives under the workspace:

- `.apw/tasks.db`: owner-bound jobs, events, Telegram inbox/outbox.
- `.apw/plans/` (plan and checkpoint files): saved plan/progress, source-read context, pending mutation and failover cooldowns.
- `.worktrees/<task-id>/`: retained source snapshot and edits.
- `.apw/transactions/`: prepared journals and committed apply receipts.
- `.apw/daemon.json`: local endpoint and authentication token; `daemon.log`: startup/runtime output.

Status performs an authenticated health request and verifies process, workspace and protocol. It never invents a daemon process. `apw daemon start` starts it; `apw daemon stop` requests authenticated shutdown. One service owns the workspace. Jobs interrupted by shutdown/restart remain `interrupted` and require explicit resume; the daemon does not silently repeat model calls. Cancellation terminates process descendants using Windows jobs or Unix process groups. Persistence failure stops the service.

The loopback API uses Bearer authentication, loopback Host checks, same-origin browser checks and bounded strict JSON bodies. Desktop has its own per-process token and forwards task calls to the daemon. Task submission uses a request ID: retrying the same owner/request/body returns the original task. Local clients cannot claim Telegram ownership.

Model/trust changes in the daemon's configuration file are reloaded before new work. Environment-variable credentials belong to the daemon process; restart it after changing those variables. A workspace-root change requires restarting the daemon. Configuration paths must refer to the same workspace; use one configuration per running daemon.

Legacy checkpoints without a matching snapshot manifest are preserved but cannot safely apply/resume through the new pipeline. Inspect their worktree and review/export their diff manually before creating a new task. Do not delete a worktree containing needed changes.

## Retention and cleanup

Nothing is deleted while a task runs, and nothing is deleted silently by default. Cleanup is an explicit command that runs with the workspace service lease, so it refuses while the daemon owns the workspace:

```powershell
apw cleanup --dry-run            # list what would be removed and why others are kept
apw daemon stop
apw cleanup                      # remove eligible state
apw cleanup --export-unapplied   # export unapplied worktree changes to .apw/recovery first, then remove
apw cleanup --vacuum             # also rebuild tasks.db (VACUUM) to return freed pages to the OS
```

`--dry-run` is read-only and also works while the daemon runs. `--json` prints the machine-readable report.

Eligible tasks (age is measured from the task's last status change):

| State | Default age | Notes |
| --- | --- | --- |
| `succeeded` (applied, or review/verify) | 14 days | Applied work already lives in the source workspace. |
| `failed`, `cancelled`, `provider_unavailable` | 30 days | After cleanup the task can no longer be resumed. |

Never removed automatically, whatever its age:

- `ready_to_apply`, `preview_ready`, `interrupted` and every non-terminal state (resumable or unapplied work).
- Task IDs in `retention.protect_tasks`, and the legacy task `task-1792166521189239500`.
- Any worktree without a snapshot manifest (legacy work): export it with `apw recover` and remove it by hand after review.
- A task whose apply journal is not finalized (`prepared`); service startup recovery owns it.
- A task with more than one checkpoint, a worktree whose HEAD moved past the snapshot base (commits were made inside it), a worktree not registered with Git, or one reached through a link/junction.
- A worktree that contains unapplied changes relative to its snapshot, unless a completed export already exists in `.apw/recovery/<task-id>-*` or `--export-unapplied` produces one first. If the export fails, the worktree stays.
- Unknown directories under `.worktrees/` (listed as orphans, never removed).
- The source workspace itself, `.apw/recovery` exports, conversation history, configuration and committed apply receipts.

For an eligible task cleanup removes, in order: the worktree through `git worktree remove --force` and its `apw/` branch through `git branch -D` (both through the bounded Git runner, only after confirming the registered path, branch and base commit), the task's `.apw/plans/<plan>.json` and `.checkpoint.json`, and a `rolled_back` apply journal. Committed apply receipts stay because they prove the task was applied. Only when every file step succeeded does one SQLite transaction delete the task's events and clear its report BLOB; the `tasks`/`task_runs` rows keep status and error, and a single `retention` event records the pruning. A failed step leaves that task's evidence for the next run.

The same transaction deletes delivered Telegram outbox rows older than 7 days (undelivered rows are kept) and keeps only the newest 1000 inbox update IDs per account, which preserves the Telegram offset and recent deduplication. Afterwards SQLite runs `incremental_vacuum` when the database uses incremental auto-vacuum (new databases do), or a full `VACUUM` with `--vacuum`. `.apw/daemon.log` is rotated to `daemon.log.1` (one rotated file kept) when it exceeds 8 MiB, both when the CLI starts the daemon and during cleanup.

Configuration (`apw.json`; omitted or `0` uses the default, a negative value disables that rule):

```json
"retention": {
  "succeeded_days": 14,
  "failed_days": 30,
  "outbox_sent_days": 7,
  "inbox_keep": 1000,
  "log_max_mb": 8,
  "auto_cleanup": false,
  "protect_tasks": []
}
```

`auto_cleanup: true` makes `apwd` run the same conservative pass once at startup, before it accepts requests. It never exports, never runs a full `VACUUM` and never rotates its own log; results go to `daemon.log`.

## Telegram

Set `APW_TELEGRAM_TOKEN` and `APW_TELEGRAM_ALLOW` (comma-separated numeric user IDs) before starting `apwd`. Commands are restricted to allowlisted private users and that workspace; each user's tasks are isolated by owner:

`/review`, `/preview`, `/run`, `/status [task-id]`, `/report <task-id>`, `/cancel <task-id>`, `/resume <task-id>`, `/apply <task-id>`.

Inbox updates and outgoing replies persist in SQLite. Task submissions deduplicate by Telegram update ID. Failed sends retry with bounded backoff and Telegram `retry_after`. Delivery is at least once: a crash after Telegram accepts a message but before the local receipt can produce a duplicate reply.

## Validation

```powershell
go test -mod=readonly -race -count=1 -skip 'TestLive|TestBot_LiveOptIn' ./...
go vet ./...
go build ./cmd/apw ./cmd/apwd
```

Offline tests cover workspace conflicts, dirty snapshots, recovery, strict model output, acceptance checks, process descendants, owner isolation, deduplication, daemon health, HTTP guards and Telegram retry persistence. Live provider quota accuracy and real Telegram delivery require the respective external services and are not established by offline tests.
