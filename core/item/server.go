package item

import (
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// CategoryServer items keep their username and password as the item's own login data and their admin console as its
// website, so references, the browser extension and AutoFill treat them as they do logins; the host, port and linked SSH
// key are custom fields with the names below, as an API credential's values are.
const CategoryServer = "server"

// The custom field names of a server's values; its host is FieldHost, as an API credential's. FieldSSHKey holds the ID of
// an ssh-key item in the same vault.
const (
	FieldPort   = "Port"
	FieldSSHKey = "SSH key"
)

// Server is what a server item holds besides its login data.
type Server struct {
	Host     string
	Port     string
	SSHKeyID string
}

// ServerField is one of a server's values.
type ServerField = NamedField[Server]

// ListServerFields returns a server's fields in the order they are stored and asked for.
func ListServerFields() []ServerField { return slices.Clone(serverFields) }

var serverFields = []ServerField{
	{FieldHost, false, func(server *Server) *string { return &server.Host }},
	{FieldPort, false, func(server *Server) *string { return &server.Port }},
	{FieldSSHKey, false, func(server *Server) *string { return &server.SSHKeyID }},
}

// ReadServer returns the server's values and the item's other custom fields, matching names ignoring case.
func ReadServer(vaultItem *Item) (Server, []Field) { return readRecord(vaultItem, serverFields) }

// WriteServer stores server as the item's first custom fields, in a fixed order, followed by otherFields; an empty value
// is left out.
func WriteServer(vaultItem *Item, server Server, otherFields []Field) {
	writeRecord(vaultItem, server, serverFields, otherFields)
}

// NormalizeServer trims a server item's values and puts its fields in order; other items are left alone.
func NormalizeServer(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryServer) {
		return
	}
	server, otherFields := ReadServer(vaultItem)
	WriteServer(vaultItem, server.Normalize(), otherFields)
}

// Normalize trims the server's values and splits a host typed with a scheme or port, such as ssh://build.example.com:2222
// or db.example.com:5432, into the host and an empty port. A link with a path, a bare IPv6 address and a port that is
// already set stay as typed.
func (server Server) Normalize() Server {
	server.Host, server.Port, server.SSHKeyID = strings.TrimSpace(server.Host), strings.TrimSpace(server.Port), strings.TrimSpace(server.SSHKeyID)
	if _, _, hasScheme := strings.Cut(server.Host, "://"); hasScheme {
		parsed, err := url.Parse(server.Host)
		if err != nil || parsed.Hostname() == "" || strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" {
			return server
		}
		server.Host = parsed.Hostname()
		if server.Port == "" {
			server.Port = parsed.Port()
		}
		return server
	}
	if server.Port != "" {
		return server
	}
	if host, port, err := net.SplitHostPort(server.Host); err == nil && host != "" && port != "" {
		server.Host, server.Port = host, port
	}
	return server
}

// PortWarning is what the editors show under a port that is not a number from 1 to 65535; it is "" for an empty port. The
// server saves anyway.
func (server Server) PortWarning() string {
	if server.Port == "" {
		return ""
	}
	if port, err := strconv.Atoi(server.Port); err != nil || port < 1 || port > 65535 {
		return "The port is not a number from 1 to 65535"
	}
	return ""
}

// Summary is the server's line in lists: its host and port, such as "build.example.com:2222", or the host alone.
func (server Server) Summary() string {
	if server.Host == "" || server.Port == "" {
		return server.Host
	}
	return net.JoinHostPort(server.Host, server.Port)
}

// OffersLoginFill reports whether the browser extension and AutoFill offer the item's username and password on its
// websites: a login's, and a server's on its admin console.
func (vaultItem *Item) OffersLoginFill() bool {
	return vaultItem.IsCategory(CategoryLogin) || vaultItem.IsCategory(CategoryServer)
}
