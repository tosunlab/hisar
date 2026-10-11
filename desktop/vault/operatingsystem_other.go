//go:build !darwin

package vault

import "runtime"

func getOperatingSystem() string {
	return runtime.GOOS
}
