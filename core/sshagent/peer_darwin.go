package sshagent

import (
	"bytes"
	"errors"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func getPeerPID(descriptor int) (int, error) {
	return unix.GetsockoptInt(descriptor, unix.SOL_LOCAL, unix.LOCAL_PEERPID)
}

func getExecutablePath(pid int) (string, error) {
	if path, err := readProcessArgumentPath(pid); err == nil {
		return path, nil
	}
	output, err := exec.Command("/bin/ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", errors.New("sshagent: empty process path")
	}
	return path, nil
}

// kern.procargs2 starts with a 4-byte argc followed by the NUL-terminated executable path.
func readProcessArgumentPath(pid int) (string, error) {
	processArgument, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", err
	}
	if len(processArgument) < 5 {
		return "", errors.New("sshagent: short kern.procargs2")
	}
	path, _, _ := bytes.Cut(processArgument[4:], []byte{0})
	if len(path) == 0 {
		return "", errors.New("sshagent: empty process path")
	}
	return string(path), nil
}
