//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

type processJob struct{ pid int }

func prepare(cmd *exec.Cmd, _ Limits) (*processJob, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return &processJob{}, nil
}

// HideWindow is a no-op on non-Windows platforms.
func HideWindow(_ *exec.Cmd)                     {}
func (j *processJob) attach(cmd *exec.Cmd) error { j.pid = cmd.Process.Pid; return nil }
func (j *processJob) kill(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
func (j *processJob) close() {
	if j.pid > 0 {
		_ = syscall.Kill(-j.pid, syscall.SIGKILL)
	}
}
