//go:build !darwin || !cgo

package appinfo

import "errors"

func getBundleDisplayName(bundlePath string) string {
	return ""
}

func getBundleIconPNG(bundlePath string, size int) ([]byte, error) {
	return nil, errors.New("appinfo: icons are only available on macOS")
}
