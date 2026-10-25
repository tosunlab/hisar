package publicnet

import (
	"errors"
	"net/netip"
	"testing"
)

func TestIsPublicAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"140.82.112.3": true, "2606:4700::6810:84e5": true, "127.0.0.1": false, "10.0.0.1": false, "192.168.1.1": false,
		"172.16.5.4": false, "169.254.169.254": false, "100.64.1.1": false, "::1": false, "fe80::1": false, "fd00::1": false,
		"0.0.0.0": false, "::ffff:127.0.0.1": false, "224.0.0.1": false,
		// NAT64 and 6to4 carry an IPv4 address, which decides.
		"64:ff9b::a00:1": false, "64:ff9b::7f00:1": false, "64:ff9b::8c52:7003": true,
		"2002:c0a8:1::": false, "2002:a9fe:a9fe::1": false, "2002:8c52:7003::1": true,
		"64:ff9b:1::8c52:7003": false, "2001:0:4136:e378:8000:63bf:3fff:fdd2": false,
		// Special-purpose ranges that are not reachable on the internet: this network, IETF protocol assignments,
		// documentation, benchmarking, reserved, IPv4-compatible IPv6, discard-only and SRv6.
		"0.1.2.3": false, "192.0.0.8": false, "192.0.2.1": false, "198.18.0.1": false, "198.51.100.7": false, "203.0.113.9": false,
		"240.0.0.1": false, "255.255.255.255": false, "2001:db8::1": false, "3fff::1": false, "2001:2::1": false, "100::1": false,
		"::808:808": false, "5f00::1": false, "2a00:1450:4001:81d::200e": true,
	} {
		if got := IsPublicAddress(netip.MustParseAddr(address)); got != want {
			t.Errorf("IsPublicAddress(%s) = %v", address, got)
		}
	}
}

func TestRefuseNonPublic(t *testing.T) {
	for _, address := range []string{"127.0.0.1:443", "[::1]:8080", "10.1.2.3:443", "[64:ff9b::a00:1]:443"} {
		if err := RefuseNonPublic("tcp", address, nil); !errors.Is(err, ErrNotPublic) {
			t.Errorf("dialing %s: %v", address, err)
		}
	}
	if err := RefuseNonPublic("tcp", "140.82.112.3:443", nil); err != nil {
		t.Errorf("a public address was refused: %v", err)
	}
}
