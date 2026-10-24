package main

import (
	"path/filepath"
	"testing"
)

func TestReconcileDesktopInstance_KillsStaleBuildAndKeepsMatchingBuild(t *testing.T) {
	dir := t.TempDir()
	instFile := filepath.Join(dir, "instance.json")

	// 1. Legacy running window with no instance.json -> killed and replaced
	winPID := 4242
	var killed []int
	replaced := ReconcileDesktopInstance(
		instFile,
		5000,
		`C:\APW\apw-desktop.exe`,
		"sha-v2",
		func() int { return winPID },
		func(pid int) error {
			killed = append(killed, pid)
			winPID = 0
			return nil
		},
	)
	if !replaced || len(killed) != 1 || killed[0] != 4242 {
		t.Fatalf("expected legacy window PID 4242 to be killed, got replaced=%v killed=%v", replaced, killed)
	}
	saved, err := LoadInstanceState(instFile)
	if err != nil || saved.PID != 5000 || saved.SHA256 != "sha-v2" {
		t.Fatalf("saved state = %+v, err = %v", saved, err)
	}

	// 2. Second launch of the EXACT SAME build ("sha-v2") while PID 5000 is running -> NOT killed
	winPID = 5000
	killed = nil
	replaced = ReconcileDesktopInstance(
		instFile,
		6000,
		`C:\APW\apw-desktop.exe`,
		"sha-v2",
		func() int { return winPID },
		func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
	)
	if replaced || len(killed) != 0 {
		t.Fatalf("expected matching build PID 5000 to stay alive, got replaced=%v killed=%v", replaced, killed)
	}

	// 3. Launching a NEW build ("sha-v3") while PID 5000 ("sha-v2") is running -> kills PID 5000
	replaced = ReconcileDesktopInstance(
		instFile,
		7000,
		`C:\APW\apw-desktop.exe`,
		"sha-v3",
		func() int { return winPID },
		func(pid int) error {
			killed = append(killed, pid)
			winPID = 0
			return nil
		},
	)
	if !replaced || len(killed) != 1 || killed[0] != 5000 {
		t.Fatalf("expected outdated build PID 5000 to be killed, got replaced=%v killed=%v", replaced, killed)
	}
	saved, err = LoadInstanceState(instFile)
	if err != nil || saved.PID != 7000 || saved.SHA256 != "sha-v3" {
		t.Fatalf("saved state after upgrade = %+v, err = %v", saved, err)
	}
}
