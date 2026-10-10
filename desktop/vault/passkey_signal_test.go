package vault

import (
	"testing"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/desktop/browser"
)

func sendPasskeySignal(t *testing.T, testBridge *browserTestBridge, token, signalType string, passkeyRequest browser.PasskeyRequest) (int, string) {
	t.Helper()
	result, code := testBridge.sendPasskeyRequest(t, signalType, token, passkeyRequest)
	if code != "" {
		return 0, code
	}
	return result.(browser.PasskeySignalResult).ChangedPasskeyCount, ""
}

func getItemDetail(t *testing.T, service *Service, vaultID, itemID string) ItemDetail {
	t.Helper()
	detail, err := service.GetItem(vaultID, itemID)
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func TestPasskeySignals(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	personalID, familyID := testBridge.testService.personalID, testBridge.testService.familyID
	token := testBridge.pair(t, "chrome-1")
	existing, err := service.SaveItem(personalID, ItemDetail{Title: "Example", Username: "alex@example.com", Password: "pw", Notes: "keep", URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	familyLogin, err := service.SaveItem(familyID, ItemDetail{Title: "Example (family)", Username: "alex", URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	alex := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("user-alex"))
	kim := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("user-kim"))
	register := func(relyingParty *passkeyRelyingParty, passkeyRequest browser.PasskeyRequest) browser.PasskeyCreationResult {
		t.Helper()
		creation, err := relyingParty.register(t, testBridge, token, passkeyRequest)
		if err != nil {
			t.Fatal(err)
		}
		return creation
	}
	inExisting := register(alex, browser.PasskeyRequest{Origin: "https://example.com", UserName: "alex@example.com", VaultID: personalID, ItemID: existing.ID})
	inFamily := register(alex, browser.PasskeyRequest{Origin: "https://example.com", UserName: "alex", VaultID: familyID, ItemID: familyLogin.ID})
	inNewLogin := register(alex, browser.PasskeyRequest{Origin: "https://example.com", UserName: "alex-second"})
	kimPasskey := register(kim, browser.PasskeyRequest{Origin: "https://example.com", UserName: "kim", UserDisplayName: "Kim"})
	otherSite := register(createPasskeyRelyingParty(t, "other.example.net", "https://other.example.net", []byte("user-alex")), browser.PasskeyRequest{Origin: "https://other.example.net", UserName: "alex"})

	for name, refused := range map[string]browser.PasskeyRequest{
		"RP ID of another site":  {Origin: "https://evil.example.net", RelyingPartyID: "example.com", CredentialID: inExisting.CredentialID},
		"public suffix as RP ID": {Origin: "https://example.com", RelyingPartyID: "com", CredentialID: inExisting.CredentialID},
		"no RP ID":               {Origin: "https://example.com", CredentialID: inExisting.CredentialID},
		"plain http":             {Origin: "http://example.com", RelyingPartyID: "example.com", CredentialID: inExisting.CredentialID},
		"credential ID":          {Origin: "https://example.com", RelyingPartyID: "example.com", CredentialID: "*"},
		"empty credential ID":    {Origin: "https://example.com", RelyingPartyID: "example.com"},
	} {
		if _, code := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, refused); code != browser.ErrorInvalidRequest {
			t.Errorf("unknown credential, %s: %s", name, code)
		}
	}
	for name, refused := range map[string]browser.PasskeyRequest{
		"RP ID of another site": {Origin: "https://evil.example.net", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-alex"))},
		"no user handle":        {Origin: "https://example.com", RelyingPartyID: "example.com"},
		"65-byte user handle":   {Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL(make([]byte, 65))},
		"bad accepted ID":       {Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: "AQ", AllAcceptedCredentialID: []string{"*"}},
		"65 accepted IDs":       {Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: "AQ", AllAcceptedCredentialID: make([]string, 65)},
	} {
		if _, code := sendPasskeySignal(t, testBridge, token, PasskeySignalAllAcceptedCredentials, refused); code != browser.ErrorInvalidRequest {
			t.Errorf("all accepted credentials, %s: %s", name, code)
		}
	}
	if _, code := sendPasskeySignal(t, testBridge, token, PasskeySignalCurrentUserDetails, browser.PasskeyRequest{
		Origin: "https://evil.example.net", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-kim")), UserName: "evil",
	}); code != browser.ErrorInvalidRequest {
		t.Errorf("current user details for another site: %s", code)
	}
	if _, code := sendPasskeySignal(t, testBridge, token, PasskeySignalCurrentUserDetails, browser.PasskeyRequest{
		Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-kim")), UserName: string(make([]rune, 257)),
	}); code != browser.ErrorInvalidRequest {
		t.Errorf("long user name: %s", code)
	}
	if _, code := sendPasskeySignal(t, testBridge, "", PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", CredentialID: inExisting.CredentialID}); code != browser.ErrorNotPaired {
		t.Errorf("without token: %s", code)
	}
	if len(getCachedItem(t, service, personalID, existing.ID).Details.Passkeys) != 1 {
		t.Fatal("a refused signal changed the item")
	}

	if count, code := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", CredentialID: otherSite.CredentialID}); code != "" || count != 0 {
		t.Errorf("unknown credential of another RP ID: %d %s", count, code)
	}
	if count, _ := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://other.example.net", RelyingPartyID: "other.example.net", CredentialID: inExisting.CredentialID}); count != 0 {
		t.Errorf("another site removed a passkey of example.com: %d", count)
	}

	beforeRevision := getItemDetail(t, service, personalID, existing.ID).Revision
	count, code := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com", CredentialID: inExisting.CredentialID})
	if code != "" || count != 1 {
		t.Fatalf("unknown credential: %d %s", count, code)
	}
	afterUnknown := getItemDetail(t, service, personalID, existing.ID)
	if len(afterUnknown.AllPasskey) != 0 || afterUnknown.Password != "pw" || afterUnknown.Notes != "keep" || afterUnknown.Username != "alex@example.com" || afterUnknown.Title != "Example" || afterUnknown.Revision == beforeRevision {
		t.Errorf("item after the unknown credential signal: %+v", afterUnknown)
	}
	if cachedItem := getCachedItem(t, service, personalID, existing.ID); cachedItem.Overview.PasskeyRelyingPartyIDs != nil {
		t.Errorf("overview still marks a passkey: %+v", cachedItem.Overview)
	}
	if stored := testBridge.testService.apiServer.allItemByVaultID[personalID][existing.ID]; stored.Revision != afterUnknown.Revision {
		t.Errorf("the change did not reach the server: %s, want %s", stored.Revision, afterUnknown.Revision)
	}
	if err := alex.signIn(t, testBridge, token, browser.PasskeyRequest{Origin: "https://example.com", CredentialID: inExisting.CredentialID}); err == nil || err.Error() != browser.ErrorNotFound {
		t.Errorf("sign in with the removed passkey: %v", err)
	}
	if count, _ := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", CredentialID: inExisting.CredentialID}); count != 0 || getItemDetail(t, service, personalID, existing.ID).Revision != afterUnknown.Revision {
		t.Errorf("repeated signal saved again: %d", count)
	}

	service.mutex.Lock()
	familyRole := service.allVaultByID[familyID].Role
	service.allVaultByID[familyID].Role = proto.VaultReader
	service.mutex.Unlock()
	if count, _ := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", CredentialID: inFamily.CredentialID}); count != 0 {
		t.Errorf("changed a passkey in a read-only vault: %d", count)
	}
	service.mutex.Lock()
	service.allVaultByID[familyID].Role = familyRole
	service.mutex.Unlock()

	kimRevision := getItemDetail(t, service, personalID, kimPasskey.ItemID).Revision
	count, code = sendPasskeySignal(t, testBridge, token, PasskeySignalAllAcceptedCredentials, browser.PasskeyRequest{
		Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-alex")), AllAcceptedCredentialID: []string{inNewLogin.CredentialID, kimPasskey.CredentialID},
	})
	if code != "" || count != 1 {
		t.Fatalf("all accepted credentials: %d %s", count, code)
	}
	if familyDetail := getItemDetail(t, service, familyID, familyLogin.ID); len(familyDetail.AllPasskey) != 0 || familyDetail.Title != "Example (family)" {
		t.Errorf("family login after all accepted credentials: %+v", familyDetail)
	}
	if newLogin := getItemDetail(t, service, personalID, inNewLogin.ItemID); len(newLogin.AllPasskey) != 1 {
		t.Errorf("accepted passkey removed: %+v", newLogin)
	}
	if getItemDetail(t, service, personalID, kimPasskey.ItemID).Revision != kimRevision {
		t.Error("another user's passkey item was saved")
	}
	if otherSiteDetail := getItemDetail(t, service, personalID, otherSite.ItemID); len(otherSiteDetail.AllPasskey) != 1 {
		t.Errorf("passkey of another RP ID with the same user handle removed: %+v", otherSiteDetail)
	}
	count, _ = sendPasskeySignal(t, testBridge, token, PasskeySignalAllAcceptedCredentials, browser.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-alex"))})
	if newLogin := getItemDetail(t, service, personalID, inNewLogin.ItemID); count != 1 || len(newLogin.AllPasskey) != 0 || newLogin.Title != "example.com" || newLogin.Username != "alex-second" {
		t.Errorf("an empty accepted list: %d %+v", count, newLogin)
	}

	kimDetails := browser.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-kim")), UserName: "  kim@example.com ", UserDisplayName: "Kim Example"}
	count, code = sendPasskeySignal(t, testBridge, token, PasskeySignalCurrentUserDetails, kimDetails)
	kimItem := getItemDetail(t, service, personalID, kimPasskey.ItemID)
	if code != "" || count != 1 || kimItem.AllPasskey[0].UserName != "kim@example.com" || kimItem.AllPasskey[0].UserDisplayName != "Kim Example" || kimItem.Username != "kim" || kimItem.Revision == kimRevision {
		t.Errorf("current user details: %d %s %+v", count, code, kimItem)
	}
	if storedPasskey := getCachedItem(t, service, personalID, kimPasskey.ItemID).Details.Passkeys[0]; encodeBase64URL(storedPasskey.CredentialID) != kimPasskey.CredentialID || len(storedPasskey.PrivateKey) == 0 {
		t.Errorf("the update changed the credential: %+v", storedPasskey)
	}
	if count, _ := sendPasskeySignal(t, testBridge, token, PasskeySignalCurrentUserDetails, kimDetails); count != 0 || getItemDetail(t, service, personalID, kimPasskey.ItemID).Revision != kimItem.Revision {
		t.Errorf("unchanged user details saved again: %d", count)
	}
	if err := kim.signIn(t, testBridge, token, browser.PasskeyRequest{Origin: "https://example.com", CredentialID: kimPasskey.CredentialID}); err != nil {
		t.Errorf("sign in after the user details changed: %v", err)
	}
}

func TestPasskeySignalsWhileLocked(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	token := testBridge.pair(t, "chrome-1")
	creation, err := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("user-alex")).register(t, testBridge, token, browser.PasskeyRequest{Origin: "https://example.com", UserName: "alex"})
	if err != nil {
		t.Fatal(err)
	}
	beforeLock := testBridge.testService.apiServer.allItemByVaultID[creation.VaultID][creation.ItemID]
	service.Lock()
	for signalType, passkeyRequest := range map[string]browser.PasskeyRequest{
		PasskeySignalUnknownCredential:      {Origin: "https://example.com", RelyingPartyID: "example.com", CredentialID: creation.CredentialID},
		PasskeySignalAllAcceptedCredentials: {Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-alex"))},
		PasskeySignalCurrentUserDetails:     {Origin: "https://example.com", RelyingPartyID: "example.com", UserHandle: encodeBase64URL([]byte("user-alex")), UserName: "renamed"},
	} {
		if count, code := sendPasskeySignal(t, testBridge, token, signalType, passkeyRequest); code != "" || count != 0 {
			t.Errorf("%s while locked: %d %s", signalType, count, code)
		}
	}
	if _, code := sendPasskeySignal(t, testBridge, token, PasskeySignalUnknownCredential, browser.PasskeyRequest{Origin: "https://evil.example.net", RelyingPartyID: "example.com", CredentialID: creation.CredentialID}); code != browser.ErrorInvalidRequest {
		t.Errorf("wrong RP ID while locked: %s", code)
	}
	if afterLock := testBridge.testService.apiServer.allItemByVaultID[creation.VaultID][creation.ItemID]; afterLock.Revision != beforeLock.Revision {
		t.Error("a signal changed the item while locked")
	}
}
