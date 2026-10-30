//go:build desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ShellState is the native shell snapshot exposed to the frontend.
type ShellState struct {
	CurrentWorkspace string              `json:"current_workspace"`
	Recent           []string            `json:"recent"`
	Projects         []ProjectFolderView `json:"projects,omitempty"`
	Theme            string              `json:"theme"`
	UI               UIPreferences       `json:"ui"`
}

// App owns the window lifecycle and is bound to the page as window.go.main.App.
// Only its exported methods are callable from JavaScript.
//
// Wails calls the lifecycle hooks, the bound methods and the second-instance callback on
// different goroutines, and a second launch can arrive before OnStartup, so all state,
// including the runtime context, is guarded by mu.
type App struct {
	host         *WorkspaceHost
	settingsPath string

	mu       sync.Mutex
	ctx      context.Context // nil until OnStartup
	settings Settings
	shown    bool
	pending  string // workspace a second launch asked for before the window was ready
}

func NewApp(host *WorkspaceHost, settings Settings, settingsPath string) *App {
	return &App{host: host, settings: settings, settingsPath: settingsPath}
}

// context returns the Wails runtime context, or nil before OnStartup. Runtime calls with a
// nil context exit the process, so every caller checks it.
func (a *App) context() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx
}

var errNotReady = errors.New("window is not ready yet")

// takePending returns and clears the workspace a second launch asked for before the window
// was ready.
func (a *App) takePending() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	dir := a.pending
	a.pending = ""
	return dir
}

// ChooseWorkspace asks for a folder and opens it. Cancelling the dialog is not an error.
func (a *App) ChooseWorkspace() error {
	ctx := a.context()
	if ctx == nil {
		return errNotReady
	}
	dir, err := runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            "เลือกโฟลเดอร์โปรเจกต์ (Workspace)",
		DefaultDirectory: a.host.Current(),
	})
	if err != nil || dir == "" {
		return err
	}
	return a.openOrReport(ctx, dir)
}

// OpenWorkspace switches to dir, remembers it, and reloads the page onto its Desktop.
// Reopening the workspace that is already open is a no-op so approvals and page state stay intact.
func (a *App) OpenWorkspace(dir string) error {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		return errors.New("workspace path is empty")
	}
	if abs, err := filepath.Abs(clean); err == nil {
		if _, statErr := os.Stat(abs); errors.Is(statErr, os.ErrNotExist) {
			if mkErr := os.MkdirAll(abs, 0o755); mkErr != nil {
				return mkErr
			}
		}
		dir = abs
	}
	prev := a.host.Current()
	a.mu.Lock()
	last := a.settings.LastWorkspace
	recent := append([]string(nil), a.settings.Recent...)
	a.mu.Unlock()
	candidates := append([]string{prev, last}, recent...)
	if err := BootstrapWorkspace(dir, candidates...); err != nil {
		return err
	}
	if err := a.host.Open(dir); err != nil {
		return err
	}
	cur := a.host.Current()
	wasChooser := prev == "" && cur != ""
	a.mu.Lock()
	a.settings.RememberWorkspace(cur)
	err := SaveSettings(a.settingsPath, a.settings)
	ctx, shown := a.ctx, a.shown
	a.mu.Unlock()
	if wasChooser && ctx != nil && shown {
		runtime.WindowReloadApp(ctx)
	}
	return err
}

func (a *App) openOrReport(ctx context.Context, dir string) error {
	err := a.OpenWorkspace(dir)
	if err != nil {
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "เปิด workspace ไม่ได้",
			Message: err.Error(),
		})
	}
	return err
}

// SetTheme applies "system", "light" or "dark" to the window caption and saves it.
func (a *App) SetTheme(mode string) error {
	switch mode {
	case "system", "light", "dark":
	default:
		return fmt.Errorf("unknown theme %q", mode)
	}
	if ctx := a.context(); ctx != nil {
		applyTheme(ctx, mode)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings.Theme = mode
	return SaveSettings(a.settingsPath, a.settings)
}

// GetShellState returns the active workspace, recent list, multi-project tree, theme and persisted UI preferences.
func (a *App) GetShellState() ShellState {
	a.mu.Lock()
	cur := a.host.Current()
	recent := append([]string(nil), a.settings.Recent...)
	theme := a.settings.Theme
	ui := a.settings.UI
	a.mu.Unlock()
	return ShellState{
		CurrentWorkspace: cur,
		Recent:           recent,
		Projects:         ListProjectFolders(cur, recent),
		Theme:            theme,
		UI:               ui,
	}
}

// SwitchProjectSession switches to project folder dir and selects conversation sessionID.
func (a *App) SwitchProjectSession(dir string, sessionID string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.settings.UI.Selections == nil {
		a.settings.UI.Selections = make(map[string]WorkspaceSelection)
	}
	a.settings.UI.Selections[abs] = WorkspaceSelection{
		SessionID: sessionID,
		TaskID:    "",
	}
	_ = SaveSettings(a.settingsPath, a.settings)
	a.mu.Unlock()
	return a.OpenWorkspace(abs)
}

// RemoveProject removes dir from the Sidebar Projects list without deleting files on disk.
// If dir is currently active and another project exists in Recent, it switches to that project.
func (a *App) RemoveProject(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	wasActive := a.host.Current() != "" && samePath(a.host.Current(), abs)
	a.mu.Lock()
	a.settings.ForgetWorkspace(abs)
	nextWS := a.settings.LastWorkspace
	err = SaveSettings(a.settingsPath, a.settings)
	ctx, shown := a.ctx, a.shown
	a.mu.Unlock()

	a.host.Remove(abs)
	if wasActive {
		if nextWS != "" {
			_ = a.OpenWorkspace(nextWS)
		} else if ctx != nil && shown {
			runtime.WindowReloadApp(ctx)
		}
	}
	return err
}

// OpenNewWindow launches a separate side-by-side APW Desktop window for dir (or current workspace if empty).
func (a *App) OpenNewWindow(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := strings.TrimSpace(dir)
	if target == "" {
		target = a.host.Current()
	}
	if target != "" {
		a.mu.Lock()
		last := a.settings.LastWorkspace
		recent := append([]string(nil), a.settings.Recent...)
		a.mu.Unlock()
		_ = BootstrapWorkspace(target, append([]string{a.host.Current(), last}, recent...)...)
		a.mu.Lock()
		a.settings.RememberWorkspace(target)
		_ = SaveSettings(a.settingsPath, a.settings)
		a.mu.Unlock()
	}
	cmdArgs := []string{"--new-window"}
	if target != "" {
		cmdArgs = append(cmdArgs, target)
	}
	cmd := exec.Command(exe, cmdArgs...)
	if target != "" {
		cmd.Dir = target
	}
	return cmd.Start()
}

// ChooseWorkspaceNewWindow asks for a project folder and opens it in a new parallel window.
func (a *App) ChooseWorkspaceNewWindow() error {
	ctx := a.context()
	if ctx == nil {
		return errNotReady
	}
	dir, err := runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            "เลือกโฟลเดอร์โปรเจกต์เพื่อเปิดในหน้าต่างใหม่",
		DefaultDirectory: a.host.Current(),
	})
	if err != nil || dir == "" {
		return err
	}
	return a.OpenNewWindow(dir)
}

// SaveUIPreferences persists panel widths, collapsed cards and per-workspace selection in desktop.json.
func (a *App) SaveUIPreferences(ui UIPreferences) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings.UI = ui
	if a.settingsPath == "" {
		return nil
	}
	return SaveSettings(a.settingsPath, a.settings)
}

// RevealInExplorer opens Windows Explorer at target (relative to the current workspace or absolute).
func (a *App) RevealInExplorer(target string) error {
	ws := a.host.Current()
	if ws == "" {
		return errors.New("no workspace open")
	}
	clean := strings.TrimSpace(target)
	var full string
	if clean == "" || clean == "." {
		full = ws
	} else if filepath.IsAbs(clean) {
		full = filepath.Clean(clean)
	} else {
		full = filepath.Join(ws, filepath.FromSlash(clean))
	}
	if goruntime.GOOS != "windows" {
		return nil
	}
	fi, err := os.Stat(full)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return exec.Command("explorer.exe", full).Start()
	}
	return exec.Command("explorer.exe", "/select,"+full).Start()
}

func applyTheme(ctx context.Context, mode string) {
	switch mode {
	case "light":
		runtime.WindowSetLightTheme(ctx)
	case "dark":
		runtime.WindowSetDarkTheme(ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(ctx)
	}
}

// startup receives the runtime context. The startup workspace is already open: main opens it
// before the window loads its first page.
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	theme := a.settings.Theme
	a.mu.Unlock()
	applyTheme(ctx, theme)
}

// domReady shows the window once the first page is ready, after restoring its geometry, so
// the user never sees an empty white window. Later calls (page reloads) only re-show it.
// A workspace a second launch asked for in the meantime is opened once the window is shown.
func (a *App) domReady(ctx context.Context) {
	a.mu.Lock()
	first := !a.shown
	a.shown = true
	saved := a.settings.Window
	a.mu.Unlock()
	if first {
		restoreGeometry(saved)
		if saved.Maximised {
			runtime.WindowMaximise(ctx)
		}
		applyWindowIcon()
	}
	runtime.WindowShow(ctx)
	applyWindowIcon()
	if first {
		focusWindow()
		if a.host.Current() != "" {
			if notice := a.host.TakeNotice(); notice != "" {
				go runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
					Type:    runtime.ErrorDialog,
					Title:   "เปิด workspace ไม่ได้",
					Message: notice,
				})
			}
		}
	}
	if dir := a.takePending(); dir != "" {
		go a.openOrReport(ctx, dir)
	}
}

// restoreGeometry moves the window to its saved rectangle, fitted into the work area of the
// nearest monitor. Geometry is handled in Win32 physical pixels throughout: the Wails v2
// position getter and setter use different origins and its sizes are DPI-scaled, so they
// cannot round-trip a window on a scaled display or with a top/left taskbar.
func restoreGeometry(saved WindowState) {
	if !saved.Placed {
		return // first run: keep the centred position Wails gives the window
	}
	h, ok := mainWindow()
	if !ok {
		return
	}
	if work, ok := workAreaNear(saved.Rect()); ok {
		moveWindow(h, ClampRect(saved.Rect(), work))
	}
}

// beforeClose saves the window geometry. A maximised window keeps its last normal rectangle;
// a minimised one (closed from the taskbar) keeps everything as last saved.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h, ok := mainWindow(); ok && !isMinimised(h) {
		if isMaximised(h) {
			a.settings.Window.Maximised = true
		} else if r, ok := windowRect(h); ok {
			a.settings.Window = WindowState{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height, Placed: true}
		}
	}
	_ = SaveSettings(a.settingsPath, a.settings)
	return false
}

// secondInstance brings this window forward and opens the folder the second launch named.
// Before the window is ready the request is kept for domReady: the runtime cannot be called
// without its context, and showing the window early would skip the geometry restore.
// WindowShow also restores a minimised window; a maximised window stays maximised.
func (a *App) secondInstance(data options.SecondInstanceData) {
	dir := WorkspaceArgFrom(data.Args, data.WorkingDirectory)
	a.mu.Lock()
	ctx, shown := a.ctx, a.shown
	if ctx == nil || !shown {
		if dir != "" {
			a.pending = dir
		}
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	runtime.WindowShow(ctx)
	focusWindow()
	if dir != "" {
		_ = a.openOrReport(ctx, dir)
	}
}
