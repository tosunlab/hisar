// Package publicnet tells public internet addresses from loopback, private, link-local and special-purpose ones, so a
// connection to an address a user chose never reaches the local network.
package publicnet

import (
	"net/netip"
	"slices"
)

var (
	sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")
	// IPv6 ranges that carry an IPv4 address: NAT64's well-known prefix ends in it and 6to4 starts with it, so they are
	// judged by that address. Local-use NAT64 and Teredo (whose address is obfuscated) are refused outright.
	nat64WellKnown = netip.MustParsePrefix("64:ff9b::/96")
	nat64LocalUse  = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFour      = netip.MustParsePrefix("2002::/16")
	teredo         = netip.MustParsePrefix("2001::/32")
	// globalUnicastIPv6 is the only IPv6 space allocated for the internet.
	globalUnicastIPv6 = netip.MustParsePrefix("2000::/3")
	// specialPurposeRanges are the IANA special-purpose ranges no public service is on that IsGlobalUnicast and IsPrivate let
	// through: this network, IETF protocol assignments, documentation, benchmarking and reserved addresses.
	specialPurposeRanges = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("3fff::/20"),
	}
)

// IsPublicAddress reports whether address is a public unicast address on the internet.
func IsPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	switch {
	case nat64WellKnown.Contains(address):
		bytes := address.As16()
		return IsPublicAddress(netip.AddrFrom4([4]byte(bytes[12:16])))
	case sixToFour.Contains(address):
		bytes := address.As16()
		return IsPublicAddress(netip.AddrFrom4([4]byte(bytes[2:6])))
	case nat64LocalUse.Contains(address), teredo.Contains(address):
		return false
	case address.Is6() && !globalUnicastIPv6.Contains(address):
		return false
	}
	if slices.ContainsFunc(specialPurposeRanges, func(prefix netip.Prefix) bool { return prefix.Contains(address) }) {
		return false
	}
	return address.IsGlobalUnicast() && !address.IsPrivate() && !sharedAddressSpace.Contains(address)
}
