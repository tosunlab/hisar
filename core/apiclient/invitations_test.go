package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestShareInvitationRequests(t *testing.T) {
	var requests []string
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/api/v1/vaults/vault-1/share-invitations":
			var shareRequest apitypes.ShareInvitationRequest
			if err := json.NewDecoder(request.Body).Decode(&shareRequest); err != nil || shareRequest.Email != "bob@example.com" || shareRequest.Role != apitypes.VaultReader {
				t.Errorf("share request: %+v %v", shareRequest, err)
			}
			writer.WriteHeader(http.StatusAccepted)
			json.NewEncoder(writer).Encode(apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage})
		case "/api/v1/share-invitations":
			json.NewEncoder(writer).Encode(apitypes.ShareInvitationList{Received: []apitypes.ReceivedShareInvitation{{ID: "invitation-1"}}})
		case "/api/v1/invites":
			var inviteRequest apitypes.InviteRequest
			if err := json.NewDecoder(request.Body).Decode(&inviteRequest); err != nil || inviteRequest.Email != "carol@example.com" || inviteRequest.Role != "" {
				t.Errorf("invite request: %+v %v", inviteRequest, err)
			}
			writer.WriteHeader(http.StatusAccepted)
			json.NewEncoder(writer).Encode(apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage})
		default:
			writer.WriteHeader(http.StatusNoContent)
		}
	})
	ctx := context.Background()
	if response, err := fakeClient.CreateShareInvitation(ctx, "vault-1", &apitypes.ShareInvitationRequest{Email: "bob@example.com", Role: apitypes.VaultReader}); err != nil || response.Message != apitypes.InvitationSentMessage {
		t.Fatalf("create: %+v %v", response, err)
	}
	if invitationList, err := fakeClient.ListShareInvitations(ctx); err != nil || len(invitationList.Received) != 1 || invitationList.Received[0].ID != "invitation-1" {
		t.Fatalf("list: %+v %v", invitationList, err)
	}
	for _, send := range []func(context.Context, string) error{fakeClient.AcceptShareInvitation, fakeClient.DeclineShareInvitation, fakeClient.CancelShareInvitation} {
		if err := send(ctx, "invitation-1"); err != nil {
			t.Fatal(err)
		}
	}
	if response, err := fakeClient.Invite(ctx, &apitypes.InviteRequest{Email: "carol@example.com"}); err != nil || response.Message != apitypes.InvitationSentMessage {
		t.Fatalf("invite: %+v %v", response, err)
	}
	expected := []string{
		"POST /api/v1/vaults/vault-1/share-invitations",
		"GET /api/v1/share-invitations",
		"POST /api/v1/share-invitations/invitation-1/accept",
		"POST /api/v1/share-invitations/invitation-1/decline",
		"DELETE /api/v1/share-invitations/invitation-1",
		"POST /api/v1/invites",
	}
	if strings.Join(requests, "\n") != strings.Join(expected, "\n") {
		t.Errorf("requests:\n%s", strings.Join(requests, "\n"))
	}
}
