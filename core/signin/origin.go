package signin

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

var errNotWebsite = errors.New("signin: not the address of a website")

var defaultPorts = map[string]string{"http": "80", "https": "443"}

// Origin is the scheme, host and port of an http or https page, in lower case and without the scheme's default port.
// ParseOrigin is the only way to make one, so an Origin never stands for about:, data:, blob:, file: or a srcdoc page,
// and two Origins are the same website exactly when they are equal.
type Origin struct {
	scheme string
	// host is ASCII, an internationalized name in punycode, and an IPv6 address keeps its brackets.
	host string
	port string
}

// ParseOrigin reads the origin of a page's URL as its browser reports it.
func ParseOrigin(rawURL string) (Origin, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Origin{}, errNotWebsite
	}
	scheme := strings.ToLower(parsed.Scheme)
	defaultPort, isWeb := defaultPorts[scheme]
	if !isWeb || parsed.Opaque != "" || parsed.Hostname() == "" {
		return Origin{}, errNotWebsite
	}
	host := strings.ToLower(parsed.Hostname())
	if address := net.ParseIP(host); address != nil && address.To4() == nil {
		host = "[" + address.String() + "]"
	} else if address == nil {
		if host, err = idna.Lookup.ToASCII(host); err != nil {
			return Origin{}, errNotWebsite
		}
	}
	port := parsed.Port()
	if port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return Origin{}, errNotWebsite
		}
		port = strconv.Itoa(number)
	}
	if port == defaultPort {
		port = ""
	}
	return Origin{scheme: scheme, host: host, port: port}, nil
}

// String is scheme://host[:port], as the approval and the agent see it.
func (origin Origin) String() string {
	if origin.port == "" {
		return origin.scheme + "://" + origin.host
	}
	return origin.scheme + "://" + origin.host + ":" + origin.port
}

func (origin Origin) isSecure() bool {
	if origin.scheme == "https" {
		return true
	}
	host := strings.Trim(origin.host, "[]")
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
