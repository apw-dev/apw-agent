package daemon

import (
	"os/exec"
	"syscall"
)

func configureBackground(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x00000008}
}
