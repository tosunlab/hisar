package mcpserver

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// processGroup is a job object that holds the command and every process it starts, so stopping the command also
// stops the children that inherited its secrets.
type processGroup struct {
	job windows.Handle
}

// startProcessGroup starts the command suspended and resumes it only once it is in the job, so nothing it starts can
// run outside the job. The job ends what it holds when its last handle closes, which hisar mcp's exit or crash does
// while the command runs.
func startProcessGroup(command *exec.Cmd) (processGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processGroup{}, err
	}
	if err := setKillOnJobClose(job, true); err != nil {
		windows.CloseHandle(job)
		return processGroup{}, err
	}
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := command.Start(); err != nil {
		windows.CloseHandle(job)
		return processGroup{}, err
	}
	if err := assignAndResume(job, uint32(command.Process.Pid)); err != nil {
		command.Process.Kill()
		command.Wait()
		windows.CloseHandle(job)
		return processGroup{}, err
	}
	return processGroup{job: job}, nil
}

func setKillOnJobClose(job windows.Handle, isKilling bool) error {
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if isKilling {
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	}
	_, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	return err
}

func assignAndResume(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return err
	}
	if status, _, _ := ntResumeProcess.Call(uintptr(process)); status != 0 {
		return fmt.Errorf("resume the command: NTSTATUS %#x", status)
	}
	return nil
}

// stop ends every process in the job; Windows has no SIGTERM to send first.
func (group processGroup) stop(waitResult <-chan error, _ time.Duration) error {
	windows.TerminateJobObject(group.job, 1)
	return <-waitResult
}

// close lets go of the job once the command finished or was stopped. It first turns off ending the job on close, so a
// background process a finished command left behind keeps running, as on Unix.
func (group processGroup) close() {
	setKillOnJobClose(group.job, false)
	windows.CloseHandle(group.job)
}
