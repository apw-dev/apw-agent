# APW — Apiwat Agent

**A Ground-Up Autonomous AI Developer OS & Multi-Agent Coding Runtime in Go**

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go&logoColor=white)](go.mod)
[![Frontend](https://img.shields.io/badge/Desktop-Wails_v2_%2B_Svelte_5-FF3E00?logo=svelte&logoColor=white)](internal/desktop/web)
[![Protocol](https://img.shields.io/badge/Protocol-ACP_%7C_JSON--RPC_2.0-4A90E2)](internal/acp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**APW (`apw`, `apwd`, `APW.exe`)** is a native, zero-bloat AI software engineering platform written from the ground up in Go. It combines a dual-brain reasoning architecture (**System 2: Architect** for spec-first DAG planning and **System 1: Reflex** for fast patching and compiler/test self-healing), deterministic **Precision Code Surgery** inside isolated Git worktrees, a 24/7 background daemon (**`apwd`**), and a native Windows Desktop IDE shell (**`APW.exe`** built with Wails v2 and Svelte 5).

- **Roadmap & Milestones:** [docs/roadmap.md](docs/roadmap.md)
- **Task Lifecycle & Security Model:** [docs/task-lifecycle.md](docs/task-lifecycle.md)
- **Verification & Acceptance Evidence:** [docs/acceptance.md](docs/acceptance.md)
- **Release & Packaging Guide:** [docs/release.md](docs/release.md)

---

## Table of Contents

1. [Core Philosophy](#1-core-philosophy)
2. [System Architecture](#2-system-architecture)
3. [Key Capabilities](#3-key-capabilities)
4. [Repository Structure](#4-repository-structure)
5. [Quick Start & Installation](#5-quick-start--installation)
6. [CLI Reference & Execution Modes](#6-cli-reference--execution-modes)
7. [Supported Languages & Verification Pipelines](#7-supported-languages--verification-pipelines)
8. [Security, Trust & Verification Sandbox](#8-security-trust--verification-sandbox)
9. [Multi-Agent Subagents, Failover & Prompt Caching](#9-multi-agent-subagents-failover--prompt-caching)
10. [Configuration (`apw.json` & `APW.md`)](#10-configuration-apwjson--apwmd)
11. [Evaluation, GitHub Issues & ACP Integration](#11-evaluation-github-issues--acp-integration)
12. [License](#12-license)

---

## 1. Core Philosophy

| Pillar | Engineering Guarantee | Core Packages |
| --- | --- | --- |
| **1. System 1 — Fast Reflex & Verification** | Sub-10ms intent routing, deterministic type-safe schemas, AST symbol inspection, and automated compiler/linter/test repair loops. | `internal/reflex`, `internal/project` |
| **2. System 2 — Spec-First Architecture** | Decomposes complex goals into validated DAG execution plans (`ExecutionPlan` / `plan.json`) with executable acceptance criteria before touching source files. | `internal/architect`, `internal/executor` |
| **3. Precision Code Surgery** | Enforces Read-Before-Write snapshots, SHA-256 base-hash drift detection, and surgical block patching inside isolated Git worktrees—never corrupting dirty working trees. | `internal/surgeon`, `internal/workspace` |
| **4. Multi-Agent Subagents & Self-Evolving Memory** | Dispatches read-only investigation subagents (`delegate`), pre-plan domain auditors, cross-file call-impact graphs, and 3-tier episodic memory that crystallizes verified tasks into reusable `SKILL.md` assets. | `internal/executor`, `internal/store`, `skills/` |
| **5. Headless Core & Multi-Client Surface** | Decouples the 24/7 background daemon (`apwd`) from user surfaces: Interactive CLI (`apw`), Native Windows Desktop (`APW.exe`), Editor Agent Client Protocol (`apw acp`), and Messaging Gateways (`Telegram`). | `cmd/apw`, `cmd/apw-desktop`, `internal/desktop`, `internal/acp`, `internal/channels` |

---

## 2. System Architecture

```text
┌────────────────────────────────────────────────────────────────────────────┐
│                        Surface Layer (Multi-Client)                        │
│   [ $ apw CLI ]    [ APW.exe Native Desktop ]    [ ACP Editors / Chat ]    │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      │ JSON-RPC 2.0 / In-Process / HTTP
┌─────────────────────────────────────▼──────────────────────────────────────┐
│                       APW Core Daemon ($ apwd - 24/7)                      │
│                                                                            │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                    1. Reflex Engine (System 1)                       │  │
│  │  • Fast Intent Router (<10ms)    • AST Symbol & Call-Impact Graph    │  │
│  │  • Direct Q&A & Fast Patching    • Deterministic Type-Safe Schema    │  │
│  └──────────────────┬───────────────────────────────┬───────────────────┘  │
│                     │ (Fast Path)                   │ (Deep Spec Path)     │
│                     │                               ▼                      │
│                     │              ┌────────────────────────────────────┐  │
│                     │              │   2. Architect Engine (System 2)   │  │
│                     │              │  • Multi-Model Plan Proposals      │  │
│                     │              │  • Domain Auditors & Delegates     │  │
│                     │              │  • DAG Decomposition (plan.json)   │  │
│                     │              └────────────────┬───────────────────┘  │
│                     │                               │ (Verified Plan)      │
│                     ▼                               ▼                      │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                3. Surgeon & Verification Sandbox                     │  │
│  │  • Read-Before-Write & SHA-256   • Auto Compiler/Test Gatekeeper     │  │
│  │  • Isolated Git Worktrees        • Self-Healing & Re-Planning Loop   │  │
│  └──────────────────────────────────┬───────────────────────────────────┘  │
│                                     │                                      │
│  ┌──────────────────────────────────▼───────────────────────────────────┐  │
│  │                4. Memory & Self-Evolving Skill Hub                   │  │
│  │  • Cross-File Call Graph         • Procedural Skill Crystallizer     │  │
│  │  • Episodic Memory (SQLite)      • 24/7 Autonomous Safety Governor   │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Key Capabilities

- **Isolated Worktree Execution & Explicit Apply:** Every code modification runs in an external Git worktree (`%LOCALAPPDATA%\apw\wt\<workspace>\<task>` on Windows or `~/.cache/apw/wt/...` on Linux). Your working tree remains untouched until verification passes and you explicitly run `apw apply` (or opt into `--auto`).
- **Native Windows Desktop IDE (`APW.exe`):** Built with Wails v2 and Svelte 5, running completely in-process without opening a localhost network port:
  - **Multi-Project Workspace Tree:** Switch instantaneously across multiple repositories while background tasks stay alive in memory, or open side-by-side windows (`--new-window`).
  - **Agent Board & Model Flow Runner:** Interactive XYFlow visualization of parallel subagent waves, Architect/Reflex failover chains, and real-time model routing.
  - **External AI Prompt Inspector:** Inspect exact outgoing prompts, system instructions, byte sizes, latency, and raw model responses for every planning, subagent, and patching step.
  - **Split/Unified Diff & Evidence Review Panel:** Review Shiki syntax-highlighted diffs, verification logs, acceptance criteria results, and workspace file trees before applying changes.
  - **Bilingual UI (English / Thai):** Instant language switching with WCAG 2.1 AA contrast themes (*Warm Paper* light & *Warm Ink* dark) and bundled `Noto Sans` / `Noto Sans Thai` typography.
- **Cross-File AST Impact & Iterative Re-Planning:** Indexes top-level symbols and cross-file call edges (`internal/surgeon/graph.go`) so compiler/test failures automatically pull upstream callers into the repair context or trigger dynamic architectural re-planning.
- **Multi-Provider Routing & Prompt Caching:** Supports Anthropic API (with 1-hour and 5-minute ephemeral prompt caching and SSE streaming), OpenAI-compatible Local LLMs (Ollama, vLLM, LM Studio), Antigravity (`agy`), Claude CLI, and Codex CLI with per-group quota cooldowns and automatic failover pools.

---

## 4. Repository Structure

```text
apw-agent/
├── cmd/
│   ├── apw/                        # Surface CLI & Interactive Shell (apw > _)
│   ├── apwd/                       # 24/7 Background Core Daemon
│   └── apw-desktop/                # Native Windows Desktop Shell (APW.exe via Wails v2)
├── internal/
│   ├── core/                       # Core Domain Models (Task, Request, PromptTrace, RouteCandidate)
│   ├── config/                     # Atomic Configuration Loader & Validator (apw.json)
│   ├── reflex/                     # [System 1] Verification Pipeline & Self-Healing Loop
│   ├── architect/                  # [System 2] Spec-First Planner, Multi-Proposal Judge & Validator
│   ├── surgeon/                    # [Surgeon] Read-Before-Write, SHA-256 Guard, AST & Call Graph
│   ├── executor/                   # Triad Orchestrator, Subagent Waves, Delegates & Failover Routing
│   ├── daemon/                     # Lifecycle Manager for apwd & Skill Discovery
│   ├── desktop/                    # Desktop Server & Embedded Svelte 5 Frontend (internal/desktop/web/)
│   ├── acp/                        # Agent Client Protocol (ACP) stdio server for IDE/Editor integration
│   ├── eval/                       # Benchmark Runner with Hidden Test Fixtures (apw eval)
│   ├── project/                    # Automatic Language & Build Pipeline Detection (Go/TS/Rust/Python/.NET/HTML)
│   ├── sandbox/                    # Process Isolation (Windows Low-Integrity Job Objects, Bubblewrap, Docker)
│   ├── browser/                    # Headless Chromium/Edge Runner for UI & Runtime Error Verification
│   ├── research/                   # Allowlisted Documentation Fetcher & Web Search before Planning
│   ├── retention/                  # Safe Task Pruning & Worktree Retention Policy (apw cleanup)
│   ├── workspace/                  # Exclusive & Shared Workspace File Leases
│   ├── channels/telegram/          # Telegram Bot Gateway (Long-polling, Allowlist, Task Commands)
│   ├── provider/                   # AI Providers (Anthropic API, Local OpenAI-compatible, CLI, Mock)
│   └── store/                      # SQLite Persistent Store (Tasks, Sessions, Episodic Memory, Autonomy)
└── docs/                           # Architecture Specs, Lifecycle Guides, Acceptance & Release Docs
```

---

## 5. Quick Start & Installation

### Build from Source

Requires **Go 1.27+** (see [`go.mod`](go.mod)):

```bash
git clone https://github.com/apiwat/apw-agent.git
cd apw-agent
go install ./cmd/...
```

Or install using the Windows NSIS installer (`APW-Setup-<version>.exe`) or portable bundle (`APW.exe`, `apw.exe`, `apwd.exe` placed in the same directory). See [docs/release.md](docs/release.md) for full build and packaging instructions.

### 1) Interactive CLI Shell (`apw > _`)

Run `apw` inside any project directory. It automatically starts `apwd` in the background and opens the interactive shell:

```text
  ▄▀█ █▀█ █░█░█
  █▀█ █▀▀ ▀▄▀▄▀   v0.1.0-alpha (Apiwat Agent)
 ───────────────────────────────────────────────────────
  Brain 1 (Reflex) : Reflex Engine [Available]
  Brain 2 (Deep)   : Architect     [Standby]
  Daemon (apwd)    : Running (PID: 14820) | 17 Skills Loaded
 ───────────────────────────────────────────────────────
  Workspace        : D:/APW_Project/apw-agent (Go)

apw > _
```

### 2) Native Windows Desktop (`APW.exe` / `apw desktop`)

```bash
apw desktop
```

On Windows, if `APW.exe` is alongside `apw.exe`, running `apw desktop` launches the native Wails v2 + Svelte 5 desktop application (or launch `APW.exe` directly from the Start Menu / Explorer). If `APW.exe` is not present, it falls back to opening the web UI in an Edge `--app` window.

---

## 6. CLI Reference & Execution Modes

```bash
# Core Task Workflow
apw run "Describe the feature or bugfix"  # Plan, patch, and verify in an isolated worktree (requires trust)
apw run --auto "..."                      # Verify and automatically apply changes to the workspace
apw run --no-verify "..."                 # Skip build/test verification (does not require trust); wait for apply
apw apply <task-id>                       # Apply a verified task's patch to the live workspace
apw preview "Prepare changes"             # Generate a worktree diff without running repository build/test code
apw review "Audit code and report bugs"   # Read-only architectural and code review with file:line citations
apw plan "Design execution plan"          # Produce a structured ExecutionPlan (.apw/plans/<id>.json)

# Task Inspection & Lifecycle
apw tasks | apw task <task-id>            # List all tasks or inspect a full JSON task report
apw diff <task-id>                        # Display the unified source diff for a prepared task
apw watch <task-id>                       # Reattach to a running task's live progress stream
apw resume [task-id]                      # Resume an interrupted task from its exact completed subtask checkpoint
apw cancel <task-id>                      # Cancel a running task and terminate its entire subprocess tree
apw recover <legacy-task-id>              # Safely export a legacy worktree task for manual inspection

# Workspace Trust, Verification & Maintenance
apw trust                                 # Authorize running build/test pipelines for the current repository
apw verify .                              # Run the detected Reflex verification pipeline on the workspace
apw cleanup [--dry-run]                   # Prune old tasks per retention policy (stop daemon first for live prune)

# Models, Auth, Skills & Daemon
apw model [status|refresh|set ...]        # Inspect or switch active Architect / Reflex AI models
apw auth                                  # Discover and manage connected providers (AGY, Claude CLI, Codex, API, Local)
apw skills                                # List active procedural skills loaded in the workspace
apw symbols internal/surgeon/surgeon.go   # Extract AST symbols from a source file
apw daemon [status|start|stop]            # Manage the background apwd service
apw version                               # Print version, commit, and platform metadata
```

### Interactive Modes (`/mode` or `Shift+Tab`)

Set the default mode in `apw.json` via `"approval"`, or switch live in the CLI/Desktop UI:
- **`ask` (default):** Plans, edits, and verifies in an isolated worktree, then prompts `Apply ... [y/N]` before touching your workspace.
- **`auto`:** Automatically applies changes to the workspace as soon as verification passes.
- **`plan`:** Generates an execution plan only; modifies no files and executes no commands.
- **`unverified`:** Modifies files in a worktree and performs file-level checks without running project install/build/test commands (does not require `apw trust`). Marks the report clearly as `UNVERIFIED`.

---

## 7. Supported Languages & Verification Pipelines

APW automatically detects your project stack from root manifest files and runs the appropriate verification pipeline inside the isolated worktree:

| Project Type (Detection Files) | Automatic Verification Pipeline |
| --- | --- |
| **Go** (`go.mod`) | `gofmt` → `go vet` → `go build` → `go test` |
| **Node.js / TypeScript** (`package.json`) | `install` → `format:check` → `lint` → `typecheck` → `build` → `test` (runs existing scripts) |
| **Rust** (`Cargo.toml`) | `cargo fmt --check` → `cargo build` → `cargo test` |
| **Python** (`pyproject.toml`, `requirements.txt`) | `ruff check` (if installed) → `pytest` (if tests exist) |
| **.NET / C# / F#** (`*.sln`, `*.csproj`, `*.fsproj`) | `dotnet test` (supports root and nested solution/project layouts) |
| **Static HTML/CSS** (`index.html`) | Validates that all local `src` and `href` asset references exist |

### Custom Verification Pipelines

For custom languages, monorepos, or specialized toolchains, define `verify.stages` in `apw.json`. Each stage runs directly as an argument vector (no shell interpolation):

```json
{
  "verify": {
    "stages": [
      { "name": "lint", "run": ["pnpm", "lint"] },
      { "name": "typecheck", "run": ["pnpm", "exec", "tsc", "--noEmit"] },
      { "name": "test", "run": ["pnpm", "test"], "env": { "CI": "true" }, "timeout_sec": 600 }
    ]
  }
}
```

### Headless Browser Verification

For client-side web apps (Vite, React, Vue, Svelte, Preact, Solid with a `build` script) and static HTML projects, APW automatically runs a headless **Chromium / Edge / Chrome** check after `build`:
- Serves the built bundle on `127.0.0.1` and loads the page in a headless browser.
- Fails verification if uncaught runtime exceptions occur or if the rendered root is blank (white-screen crash), feeding the stack trace back into the Reflex repair loop.
- Captures a screenshot at `<worktree>/.apw/browser/index.png` and supports `browser` acceptance checks (e.g., `{"criterion": "Counter displays 10", "kind": "browser", "file": "/", "value": "count is 10"}`).
- Disable anytime via `"verify": { "browser": "off" }`.

---

## 8. Security, Trust & Verification Sandbox

- **Explicit Repository Trust (`apw trust`):** Newly opened folders start **untrusted** by default. Read-only and non-executing commands (`plan`, `review`, `preview`, and `unverified` mode) work immediately, while `run` and `verify` refuse to execute repository build/test scripts until you explicitly trust the workspace.
- **Process Sandboxing (`sandbox` in `apw.json`):** When working with untrusted code, enable OS-level or container isolation for verification commands:

```json
{
  "sandbox": {
    "mode": "restricted",
    "network": "install",
    "memory_mb": 4096,
    "pass_env": ["NPM_TOKEN"]
  }
}
```

- **`restricted` (Windows):** Executes inside a Win32 Job Object with a Low-Integrity access token, UI restrictions, and memory limits. Write access is restricted strictly to the isolated worktree—preventing writes to the primary repository, `.git` hooks, task manifests, or user profile directories.
- **`restricted` (Linux):** Uses `bubblewrap` (`bwrap`) with a read-only root filesystem, writable worktree, isolated `/tmp`, and network disabled except during `install` stages.
- **`docker`:** Runs every verification stage inside a fresh container (`sandbox.image`) with all Linux capabilities dropped and only the worktree mounted.
- **Secret Scrubbing:** Both `restricted` and `docker` modes strip environment variables matching sensitive patterns (`*KEY*`, `*TOKEN*`, `*SECRET*`, `*PASSWORD*`, `APW_*`) unless explicitly allowlisted in `pass_env`.

---

## 9. Multi-Agent Subagents, Failover & Prompt Caching

### Pre-Plan Lookup, Delegation & Domain Auditors

- **Codebase Lookup (`internal/executor/lookup.go`):** When a repository exceeds the inline context budget, the Architect performs read-only `grep`, `glob`, and line-range `read` queries before generating a plan (excluding `.git`, `.apw`, `.env`, and private keys).
- **Read-Only Delegate Subagents (`internal/executor/delegate.go`):** Both planning and patch/repair steps can spawn read-only subagents to investigate targeted questions across the codebase and return cited `file:line` evidence.
- **Domain Auditors & Parallel Waves (`internal/executor/subagent.go`):** Optional model-backed domain auditors (Backend, Frontend, Security/Test) review relevant files before planning, while disjoint file patches execute in parallel waves.
- **Multi-Model Plan Proposals (`planning.proposals` & `planning.judge`):** Request multiple Architect models to propose plans concurrently. APW scores each plan deterministically (executable checks, valid stage references, minimal file blast radius) and optionally combines with an Architect judge score to select the highest-ranked plan.

### Failover Routing & Quota Cooldowns

Configure primary roles and ordered fallback chains in `apw.json`:

```json
{
  "roles": {
    "architect": "agy:claude-sonnet-5-5-high",
    "reflex": "agy:gemini-3.8-flash-high"
  },
  "failover": {
    "reflex": ["claude-sub:claude-haiku-5-5"],
    "architect": ["agy:claude-opus-5-5-high"],
    "directive_timeout_sec": 90
  }
}
```

- **Smart Cooldowns:** Quota exhaustion errors pause only the affected provider quota group for the duration reported by `Resets in ...` (or 5 minutes by default), while timeouts cool down only the specific model for 5 minutes.
- **Sticky Winner:** Once a fallback model succeeds during a task, subsequent steps in that same task prioritize the working model automatically.

---

## 10. Configuration (`apw.json` & `APW.md`)

- **Project Instructions (`APW.md` or `AGENTS.md`):** Place an `APW.md` (or `AGENTS.md`) file at the repository root to specify coding conventions, architectural invariants, or preferred UI languages. APW injects up to 12 KiB of these instructions into planning, review, and patching prompts without allowing them to override core safety boundaries.
- **Pre-Plan Documentation Research (`research`):** Enable `"research": { "enabled": true }` in `apw.json` so the Architect can fetch official documentation over HTTPS from allowlisted domains (`pkg.go.dev`, `docs.rs`, `developer.mozilla.org`, `react.dev`, `vite.dev`, `registry.npmjs.org`, plus custom `research.hosts`) or via Brave Search (`BRAVE_API_KEY`).

---

## 11. Evaluation, GitHub Issues & ACP Integration

```bash
# Run the benchmark suite with hidden post-apply test assertions
apw eval
apw eval --baseline .apw/eval/<previous-run>.json

# Turn a GitHub issue (#42) into a verified branch and open a draft PR via `gh`
apw issue 42

# Launch the Agent Client Protocol (ACP) server over stdio for compatible IDEs/editors
apw acp
```

- **`apw eval`:** Executes end-to-end tasks against disposable fixture repositories, applies the resulting patch, and runs hidden test suites that the agent never saw during execution to measure both solve rate and false-positive rate (see [docs/eval.md](docs/eval.md)).
- **`apw issue <number>`:** Fetches the GitHub issue via `gh`, plans and verifies the fix in an isolated worktree, asks for confirmation, pushes a clean `apw/issue-<n>-...` branch, and opens a draft Pull Request without touching your local working tree.
- **`apw acp`:** Exposes APW modes (`ask`, `auto`, `plan`, `unverified`), live tool-call progress, and apply approval prompts over the standardized Agent Client Protocol (`stdio`).

---

## 12. License

Released under the [MIT License](LICENSE).
