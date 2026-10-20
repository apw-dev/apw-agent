package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestLoadSettingsMissingReturnsDefaults(t *testing.T) {
	got, err := LoadSettings(filepath.Join(t.TempDir(), "desktop.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultSettings()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// No saved geometry yet: the window keeps the position Wails gives it (centred).
	if want.Version != 1 || want.Theme != "system" || want.Window.Placed {
		t.Fatalf("unexpected defaults %+v", want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "APW", "desktop.json")
	s := DefaultSettings()
	s.Theme = "dark"
	s.RememberWorkspace(filepath.Join(t.TempDir(), "repo"))
	s.Window = WindowState{X: -40, Y: 50, Width: 1400, Height: 900, Maximised: true, Placed: true}
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("round trip: got %+v, want %+v", got, s)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestLoadSettingsCorruptKeepsBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	bad := []byte(`{"version":`)
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("corrupt file must not fail: %v", err)
	}
	if !reflect.DeepEqual(got, DefaultSettings()) {
		t.Fatalf("got %+v, want defaults", got)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != string(bad) {
		t.Fatalf("backup = %q, %v", backup, err)
	}
}

func TestRememberWorkspaceDedupesAndCaps(t *testing.T) {
	var s Settings
	dirs := make([]string, 12)
	for i := range dirs {
		dirs[i] = filepath.Join(string(filepath.Separator), fmt.Sprintf("ws%02d", i))
		s.RememberWorkspace(dirs[i])
	}
	s.RememberWorkspace(dirs[2])
	if len(s.Recent) != 10 {
		t.Fatalf("len(Recent) = %d, want 10", len(s.Recent))
	}
	if s.Recent[0] != dirs[2] || s.LastWorkspace != dirs[2] {
		t.Fatalf("front = %q last = %q, want %q", s.Recent[0], s.LastWorkspace, dirs[2])
	}
	seen := map[string]bool{}
	for _, d := range s.Recent {
		if seen[d] {
			t.Fatalf("duplicate %q in %v", d, s.Recent)
		}
		seen[d] = true
	}
	if runtime.GOOS == "windows" {
		var w Settings
		w.RememberWorkspace(`C:\A`)
		w.RememberWorkspace(`c:\a`)
		if len(w.Recent) != 1 {
			t.Fatalf("case variants not deduped: %v", w.Recent)
		}
	}
}

func TestLoadSettingsInvalidThemeFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"theme":"neon"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "system" {
		t.Fatalf("theme = %q, want system", got.Theme)
	}
}

func TestLoadSettingsUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	raw := append([]byte("\xef\xbb\xbf"), []byte(`{"version":1,"theme":"dark","last_workspace":"D:\\repo"}`)...)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dark" || got.LastWorkspace != `D:\repo` {
		t.Fatalf("got %+v, want dark and D:\\repo", got)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("UTF-8 BOM file must not be treated as corrupt (.bak err = %v)", err)
	}
}
