package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
)

func (server *fakeAPIServer) setRecoveryWrappedKey(vaultID string, wrappedKey []byte) {
	if server.allRecoveryWrappedKeyByVaultID == nil {
		server.allRecoveryWrappedKeyByVaultID = map[string][]byte{}
	}
	server.allRecoveryWrappedKeyByVaultID[vaultID] = wrappedKey
	for index := range server.allVault {
		if server.allVault[index].ID == vaultID {
			server.allVault[index].Recoverable = wrappedKey != nil
			server.allVault[index].RecoveryOptOut = wrappedKey == nil
		}
	}
}

func (server *fakeAPIServer) addAllRecoveryRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/recovery/keyset", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.recoveryKeyset == nil {
			writeJSON(writer, http.StatusNotFound, proto.Error{Error: "not found"})
			return
		}
		response := *server.recoveryKeyset
		if response.CreatedBy != server.accountID {
			response.EncKeyset = nil
		}
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/recovery/keyset", func(writer http.ResponseWriter, request *http.Request) {
		var createRequest proto.CreateRecoveryKeysetRequest
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, proto.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.role != proto.RoleAdmin {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "admin only"})
			return
		}
		if server.recoveryKeyset != nil {
			writeJSON(writer, http.StatusConflict, proto.Error{Error: "recovery is already set up"})
			return
		}
		server.recoveryKeyset = &proto.RecoveryKeyset{PublicKeys: createRequest.PublicKeys, EncKeyset: createRequest.EncKeyset, CreatedBy: server.accountID}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/recovery", func(writer http.ResponseWriter, request *http.Request) {
		var setRequest proto.SetVaultRecoveryRequest
		if err := json.NewDecoder(request.Body).Decode(&setRequest); err != nil {
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
		server.setRecoveryWrappedKey(vaultID, setRequest.WrappedKey)
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/vaults/{vaultID}/recovery", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) != proto.VaultOwner {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "owner only"})
			return
		}
		server.setRecoveryWrappedKey(vaultID, nil)
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/admin/recovery/{accountID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		accountID := request.PathValue("accountID")
		if server.role != proto.RoleAdmin {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "admin only"})
			return
		}
		if !slices.ContainsFunc(server.allMember, func(member proto.Member) bool { return member.AccountID == accountID }) {
			writeJSON(writer, http.StatusNotFound, proto.Error{Error: "account not found"})
			return
		}
		writeJSON(writer, http.StatusOK, proto.RecoveryLinkResponse{Token: "token", URL: "https://hisar.test/recover/token-" + accountID})
	})
	mux.HandleFunc("GET /api/v1/admin/recovery", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.role != proto.RoleAdmin {
			writeJSON(writer, http.StatusForbidden, proto.Error{Error: "admin only"})
			return
		}
		writeJSON(writer, http.StatusOK, append([]proto.PendingRecovery{}, server.allPendingRecovery...))
	})
	mux.HandleFunc("POST /api/v1/admin/recovery/{accountID}/complete", func(writer http.ResponseWriter, request *http.Request) {
		var completeRequest proto.CompleteRecoveryRequest
		if err := json.NewDecoder(request.Body).Decode(&completeRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, proto.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		accountID := request.PathValue("accountID")
		index := slices.IndexFunc(server.allPendingRecovery, func(pending proto.PendingRecovery) bool { return pending.AccountID == accountID })
		if server.role != proto.RoleAdmin || index < 0 {
			writeJSON(writer, http.StatusNotFound, proto.Error{Error: "no pending recovery for this account"})
			return
		}
		pending := server.allPendingRecovery[index]
		response := proto.CompleteRecoveryResponse{AllUnrecoverableVaultID: []string{}}
		for _, recoveryVault := range pending.AllVault {
			if !slices.ContainsFunc(completeRequest.AllVaultKey, func(vaultKey proto.VaultKey) bool { return vaultKey.VaultID == recoveryVault.VaultID }) {
				response.AllUnrecoverableVaultID = append(response.AllUnrecoverableVaultID, recoveryVault.VaultID)
			}
		}
		for memberIndex := range server.allMember {
			if server.allMember[memberIndex].AccountID == accountID {
				server.allMember[memberIndex].PublicKeys = pending.PublicKeys
			}
		}
		if server.allCompletedVaultKeyByAccount == nil {
			server.allCompletedVaultKeyByAccount = map[string][]proto.VaultKey{}
		}
		server.allCompletedVaultKeyByAccount[accountID] = completeRequest.AllVaultKey
		server.allPendingRecovery = slices.Delete(server.allPendingRecovery, index, index+1)
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/auth/recovery/account", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusForbidden, proto.Error{Error: "invalid recovery link"})
	})
}

// submitRecovery simulates a member who opened their recovery link: new keys, the shared vault with its recovery
// wrapping and a personal vault without one.
func (server *fakeAPIServer) submitRecovery(t *testing.T, member proto.Member, sharedVaultID string) (*crypt.Keyset, string) {
	t.Helper()
	newKeyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	personalVaultID := "8c4d2f6e-3b1a-4c9e-9f0d-2a7b5e1c4d3f"
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.allPendingRecovery = append(server.allPendingRecovery, proto.PendingRecovery{
		AccountID:   member.AccountID,
		Email:       member.Email,
		Name:        member.Name,
		PublicKeys:  newKeyset.GetAllPublicKey(member.AccountID),
		SubmittedAt: time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC),
		AllVault: []proto.RecoveryVault{
			{VaultID: sharedVaultID, Type: proto.VaultShared, WrappedKey: server.allRecoveryWrappedKeyByVaultID[sharedVaultID]},
			{VaultID: personalVaultID, Type: proto.VaultPersonal},
		},
	})
	return newKeyset, personalVaultID
}

func (server *fakeAPIServer) setRole(role string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.role = role
}

func findVaultSummary(t *testing.T, allVault []VaultSummary, vaultID string) VaultSummary {
	t.Helper()
	index := slices.IndexFunc(allVault, func(vault VaultSummary) bool { return vault.ID == vaultID })
	if index < 0 {
		t.Fatalf("vault %s missing from %+v", vaultID, allVault)
	}
	return allVault[index]
}

func TestSetUpRecoveryWrapsOwnedVaults(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service

	status, err := service.GetRecoveryStatus()
	if err != nil || status.IsSetUp || !status.IsAdmin {
		t.Fatalf("status before setup: %+v %v", status, err)
	}
	if _, err := service.EnableRecoveryForAllVault(); !errors.Is(err, errRecoveryNotSetUp) {
		t.Errorf("enable before setup: %v", err)
	}
	if _, err := service.StartAccountRecovery(test.accountID); !errors.Is(err, errRecoverSelf) {
		t.Errorf("start own recovery: %v", err)
	}

	allFailedVaultName, err := service.SetUpRecovery()
	if err != nil || len(allFailedVaultName) != 0 {
		t.Fatalf("set up: %v %v", allFailedVaultName, err)
	}
	status, err = service.GetRecoveryStatus()
	if err != nil || !status.IsSetUp || !status.IsCreatedByCurrentUser || status.CreatedByName != "Alice" {
		t.Fatalf("status after setup: %+v %v", status, err)
	}
	allVault, err := service.ListAllVault()
	if err != nil {
		t.Fatal(err)
	}
	for _, vault := range allVault {
		if !vault.Recoverable || vault.RecoveryOptOut {
			t.Errorf("vault not recoverable after setup: %+v", vault)
		}
	}
	reloadedState, err := localstate.Load(service.config.StatePath)
	if err != nil || !bytes.Equal(reloadedState.AllPin[recoveryPinKey], test.apiServer.recoveryKeyset.PublicKeys.KEM) {
		t.Fatalf("recovery key not pinned: %v", err)
	}

	if allFailedVaultName, err := service.SetUpRecovery(); err != nil || len(allFailedVaultName) != 0 {
		t.Errorf("set up again: %v %v", allFailedVaultName, err)
	}
	created, err := service.CreateVault("Household")
	if err != nil || !created.Recoverable {
		t.Errorf("new vault is not recoverable: %+v %v", created, err)
	}

	test.apiServer.mutex.Lock()
	test.apiServer.recoveryKeyset.PublicKeys.KEM = bytes.Repeat([]byte{1}, len(test.apiServer.recoveryKeyset.PublicKeys.KEM))
	test.apiServer.mutex.Unlock()
	if _, err := service.EnableRecoveryForAllVault(); err == nil {
		t.Error("substituted recovery key accepted")
	}
}

func TestVaultRecoveryOptOut(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	if _, err := service.EnableVaultRecovery(test.personalID); !errors.Is(err, errRecoveryNotSetUp) {
		t.Errorf("enable before setup: %v", err)
	}
	if _, err := service.SetUpRecovery(); err != nil {
		t.Fatal(err)
	}

	disabled, err := service.DisableVaultRecovery(test.personalID)
	if err != nil || disabled.Recoverable || !disabled.RecoveryOptOut || disabled.Name != "Personal" {
		t.Fatalf("disable: %+v %v", disabled, err)
	}
	if _, err := service.DisableVaultRecovery(test.familyID); err == nil || !strings.Contains(err.Error(), "shared vaults") {
		t.Errorf("disable a shared vault: %v", err)
	}
	if _, err := service.EnableRecoveryForAllVault(); err != nil {
		t.Fatal(err)
	}
	allVault, err := service.ListAllVault()
	if err != nil {
		t.Fatal(err)
	}
	if personal := findVaultSummary(t, allVault, test.personalID); personal.Recoverable || !personal.RecoveryOptOut {
		t.Errorf("enabling all vaults undid the opt-out: %+v", personal)
	}

	enabled, err := service.EnableVaultRecovery(test.personalID)
	if err != nil || !enabled.Recoverable || enabled.RecoveryOptOut {
		t.Fatalf("enable: %+v %v", enabled, err)
	}
	allVault, _ = service.ListAllVault()
	if personal := findVaultSummary(t, allVault, test.personalID); !personal.Recoverable {
		t.Errorf("personal vault after enabling: %+v", personal)
	}

	test.apiServer.setVaultRole(test.familyID, proto.VaultEditor)
	if _, err := service.EnableVaultRecovery(test.familyID); err == nil || !strings.Contains(err.Error(), "only owners") {
		t.Errorf("enable as editor: %v", err)
	}
	if _, err := service.EnableVaultRecovery("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing vault: %v", err)
	}
}

func TestCompleteAccountRecovery(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob Stone", "b@example.com")
	if _, err := service.SetUpRecovery(); err != nil {
		t.Fatal(err)
	}
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultEditor); err != nil {
		t.Fatal(err)
	}

	link, err := service.StartAccountRecovery(bob.AccountID)
	if err != nil || link != "https://hisar.test/recover/token-"+bob.AccountID {
		t.Fatalf("start: %q %v", link, err)
	}
	if _, err := service.StartAccountRecovery("NOBODY"); err == nil || !strings.Contains(err.Error(), "no longer has an account") {
		t.Errorf("start for an unknown account: %v", err)
	}
	if allPending, err := service.ListAllPendingRecovery(); err != nil || len(allPending) != 0 {
		t.Fatalf("pending before submission: %+v %v", allPending, err)
	}

	bobNewKeyset, bobPersonalID := test.apiServer.submitRecovery(t, bob, test.familyID)
	allPending, err := service.ListAllPendingRecovery()
	if err != nil || len(allPending) != 1 {
		t.Fatalf("pending: %+v %v", allPending, err)
	}
	if pending := allPending[0]; pending.AccountID != bob.AccountID || pending.Name != "Bob Stone" || pending.Email != "b@example.com" ||
		pending.SubmittedAt != "2026-10-01T09:30:00Z" || pending.RecoverableVaultCount != 1 || pending.UnrecoverableVaultCount != 1 {
		t.Errorf("pending = %+v", pending)
	}

	allUnrecoverableVaultName, err := service.CompleteAccountRecovery(bob.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(allUnrecoverableVaultName, []string{"Bob's personal vault"}) {
		t.Errorf("unrecoverable vaults = %v (personal vault %s)", allUnrecoverableVaultName, bobPersonalID)
	}
	test.apiServer.mutex.Lock()
	allVaultKey := test.apiServer.allCompletedVaultKeyByAccount[bob.AccountID]
	test.apiServer.mutex.Unlock()
	if len(allVaultKey) != 1 || allVaultKey[0].VaultID != test.familyID {
		t.Fatalf("completed vault keys: %+v", allVaultKey)
	}
	unwrappedKey, err := bobNewKeyset.UnwrapVaultKey(test.familyID, bob.AccountID, allVaultKey[0].WrappedKey)
	if err != nil || !bytes.Equal(unwrappedKey, getCachedVaultKey(t, service, test.familyID)) {
		t.Fatalf("Bob's new keys cannot open the family vault: %v", err)
	}
	reloadedState, err := localstate.Load(service.config.StatePath)
	if err != nil || !bytes.Equal(reloadedState.AllPin[bob.AccountID], bobNewKeyset.GetAllPublicKey(bob.AccountID).Sign) {
		t.Fatalf("new signing key not pinned: %v", err)
	}
	if allMember, _ := service.ListAllMember(); allMember[1].IsKeyChanged {
		t.Errorf("recovered member shows a key change: %+v", allMember[1])
	}
	if err := service.ShareVault(test.familyID, "b@example.com", proto.VaultReader); err != nil {
		t.Errorf("share after recovery: %v", err)
	}
	if _, err := service.CompleteAccountRecovery(bob.AccountID); err == nil || !strings.Contains(err.Error(), "no longer pending") {
		t.Errorf("complete twice: %v", err)
	}
}

func TestCompleteAccountRecoveryChecksKeysAndCreator(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	if _, err := service.SetUpRecovery(); err != nil {
		t.Fatal(err)
	}
	test.apiServer.submitRecovery(t, bob, test.familyID)

	test.apiServer.mutex.Lock()
	test.apiServer.allPendingRecovery[0].PublicKeys.KEMSig = bytes.Repeat([]byte{7}, len(test.apiServer.allPendingRecovery[0].PublicKeys.KEMSig))
	test.apiServer.mutex.Unlock()
	if _, err := service.CompleteAccountRecovery(bob.AccountID); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Errorf("forged keys: %v", err)
	}

	test.apiServer.mutex.Lock()
	test.apiServer.allPendingRecovery = nil
	test.apiServer.recoveryKeyset.CreatedBy = bob.AccountID
	test.apiServer.mutex.Unlock()
	test.apiServer.submitRecovery(t, bob, test.familyID)
	status, err := service.GetRecoveryStatus()
	if err != nil || status.IsCreatedByCurrentUser || status.CreatedByName != "Bob" {
		t.Fatalf("status when another admin set up recovery: %+v %v", status, err)
	}
	if _, err := service.CompleteAccountRecovery(bob.AccountID); err == nil {
		t.Error("completed a recovery without being the recovery admin")
	}

	test.apiServer.setRole(proto.RoleMember)
	service.mutex.Lock()
	service.unlockedSession.Role = proto.RoleMember
	service.mutex.Unlock()
	if _, err := service.ListAllPendingRecovery(); !errors.Is(err, errRecoveryAdminOnly) {
		t.Errorf("member lists recoveries: %v", err)
	}
	if _, err := service.StartAccountRecovery(bob.AccountID); !errors.Is(err, errRecoveryAdminOnly) {
		t.Errorf("member starts a recovery: %v", err)
	}
	if _, err := service.SetUpRecovery(); !errors.Is(err, errRecoveryAdminOnly) {
		t.Errorf("member sets up recovery: %v", err)
	}
	if status, err := service.GetRecoveryStatus(); err != nil || status.IsAdmin || !status.IsSetUp {
		t.Errorf("member status: %+v %v", status, err)
	}
}

func TestParseRecoveryLink(t *testing.T) {
	server, token, err := parseRecoveryLink(" https://hisar.example.com/recover/abc123 ")
	if err != nil || server != "https://hisar.example.com" || token != "abc123" {
		t.Errorf("valid link: %q %q %v", server, token, err)
	}
	for _, link := range []string{"https://hisar.example.com/invite/abc", "hisar.example.com/recover/abc", "https://hisar.example.com/recover/", "https://x/recover/a/b"} {
		if _, _, err := parseRecoveryLink(link); err == nil {
			t.Errorf("accepted %q", link)
		}
	}
}

func TestRecoverAccountValidates(t *testing.T) {
	test := createUnlockedTestService(t)
	if _, err := test.service.RecoverAccount("https://hisar.test/recover/token", "b@example.com", "new master password"); !errors.Is(err, errSignedInRecovery) {
		t.Errorf("recover while signed in: %v", err)
	}

	signedOutPath := filepath.Join(t.TempDir(), "signed-out.json")
	service, _, _ := createTestService(t, signedOutPath, createMemorySecretKeyStore())
	allCase := map[[3]string]string{
		{"https://hisar.test/invite/token", "b@example.com", "new master password"}: "recovery link",
		{"https://hisar.test/recover/token", "nobody", "new master password"}:       "email",
		{"https://hisar.test/recover/token", "b@example.com", "short"}:              "at least 10",
	}
	for input, want := range allCase {
		if _, err := service.RecoverAccount(input[0], input[1], input[2]); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: %v, want %q", input, err, want)
		}
	}

	if _, err := service.RecoverAccount(test.service.GetStatus().Server+"/recover/used-token", "b@example.com", "new master password"); !errors.Is(err, errInvalidRecoveryLink) {
		t.Errorf("invalid token: %v", err)
	}
	if status := service.GetStatus(); status.State != StateSignedOut || status.Server != "" {
		t.Errorf("status after a failed recovery: %+v", status)
	}
}

func TestSaveRecoveredAccountEmergencyKitSignedOut(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if _, err := service.SaveNewAccountEmergencyKit(); !errors.Is(err, ErrSignedOut) {
		t.Errorf("kit without a recovered account: %v", err)
	}
	secretKey, err := crypt.GenerateSecretKey(crypt.GenerateAccountID())
	if err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	service.state.Server, service.state.Email = "https://hisar.test", "b@example.com"
	service.recoveredAccountSecretKey = &secretKey
	service.mutex.Unlock()

	fileSaver := service.config.FileSaver.(*fakeFileSaver)
	fileSaver.path = filepath.Join(t.TempDir(), "kit.pdf")
	if savedPath, err := service.SaveNewAccountEmergencyKit(); err != nil || savedPath != fileSaver.path {
		t.Fatalf("save: %q %v", savedPath, err)
	}
	if document, err := os.ReadFile(fileSaver.path); err != nil || !strings.HasPrefix(string(document), "%PDF-") {
		t.Errorf("saved kit is not a PDF: %v", err)
	}
	service.ForgetNewAccountSecretKey()
	if _, err := service.SaveNewAccountEmergencyKit(); !errors.Is(err, ErrSignedOut) {
		t.Errorf("kit after forgetting the recovered Secret Key: %v", err)
	}
}

// checkRecoveryFlow needs the admin who set up recovery on the test server (HISAR_TEST_RECOVERY_ADMIN_EMAIL,
// HISAR_TEST_RECOVERY_ADMIN_SECRET_KEY and HISAR_TEST_RECOVERY_ADMIN_PASSWORD); service is another admin.
func checkRecoveryFlow(t *testing.T, service *Service, serverURL, personalID, familyID string) {
	t.Helper()
	recoveryAdminEmail, recoveryAdminSecretKey, recoveryAdminPassword := os.Getenv("HISAR_TEST_RECOVERY_ADMIN_EMAIL"),
		os.Getenv("HISAR_TEST_RECOVERY_ADMIN_SECRET_KEY"), os.Getenv("HISAR_TEST_RECOVERY_ADMIN_PASSWORD")
	if recoveryAdminEmail == "" || recoveryAdminSecretKey == "" || recoveryAdminPassword == "" {
		t.Log("HISAR_TEST_RECOVERY_ADMIN_* not set; skipping the recovery round trip")
		return
	}

	status, err := service.GetRecoveryStatus()
	if err != nil || !status.IsSetUp || !status.IsAdmin || status.IsCreatedByCurrentUser || status.CreatedByName == "" {
		t.Fatalf("recovery status of the second admin: %+v %v", status, err)
	}
	if allFailedVaultName, err := service.SetUpRecovery(); err != nil || len(allFailedVaultName) != 0 {
		t.Fatalf("set up when another admin already did: %v %v", allFailedVaultName, err)
	}
	allVault, err := service.ListAllVault()
	if err != nil {
		t.Fatal(err)
	}
	for _, vault := range allVault {
		if !vault.Recoverable {
			t.Errorf("vault not recoverable: %+v", vault)
		}
	}
	if disabled, err := service.DisableVaultRecovery(personalID); err != nil || disabled.Recoverable || !disabled.RecoveryOptOut {
		t.Fatalf("opt out: %+v %v", disabled, err)
	}
	if enabled, err := service.EnableVaultRecovery(personalID); err != nil || !enabled.Recoverable {
		t.Fatalf("opt in again: %+v %v", enabled, err)
	}

	const memberEmail, memberPassword, newMemberPassword = "desktop-recovered@example.com", "member master password", "recovered master password"
	inviteLink, err := service.InviteMember(memberEmail, proto.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	member, _, _ := createTestService(t, filepath.Join(t.TempDir(), "recovered.json"), createMemorySecretKeyStore())
	t.Cleanup(func() { member.Lock() })
	signUpResult, err := member.SignUp(inviteLink, memberEmail, "Recovered Member", memberPassword)
	if err != nil {
		t.Fatal(err)
	}
	memberAccountID := member.state.AccountID
	memberVault, err := member.ListAllVault()
	if err != nil || len(memberVault) != 1 || !memberVault[0].Recoverable {
		t.Fatalf("member's personal vault: %+v %v", memberVault, err)
	}
	personalItem, err := member.SaveItem(memberVault[0].ID, ItemDetail{Title: "Bank", Password: "personal bank password"})
	if err != nil {
		t.Fatal(err)
	}
	familyItem, err := service.SaveItem(familyID, ItemDetail{Title: "Wi-Fi", Password: "family wifi password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ShareVault(familyID, memberEmail, proto.VaultReader); err != nil {
		t.Fatal(err)
	}

	recoveryAdmin, _, _ := createTestService(t, filepath.Join(t.TempDir(), "recovery-admin.json"), createMemorySecretKeyStore())
	t.Cleanup(func() { recoveryAdmin.Lock() })
	if _, err := recoveryAdmin.SignIn(serverURL, recoveryAdminEmail, recoveryAdminSecretKey, recoveryAdminPassword); err != nil {
		t.Fatalf("recovery admin signs in: %v", err)
	}
	if status, err := recoveryAdmin.GetRecoveryStatus(); err != nil || !status.IsCreatedByCurrentUser {
		t.Fatalf("recovery admin status: %+v %v", status, err)
	}

	recoveryLink, err := service.StartAccountRecovery(memberAccountID)
	if err != nil || !strings.HasPrefix(recoveryLink, serverURL+"/recover/") {
		t.Fatalf("start recovery: %q %v", recoveryLink, err)
	}
	if _, err := member.StartAccountRecovery(service.state.AccountID); !errors.Is(err, errRecoveryAdminOnly) {
		t.Errorf("member starts a recovery: %v", err)
	}
	if _, err := member.RecoverAccount(recoveryLink, memberEmail, newMemberPassword); !errors.Is(err, errSignedInRecovery) {
		t.Errorf("recover while signed in: %v", err)
	}
	if _, err := member.SignOut(); err != nil {
		t.Fatal(err)
	}
	recovered, err := member.RecoverAccount(recoveryLink, " Desktop-Recovered@example.com ", newMemberPassword)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered.Status.State != StateSignedOut || recovered.Status.Server != serverURL || recovered.Status.Email != memberEmail ||
		!strings.HasPrefix(recovered.SecretKey, "H1-") || recovered.SecretKey == signUpResult.SecretKey {
		t.Fatalf("recover result: %+v", recovered)
	}
	fileSaver := member.config.FileSaver.(*fakeFileSaver)
	fileSaver.path = filepath.Join(t.TempDir(), "recovered-kit.pdf")
	if savedPath, err := member.SaveNewAccountEmergencyKit(); err != nil || savedPath != fileSaver.path {
		t.Errorf("save the recovered account's kit signed out: %q %v", savedPath, err)
	}
	member.ForgetNewAccountSecretKey()
	if _, err := member.RecoverAccount(recoveryLink, memberEmail, newMemberPassword); !errors.Is(err, errInvalidRecoveryLink) {
		t.Errorf("recovery link used twice: %v", err)
	}
	if _, err := member.SignIn(serverURL, memberEmail, recovered.SecretKey, newMemberPassword); err == nil {
		t.Error("signed in with the new Secret Key before the admin completed the recovery")
	}

	allPending, err := service.ListAllPendingRecovery()
	if err != nil {
		t.Fatal(err)
	}
	pendingIndex := slices.IndexFunc(allPending, func(pending PendingRecovery) bool { return pending.AccountID == memberAccountID })
	if pendingIndex < 0 || allPending[pendingIndex].Name != "Recovered Member" || allPending[pendingIndex].RecoverableVaultCount != 2 ||
		allPending[pendingIndex].UnrecoverableVaultCount != 0 {
		t.Fatalf("pending recoveries: %+v", allPending)
	}
	if _, err := service.CompleteAccountRecovery(memberAccountID); err == nil || !strings.Contains(err.Error(), "only the admin who set up recovery") {
		t.Errorf("second admin completes: %v", err)
	}
	allUnrecoverableVaultName, err := recoveryAdmin.CompleteAccountRecovery(memberAccountID)
	if err != nil || len(allUnrecoverableVaultName) != 0 {
		t.Fatalf("complete: %v %v", allUnrecoverableVaultName, err)
	}

	if _, err := member.SignIn(serverURL, memberEmail, signUpResult.SecretKey, memberPassword); err == nil {
		t.Error("old Secret Key still works after the recovery")
	}
	if _, err := member.SignIn(serverURL, memberEmail, recovered.SecretKey, newMemberPassword); err != nil {
		t.Fatalf("sign in with the new Secret Key: %v", err)
	}
	if fetched, err := member.ListAllItem(memberVault[0].ID); err != nil || len(fetched) != 1 {
		t.Fatalf("recovered personal vault: %+v %v", fetched, err)
	}
	if fetched, err := member.GetItem(memberVault[0].ID, personalItem.ID); err != nil || fetched.Password != "personal bank password" {
		t.Errorf("recovered personal item: %+v %v", fetched, err)
	}
	if _, err := member.ListAllItem(familyID); err != nil {
		t.Fatal(err)
	}
	if fetched, err := member.GetItem(familyID, familyItem.ID); err != nil || fetched.Password != "family wifi password" {
		t.Errorf("recovered shared item: %+v %v", fetched, err)
	}
}
