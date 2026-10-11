package vault

import (
	"errors"
	"net/url"
	"strings"
	"sync"
)

var errNotInviteLink = errors.New("not a Hisar invite link")

// ParseInviteDeepLink turns hisar://invite/<code>?server=<origin> into what Create account accepts:
// the invite link on that server, or the bare code for the default server.
func ParseInviteDeepLink(link string) (string, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.EqualFold(parsedURL.Scheme, "hisar") || !strings.EqualFold(parsedURL.Host, "invite") {
		return "", errNotInviteLink
	}
	code := strings.TrimPrefix(parsedURL.Path, "/")
	if !inviteCodePattern.MatchString(code) {
		return "", errNotInviteLink
	}
	serverText := parsedURL.Query().Get("server")
	if serverText == "" {
		return code, nil
	}
	server, err := resolveServer(serverText)
	if err != nil {
		return "", errNotInviteLink
	}
	if server == defaultServer {
		return code, nil
	}
	return server + "/invite/" + code, nil
}

// InviteLinkService hands an invite opened through hisar://invite/… to the create-account screen.
type InviteLinkService struct {
	mutex         sync.Mutex
	pendingInvite string
}

func CreateInviteLinkService() *InviteLinkService { return &InviteLinkService{} }

// OpenInviteLink keeps the invite until the UI takes it; not a method, so the UI cannot call it.
func OpenInviteLink(service *InviteLinkService, link string) (string, error) {
	invite, err := ParseInviteDeepLink(link)
	if err != nil {
		return "", err
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.pendingInvite = invite
	return invite, nil
}

// TakePendingInvite returns the invite from the last hisar://invite link once, or "".
func (service *InviteLinkService) TakePendingInvite() string {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	invite := service.pendingInvite
	service.pendingInvite = ""
	return invite
}
