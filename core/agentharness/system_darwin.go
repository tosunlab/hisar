package agentharness

import (
	"context"
	"time"

	"github.com/tryhisar/hisar/core/localsocket"
	"golang.org/x/sys/unix"
)

// zombieState is SZOMB in <sys/proc.h>: the process exited and waits for its parent.
const zombieState = 5

// SystemReader reads processes with sysctl only, so it needs no cgo; it reads no working directory or signature.
func SystemReader() Reader {
	return Reader{ReadInstance: ReadInstance, GetPath: localsocket.GetProcessPath, ListArguments: ListArguments}
}

// ReadInstance reads the start time and the parent in one kern.proc.pid read, so both belong to the same instance.
func ReadInstance(pid int) (Instance, error) {
	processInfo, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Instance{}, err
	}
	if processInfo.Proc.P_pid != int32(pid) {
		return Instance{}, errNoProcess
	}
	startTime := processInfo.Proc.P_starttime
	return Instance{
		StartTime: startTime.Sec*1_000_000 + int64(startTime.Usec),
		ParentPID: int(processInfo.Eproc.Ppid),
		IsExited:  processInfo.Proc.P_stat == zombieState,
	}, nil
}

func ListArguments(pid int) ([]string, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err
	}
	return parseKernelProcessArgument(raw)
}

// WaitForExit returns once the session's harness process exits, or when ctx ends. kqueue reports the exit; it is
// checked every second for ctx.
func WaitForExit(ctx context.Context, session Session) {
	queue, err := unix.Kqueue()
	if err != nil {
		pollForExit(ctx, session)
		return
	}
	defer unix.Close(queue)
	var change unix.Kevent_t
	unix.SetKevent(&change, session.PID, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
	change.Fflags = unix.NOTE_EXIT
	if _, err := unix.Kevent(queue, []unix.Kevent_t{change}, nil, nil); err != nil {
		if err != unix.ESRCH {
			pollForExit(ctx, session)
		}
		return
	}
	if !IsRunning(session) {
		return
	}
	events := make([]unix.Kevent_t, 1)
	timeout := unix.NsecToTimespec(int64(time.Second))
	for ctx.Err() == nil {
		count, err := unix.Kevent(queue, nil, events, &timeout)
		if count > 0 {
			return
		}
		if err != nil && err != unix.EINTR {
			pollForExit(ctx, session)
			return
		}
	}
}
