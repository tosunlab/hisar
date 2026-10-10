package commandline

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func getPeerCredential(descriptor int) (Peer, error) {
	credential, err := unix.GetsockoptUcred(descriptor, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return Peer{}, err
	}
	return Peer{UID: int(credential.Uid), PID: int(credential.Pid)}, nil
}

// readAllStatField returns the fields of /proc/<pid>/stat after the command name, so index 0 is field 3 (state).
func readAllStatField(pid int) ([]string, error) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil, err
	}
	allField := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	if len(allField) < 20 {
		return nil, errors.New("commandline: unexpected /proc stat format")
	}
	return allField, nil
}

func readProcessSession(pid int) (TerminalSession, error) {
	allField, err := readAllStatField(pid)
	if err != nil {
		return TerminalSession{}, err
	}
	sessionID, err := strconv.Atoi(allField[3])
	if err != nil {
		return TerminalSession{}, err
	}
	terminalDevice, err := strconv.ParseInt(allField[4], 10, 64)
	if err != nil {
		return TerminalSession{}, err
	}
	session := TerminalSession{SessionID: sessionID, TerminalDevice: terminalDevice}
	if allLeaderField, err := readAllStatField(sessionID); err == nil {
		session.LeaderStartTime, _ = strconv.ParseInt(allLeaderField[19], 10, 64)
	}
	return session, nil
}

// findTerminalName decodes tty_nr from /proc: major in bits 8-15, minor in bits 0-7 and 20-31.
func findTerminalName(terminalDevice int64) string {
	if terminalDevice == 0 {
		return ""
	}
	major := uint32((terminalDevice >> 8) & 0xfff)
	minor := uint32((terminalDevice & 0xff) | ((terminalDevice >> 12) & 0xfff00))
	allPseudoTerminal, _ := filepath.Glob("/dev/pts/*")
	allTerminal, _ := filepath.Glob("/dev/tty*")
	for _, path := range append(allPseudoTerminal, allTerminal...) {
		var stat unix.Stat_t
		if unix.Stat(path, &stat) == nil && unix.Major(stat.Rdev) == major && unix.Minor(stat.Rdev) == minor {
			return strings.TrimPrefix(path, "/dev/")
		}
	}
	return ""
}
