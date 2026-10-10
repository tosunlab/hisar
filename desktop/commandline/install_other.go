//go:build !darwin && !linux

package commandline

func isWritableDirectory(string) bool { return false }
