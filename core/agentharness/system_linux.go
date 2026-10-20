package agentharness

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/localsocket"
	"golang.org/x/sys/unix"
)

// SystemReader reads processes from /proc; it reads no signature.
func SystemReader() Reader {
	return Reader{ReadInstance: ReadInstance, GetPath: localsocket.GetProcessPath, ListArguments: ListArguments, GetWorkingDirectory: getWorkingDirectory}
}

// ReadInstance reads the start time and the parent in one read of /proc/<pid>/stat, so both belong to the same
// instance. The start time is in clock ticks since boot.
func ReadInstance(pid int) (Instance, error) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return Instance{}, err
	}
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	if len(fields) < 20 {
		return Instance{}, errors.New("agentharness: unexpected /proc stat format")
	}
	parentPID, err := strconv.Atoi(fields[1])
	if err != nil {
		return Instance{}, err
	}
	startTime, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return Instance{}, err
	}
	return Instance{StartTime: startTime, ParentPID: parentPID, IsExited: fields[0] == "Z" || fields[0] == "X"}, nil
}

func ListArguments(pid int) ([]string, error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, err
	}
	return parseLinuxCommandLine(raw), nil
}

func getWorkingDirectory(pid int) (string, error) {
	return os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
}

// WaitForExit returns once the session's harness process exits, or when ctx ends. A pidfd becomes readable at the
// exit; kernels before 5.3 have none, so the process is polled.
func WaitForExit(ctx context.Context, session Session) {
	descriptor, err := unix.PidfdOpen(session.PID, 0)
	if err != nil {
		if err != unix.ESRCH {
			pollForExit(ctx, session)
		}
		return
	}
	defer unix.Close(descriptor)
	if !IsRunning(session) {
		return
	}
	pollDescriptors := []unix.PollFd{{Fd: int32(descriptor), Events: unix.POLLIN}}
	for ctx.Err() == nil {
		count, err := unix.Poll(pollDescriptors, int(time.Second/time.Millisecond))
		if count > 0 {
			return
		}
		if err != nil && err != unix.EINTR {
			pollForExit(ctx, session)
			return
		}
	}
}
