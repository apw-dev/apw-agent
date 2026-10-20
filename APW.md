# Project instructions (`apw-agent`)

APW reads this file before it plans, reviews, or patches changes.

## Project overview

- `apw-agent` is a Go 1.27+ AI Developer OS built around the Triad architecture:
  - `cmd/apw` (CLI & interactive shell), `cmd/apwd` (24/7 background daemon), `cmd/apw-desktop` (native Windows Wails v2 shell `APW.exe`).
  - `internal/architect` (Spec-First planner), `internal/surgeon` (Read-Before-Write snapshot, SHA-256 drift guard, AST & cross-file symbol graph), `internal/reflex` (multi-language verification pipeline & self-healing loop), `internal/executor` (Triad orchestrator & iterative re-planner).
  - `internal/desktop` (backend server) + `internal/desktop/web` (embedded Svelte 5 + TypeScript + Vite frontend).
  - `internal/store` (SQLite task store, episodic memory, 24/7 autonomy schedules & goals).

## Conventions

- Keep Go code `gofmt`-clean and free of `go vet` warnings.
- Preserve rooted path safety (`surgeon/root.go`), worktree isolation, and explicit `apply` approval boundaries.
- Frontend (`internal/desktop/web`) follows `DESIGN.md` (WCAG 2.1 AA contrast tokens, no em dashes in UI copy, keyboard-accessible controls, rebuild `dist/` via `npm --prefix internal/desktop/web run build` before embedding).

## Commands to prefer

- Build: `go build ./cmd/apw ./cmd/apwd` and `npm --prefix internal/desktop/web run build`
- Test: `go test -mod=readonly -race -count=1 -skip 'TestLive|TestBot_LiveOptIn' ./...` and `npm --prefix internal/desktop/web test`
- Lint or format: `gofmt -l .`, `go vet ./...`, and `npm --prefix internal/desktop/web run check`

## Things to avoid

- Never read or expose `.env`, `.apw`, `.git`, or secret keys in tool/file outputs.
- Never bypass worktree isolation or snapshot hash checks during task execution or apply.
