package main

const (
	minWindowWidth  = 960
	minWindowHeight = 600
	// titleGrabWidth/Height is the part of the title bar that must be on screen for the
	// user to be able to grab and move the window.
	titleGrabWidth  = 100
	titleGrabHeight = 40
)

type Size struct{ Width, Height int }

// ClampWindow fits a saved window onto the current monitor. ws.X/Y are monitor-relative, as the
// Wails v2 runtime reports and applies them. The size is raised to the minimum and capped at
// the screen; a window whose title area is off screen (its monitor was removed) is centred,
// otherwise it is pulled fully onto the screen.
func ClampWindow(ws WindowState, screen Size) WindowState {
	ws.Width = min(max(ws.Width, minWindowWidth), screen.Width)
	ws.Height = min(max(ws.Height, minWindowHeight), screen.Height)
	titleVisible := ws.X >= 0 && ws.Y >= 0 && ws.X+titleGrabWidth <= screen.Width && ws.Y+titleGrabHeight <= screen.Height
	if !titleVisible {
		ws.X = (screen.Width - ws.Width) / 2
		ws.Y = (screen.Height - ws.Height) / 2
		return ws
	}
	ws.X = min(ws.X, screen.Width-ws.Width)
	ws.Y = min(ws.Y, screen.Height-ws.Height)
	return ws
}
