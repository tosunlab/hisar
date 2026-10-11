package appinfo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"strings"
)

const tcpEstablishedState = "01"

// findTCPRemoteHost reads a /proc/net/tcp table: the remote address of an established socket among allSocketInode.
func findTCPRemoteHost(table string, allSocketInode map[string]bool) string {
	for _, line := range strings.Split(table, "\n") {
		allField := strings.Fields(line)
		if len(allField) < 10 || allField[3] != tcpEstablishedState || !allSocketInode[allField[9]] {
			continue
		}
		hexAddress, _, _ := strings.Cut(allField[2], ":")
		raw, err := hex.DecodeString(hexAddress)
		if err != nil || (len(raw) != net.IPv4len && len(raw) != net.IPv6len) {
			continue
		}
		for start := 0; start < len(raw); start += 4 {
			raw[start], raw[start+1], raw[start+2], raw[start+3] = raw[start+3], raw[start+2], raw[start+1], raw[start]
		}
		address := net.IP(raw)
		if mapped := address.To4(); mapped != nil {
			address = mapped
		}
		return address.String()
	}
	return ""
}

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
