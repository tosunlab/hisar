package appinfo

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func GetProcessWorkingDirectory(pid int) (string, error) {
	return os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
}

func setPlatformProcessFunction(system *System) {
	system.ListProcessIDs = listProcessIDs
	system.GetSSHRemoteHost = getSSHRemoteHost
	system.FindDesktopApp = newSystemDesktopApps().find
}

func listProcessIDs() ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	processIDs := []int{}
	for _, entry := range entries {
		if pid, err := strconv.Atoi(entry.Name()); err == nil {
			processIDs = append(processIDs, pid)
		}
	}
	return processIDs, nil
}

// getSSHRemoteHost finds the peer of the TCP socket an sshd process holds, through /proc/net/tcp and tcp6.
func getSSHRemoteHost(pid int) (string, error) {
	links, err := filepath.Glob("/proc/" + strconv.Itoa(pid) + "/fd/*")
	if err != nil {
		return "", err
	}
	socketInodes := map[string]bool{}
	for _, link := range links {
		if target, err := os.Readlink(link); err == nil {
			if inode, isSocket := strings.CutPrefix(target, "socket:["); isSocket {
				socketInodes[strings.TrimSuffix(inode, "]")] = true
			}
		}
	}
	for _, tablePath := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		table, err := os.ReadFile(tablePath)
		if err != nil {
			continue
		}
		if host := findTCPRemoteHost(string(table), socketInodes); host != "" {
			return host, nil
		}
	}
	return "", errors.New("appinfo: no TCP connection")
}
