package item

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// AndroidApp is an Android app a login fills into: its package name and the SHA-256 of its signing certificate. The
// package name alone is not enough, since a sideloaded app can take any package name.
//
// It is stored among the item's URLs as androidapp://<package>?sha256=<hex>, so an app that predates it keeps it when it
// edits the login, and no app fills it into a web page: only http and https URLs match pages.
type AndroidApp struct {
	PackageName       string
	CertificateSHA256 [sha256.Size]byte
}

var ErrInvalidAndroidApp = errors.New("item: not an Android package name and signing certificate SHA-256")

var androidPackageName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

// ParseAndroidApp reads a package name and the hex SHA-256 of its signing certificate, in either case and with or without
// colons between the bytes, as keytool and assetlinks.json write it.
func ParseAndroidApp(packageName, certificateSHA256 string) (AndroidApp, error) {
	digest, err := hex.DecodeString(strings.ReplaceAll(certificateSHA256, ":", ""))
	if !androidPackageName.MatchString(packageName) || err != nil || len(digest) != sha256.Size {
		return AndroidApp{}, ErrInvalidAndroidApp
	}
	app := AndroidApp{PackageName: packageName}
	copy(app.CertificateSHA256[:], digest)
	return app, nil
}

// parseAndroidAppURL reads an item URL that holds an AndroidApp; any other URL, an androidapp:// one without a valid
// certificate included, is a website.
func parseAndroidAppURL(link string) (AndroidApp, bool) {
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil || parsed.Scheme != "androidapp" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.Fragment != "" {
		return AndroidApp{}, false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 1 || len(query["sha256"]) != 1 {
		return AndroidApp{}, false
	}
	app, err := ParseAndroidApp(parsed.Host, query.Get("sha256"))
	return app, err == nil
}

// URL is the app as the item stores it among its URLs.
func (app AndroidApp) URL() string {
	return "androidapp://" + app.PackageName + "?sha256=" + hex.EncodeToString(app.CertificateSHA256[:])
}

// CertificateFingerprint is the certificate's SHA-256 as apksigner and assetlinks.json show it: upper-case hex bytes
// joined by colons.
func (app AndroidApp) CertificateFingerprint() string {
	pairs := make([]string, len(app.CertificateSHA256))
	for index, value := range app.CertificateSHA256 {
		pairs[index] = strings.ToUpper(hex.EncodeToString([]byte{value}))
	}
	return strings.Join(pairs, ":")
}

// AndroidApps returns the Android apps the item fills into, in the order it stores them.
func (vaultItem *Item) AndroidApps() []AndroidApp {
	var apps []AndroidApp
	for _, link := range vaultItem.Overview.URLs {
		if app, isApp := parseAndroidAppURL(link); isApp && !slices.Contains(apps, app) {
			apps = append(apps, app)
		}
	}
	return apps
}

// Websites returns the item's URLs that are not Android apps.
func (vaultItem *Item) Websites() []string {
	websites := []string{}
	for _, link := range vaultItem.Overview.URLs {
		if _, isApp := parseAndroidAppURL(link); !isApp {
			websites = append(websites, link)
		}
	}
	return websites
}

// IsForAndroidApp reports whether the item fills into app: its package name and its certificate both match.
func (vaultItem *Item) IsForAndroidApp(app AndroidApp) bool {
	return slices.Contains(vaultItem.AndroidApps(), app)
}

// AddAndroidApp adds app after the item's URLs and reports whether the item did not have it yet.
func (vaultItem *Item) AddAndroidApp(app AndroidApp) bool {
	if vaultItem.IsForAndroidApp(app) {
		return false
	}
	vaultItem.Overview.URLs = append(slices.Clone(vaultItem.Overview.URLs), app.URL())
	return true
}

// RemoveAndroidApp removes app from the item's URLs and reports whether the item had it.
func (vaultItem *Item) RemoveAndroidApp(app AndroidApp) bool {
	links := slices.DeleteFunc(slices.Clone(vaultItem.Overview.URLs), func(link string) bool {
		stored, isApp := parseAndroidAppURL(link)
		return isApp && stored == app
	})
	if len(links) == len(vaultItem.Overview.URLs) {
		return false
	}
	vaultItem.Overview.URLs = nil
	if len(links) > 0 {
		vaultItem.Overview.URLs = links
	}
	return true
}
