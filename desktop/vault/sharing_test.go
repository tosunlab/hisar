package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
)

func (server *fakeAPIServer) getVaultRole(vaultID string) string {
	for _, access := range server.allAccessByVaultID[vaultID] {
		if access.AccountID == server.accountID {
			return access.Role
		}
	}
	return ""
}

func (server *fakeAPIServer) addAllSharingRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/members", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		allPerson := []proto.Member{}
		for _, member := range server.allMember {
			if member.AccountID == server.accountID || server.isSharingVaultWith(member.AccountID) {
				allPerson = append(allPerson, member)
			}
		}
		writeJSON(writer, http.StatusOK, allPerson)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/share-invitations", func(writer http.ResponseWriter, request *http.Request) {
		var shareRequest proto.ShareInvitationRequest
		if err := json.NewDecoder(request.Body).Decode(&shareRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, proto.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) != proto.VaultOwner {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "owner only"})
			return
		}
		for _, access := range server.allAccessByVaultID[vaultID] {
			if member := server.findMember(access.AccountID); member != nil && member.Email == shareRequest.Email {
				writeJSON(writer, http.StatusConflict, proto.Error{Error: "this person already has access to the vault"})
				return
			}
		}
		server.allShareInvitation = slices.DeleteFunc(server.allShareInvitation, func(invitation proto.SentShareInvitation) bool {
			return invitation.VaultID == vaultID && invitation.Email == shareRequest.Email
		})
		server.allShareInvitation = append(server.allShareInvitation, proto.SentShareInvitation{
			ID: fmt.Sprintf("invitation-%d", len(server.allShareInvitation)+1), VaultID: vaultID, Email: shareRequest.Email, Role: shareRequest.Role,
			State: proto.ShareInvitationPending, InvitedBy: server.accountID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		})
		writeJSON(writer, http.StatusAccepted, proto.InvitationSentResponse{Message: proto.InvitationSentMessage})
	})
	mux.HandleFunc("GET /api/v1/share-invitations", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		response := proto.ShareInvitationList{AllReceived: append([]proto.ReceivedShareInvitation{}, server.allReceivedShareInvitation...), AllSent: []proto.SentShareInvitation{},
			AllWaiting: server.allWaitingShareInvitation}
		for _, invitation := range server.allShareInvitation {
			if server.getVaultRole(invitation.VaultID) == proto.VaultOwner {
				response.AllSent = append(response.AllSent, invitation)
			}
		}
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/share-invitations/{invitationID}/{answer}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		invitationID := request.PathValue("invitationID")
		index := slices.IndexFunc(server.allReceivedShareInvitation, func(invitation proto.ReceivedShareInvitation) bool { return invitation.ID == invitationID })
		if index < 0 {
			writeJSON(writer, http.StatusNotFound, proto.Error{Error: "this invitation no longer exists"})
			return
		}
		server.allReceivedShareInvitation = slices.Delete(server.allReceivedShareInvitation, index, index+1)
		server.allAnsweredShareInvitation = append(server.allAnsweredShareInvitation, invitationID+" "+request.PathValue("answer"))
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/share-invitations/{invitationID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		invitationID := request.PathValue("invitationID")
		index := slices.IndexFunc(server.allShareInvitation, func(invitation proto.SentShareInvitation) bool { return invitation.ID == invitationID })
		if index < 0 || server.getVaultRole(server.allShareInvitation[index].VaultID) != proto.VaultOwner {
			writeJSON(writer, http.StatusNotFound, proto.Error{Error: "this invitation no longer exists"})
			return
		}
		server.allShareInvitation = slices.Delete(server.allShareInvitation, index, index+1)
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/members", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) == "" {
			writeJSON(writer, http.StatusNotFound, proto.Error{Error: "not found"})
			return
		}
		allVaultMember := []proto.VaultMember{}
		for _, access := range server.allAccessByVaultID[vaultID] {
			allVaultMember = append(allVaultMember, proto.VaultMember{AccountID: access.AccountID, Role: access.Role})
		}
		writeJSON(writer, http.StatusOK, allVaultMember)
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/access/{accountID}", func(writer http.ResponseWriter, request *http.Request) {
		var setRequest proto.SetAccessRequest
		if err := json.NewDecoder(request.Body).Decode(&setRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, proto.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID, accountID := request.PathValue("vaultID"), request.PathValue("accountID")
		if server.getVaultRole(vaultID) != proto.VaultOwner {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "owner only"})
			return
		}
		allAccess := server.allAccessByVaultID[vaultID]
		access := proto.RotatedAccess{AccountID: accountID, Role: setRequest.Role, WrappedKey: setRequest.WrappedKey}
		if index := slices.IndexFunc(allAccess, func(existing proto.RotatedAccess) bool { return existing.AccountID == accountID }); index >= 0 {
			allAccess[index] = access
		} else {
			invitationIndex := slices.IndexFunc(server.allShareInvitation, func(invitation proto.SentShareInvitation) bool {
				return invitation.VaultID == vaultID && invitation.State == proto.ShareInvitationAccepted && invitation.Recipient != nil && invitation.Recipient.AccountID == accountID
			})
			if invitationIndex < 0 {
				writeJSON(writer, http.StatusConflict, proto.Error{Error: "this person has not accepted an invitation to this vault"})
				return
			}
			server.allShareInvitation = slices.Delete(server.allShareInvitation, invitationIndex, invitationIndex+1)
			server.allAccessByVaultID[vaultID] = append(allAccess, access)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/rotate", func(writer http.ResponseWriter, request *http.Request) {
		var rotateRequest proto.RotateVaultRequest
		if err := json.NewDecoder(request.Body).Decode(&rotateRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, proto.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) != proto.VaultOwner {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "owner only"})
			return
		}
		allItem := server.allItemByVaultID[vaultID]
		if len(rotateRequest.AllItem) != len(allItem) {
			writeJSON(writer, http.StatusConflict, proto.Error{Error: "items changed"})
			return
		}
		for _, rotatedItem := range rotateRequest.AllItem {
			if stored, found := allItem[rotatedItem.ID]; !found || stored.Revision != rotatedItem.BaseRevision {
				writeJSON(writer, http.StatusConflict, proto.Error{Error: "items changed"})
				return
			}
		}
		for _, rotatedItem := range rotateRequest.AllItem {
			server.seq++
			allItem[rotatedItem.ID] = proto.Item{ID: rotatedItem.ID, Revision: rotatedItem.Revision, Seq: server.seq, Deleted: rotatedItem.Deleted, Overview: rotatedItem.Overview, Details: rotatedItem.Details}
		}
		server.allAccessByVaultID[vaultID] = rotateRequest.AllAccess
		for index := range server.allVault {
			if server.allVault[index].ID != vaultID {
				continue
			}
			server.allVault[index].EncName = rotateRequest.EncName
			for _, access := range rotateRequest.AllAccess {
				if access.AccountID == server.accountID {
					server.allVault[index].WrappedKey = access.WrappedKey
				}
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/invites", func(writer http.ResponseWriter, request *http.Request) {
		var inviteRequest proto.InviteRequest
		if err := json.NewDecoder(request.Body).Decode(&inviteRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, proto.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.isEmailUnavailable {
			writeJSON(writer, http.StatusNotImplemented, proto.Error{Error: "email is not configured on this server"})
			return
		}
		server.allInvitedEmail = append(server.allInvitedEmail, inviteRequest.Email)
		writeJSON(writer, http.StatusAccepted, proto.InvitationSentResponse{Message: proto.InvitationSentMessage})
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
		writeJSON(writer, http.StatusOK, server.allDevice)
	})
	mux.HandleFunc("DELETE /api/v1/devices/{deviceID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		deviceID := request.PathValue("deviceID")
		server.allDevice = slices.DeleteFunc(server.allDevice, func(device proto.Device) bool { return device.ID == deviceID })
		writer.WriteHeader(http.StatusNoContent)
	})
}

func (server *fakeAPIServer) findMember(accountID string) *proto.Member {
	for index := range server.allMember {
		if server.allMember[index].AccountID == accountID {
			member := server.allMember[index]
			return &member
		}
	}
	return nil
}

func (server *fakeAPIServer) isSharingVaultWith(accountID string) bool {
	for _, allAccess := range server.allAccessByVaultID {
		isMine := slices.ContainsFunc(allAccess, func(access proto.RotatedAccess) bool { return access.AccountID == server.accountID })
		if isMine && slices.ContainsFunc(allAccess, func(access proto.RotatedAccess) bool { return access.AccountID == accountID }) {
			return true
		}
	}
	return false
}

// acceptShareInvitation plays the recipient accepting their pending invitation.
func (server *fakeAPIServer) acceptShareInvitation(t *testing.T, recipient proto.Member) {
	t.Helper()
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for index := range server.allShareInvitation {
		invitation := &server.allShareInvitation[index]
		if invitation.Email == recipient.Email && invitation.State == proto.ShareInvitationPending {
			acceptedAt := time.Date(2026, 10, 11, 8, 30, 0, 0, time.UTC)
			invitation.State = proto.ShareInvitationAccepted
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
	return len(server.allShareInvitation)
}

// shareThroughInvitation invites the member, accepts as them and lets the service complete the share.
func shareThroughInvitation(t *testing.T, test unlockedTestService, vaultID string, member proto.Member, role string) {
	t.Helper()
	if err := test.service.ShareVault(vaultID, member.Email, role); err != nil {
		t.Fatal(err)
	}
	test.apiServer.acceptShareInvitation(t, member)
	invitations, err := test.service.ListAllShareInvitation()
	if err != nil || invitations.CompletionError != "" {
		t.Fatalf("complete the share: %+v %v", invitations, err)
	}
	if _, found := test.apiServer.getAccess(vaultID, member.AccountID); !found {
		t.Fatalf("%s has no access after completing", member.Email)
	}
}

func (server *fakeAPIServer) addMember(t *testing.T, name, email string) (proto.Member, *crypt.Keyset) {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	accountID := crypt.GenerateAccountID()
	member := proto.Member{AccountID: accountID, Email: email, Name: name, Role: proto.RoleMember, PublicKeys: keyset.GetAllPublicKey(accountID)}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.allMember = append(server.allMember, member)
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
	for index := range server.allMember {
		if server.allMember[index].AccountID == accountID {
			server.allMember[index].PublicKeys = keyset.GetAllPublicKey(accountID)
		}
	}
}

func (server *fakeAPIServer) getAccess(vaultID, accountID string) (proto.RotatedAccess, bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for _, access := range server.allAccessByVaultID[vaultID] {
		if access.AccountID == accountID {
			return access, true
		}
	}
	return proto.RotatedAccess{}, false
}

func (server *fakeAPIServer) setVaultRole(vaultID, role string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for index := range server.allVault {
		if server.allVault[index].ID == vaultID {
			server.allVault[index].Role = role
		}
	}
	for index, access := range server.allAccessByVaultID[vaultID] {
		if access.AccountID == server.accountID {
			server.allAccessByVaultID[vaultID][index].Role = role
		}
	}
}

func getCachedVaultKey(t *testing.T, service *Service, vaultID string) []byte {
	t.Helper()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	vault, found := service.allVaultByID[vaultID]
	if !found {
		t.Fatalf("vault %s not cached", vaultID)
	}
	return append([]byte(nil), vault.Key...)
}

func TestShareVaultSendsInvitationAndCompletesOnAccept(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, bobKeyset := test.apiServer.addMember(t, "Bob", "b@example.com")

	if err := service.ShareVault(test.familyID, " B@example.com ", proto.VaultEditor); err != nil {
		t.Fatal(err)
	}
	invitations, err := service.ListAllShareInvitation()
	if err != nil || len(invitations.AllSent) != 1 || invitations.AllSent[0].Email != "b@example.com" || invitations.AllSent[0].VaultName != "Family" ||
		invitations.AllSent[0].State != proto.ShareInvitationPending || invitations.AllSent[0].Role != proto.VaultEditor {
		t.Fatalf("sent invitations: %+v %v", invitations, err)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Fatal("Bob got access before accepting")
	}
	if allPerson, _ := service.ListAllPerson(); len(allPerson) != 1 {
		t.Errorf("people before Bob accepted: %+v", allPerson)
	}

	test.apiServer.acceptShareInvitation(t, bob)
	if !service.syncAllVault(context.Background()) {
		t.Fatal("sync failed")
	}
	access, found := test.apiServer.getAccess(test.familyID, bob.AccountID)
	if !found || access.Role != proto.VaultEditor {
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
	if err != nil || !bytes.Equal(reloadedState.AllPin[bob.AccountID], bob.PublicKeys.Sign) {
		t.Fatalf("pin not saved: %v", err)
	}

	allVaultMember, err := service.ListAllVaultMember(test.familyID)
	if err != nil || len(allVaultMember) != 2 {
		t.Fatalf("vault members: %+v %v", allVaultMember, err)
	}
	if owner := allVaultMember[0]; owner.Name != "Alice" || owner.Role != proto.VaultOwner || !owner.IsCurrentUser {
		t.Errorf("owner = %+v", owner)
	}
	if member := allVaultMember[1]; member.Email != "b@example.com" || member.Role != proto.VaultEditor || member.IsCurrentUser ||
		member.SecurityCode != crypt.FormatKeyFingerprint(bob.PublicKeys.Sign) || member.IsKeyChanged {
		t.Errorf("member = %+v", member)
	}

	allPerson, err := service.ListAllPerson()
	if err != nil || len(allPerson) != 2 || allPerson[0].Name != "Alice" || !allPerson[0].IsCurrentUser || allPerson[0].Role != proto.RoleAdmin {
		t.Fatalf("people: %+v %v", allPerson, err)
	}
	if allPerson[1].SecurityCode != crypt.FormatKeyFingerprint(bob.PublicKeys.Sign) || allPerson[1].IsCurrentUser {
		t.Errorf("Bob = %+v", allPerson[1])
	}

	test.apiServer.replaceMemberKey(t, bob.AccountID)
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, proto.VaultReader); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Errorf("role change after the key changed: %v", err)
	}
	if access, _ := test.apiServer.getAccess(test.familyID, bob.AccountID); access.Role != proto.VaultEditor {
		t.Errorf("role changed despite the key change: %+v", access)
	}
	if allPerson, _ := service.ListAllPerson(); !allPerson[1].IsKeyChanged {
		t.Error("changed key not reported")
	}
}

func TestCompletingAShareRefusesAChangedKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	travelID := createSharedTestVault(t, service, "Travel")
	shareThroughInvitation(t, test, test.familyID, bob, proto.VaultReader)

	test.apiServer.replaceMemberKey(t, bob.AccountID)
	substituted := *test.apiServer.findMember(bob.AccountID)
	if err := service.ShareVault(travelID, "b@example.com", proto.VaultReader); err != nil {
		t.Fatal(err)
	}
	test.apiServer.acceptShareInvitation(t, substituted)
	invitations, err := service.ListAllShareInvitation()
	if err != nil || !strings.Contains(invitations.CompletionError, "signing key of b@example.com changed") ||
		len(invitations.AllSent) != 1 || invitations.AllSent[0].State != proto.ShareInvitationAccepted ||
		invitations.AllSent[0].RecipientName != "Bob" || invitations.AllSent[0].AcceptedAt != "2026-10-11T08:30:00Z" || invitations.AllSent[0].VaultName != "Travel" {
		t.Fatalf("completing with a substituted key: %+v %v", invitations, err)
	}
	if _, found := test.apiServer.getAccess(travelID, bob.AccountID); found {
		t.Error("the vault key was wrapped to a substituted key")
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

	if err := service.ShareVault(test.familyID, "nobody@example.com", proto.VaultReader); err != nil {
		t.Errorf("unknown email: %v", err)
	}
	if err := service.ShareVault(test.familyID, "A@example.com", proto.VaultReader); !errors.Is(err, errShareWithOwnAccount) {
		t.Errorf("share with yourself: %v", err)
	}
	if err := service.ShareVault(test.familyID, "b@example.com", "admin"); err == nil || !strings.Contains(err.Error(), "owner, editor or reader") {
		t.Errorf("invalid role: %v", err)
	}
	for _, badEmail := range []string{"", "bob", "@example.com", "bob@", "Bob <b@example.com>"} {
		if err := service.ShareVault(test.familyID, badEmail, proto.VaultReader); !errors.Is(err, errInvalidEmail) {
			t.Errorf("email %q: %v", badEmail, err)
		}
	}
	shareThroughInvitation(t, test, test.familyID, bob, proto.VaultReader)
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultEditor); !errors.Is(err, errAlreadyHasAccess) {
		t.Errorf("share with a member: %v", err)
	}
	for status, expected := range map[int]error{
		http.StatusNotImplemented:  errEmailNotConfigured,
		http.StatusTooManyRequests: errTooManyInvitation,
		http.StatusNotFound:        errInvitationNotFound,
		http.StatusForbidden:       errOwnerOnly,
	} {
		if err := convertInvitationError(&client.APIError{Status: status, Message: "server message"}); !errors.Is(err, expected) {
			t.Errorf("%d: %v", status, err)
		}
	}
	if err := convertInvitationError(&client.APIError{Status: http.StatusBadRequest, Message: "personal vaults cannot be shared"}); err == nil || err.Error() != "personal vaults cannot be shared" {
		t.Errorf("400: %v", err)
	}
}

func TestShareVaultRefusesPersonalVaultAndNonOwner(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")

	if err := service.ShareVault(test.personalID, "b@example.com", proto.VaultReader); !errors.Is(err, errPersonalVault) {
		t.Errorf("share personal vault: %v", err)
	}
	if err := service.ChangeVaultMemberRole(test.personalID, bob.AccountID, proto.VaultReader); !errors.Is(err, errPersonalVault) {
		t.Errorf("role change in personal vault: %v", err)
	}
	if err := service.RemoveVaultMember(test.personalID, bob.AccountID); !errors.Is(err, errPersonalVault) {
		t.Errorf("remove from personal vault: %v", err)
	}
	if test.apiServer.countShareInvitation() != 0 {
		t.Error("an invitation to the personal vault was sent")
	}

	test.apiServer.setVaultRole(test.familyID, proto.VaultEditor)
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultReader); !errors.Is(err, errOwnerOnly) {
		t.Errorf("share as editor: %v", err)
	}
	if err := service.RemoveVaultMember(test.familyID, bob.AccountID); !errors.Is(err, errOwnerOnly) {
		t.Errorf("remove as editor: %v", err)
	}
	if allVaultMember, err := service.ListAllVaultMember(test.familyID); err != nil || len(allVaultMember) != 1 {
		t.Errorf("editors can list members: %+v %v", allVaultMember, err)
	}
	if err := convertSharingError(&client.APIError{Status: http.StatusForbidden, Message: "owner only"}); !errors.Is(err, errOwnerOnly) {
		t.Errorf("403 from the server: %v", err)
	}
}

func TestReceivedShareInvitationsCanBeAnswered(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	expiresAt := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	test.apiServer.allReceivedShareInvitation = []proto.ReceivedShareInvitation{
		{ID: "from-carol", VaultID: "vault-1", Role: proto.VaultEditor, InvitedByName: "Carol", InvitedByEmail: "c@example.com", ExpiresAt: expiresAt},
		{ID: "from-dave", VaultID: "vault-2", Role: proto.VaultReader, InvitedByName: "Dave", InvitedByEmail: "d@example.com", ExpiresAt: expiresAt},
	}
	invitations, err := service.ListAllShareInvitation()
	if err != nil || len(invitations.AllReceived) != 2 || invitations.AllReceived[0].InvitedByName != "Carol" ||
		invitations.AllReceived[0].ExpiresAt != "2026-10-18T09:00:00Z" || len(invitations.AllSent) != 0 {
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
	if !slices.Equal(test.apiServer.allAnsweredShareInvitation, []string{"from-carol accept", "from-dave decline"}) {
		t.Errorf("answers: %v", test.apiServer.allAnsweredShareInvitation)
	}

	if err := service.ShareVault(test.familyID, "e@example.com", proto.VaultReader); err != nil {
		t.Fatal(err)
	}
	invitations, _ = service.ListAllShareInvitation()
	if len(invitations.AllSent) != 1 {
		t.Fatalf("sent: %+v", invitations.AllSent)
	}
	if err := service.CancelShareInvitation(invitations.AllSent[0].ID); err != nil {
		t.Fatal(err)
	}
	if test.apiServer.countShareInvitation() != 0 {
		t.Error("cancelled invitation still there")
	}
	if err := service.CancelShareInvitation(invitations.AllSent[0].ID); !errors.Is(err, errInvitationNotFound) {
		t.Errorf("cancel twice: %v", err)
	}
}

func TestRecipientSeesWhoTheShareIsWaitingFor(t *testing.T) {
	test := createUnlockedTestService(t)
	acceptedAt := time.Date(2026, 10, 11, 7, 0, 0, 0, time.UTC)
	test.apiServer.allWaitingShareInvitation = []proto.WaitingShareInvitation{
		{ID: "from-carol", VaultID: "vault-1", Role: proto.VaultEditor, InvitedByName: "Carol", InvitedByEmail: "c@example.com", AcceptedAt: acceptedAt},
	}
	invitations, err := test.service.ListAllShareInvitation()
	if err != nil || len(invitations.AllWaiting) != 1 || len(invitations.AllReceived) != 0 {
		t.Fatalf("waiting: %+v %v", invitations, err)
	}
	if waiting := invitations.AllWaiting[0]; waiting.ID != "from-carol" || waiting.InvitedByName != "Carol" || waiting.InvitedByEmail != "c@example.com" ||
		waiting.Role != proto.VaultEditor || waiting.AcceptedAt != "2026-10-11T07:00:00Z" {
		t.Errorf("waiting = %+v", waiting)
	}
	test.apiServer.allWaitingShareInvitation = nil
	if invitations, _ := test.service.ListAllShareInvitation(); invitations.AllWaiting == nil || len(invitations.AllWaiting) != 0 {
		t.Errorf("an old server's missing list: %#v", invitations.AllWaiting)
	}
}

func TestLockedAppListsSharesAwaitingUnlock(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultEditor); err != nil {
		t.Fatal(err)
	}
	test.apiServer.acceptShareInvitation(t, bob)
	if allAwaiting, err := service.ListAllShareAwaitingUnlock(); err != nil || len(allAwaiting) != 0 {
		t.Errorf("unlocked, the app completes shares itself: %+v %v", allAwaiting, err)
	}

	service.Lock()
	allAwaiting, err := service.ListAllShareAwaitingUnlock()
	if err != nil || len(allAwaiting) != 1 {
		t.Fatalf("awaiting unlock: %+v %v", allAwaiting, err)
	}
	if awaiting := allAwaiting[0]; awaiting.RecipientName != "Bob" || awaiting.RecipientEmail != "b@example.com" || awaiting.AcceptedAt != "2026-10-11T08:30:00Z" || awaiting.InvitationID == "" {
		t.Errorf("awaiting = %+v", awaiting)
	}
	if _, found := test.apiServer.getAccess(test.familyID, bob.AccountID); found {
		t.Error("a locked app completed the share")
	}
	if _, err := service.ListAllShareInvitation(); !errors.Is(err, ErrLocked) {
		t.Errorf("full invitation list while locked: %v", err)
	}

	test.apiServer.setUnreachable(true)
	if allAwaiting, err := service.ListAllShareAwaitingUnlock(); err != nil || len(allAwaiting) != 0 {
		t.Errorf("offline: %+v %v", allAwaiting, err)
	}
	test.apiServer.setUnreachable(false)
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	if allAwaiting, err := service.ListAllShareAwaitingUnlock(); err != nil || len(allAwaiting) != 0 {
		t.Errorf("signed out: %+v %v", allAwaiting, err)
	}
}

func TestShareInvitationEventCompletesShare(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultReader); err != nil {
		t.Fatal(err)
	}
	remoteChange := make(chan struct{}, 4)
	service.mutex.Lock()
	service.config.OnRemoteChange = func() { remoteChange <- struct{}{} }
	test.apiServer.eventChannel = make(chan string, 4)
	service.mutex.Unlock()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go service.watchEvents(ctx, service.unlockedSession.Client, make(chan struct{}))

	test.apiServer.acceptShareInvitation(t, bob)
	test.apiServer.eventChannel <- proto.EventShareInvitationsChanged
	select {
	case <-remoteChange:
	case <-time.After(5 * time.Second):
		t.Fatal("no refresh after share-invitations-changed")
	}
	if access, found := test.apiServer.getAccess(test.familyID, bob.AccountID); !found || access.Role != proto.VaultReader {
		t.Errorf("access after the event: %+v %v", access, found)
	}
}

func TestChangeVaultMemberRole(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, _ := test.apiServer.addMember(t, "Carol", "c@example.com")

	shareThroughInvitation(t, test, test.familyID, bob, proto.VaultEditor)
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, proto.VaultReader); err != nil {
		t.Fatal(err)
	}
	if access, _ := test.apiServer.getAccess(test.familyID, bob.AccountID); access.Role != proto.VaultReader {
		t.Errorf("role after change: %+v", access)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, proto.VaultOwner); err != nil {
		t.Fatal(err)
	}
	if allVaultMember, _ := service.ListAllVaultMember(test.familyID); len(allVaultMember) != 2 || allVaultMember[1].Role != proto.VaultOwner {
		t.Errorf("members after promotion: %+v", allVaultMember)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, test.accountID, proto.VaultReader); !errors.Is(err, errOwnAccess) {
		t.Errorf("change own role: %v", err)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, carol.AccountID, proto.VaultReader); err == nil || !strings.Contains(err.Error(), "no longer has access") {
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
	shareThroughInvitation(t, test, test.familyID, bob, proto.VaultEditor)
	shareThroughInvitation(t, test, test.familyID, carol, proto.VaultEditor)
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
	if !found || carolAccess.Role != proto.VaultEditor {
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
	shareThroughInvitation(t, test, test.familyID, dave, proto.VaultReader)
	test.apiServer.replaceMemberKey(t, carol.AccountID)
	if err := service.RemoveVaultMember(test.familyID, dave.AccountID); err == nil || !strings.Contains(err.Error(), "signing key of c@example.com changed") {
		t.Errorf("rotation wrapped the key for a substituted key: %v", err)
	}
	if !bytes.Equal(getCachedVaultKey(t, service, test.familyID), newKey) {
		t.Error("vault key changed although the rotation was refused")
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
	if allPerson, err := service.ListAllPerson(); err != nil || slices.ContainsFunc(allPerson, func(person Person) bool { return person.AccountID == memberAccountID }) {
		t.Fatalf("a stranger is listed: %+v %v", allPerson, err)
	}

	if err := service.ShareVault(familyID, memberEmail, proto.VaultReader); err != nil {
		t.Fatalf("share: %v", err)
	}
	if err := service.ShareVault(familyID, "nobody-"+memberEmail, proto.VaultReader); err != nil {
		t.Fatalf("share with an unknown email: %v", err)
	}
	received, err := member.ListAllShareInvitation()
	if err != nil || len(received.AllReceived) != 1 || received.AllReceived[0].VaultID != familyID || received.AllReceived[0].Role != proto.VaultReader {
		t.Fatalf("member's invitations: %+v %v", received, err)
	}
	if err := member.AcceptShareInvitation(received.AllReceived[0].ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if allMemberVault, err := member.ListAllVault(); err != nil || slices.ContainsFunc(allMemberVault, func(vault VaultSummary) bool { return vault.ID == familyID }) {
		t.Fatalf("member has the vault before the owner completed: %+v %v", allMemberVault, err)
	}
	sent, err := service.ListAllShareInvitation()
	if err != nil || sent.CompletionError != "" || slices.ContainsFunc(sent.AllSent, func(invitation SentShareInvitation) bool { return invitation.Email == memberEmail }) {
		t.Fatalf("owner's invitations after completing: %+v %v", sent, err)
	}
	allPerson, err := service.ListAllPerson()
	if err != nil {
		t.Fatal(err)
	}
	personIndex := slices.IndexFunc(allPerson, func(person Person) bool { return person.AccountID == memberAccountID })
	if personIndex < 0 || allPerson[personIndex].Role != proto.RoleMember || len(allPerson[personIndex].SecurityCode) != 19 {
		t.Fatalf("people: %+v", allPerson)
	}
	allMemberVault, err := member.ListAllVault()
	if err != nil {
		t.Fatal(err)
	}
	if index := slices.IndexFunc(allMemberVault, func(vault VaultSummary) bool { return vault.ID == familyID }); index < 0 || allMemberVault[index].Role != proto.VaultReader {
		t.Fatalf("member's vaults after share: %+v", allMemberVault)
	}
	if fetched, err := member.GetItem(familyID, saved.ID); err != nil || fetched.Password != "shared family password" {
		t.Fatalf("member reads the shared item: %+v %v", fetched, err)
	}
	if err := member.ShareVault(familyID, "desktop@example.com", proto.VaultReader); !errors.Is(err, errOwnerOnly) {
		t.Errorf("reader shares: %v", err)
	}

	if err := service.ChangeVaultMemberRole(familyID, memberAccountID, proto.VaultEditor); err != nil {
		t.Fatalf("change role: %v", err)
	}
	allVaultMember, err := service.ListAllVaultMember(familyID)
	if err != nil || len(allVaultMember) != 2 || !allVaultMember[0].IsCurrentUser || allVaultMember[1].Role != proto.VaultEditor || allVaultMember[1].Email != memberEmail {
		t.Fatalf("vault members: %+v %v", allVaultMember, err)
	}

	if err := service.RemoveVaultMember(familyID, memberAccountID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := member.ListAllItem(familyID); err == nil {
		t.Error("removed member still reads the vault")
	}
	if allMemberVault, err := member.ListAllVault(); err != nil || slices.ContainsFunc(allMemberVault, func(vault VaultSummary) bool { return vault.ID == familyID }) {
		t.Errorf("removed member still lists the vault: %+v %v", allMemberVault, err)
	}
	if fetched, err := service.GetItem(familyID, saved.ID); err != nil || fetched.Password != "shared family password" {
		t.Fatalf("owner reads the item after rotation: %+v %v", fetched, err)
	}
	if err := service.DeleteItem(familyID, saved.ID); err != nil {
		t.Fatalf("delete after rotation: %v", err)
	}
	if sent, err := service.ListAllShareInvitation(); err == nil {
		for _, invitation := range sent.AllSent {
			service.CancelShareInvitation(invitation.ID)
		}
	}

	allDevice, err := service.ListAllDevice()
	if err != nil || len(allDevice) == 0 || !allDevice[0].IsCurrent {
		t.Fatalf("devices: %+v %v", allDevice, err)
	}
	if err := service.RevokeDevice(allDevice[0].ID); !errors.Is(err, errCurrentDevice) {
		t.Errorf("revoke this device: %v", err)
	}
}

func TestInvitePerson(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service

	if err := service.InvitePerson(" New@Example.com "); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(test.apiServer.allInvitedEmail, []string{"new@example.com"}) {
		t.Errorf("invited: %v", test.apiServer.allInvitedEmail)
	}
	if err := service.InvitePerson("not an email"); !errors.Is(err, errInvalidEmail) {
		t.Errorf("invalid email: %v", err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.role = proto.RoleMember
	test.apiServer.mutex.Unlock()
	if err := service.InvitePerson("other@example.com"); err != nil {
		t.Errorf("a member's invite: %v", err)
	}
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
	test.apiServer.allDevice = []proto.Device{
		{ID: "old-phone", Name: "iPhone", Platform: "ios/mobile", LastSeenAt: now.Add(-48 * time.Hour)},
		{ID: "this-mac", Name: "MacBook", Platform: "darwin/desktop", LastSeenAt: now.Add(-time.Hour), Current: true},
		{ID: "laptop", Name: "ThinkPad", Platform: "linux/cli", LastSeenAt: now},
	}

	allDevice, err := service.ListAllDevice()
	if err != nil {
		t.Fatal(err)
	}
	var allID []string
	for _, device := range allDevice {
		allID = append(allID, device.ID)
	}
	if !slices.Equal(allID, []string{"this-mac", "laptop", "old-phone"}) || !allDevice[0].IsCurrent || allDevice[1].IsCurrent {
		t.Fatalf("devices: %+v", allDevice)
	}
	if lastSeen, err := time.Parse(time.RFC3339, allDevice[1].LastSeenAt); err != nil || lastSeen.Unix() != now.Unix() {
		t.Errorf("last seen: %q %v", allDevice[1].LastSeenAt, err)
	}

	if err := service.RevokeDevice("this-mac"); !errors.Is(err, errCurrentDevice) {
		t.Errorf("revoke this device: %v", err)
	}
	if err := service.RevokeDevice("old-phone"); err != nil {
		t.Fatal(err)
	}
	if allDevice, _ := service.ListAllDevice(); len(allDevice) != 2 {
		t.Errorf("devices after revoke: %+v", allDevice)
	}
	if err := service.RevokeDevice("old-phone"); err == nil || !strings.Contains(err.Error(), "already signed out") {
		t.Errorf("revoke twice: %v", err)
	}
}
