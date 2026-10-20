//go:build desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
		Title:            "เลือก workspace ที่มี apw.json",
		DefaultDirectory: a.host.Current(),
	})
	if err != nil || dir == "" {
		return err
	}
	return a.openOrReport(ctx, dir)
}

// OpenWorkspace switches to dir, remembers it, and reloads the page onto its Desktop.
func (a *App) OpenWorkspace(dir string) error {
	if err := a.host.Open(dir); err != nil {
		return err
	}
	a.mu.Lock()
	a.settings.RememberWorkspace(a.host.Current())
	err := SaveSettings(a.settingsPath, a.settings)
	ctx, shown := a.ctx, a.shown
	a.mu.Unlock()
	if ctx != nil && shown {
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
		if screen, ok := currentScreen(ctx); ok {
			ws := ClampWindow(saved, screen)
			runtime.WindowSetSize(ctx, ws.Width, ws.Height)
			runtime.WindowSetPosition(ctx, ws.X, ws.Y)
		}
		if saved.Maximised {
			runtime.WindowMaximise(ctx)
		}
	}
	runtime.WindowShow(ctx)
	if dir := a.takePending(); dir != "" {
		go a.openOrReport(ctx, dir)
	}
}

func currentScreen(ctx context.Context) (Size, bool) {
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return Size{}, false
	}
	pick := screens[0]
	for _, s := range screens {
		if s.IsCurrent {
			pick = s
			break
		}
		if s.IsPrimary {
			pick = s
		}
	}
	return Size{Width: pick.Size.Width, Height: pick.Size.Height}, true
}

// beforeClose saves the window geometry. A maximised window keeps its last normal geometry.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if runtime.WindowIsMaximised(ctx) {
		a.settings.Window.Maximised = true
	} else {
		x, y := runtime.WindowGetPosition(ctx)
		w, h := runtime.WindowGetSize(ctx)
		a.settings.Window = WindowState{X: x, Y: y, Width: w, Height: h}
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
	if dir != "" {
		_ = a.openOrReport(ctx, dir)
	}
}
