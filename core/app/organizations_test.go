package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

func (server *fakeAPIServer) addOrganizationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/vault-key-requests", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		response := apitypes.VaultKeyRequestList{Pending: []apitypes.VaultKeyRequest{}, Waiting: append([]apitypes.WaitingVaultKey{}, server.waitingVaultKeys...)}
		for _, keyRequest := range server.vaultKeyRequests {
			if server.getVaultRole(keyRequest.VaultID) == apitypes.VaultOwner {
				response.Pending = append(response.Pending, keyRequest)
			}
		}
		writeJSON(writer, http.StatusOK, response)
	})
}

func (server *fakeAPIServer) getKeyGeneration(vaultID string) int64 {
	for _, vault := range server.vaults {
		if vault.ID == vaultID {
			return vault.KeyGeneration
		}
	}
	return 0
}

func (server *fakeAPIServer) findVaultKeyRequest(vaultID, accountID string) int {
	return slices.IndexFunc(server.vaultKeyRequests, func(keyRequest apitypes.VaultKeyRequest) bool {
		return keyRequest.VaultID == vaultID && keyRequest.Recipient.AccountID == accountID
	})
}

// requestVaultKey plays an admin adding recipient to a team that has the vault.
func (server *fakeAPIServer) requestVaultKey(vaultID, role string, keyGeneration int64, recipient apitypes.Member) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.vaultKeyRequests = append(server.vaultKeyRequests, apitypes.VaultKeyRequest{VaultID: vaultID, Role: role, KeyGeneration: keyGeneration, Recipient: recipient})
}

func (server *fakeAPIServer) countVaultKeyRequest() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return len(server.vaultKeyRequests)
}

func (server *fakeAPIServer) countSetAccessRequest() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return len(server.setAccessRequests)
}

func (server *fakeAPIServer) getVault(vaultID string) apitypes.Vault {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for _, vault := range server.vaults {
		if vault.ID == vaultID {
			return vault
		}
	}
	return apitypes.Vault{}
}

// addTeamAccess plays a key delivered earlier by another owner's app.
func (server *fakeAPIServer) addTeamAccess(t *testing.T, vaultID string, member apitypes.Member, role string, vaultKey []byte) {
	t.Helper()
	wrappedKey, err := crypt.WrapVaultKey(member.PublicKeys.KEM, vaultID, member.AccountID, vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.accessesByVaultID[vaultID] = append(server.accessesByVaultID[vaultID], apitypes.RotatedAccess{AccountID: member.AccountID, Role: role, WrappedKey: wrappedKey})
}

func createOrganizationTestVault(t *testing.T, service *Service) string {
	t.Helper()
	summary, err := service.CreateOrganizationVault("organization-1", "team-1", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Type != apitypes.VaultOrganization || summary.OrganizationID != "organization-1" || summary.Role != apitypes.VaultOwner {
		t.Fatalf("organization vault %+v", summary)
	}
	return summary.ID
}

func TestVaultKeyRequestIsWrappedForTheRecipient(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, bobKeyset := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, test.service)
	test.apiServer.requestVaultKey(vaultID, apitypes.VaultEditor, 1, bob)

	keys, err := test.service.ListWaitingVaultKeys()
	if err != nil || keys.DeliveryError != "" || keys.RotationError != "" {
		t.Fatalf("delivering: %+v %v", keys, err)
	}
	access, found := test.apiServer.getAccess(vaultID, bob.AccountID)
	if !found || access.Role != apitypes.VaultEditor {
		t.Fatalf("Bob's access %+v %v", access, found)
	}
	unwrappedKey, err := bobKeyset.UnwrapVaultKey(vaultID, bob.AccountID, access.WrappedKey)
	if err != nil || !bytes.Equal(unwrappedKey, getCachedVaultKey(t, test.service, vaultID)) {
		t.Errorf("Bob's keyset does not open the vault key: %v", err)
	}
	test.apiServer.mutex.Lock()
	sent := test.apiServer.setAccessRequests[len(test.apiServer.setAccessRequests)-1]
	test.apiServer.mutex.Unlock()
	if sent.Role != apitypes.VaultEditor || sent.KeyGeneration != 1 {
		t.Errorf("sent %+v, want the request's role and key generation", sent)
	}
	if test.apiServer.countVaultKeyRequest() != 0 {
		t.Error("the key request is still pending")
	}
}

func TestVaultKeyRequestRefusesUnverifiedAndChangedKeys(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, _ := test.apiServer.addMember(t, "Carol", "c@example.com")
	vaultID := createOrganizationTestVault(t, test.service)

	otherKeyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	forgedBob := bob
	forgedBob.PublicKeys.KEM = otherKeyset.GetPublicKeys(bob.AccountID).KEM
	test.service.mutex.Lock()
	pinErr := test.service.state.CheckMemberPin(&carol)
	test.service.mutex.Unlock()
	if pinErr != nil {
		t.Fatal(pinErr)
	}
	replacedCarol := carol
	replacedCarol.PublicKeys = otherKeyset.GetPublicKeys(carol.AccountID)
	test.apiServer.requestVaultKey(vaultID, apitypes.VaultReader, 1, forgedBob)
	test.apiServer.requestVaultKey(vaultID, apitypes.VaultReader, 1, replacedCarol)

	keys, err := test.service.ListWaitingVaultKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(keys.DeliveryError, "b@example.com") || !strings.Contains(keys.DeliveryError, "signing key of c@example.com changed") {
		t.Errorf("delivery error %q", keys.DeliveryError)
	}
	for _, member := range []apitypes.Member{bob, carol} {
		if _, found := test.apiServer.getAccess(vaultID, member.AccountID); found {
			t.Errorf("the vault key was wrapped for %s", member.Email)
		}
	}
	if test.apiServer.countVaultKeyRequest() != 2 {
		t.Error("a refused key request was dropped")
	}
}

func TestVaultKeyRequestSavesThePinWhenTheDeliveryFails(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, test.service)
	test.apiServer.requestVaultKey(vaultID, apitypes.VaultEditor, 1, bob)
	test.apiServer.mutex.Lock()
	test.apiServer.isSetAccessFailing = true
	test.apiServer.mutex.Unlock()

	keys, err := test.service.ListWaitingVaultKeys()
	if err != nil || !strings.Contains(keys.DeliveryError, "b@example.com") {
		t.Fatalf("a delivery the server refused: %+v %v", keys, err)
	}
	saved, err := localstate.Load(test.service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Pins[bob.AccountID], bob.PublicKeys.Sign) {
		t.Errorf("state.json pins %x for Bob, want his signing key", saved.Pins[bob.AccountID])
	}
}

func TestVaultKeyRequestReportsPinsThatCouldNotBeSaved(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, test.service)
	test.apiServer.requestVaultKey(vaultID, apitypes.VaultEditor, 1, bob)
	unblockStateSaves := blockStateSaves(t, test.service.config.StatePath)

	keys, err := test.service.ListWaitingVaultKeys()
	if err != nil || !strings.Contains(keys.DeliveryError, "could not save the pinned signing keys") || keys.RotationError != "" {
		t.Fatalf("a pin that could not be saved was not reported: %+v %v", keys, err)
	}
	if _, found := test.apiServer.getAccess(vaultID, bob.AccountID); !found {
		t.Error("the vault key was not delivered")
	}
	// The key is delivered, so the next pass pins nothing new; Bob's pin is still unsaved.
	if keys, err := test.service.ListWaitingVaultKeys(); err != nil || !strings.Contains(keys.DeliveryError, "could not save the pinned signing keys") || keys.RotationError != "" {
		t.Fatalf("a pass that pinned nothing forgot the unsaved pin: %+v %v", keys, err)
	}

	unblockStateSaves()
	if keys, err := test.service.ListWaitingVaultKeys(); err != nil || keys.DeliveryError != "" || keys.RotationError != "" {
		t.Fatalf("after a save that works: %+v %v", keys, err)
	}
	saved, err := localstate.Load(test.service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Pins[bob.AccountID], bob.PublicKeys.Sign) {
		t.Errorf("state.json pins %x for Bob, want his signing key", saved.Pins[bob.AccountID])
	}
}

func TestVaultKeyRequestForAnotherKeyGenerationWaits(t *testing.T) {
	test := createUnlockedTestService(t)
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, test.service)
	sentBefore := test.apiServer.countSetAccessRequest()
	test.apiServer.requestVaultKey(vaultID, apitypes.VaultReader, 2, bob)

	keys, err := test.service.ListWaitingVaultKeys()
	if err != nil || keys.DeliveryError != "" {
		t.Fatalf("%+v %v", keys, err)
	}
	if test.apiServer.countSetAccessRequest() != sentBefore {
		t.Error("a key of another generation was sent")
	}
	if _, found := test.apiServer.getAccess(vaultID, bob.AccountID); found {
		t.Error("Bob got a key of another generation")
	}
}

func TestRotatePendingVaultsKeepsEveryMemberAndRole(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, bobKeyset := test.apiServer.addMember(t, "Bob", "b@example.com")
	carol, carolKeyset := test.apiServer.addMember(t, "Carol", "c@example.com")
	vaultID := createOrganizationTestVault(t, service)
	if _, err := service.SaveItem(vaultID, ItemDetail{Title: "Deploy key", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	oldKey := getCachedVaultKey(t, service, vaultID)
	test.apiServer.addTeamAccess(t, vaultID, bob, apitypes.VaultEditor, oldKey)
	test.apiServer.addTeamAccess(t, vaultID, carol, apitypes.VaultReader, oldKey)
	test.apiServer.mutex.Lock()
	for index := range test.apiServer.vaults {
		if test.apiServer.vaults[index].ID == vaultID {
			test.apiServer.vaults[index].NeedsRotation = true
		}
	}
	test.apiServer.mutex.Unlock()

	if !service.syncVaults(context.Background()) {
		t.Fatal("sync failed")
	}
	if keys, err := service.ListWaitingVaultKeys(); err != nil || keys.RotationError != "" {
		t.Fatalf("rotation: %+v %v", keys, err)
	}
	newKey := getCachedVaultKey(t, service, vaultID)
	if bytes.Equal(newKey, oldKey) {
		t.Fatal("the vault key was not rotated")
	}
	wantRoles := map[string]string{test.accountID: apitypes.VaultOwner, bob.AccountID: apitypes.VaultEditor, carol.AccountID: apitypes.VaultReader}
	test.apiServer.mutex.Lock()
	accesses := append([]apitypes.RotatedAccess{}, test.apiServer.accessesByVaultID[vaultID]...)
	test.apiServer.mutex.Unlock()
	if len(accesses) != len(wantRoles) {
		t.Fatalf("accesses after rotation %+v", accesses)
	}
	keysetsByID := map[string]*crypt.Keyset{bob.AccountID: bobKeyset, carol.AccountID: carolKeyset}
	for _, access := range accesses {
		if access.Role != wantRoles[access.AccountID] {
			t.Errorf("%s has %s after rotation, want %s", access.AccountID, access.Role, wantRoles[access.AccountID])
		}
		if keyset := keysetsByID[access.AccountID]; keyset != nil {
			if unwrappedKey, err := keyset.UnwrapVaultKey(vaultID, access.AccountID, access.WrappedKey); err != nil || !bytes.Equal(unwrappedKey, newKey) {
				t.Errorf("%s does not get the new key: %v", access.AccountID, err)
			}
		}
	}
	if serverVault := test.apiServer.getVault(vaultID); serverVault.NeedsRotation || serverVault.KeyGeneration != 2 {
		t.Errorf("server vault after rotation %+v", serverVault)
	}
	details, err := service.ListItems(vaultID)
	if err != nil || len(details) != 1 || details[0].Title != "Deploy key" {
		t.Errorf("items after rotation %+v %v", details, err)
	}

	service.mutex.Lock()
	unlockedSession := service.unlockedSession
	service.mutex.Unlock()
	vault, err := unlockedSession.FindVault(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	vault.NeedsRotation = true
	grants := []session.AccessGrant{{AccountID: test.accountID, Role: apitypes.VaultOwner}, {AccountID: bob.AccountID, Role: apitypes.VaultEditor}, {AccountID: carol.AccountID, Role: apitypes.VaultReader}}
	if err := unlockedSession.RotateVaultKey(context.Background(), vault, grants); err != nil {
		t.Fatal(err)
	}
	if vault.KeyGeneration != 3 || vault.NeedsRotation {
		t.Errorf("local vault after rotation: generation %d, needs rotation %v", vault.KeyGeneration, vault.NeedsRotation)
	}
}

func TestOrganizationVaultCannotBeLeftOrUnshared(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, service)
	test.apiServer.addTeamAccess(t, vaultID, bob, apitypes.VaultEditor, getCachedVaultKey(t, service, vaultID))

	actions, err := service.GetVaultActions(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if actions.CanLeave || actions.LeaveBlockedReason != session.OrganizationVaultLeaveReason || !actions.CanRename || !actions.CanDelete {
		t.Errorf("actions %+v", actions)
	}
	if err := service.LeaveVault(vaultID); !errors.Is(err, session.ErrOrganizationVaultAccess) {
		t.Errorf("leave: %v", err)
	}
	if err := service.RemoveVaultMember(vaultID, bob.AccountID); !errors.Is(err, session.ErrOrganizationVaultAccess) {
		t.Errorf("remove Bob: %v", err)
	}
	if err := service.ShareVault(vaultID, "c@example.com", apitypes.VaultReader); !errors.Is(err, session.ErrOrganizationVaultAccess) {
		t.Errorf("share: %v", err)
	}
	if _, found := test.apiServer.getAccess(vaultID, test.accountID); !found {
		t.Error("the owner lost access")
	}
	if _, found := test.apiServer.getAccess(vaultID, bob.AccountID); !found {
		t.Error("Bob was removed")
	}
}

func TestOrganizationsChangedEventDeliversKeys(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, service)
	remoteChange := make(chan struct{}, 4)
	service.mutex.Lock()
	service.config.OnRemoteChange = func() { remoteChange <- struct{}{} }
	test.apiServer.eventChannel = make(chan string, 4)
	service.mutex.Unlock()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go service.watchEvents(ctx, service.unlockedSession.Client, make(chan struct{}))

	test.apiServer.requestVaultKey(vaultID, apitypes.VaultReader, 1, bob)
	test.apiServer.eventChannel <- apitypes.EventOrganizationsChanged
	select {
	case <-remoteChange:
	case <-time.After(5 * time.Second):
		t.Fatal("no sync after organizations-changed")
	}
	if access, found := test.apiServer.getAccess(vaultID, bob.AccountID); !found || access.Role != apitypes.VaultReader {
		t.Errorf("Bob's access after the event: %+v %v", access, found)
	}
}

func TestCommandLineCompletesOrganizationKeys(t *testing.T) {
	test := createCommandLineTest(t)
	apiServer := test.testService.apiServer
	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureOrganizations) {
		t.Errorf("hello features = %v", hello.Features)
	}

	var created cliprotocol.VaultResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeCreateOrganizationVault, OrganizationID: "organization-1", TeamID: "team-1", Name: "Engineering"}, &created)
	if created.Vault.OrganizationID != "organization-1" || created.Vault.Type != apitypes.VaultOrganization {
		t.Fatalf("created %+v", created)
	}
	bob, _ := apiServer.addMember(t, "Bob", "b@example.com")
	apiServer.requestVaultKey(created.Vault.ID, apitypes.VaultEditor, 1, bob)
	apiServer.mutex.Lock()
	apiServer.waitingVaultKeys = []apitypes.WaitingVaultKey{{VaultID: "vault-elsewhere", OrganizationID: "organization-1", Role: apitypes.VaultReader}}
	apiServer.mutex.Unlock()

	var keys cliprotocol.OrganizationKeysResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeCompleteOrganizationKeys}, &keys)
	if keys.DeliveredCount != 1 || keys.RotatedCount != 0 || keys.DeliveryError != "" || len(keys.Waiting) != 1 || keys.Waiting[0].VaultID != "vault-elsewhere" {
		t.Errorf("complete keys %+v", keys)
	}
	if _, found := apiServer.getAccess(created.Vault.ID, bob.AccountID); !found {
		t.Error("Bob has no key")
	}
}

// TestItemWritesAfterAnotherOwnerRotated: the server refuses items sealed with a key another owner's rotation replaced,
// so the app reloads the vault and seals them again with the current key, which every member opens.
func TestItemWritesAfterAnotherOwnerRotated(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	ctx := context.Background()
	bob, bobKeyset := test.apiServer.addMember(t, "Bob", "b@example.com")
	vaultID := createOrganizationTestVault(t, service)
	note, err := service.SaveItem(test.personalID, ItemDetail{Title: "Deploy note", Notes: "rotate quarterly"})
	if err != nil {
		t.Fatal(err)
	}
	test.apiServer.addTeamAccess(t, vaultID, bob, apitypes.VaultEditor, getCachedVaultKey(t, service, vaultID))
	service.mutex.Lock()
	unlockedSession := service.unlockedSession
	service.mutex.Unlock()
	// Another owner's app rotates the key and gives Bob the new one; this app still holds the vault under the replaced key.
	rotateAsAnotherOwner := func() {
		t.Helper()
		otherOwnersVault, err := unlockedSession.FindVault(ctx, vaultID)
		if err != nil {
			t.Fatal(err)
		}
		grants := []session.AccessGrant{{AccountID: test.accountID, Role: apitypes.VaultOwner}, {AccountID: bob.AccountID, Role: apitypes.VaultEditor}}
		if err := unlockedSession.RotateVaultKey(ctx, otherOwnersVault, grants); err != nil {
			t.Fatal(err)
		}
	}
	readAsBob := func() []item.Item {
		t.Helper()
		access, _ := test.apiServer.getAccess(vaultID, bob.AccountID)
		bobKey, err := bobKeyset.UnwrapVaultKey(vaultID, bob.AccountID, access.WrappedKey)
		if err != nil {
			t.Fatal(err)
		}
		bobSession := &session.Session{Client: unlockedSession.Client, AccountID: bob.AccountID, Keyset: bobKeyset}
		items, err := bobSession.ListItems(ctx, &session.Vault{ID: vaultID, Type: apitypes.VaultOrganization, Role: apitypes.VaultEditor, Key: bobKey})
		if err != nil {
			t.Fatalf("Bob opens the vault: %v", err)
		}
		return items
	}

	rotateAsAnotherOwner()
	saved, err := service.SaveItem(vaultID, ItemDetail{Title: "Staging database", Password: "staging-password"})
	if err != nil {
		t.Fatalf("saving a new item after the rotation: %v", err)
	}
	if items := readAsBob(); len(items) != 1 || items[0].ID != saved.ID || items[0].Details.Password != "staging-password" {
		t.Fatalf("Bob reads %+v", items)
	}
	rotateAsAnotherOwner()
	if _, err := service.MoveItem(test.personalID, note.ID, vaultID); err != nil {
		t.Fatalf("moving an item in after the rotation: %v", err)
	}
	if items := readAsBob(); len(items) != 2 || items[0].ID != note.ID || items[0].Details.Notes != "rotate quarterly" {
		t.Fatalf("Bob reads %+v", items)
	}

	// While rotations keep coming, the save is refused with a sentence and nothing is lost: trying again saves the item.
	test.apiServer.mutex.Lock()
	test.apiServer.keyChangeRefusalCount = 2
	test.apiServer.mutex.Unlock()
	production := ItemDetail{Title: "Production database", Password: "production-password"}
	if _, err := service.SaveItem(vaultID, production); err == nil || err.Error() != "the vault's key changed on another device; try again" {
		t.Fatalf("saving while the key keeps changing: %v", err)
	}
	if items := readAsBob(); len(items) != 2 {
		t.Errorf("the refused save stored %+v", items)
	}
	if retried, err := service.SaveItem(vaultID, production); err != nil || retried.Password != "production-password" {
		t.Errorf("trying again: %+v %v", retried, err)
	}
}

// TestEditsAfterAnotherOwnerRotated: a rotation gives every item a new revision, so an edit, a deletion or a move out
// made with the replaced key is retried on the rotation's revision when the item is unchanged, and is a conflict
// when someone really changed it.
func TestEditsAfterAnotherOwnerRotated(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	ctx := context.Background()
	vaultID := createOrganizationTestVault(t, service)
	service.mutex.Lock()
	unlockedSession := service.unlockedSession
	service.mutex.Unlock()
	currentVault := func() *session.Vault {
		t.Helper()
		vault, err := unlockedSession.FindVault(ctx, vaultID)
		if err != nil {
			t.Fatal(err)
		}
		return vault
	}
	rotateAsAnotherOwner := func() {
		t.Helper()
		if err := unlockedSession.RotateVaultKey(ctx, currentVault(), []session.AccessGrant{{AccountID: test.accountID, Role: apitypes.VaultOwner}}); err != nil {
			t.Fatal(err)
		}
	}
	storedItems := func() map[string]item.Item {
		t.Helper()
		items, err := unlockedSession.ListItems(ctx, currentVault())
		if err != nil {
			t.Fatal(err)
		}
		itemsByID := map[string]item.Item{}
		for _, vaultItem := range items {
			itemsByID[vaultItem.ID] = vaultItem
		}
		return itemsByID
	}
	save := func(detail ItemDetail) ItemDetail {
		t.Helper()
		saved, err := service.SaveItem(vaultID, detail)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	database := save(ItemDetail{Title: "Database", Password: "first-password", Tags: []string{"prod"}})
	runbook := save(ItemDetail{Title: "Runbook", Notes: "restart the workers"})
	token := save(ItemDetail{Title: "Token", Password: "token-password"})
	shared := save(ItemDetail{Title: "Shared login", Password: "before"})

	rotateAsAnotherOwner()
	database.Password = "second-password"
	if _, err := service.SaveItem(vaultID, database); err != nil {
		t.Fatalf("editing after the rotation: %v", err)
	}
	if stored := storedItems()[database.ID]; stored.Details.Password != "second-password" || len(storedItems()) != 4 {
		t.Fatalf("after the edit the vault holds %+v", storedItems())
	}

	rotateAsAnotherOwner()
	if err := service.DeleteItem(vaultID, runbook.ID); err != nil {
		t.Fatalf("deleting after the rotation: %v", err)
	}
	if _, isFound := storedItems()[runbook.ID]; isFound {
		t.Fatal("the deleted runbook is still there")
	}

	rotateAsAnotherOwner()
	if _, err := service.MoveItem(vaultID, token.ID, test.personalID); err != nil {
		t.Fatalf("moving out after the rotation: %v", err)
	}
	if _, isFound := storedItems()[token.ID]; isFound {
		t.Fatal("the moved token is still in the organization vault")
	}

	otherCopy, isFound := storedItems()[shared.ID]
	if !isFound {
		t.Fatal("the shared login is missing")
	}
	otherCopy.Details.Password = "changed elsewhere"
	if _, err := unlockedSession.SaveItem(ctx, currentVault(), &otherCopy); err != nil {
		t.Fatal(err)
	}
	rotateAsAnotherOwner()
	shared.Password = "changed here"
	if _, err := service.SaveItem(vaultID, shared); err == nil || !strings.Contains(err.Error(), "changed on another device") {
		t.Fatalf("an edit of an item someone changed before the rotation: %v", err)
	}
	if stored := storedItems()[shared.ID]; stored.Details.Password != "changed elsewhere" {
		t.Errorf("the other edit was replaced: %+v", stored)
	}
}

// TestVaultsThisAppCreatesCountKeyGenerations: a shared vault starts at generation 1 and a rotation counts up, so a
// save after it carries the current generation and the server checks it.
func TestVaultsThisAppCreatesCountKeyGenerations(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	ctx := context.Background()
	summary, err := service.CreateVault("Trip")
	if err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	vault := service.vaultsByID[summary.ID]
	unlockedSession := service.unlockedSession
	service.mutex.Unlock()
	if vault.KeyGeneration != 1 {
		t.Fatalf("a new shared vault has generation %d", vault.KeyGeneration)
	}
	if err := unlockedSession.RotateVaultKey(ctx, vault, []session.AccessGrant{{AccountID: test.accountID, Role: apitypes.VaultOwner}}); err != nil {
		t.Fatal(err)
	}
	if vault.KeyGeneration != 2 || test.apiServer.getKeyGeneration(summary.ID) != 2 {
		t.Fatalf("after a rotation: local generation %d, server %d", vault.KeyGeneration, test.apiServer.getKeyGeneration(summary.ID))
	}
	test.apiServer.mutex.Lock()
	test.apiServer.keyGenerationsSent = nil
	test.apiServer.mutex.Unlock()
	if _, err := service.SaveItem(summary.ID, ItemDetail{Title: "Hotel", Password: "hotel-password"}); err != nil {
		t.Fatal(err)
	}
	test.apiServer.mutex.Lock()
	sent := slices.Clone(test.apiServer.keyGenerationsSent)
	test.apiServer.mutex.Unlock()
	if !slices.Equal(sent, []int64{2}) {
		t.Errorf("the save sent generations %v", sent)
	}
}
