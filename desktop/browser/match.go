package browser

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// IsURLMatch reports whether an item URL may be filled into a page.
func IsURLMatch(itemURL, pageURL string, isExactHost bool) bool {
	page, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || (page.Scheme != "https" && page.Scheme != "http") || page.Hostname() == "" {
		return false
	}
	itemURL = strings.TrimSpace(itemURL)
	if !strings.Contains(itemURL, "://") {
		itemURL = "https://" + itemURL
	}
	saved, err := url.Parse(itemURL)
	if err != nil || saved.Hostname() == "" {
		return false
	}
	if saved.Scheme != page.Scheme && !(saved.Scheme == "http" && page.Scheme == "https") {
		return false
	}
	if getPort(saved) != getPort(page) {
		return false
	}
	savedHost, pageHost := normalizeHost(saved.Hostname()), normalizeHost(page.Hostname())
	if savedHost == pageHost {
		return true
	}
	if isExactHost || net.ParseIP(savedHost) != nil || net.ParseIP(pageHost) != nil || savedHost == "localhost" || pageHost == "localhost" {
		return false
	}
	savedSite, savedErr := publicsuffix.EffectiveTLDPlusOne(savedHost)
	pageSite, pageErr := publicsuffix.EffectiveTLDPlusOne(pageHost)
	return savedErr == nil && pageErr == nil && savedSite == pageSite
}

// GetSiteName is the host without a leading "www.", used as the title of saved logins.
func GetSiteName(pageURL string) string {
	page, err := url.Parse(pageURL)
	if err != nil {
		return pageURL
	}
	return strings.TrimPrefix(normalizeHost(page.Hostname()), "www.")
}

// GetOrigin returns scheme://host[:port] of a page URL.
func GetOrigin(pageURL string) string {
	page, err := url.Parse(pageURL)
	if err != nil {
		return pageURL
	}
	return page.Scheme + "://" + page.Host
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

func getPort(parsedURL *url.URL) string {
	port := parsedURL.Port()
	if (parsedURL.Scheme == "https" && port == "443") || (parsedURL.Scheme == "http" && port == "80") {
		return ""
	}
	return port
}
