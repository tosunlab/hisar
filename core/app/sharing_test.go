package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

func (server *fakeAPIServer) getVaultRole(vaultID string) string {
	for _, access := range server.accessesByVaultID[vaultID] {
		if access.AccountID == server.accountID {
			return access.Role
		}
	}
	return ""
}

func (server *fakeAPIServer) addSharingRoutes(mux *http.ServeMux) {
	server.addVaultTransitionRoutes(mux)
	mux.HandleFunc("GET /api/v1/members", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		people := []apitypes.Member{}
		for _, member := range server.members {
			if member.AccountID == server.accountID || server.isSharingVaultWith(member.AccountID) {
				people = append(people, member)
			}
		}
		writeJSON(writer, http.StatusOK, people)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/signed-share-invitations", func(writer http.ResponseWriter, request *http.Request) {
		var shareRequest apitypes.ShareInvitationRequest
		if err := json.NewDecoder(request.Body).Decode(&shareRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		if shareRequest.Intent == nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "owner signature required"})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) != apitypes.VaultOwner {
			writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: "owner only"})
			return
		}
		for _, access := range server.accessesByVaultID[vaultID] {
			if member := server.findMember(access.AccountID); member != nil && member.Email == shareRequest.Email {
				writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "this person already has access to the vault"})
				return
			}
		}
		server.shareInvitations = slices.DeleteFunc(server.shareInvitations, func(invitation apitypes.SentShareInvitation) bool {
			return invitation.VaultID == vaultID && invitation.Email == shareRequest.Email
		})
		server.shareInvitations = append(server.shareInvitations, apitypes.SentShareInvitation{
			ID: shareRequest.Intent.Context.ID, VaultID: vaultID, Email: shareRequest.Email, Role: shareRequest.Role,
			State: apitypes.ShareInvitationPending, InvitedBy: server.accountID, ExpiresAt: time.Unix(shareRequest.Intent.Context.ExpiresAt, 0), Intent: shareRequest.Intent,
		})
		writeJSON(writer, http.StatusAccepted, apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage})
	})
	mux.HandleFunc("GET /api/v1/share-invitations", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		response := apitypes.ShareInvitationList{Received: append([]apitypes.ReceivedShareInvitation{}, server.receivedShareInvitations...), Sent: []apitypes.SentShareInvitation{},
			Waiting: server.waitingShareInvitations}
		for _, invitation := range server.shareInvitations {
			if server.getVaultRole(invitation.VaultID) == apitypes.VaultOwner {
				response.Sent = append(response.Sent, invitation)
			}
		}
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/share-invitations/{invitationID}/complete", func(writer http.ResponseWriter, request *http.Request) {
		var completion apitypes.CompleteShareRequest
		if err := json.NewDecoder(request.Body).Decode(&completion); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.isSetAccessFailing {
			writeJSON(writer, http.StatusInternalServerError, apitypes.Error{Error: "internal error"})
			return
		}
		intent := completion.Intent.Context
		index := slices.IndexFunc(server.shareInvitations, func(invitation apitypes.SentShareInvitation) bool {
			return invitation.ID == request.PathValue("invitationID") && invitation.State == apitypes.ShareInvitationAccepted
		})
		if index < 0 {
			writer.WriteHeader(http.StatusConflict)
			return
		}
		invitation := server.shareInvitations[index]
		signer := server.findMember(intent.InvitedBy)
		if signer == nil || invitation.Recipient == nil || intent.RecipientID != invitation.Recipient.AccountID || intent.Role != invitation.Role ||
			!bytes.Equal(intent.RecipientKeyHash, crypt.ShareRecipientKeyHash(invitation.Recipient.PublicKeys.Sign, invitation.Recipient.PublicKeys.KEM)) ||
			crypt.VerifyShareIntent(signer.PublicKeys.Sign, intent, completion.Intent.Signature) != nil {
			writer.WriteHeader(http.StatusConflict)
			return
		}
		server.setAccessRequests = append(server.setAccessRequests, apitypes.SetAccessRequest{WrappedKey: completion.WrappedKey, Role: intent.Role, KeyGeneration: intent.KeyGeneration})
		server.accessesByVaultID[intent.VaultID] = append(server.accessesByVaultID[intent.VaultID], apitypes.RotatedAccess{AccountID: intent.RecipientID, Role: intent.Role, WrappedKey: completion.WrappedKey})
		server.shareInvitations = slices.Delete(server.shareInvitations, index, index+1)
		for i := range server.vaults {
			if server.vaults[i].ID == intent.VaultID {
				server.vaults[i].AccessRevision++
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/share-invitations/{invitationID}/{answer}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		invitationID := request.PathValue("invitationID")
		index := slices.IndexFunc(server.receivedShareInvitations, func(invitation apitypes.ReceivedShareInvitation) bool { return invitation.ID == invitationID })
		if index < 0 {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "this invitation no longer exists"})
			return
		}
		server.receivedShareInvitations = slices.Delete(server.receivedShareInvitations, index, index+1)
		server.answeredShareInvitations = append(server.answeredShareInvitations, invitationID+" "+request.PathValue("answer"))
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/share-invitations/{invitationID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		invitationID := request.PathValue("invitationID")
		index := slices.IndexFunc(server.shareInvitations, func(invitation apitypes.SentShareInvitation) bool { return invitation.ID == invitationID })
		if index < 0 || server.getVaultRole(server.shareInvitations[index].VaultID) != apitypes.VaultOwner {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "this invitation no longer exists"})
			return
		}
		server.shareInvitations = slices.Delete(server.shareInvitations, index, index+1)
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/members", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) == "" {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "not found"})
			return
		}
		vaultMembers := []apitypes.VaultMember{}
		for _, access := range server.accessesByVaultID[vaultID] {
			vaultMembers = append(vaultMembers, apitypes.VaultMember{AccountID: access.AccountID, Role: access.Role})
		}
		writeJSON(writer, http.StatusOK, vaultMembers)
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/access/{accountID}", func(writer http.ResponseWriter, request *http.Request) {
		var setRequest apitypes.SetAccessRequest
		if err := json.NewDecoder(request.Body).Decode(&setRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID, accountID := request.PathValue("vaultID"), request.PathValue("accountID")
		if server.isSetAccessFailing {
			writeJSON(writer, http.StatusInternalServerError, apitypes.Error{Error: "internal error"})
			return
		}
		if server.getVaultRole(vaultID) != apitypes.VaultOwner {
			writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: "owner only"})
			return
		}
		if setRequest.KeyGeneration != 0 && setRequest.KeyGeneration != server.getKeyGeneration(vaultID) {
			writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "the vault key was rotated since"})
			return
		}
		server.setAccessRequests = append(server.setAccessRequests, setRequest)
		accesses := server.accessesByVaultID[vaultID]
		access := apitypes.RotatedAccess{AccountID: accountID, Role: setRequest.Role, WrappedKey: setRequest.WrappedKey}
		if index := slices.IndexFunc(accesses, func(existing apitypes.RotatedAccess) bool { return existing.AccountID == accountID }); index >= 0 {
			accesses[index] = access
		} else if keyRequestIndex := server.findVaultKeyRequest(vaultID, accountID); keyRequestIndex >= 0 {
			server.vaultKeyRequests = slices.Delete(server.vaultKeyRequests, keyRequestIndex, keyRequestIndex+1)
			server.accessesByVaultID[vaultID] = append(accesses, access)
		} else {
			invitationIndex := slices.IndexFunc(server.shareInvitations, func(invitation apitypes.SentShareInvitation) bool {
				return invitation.VaultID == vaultID && invitation.State == apitypes.ShareInvitationAccepted && invitation.Recipient != nil && invitation.Recipient.AccountID == accountID
			})
			if invitationIndex < 0 {
				writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "this person has not accepted an invitation to this vault"})
				return
			}
			server.shareInvitations = slices.Delete(server.shareInvitations, invitationIndex, invitationIndex+1)
			server.accessesByVaultID[vaultID] = append(accesses, access)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/invites", func(writer http.ResponseWriter, request *http.Request) {
		var inviteRequest apitypes.InviteRequest
		if err := json.NewDecoder(request.Body).Decode(&inviteRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.isEmailUnavailable {
			writeJSON(writer, http.StatusNotImplemented, apitypes.Error{Error: "email is not configured on this server"})
			return
		}
		if server.isInviteAdminsOnly && server.role != apitypes.RoleAdmin {
			writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: "Only admins can invite people to the Hisar server."})
			return
		}
		server.invitedEmails = append(server.invitedEmails, inviteRequest.Email)
		writeJSON(writer, http.StatusAccepted, apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage})
	})
	mux.HandleFunc("GET /api/v1/devices", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		hangingChannel := server.deviceListHangingChannel
		server.mutex.Unlock()
		if hangingChannel != nil {
			select {
			case <-hangingChannel:
			case <-request.Context().Done():
				return
			}
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, server.devices)
	})
	mux.HandleFunc("DELETE /api/v1/devices/{deviceID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		deviceID := request.PathValue("deviceID")
		server.devices = slices.DeleteFunc(server.devices, func(device apitypes.Device) bool { return device.ID == deviceID })
		writer.WriteHeader(http.StatusNoContent)
	})
}

func (server *fakeAPIServer) findMember(accountID string) *apitypes.Member {
	for index := range server.members {
		if server.members[index].AccountID == accountID {
			member := server.members[index]
			return &member
		}
	}
	return nil
}

func (server *fakeAPIServer) isSharingVaultWith(accountID string) bool {
	for _, accesses := range server.accessesByVaultID {
		isMine := slices.ContainsFunc(accesses, func(access apitypes.RotatedAccess) bool { return access.AccountID == server.accountID })
		if isMine && slices.ContainsFunc(accesses, func(access apitypes.RotatedAccess) bool { return access.AccountID == accountID }) {
			return true
		}
	}
	return false
}

// acceptShareInvitation plays the recipient accepting their pending invitation.
func (server *fakeAPIServer) acceptShareInvitation(t *testing.T, recipient apitypes.Member) {
	t.Helper()
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for index := range server.shareInvitations {
		invitation := &server.shareInvitations[index]
		if invitation.Email == recipient.Email && invitation.State == apitypes.ShareInvitationPending {
			acceptedAt := time.Date(2026, 10, 11, 8, 30, 0, 0, time.UTC)
			invitation.State = apitypes.ShareInvitationAccepted
			invitation.Recipient = &recipient
			invitation.AcceptedAt = &acceptedAt
			return
		}
	}
	t.Fatalf("no pending invitation to %s", recipient.Email)
}

func (server *fakeAPIServer) countShareInvitation() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return len(server.shareInvitations)
}

// shareThroughInvitation invites the member, accepts as them and lets the service complete the share.
func shareThroughInvitation(t *testing.T, test unlockedTestService, vaultID string, member apitypes.Member, role string) {
	t.Helper()
	if err := test.service.ShareVault(vaultID, member.Email, role); err != nil {
		t.Fatal(err)
	}
	test.apiServer.acceptShareInvitation(t, member)
	invitations, err := test.service.ListShareInvitations()
	if err != nil || invitations.CompletionError != "" {
		t.Fatalf("complete the share: %+v %v", invitations, err)
	}
	for _, invitation := range invitations.Sent {
		if invitation.VaultID == vaultID && invitation.RequiresConfirmation {
			if err := test.service.ConfirmShareRecipient(invitation.ID, invitation.ReviewToken); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, found := test.apiServer.getAccess(vaultID, member.AccountID); !found {
		t.Fatalf("%s has no access after completing", member.Email)
	}
}

func authorizeInvitedRecipient(t *testing.T, test unlockedTestService, member apitypes.Member) {
	t.Helper()
	test.apiServer.mutex.Lock()
	defer test.apiServer.mutex.Unlock()
	for i := range test.apiServer.shareInvitations {
		invitation := &test.apiServer.shareInvitations[i]
		if invitation.Email != member.Email {
			continue
		}
		invitation.Intent.Context.RecipientID = member.AccountID
		invitation.Intent.Context.RecipientKeyHash = crypt.ShareRecipientKeyHash(member.PublicKeys.Sign, member.PublicKeys.KEM)
		invitation.Intent.Signature = test.service.unlockedSession.Keyset.SignShareIntent(invitation.Intent.Context)
	}
}

func (server *fakeAPIServer) addMember(t *testing.T, name, email string) (apitypes.Member, *crypt.Keyset) {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	accountID := crypt.GenerateAccountID()
	member := apitypes.Member{AccountID: accountID, Email: email, Name: name, Role: apitypes.RoleMember, PublicKeys: keyset.GetPublicKeys(accountID)}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.members = append(server.members, member)
	return member, keyset
}

// replaceMemberKey simulates a server that substitutes a member's public keys.
func (server *fakeAPIServer) replaceMemberKey(t *testing.T, accountID string) {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for index := range server.members {
		if server.members[index].AccountID == accountID {
			server.members[index].PublicKeys = keyset.GetPublicKeys(accountID)
		}
	}
}

func (server *fakeAPIServer) getAccess(vaultID, accountID string) (apitypes.RotatedAccess, bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for _, access := range server.accessesByVaultID[vaultID] {
		if access.AccountID == accountID {
			return access, true
		}
	}
	return apitypes.RotatedAccess{}, false
}

func (server *fakeAPIServer) setVaultRole(vaultID, role string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for index := range server.vaults {
		if server.vaults[index].ID == vaultID {
			server.vaults[index].Role = role
		}
	}
	for index, access := range server.accessesByVaultID[vaultID] {
		if access.AccountID == server.accountID {
			server.accessesByVaultID[vaultID][index].Role = role
		}
	}
}

func getCachedVaultKey(t *testing.T, service *Service, vaultID string) []byte {
	t.Helper()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	vault, found := service.vaultsByID[vaultID]
	if !found {
		t.Fatalf("vault %s not cached", vaultID)
	}
	return append([]byte(nil), vault.Key...)
}

func TestShareVaultSendsInvitationAndCompletesOnAccept(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, bobKeyset := test.apiServer.addMember(t, "Bob", "b@example.com")

	if err := service.ShareVault(test.familyID, " B@example.com ", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	invitations, err := service.ListShareInvitations()
	if err != nil || len(invitations.Sent) != 1 || invitations.Sent[0].Email != "b@example.com" || invitations.Sent[0].VaultName != "Family" ||
		invitations.Sent[0].State != apitypes.ShareInvitationPending || invitations.Sent[0].Role != apitypes.VaultEditor {
		t.Fatalf("sent invitations: %+v %v", invitations, err)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Fatal("Bob got access before accepting")
	}
	if people, _ := service.ListPeople(); len(people) != 1 {
		t.Errorf("people before Bob accepted: %+v", people)
	}

	test.apiServer.acceptShareInvitation(t, bob)
	if !service.syncVaults(context.Background()) {
		t.Fatal("sync failed")
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Fatal("unknown identity got an automatic key")
	}
	reviews, reviewErr := service.ListShareInvitations()
	if reviewErr != nil || len(reviews.Sent) != 1 {
		t.Fatalf("recipient reviews: %+v %v", reviews, reviewErr)
	}
	review := reviews.Sent[0]
	if !review.RequiresConfirmation || review.RecipientID != bob.AccountID || review.SecurityCode == "" {
		t.Fatalf("missing recipient review: %+v", review)
	}
	if err := service.ConfirmShareRecipient(review.ID, review.ReviewToken); err != nil {
		t.Fatal(err)
	}
	access, found := test.apiServer.getAccess(test.familyID, bob.AccountID)
	if !found || access.Role != apitypes.VaultEditor {
		t.Fatalf("access after the sync completed the share: %+v %v", access, found)
	}
	unwrappedKey, err := bobKeyset.UnwrapVaultKey(test.familyID, bob.AccountID, access.WrappedKey)
	if err != nil || !bytes.Equal(unwrappedKey, getCachedVaultKey(t, service, test.familyID)) {
		t.Fatalf("Bob cannot unwrap the vault key: %v", err)
	}
	if test.apiServer.countShareInvitation() != 0 {
		t.Error("the completed invitation is still there")
	}
	reloadedState, err := localstate.Load(service.config.StatePath)
	if err != nil || !bytes.Equal(reloadedState.Pins[bob.AccountID], bob.PublicKeys.Sign) {
		t.Fatalf("pin not saved: %v", err)
	}

	vaultMembers, err := service.ListVaultMembers(test.familyID)
	if err != nil || len(vaultMembers) != 2 {
		t.Fatalf("vault members: %+v %v", vaultMembers, err)
	}
	if owner := vaultMembers[0]; owner.Name != "Alice" || owner.Role != apitypes.VaultOwner || !owner.IsCurrentUser {
		t.Errorf("owner = %+v", owner)
	}
	if member := vaultMembers[1]; member.Email != "b@example.com" || member.Role != apitypes.VaultEditor || member.IsCurrentUser ||
		member.SecurityCode != crypt.FormatKeyFingerprint(bob.PublicKeys.Sign) || member.IsKeyChanged {
		t.Errorf("member = %+v", member)
	}

	people, err := service.ListPeople()
	if err != nil || len(people) != 2 || people[0].Name != "Alice" || !people[0].IsCurrentUser || people[0].Role != apitypes.RoleAdmin {
		t.Fatalf("people: %+v %v", people, err)
	}
	if people[1].SecurityCode != crypt.FormatKeyFingerprint(bob.PublicKeys.Sign) || people[1].IsCurrentUser {
		t.Errorf("Bob = %+v", people[1])
	}

	test.apiServer.replaceMemberKey(t, bob.AccountID)
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, apitypes.VaultReader); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Errorf("role change after the key changed: %v", err)
	}
	if access, _ := test.apiServer.getAccess(test.familyID, bob.AccountID); access.Role != apitypes.VaultEditor {
		t.Errorf("role changed despite the key change: %+v", access)
	}
	if people, _ := service.ListPeople(); !people[1].IsKeyChanged {
		t.Error("changed key not reported")
	}
}

func TestCompletingAShareRefusesAChangedKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	travelID := createSharedTestVault(t, service, "Travel")
	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultReader)

	test.apiServer.replaceMemberKey(t, bob.AccountID)
	if err := service.ShareVault(travelID, "b@example.com", apitypes.VaultReader); err == nil || !strings.Contains(err.Error(), "signing key of b@example.com changed") {
		t.Fatalf("inviting a substituted known key: %v", err)
	}
	if _, found := test.apiServer.getAccess(travelID, bob.AccountID); found {
		t.Error("the vault key was wrapped to a substituted key")
	}
}

func TestCompletingAShareSavesThePinWhenTheAccessFails(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if err := test.service.ShareVault(test.familyID, "b@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	authorizeInvitedRecipient(t, test, bob)
	test.apiServer.acceptShareInvitation(t, bob)
	test.apiServer.mutex.Lock()
	test.apiServer.isSetAccessFailing = true
	test.apiServer.mutex.Unlock()

	invitations, err := test.service.ListShareInvitations()
	if err != nil || !strings.Contains(invitations.CompletionError, "b@example.com") {
		t.Fatalf("a share the server refused: %+v %v", invitations, err)
	}
	saved, err := localstate.Load(test.service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Pins[bob.AccountID], bob.PublicKeys.Sign) {
		t.Errorf("state.json pins %x for Bob, want his signing key", saved.Pins[bob.AccountID])
	}
}

func TestCompletingAShareReportsPinsThatCouldNotBeSaved(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if err := test.service.ShareVault(test.familyID, "b@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	authorizeInvitedRecipient(t, test, bob)
	test.apiServer.acceptShareInvitation(t, bob)
	unblockStateSaves := blockStateSaves(t, test.service.config.StatePath)

	invitations, err := test.service.ListShareInvitations()
	if err != nil || !strings.Contains(invitations.CompletionError, "could not save the pinned signing keys") {
		t.Fatalf("a pin that could not be saved was not reported: %+v %v", invitations, err)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); !found {
		t.Error("the share was not completed")
	}
	// The share is complete, so the next pass pins nothing new; Bob's pin is still unsaved.
	if invitations, err := test.service.ListShareInvitations(); err != nil || !strings.Contains(invitations.CompletionError, "could not save the pinned signing keys") {
		t.Fatalf("a pass that pinned nothing forgot the unsaved pin: %+v %v", invitations, err)
	}

	unblockStateSaves()
	if invitations, err := test.service.ListShareInvitations(); err != nil || invitations.CompletionError != "" {
		t.Fatalf("after a save that works: %+v %v", invitations, err)
	}
	saved, err := localstate.Load(test.service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Pins[bob.AccountID], bob.PublicKeys.Sign) {
		t.Errorf("state.json pins %x for Bob, want his signing key", saved.Pins[bob.AccountID])
	}
}

func createSharedTestVault(t *testing.T, service *Service, name string) string {
	t.Helper()
	vault, err := service.CreateVault(name)
	if err != nil {
		t.Fatal(err)
	}
	return vault.ID
}

func TestShareVaultValidatesAndReportsServerErrors(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")

	if err := service.ShareVault(test.familyID, "nobody@example.com", apitypes.VaultReader); err != nil {
		t.Errorf("unknown email: %v", err)
	}
	if err := service.ShareVault(test.familyID, "A@example.com", apitypes.VaultReader); !errors.Is(err, errShareWithOwnAccount) {
		t.Errorf("share with yourself: %v", err)
	}
	if err := service.ShareVault(test.familyID, "b@example.com", "admin"); err == nil || !strings.Contains(err.Error(), "owner, editor or reader") {
		t.Errorf("invalid role: %v", err)
	}
	for _, badEmail := range []string{"", "bob", "@example.com", "bob@", "Bob <b@example.com>"} {
		if err := service.ShareVault(test.familyID, badEmail, apitypes.VaultReader); !errors.Is(err, errInvalidEmail) {
			t.Errorf("email %q: %v", badEmail, err)
		}
	}
	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultReader)
	if err := service.ShareVault(test.familyID, "b@example.com", apitypes.VaultEditor); !errors.Is(err, errAlreadyHasAccess) {
		t.Errorf("share with a member: %v", err)
	}
	for status, expected := range map[int]error{
		http.StatusNotImplemented:  errEmailNotConfigured,
		http.StatusTooManyRequests: errTooManyInvitation,
		http.StatusNotFound:        errInvitationNotFound,
		http.StatusForbidden:       errOwnerOnly,
	} {
		if err := convertInvitationError(&apiclient.APIError{Status: status, Message: "server message"}); !errors.Is(err, expected) {
			t.Errorf("%d: %v", status, err)
		}
	}
	if err := convertInvitationError(&apiclient.APIError{Status: http.StatusBadRequest, Message: "personal vaults cannot be shared"}); err == nil || err.Error() != "personal vaults cannot be shared" {
		t.Errorf("400: %v", err)
	}
}

func TestShareVaultRefusesPersonalVaultAndNonOwner(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")

	if err := service.ShareVault(test.personalID, "b@example.com", apitypes.VaultReader); !errors.Is(err, errPersonalVault) {
		t.Errorf("share personal vault: %v", err)
	}
	if err := service.ChangeVaultMemberRole(test.personalID, bob.AccountID, apitypes.VaultReader); !errors.Is(err, errPersonalVault) {
		t.Errorf("role change in personal vault: %v", err)
	}
	if err := service.RemoveVaultMember(test.personalID, bob.AccountID); !errors.Is(err, errPersonalVault) {
		t.Errorf("remove from personal vault: %v", err)
	}
	if test.apiServer.countShareInvitation() != 0 {
		t.Error("an invitation to the personal vault was sent")
	}

	test.apiServer.setVaultRole(test.familyID, apitypes.VaultEditor)
	if _, err := service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if err := service.ShareVault(test.familyID, "b@example.com", apitypes.VaultReader); !errors.Is(err, errOwnerOnly) {
		t.Errorf("share as editor: %v", err)
	}
	if err := service.RemoveVaultMember(test.familyID, bob.AccountID); !errors.Is(err, errOwnerOnly) {
		t.Errorf("remove as editor: %v", err)
	}
	if vaultMembers, err := service.ListVaultMembers(test.familyID); err != nil || len(vaultMembers) != 1 {
		t.Errorf("editors can list members: %+v %v", vaultMembers, err)
	}
	if err := convertSharingError(&apiclient.APIError{Status: http.StatusForbidden, Message: "owner only"}); !errors.Is(err, errOwnerOnly) {
		t.Errorf("403 from the server: %v", err)
	}
}

func TestReceivedShareInvitationsCanBeAnswered(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	expiresAt := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	test.apiServer.receivedShareInvitations = []apitypes.ReceivedShareInvitation{
		{ID: "from-carol", VaultID: "vault-1", Role: apitypes.VaultEditor, InvitedByName: "Carol", InvitedByEmail: "c@example.com", ExpiresAt: expiresAt},
		{ID: "from-dave", VaultID: "vault-2", Role: apitypes.VaultReader, InvitedByName: "Dave", InvitedByEmail: "d@example.com", ExpiresAt: expiresAt},
	}
	invitations, err := service.ListShareInvitations()
	if err != nil || len(invitations.Received) != 2 || invitations.Received[0].InvitedByName != "Carol" ||
		invitations.Received[0].ExpiresAt != "2026-10-18T09:00:00Z" || len(invitations.Sent) != 0 {
		t.Fatalf("received: %+v %v", invitations, err)
	}
	if err := service.AcceptShareInvitation("from-carol"); err != nil {
		t.Fatal(err)
	}
	if err := service.DeclineShareInvitation("from-dave"); err != nil {
		t.Fatal(err)
	}
	if err := service.AcceptShareInvitation("from-carol"); !errors.Is(err, errInvitationNotFound) {
		t.Errorf("accept twice: %v", err)
	}
	if err := service.DeclineShareInvitation(" "); !errors.Is(err, errInvitationNotFound) {
		t.Errorf("empty ID: %v", err)
	}
	if !slices.Equal(test.apiServer.answeredShareInvitations, []string{"from-carol accept", "from-dave decline"}) {
		t.Errorf("answers: %v", test.apiServer.answeredShareInvitations)
	}

	if err := service.ShareVault(test.familyID, "e@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	invitations, _ = service.ListShareInvitations()
	if len(invitations.Sent) != 1 {
		t.Fatalf("sent: %+v", invitations.Sent)
	}
	if err := service.CancelShareInvitation(invitations.Sent[0].ID); err != nil {
		t.Fatal(err)
	}
	if test.apiServer.countShareInvitation() != 0 {
		t.Error("cancelled invitation still there")
	}
	if err := service.CancelShareInvitation(invitations.Sent[0].ID); !errors.Is(err, errInvitationNotFound) {
		t.Errorf("cancel twice: %v", err)
	}
}

func TestRecipientSeesWhoTheShareIsWaitingFor(t *testing.T) {
	test := createUnlockedTestService(t)
	acceptedAt := time.Date(2026, 10, 11, 7, 0, 0, 0, time.UTC)
	test.apiServer.waitingShareInvitations = []apitypes.WaitingShareInvitation{
		{ID: "from-carol", VaultID: "vault-1", Role: apitypes.VaultEditor, InvitedByName: "Carol", InvitedByEmail: "c@example.com", AcceptedAt: acceptedAt},
	}
	invitations, err := test.service.ListShareInvitations()
	if err != nil || len(invitations.Waiting) != 1 || len(invitations.Received) != 0 {
		t.Fatalf("waiting: %+v %v", invitations, err)
	}
	if waiting := invitations.Waiting[0]; waiting.ID != "from-carol" || waiting.InvitedByName != "Carol" || waiting.InvitedByEmail != "c@example.com" ||
		waiting.Role != apitypes.VaultEditor || waiting.AcceptedAt != "2026-10-11T07:00:00Z" {
		t.Errorf("waiting = %+v", waiting)
	}
	test.apiServer.waitingShareInvitations = nil
	if invitations, _ := test.service.ListShareInvitations(); invitations.Waiting == nil || len(invitations.Waiting) != 0 {
		t.Errorf("an old server's missing list: %#v", invitations.Waiting)
	}
}

func TestLockedAppListsSharesAwaitingUnlock(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if err := service.ShareVault(test.familyID, "b@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	test.apiServer.acceptShareInvitation(t, bob)
	if awaitingShares, err := service.ListSharesAwaitingUnlock(); err != nil || len(awaitingShares) != 0 {
		t.Errorf("unlocked, the app completes shares itself: %+v %v", awaitingShares, err)
	}

	service.Lock()
	awaitingShares, err := service.ListSharesAwaitingUnlock()
	if err != nil || len(awaitingShares) != 1 {
		t.Fatalf("awaiting unlock: %+v %v", awaitingShares, err)
	}
	if awaiting := awaitingShares[0]; awaiting.RecipientName != "Bob" || awaiting.RecipientEmail != "b@example.com" || awaiting.AcceptedAt != "2026-10-11T08:30:00Z" || awaiting.InvitationID == "" {
		t.Errorf("awaiting = %+v", awaiting)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Error("a locked app completed the share")
	}
	if _, err := service.ListShareInvitations(); !errors.Is(err, ErrLocked) {
		t.Errorf("full invitation list while locked: %v", err)
	}

	test.apiServer.setUnreachable(true)
	if awaitingShares, err := service.ListSharesAwaitingUnlock(); err != nil || len(awaitingShares) != 0 {
		t.Errorf("offline: %+v %v", awaitingShares, err)
	}
	test.apiServer.setUnreachable(false)
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	if awaitingShares, err := service.ListSharesAwaitingUnlock(); err != nil || len(awaitingShares) != 0 {
		t.Errorf("signed out: %+v %v", awaitingShares, err)
	}
}

func TestShareInvitationEventCompletesShare(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if err := service.ShareVault(test.familyID, "b@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	authorizeInvitedRecipient(t, test, bob)
	remoteChange := make(chan struct{}, 4)
	service.mutex.Lock()
	service.config.OnRemoteChange = func() { remoteChange <- struct{}{} }
	test.apiServer.eventChannel = make(chan string, 4)
	service.mutex.Unlock()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go service.watchEvents(ctx, service.unlockedSession.Client, make(chan struct{}))

	test.apiServer.acceptShareInvitation(t, bob)
	test.apiServer.eventChannel <- apitypes.EventShareInvitationsChanged
	select {
	case <-remoteChange:
	case <-time.After(5 * time.Second):
		t.Fatal("no refresh after share-invitations-changed")
	}
	if access, found := test.apiServer.getAccess(test.familyID, bob.AccountID); !found || access.Role != apitypes.VaultReader {
		t.Errorf("access after the event: %+v %v", access, found)
	}
}

func TestChangeVaultMemberRole(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, _ := test.apiServer.addMember(t, "Carol", "c@example.com")

	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultEditor)
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	if access, _ := test.apiServer.getAccess(test.familyID, bob.AccountID); access.Role != apitypes.VaultReader {
		t.Errorf("role after change: %+v", access)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, apitypes.VaultOwner); err != nil {
		t.Fatal(err)
	}
	if vaultMembers, _ := service.ListVaultMembers(test.familyID); len(vaultMembers) != 2 || vaultMembers[1].Role != apitypes.VaultOwner {
		t.Errorf("members after promotion: %+v", vaultMembers)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, test.accountID, apitypes.VaultReader); !errors.Is(err, errOwnAccess) {
		t.Errorf("change own role: %v", err)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, carol.AccountID, apitypes.VaultReader); err == nil || !strings.Contains(err.Error(), "no longer has access") {
		t.Errorf("change role of a non-member: %v", err)
	}
	if _, found := test.apiServer.getAccess(test.familyID, carol.AccountID); found {
		t.Error("role change shared the vault with a non-member")
	}
}

func TestRemoveVaultMemberRotatesKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, carolKeyset := test.apiServer.addMember(t, "Carol", "c@example.com")

	saved, err := service.SaveItem(test.familyID, ItemDetail{Title: "Netflix", Password: "hunter2hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultEditor)
	shareThroughInvitation(t, test, test.familyID, carol, apitypes.VaultEditor)
	oldKey := getCachedVaultKey(t, service, test.familyID)

	if err := service.RemoveVaultMember(test.familyID, test.accountID); err == nil || !strings.Contains(err.Error(), "yourself") {
		t.Errorf("remove yourself: %v", err)
	}
	if err := service.RemoveVaultMember(test.familyID, bob.AccountID); err != nil {
		t.Fatal(err)
	}
	newKey := getCachedVaultKey(t, service, test.familyID)
	if bytes.Equal(oldKey, newKey) {
		t.Fatal("vault key did not change")
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Error("Bob still has access")
	}
	carolAccess, found := test.apiServer.getAccess(test.familyID, carol.AccountID)
	if !found || carolAccess.Role != apitypes.VaultEditor {
		t.Fatalf("Carol's access: %+v %v", carolAccess, found)
	}
	if carolKey, err := carolKeyset.UnwrapVaultKey(test.familyID, carol.AccountID, carolAccess.WrappedKey); err != nil || !bytes.Equal(carolKey, newKey) {
		t.Errorf("Carol did not get the new key: %v", err)
	}
	fetched, err := service.GetItem(test.familyID, saved.ID)
	if err != nil || fetched.Password != "hunter2hunter2" || fetched.Revision == saved.Revision {
		t.Fatalf("item after rotation: %+v %v", fetched, err)
	}
	if _, err := service.SaveItem(test.familyID, ItemDetail{ID: fetched.ID, Revision: fetched.Revision, Title: "Netflix", Password: "changed"}); err != nil {
		t.Errorf("edit after rotation: %v", err)
	}
	if err := service.RemoveVaultMember(test.familyID, bob.AccountID); err == nil || !strings.Contains(err.Error(), "no longer has access") {
		t.Errorf("remove twice: %v", err)
	}

	dave, _ := test.apiServer.addMember(t, "Dave", "d@example.com")
	shareThroughInvitation(t, test, test.familyID, dave, apitypes.VaultReader)
	test.apiServer.replaceMemberKey(t, carol.AccountID)
	if err := service.RemoveVaultMember(test.familyID, dave.AccountID); err == nil || !strings.Contains(err.Error(), "signing key of c@example.com changed") {
		t.Errorf("rotation wrapped the key for a substituted key: %v", err)
	}
	if !bytes.Equal(getCachedVaultKey(t, service, test.familyID), newKey) {
		t.Error("vault key changed although the rotation was refused")
	}
}

// TestRemoveVaultMemberRotatesPastAMalformedTombstone: a deleted item's sealed content is never read, so a tombstone an
// editor wrote with ciphertext nobody can open does not stop an owner from removing them.
func TestRemoveVaultMemberRotatesPastAMalformedTombstone(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultEditor)
	tombstoneID := "4b0d1c34-6f07-4f3c-9a55-0e2f8d1c7a10"
	test.apiServer.mutex.Lock()
	test.apiServer.seq++
	test.apiServer.itemsByVaultID[test.familyID][tombstoneID] = apitypes.Item{
		ID: tombstoneID, Revision: "019a0000-0000-7000-8000-000000000001", Seq: test.apiServer.seq, Deleted: true, Overview: []byte{1}, Details: []byte{1},
	}
	test.apiServer.mutex.Unlock()

	if err := service.RemoveVaultMember(test.familyID, bob.AccountID); err != nil {
		t.Fatalf("removing Bob past the malformed tombstone: %v", err)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Error("Bob still has access")
	}
	test.apiServer.mutex.Lock()
	tombstone := test.apiServer.itemsByVaultID[test.familyID][tombstoneID]
	test.apiServer.mutex.Unlock()
	if !tombstone.Deleted {
		t.Errorf("the rotation brought the tombstone back: %+v", tombstone)
	}
	if _, err := crypt.OpenItemPart(getCachedVaultKey(t, service, test.familyID), test.familyID, tombstoneID, tombstone.Revision, crypt.Details, tombstone.Details); err != nil {
		t.Errorf("the rotated tombstone is not sealed with the new key: %v", err)
	}
}

// TestRemoveVaultMemberAfterAnotherOwnerRotated: another owner's rotation replaced the key this app holds, so the
// removal reloads the vault and rotates from the current key instead of failing every attempt.
func TestRemoveVaultMemberAfterAnotherOwnerRotated(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	ctx := context.Background()
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, carolKeyset := test.apiServer.addMember(t, "Carol", "c@example.com")
	saved, err := service.SaveItem(test.familyID, ItemDetail{Title: "Netflix", Password: "hunter2hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultEditor)
	shareThroughInvitation(t, test, test.familyID, carol, apitypes.VaultEditor)
	staleKey := getCachedVaultKey(t, service, test.familyID)
	service.mutex.Lock()
	unlockedSession := service.unlockedSession
	service.mutex.Unlock()
	otherOwnersVault, err := unlockedSession.FindVault(ctx, test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	grants := []session.AccessGrant{
		{AccountID: test.accountID, Role: apitypes.VaultOwner}, {AccountID: bob.AccountID, Role: apitypes.VaultEditor}, {AccountID: carol.AccountID, Role: apitypes.VaultEditor},
	}
	if err := unlockedSession.RotateVaultKey(ctx, otherOwnersVault, grants); err != nil {
		t.Fatal(err)
	}

	if err := service.RemoveVaultMember(test.familyID, bob.AccountID); err != nil {
		t.Fatalf("removing Bob after another owner rotated: %v", err)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Error("Bob still has access")
	}
	newKey := getCachedVaultKey(t, service, test.familyID)
	if bytes.Equal(newKey, staleKey) || bytes.Equal(newKey, otherOwnersVault.Key) {
		t.Fatal("the removal did not rotate to a new key")
	}
	carolAccess, _ := test.apiServer.getAccess(test.familyID, carol.AccountID)
	if carolKey, err := carolKeyset.UnwrapVaultKey(test.familyID, carol.AccountID, carolAccess.WrappedKey); err != nil || !bytes.Equal(carolKey, newKey) {
		t.Errorf("Carol did not get the new key: %v", err)
	}
	if fetched, err := service.GetItem(test.familyID, saved.ID); err != nil || fetched.Password != "hunter2hunter2" {
		t.Errorf("item after the removal: %+v %v", fetched, err)
	}
}

// checkSharingFlow runs against a real server: service is an admin who owns the shared vault familyID, and
// memberInviteLink is an operator invite for memberEmail.
func checkSharingFlow(t *testing.T, service *Service, familyID, memberEmail, memberInviteLink string) {
	t.Helper()
	member, _, _ := createTestService(t, filepath.Join(t.TempDir(), "member.json"), createMemorySecretKeyStore())
	t.Cleanup(func() { member.Lock() })
	if _, err := member.SignUp(memberInviteLink, memberEmail, "Desktop Member", "member master password"); err != nil {
		t.Fatalf("sign up with the invite: %v", err)
	}
	memberAccountID := member.state.AccountID
	if err := member.InvitePerson("someone-invited-by-a-member@example.com"); err != nil && !errors.Is(err, errEmailNotConfigured) {
		t.Errorf("member invites: %v", err)
	}

	saved, err := service.SaveItem(familyID, ItemDetail{Title: "Netflix", Password: "shared family password"})
	if err != nil {
		t.Fatal(err)
	}
	if people, err := service.ListPeople(); err != nil || slices.ContainsFunc(people, func(person Person) bool { return person.AccountID == memberAccountID }) {
		t.Fatalf("a stranger is listed: %+v %v", people, err)
	}

	if err := service.ShareVault(familyID, memberEmail, apitypes.VaultReader); err != nil {
		t.Fatalf("share: %v", err)
	}
	if err := service.ShareVault(familyID, "nobody-"+memberEmail, apitypes.VaultReader); err != nil {
		t.Fatalf("share with an unknown email: %v", err)
	}
	received, err := member.ListShareInvitations()
	if err != nil || len(received.Received) != 1 || received.Received[0].VaultID != familyID || received.Received[0].Role != apitypes.VaultReader {
		t.Fatalf("member's invitations: %+v %v", received, err)
	}
	if err := member.AcceptShareInvitation(received.Received[0].ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if memberVaults, err := member.ListVaults(); err != nil || slices.ContainsFunc(memberVaults, func(vault VaultSummary) bool { return vault.ID == familyID }) {
		t.Fatalf("member has the vault before the owner completed: %+v %v", memberVaults, err)
	}
	sent, err := service.ListShareInvitations()
	if err != nil || sent.CompletionError != "" {
		t.Fatalf("owner's invitations before review: %+v %v", sent, err)
	}
	reviewIndex := slices.IndexFunc(sent.Sent, func(invitation SentShareInvitation) bool {
		return invitation.Email == memberEmail && invitation.RequiresConfirmation
	})
	if reviewIndex < 0 {
		t.Fatalf("no recipient review: %+v", sent)
	}
	if memberVaults, err := member.ListVaults(); err != nil || slices.ContainsFunc(memberVaults, func(vault VaultSummary) bool { return vault.ID == familyID }) {
		t.Fatalf("member got the vault before the review: %+v %v", memberVaults, err)
	}
	if err := service.ConfirmShareRecipient(sent.Sent[reviewIndex].ID, sent.Sent[reviewIndex].ReviewToken); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	sent, err = service.ListShareInvitations()
	if err != nil || slices.ContainsFunc(sent.Sent, func(invitation SentShareInvitation) bool { return invitation.Email == memberEmail }) {
		t.Fatalf("owner's invitations after confirming: %+v %v", sent, err)
	}
	people, err := service.ListPeople()
	if err != nil {
		t.Fatal(err)
	}
	personIndex := slices.IndexFunc(people, func(person Person) bool { return person.AccountID == memberAccountID })
	if personIndex < 0 || people[personIndex].Role != apitypes.RoleMember || len(people[personIndex].SecurityCode) != 19 {
		t.Fatalf("people: %+v", people)
	}
	memberVaults, err := member.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if index := slices.IndexFunc(memberVaults, func(vault VaultSummary) bool { return vault.ID == familyID }); index < 0 || memberVaults[index].Role != apitypes.VaultReader {
		t.Fatalf("member's vaults after share: %+v", memberVaults)
	}
	if fetched, err := member.GetItem(familyID, saved.ID); err != nil || fetched.Password != "shared family password" {
		t.Fatalf("member reads the shared item: %+v %v", fetched, err)
	}
	if err := member.ShareVault(familyID, "desktop@example.com", apitypes.VaultReader); !errors.Is(err, errOwnerOnly) {
		t.Errorf("reader shares: %v", err)
	}

	if err := service.ChangeVaultMemberRole(familyID, memberAccountID, apitypes.VaultEditor); err != nil {
		t.Fatalf("change role: %v", err)
	}
	vaultMembers, err := service.ListVaultMembers(familyID)
	if err != nil || len(vaultMembers) != 2 || !vaultMembers[0].IsCurrentUser || vaultMembers[1].Role != apitypes.VaultEditor || vaultMembers[1].Email != memberEmail {
		t.Fatalf("vault members: %+v %v", vaultMembers, err)
	}

	if err := service.RemoveVaultMember(familyID, memberAccountID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := member.ListItems(familyID); err == nil {
		t.Error("removed member still reads the vault")
	}
	if memberVaults, err := member.ListVaults(); err != nil || slices.ContainsFunc(memberVaults, func(vault VaultSummary) bool { return vault.ID == familyID }) {
		t.Errorf("removed member still lists the vault: %+v %v", memberVaults, err)
	}
	if fetched, err := service.GetItem(familyID, saved.ID); err != nil || fetched.Password != "shared family password" {
		t.Fatalf("owner reads the item after rotation: %+v %v", fetched, err)
	}
	if err := service.DeleteItem(familyID, saved.ID); err != nil {
		t.Fatalf("delete after rotation: %v", err)
	}
	if sent, err := service.ListShareInvitations(); err == nil {
		for _, invitation := range sent.Sent {
			service.CancelShareInvitation(invitation.ID)
		}
	}

	devices, err := service.ListDevices()
	if err != nil || len(devices) == 0 || !devices[0].IsCurrent {
		t.Fatalf("devices: %+v %v", devices, err)
	}
	if err := service.RevokeDevice(devices[0].ID); !errors.Is(err, errCurrentDevice) {
		t.Errorf("revoke this device: %v", err)
	}
}

func TestInvitePerson(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service

	if err := service.InvitePerson(" New@Example.com "); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(test.apiServer.invitedEmails, []string{"new@example.com"}) {
		t.Errorf("invited: %v", test.apiServer.invitedEmails)
	}
	if err := service.InvitePerson("not an email"); !errors.Is(err, errInvalidEmail) {
		t.Errorf("invalid email: %v", err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.role = apitypes.RoleMember
	test.apiServer.mutex.Unlock()
	if err := service.InvitePerson("other@example.com"); err != nil {
		t.Errorf("a member's invite: %v", err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.isInviteAdminsOnly = true
	test.apiServer.mutex.Unlock()
	if err := service.InvitePerson("refused@example.com"); !errors.Is(err, errInviteAdminsOnly) || err.Error() != "Only admins can invite people to the Hisar server." {
		t.Errorf("a member's invite under admins only: %v", err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.isInviteAdminsOnly = false
	test.apiServer.mutex.Unlock()
	test.apiServer.mutex.Lock()
	test.apiServer.isEmailUnavailable = true
	test.apiServer.mutex.Unlock()
	if err := service.InvitePerson("third@example.com"); !errors.Is(err, errEmailNotConfigured) {
		t.Errorf("server without email: %v", err)
	}
}

func TestListAllDeviceAndRevoke(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	now := time.Now()
	test.apiServer.devices = []apitypes.Device{
		{ID: "old-phone", Name: "iPhone", Platform: "ios/mobile", LastSeenAt: now.Add(-48 * time.Hour)},
		{ID: "this-mac", Name: "MacBook", Platform: "darwin/desktop", LastSeenAt: now.Add(-time.Hour), Current: true},
		{ID: "laptop", Name: "ThinkPad", Platform: "linux/cli", LastSeenAt: now},
	}

	devices, err := service.ListDevices()
	if err != nil {
		t.Fatal(err)
	}
	var iDs []string
	for _, device := range devices {
		iDs = append(iDs, device.ID)
	}
	if !slices.Equal(iDs, []string{"this-mac", "laptop", "old-phone"}) || !devices[0].IsCurrent || devices[1].IsCurrent {
		t.Fatalf("devices: %+v", devices)
	}
	if lastSeen, err := time.Parse(time.RFC3339, devices[1].LastSeenAt); err != nil || lastSeen.Unix() != now.Unix() {
		t.Errorf("last seen: %q %v", devices[1].LastSeenAt, err)
	}

	if err := service.RevokeDevice("this-mac"); !errors.Is(err, errCurrentDevice) {
		t.Errorf("revoke this device: %v", err)
	}
	if err := service.RevokeDevice("old-phone"); err != nil {
		t.Fatal(err)
	}
	if devices, _ := service.ListDevices(); len(devices) != 2 {
		t.Errorf("devices after revoke: %+v", devices)
	}
	if err := service.RevokeDevice("old-phone"); err == nil || !strings.Contains(err.Error(), "already signed out") {
		t.Errorf("revoke twice: %v", err)
	}
}
