package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// InstanceState records the PID and binary SHA-256 of the running APW Desktop process in
// %APPDATA%\APW\instance.json so a newly rebuilt/updated binary can detect and terminate
// a stale in-memory apw-desktop.exe before Wails SingleInstanceLock hands off to it.
type InstanceState struct {
	PID        int    `json:"pid"`
	Executable string `json:"executable,omitempty"`
	SHA256     string `json:"sha256"`
}

func InstanceStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "APW", "instance.json"), nil
}

func LoadInstanceState(path string) (InstanceState, error) {
	if path == "" {
		return InstanceState{}, errors.New("empty path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return InstanceState{}, err
	}
	var st InstanceState
	if err := json.Unmarshal(data, &st); err != nil {
		return InstanceState{}, err
	}
	return st, nil
}

func SaveInstanceState(path string, st InstanceState) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".instance-*.json")
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

func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ReconcileDesktopInstance checks if an existing APW Desktop window/process is running an older
// or different binary build. If it matches selfSHA, it leaves the running instance intact so
// SingleInstanceLock can focus it. If it differs, it kills the stale process and waits for the
// window to close before returning true.
func ReconcileDesktopInstance(
	instancePath string,
	selfPID int,
	selfExe string,
	selfSHA string,
	findWindowPID func() int,
	killPID func(int) error,
) bool {
	if selfSHA == "" {
		return false
	}
	saved, _ := LoadInstanceState(instancePath)
	winPID := 0
	if findWindowPID != nil {
		winPID = findWindowPID()
	}

	// Case 1: An APWDesktopWindow is currently open in another process.
	if winPID > 0 && winPID != selfPID {
		if saved.PID == winPID && saved.SHA256 == selfSHA {
			// Running instance is already the exact same binary build; let SingleInstanceLock focus it.
			return false
		}
		if killPID != nil {
			_ = killPID(winPID)
		}
		if saved.PID > 0 && saved.PID != winPID && saved.PID != selfPID {
			if killPID != nil {
				_ = killPID(saved.PID)
			}
		}
		if findWindowPID != nil {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if findWindowPID() == 0 {
					break
				}
				time.Sleep(40 * time.Millisecond)
			}
		}
		_ = SaveInstanceState(instancePath, InstanceState{
			PID:        selfPID,
			Executable: selfExe,
			SHA256:     selfSHA,
		})
		return true
	}

	// Case 2: No window handle found (or hidden), but instance.json points to a different build's PID.
	if saved.PID > 0 && saved.PID != selfPID && saved.SHA256 != "" && saved.SHA256 != selfSHA {
		if killPID != nil {
			_ = killPID(saved.PID)
		}
		time.Sleep(100 * time.Millisecond)
		_ = SaveInstanceState(instancePath, InstanceState{
			PID:        selfPID,
			Executable: selfExe,
			SHA256:     selfSHA,
		})
		return true
	}

	// Case 3: Fresh startup with no running window.
	_ = SaveInstanceState(instancePath, InstanceState{
		PID:        selfPID,
		Executable: selfExe,
		SHA256:     selfSHA,
	})
	return false
}

func killProcessByPID(pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
