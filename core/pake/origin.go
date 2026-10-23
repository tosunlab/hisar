package pake

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/bytemare/opaque"
	"golang.org/x/net/idna"
)

// AuthPurpose separates login from checks made by an already signed-in session.
type AuthPurpose string

const (
	Login          AuthPurpose = "login"
	Reauthenticate AuthPurpose = "reauthenticate"
)

var errInvalidOrigin = errors.New("pake: use an http or https server URL without credentials, query, fragment, or ambiguous host spelling")

// CanonicalOrigin extracts an HTTP origin without changing the transport's path prefix.
func CanonicalOrigin(baseURL string) (string, error) {
	invalid := errInvalidOrigin
	u, err := url.Parse(baseURL)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(baseURL, "#") {
		return "", invalid
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", invalid
	}
	host := u.Hostname()
	if host == "" || strings.HasSuffix(host, ".") || strings.Contains(host, "%") {
		return "", invalid
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if address.Zone() != "" || address.Is4() && strings.HasPrefix(u.Host, "[") {
			return "", invalid
		}
		host = address.String()
	} else {
		if strings.ContainsAny(host, ":[]") {
			return "", invalid
		}
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || len(host) > 253 {
			return "", invalid
		}
		host = strings.ToLower(host)
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", invalid
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", invalid
				}
			}
		}
		// Numeric host spellings can be interpreted as IPv4 by other URL parsers.
		last := host[strings.LastIndex(host, ".")+1:]
		if _, err := strconv.ParseUint(last, 0, 64); err == nil {
			return "", invalid
		}
		if last[0] >= '0' && last[0] <= '9' {
			if _, err := strconv.ParseUint(last, 10, 64); err == nil {
				return "", invalid
			}
		}
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return "", invalid
	}
	if port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return "", invalid
		}
		port = strconv.FormatUint(value, 10)
		if scheme == "https" && port == "443" || scheme == "http" && port == "80" {
			port = ""
		}
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, nil
}

// NewAuthConfig binds KE2 and KE3 to the independently configured intended origin.
// Registration keeps NewConfig and ServerIdentity so existing records remain usable.
func NewAuthConfig(baseURL string, purpose AuthPurpose) (*opaque.Configuration, error) {
	if purpose != Login && purpose != Reauthenticate {
		return nil, errors.New("pake: invalid authentication purpose")
	}
	origin, err := CanonicalOrigin(baseURL)
	if err != nil {
		return nil, err
	}
	configuration := NewConfig()
	context := make([]byte, 0, 12+len("hisar/v2/opaque")+len(origin)+len(purpose))
	for _, field := range []string{"hisar/v2/opaque", origin, string(purpose)} {
		context = binary.BigEndian.AppendUint32(context, uint32(len(field)))
		context = append(context, field...)
	}
	configuration.Context = context
	return configuration, nil
}
