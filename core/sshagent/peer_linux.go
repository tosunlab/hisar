package sshagent

import (
	"os"
	"strconv"

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
