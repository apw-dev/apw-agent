//go:build desktop

// Command apw-desktop is APW.exe: the native Windows window for the APW Desktop. It serves the
// selected workspace's desktop.Server in-process through the Wails asset server.
package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	args := os.Args[1:]
	settingsPath, err := SettingsPath()
	settings := DefaultSettings()
	if err == nil {
		settings, _ = LoadSettings(settingsPath)
	}
	host := NewWorkspaceHost()
	// Open the startup workspace before the window exists, so its first page is never the
	// chooser for a workspace that is still being opened.
	OpenInitialWorkspace(host, args, settings.LastWorkspace)
	if dir := host.Current(); dir != "" {
		settings.RememberWorkspace(dir)
		if settingsPath != "" {
			_ = SaveSettings(settingsPath, settings)
		}
	}
	app := NewApp(host, settings, settingsPath)

	bg := options.NewRGB(0xf7, 0xf7, 0xf5)
	if settings.Theme == "dark" || (settings.Theme == "system" && systemPrefersDark()) {
		bg = options.NewRGB(0x1f, 0x1e, 0x1c)
	}

	err = wails.Run(&options.App{
		Title:            "APW",
		Width:            1280, // first-run size; a saved geometry is restored in domReady
		Height:           820,
		MinWidth:         minWindowWidth,
		MinHeight:        minWindowHeight,
		StartHidden:      true,
		BackgroundColour: bg,
		AssetServer:      &assetserver.Options{Handler: host},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "8e0c1f9a-apw-desktop",
			OnSecondInstanceLaunch: app.secondInstance,
		},
		Windows: &windows.Options{
			WindowClassName:      windowClassName,
			BackdropType:         windows.Mica,
			Theme:                windows.SystemDefault,
			WebviewIsTransparent: false,
			IsZoomControlEnabled: false,
			DisablePinchZoom:     true,
		},
		DragAndDrop:              &options.DragAndDrop{DisableWebViewDrop: true},
		EnableDefaultContextMenu: false,
		Debug:                    options.Debug{OpenInspectorOnStartup: DevtoolsRequested(args)},
		Bind:                     []interface{}{app},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "APW:", err)
		os.Exit(1)
	}
}
