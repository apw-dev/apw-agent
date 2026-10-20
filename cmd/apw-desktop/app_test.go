//go:build desktop

package main

import (
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

// Wails delivers a second launch as soon as the single-instance window exists, which can be
// before OnStartup has given the app its context. Calling the runtime then exits the process
// (log.Fatalf on a nil context), so the request must be kept until the window is ready.
func TestSecondInstanceBeforeStartupIsDeferred(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir)
	app := NewApp(NewWorkspaceHost(), DefaultSettings(), filepath.Join(t.TempDir(), "desktop.json"))

	app.secondInstance(options.SecondInstanceData{Args: []string{"sub"}, WorkingDirectory: filepath.Dir(dir)})
	app.secondInstance(options.SecondInstanceData{Args: []string{dir}, WorkingDirectory: t.TempDir()})

	if got := app.takePending(); got != dir {
		t.Fatalf("pending workspace = %q, want %q (latest request wins)", got, dir)
	}
	if got := app.takePending(); got != "" {
		t.Fatalf("pending not cleared: %q", got)
	}
}
