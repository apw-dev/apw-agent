package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxRecentWorkspaces = 10

// WindowState is the last window geometry. X and Y are relative to the monitor the window
// was on, which is how the Wails v2 runtime reports and applies positions.
type WindowState struct {
	X, Y, Width, Height int
	Maximised           bool
}

// Settings is %APPDATA%\APW\desktop.json: shell state that must survive restarts and reinstalls.
type Settings struct {
	Version       int         `json:"version"`
	Recent        []string    `json:"recent"`
	LastWorkspace string      `json:"last_workspace"`
	Theme         string      `json:"theme"` // "system" | "light" | "dark"
	Window        WindowState `json:"window"`
}

func DefaultSettings() Settings {
	return Settings{Version: 1, Theme: "system", Window: WindowState{Width: 1280, Height: 820}}
}

func SettingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "APW", "desktop.json"), nil
}

// LoadSettings returns defaults for a missing file. A file that cannot be parsed is kept as
// <path>.bak and defaults are returned, so a bad hand edit never stops the app from starting.
func LoadSettings(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return DefaultSettings(), err
	}
	s := DefaultSettings()
	if err := json.Unmarshal(data, &s); err != nil {
		if renameErr := os.Rename(path, path+".bak"); renameErr != nil {
			return DefaultSettings(), renameErr
		}
		return DefaultSettings(), nil
	}
	switch s.Theme {
	case "system", "light", "dark":
	default:
		s.Theme = "system"
	}
	s.Version = 1
	return s, nil
}

// SaveSettings writes s atomically: a temp file in the same directory, then rename.
func SaveSettings(path string, s Settings) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".desktop-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.Write(data)
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RememberWorkspace moves dir to the front of Recent (deduplicated, capped) and makes it the
// workspace reopened on next launch.
func (s *Settings) RememberWorkspace(dir string) {
	recent := []string{dir}
	for _, d := range s.Recent {
		if !samePath(d, dir) && len(recent) < maxRecentWorkspaces {
			recent = append(recent, d)
		}
	}
	s.Recent = recent
	s.LastWorkspace = dir
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
