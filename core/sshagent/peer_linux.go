package sshagent

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func getPeerPID(descriptor int) (int, error) {
	credential, err := unix.GetsockoptUcred(descriptor, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return 0, err
	}
	return int(credential.Pid), nil
}

func getExecutablePath(pid int) (string, error) {
	return os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
}

// GetParentPID returns the parent of a process, 0 when it has none.
func GetParentPID(pid int) (int, error) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	afterName := stat[bytes.LastIndexByte(stat, ')')+1:]
	allField := strings.Fields(string(afterName))
	if len(allField) < 2 {
		return 0, errors.New("sshagent: unexpected /proc stat format")
	}
	return strconv.Atoi(allField[1])
}
