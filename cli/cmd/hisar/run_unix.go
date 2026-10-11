//go:build !windows

package main

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

var allForwardedSignal = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}

func isTerminalSignal(receivedSignal os.Signal) bool {
	return receivedSignal == syscall.SIGINT || receivedSignal == syscall.SIGQUIT
}

// checkForegroundJob reports whether hisar's process group is the foreground job of its controlling terminal.
func checkForegroundJob() bool {
	terminal, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	defer terminal.Close()
	foregroundGroup, err := unix.IoctlGetInt(int(terminal.Fd()), unix.TIOCGPGRP)
	return err == nil && foregroundGroup == unix.Getpgrp()
}
