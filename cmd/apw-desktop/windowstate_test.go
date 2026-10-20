package main

import "testing"

func TestClampWindow(t *testing.T) {
	screen := Size{Width: 1920, Height: 1080}
	cases := []struct {
		name string
		in   WindowState
		want WindowState
	}{
		// The monitor the window was on is gone: its old position is past this screen.
		{"OffscreenCentres", WindowState{X: 3000, Y: 100, Width: 1280, Height: 820}, WindowState{X: 320, Y: 130, Width: 1280, Height: 820}},
		{"NegativeCentres", WindowState{X: -2000, Y: -50, Width: 1280, Height: 820}, WindowState{X: 320, Y: 130, Width: 1280, Height: 820}},
		{"EnforcesMinimum", WindowState{X: 10, Y: 10, Width: 400, Height: 300}, WindowState{X: 10, Y: 10, Width: 960, Height: 600}},
		{"ShrinksToScreen", WindowState{X: 500, Y: 300, Width: 3000, Height: 2000}, WindowState{X: 0, Y: 0, Width: 1920, Height: 1080}},
		{"KeepsVisiblePosition", WindowState{X: 100, Y: 80, Width: 1280, Height: 820}, WindowState{X: 100, Y: 80, Width: 1280, Height: 820}},
		{"PullsPartlyOffscreenBackIn", WindowState{X: 1000, Y: 500, Width: 1280, Height: 820}, WindowState{X: 640, Y: 260, Width: 1280, Height: 820}},
		{"KeepsMaximised", WindowState{X: 0, Y: 0, Width: 1280, Height: 820, Maximised: true}, WindowState{X: 0, Y: 0, Width: 1280, Height: 820, Maximised: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClampWindow(tc.in, screen); got != tc.want {
				t.Fatalf("ClampWindow(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

// A screen smaller than the minimum window still gets a window that fits on it.
func TestClampWindowTinyScreen(t *testing.T) {
	got := ClampWindow(WindowState{Width: 1280, Height: 820}, Size{Width: 800, Height: 600})
	if got.Width != 800 || got.Height != 600 || got.X != 0 || got.Y != 0 {
		t.Fatalf("got %+v", got)
	}
}
