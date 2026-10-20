# APW Desktop Phase 1 — Native Shell Spike Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `APW.exe`, a Wails v2 window that hosts the current Desktop UI (`internal/desktop/ui.go`) in-process. It proves every native-window capability the spec relies on before the frontend rewrite (phase 2) starts.

**Architecture:**
- A new nested Go module `cmd/apw-desktop` runs `wails.Run`.
- Its AssetServer `Handler` delegates to the selected workspace's existing `desktop.Server.Handler()`, so no TCP port is opened.
- `desktop.Server` learns one new thing: trust requests addressed to the in-process host (`wails.localhost`) when, and only when, `InProcessHost` is set.
- Workspace choice and window state persist in `%APPDATA%\APW\desktop.json`.

**Tech Stack:** Go 1.27, Wails v2 (latest v2 release at execution time, pinned in `cmd/apw-desktop/go.mod`), WebView2 (Windows).

**Spec:** `docs/superpowers/specs/2026-10-20-apw-desktop-wails-design.md` (§2, §3, §5, §6 shell module, §7 desktop-windows job, §8 phase 1). Phases 2 and 3 get their own plans after this spike's results are recorded.

## Global Constraints

- Root module stays Wails-free: `go build ./...`, `go vet ./...`, `go mod tidy -diff` and `gofmt -l .` at the repo root pass on ubuntu without Node/Wails.
- No TCP listener in `APW.exe`. All UI traffic goes through the Wails AssetServer `Handler`.
- HTTP contract of `desktop.Server` unchanged. The token guard still applies to every `/api` and `/v1` request in-process.
- `apw desktop` (TCP server) behavior unchanged. `InProcessHost` is empty there, so `wails.localhost` requests are rejected.
- Window: default 1280×820, minimum 960×600; native frame, Mica on Windows 11; theme follows Windows unless `desktop.json` `theme` is `light`/`dark`.
- Single instance: a second launch focuses the first window and forwards an optional workspace path.
- Production build: no default context menu, zoom/pinch-zoom off, `Ctrl+R`, `F5`, `Ctrl+F`, `Ctrl+P`, `Ctrl+U`, `F12`, `Ctrl+Shift+I` blocked; DevTools only with `--devtools`.
- Settings file: `%APPDATA%\APW\desktop.json` (`os.UserConfigDir()` + `APW`), written atomically; unreadable file → defaults + `.bak` copy.
- Code style: match existing repo (gofmt, small files, table tests, `t.TempDir()`).

## Review Focus

1. **DNS-rebinding style request to the `apw desktop` TCP server with `Host: wails.localhost`:** must get 403 because `InProcessHost` is unset. Test in Task 1.
2. **Startup with a last workspace that was deleted or lost its `apw.json`:** must show the chooser page, not crash or show an API error. Test in Task 5.
3. **Saved window position on a monitor that is no longer attached, or a saved size larger than the current screen:** the window opens fully visible and centred on the monitor it lands on. Test in Task 4. Wails v2 positions are monitor-relative, so restoring onto a specific secondary monitor is not guaranteed; record it in the spike report.
4. **Corrupt or hand-edited `desktop.json`:** the app starts with defaults and keeps the bad file as `desktop.json.bak`. Test in Task 3.
5. **Second launch with a folder argument while the first instance is open:** the first window switches to that workspace and comes to front. Arg-parsing test in Task 4, behavior in the Task 7 manual checklist.

---

### Task 1: In-process host trust for `desktop.Server`

**Files:**
- Modify: `internal/transport/http.go:45` (`Guard`)
- Modify: `internal/desktop/server.go` (`Server` struct, `Handler`, `handleIndex`, `desktopCSP`)
- Test: `internal/transport/http_test.go`, `internal/desktop/server_test.go`

**Interfaces:**
- Produces: `transport.GuardWith(token string, allow func(*http.Request) bool, next http.Handler) http.Handler`. `Guard(token, next)` becomes `GuardWith(token, LocalRequest, next)`.
- Produces: `desktop.Server.InProcessHost string`. When non-empty, a request whose `r.Host` (port stripped) equals it, and whose `Origin` is empty or `http://<InProcessHost>`, is treated as local.
- Produces: `const desktop.WailsHost = "wails.localhost"` (Task 6 verifies the real host and corrects this constant if Wails differs).

- [ ] **Step 1: Write failing tests** in `internal/desktop/server_test.go`:

```go
func TestInProcessHostTrustedOnlyWhenConfigured(t *testing.T) {
	srv := NewServer(filepath.Join(t.TempDir(), "apw.json"), "test")
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "http://wails.localhost/", nil)
		r.Host = "wails.localhost"
		return r
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unset InProcessHost: got %d, want 403", rec.Code)
	}
	srv.InProcessHost = WailsHost
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req())
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("in-process index: %d csp=%q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
	api := req()
	api.URL.Path = "/api/status"
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, api)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("in-process API without token: got %d, want 401", rec.Code)
	}
}
```

  Also assert that the loopback index CSP (existing `TestIndexServesStrictCSP`) still has no `'self'` in `script-src`.

- [ ] **Step 2:** Run `go test ./internal/desktop/ -run InProcessHost`. Expected: FAIL (`InProcessHost`/`WailsHost` undefined).
- [ ] **Step 3: Implement.**
  - Add `GuardWith`.
  - Add `func (s *Server) allowRequest(r *http.Request) bool`: `LocalRequest(r)` or in-process match. Use it for both the `/` check and `GuardWith`.
  - `handleIndex` sets the CSP to `desktopCSP` with `script-src 'self' 'unsafe-inline'` when the request matched in-process, so Wails' injected `/wails/*.js` can load.
- [ ] **Step 4:** Run `go test ./internal/transport/ ./internal/desktop/`. Expected: PASS.
- [ ] **Step 5:** Commit: `feat(desktop): trust Wails in-process host when configured`.

### Task 2: Native-shell behaviors in the current UI script

**Files:**
- Modify: `internal/desktop/ui.go` (script section near the global `keydown` handler at ~line 1099)
- Test: `internal/desktop/ui_test.go`

**Interfaces:**
- Consumes: Wails JS globals when present: `window.runtime.BrowserOpenURL(url)`, `window.go.main.App.ChooseWorkspace()` (bound in Task 6).
- Produces: a function `function inDesktopShell() { return !!(window.runtime && window.go); }`. All behaviors below run only when it returns true, so the browser `apw desktop` keeps standard browser keys.

- [ ] **Step 1: Write failing test** `TestDesktopScriptShellGuards` in `ui_test.go`. It asserts `uiScript(t)` contains `inDesktopShell()`, `"F5"`, `"F12"`, `BrowserOpenURL(` and `ChooseWorkspace(`, and that `TestDesktopScriptSyntax` still passes (node `--check`).
- [ ] **Step 2:** Run `go test ./internal/desktop/ -run ShellGuards`. Expected: FAIL.
- [ ] **Step 3: Implement**, in a capture-phase `keydown` listener and a capture-phase `click` listener:
  - `preventDefault` for `F5`, `F12`, and `Ctrl` + `r`/`f`/`p`/`u`/`=`/`-`/`0`, and `Ctrl+Shift+I`
  - `Ctrl+O` → `window.go.main.App.ChooseWorkspace()`
  - clicks on `a[href^="http:"], a[href^="https:"]` → `preventDefault` + `window.runtime.BrowserOpenURL(href)`
  - `dragover`/`drop` on `document` → `preventDefault`
- [ ] **Step 4:** Run `go test ./internal/desktop/`. Expected: PASS (including the node syntax check).
- [ ] **Step 5:** Commit: `feat(desktop): block browser shortcuts and route links inside the native shell`.

### Task 3: `cmd/apw-desktop` module and `desktop.json` store

**Files:**
- Create: `cmd/apw-desktop/go.mod` (module `github.com/apiwat/apw-agent/cmd/apw-desktop`, `go 1.27.0`, `require github.com/apiwat/apw-agent v0.0.0-00010101000000-000000000000`, `replace github.com/apiwat/apw-agent => ../..`, `require github.com/wailsapp/wails/v2 <latest v2>`)
- Create: `cmd/apw-desktop/settings.go`, `cmd/apw-desktop/settings_test.go`

**Interfaces:**
- Produces:

```go
type WindowState struct {
	X, Y, Width, Height int
	Maximised           bool
}
type Settings struct {
	Version       int         `json:"version"`        // 1
	Recent        []string    `json:"recent"`         // absolute workspace dirs, most recent first, max 10
	LastWorkspace string      `json:"last_workspace"`
	Theme         string      `json:"theme"`          // "system" | "light" | "dark"
	Window        WindowState `json:"window"`
}
func DefaultSettings() Settings                       // Version 1, Theme "system", Window{Width:1280, Height:820}
func SettingsPath() (string, error)                    // filepath.Join(os.UserConfigDir(), "APW", "desktop.json")
func LoadSettings(path string) (Settings, error)       // missing → defaults, nil; corrupt → rename to path+".bak", defaults, nil
func SaveSettings(path string, s Settings) error       // MkdirAll(dir, 0o700); temp file in same dir + os.Rename
func (s *Settings) RememberWorkspace(dir string)       // move/insert to front of Recent, dedupe case-insensitively on Windows, cap 10, set LastWorkspace
```

- [ ] **Step 1: Write failing tests:**
  - `TestLoadSettingsMissingReturnsDefaults`
  - `TestSaveLoadRoundTrip`
  - `TestLoadSettingsCorruptKeepsBackup`: write `{"version":` → `LoadSettings` returns `DefaultSettings()`, `nil` error, and `desktop.json.bak` contains the original bytes
  - `TestRememberWorkspaceDedupesAndCaps`: 12 dirs plus re-adding the 3rd → len 10, front is the 3rd, no duplicates; `C:\A` and `c:\a` count as one on Windows
  - `TestLoadSettingsInvalidThemeFallsBack`: `"theme":"neon"` → `"system"`
- [ ] **Step 2:** `cd cmd/apw-desktop && go test ./...`. Expected: FAIL (undefined symbols).
- [ ] **Step 3:** Implement `settings.go`. Run `go mod tidy` inside `cmd/apw-desktop`.
- [ ] **Step 4:** `cd cmd/apw-desktop && go test ./...` → PASS. Also confirm the root module is untouched: `go mod tidy -diff` at the repo root → no output.
- [ ] **Step 5:** Commit: `feat(apw-desktop): add desktop.json settings store`.

### Task 4: Window-state clamping and launch-argument parsing

**Files:**
- Create: `cmd/apw-desktop/windowstate.go`, `cmd/apw-desktop/args.go`, plus tests for each

**Interfaces:**
- Produces:

```go
type Size struct{ Width, Height int }
// ClampWindow takes a window state whose X/Y are relative to its monitor (as Wails v2
// WindowGetPosition/WindowSetPosition report them) and the current monitor size from
// runtime.ScreenGetAll (IsCurrent). It enforces at least 960×600 and at most the screen,
// and centres the window when its top-left 100×40 title area would fall outside the screen.
func ClampWindow(ws WindowState, screen Size) WindowState
// WorkspaceArg returns the first argument that is not a flag (does not start with "-"), made absolute; "" if none.
func WorkspaceArg(args []string) string
// DevtoolsRequested reports whether args contain "--devtools".
func DevtoolsRequested(args []string) bool
```

- [ ] **Step 1: Write failing table tests:**
  - `TestClampWindowOffscreenCentres`: `{X:3000,Y:100,Width:1280,Height:820}` on `{1920,1080}` → `X=320,Y=130`, size kept (monitor removed since last run)
  - `TestClampWindowNegativeCentres`: `{X:-2000,Y:-50,…}` → centred
  - `TestClampWindowEnforcesMinimum`: `{Width:400,Height:300}` → 960×600
  - `TestClampWindowShrinksToScreen`: 3000×2000 on `{1920,1080}` → 1920×1080 at 0,0
  - `TestClampWindowKeepsVisiblePosition`: `{X:100,Y:80,Width:1280,Height:820}` on `{1920,1080}` → unchanged
  - `TestWorkspaceArg`: `["--devtools","D:\\repo"]` → `D:\repo`; `[]` → `""`
  - `TestDevtoolsRequested`
- [ ] **Step 2:** `cd cmd/apw-desktop && go test ./...` → FAIL.
- [ ] **Step 3:** Implement both files (pure functions, no Wails imports).
- [ ] **Step 4:** `go test ./...` → PASS.
- [ ] **Step 5:** Commit: `feat(apw-desktop): clamp restored window and parse launch args`.

### Task 5: Workspace host (handler swap and chooser page)

**Files:**
- Create: `cmd/apw-desktop/workspace.go`, `cmd/apw-desktop/workspace_test.go`

**Interfaces:**
- Consumes: `desktop.NewServer(configPath, version string) *desktop.Server`, `desktop.Server.InProcessHost`, `desktop.WailsHost`, `buildinfo.Version`, `Settings.RememberWorkspace`.
- Produces:

```go
// WorkspaceHost is the AssetServer Handler. It serves the chooser page until a workspace is open,
// then delegates to that workspace's desktop.Server.Handler(). Safe for concurrent use.
type WorkspaceHost struct{ /* atomic.Pointer[http.Handler], current dir */ }
func NewWorkspaceHost() *WorkspaceHost
func (h *WorkspaceHost) ServeHTTP(w http.ResponseWriter, r *http.Request)
// Open validates dir/apw.json (regular file), builds a desktop.Server with InProcessHost = WailsHost,
// and swaps it in. Returns ErrNoConfig (wrapping the path) when apw.json is missing.
func (h *WorkspaceHost) Open(dir string) error
func (h *WorkspaceHost) Current() string // absolute dir or ""
var ErrNoConfig = errors.New("folder has no apw.json")
```

  The chooser page is a const HTML string with an inline `<script>` and CSP `default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'`:
  - text "เลือก workspace ที่มี apw.json" and a button calling `window.go.main.App.ChooseWorkspace()`
  - a `<p id="err">` that the Go side fills via `?error=` (HTML-escaped)

- [ ] **Step 1: Write failing tests:**
  - `TestWorkspaceHostServesChooserUntilOpened`: GET `/` with Host `wails.localhost` → 200, body contains `ChooseWorkspace`
  - `TestWorkspaceHostOpenRejectsMissingConfig`: `Open(t.TempDir())` → `errors.Is(err, ErrNoConfig)`, `Current()==""`, still serves the chooser
  - `TestWorkspaceHostOpenServesDesktop`: temp dir with a minimal `apw.json` (same JSON as `internal/desktop/server_test.go` uses) → `Open` ok; GET `/` with Host `wails.localhost` → 200 and the body contains `apiToken`; GET `/api/status` without token → 401
  - `TestWorkspaceHostChooserEscapesError`: `/?error=<script>` → body contains `&lt;script&gt;`
- [ ] **Step 2:** `cd cmd/apw-desktop && go test ./...` → FAIL.
- [ ] **Step 3:** Implement `workspace.go`.
- [ ] **Step 4:** `go test -race ./...` (Windows: `go test ./...`) → PASS.
- [ ] **Step 5:** Commit: `feat(apw-desktop): add workspace host with chooser and handler swap`.

### Task 6: Wails app wiring and build

**Files:**
- Create: `cmd/apw-desktop/main.go`, `cmd/apw-desktop/app.go`, `cmd/apw-desktop/wails.json`, `cmd/apw-desktop/build/windows/icon.ico`, `cmd/apw-desktop/build/appicon.png` (1024×1024, terracotta `#c96442` rounded square with white "A"), `cmd/apw-desktop/README.md` (build/run commands)

**Interfaces:**
- Consumes: Tasks 3–5.
- Produces (bound to JS as `window.go.main.App`):

```go
type App struct{ /* ctx, host *WorkspaceHost, settings Settings, settingsPath string */ }
func (a *App) ChooseWorkspace() error        // runtime.OpenDirectoryDialog; Open(dir); on ErrNoConfig reload chooser with ?error=
func (a *App) OpenWorkspace(dir string) error // host.Open + RememberWorkspace + SaveSettings + runtime.WindowReloadApp
func (a *App) SetTheme(mode string) error     // "system"|"light"|"dark" → WindowSetSystemDefaultTheme/LightTheme/DarkTheme + save
```

- [ ] **Step 1: Wire `main.go` with `wails.Run(&options.App{...})`:**
  - Title `APW`; `Width`/`Height` from settings; `MinWidth 960`, `MinHeight 600`; `StartHidden: true`
  - `BackgroundColour`: `#f7f7f5` for light, `#1f1e1c` for dark (resolved from `Theme`)
  - `AssetServer: &assetserver.Options{Handler: host}`
  - `OnStartup`: store ctx, apply theme, open `WorkspaceArg(os.Args[1:])` → else `LastWorkspace` → else stay on chooser
  - `OnDomReady`, first call only: `ClampWindow(settings.Window, current screen from runtime.ScreenGetAll)` → `WindowSetSize` + `WindowSetPosition`, restore `Maximised`, then `runtime.WindowShow`; later calls (after a workspace reload) only ensure the window is shown
  - `OnBeforeClose`: read `WindowGetPosition`/`WindowGetSize`/`WindowIsMaximised` into settings and save
  - `SingleInstanceLock{UniqueId: "8e0c1f9a-apw-desktop", OnSecondInstanceLaunch}`: unminimise, show, focus, and `OpenWorkspace(WorkspaceArg(data.Args))` when non-empty
  - `Windows: &windows.Options{BackdropType: windows.Mica, Theme: windows.SystemDefault, WebviewIsTransparent: false, IsZoomControlEnabled: false, DisablePinchZoom: true}`
  - `DragAndDrop: &options.DragAndDrop{DisableWebViewDrop: true}`
  - `EnableDefaultContextMenu: false`
  - `Debug.OpenInspectorOnStartup` only when `DevtoolsRequested`
  - `Bind: []any{app}`

  If a field name differs in the pinned Wails version, use that version's equivalent and note it in the spike report (Task 7).
- [ ] **Step 2: Compile check.** `cd cmd/apw-desktop && go vet -tags desktop,production ./... && go build -tags desktop,production -ldflags "-H windowsgui" -o ../../bin/APW.exe .`. Expected: exit 0.
- [ ] **Step 3: Packaged build with icon and manifest.** `go install github.com/wailsapp/wails/v2/cmd/wails@<same version as go.mod>` then `cd cmd/apw-desktop && wails build -s -clean`. `wails.json`: `name` `APW`, `outputfilename` `APW`, empty `frontend:install`/`frontend:build`. Expected: `build/bin/APW.exe` with the APW icon. If the CLI insists on a frontend dir, record that in the spike report and continue with the Step 2 binary.
- [ ] **Step 4: Verify the in-process host.** Run with `--devtools` against this repo's `apw.json` and confirm in DevTools that the document URL host equals `desktop.WailsHost`. If it differs, change the constant (Task 1) and its test, then rerun `go test ./internal/desktop/`.
- [ ] **Step 5:** Commit: `feat(apw-desktop): run Desktop UI in a native Wails window`.

### Task 7: CI job, manual acceptance checklist, spike report

**Files:**
- Modify: `.github/workflows/ci.yml` (add job `desktop-windows`)
- Modify: `docs/acceptance.md` (append section "APW.exe native shell (phase 1)")
- Modify: `.gitignore` (add `cmd/apw-desktop/build/bin/`)

**Interfaces:** none (verification only).

- [ ] **Step 1: Add the CI job** `desktop-windows` (`runs-on: windows-latest`, `setup-go` with `go-version-file: cmd/apw-desktop/go.mod`, `working-directory: cmd/apw-desktop`). Steps: `go mod tidy -diff`, `go vet -tags desktop,production ./...`, `go test ./...`, `go build -tags desktop,production -ldflags "-H windowsgui" ./...`.
- [ ] **Step 2: Run root and module checks locally.** Root: `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`. Module: the Step 1 commands. Expected: all pass, no gofmt output.
- [ ] **Step 3: Run the manual checklist** on Windows 11 with the Task 6 binary. Record PASS/FAIL plus a note for each row in `docs/acceptance.md`:

| # | Check | Expected |
|---|---|---|
| 1 | Launch from Explorer | APW icon and title in window and taskbar, not grouped with Edge |
| 2 | Start | No white flash; chooser or last workspace appears |
| 3 | Theme | Caption and Mica follow Windows light/dark; `"theme":"dark"` in desktop.json forces dark |
| 4 | Right-click on a message | No browser menu |
| 5 | `Ctrl+R`, `F5`, `Ctrl+F`, `Ctrl+P`, `Ctrl+U`, `F12`, `Ctrl+Shift+I`, `Ctrl +/-/0`, pinch | Nothing happens |
| 6 | Drop a file on the window | No navigation |
| 7 | External link in a message | Opens in default browser |
| 8 | Hover maximize button | Snap Layouts flyout |
| 9 | Second launch `APW.exe D:\other-repo` | First window focuses and switches workspace |
| 10 | Close and reopen | Same size/position/maximized and same workspace |
| 11 | Workspace folder renamed, reopen | Chooser with error text |
| 12 | Desktop completion gate (spec §1 criterion 2) on current UI | Passes end to end |

- [ ] **Step 4: Write the spike report.** Add a short "Phase 1 results" subsection under §10 Risks in the spec: Wails version pinned, each capability ✔/✘, deviations, and whether phase 2 can proceed as designed. Any ✘ is raised with the user before phase 2 planning.
- [ ] **Step 5:** Commit: `ci(apw-desktop): add windows job; docs: record phase 1 shell acceptance`.
