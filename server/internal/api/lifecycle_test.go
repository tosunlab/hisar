package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
)

type lifecycleAccount struct {
	session   *session.Session
	password  []byte
	secretKey crypt.SecretKey
}

func signUpLifecycleAccount(t *testing.T, server *Server, baseURL, email string) *lifecycleAccount {
	t.Helper()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(context.Background(), email, apitypes.RoleMember, nil))
	password := []byte(email + " password")
	signedUp, secretKey, err := session.SignUp(context.Background(), apiclient.New(baseURL), invite.Token, email, strings.Split(email, "@")[0], password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	return &lifecycleAccount{session: signedUp, password: password, secretKey: secretKey}
}

func setupLifecycleServer(t *testing.T) (string, *Server, string, *mailtest.Recorder) {
	baseURL, server, databaseURL := setupServer(t)
	server.service.Limiters.Reauthentication = ratelimit.New(rate.Inf, 1)
	return baseURL, server, databaseURL, attachMailRecorder(server)
}

func findLatestEmail(t *testing.T, server *Server, recorder *mailtest.Recorder, recipient, subject string) string {
	t.Helper()
	server.WaitForBackgroundWork()
	messages := recorder.ListMessages()
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].To == recipient && messages[index].Subject == subject {
			return messages[index].Body
		}
	}
	t.Fatalf("no %q email to %s", subject, recipient)
	return ""
}

func readItemTitles(t *testing.T, signedIn *session.Session, vaultName string) []string {
	t.Helper()
	ctx := context.Background()
	vault := requireValue[*session.Vault](t)(signedIn.FindVault(ctx, vaultName))
	titles := []string{}
	for _, vaultItem := range requireValue[[]item.Item](t)(signedIn.ListItems(ctx, vault)) {
		titles = append(titles, vaultItem.Overview.Title+"="+vaultItem.Details.Password)
	}
	slices.Sort(titles)
	return titles
}

func TestChangeMasterPassword(t *testing.T) {
	baseURL, server, _, recorder := setupLifecycleServer(t)
	ctx := context.Background()
	alice := signUpLifecycleAccount(t, server, baseURL, "alice@example.com")
	bob := signUpLifecycleAccount(t, server, baseURL, "bob@example.com")
	personal := requireValue[*session.Vault](t)(alice.session.FindVault(ctx, "Personal"))
	requireValue[*item.Item](t)(alice.session.SaveItem(ctx, personal, &item.Item{Overview: item.Overview{Title: "GitHub"}, Details: item.Details{Password: "gh-pass"}}))
	team := requireValue[*session.Vault](t)(alice.session.CreateVault(ctx, "Team", apitypes.VaultShared))
	requireValue[*item.Item](t)(alice.session.SaveItem(ctx, team, &item.Item{Overview: item.Overview{Title: "AWS"}, Details: item.Details{Password: "aws-pass"}}))
	shareThroughInvitation(t, alice.session, bob.session, team, apitypes.VaultEditor)
	laptop := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alice.password, alice.secretKey, apitypes.DeviceInfo{Name: "laptop", Platform: "go"}))
	laptopEvents := openEventStream(t, laptop.Client)
	waitForSubscriber(t, server, alice.session.AccountID, 1)
	newPassword := []byte("a brand new master password")

	if _, err := alice.session.ChangeMasterPassword(ctx, []byte("not the password"), newPassword, alice.secretKey); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("wrong current password: %v", err)
	}
	if _, err := alice.session.ChangeMasterPassword(ctx, alice.password, []byte("short"), alice.secretKey); !errors.Is(err, session.ErrPasswordTooWeak) {
		t.Errorf("short new password: %v", err)
	}
	if _, err := alice.session.ChangeMasterPassword(ctx, alice.password, alice.password, alice.secretKey); !errors.Is(err, session.ErrSamePassword) {
		t.Errorf("same password: %v", err)
	}

	change, err := alice.session.ChangeMasterPassword(ctx, alice.password, newPassword, alice.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(change.AccountUnlockKey)
	if change.SignedOutDeviceCount != 1 || change.SecretKey.Encode() != alice.secretKey.Encode() {
		t.Errorf("change: %d devices signed out, Secret Key changed: %v", change.SignedOutDeviceCount, change.SecretKey.Encode() != alice.secretKey.Encode())
	}

	select {
	case <-laptopEvents.finished:
	case <-time.After(5 * time.Second):
		t.Error("the other device's event stream stayed open")
	}
	if _, err := laptop.Client.ListVaults(ctx); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("other device after the change: %v", err)
	}
	if devices := requireValue[[]apitypes.Device](t)(alice.session.Client.ListDevices(ctx)); len(devices) != 1 || !devices[0].Current {
		t.Errorf("devices after the change: %+v", devices)
	}
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alice.password, alice.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("old password still signs in: %v", err)
	}
	if _, err := session.Unlock(ctx, alice.session.Client, nil, alice.password, alice.secretKey); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("old password still unlocks: %v", err)
	}
	unlocked := requireValue[*session.Session](t)(session.UnlockWithAccountUnlockKey(ctx, alice.session.Client, nil, change.AccountUnlockKey, alice.session.AccountID))
	if !bytes.Equal(unlocked.Keyset.GetPublicKeys(unlocked.AccountID).Sign, alice.session.Keyset.GetPublicKeys(alice.session.AccountID).Sign) {
		t.Error("the keyset changed")
	}
	signedIn := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", newPassword, alice.secretKey, testDevice))
	if titles := readItemTitles(t, signedIn, "Personal"); !slices.Equal(titles, []string{"GitHub=gh-pass"}) {
		t.Errorf("personal items after the change: %v", titles)
	}
	if titles := readItemTitles(t, signedIn, "Team"); !slices.Equal(titles, []string{"AWS=aws-pass"}) {
		t.Errorf("shared items after the change: %v", titles)
	}
	if titles := readItemTitles(t, bob.session, "Team"); !slices.Equal(titles, []string{"AWS=aws-pass"}) {
		t.Errorf("the other member's items after the change: %v", titles)
	}
	aliceMember := requireValue[*apitypes.Member](t)(bob.session.GetVerifiedMember(ctx, "alice@example.com"))
	if !bytes.Equal(aliceMember.PublicKeys.Sign, alice.session.Keyset.GetPublicKeys(alice.session.AccountID).Sign) {
		t.Error("alice's public keys changed, which would break pinned keys")
	}

	body := findLatestEmail(t, server, recorder, "alice@example.com", "Your Hisar master password was changed")
	if !strings.Contains(body, "Your other device was signed out.") || strings.Contains(body, string(newPassword)) {
		t.Errorf("email: %s", body)
	}
}

func TestReplaceSecretKey(t *testing.T) {
	baseURL, server, _, recorder := setupLifecycleServer(t)
	ctx := context.Background()
	alice := signUpLifecycleAccount(t, server, baseURL, "alice@example.com")
	bob := signUpLifecycleAccount(t, server, baseURL, "bob@example.com")
	team := requireValue[*session.Vault](t)(bob.session.CreateVault(ctx, "Team", apitypes.VaultShared))
	requireValue[*item.Item](t)(bob.session.SaveItem(ctx, team, &item.Item{Overview: item.Overview{Title: "Deploy"}, Details: item.Details{Password: "deploy-pass"}}))
	shareThroughInvitation(t, bob.session, alice.session, team, apitypes.VaultReader)

	if _, err := alice.session.ReplaceSecretKey(ctx, []byte("wrong"), alice.secretKey); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("wrong password: %v", err)
	}
	change, err := alice.session.ReplaceSecretKey(ctx, alice.password, alice.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	clear(change.AccountUnlockKey)
	if change.SecretKey.Encode() == alice.secretKey.Encode() || change.SecretKey.AccountID != alice.secretKey.AccountID {
		t.Fatalf("new Secret Key: %s", change.SecretKey.Encode())
	}
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alice.password, alice.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("old Emergency Kit still signs in: %v", err)
	}
	signedIn := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alice.password, change.SecretKey, testDevice))
	if titles := readItemTitles(t, signedIn, "Team"); !slices.Equal(titles, []string{"Deploy=deploy-pass"}) {
		t.Errorf("shared items after replacing the Secret Key: %v", titles)
	}
	if titles := readItemTitles(t, signedIn, "Personal"); len(titles) != 0 {
		t.Errorf("personal items: %v", titles)
	}
	if _, err := alice.session.ReplaceSecretKey(ctx, alice.password, alice.secretKey); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("replacing again with the old Secret Key: %v", err)
	}
	body := findLatestEmail(t, server, recorder, "alice@example.com", "Your Hisar Secret Key was replaced")
	if strings.Contains(body, change.SecretKey.Encode()) || !strings.Contains(body, "Emergency Kit") {
		t.Errorf("email: %s", body)
	}
}

func startReauthentication(t *testing.T, apiClient *apiclient.Client, password []byte, secretKey crypt.SecretKey) (*apitypes.ReauthenticateResponse, []byte) {
	t.Helper()
	ctx := context.Background()
	account := requireValue[*apitypes.Account](t)(apiClient.GetAccount(ctx))
	derivedKeys := requireValue[*crypt.DerivedKeys](t)(crypt.DeriveKeys(password, account.Salt, account.Argon2, secretKey))
	defer derivedKeys.Wipe()
	opaqueClient, err := pake.NewConfig().Client()
	if err != nil {
		t.Fatal(err)
	}
	keyExchange1, err := opaqueClient.GenerateKE1(derivedKeys.AuthKey)
	if err != nil {
		t.Fatal(err)
	}
	response := requireValue[*apitypes.ReauthenticateResponse](t)(apiClient.Reauthenticate(ctx, &apitypes.ReauthenticateRequest{KE1: keyExchange1.Serialize()}))
	keyExchange2, err := opaqueClient.Deserialize.KE2(response.KE2)
	if err != nil {
		t.Fatal(err)
	}
	keyExchange3, _, _, err := opaqueClient.GenerateKE3(keyExchange2, []byte(account.AccountID), pake.ServerIdentity())
	if err != nil {
		return response, []byte("not a proof")
	}
	return response, keyExchange3.Serialize()
}

func TestReauthenticationIsBoundToItsSession(t *testing.T) {
	baseURL, server, databaseURL, _ := setupLifecycleServer(t)
	ctx := context.Background()
	alice := signUpLifecycleAccount(t, server, baseURL, "alice@example.com")
	laptop := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alice.password, alice.secretKey, testDevice))

	reauthentication, keyExchange3 := startReauthentication(t, alice.session.Client, alice.password, alice.secretKey)
	request := &apitypes.DeleteAccountRequest{ReauthenticationID: reauthentication.ReauthenticationID, KE3: keyExchange3, Email: "alice@example.com"}
	if err := laptop.Client.DeleteAccount(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("proof used by another session: %v", err)
	}

	reauthentication, keyExchange3 = startReauthentication(t, alice.session.Client, alice.password, alice.secretKey)
	if _, err := alice.session.Client.FinishLogin(ctx, &apitypes.LoginFinishRequest{LoginID: reauthentication.ReauthenticationID, KE3: keyExchange3, Device: testDevice}); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("a re-authentication created a session: %v", err)
	}

	reauthentication, keyExchange3 = startReauthentication(t, alice.session.Client, []byte("wrong password"), alice.secretKey)
	request = &apitypes.DeleteAccountRequest{ReauthenticationID: reauthentication.ReauthenticationID, KE3: keyExchange3, Email: "alice@example.com"}
	if err := alice.session.Client.DeleteAccount(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("a wrong proof: %v", err)
	}

	reauthentication, keyExchange3 = startReauthentication(t, alice.session.Client, alice.password, alice.secretKey)
	request = &apitypes.DeleteAccountRequest{ReauthenticationID: reauthentication.ReauthenticationID, KE3: keyExchange3, Email: "someone@example.com"}
	if err := alice.session.Client.DeleteAccount(ctx, request); !apiclient.IsStatus(err, http.StatusBadRequest) {
		t.Errorf("another email: %v", err)
	}
	if _, err := alice.session.Client.GetAccount(ctx); err != nil {
		t.Fatalf("a refused deletion removed the account: %v", err)
	}

	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
	defer connection.Close(ctx)
	var failureCount int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE type = 'reauthentication_failed' AND account_id = $1`, alice.session.AccountID).Scan(&failureCount); err != nil || failureCount != 1 {
		t.Errorf("audited failures: %d %v", failureCount, err)
	}

	server.service.Limiters.Reauthentication = service.NewReauthenticationLimiter()
	var lastErr error
	for range 6 {
		_, lastErr = alice.session.Client.Reauthenticate(ctx, &apitypes.ReauthenticateRequest{KE1: []byte{1}})
	}
	if !apiclient.IsStatus(lastErr, http.StatusTooManyRequests) {
		t.Errorf("re-authentication is not rate limited: %v", lastErr)
	}
}

func TestDeleteAccount(t *testing.T) {
	baseURL, server, databaseURL, recorder := setupLifecycleServer(t)
	ctx := context.Background()
	alice := signUpLifecycleAccount(t, server, baseURL, "alice@example.com")
	bob := signUpLifecycleAccount(t, server, baseURL, "bob@example.com")
	carol := signUpLifecycleAccount(t, server, baseURL, "carol@example.com")

	family := requireValue[*session.Vault](t)(alice.session.CreateVault(ctx, "Family", apitypes.VaultShared))
	requireValue[*item.Item](t)(alice.session.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Wi-Fi"}, Details: item.Details{Password: "wifi"}}))
	shareThroughInvitation(t, alice.session, bob.session, family, apitypes.VaultEditor)
	shareThroughInvitation(t, alice.session, carol.session, family, apitypes.VaultReader)
	team := requireValue[*session.Vault](t)(alice.session.CreateVault(ctx, "Team", apitypes.VaultShared))
	requireValue[*item.Item](t)(alice.session.SaveItem(ctx, team, &item.Item{Overview: item.Overview{Title: "CI"}, Details: item.Details{Password: "ci-pass"}}))
	shareThroughInvitation(t, alice.session, carol.session, team, apitypes.VaultOwner)
	bobVault := requireValue[*session.Vault](t)(bob.session.CreateVault(ctx, "Bob's", apitypes.VaultShared))
	shareThroughInvitation(t, bob.session, alice.session, bobVault, apitypes.VaultEditor)
	requireValue[*session.ShareLink](t)(alice.session.ShareText(ctx, "Note", "text", session.ShareLinkOptions{Expiry: time.Hour, MaxViews: 1}))
	if err := alice.session.InviteToVault(ctx, family, "dave@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}

	preview := requireValue[*session.AccountDeletionPreview](t)(alice.session.PreviewAccountDeletion(ctx))
	ownedNames := []string{}
	for _, ownedVault := range preview.OwnedVaults {
		ownedNames = append(ownedNames, ownedVault.Name)
		if ownedVault.Name == "Family" && ownedVault.OtherMemberCount != 2 {
			t.Errorf("Family members losing access: %d", ownedVault.OtherMemberCount)
		}
	}
	slices.Sort(ownedNames)
	memberNames := []string{}
	for _, memberVault := range preview.MemberVaults {
		memberNames = append(memberNames, memberVault.Name+":"+memberVault.Role)
	}
	slices.Sort(memberNames)
	if !slices.Equal(ownedNames, []string{"Family", "Personal"}) || !slices.Equal(memberNames, []string{"Bob's:editor", "Team:owner"}) {
		t.Errorf("preview vaults: owned %v, member %v", ownedNames, memberNames)
	}
	if preview.ShareLinkCount != 1 || preview.SentInvitationCount != 1 || preview.DeviceCount != 1 {
		t.Errorf("preview counts: %+v", preview)
	}

	if err := alice.session.DeleteAccount(ctx, alice.password, alice.secretKey, "bob@example.com"); !errors.Is(err, session.ErrEmailMismatch) {
		t.Errorf("another email: %v", err)
	}
	if err := alice.session.DeleteAccount(ctx, []byte("wrong"), alice.secretKey, "alice@example.com"); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("wrong password: %v", err)
	}

	bobEvents := openEventStream(t, bob.session.Client)
	carolEvents := openEventStream(t, carol.session.Client)
	aliceEvents := openEventStream(t, alice.session.Client)
	waitForSubscriber(t, server, bob.session.AccountID, 1)
	waitForSubscriber(t, server, carol.session.AccountID, 1)
	waitForSubscriber(t, server, alice.session.AccountID, 1)

	if err := alice.session.DeleteAccount(ctx, alice.password, alice.secretKey, " Alice@Example.com "); err != nil {
		t.Fatal(err)
	}
	bobEvents.waitForEvent(t, apitypes.EventVaultsChanged)
	carolEvents.waitForEvent(t, apitypes.EventVaultsChanged)
	select {
	case <-aliceEvents.finished:
	case <-time.After(5 * time.Second):
		t.Error("the deleted account's event stream stayed open")
	}

	if _, err := alice.session.Client.GetAccount(ctx); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("deleted account's session: %v", err)
	}
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alice.password, alice.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("deleted account signs in: %v", err)
	}
	if _, err := bob.session.FindVault(ctx, "Family"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("bob still has the deleted vault: %v", err)
	}
	if titles := readItemTitles(t, carol.session, "Team"); !slices.Equal(titles, []string{"CI=ci-pass"}) {
		t.Errorf("the co-owner's items: %v", titles)
	}
	if members := requireValue[[]apitypes.VaultMember](t)(bob.session.Client.ListVaultMembers(ctx, bobVault.ID)); len(members) != 1 {
		t.Errorf("bob's vault members: %+v", members)
	}
	invitationList := requireValue[*apitypes.ShareInvitationList](t)(bob.session.Client.ListShareInvitations(ctx))
	if len(invitationList.Sent) != 0 {
		t.Errorf("invitations bob sees: %+v", invitationList)
	}

	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
	defer connection.Close(ctx)
	for name, query := range map[string]string{
		"accounts":          `SELECT count(*) FROM accounts WHERE id = $1`,
		"devices":           `SELECT count(*) FROM devices WHERE account_id = $1`,
		"vault access":      `SELECT count(*) FROM vault_access WHERE account_id = $1`,
		"share links":       `SELECT count(*) FROM share_links WHERE account_id = $1`,
		"share invitations": `SELECT count(*) FROM share_invitations WHERE invited_by = $1 OR accepted_by = $1`,
	} {
		var rowCount int
		if err := connection.QueryRow(ctx, query, alice.session.AccountID).Scan(&rowCount); err != nil || rowCount != 0 {
			t.Errorf("%s left: %d %v", name, rowCount, err)
		}
	}
	var deletionAuditCount int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE type = 'account_deleted' AND account_id = $1 AND detail::text NOT LIKE '%alice%'`, alice.session.AccountID).Scan(&deletionAuditCount); err != nil || deletionAuditCount != 1 {
		t.Errorf("deletion audit: %d %v", deletionAuditCount, err)
	}

	body := findLatestEmail(t, server, recorder, "alice@example.com", "Your Hisar account was deleted")
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("confirmation email: %s", body)
	}

	again := signUpLifecycleAccount(t, server, baseURL, "alice@example.com")
	if again.session.AccountID == alice.session.AccountID && again.secretKey.Encode() == alice.secretKey.Encode() {
		t.Error("signing up again reused the old Secret Key")
	}
}
