//go:build !darwin

package app

import "runtime"

func getOperatingSystem() string {
	return runtime.GOOS
}
