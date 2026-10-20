package process

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
	"unsafe"
)

type processJob struct {
	handle windows.Handle
	// watched is closed once the exit watcher started by attach has stopped using handle.
	watched chan struct{}
}

func prepare(cmd *exec.Cmd, limits Limits) (*processJob, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if limits.MemoryBytes > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		info.JobMemoryLimit = uintptr(limits.MemoryBytes)
	}
	if _, err = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if limits.RestrictUI {
		ui := windows.JOBOBJECT_BASIC_UI_RESTRICTIONS{UIRestrictionsClass: windows.JOB_OBJECT_UILIMIT_DESKTOP |
			windows.JOB_OBJECT_UILIMIT_DISPLAYSETTINGS | windows.JOB_OBJECT_UILIMIT_EXITWINDOWS |
			windows.JOB_OBJECT_UILIMIT_GLOBALATOMS | windows.JOB_OBJECT_UILIMIT_HANDLES |
			windows.JOB_OBJECT_UILIMIT_READCLIPBOARD | windows.JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS |
			windows.JOB_OBJECT_UILIMIT_WRITECLIPBOARD}
		if _, err = windows.SetInformationJobObject(h, windows.JobObjectBasicUIRestrictions, uintptr(unsafe.Pointer(&ui)), uint32(unsafe.Sizeof(ui))); err != nil {
			windows.CloseHandle(h)
			return nil, err
		}
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	return &processJob{handle: h}, nil
}
func (j *processJob) attach(cmd *exec.Cmd) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	if err := windows.AssignProcessToJobObject(j.handle, h); err != nil {
		windows.CloseHandle(h)
		return err
	}
	if err := resume(cmd); err != nil {
		windows.CloseHandle(h)
		return err
	}
	// Kill the rest of the tree as soon as the parent exits. Descendants that inherited the
	// parent's output pipes (a browser's crash handler, for example) would otherwise keep them
	// open until cmd.WaitDelay expires and turn a normal exit into exec.ErrWaitDelay.
	j.watched = make(chan struct{})
	go func() {
		defer close(j.watched)
		defer windows.CloseHandle(h)
		if ev, err := windows.WaitForSingleObject(h, windows.INFINITE); err == nil && ev == windows.WAIT_OBJECT_0 {
			_ = windows.TerminateJobObject(j.handle, 1)
		}
	}()
	return nil
}

func resume(cmd *exec.Cmd) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			th, e := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if e != nil {
				return e
			}
			_, e = windows.ResumeThread(th)
			windows.CloseHandle(th)
			return e
		}
	}
	return fmt.Errorf("cannot resume child process thread: %w", err)
}
func (j *processJob) kill(_ *exec.Cmd) error { return windows.TerminateJobObject(j.handle, 1) }
func (j *processJob) close() {
	if j.watched != nil {
		<-j.watched
	}
	windows.CloseHandle(j.handle)
}
