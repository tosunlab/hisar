package desktopupdate

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func escapeXML(text string) string {
	var buffer bytes.Buffer
	xml.EscapeText(&buffer, []byte(text))
	return buffer.String()
}

// BuildDownloadURL is where the app downloads a release file: the Hisar server, not GitHub.
func BuildDownloadURL(publicURL, version, fileName string) string {
	return strings.TrimRight(publicURL, "/") + "/api/v1/desktop/updates/" + url.PathEscape(version) + "/" + url.PathEscape(fileName)
}

// BuildAppcast writes a Sparkle appcast; allRelease is newest first.
func BuildAppcast(publicURL string, allRelease []*Release) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	buffer.WriteString(`<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">` + "\n")
	buffer.WriteString("<channel>\n<title>Hisar for macOS</title>\n")
	for _, release := range allRelease {
		buffer.WriteString("<item>\n")
		fmt.Fprintf(&buffer, "<title>%s</title>\n", escapeXML("Hisar "+release.Version))
		fmt.Fprintf(&buffer, "<pubDate>%s</pubDate>\n", release.PublishedAt.UTC().Format(time.RFC1123Z))
		fmt.Fprintf(&buffer, "<sparkle:version>%d</sparkle:version>\n", release.BuildNumber)
		fmt.Fprintf(&buffer, "<sparkle:shortVersionString>%s</sparkle:shortVersionString>\n", escapeXML(release.Version))
		fmt.Fprintf(&buffer, "<sparkle:minimumSystemVersion>%s</sparkle:minimumSystemVersion>\n", escapeXML(release.MinimumSystemVersion))
		if release.ReleaseNotes != "" {
			fmt.Fprintf(&buffer, "<description sparkle:format=\"%s\">%s</description>\n", escapeXML(release.ReleaseNotesFormat), escapeXML(release.ReleaseNotes))
		}
		fmt.Fprintf(&buffer, "<enclosure url=\"%s\" length=\"%d\" type=\"application/octet-stream\" sparkle:edSignature=\"%s\"/>\n",
			escapeXML(BuildDownloadURL(publicURL, release.Version, release.Archive)), release.Length, escapeXML(release.EdSignature))
		buffer.WriteString("</item>\n")
	}
	buffer.WriteString("</channel>\n</rss>\n")
	return buffer.Bytes()
}
