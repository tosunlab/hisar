package agentharness

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const maximumArgumentCount = 4096

// parseKernelProcessArgument reads kern.procargs2: a 32-bit argc, the executable path, NUL padding, argc arguments, then the environment, which is never read.
func parseKernelProcessArgument(raw []byte) ([]string, error) {
	if len(raw) < 4 {
		return nil, errors.New("agentharness: short process arguments")
	}
	argumentCount := int(binary.LittleEndian.Uint32(raw[:4]))
	if argumentCount > maximumArgumentCount {
		return nil, errors.New("agentharness: too many process arguments")
	}
	_, rest, isFound := bytes.Cut(raw[4:], []byte{0})
	if !isFound {
		return nil, errors.New("agentharness: no executable path in process arguments")
	}
	rest = bytes.TrimLeft(rest, "\x00")
	arguments := make([]string, 0, argumentCount)
	for len(arguments) < argumentCount && len(rest) > 0 {
		argument, remaining, _ := bytes.Cut(rest, []byte{0})
		arguments = append(arguments, string(argument))
		rest = remaining
	}
	return arguments, nil
}

func parseLinuxCommandLine(raw []byte) []string {
	raw = bytes.TrimSuffix(raw, []byte{0})
	if len(raw) == 0 {
		return []string{}
	}
	arguments := []string{}
	for _, argument := range bytes.Split(raw, []byte{0}) {
		arguments = append(arguments, string(argument))
	}
	return arguments
}
