package appinfo

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ListAllProcessArgument(pid int) ([]string, error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, err
	}
	return parseLinuxCommandLine(raw), nil
}

func GetProcessWorkingDirectory(pid int) (string, error) {
	return os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
}

func setPlatformProcessFunction(system *System) {
	system.ListAllProcessID = listAllProcessID
	system.GetSSHRemoteHost = getSSHRemoteHost
}

func listAllProcessID() ([]int, error) {
	allEntry, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	allProcessID := []int{}
	for _, entry := range allEntry {
		if pid, err := strconv.Atoi(entry.Name()); err == nil {
			allProcessID = append(allProcessID, pid)
		}
	}
	return allProcessID, nil
}

// getSSHRemoteHost finds the peer of the TCP socket an sshd process holds, through /proc/net/tcp and tcp6.
func getSSHRemoteHost(pid int) (string, error) {
	allLink, err := filepath.Glob("/proc/" + strconv.Itoa(pid) + "/fd/*")
	if err != nil {
		return "", err
	}
	allSocketInode := map[string]bool{}
	for _, link := range allLink {
		if target, err := os.Readlink(link); err == nil {
			if inode, isSocket := strings.CutPrefix(target, "socket:["); isSocket {
				allSocketInode[strings.TrimSuffix(inode, "]")] = true
			}
		}
	}
	for _, tablePath := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		table, err := os.ReadFile(tablePath)
		if err != nil {
			continue
		}
		if host := findTCPRemoteHost(string(table), allSocketInode); host != "" {
			return host, nil
		}
	}
	return "", errors.New("appinfo: no TCP connection")
}
