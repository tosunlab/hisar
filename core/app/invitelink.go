package app

import (
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/tryhisar/hisar/core/apitypes"
)

var errNotInviteLink = errors.New("not a Hisar invite link")

// The invitation an invite link was sent for, as the invite page's Open in Hisar names it.
const (
	InviteForOrganization = "organization"
	InviteForShare        = "share"
)

// InviteLink is a hisar://invite link. Invite is what Create account accepts: the invite link on its server, or the
// bare code for the default server. Server is that server's origin. Invitation is InviteForOrganization or
// InviteForShare when an organization or a vault share invitation sent the link, and Email is then the address it is
// for: an app signed in to that account on that server shows the invitation instead.
type InviteLink struct {
	Invite     string `json:"invite"`
	Server     string `json:"server"`
	Invitation string `json:"invitation"`
	Email      string `json:"email"`
}

// ParseInviteDeepLink reads hisar://invite/<code>?server=<origin>[&invitation=organization|share&email=<address>].
func ParseInviteDeepLink(link string) (InviteLink, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.EqualFold(parsedURL.Scheme, "hisar") || !strings.EqualFold(parsedURL.Host, "invite") {
		return InviteLink{}, errNotInviteLink
	}
	code := strings.TrimPrefix(parsedURL.Path, "/")
	if !inviteCodePattern.MatchString(code) {
		return InviteLink{}, errNotInviteLink
	}
	query := parsedURL.Query()
	server := defaultServer
	if serverText := query.Get("server"); serverText != "" {
		if server, err = resolveServer(serverText); err != nil {
			return InviteLink{}, errNotInviteLink
		}
	}
	parsed := InviteLink{Invite: code, Server: server}
	if server != defaultServer {
		parsed.Invite = server + "/invite/" + code
	}
	// An unknown invitation is a plain invite, as in an older app.
	if invitation := query.Get("invitation"); invitation == InviteForOrganization || invitation == InviteForShare {
		parsed.Invitation, parsed.Email = invitation, apitypes.NormalizeEmail(query.Get("email"))
	}
	return parsed, nil
}

// InviteLinkService hands an invite opened through hisar://invite/… to the UI.
type InviteLinkService struct {
	mutex       sync.Mutex
	pendingLink *InviteLink
}

func NewInviteLinkService() *InviteLinkService { return &InviteLinkService{} }

// OpenInviteLink keeps the link until the UI takes it; not a method, so the UI cannot call it.
func OpenInviteLink(service *InviteLinkService, link string) (InviteLink, error) {
	parsed, err := ParseInviteDeepLink(link)
	if err != nil {
		return InviteLink{}, err
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.pendingLink = &parsed
	return parsed, nil
}

// TakePendingInvite returns the last hisar://invite link once, or nil.
func (service *InviteLinkService) TakePendingInvite() *InviteLink {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	link := service.pendingLink
	service.pendingLink = nil
	return link
}
