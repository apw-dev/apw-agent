//go:build desktop && windows

package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowClassName is the Win32 class of the main window (windows.Options.WindowClassName),
// unique so the window can be found without depending on its title.
const windowClassName = "APWDesktopWindow"

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procIsZoomed                 = user32.NewProc("IsZoomed")
	procIsIconic                 = user32.NewProc("IsIconic")
	procMonitorFromRect          = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
)

type winRect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
}

const (
	monitorDefaultToNearest = 2
	swpNoZOrder             = 0x0004
	swpNoActivate           = 0x0010
	swRestore               = 9
)

func (r winRect) rect() Rect {
	return Rect{int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)}
}

// existingWindowPID returns the PID of any process currently owning an APWDesktopWindow, or 0.
func existingWindowPID() int {
	cls, _ := windows.UTF16PtrFromString(windowClassName)
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return 0
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return int(pid)
}

// mainWindow returns this process's APW window.
func mainWindow() (uintptr, bool) {
	cls, _ := windows.UTF16PtrFromString(windowClassName)
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return 0, false
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return h, int(pid) == os.Getpid()
}

func windowRect(h uintptr) (Rect, bool) {
	var r winRect
	ok, _, _ := procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return r.rect(), ok != 0
}

func isMaximised(h uintptr) bool { r, _, _ := procIsZoomed.Call(h); return r != 0 }
func isMinimised(h uintptr) bool { r, _, _ := procIsIconic.Call(h); return r != 0 }

// workAreaNear returns the work area (monitor minus taskbar) of the monitor nearest r.
func workAreaNear(r Rect) (Rect, bool) {
	wr := winRect{int32(r.X), int32(r.Y), int32(r.X + r.Width), int32(r.Y + r.Height)}
	mon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&wr)), monitorDefaultToNearest)
	if mon == 0 {
		return Rect{}, false
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&info)))
	return info.Work.rect(), ok != 0
}

func moveWindow(h uintptr, r Rect) {
	procSetWindowPos.Call(h, 0, uintptr(int32(r.X)), uintptr(int32(r.Y)), uintptr(int32(r.Width)), uintptr(int32(r.Height)), swpNoZOrder|swpNoActivate)
}

// focusWindow unminimises the window (leaving a maximised window maximised) and brings it to
// the foreground.
func focusWindow() {
	h, ok := mainWindow()
	if !ok {
		return
	}
	if isMinimised(h) {
		procShowWindow.Call(h, swRestore)
	}
	procSetForegroundWindow.Call(h)
}
