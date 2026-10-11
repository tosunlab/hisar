//go:build !linux && !(darwin && !ios && cgo)

package appinfo

import "errors"

var errProcessUnavailable = errors.New("appinfo: process details are not available on this platform")

func ListAllProcessArgument(pid int) ([]string, error) {
	return nil, errProcessUnavailable
}

func GetProcessWorkingDirectory(pid int) (string, error) {
	return "", errProcessUnavailable
}

func setPlatformProcessFunction(system *System) {}
