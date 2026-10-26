//go:build windows

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// A PATH far longer than NSIS can read must survive an add and a remove unchanged.
func TestEditUserPathKeepsLongExpandPath(t *testing.T) {
	keyPath := fmt.Sprintf(`Software\apw-path-test-%d`, time.Now().UnixNano())
	prev := userEnvKey
	userEnvKey = keyPath
	t.Cleanup(func() {
		userEnvKey = prev
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath)
	})
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for i := 0; i < 120; i++ {
		parts = append(parts, fmt.Sprintf(`C:\Users\someone\tools\entry-%03d\bin`, i))
	}
	parts = append(parts, `%USERPROFILE%\AppData\Roaming\npm`)
	orig := strings.Join(parts, ";")
	if len(orig) < 4000 {
		t.Fatalf("fixture too short: %d", len(orig))
	}
	if err := k.SetExpandStringValue("Path", orig); err != nil {
		t.Fatal(err)
	}
	k.Close()

	read := func() (string, uint32) {
		k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer k.Close()
		v, kind, err := k.GetStringValue("Path")
		if err != nil {
			t.Fatal(err)
		}
		return v, kind
	}

	if changed, err := editUserPath(`C:\APW\bin`, true); err != nil || !changed {
		t.Fatalf("add: %v %v", changed, err)
	}
	if v, kind := read(); v != orig+`;C:\APW\bin` || kind != registry.EXPAND_SZ {
		t.Fatalf("after add: kind=%d len=%d tail=%q", kind, len(v), v[len(v)-60:])
	}
	if changed, err := editUserPath(`C:\APW\bin`, false); err != nil || !changed {
		t.Fatalf("remove: %v %v", changed, err)
	}
	if v, kind := read(); v != orig || kind != registry.EXPAND_SZ {
		t.Fatalf("after remove: kind=%d len=%d", kind, len(v))
	}
}
