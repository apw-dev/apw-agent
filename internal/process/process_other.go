//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

type processJob struct{ pid int }

func prepare(cmd *exec.Cmd) (*processJob, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &processJob{}, nil
}
func (j *processJob) attach(cmd *exec.Cmd) error { j.pid = cmd.Process.Pid; return nil }
func (j *processJob) kill(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
func (j *processJob) close() {
	if j.pid > 0 {
		_ = syscall.Kill(-j.pid, syscall.SIGKILL)
	}
}
