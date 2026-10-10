package vault

import (
	"bytes"
	"encoding/json"
	"errors"
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
		writeJSON(writer, http.StatusOK, server.allMember)
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
		if server.role != proto.RoleAdmin {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "admin only"})
			return
		}
		writeJSON(writer, http.StatusOK, proto.InviteResponse{Token: "token", URL: "https://hisar.test/invite/token-" + inviteRequest.Role})
	})
	mux.HandleFunc("GET /api/v1/devices", func(writer http.ResponseWriter, request *http.Request) {
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

func TestShareVaultPinsSigningKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, bobKeyset := test.apiServer.addMember(t, "Bob", "b@example.com")

	if err := service.ShareVault(test.familyID, " B@example.com ", proto.VaultEditor); err != nil {
		t.Fatal(err)
	}
	access, found := test.apiServer.getAccess(test.familyID, bob.AccountID)
	if !found || access.Role != proto.VaultEditor {
		t.Fatalf("access after share: %+v %v", access, found)
	}
	unwrappedKey, err := bobKeyset.UnwrapVaultKey(test.familyID, bob.AccountID, access.WrappedKey)
	if err != nil || !bytes.Equal(unwrappedKey, getCachedVaultKey(t, service, test.familyID)) {
		t.Fatalf("Bob cannot unwrap the vault key: %v", err)
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

	allMember, err := service.ListAllMember()
	if err != nil || len(allMember) != 2 || allMember[0].Name != "Alice" || !allMember[0].IsCurrentUser || allMember[0].Role != proto.RoleAdmin {
		t.Fatalf("family members: %+v %v", allMember, err)
	}
	if allMember[1].SecurityCode != crypt.FormatKeyFingerprint(bob.PublicKeys.Sign) || allMember[1].IsCurrentUser {
		t.Errorf("Bob = %+v", allMember[1])
	}

	test.apiServer.replaceMemberKey(t, bob.AccountID)
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultReader); err == nil || !strings.Contains(err.Error(), "signing key of b@example.com changed") {
		t.Errorf("share after the key changed: %v", err)
	}
	if err := service.ChangeVaultMemberRole(test.familyID, bob.AccountID, proto.VaultReader); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Errorf("role change after the key changed: %v", err)
	}
	if access, _ := test.apiServer.getAccess(test.familyID, bob.AccountID); access.Role != proto.VaultEditor {
		t.Errorf("role changed despite the key change: %+v", access)
	}
	if allMember, _ := service.ListAllMember(); !allMember[1].IsKeyChanged {
		t.Error("changed key not reported")
	}

	if err := service.ShareVault(test.familyID, "nobody@example.com", proto.VaultReader); !errors.Is(err, errMemberNotFound) {
		t.Errorf("unknown email: %v", err)
	}
	if err := service.ShareVault(test.familyID, "a@example.com", proto.VaultReader); !errors.Is(err, errOwnAccess) {
		t.Errorf("share with yourself: %v", err)
	}
	if err := service.ShareVault(test.familyID, "b@example.com", "admin"); err == nil || !strings.Contains(err.Error(), "owner, editor or reader") {
		t.Errorf("invalid role: %v", err)
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
	if _, found := test.apiServer.getAccess(test.personalID, bob.AccountID); found {
		t.Error("personal vault was shared")
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

func TestChangeVaultMemberRole(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, _ := test.apiServer.addMember(t, "Carol", "c@example.com")

	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultEditor); err != nil {
		t.Fatal(err)
	}
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
	for _, email := range []string{"b@example.com", "c@example.com"} {
		if err := service.ShareVault(test.familyID, email, proto.VaultEditor); err != nil {
			t.Fatal(err)
		}
	}
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

	test.apiServer.replaceMemberKey(t, carol.AccountID)
	dave, _ := test.apiServer.addMember(t, "Dave", "d@example.com")
	if err := service.ShareVault(test.familyID, "d@example.com", proto.VaultReader); err != nil {
		t.Fatal(err)
	}
	if err := service.RemoveVaultMember(test.familyID, dave.AccountID); err == nil || !strings.Contains(err.Error(), "signing key of c@example.com changed") {
		t.Errorf("rotation wrapped the key for a substituted key: %v", err)
	}
	if !bytes.Equal(getCachedVaultKey(t, service, test.familyID), newKey) {
		t.Error("vault key changed although the rotation was refused")
	}
}

// checkSharingFlow runs against a real server: service is an admin who owns the shared vault familyID.
func checkSharingFlow(t *testing.T, service *Service, familyID string) {
	t.Helper()
	const memberEmail = "desktop-member@example.com"
	inviteLink, err := service.InviteMember(memberEmail, proto.RoleMember)
	if err != nil || !strings.Contains(inviteLink, "/invite/") {
		t.Fatalf("invite: %q %v", inviteLink, err)
	}
	member, _, _ := createTestService(t, filepath.Join(t.TempDir(), "member.json"), createMemorySecretKeyStore())
	t.Cleanup(func() { member.Lock() })
	if _, err := member.SignUp(inviteLink, memberEmail, "Desktop Member", "member master password"); err != nil {
		t.Fatalf("sign up with the invite: %v", err)
	}
	memberAccountID := member.state.AccountID
	if _, err := member.InviteMember("someone@example.com", proto.RoleMember); !errors.Is(err, errAdminOnly) {
		t.Errorf("member invites: %v", err)
	}

	saved, err := service.SaveItem(familyID, ItemDetail{Title: "Netflix", Password: "shared family password"})
	if err != nil {
		t.Fatal(err)
	}
	allFamilyMember, err := service.ListAllMember()
	if err != nil {
		t.Fatal(err)
	}
	memberIndex := slices.IndexFunc(allFamilyMember, func(familyMember FamilyMember) bool { return familyMember.AccountID == memberAccountID })
	if memberIndex < 0 || allFamilyMember[memberIndex].Role != proto.RoleMember || len(allFamilyMember[memberIndex].SecurityCode) != 19 {
		t.Fatalf("family members: %+v", allFamilyMember)
	}

	if err := service.ShareVault(familyID, memberEmail, proto.VaultReader); err != nil {
		t.Fatalf("share: %v", err)
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

	allDevice, err := service.ListAllDevice()
	if err != nil || len(allDevice) == 0 || !allDevice[0].IsCurrent {
		t.Fatalf("devices: %+v %v", allDevice, err)
	}
	if err := service.RevokeDevice(allDevice[0].ID); !errors.Is(err, errCurrentDevice) {
		t.Errorf("revoke this device: %v", err)
	}
}

func TestInviteMember(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service

	link, err := service.InviteMember(" new@example.com ", proto.RoleMember)
	if err != nil || link != "https://hisar.test/invite/token-member" {
		t.Fatalf("invite: %q %v", link, err)
	}
	if _, err := service.InviteMember("not an email", proto.RoleMember); err == nil {
		t.Error("invalid email accepted")
	}
	if _, err := service.InviteMember("new@example.com", "owner"); err == nil {
		t.Error("invalid role accepted")
	}

	test.apiServer.mutex.Lock()
	test.apiServer.role = proto.RoleMember
	test.apiServer.mutex.Unlock()
	if _, err := service.InviteMember("new@example.com", proto.RoleMember); !errors.Is(err, errAdminOnly) {
		t.Errorf("server refused the invite: %v", err)
	}

	test.apiServer.mutex.Lock()
	test.apiServer.role = proto.RoleAdmin
	test.apiServer.mutex.Unlock()
	service.mutex.Lock()
	service.unlockedSession.Role = proto.RoleMember
	service.mutex.Unlock()
	if _, err := service.InviteMember("new@example.com", proto.RoleMember); err != nil {
		t.Errorf("promoted to admin after unlocking, invite refused: %v", err)
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
