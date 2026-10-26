//go:build windows

package main

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// userEnvKey is the registry key holding the user's environment; tests point it elsewhere.
var userEnvKey = `Environment`

// editUserPath adds or removes dir in HKCU\Environment\Path. It reads the whole value (no length
// limit), keeps its type and every other entry as written, and writes nothing when the value
// cannot be read.
func editUserPath(dir string, add bool) (bool, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, userEnvKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	current, kind, err := k.GetStringValue("Path")
	switch {
	case errors.Is(err, registry.ErrNotExist):
		current, kind = "", registry.EXPAND_SZ
	case err != nil:
		return false, err
	}
	next, changed := pathWithoutEntry(current, dir)
	if add {
		next, changed = pathWithEntry(current, dir)
	}
	if !changed {
		return false, nil
	}
	if kind == registry.SZ {
		err = k.SetStringValue("Path", next)
	} else {
		err = k.SetExpandStringValue("Path", next)
	}
	if err != nil {
		return false, err
	}
	broadcastEnvironmentChange()
	return true, nil
}

// broadcastEnvironmentChange tells running programs (Explorer, new terminals) to reload the
// environment.
func broadcastEnvironmentChange() {
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	var result uintptr
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	_, _, _ = proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
