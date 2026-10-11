package appinfo

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const maximumArgumentCount = 4096

// parseKernelProcessArgument reads kern.procargs2: a 32-bit argc, the executable path, NUL padding, argc arguments, then the environment, which is never read.
func parseKernelProcessArgument(raw []byte) ([]string, error) {
	if len(raw) < 4 {
		return nil, errors.New("appinfo: short process arguments")
	}
	argumentCount := int(binary.LittleEndian.Uint32(raw[:4]))
	if argumentCount > maximumArgumentCount {
		return nil, errors.New("appinfo: too many process arguments")
	}
	_, rest, isFound := bytes.Cut(raw[4:], []byte{0})
	if !isFound {
		return nil, errors.New("appinfo: no executable path in process arguments")
	}
	rest = bytes.TrimLeft(rest, "\x00")
	allArgument := make([]string, 0, argumentCount)
	for len(allArgument) < argumentCount && len(rest) > 0 {
		argument, remaining, _ := bytes.Cut(rest, []byte{0})
		allArgument = append(allArgument, string(argument))
		rest = remaining
	}
	return allArgument, nil
}

func parseLinuxCommandLine(raw []byte) []string {
	raw = bytes.TrimSuffix(raw, []byte{0})
	if len(raw) == 0 {
		return []string{}
	}
	allArgument := []string{}
	for _, argument := range bytes.Split(raw, []byte{0}) {
		allArgument = append(allArgument, string(argument))
	}
	return allArgument
}
