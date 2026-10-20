# APW Desktop as a native Windows app (Wails + Svelte) — design

Date: 2026-10-20 · Status: approved in brainstorming, pending written-spec review

## 1. Intent

**Problem (user's words, summarized):** the Desktop is an Edge `--app` window, not a real app. The window and taskbar entry are Edge's, browser behaviors leak through (right-click Back/Reload/Inspect, `Ctrl+R`, `Ctrl+F`, zoom), the user's Edge extensions run inside it, and the layout looks like a web page that is not pleasant to use.

**Goal:** a real Windows desktop application that looks and feels like Claude Desktop / the Codex app: conversation-centred, with a review panel on the right.

**Decisions made with the user:**

| Topic | Decision |
|---|---|
| Shell | Wails v2 (Go), not Tauri |
| Frontend | Svelte 5 + TypeScript + Vite |
| Platform | Windows first; Linux/macOS keep `apw desktop` (browser app-mode) with the new frontend |
| Distribution | Separate GUI binary `APW.exe` plus an NSIS installer |
| Theme | Follows Windows light/dark; user can override in Settings |
| Title bar | Native frame with themed caption colour and Mica backdrop (keeps Snap Layouts) |
| Layout | Sidebar + conversation thread + right review panel (layout "A") |
| Right panel | Collapsible stacked cards (adapted from the Claude Code side panel) |
| Fonts | Noto Sans + Noto Sans Thai, bundled; code uses Cascadia Code → Consolas |
| Diff | Syntax highlighting in the first release |

**Assumptions (confirmed):**
- Feature scope is today's Desktop feature set. The only new capability is a workspace picker, which a double-clicked app requires.
- Backend contract is unchanged. The app is a client of `apwd` through the existing `desktop.Server` HTTP API; the Desktop never runs its own executor.
- Root-module CI (ubuntu `-race`, `go build ./...`, `go mod tidy -diff`, gofmt) keeps passing without Node or Wails installed.

**Success criteria:**
1. Double-clicking `APW.exe` (or the Start Menu shortcut) opens a window with APW's own icon, title and taskbar entry, with no browser menus, shortcuts, zoom or extensions.
2. The existing Desktop completion gate passes in the new app: create a conversation → review → preview → inspect diff → execute/verify → cancel/resume → apply → reopen the same task without losing history.
3. No white flash on start; a "connecting…" state is shown while `apwd` starts instead of "api offline".

## 2. Architecture and repository layout

```
cmd/apw-desktop/             separate Go module (own go.mod, replace => ../..)
  main.go                    wails.Run: window options, AssetServer, single instance
  workspaces.go              workspace picker, recent list, per-workspace desktop.Server swap
  windowstate.go             size/position/maximized persistence and on-screen clamping
  build/windows/             icon.ico, app manifest, NSIS installer script (Wails template)
internal/desktop/
  web/                       Svelte 5 + TS + Vite source
  web/dist/                  committed build output, embedded with go:embed
  server.go, backend.go, ... existing HTTP API, contract unchanged
  ui.go                      deleted once the new frontend reaches parity (phase 2)
```

- **Separate module.** `go build ./...` at the root does not descend into nested modules, so ubuntu CI and `go mod tidy -diff` never see Wails dependencies. The nested module may import `github.com/apiwat/apw-agent/internal/...` because its import path lies under the module root that owns `internal/`.
- **Data flow.** WebView2 → Wails AssetServer (`wails://`) → `desktop.Server.Handler()` (existing token guard) → `daemon.Connect` → `apwd`. Everything runs in one process and no localhost port is opened. The Desktop polls, with no SSE or WebSocket, so AssetServer's request/response model is sufficient.
- **Workspace manager.** `APW.exe` holds one `desktop.Server` for the selected workspace's `apw.json` and swaps the AssetServer handler atomically when the user switches. Recent workspaces, window state, panel widths, the last selected conversation/task per workspace, collapsed cards and the theme override are stored in `%APPDATA%\APW\desktop.json`, written atomically. The folder picker is the native Windows dialog (Wails runtime).
- **One frontend, two hosts.** `APW.exe` and `apw desktop` both serve the same embedded `web/dist`. On Windows, `apw desktop` launches `APW.exe` when it is installed next to `apw.exe`, and otherwise falls back to the current Edge `--app` window.
- **Daemon.** Unchanged: `daemon.Connect` starts `apwd` per workspace. The installer must therefore install `apwd.exe` next to `APW.exe`.
- **Token.** `handleIndex` injects the session token into a `<meta>` tag instead of the inline script, so the CSP can drop inline scripts entirely (`script-src 'self'`).

## 3. Window and native behavior

- **Frame:** native title bar with caption colours synced to the active theme, Mica backdrop on Windows 11, solid background on Windows 10. The workspace switcher lives in the sidebar header, not the title bar.
- **Size:** default 1280×820, minimum 960×600. Size, position and maximized state are restored; a window whose monitor is gone is moved onto the primary monitor.
- **Startup without a white flash:** the window starts hidden with a background colour matching the theme and is shown when the frontend signals ready. While `apwd` starts, a "Connecting…" screen replaces the current "api offline" error.
- **Theme:** follows Windows by default; the Settings override is also applied to the caption.
- **Single instance:** launching `APW.exe` again focuses the existing window and forwards an optional workspace path argument.
- **Browser behaviors removed (production builds):**
  - no default context menu; app menus instead (Copy, Copy path, Open in Explorer on messages, files and diffs)
  - zoom and pinch-zoom disabled
  - `Ctrl+R`, `F5`, `Ctrl+F`, `Ctrl+P`, `Ctrl+U`, `F12`, `Ctrl+Shift+I` blocked
  - dropping a file does not navigate the webview
  - external links open in the default browser; navigation away from the app origin is blocked
  - text is selectable only in content (messages, code, diffs); app chrome is not selectable and uses the default cursor
  - DevTools only in `wails dev` or with `--devtools`
- **Shortcuts:** existing `Ctrl+B` (sidebar), `Ctrl+J` (right panel), `Ctrl+K` (focus composer), `Esc`; new `Ctrl+N` (new conversation), `Ctrl+,` (Settings), `Ctrl+O` (open workspace).
- **Close:** closing the window exits the app; `apwd` keeps running, so in-flight tasks continue and are visible on next launch. No tray icon.

## 4. UI

### 4.1 Layout

- **Sidebar (left, resizable, collapsible, translucent over Mica):**
  - header: workspace popover (switch workspace, open folder, trust toggle, create/open `APW.md`)
  - "New conversation" (`Ctrl+N`)
  - "Running" group: tasks not finished or waiting for approval
  - conversations grouped by day
  - footer: Settings and Autonomy (with paused badge)
- **Thread (centre, max width ~760px):**
  - user messages as soft bubbles; assistant output as full-width markdown
  - **task card:** mode, file count, a Plan → Patch → Verify → Apply bar driven only by real task status from the API (no simulated progress), a collapsible log, and actions (Review, Apply, Cancel, Resume)
  - approval cards inline (Approve / Deny)
  - **composer:** rounded multi-line box with a mode chip, a model chip (Brain 1/Brain 2) and `@`/`/` menus backed by `/api/search`; `Enter` sends, `Shift+Enter` adds a newline
- **Right review panel (resizable, closable with `Ctrl+J`):** collapsible stacked cards; collapsed state is persisted.

| Card | Content |
|---|---|
| Conflict (only when present, top) | Workspace conflict / hash drift and the action required |
| Status | Mode, task status, worktree, real-status step bar |
| Changes | Total +/−, file list with per-file counts, a short diff preview of the selected file. Clicking a file opens a full-height diff view inside the panel with a ← back control |
| Verification | build / vet / test / acceptance items; failed items expand to show their log |
| Workspace | Branch, sandbox mode, trust, `apwd` connection state (moved out of the sidebar to avoid duplication) |
| Sticky footer | Discard · **Apply** |

  - With no task selected, the panel shows only the Workspace card.
  - `/symbols` and `/graph` open as full-panel views with ← back.

### 4.2 Mapping from the current Desktop (`internal/desktop/ui.go`)

| Current | New home |
|---|---|
| Workspace and trust controls, `APW.md` card | Sidebar header workspace popover |
| Sessions, recent tasks | Sidebar "Running" group + conversations by day |
| Live progress card, Cancel/Resume/Apply/Inspect Diff | Thread task card |
| Approvals | Inline approval card |
| Affected files, diff, acceptance/reflex evidence, conflict banner | Right panel cards |
| `/symbols`, `/graph` artifacts | Full-panel views in the right panel |
| Composer, slash commands, `@` mentions, mode, Brain pills | Composer |
| Settings dialog | Settings modal: General (theme) · Models · Workspace & Sandbox |
| Autonomy dialog | Autonomy view in the centre: Governor, Goals, Schedules, Episodes, Skills |
| Resizers, `Ctrl+B/J/K`, selection persistence (`localStorage`) | Kept; persisted in `desktop.json` |

### 4.3 Visual system

- **Fonts:**
  - Noto Sans + Noto Sans Thai, variable woff2, subset to Latin + Thai, bundled; no network font loading (the CSP forbids it)
  - code: Cascadia Code → Consolas
- **Type:**
  - base size 13px
  - Thai content uses line-height 1.6
  - no letter-spacing on Thai text
- **Tokens:**
  - light and dark sets meeting WCAG AA contrast
  - one terracotta accent
  - green/red reserved for diffs and status
- **Shape and motion:**
  - radius 6–12px, hairline borders, minimal shadows
  - motion 120–180ms, disabled when Windows "reduce motion" is set
- **Icons:** `lucide-svelte` (tree-shaken SVG); no emoji in UI chrome.
- **Native details:**
  - thin overlay scrollbars
  - focus rings only for keyboard focus (`:focus-visible`)
  - translucent sidebar over Mica
- **Diff syntax highlighting:**
  - Shiki fine-grained bundle with the JavaScript regex engine (no WASM)
  - languages lazy-loaded: go, ts/js, svelte, json, yaml, md, sh, python, html, css, sql; unknown languages render as plain text
  - highlight theme follows light/dark, with +/− backgrounds layered on top
  - highlighting is per file; a file whose diff exceeds 3,000 lines renders without highlighting to keep the UI responsive

### 4.4 Frontend structure

- `src/lib/api.ts`: typed client for the existing HTTP API (token from `<meta>`).
- `src/lib/stores/`: workspace, sessions, tasks (polling and status mapping), settings, search.
- `src/components/`: Sidebar, WorkspacePopover, Thread, Message, TaskCard, ApprovalCard, Composer, MentionMenu, ReviewPanel, cards (Status, Changes, Verification, Workspace, Conflict), DiffView, SettingsModal, AutonomyView.
- Keep each component under ~300 lines so the frontend does not regrow into one file like `ui.go`.

## 5. Error handling

- **`apwd` unreachable or starting:** "Connecting…" screen with retry and the underlying error text.
- **Workspace without `apw.json`:** picker shows the problem and offers to pick another folder. The Desktop does not generate config.
- **API errors:** shown inline at the action that failed (task card, card footer, Settings field), never as silent console errors.
- **HTTP 409 conflicts (skill exists, apply conflict):** shown with the server's message and the next step.
- **`desktop.json` unreadable:** start with defaults and keep a `.bak` copy of the bad file.

## 6. Testing

- **Go, root module:**
  - existing desktop API tests stay unchanged
  - `ui_test.go` is rewritten against embedded `web/dist`: `index.html` present, no remote URLs (fonts included), no inline scripts (CSP), only local API paths, token injected via `<meta>`
- **Frontend:**
  - `svelte-check` for types
  - Vitest + Testing Library for stores (polling, task status mapping, mention search discarding stale responses) and components: TaskCard actions per status, Changes/Verification cards, DiffView (highlighted, the >3,000-line plain fallback, unknown language), Thai text rendering intact
- **E2E (Playwright):** run `apw desktop --no-open` against a Go test helper backend and drive the full completion gate: create conversation → preview → inspect diff → execute/verify → cancel/resume → apply → reload and land on the same task.
- **Shell module:**
  - unit tests for the `desktop.json` store, window-state clamping and single-instance argument parsing
  - native behaviors (blocked keys, context menu, no white flash, Mica, Snap Layouts) go in a manual checklist in `docs/acceptance.md`

## 7. CI and release

- **Existing CI job:** unchanged.
- **New `frontend` job (ubuntu):**
  - `npm ci` → svelte-check → vitest → build → `git diff --exit-code internal/desktop/web/dist` (committed dist matches source) → Playwright
  - Node version pinned via `.nvmrc`; `package-lock.json` committed
- **New `desktop-windows` job:** gofmt, vet, test and build of the `cmd/apw-desktop` module with Wails' build tags (no CGO on Windows).
- **Release workflow (on tag, Windows runner):**
  - build frontend → build `apw.exe`, `apwd.exe`, `APW.exe` → `wails build -nsis`
  - publish `APW-Setup-<version>.exe`, a zip and checksums
  - the installer adds a Start Menu shortcut and an uninstaller; adding to PATH is optional
  - code signing is out of scope for now; unsigned builds trigger a SmartScreen warning (accepted risk)
  - `docs/release.md` updated

## 8. Migration plan (one PR per phase)

1. **Shell spike:** `APW.exe` wraps the current `ui.go` through the AssetServer to prove the Wails options this spec relies on:
   - caption colour + Mica
   - blocking browser accelerators and zoom
   - token guard through AssetServer
   - single instance with argument forwarding
   - hidden-until-ready start

   Any capability that fails is brought back for a design decision before phase 2.
2. **Frontend rewrite:** build `web/` against the parity table in §4.2. When parity is reached, `apw desktop` serves the new dist and `ui.go` is deleted in the same PR (no dual maintenance).
3. **Installer and release:** add the release workflow and the NSIS installer; on Windows, `apw desktop` launches `APW.exe` when present. The Edge `--app` path remains the fallback.

**Data migration:** none needed. Sessions and tasks live in workspace files and SQLite and are untouched. Existing `localStorage` selection was keyed to `127.0.0.1:<random port>`, so it never survived a restart; moving persistence to `desktop.json` fixes that latent bug.

**Docs:** update `docs/roadmap.md` (Desktop section), `README.md` and `docs/acceptance.md`.

## 9. Out of scope

- macOS/Linux native shells
- tray icon
- toast notifications
- command palette
- auto-update
- code signing
- new agent features

## 10. Risks

- **Wails v2 options:** some options (accelerator blocking, caption colours) may behave differently than documented. Phase 1 exists to surface this early.
- **Committed `web/dist`:** can drift from source. The CI freshness check guards it.
- **Bundle size:** fonts and Shiki grammars add size. Mitigated by subsetting and lazy language loading; acceptable for a desktop app.
- **Unsigned installer:** SmartScreen warnings until signing is added.
