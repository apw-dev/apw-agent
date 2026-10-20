package main

const (
	minWindowWidth  = 960
	minWindowHeight = 600
	// titleGrabWidth/Height is the part of the title bar that must be on screen for the
	// user to be able to grab and move the window.
	titleGrabWidth  = 100
	titleGrabHeight = 40
)

// Rect is a window or work-area rectangle in absolute virtual-screen physical pixels, the
// units of Win32 GetWindowRect/SetWindowPos and MONITORINFO.rcWork.
type Rect struct{ X, Y, Width, Height int }

// Rect returns the saved geometry as a Rect.
func (ws WindowState) Rect() Rect { return Rect{ws.X, ws.Y, ws.Width, ws.Height} }

// ClampRect fits a saved window into the work area of the monitor it is restored onto: the
// size is capped at the work area, a window whose title area is outside it (its monitor
// was removed) is centred, and otherwise it is pulled fully inside. The minimum window size
// is enforced by the window itself.
func ClampRect(r, work Rect) Rect {
	r.Width = min(r.Width, work.Width)
	r.Height = min(r.Height, work.Height)
	titleVisible := r.X >= work.X && r.Y >= work.Y &&
		r.X+titleGrabWidth <= work.X+work.Width && r.Y+titleGrabHeight <= work.Y+work.Height
	if !titleVisible {
		r.X = work.X + (work.Width-r.Width)/2
		r.Y = work.Y + (work.Height-r.Height)/2
		return r
	}
	r.X = min(r.X, work.X+work.Width-r.Width)
	r.Y = min(r.Y, work.Y+work.Height-r.Height)
	return r
}
