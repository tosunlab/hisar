package api

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	downloadURL       = "https://tryhisar.com/download"
	inviteDeepLinkURL = "hisar://invite/"
)

var inviteCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

//go:embed invitepage/page.html invitepage/page.css invitepage/page.js
var invitePageFiles embed.FS

type invitePageTemplate struct {
	template              *template.Template
	style                 string
	script                string
	contentSecurityPolicy string
}

type invitePageData struct {
	IsValid            bool
	InviterName        string
	InviterDescription string
	Email              string
	ExpiresOn          string
	Token              string
	InviteLink         string
	DeepLink           template.URL
	DownloadURL        string
}

var builtInvitePage = buildInvitePage()

func readInvitePageFile(name string) string {
	content, err := invitePageFiles.ReadFile("invitepage/" + name)
	if err != nil {
		panic(err)
	}
	return string(content)
}

func buildInvitePage() invitePageTemplate {
	style, script := readInvitePageFile("page.css"), readInvitePageFile("page.js")
	return invitePageTemplate{
		template: template.Must(template.New("invite").Parse(readInvitePageFile("page.html"))),
		style:    style,
		script:   script,
		contentSecurityPolicy: "default-src 'none'; script-src " + hashInlineContent(script) + "; style-src " + hashInlineContent(style) +
			"; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
	}
}

// render inlines the style before any data and the script after it, so text from users can never become either.
func (page invitePageTemplate) render(data invitePageData) ([]byte, error) {
	var buffer bytes.Buffer
	if err := page.template.Execute(&buffer, data); err != nil {
		return nil, err
	}
	body := strings.Replace(buffer.String(), "@@style@@", page.style, 1)
	scriptIndex := strings.LastIndex(body, "@@script@@")
	if scriptIndex < 0 {
		return nil, errors.New("invite page: no script placeholder")
	}
	return []byte(body[:scriptIndex] + page.script + body[scriptIndex+len("@@script@@"):]), nil
}

func createInvitePageLimiter() *ipLimiter { return createIPLimiter(rate.Every(2*time.Second), 30) }

func (s *Server) limitInvitePageRate(c fiber.Ctx) error {
	if !s.invitePageLimiter.allow(s.getClientIP(c)) {
		c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
		return c.Status(fiber.StatusTooManyRequests).SendString("Too many requests. Wait a minute and try again.\n")
	}
	return c.Next()
}

// buildPublicOrigin uses https with the public host; plain http stays only for a loopback host in development.
func (s *Server) buildPublicOrigin() string {
	parsedURL, err := url.Parse(s.config.PublicURL)
	if err != nil || parsedURL.Host == "" {
		return strings.TrimRight(s.config.PublicURL, "/")
	}
	scheme := "https"
	if parsedURL.Scheme == "http" && mail.IsLoopbackHost(parsedURL.Hostname()) {
		scheme = "http"
	}
	return scheme + "://" + parsedURL.Host
}

func (s *Server) buildInviteDeepLink(token string) string {
	return inviteDeepLinkURL + url.PathEscape(token) + "?" + url.Values{"server": {s.buildPublicOrigin()}}.Encode()
}

// sendInvitePage answers unknown, used and expired invites with the same page and status, so it reveals nothing about them.
func (s *Server) sendInvitePage(c fiber.Ctx) error {
	token := c.Params("token")
	data := invitePageData{}
	status := fiber.StatusNotFound
	if inviteCodePattern.MatchString(token) {
		invite, err := s.store.GetInviteWithInviter(c.Context(), hashToken(token))
		switch {
		case err == nil:
			status = fiber.StatusOK
			data = invitePageData{
				IsValid:     true,
				Email:       invite.Email,
				ExpiresOn:   formatExpiry(invite.ExpiresAt),
				Token:       token,
				InviteLink:  s.buildInviteLink(token),
				DeepLink:    template.URL(s.buildInviteDeepLink(token)),
				DownloadURL: downloadURL,
			}
			if invite.InviterEmail != "" {
				inviter := &store.Account{Name: invite.InviterName, Email: invite.InviterEmail}
				data.InviterName, data.InviterDescription = getInviterName(inviter), describeInviter(inviter)
			}
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
	}
	body, err := builtInvitePage.render(data)
	if err != nil {
		return err
	}
	c.Set("Content-Security-Policy", builtInvitePage.contentSecurityPolicy)
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Cross-Origin-Opener-Policy", "same-origin")
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Status(status).Send(body)
}
