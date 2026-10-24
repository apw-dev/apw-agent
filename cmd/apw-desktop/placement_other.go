//go:build desktop && !windows

package main

// Geometry restore is Windows-only for now (spec: Windows first).

const windowClassName = ""

func existingWindowPID() int          { return 0 }
func mainWindow() (uintptr, bool)     { return 0, false }
func windowRect(uintptr) (Rect, bool) { return Rect{}, false }
func isMaximised(uintptr) bool        { return false }
func isMinimised(uintptr) bool        { return false }
func workAreaNear(Rect) (Rect, bool)  { return Rect{}, false }
func moveWindow(uintptr, Rect)        {}
func focusWindow()                    {}
