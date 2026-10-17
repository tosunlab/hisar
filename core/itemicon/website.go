package itemicon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

const (
	maxPageSize       = 512 << 10
	maxIconSize       = 1 << 20
	maxRedirects      = 3
	maxIconCandidates = 4
	requestTimeout    = 5 * time.Second
	fetchTimeout      = 10 * time.Second
)

var (
	// ErrRefusedURL is returned for a URL the fetcher never asks: not https, or not a public domain name.
	ErrRefusedURL = errors.New("itemicon: not a public https website")
	// ErrNoIcon is returned when the website has no icon the fetcher can read.
	ErrNoIcon = errors.New("itemicon: the website has no usable icon")
)

// Fetcher fetches website icons straight from the websites, never through a third party, and only from public
// addresses: it refuses IP literals and local names, and checks each address it connects to, so a name that resolves
// to a loopback or private address never reaches the local network.
type Fetcher struct {
	client *http.Client
}

// NewFetcher returns a fetcher that dials only public addresses.
func NewFetcher() *Fetcher {
	dialer := &net.Dialer{Timeout: requestTimeout, Control: refusePrivateAddress}
	transport := &http.Transport{
		// No proxy: the address check must see the website's own address.
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   requestTimeout,
		ResponseHeaderTimeout: requestTimeout,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return newFetcher(transport)
}

func newFetcher(transport http.RoundTripper) *Fetcher {
	return &Fetcher{client: &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("itemicon: too many redirects")
			}
			if _, err := checkURL(request.URL); err != nil {
				return err
			}
			return nil
		},
	}}
}

// Domain returns the lower-case host of an https URL whose host is a public domain name, or ErrRefusedURL.
func Domain(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", ErrRefusedURL
	}
	return checkURL(parsed)
}

func checkURL(parsed *url.URL) (string, error) {
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if parsed.Scheme != "https" || parsed.User != nil || host == "" || !strings.Contains(host, ".") || len(host) > 253 {
		return "", ErrRefusedURL
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return "", ErrRefusedURL
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa", ".arpa"} {
		if strings.HasSuffix("."+host, suffix) {
			return "", ErrRefusedURL
		}
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.Trim(label, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return "", ErrRefusedURL
		}
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", ErrRefusedURL
	}
	return host, nil
}

// refusePrivateAddress refuses connections to addresses that are not public unicast ones.
func refusePrivateAddress(_, address string, _ syscall.RawConn) error {
	addressPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("itemicon: address %q: %w", address, err)
	}
	if !isPublicAddress(addressPort.Addr()) {
		return ErrRefusedURL
	}
	return nil
}

var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsGlobalUnicast() && !address.IsPrivate() && !sharedAddressSpace.Contains(address)
}

// Fetch returns the icon of the website rawURL names, normalized like NormalizeImage. It reads the page's
// apple-touch-icon and icon links, largest first, and falls back to /favicon.ico.
func (fetcher *Fetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	domain, err := Domain(rawURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	home := &url.URL{Scheme: "https", Host: domain, Path: "/"}
	candidates, pageURL := fetcher.findIconLinks(ctx, home)
	candidates = append(candidates, pageURL.ResolveReference(&url.URL{Path: "/favicon.ico"}).String())
	tried := []string{}
	for _, candidate := range candidates {
		if len(tried) == maxIconCandidates || ctx.Err() != nil {
			break
		}
		if slices.Contains(tried, candidate) {
			continue
		}
		tried = append(tried, candidate)
		if icon, err := fetcher.fetchImage(ctx, candidate); err == nil {
			return icon, nil
		}
	}
	return nil, ErrNoIcon
}

// findIconLinks reads the home page's head and returns its icon links, best first, and the page's URL after redirects.
func (fetcher *Fetcher) findIconLinks(ctx context.Context, home *url.URL) ([]string, *url.URL) {
	response, err := fetcher.get(ctx, home.String(), "text/html")
	if err != nil {
		return nil, home
	}
	defer response.Body.Close()
	pageURL := response.Request.URL
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "html") {
		return nil, pageURL
	}
	links := parseIconLinks(io.LimitReader(response.Body, maxPageSize))
	candidates := []string{}
	for _, link := range links {
		resolved, err := pageURL.Parse(link.href)
		if err != nil {
			continue
		}
		if _, err := checkURL(resolved); err == nil {
			candidates = append(candidates, resolved.String())
		}
	}
	return candidates, pageURL
}

func (fetcher *Fetcher) fetchImage(ctx context.Context, iconURL string) ([]byte, error) {
	response, err := fetcher.get(ctx, iconURL, "image/*")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itemicon: %s: status %d", iconURL, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxIconSize+1))
	if err != nil {
		return nil, fmt.Errorf("itemicon: %s: %w", iconURL, err)
	}
	if len(data) > maxIconSize {
		return nil, fmt.Errorf("itemicon: %s: too large", iconURL)
	}
	return NormalizeImage(data)
}

func (fetcher *Fetcher) get(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Hisar")
	request.Header.Set("Accept", accept)
	return fetcher.client.Do(request)
}

type iconLink struct {
	href  string
	score int
}

// parseIconLinks returns the icon links of an HTML head, best first: the largest declared size, with an
// apple-touch-icon without sizes counted as 180 and an icon without sizes as 32. SVG and data: links are skipped.
func parseIconLinks(page io.Reader) []iconLink {
	tokenizer := html.NewTokenizer(page)
	links := []iconLink{}
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return sortIconLinks(links)
		case html.EndTagToken:
			if name, _ := tokenizer.TagName(); string(name) == "head" {
				return sortIconLinks(links)
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttributes := tokenizer.TagName()
			if string(name) == "body" {
				return sortIconLinks(links)
			}
			if string(name) != "link" || !hasAttributes {
				continue
			}
			attributes := map[string]string{}
			for {
				key, value, isMore := tokenizer.TagAttr()
				attributes[string(key)] = string(value)
				if !isMore {
					break
				}
			}
			if link, isIcon := readIconLink(attributes); isIcon {
				links = append(links, link)
			}
		}
	}
}

func readIconLink(attributes map[string]string) (iconLink, bool) {
	href := strings.TrimSpace(attributes["href"])
	lowerHref := strings.ToLower(href)
	if href == "" || strings.HasPrefix(lowerHref, "data:") || strings.Contains(attributes["type"], "svg") ||
		strings.HasSuffix(strings.SplitN(lowerHref, "?", 2)[0], ".svg") {
		return iconLink{}, false
	}
	relations := strings.Fields(strings.ToLower(attributes["rel"]))
	isTouchIcon := slices.Contains(relations, "apple-touch-icon") || slices.Contains(relations, "apple-touch-icon-precomposed")
	if !isTouchIcon && !slices.Contains(relations, "icon") {
		return iconLink{}, false
	}
	score := 32
	if isTouchIcon {
		score = 180
	}
	for _, size := range strings.Fields(strings.ToLower(attributes["sizes"])) {
		width, _, isFound := strings.Cut(size, "x")
		if parsed, err := strconv.Atoi(width); isFound && err == nil && parsed > 0 {
			score = max(score, parsed)
		}
	}
	return iconLink{href: href, score: score}, true
}

func sortIconLinks(links []iconLink) []iconLink {
	slices.SortStableFunc(links, func(first, second iconLink) int { return second.score - first.score })
	return links
}
