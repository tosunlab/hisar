package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

type rewriteReauthenticationScope struct {
	next     http.RoundTripper
	scope    apitypes.ReauthenticationScope
	requests int
}

func (relay *rewriteReauthenticationScope) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == "/api/v1/account/reauthenticate/v2" {
		encoded, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		request.Body.Close()
		var start apitypes.ReauthenticateRequest
		if err := json.Unmarshal(encoded, &start); err != nil {
			return nil, err
		}
		start.Scope = relay.scope
		encoded, err = json.Marshal(start)
		if err != nil {
			return nil, err
		}
		request.Body = io.NopCloser(bytes.NewReader(encoded))
		request.ContentLength = int64(len(encoded))
		request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil }
		relay.requests++
	}
	return relay.next.RoundTrip(request)
}

func TestDeletionScopeRejectsOuterJSONProofSubstitutionBeforeReturningKE3(t *testing.T) {
	baseURL, server, _, _ := setupLifecycleServer(t)
	ctx := context.Background()
	owner := signUpAccountWithRole(t, server, baseURL, "scoped-admin@example.com", apitypes.RoleAdmin)
	other := signUpLifecycleAccount(t, server, baseURL, "other@example.com")
	plan := reviewSessionDeletion(t, owner.session, ctx)
	deletion := apitypes.ReauthenticationScope{Action: "account-delete", Target: owner.session.AccountID, PlanVersion: plan}
	otherTarget := deletion
	otherTarget.Target = other.session.AccountID
	otherPlan := deletion
	otherPlan.PlanVersion = "1:" + strings.Repeat("a", 64)
	if otherPlan.PlanVersion == plan {
		otherPlan.PlanVersion = "1:" + strings.Repeat("b", 64)
	}
	otherAction := deletion
	otherAction.Action = "admin-account-delete"
	for _, test := range []struct {
		name                   string
		clientScope, wireScope apitypes.ReauthenticationScope
	}{
		{"generic to delete", apitypes.ReauthenticationScope{}, deletion},
		{"delete to generic", deletion, apitypes.ReauthenticationScope{}},
		{"another target", deletion, otherTarget},
		{"another reviewed plan", deletion, otherPlan},
		{"another action", deletion, otherAction},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := apiclient.New(baseURL)
			client.Token = owner.session.Client.Token
			relay := &rewriteReauthenticationScope{next: http.DefaultTransport, scope: test.wireScope}
			client.HTTP.Transport = relay
			proof, err := session.ReauthenticateForScope(ctx, client, owner.session.Email, owner.password, owner.secretKey, test.clientScope)
			if !errors.Is(err, session.ErrBadCredentials) || len(proof.KE3) != 0 || proof.ReauthenticationID != "" || relay.requests != 1 {
				t.Fatalf("outer JSON substitution produced a usable proof: err=%v KE3bytes=%d requests=%d", err, len(proof.KE3), relay.requests)
			}
		})
	}
	proof := requireValue[apitypes.ReauthenticationProof](t)(session.ReauthenticateForScope(ctx, owner.session.Client, owner.session.Email, owner.password, owner.secretKey, deletion))
	if status, body := sendRawRequest(t, baseURL, owner.session.Client.Token, "PUT", adminAccountsPath+"/"+other.session.AccountID+"/role", apitypes.AdminRoleChangeRequest{Role: apitypes.RoleAdmin, Proof: proof}); status != http.StatusForbidden {
		t.Fatalf("a deletion proof authorized a generic administrative mutation: %d %s", status, body)
	}
	if _, err := owner.session.Client.GetAccount(ctx); err != nil {
		t.Fatalf("refused relays changed the account: %v", err)
	}
}
