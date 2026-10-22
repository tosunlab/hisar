package appinfo

import (
	"encoding/hex"
	"net"
	"strings"
)

const tcpEstablishedState = "01"

// findTCPRemoteHost reads a /proc/net/tcp table: the remote address of an established socket among socketInodes.
func findTCPRemoteHost(table string, socketInodes map[string]bool) string {
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[3] != tcpEstablishedState || !socketInodes[fields[9]] {
			continue
		}
		hexAddress, _, _ := strings.Cut(fields[2], ":")
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
