//go:build desktop && windows

package main

import "golang.org/x/sys/windows/registry"

// systemPrefersDark reads the Windows app theme so the window's first paint matches it.
func systemPrefersDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}
