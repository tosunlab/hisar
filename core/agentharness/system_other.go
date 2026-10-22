//go:build !darwin && !linux

package agentharness

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("agentharness: processes cannot be read on this platform")

// SystemReader reads nothing here, so every chain is unknown.
func SystemReader() Reader {
	return Reader{ReadInstance: ReadInstance, GetPath: func(int) (string, error) { return "", errUnsupported }}
}

func ReadInstance(int) (Instance, error) {
	return Instance{}, errUnsupported
}

func ListArguments(int) ([]string, error) {
	return nil, errUnsupported
}

func WaitForExit(ctx context.Context, session Session) {
	pollForExit(ctx, session)
}
