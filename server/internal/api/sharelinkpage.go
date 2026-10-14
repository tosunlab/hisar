package api

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v3"
)

//go:embed sharelinkpage/page.html sharelinkpage/page.css sharelinkpage/page.js
var shareLinkPageFiles embed.FS

type shareLinkPage struct {
	body                  []byte
	contentSecurityPolicy string
}

var builtShareLinkPage = buildShareLinkPage()

func readShareLinkPageFile(name string) string {
	content, err := shareLinkPageFiles.ReadFile("sharelinkpage/" + name)
	if err != nil {
		panic(err)
	}
	return string(content)
}

func hashInlineContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
}

// buildShareLinkPage inlines the script and style and allows exactly them by hash.
func buildShareLinkPage() shareLinkPage {
	style, script := readShareLinkPageFile("page.css"), readShareLinkPageFile("page.js")
	body := strings.NewReplacer("{{style}}", style, "{{script}}", script).Replace(readShareLinkPageFile("page.html"))
	return shareLinkPage{
		body: []byte(body),
		contentSecurityPolicy: "default-src 'none'; script-src " + hashInlineContent(script) + "; style-src " + hashInlineContent(style) +
			"; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
	}
}

// sendShareLinkPage sends the same bytes for every ID: it never looks a link up or counts a view.
func sendShareLinkPage(c fiber.Ctx) error {
	c.Set("Content-Security-Policy", builtShareLinkPage.contentSecurityPolicy)
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Cross-Origin-Opener-Policy", "same-origin")
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Send(builtShareLinkPage.body)
}
