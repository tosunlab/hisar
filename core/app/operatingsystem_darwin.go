package app

import "syscall"

func getOperatingSystem() string {
	version, err := syscall.Sysctl("kern.osproductversion")
	if err != nil || version == "" {
		return "macOS"
	}
	return "macOS " + version
}
