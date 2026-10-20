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
type App struct {
	ctx          context.Context
	host         *WorkspaceHost
	settingsPath string

	mu       sync.Mutex
	settings Settings
	shown    bool
}

func NewApp(host *WorkspaceHost, settings Settings, settingsPath string) *App {
	return &App{host: host, settings: settings, settingsPath: settingsPath}
}

// ChooseWorkspace asks for a folder and opens it. Cancelling the dialog is not an error.
func (a *App) ChooseWorkspace() error {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "เลือก workspace ที่มี apw.json",
		DefaultDirectory: a.host.Current(),
	})
	if err != nil || dir == "" {
		return err
	}
	if err := a.OpenWorkspace(dir); err != nil {
		_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "เปิด workspace ไม่ได้",
			Message: err.Error(),
		})
		return err
	}
	return nil
}

// OpenWorkspace switches to dir, remembers it, and reloads the page onto its Desktop.
func (a *App) OpenWorkspace(dir string) error {
	if err := a.host.Open(dir); err != nil {
		return err
	}
	a.mu.Lock()
	a.settings.RememberWorkspace(a.host.Current())
	err := SaveSettings(a.settingsPath, a.settings)
	shown := a.shown
	a.mu.Unlock()
	if shown {
		runtime.WindowReloadApp(a.ctx)
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
	a.applyTheme(mode)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings.Theme = mode
	return SaveSettings(a.settingsPath, a.settings)
}

func (a *App) applyTheme(mode string) {
	switch mode {
	case "light":
		runtime.WindowSetLightTheme(a.ctx)
	case "dark":
		runtime.WindowSetDarkTheme(a.ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(a.ctx)
	}
}

func (a *App) startup(ctx context.Context, args []string) {
	a.ctx = ctx
	a.mu.Lock()
	theme, last := a.settings.Theme, a.settings.LastWorkspace
	a.mu.Unlock()
	a.applyTheme(theme)
	dir := WorkspaceArg(args)
	if dir == "" {
		dir = last
	}
	if dir == "" {
		return
	}
	if err := a.OpenWorkspace(dir); err != nil {
		a.host.SetNotice("เปิด workspace เดิมไม่ได้: " + err.Error())
	}
}

// domReady shows the window once the first page is ready, after restoring its geometry, so
// the user never sees an empty white window. Later calls (page reloads) only re-show it.
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
func (a *App) secondInstance(data options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	dir := WorkspaceArgFrom(data.Args, data.WorkingDirectory)
	if dir == "" {
		return
	}
	if err := a.OpenWorkspace(dir); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "เปิด workspace ไม่ได้",
			Message: err.Error(),
		})
	}
}
