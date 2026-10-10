package commandline

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const noTerminalDevice = -1

func getPeerCredential(descriptor int) (Peer, error) {
	credential, err := unix.GetsockoptXucred(descriptor, unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return Peer{}, err
	}
	pid, err := unix.GetsockoptInt(descriptor, unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	if err != nil {
		return Peer{}, err
	}
	return Peer{UID: int(credential.Uid), PID: pid}, nil
}

func readProcessSession(pid int) (TerminalSession, error) {
	sessionID, err := unix.Getsid(pid)
	if err != nil {
		return TerminalSession{}, err
	}
	processInfo, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return TerminalSession{}, err
	}
	if processInfo.Proc.P_pid != int32(pid) {
		return TerminalSession{}, errors.New("commandline: process not found")
	}
	session := TerminalSession{SessionID: sessionID, TerminalDevice: int64(processInfo.Eproc.Tdev)}
	if session.TerminalDevice == noTerminalDevice {
		session.TerminalDevice = 0
	}
	if leaderInfo, err := unix.SysctlKinfoProc("kern.proc.pid", sessionID); err == nil && leaderInfo.Proc.P_pid == int32(sessionID) {
		startTime := leaderInfo.Proc.P_starttime
		session.LeaderStartTime = startTime.Sec*1_000_000 + int64(startTime.Usec)
	}
	return session, nil
}

func findTerminalName(terminalDevice int64) string {
	if terminalDevice == 0 {
		return ""
	}
	allEntry, err := os.ReadDir("/dev")
	if err != nil {
		return ""
	}
	for _, entry := range allEntry {
		if !strings.HasPrefix(entry.Name(), "tty") && entry.Name() != "console" {
			continue
		}
		var stat unix.Stat_t
		if unix.Stat("/dev/"+entry.Name(), &stat) == nil && int64(stat.Rdev) == terminalDevice {
			return entry.Name()
		}
	}
	return ""
}
