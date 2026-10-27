//go:build !darwin || !cgo

package kitreader

import (
	"errors"
	"os"
)

func openPDF(string) (*os.File, error) {
	return nil, errors.New("Emergency Kit PDF import is available only on macOS.")
}

func decodePDF([]byte) []string { return nil }
