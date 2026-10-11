package appinfo

import (
	"os"
	"strconv"
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
