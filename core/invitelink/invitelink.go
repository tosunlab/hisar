// Package invitelink parses the sign-up invite links the server emails: https://<server>/invite/<token>.
package invitelink

import (
	"net/url"
	"strings"
)

const PathPrefix = "/invite/"

// Parse returns the server origin and the token of an invite link; ok is false for anything else.
func Parse(link string) (server, token string, ok bool) {
	parsedURL, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return "", "", false
	}
	token, found := strings.CutPrefix(parsedURL.Path, PathPrefix)
	if !found || token == "" || strings.Contains(token, "/") {
		return "", "", false
	}
	return parsedURL.Scheme + "://" + parsedURL.Host, token, true
}
