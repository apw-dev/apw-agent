package main

import "testing"

// Rects are absolute virtual-screen physical pixels, as Win32 GetWindowRect reports them;
// work is the work area (monitor minus taskbar) of the monitor nearest the saved window.
func TestClampRect(t *testing.T) {
	bottomTaskbar := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	cases := []struct {
		name string
		in   Rect
		work Rect
		want Rect
	}{
		// The monitor the window was on is gone: its old position is past this work area.
		{"OffscreenCentres", Rect{3000, 100, 1280, 820}, bottomTaskbar, Rect{320, 110, 1280, 820}},
		{"NegativeCentres", Rect{-2000, -50, 1280, 820}, bottomTaskbar, Rect{320, 110, 1280, 820}},
		// Larger than the work area: fits exactly, nothing hidden behind the taskbar.
		{"OversizedFitsWorkArea", Rect{500, 300, 3000, 2000}, bottomTaskbar, Rect{0, 0, 1920, 1040}},
		{"KeepsVisiblePosition", Rect{100, 80, 1280, 820}, bottomTaskbar, Rect{100, 80, 1280, 820}},
		{"PullsPartlyOffscreenBackIn", Rect{1000, 500, 1280, 820}, bottomTaskbar, Rect{640, 220, 1280, 820}},
		// A saved position is absolute, so a top or left taskbar does not shift it again.
		{"TopTaskbarNoCreep", Rect{100, 48, 1280, 820}, Rect{0, 48, 1920, 1032}, Rect{100, 48, 1280, 820}},
		{"SecondaryMonitor", Rect{2000, 100, 1280, 820}, Rect{1920, 0, 1920, 1040}, Rect{2000, 100, 1280, 820}},
		{"MonitorLeftOfPrimary", Rect{-1800, 50, 1280, 820}, Rect{-1920, 0, 1920, 1040}, Rect{-1800, 50, 1280, 820}},
		{"CentresOnSecondary", Rect{9000, 100, 1280, 820}, Rect{1920, 0, 1920, 1040}, Rect{2240, 110, 1280, 820}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClampRect(tc.in, tc.work); got != tc.want {
				t.Fatalf("ClampRect(%+v, %+v) = %+v, want %+v", tc.in, tc.work, got, tc.want)
			}
		})
	}
}

func TestWindowStateRect(t *testing.T) {
	ws := WindowState{X: -10, Y: 20, Width: 300, Height: 400}
	if got := ws.Rect(); got != (Rect{-10, 20, 300, 400}) {
		t.Fatalf("Rect() = %+v", got)
	}
}
