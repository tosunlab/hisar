package commandline

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

type shareLinkDescribingHandler struct{ *fakeHandler }

func (shareLinkDescribingHandler) DescribeShareLink(request *cliprotocol.Request) (string, []string) {
	return "GitHub", []string{"password", "Recovery code"}
}

func createShareLinkRequest() cliprotocol.Request {
	return cliprotocol.Request{
		Type: cliprotocol.TypeCreateShareLink, Vault: "Personal", Query: "git",
		ShareLink: &cliprotocol.ShareLinkChange{Fields: []string{"password", "recovery code"}, ExpiresInSeconds: 604800, MaxViews: 1, Passphrase: "correct horse"},
	}
}

func TestShareLinkAlwaysAsksAndApprovesTheSession(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}); !response.OK {
		t.Fatalf("vaults.list: %+v", response.Error)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if response := client.send(t, createShareLinkRequest()); !response.OK {
			t.Fatalf("shareLinks.create: %+v", response.Error)
		}
		if count := fixture.approver.requestCount.Load(); count != int32(1+attempt) {
			t.Fatalf("after %d share links in an approved session: %d approvals", attempt, count)
		}
	}
	request := fixture.approver.allRequest[1]
	if request.ShareLink == nil || request.ShareLink.Title != "git" || !slices.Equal(request.ShareLink.AllFieldLabel, []string{"password", "recovery code"}) ||
		request.ShareLink.Detail != "Expires in 7 days · 1 view · passphrase" || request.IsSecretRequest {
		t.Errorf("approval request %+v %+v", request, request.ShareLink)
	}
	if request := fixture.approver.allRequest[0]; request.ShareLink != nil {
		t.Errorf("vaults.list described a share link: %+v", request.ShareLink)
	}
}

func TestShareLinkApprovalAlsoApprovesTheSession(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	client.send(t, createShareLinkRequest())
	for _, requestType := range []string{cliprotocol.TypeListShareLink, cliprotocol.TypeRevokeShareLink, cliprotocol.TypeListVault} {
		if response := client.send(t, cliprotocol.Request{Type: requestType, ShareLinkID: "AbCdEfGhIjKlMnOpQrStUv"}); !response.OK {
			t.Fatalf("%s: %+v", requestType, response.Error)
		}
	}
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Errorf("%d approvals; the share link's approval should cover the session", count)
	}
}

func TestDeniedShareLinkIsNotCreatedAndApprovesNothing(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.approver.result = ErrDenied
	client := fixture.connect(t)
	if response := client.send(t, createShareLinkRequest()); getErrorCode(response) != cliprotocol.ErrorDenied {
		t.Fatalf("denied share link: %+v", response)
	}
	if fixture.approvals.IsApproved(fixture.resolver.session) || slices.Contains(fixture.handler.allType, cliprotocol.TypeCreateShareLink) {
		t.Error("a denied share link approved the session or reached the handler")
	}
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeCreateShareLink}); getErrorCode(response) != cliprotocol.ErrorInvalidRequest {
		t.Errorf("share link without details: %+v", response)
	}
}

func TestShareLinkApprovalUsesTheHandlersNames(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.server.config.Handler = shareLinkDescribingHandler{fixture.handler}
	client := fixture.connect(t)
	client.send(t, createShareLinkRequest())
	textRequest := cliprotocol.Request{Type: cliprotocol.TypeCreateShareLink, ShareLink: &cliprotocol.ShareLinkChange{Text: "hello", ExpiresInSeconds: 3600, MaxViews: 0}}
	client.send(t, textRequest)
	first, second := fixture.approver.allRequest[0].ShareLink, fixture.approver.allRequest[1].ShareLink
	if first.Title != "GitHub" || !slices.Equal(first.AllFieldLabel, []string{"password", "Recovery code"}) {
		t.Errorf("item description %+v", first)
	}
	if second.Title != "Shared text" || !slices.Equal(second.AllFieldLabel, []string{"text"}) || second.Detail != "Expires in 1 hour · unlimited views" {
		t.Errorf("text description %+v", second)
	}
}
