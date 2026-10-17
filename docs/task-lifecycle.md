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
| `apw run --auto "<instruction>"` | Same as `run` | Applies automatically once verification and acceptance pass |
| `apw run --no-verify "<instruction>"` | None (unverified mode; no trust needed) | Waits for explicit apply; only file-level acceptance checks ran |
| `apw verify .` | fmt/vet/build/test in a snapshot worktree; no model repair | No source patch |
| `apw apply <task-id>` | No model call or repository tests | Applies the verified set after conflict checks |

### Interactive approval modes

The interactive shell (`apw`) decides what a free-text request does by its mode; `/mode <name>` or shift+tab changes it, and `apw.json` `"approval"` sets the default:

| Mode | What a request does |
| --- | --- |
| `ask` (default) | Plan, patch and verify; when the task is ready, ask `Apply ... [y/N]` and apply on yes. |
| `auto` | Plan, patch and verify; apply automatically when verification and acceptance pass. |
| `plan` | Only plan (like `apw plan`); nothing is patched or run. |
| `unverified` | Plan and patch without running install, build, test or the browser check (no trust needed); file-level acceptance checks (`contains`, `only_changes`, `unchanged`, ...) still run, and the shell asks before applying. The report and the apply prompt say UNVERIFIED. |

`/run` always verifies (and applies automatically only in `auto` mode); `/plan`, `/preview` and `/review` keep their own meaning. Declining the apply prompt keeps the task ready; `apw apply <task-id>` applies it later. Telegram and Desktop keep explicit apply.

When the shell starts in an untrusted workspace it explains what trust allows (the project's own install, build and test commands run in an isolated worktree with the user's permissions), suggests the verification sandbox when it is off, and asks `Trust this folder? [y/N]`. A verified request in an untrusted workspace asks again; `plan`, `review`, `preview` and `unverified` never need trust.

Review and preview still contact the configured model provider. Source context is bounded to 96 KiB and marks truncation; it is not a complete repository audit. The built-in mock provider is for offline workflow checks.

Execute/verify (not unverified mode) require an explicit trusted workspace and a Git repository with an initial commit. Trust authorizes repository Go code, tests and build tooling to run with the current user's permissions. The worktree isolates ordinary edits; it is not an OS security sandbox for hostile tests unless a verification sandbox is configured (see [Verification sandbox](#verification-sandbox)).

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

State lives under the workspace, except task worktrees (see [Worktree location](#worktree-location)):

- `.apw/tasks.db`: owner-bound jobs, events, Telegram inbox/outbox.
- `.apw/plans/` (plan and checkpoint files): saved plan/progress, source-read context, pending mutation and failover cooldowns.
- `<worktree root>/<workspace-key>/<short-id>/`: retained source snapshot and edits, outside the workspace. Older tasks keep theirs in `.worktrees/<task-id>/`.
- `.apw/transactions/`: prepared journals and committed apply receipts.
- `<worktree>/.apw-sandbox/`: sandboxed tool caches and temp files (only with `sandbox.mode` restricted or docker); `<worktree>/.apw/browser/`: browser-check screenshots.
- `.apw/daemon.json`: local endpoint and authentication token; `daemon.log`: startup/runtime output.

Status performs an authenticated health request and verifies process, workspace and protocol. It never invents a daemon process. `apw daemon start` starts it; `apw daemon stop` requests authenticated shutdown. One service owns the workspace. Jobs interrupted by shutdown/restart remain `interrupted` and require explicit resume; the daemon does not silently repeat model calls. Cancellation terminates process descendants using Windows jobs or Unix process groups. Persistence failure stops the service.

The loopback API uses Bearer authentication, loopback Host checks, same-origin browser checks and bounded strict JSON bodies. Desktop has its own per-process token and forwards task calls to the daemon. Task submission uses a request ID: retrying the same owner/request/body returns the original task. Local clients cannot claim Telegram ownership.

Model/trust changes in the daemon's configuration file are reloaded before new work. Environment-variable credentials belong to the daemon process; restart it after changing those variables. A workspace-root change requires restarting the daemon. Configuration paths must refer to the same workspace; use one configuration per running daemon.

Legacy checkpoints without a matching snapshot manifest are preserved but cannot safely apply/resume through the new pipeline. Inspect their worktree and review/export their diff manually before creating a new task. Do not delete a worktree containing needed changes.

## Worktree location

Task worktrees live outside the workspace:

```text
<base>/<workspace-key>/<short-id>/
```

- `<base>` is `worktree_root` in `apw.json` (an absolute path outside the workspace), else the `APW_WORKTREE_ROOT` environment variable, else the user cache directory: `%LOCALAPPDATA%\apw\wt` on Windows, `$XDG_CACHE_HOME/apw/wt` (or `~/.cache/apw/wt`) on Linux, `~/Library/Caches/apw/wt` on macOS.
- `<workspace-key>` is the first 8 hex digits of SHA-256 over the workspace's resolved absolute path (lower-cased on Windows), so several workspaces share one base.
- `<short-id>` is the first 12 hex digits of a generated `task-<32 hex>` ID (short IDs are kept, other long IDs are hashed). The Git branch keeps the full name, `apw/<task-id>`.

Why: tools run from the project (Vitest, Jest, `eslint .`) used to discover APW's retained copies under `.worktrees/`, including broken tests of failed tasks, and the extra ~50 characters pushed pnpm's nested `node_modules` past the 260-character Windows limit. The external path no longer grows with the project path. With a long profile path, set a short `worktree_root` such as `D:\apw-wt`.

Checkpoints, reports and manifests store the worktree's absolute path. Resume, apply, recovery export and cleanup accept a worktree only when it is a direct child of one of the workspace's worktree directories: the configured root, `APW_WORKTREE_ROOT`, the default root, or the legacy `<workspace>/.worktrees`. Resume, apply and export additionally require a Git worktree root on the checkpoint's branch that shares the workspace's `git-common-dir`, so a directory from another repository placed in the root is refused. Changing `worktree_root` while tasks are open strands their worktrees until it is set back (the default root is always searched).

Security:

- The trusted parent (the user cache directory, or the parent of `worktree_root`) is used as given. Every component APW creates below it (`apw`, `wt`, the configured root's last component, `<workspace-key>`, and the task directory) must be a plain directory: links and Windows junctions are refused on create, locate, list and remove.
- APW creates those directories with user-only permissions: on Windows a protected DACL that grants full control to the current user and SYSTEM only (worktrees inherit it); elsewhere mode `0700`. An existing directory owned by another account (on Windows anyone other than the user, Administrators or SYSTEM; elsewhere another uid, or group/world-writable) is refused, so a directory planted under a shared parent such as `C:\` cannot capture worktrees.
- `worktree_root` must be absolute, must not be a volume root, and must not be inside the workspace.

Legacy compatibility: tasks created before this layout keep working from `<workspace>/.worktrees/<task-id>/`. They resume, apply, export and clean up there; APW never moves or deletes legacy work automatically, and new tasks never create `.worktrees/`. Remove an empty `.worktrees/` by hand once its tasks are applied or cleaned up.

## Verification sandbox

Verification runs repository code: dependency install scripts, linters, builds and tests. The worktree keeps edits away from the source workspace, but by default that code runs as the current user with the full environment. `sandbox` in `apw.json` adds isolation:

```json
"sandbox": {
  "mode": "restricted",
  "network": "install",
  "memory_mb": 4096,
  "pass_env": ["NPM_TOKEN"]
}
```

| Mode | Isolation |
| --- | --- |
| `off` (default) | Previous behavior: commands run as the user with the full environment. |
| `restricted` (Windows) | Each command runs with a low-integrity copy of the user's token in a job object with UI restrictions (no desktop switching, clipboard, global atoms or foreign USER handles) and the optional memory cap. The worktree is labeled low integrity; its `.git` link and `.apw` manifest stay medium. Project code can write only the worktree: not the source workspace, the main repository's `.git` (so it cannot plant hooks), the snapshot manifest, the user's profile or other projects. |
| `restricted` (Linux) | bubblewrap (`bwrap`): read-only root file system, writable worktree, private `/tmp`, new namespaces, no network except where allowed. |
| `docker` | Each stage runs in a fresh container from `sandbox.image` that mounts only the worktree at `/workspace`, drops all capabilities, sets `no-new-privileges` and a pids limit, runs as the user's uid on Linux, and is removed on cancel. The image must contain the project toolchain (for example `node:22` with corepack, `golang:1.27`). |

`network`: `install` (default) gives network only to the dependency-install stage, `none` never, `all` always. Restricted mode on Windows cannot block network access; use `docker` when network isolation matters.

In `restricted` and `docker` modes the command environment is scrubbed: variables whose names contain KEY, TOKEN, SECRET, PASSWORD, PASSWD, CREDENTIAL, AUTH, COOKIE, SESSION or PRIVATE, and all `APW_*` variables, are removed unless listed in `pass_env`. Docker passes no host environment at all except `pass_env`. Tool caches and temp files (`TEMP`, `GOCACHE`, npm, pnpm store, yarn, bun, pip, `CARGO_HOME`, XDG directories and, on Windows, `LOCALAPPDATA`) move into `<worktree>/.apw-sandbox`, which is Git-excluded, never counts as a task change and is removed with the worktree.

Costs and limits:

- Caches start cold for every task: the first Go verification rebuilds the standard library (about 10-20 seconds), and pnpm/npm download packages into the task's store. Rust under restricted mode gets a fresh `CARGO_HOME`; with Linux `network: install` a `cargo build` that needs to download crates fails unless dependencies are vendored or `network` is `all`.
- pnpm 11 and later keep store locks in `%LOCALAPPDATA%\pnpm-store-operation-locks` regardless of environment variables. Restricted mode labels that one directory, which holds only lock files, low integrity so sandboxed pnpm can lock its own store.
- Low integrity limits writes, not reads: sandboxed code can still read files the user can read. Together with Windows network access this means a hostile test could read and send user data; the scrubbed environment removes the most common secrets from its reach. Use `docker` for untrusted repositories.
- A sandbox that cannot be prepared (no `bwrap`, no Docker engine, labeling failure) fails verification with stage `sandbox` before any model repair. Reports name the sandbox that was used.
- None of these modes is a virtual-machine boundary; kernel or container escapes are out of scope.

## Browser check

For client-side web apps (Vite, React, Vue, Svelte, Preact or Solid with a `build` script; not Next.js) and static HTML sites, the pipeline gains a `browser` stage when Microsoft Edge, Google Chrome or Chromium is installed (`APW_BROWSER` can name the executable). After the build, APW serves the build output (`dist/`, `build/` for react-scripts, the site root for static HTML) from its own loopback-only static server and renders `index.html` headlessly:

- It fails on uncaught page errors and on a page with no visible content (the classic blank page after a runtime crash). These failures go to the repair loop like compiler errors.
- Unhandled promise rejections (for example an API that is offline during verification) and console messages are reported but do not fail.
- It is skipped when the build produced no `index.html` (library builds).
- A screenshot is kept at `<worktree>/.apw/browser/index.png`.
- APW's server is read-only (GET/HEAD), hides `.git`, `.apw` and `.apw-sandbox`, and falls back to `index.html` for client-side routes. No project server code runs; page scripts run inside the browser's own sandbox. With an active `sandbox` the page cannot resolve any host other than APW's server.

Plans can add acceptance checks of kind `browser`: `file` is the page path (default `/`) and `value` is text the rendered page must visibly show, for example `{"criterion":"Counter shows 10","kind":"browser","file":"/","value":"count is 10"}`. The check runs only after verification passes and fails if the page also throws an uncaught error. Disable the stage with `"verify": {"browser": "off"}`.

## Documentation research

With `research` enabled the Architect may read documentation before it plans:

```json
"research": {
  "enabled": true,
  "hosts": ["docs.example.com"],
  "search": "brave",
  "search_key_env": "BRAVE_API_KEY",
  "max_sources": 4
}
```

The Architect first answers with the pages it needs (`{"fetch": [...], "search": [...]}`); APW fetches them itself and adds the text to the planning prompt, marked as untrusted reference material whose instructions must not be followed. Rules:

- HTTPS GET only, to hosts on the allowlist: built-in documentation sites (pkg.go.dev, go.dev, docs.rs, developer.mozilla.org, nodejs.org, registry.npmjs.org, react.dev, vite.dev, vitest.dev, typescriptlang.org, docs.python.org, readthedocs.io, learn.microsoft.com and others) plus `research.hosts` (bare host names; subdomains are included). github.com is not on the default list because repository owners can see which paths were fetched.
- No query strings (they could carry workspace data), credentials, cookies, proxies or custom ports; IP-literal hosts are refused.
- Connections to private, loopback, link-local, CGNAT and other non-public addresses are refused when connecting, so DNS rebinding cannot reach the local network. At most three redirects, each checked by the same rules.
- Text responses only, at most 1 MiB downloaded, 12 KiB of text per source and 40 KiB in total. npm and crate registry documents contribute their README.
- `search: "brave"` sends the Architect's short queries to the Brave Search API with the key from `search_key_env` and adds result titles, URLs and snippets. Result pages are not fetched automatically.
- Research is best effort: a malformed request or failed fetch only removes that context. Progress events and the report list every source and why it was not used.

Residual risk: a URL path on an allowed host still reaches that host, so a model following hostile instructions in the repository could encode data in a path. Research is off by default; enable it for repositories you trust and keep `hosts` short.

## Project instructions and planning context

- **Project instructions:** the first of `APW.md` or `AGENTS.md` in the workspace root (read like any source file: links refused, at most 12 KiB) is added to planning, review and patch prompts. Use it for conventions such as language, style, folder layout or test commands to prefer. It comes from the repository, so the prompt states that it never overrides APW's rules, the response format or the user's task; APW's checks enforce those rules regardless of any prompt.
- **Code lookup:** when the code context does not fit the prompt (96 KiB; files that did not fit are listed by name), the Architect may first ask for read-only lookups in the same pre-planning request as documentation research: `grep` (RE2 pattern, optional glob), `glob` (for example `src/**/*.test.ts`) and `read` (a file and line range, at most 300 lines). At most six lookups run, each bounded (40 matches, 200 files, 240 characters per line) and 32 KiB in total. Lookups read the source workspace only: no repository code runs, `.git`, `.apw`, `.env*`, key files, dependency and build directories stay invisible, and links are not followed. `"planning": {"lookup": "always"}` offers lookup on every task and `"off"` disables it (default `"auto"`).
- **Session memory:** conversation sessions stay bounded however long they run. The last three turns are shown in detail, older turns as one-line digests (outcome, request, files) within 3 KiB, turns that leave the 50-turn history survive as digests (at most 30), and only the newest 40 changed files are listed. Compaction is deterministic and never calls a model.

## Plan proposals

```json
"planning": { "proposals": 3, "judge": true }
```

With `proposals` greater than 1 and at least two architect candidates (`roles.architect` plus `failover.architect`), up to that many candidates plan in parallel (candidates in a quota cooldown are skipped). Every valid plan gets a rule score from 0 to 100 that predicts whether APW can execute and prove it: each acceptance criterion needs an executable check, verification checks must name real pipeline stages, browser checks need a browser stage, the plan should state verification and an `only_changes` scope, edits should stay within `affected_files`, and smaller plans are preferred. With `judge: true` the primary architect also rates the plans from 0 to 10 and the combined score is the average of both. The highest score runs. `plan.ranking` records every proposal (model, score, reasons, judge rating or error); `apw plan` prints it and task reports list it. Failed candidates are cooled down exactly as in sequential failover. With a single architect candidate, planning is unchanged.

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
- Unknown entries in this workspace's worktree directory or in a legacy `.worktrees/` (listed as orphans, never removed). Cleanup never removes anything outside those directories.
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
