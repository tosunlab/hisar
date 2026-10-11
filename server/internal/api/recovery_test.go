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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

func findPendingRecovery(t *testing.T, admin *session.Session, accountID string) *proto.PendingRecovery {
	t.Helper()
	allPendingRecovery, err := admin.ListAllPendingRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for index := range allPendingRecovery {
		if allPendingRecovery[index].AccountID == accountID {
			return &allPendingRecovery[index]
		}
	}
	return nil
}

func readItemPassword(t *testing.T, currentSession *session.Session, vaultName, title string) string {
	t.Helper()
	ctx := context.Background()
	vault := requireValue[*session.Vault](t)(currentSession.FindVault(ctx, vaultName))
	for _, vaultItem := range requireValue[[]item.Item](t)(currentSession.ListAllItem(ctx, vault)) {
		if vaultItem.Overview.Title == title {
			return vaultItem.Details.Password
		}
	}
	t.Fatalf("%s has no item %q", vaultName, title)
	return ""
}

func TestRecovery(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
	defer connection.Close(ctx)

	alicePassword := []byte("alice password")
	aliceInvite := requireValue[*proto.InviteResponse](t)(server.CreateInvite(ctx, "alice@example.com", proto.RoleAdmin, nil))
	alice, aliceSecretKey, err := session.SignUp(ctx, client.Create(baseURL), aliceInvite.Token, "alice@example.com", "Alice", alicePassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alice.GetRecoveryPublicKey(ctx); !errors.Is(err, session.ErrNoRecoveryKeyset) {
		t.Fatalf("recovery key before setup: %v", err)
	}

	recorder := attachMailRecorder(server)
	requireValue[*proto.InvitationSentResponse](t)(alice.Client.Invite(ctx, &proto.InviteRequest{Email: "bob@example.com"}))
	bobPassword := []byte("bob password")
	bob, bobSecretKey, err := session.SignUp(ctx, client.Create(baseURL), findInviteToken(t, server, recorder, "bob@example.com"), "bob@example.com", "Bob", bobPassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}

	if err := bob.CreateRecoveryKeyset(ctx); !client.IsStatus(err, http.StatusForbidden) {
		t.Errorf("member created the recovery keyset: %v", err)
	}
	if err := alice.CreateRecoveryKeyset(ctx); err != nil {
		t.Fatal(err)
	}
	if err := alice.CreateRecoveryKeyset(ctx); !client.IsStatus(err, http.StatusConflict) {
		t.Errorf("second recovery keyset: %v", err)
	}
	recoveryPublicKeys := requireValue[*crypt.PublicKeys](t)(bob.GetRecoveryPublicKey(ctx))
	if err := recoveryPublicKeys.Verify(crypt.RecoveryRecipientID); err != nil {
		t.Fatal(err)
	}
	if bobKeyset := requireValue[*proto.RecoveryKeyset](t)(bob.Client.GetRecoveryKeyset(ctx)); bobKeyset.EncKeyset != nil {
		t.Error("the encrypted recovery keyset was sent to a member")
	} else if bobKeyset.Creator == nil || bobKeyset.Creator.AccountID != alice.AccountID || bobKeyset.Creator.Role != proto.RoleAdmin {
		t.Errorf("recovery keyset creator: %+v", bobKeyset.Creator)
	}
	if _, err := bob.GetVerifiedMember(ctx, "alice@example.com"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("the recovery admin is listed to a member who shares no vault with them: %v", err)
	}

	t.Run("existing vaults are enabled after setup", func(t *testing.T) {
		allWithoutRecovery := requireValue[[]session.Vault](t)(alice.ListAllVaultWithoutRecovery(ctx))
		if len(allWithoutRecovery) != 1 || allWithoutRecovery[0].Name != "Personal" {
			t.Fatalf("alice vaults without recovery: %+v", allWithoutRecovery)
		}
		if allEnabled := requireValue[[]session.Vault](t)(alice.EnableRecoveryForAllVault(ctx)); len(allEnabled) != 1 {
			t.Errorf("alice enabled %d vaults", len(allEnabled))
		}
		if allEnabled := requireValue[[]session.Vault](t)(bob.EnableRecoveryForAllVault(ctx)); len(allEnabled) != 1 || allEnabled[0].Name != "Personal" {
			t.Errorf("bob enabled: %+v", allEnabled)
		}
		if allWithoutRecovery := requireValue[[]session.Vault](t)(bob.ListAllVaultWithoutRecovery(ctx)); len(allWithoutRecovery) != 0 {
			t.Errorf("bob vaults without recovery: %+v", allWithoutRecovery)
		}
	})

	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	if !family.Recoverable {
		t.Error("new shared vault is not wrapped to recovery")
	}
	requireValue[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Netflix"}, Details: item.Details{Password: "family-pass"}}))
	shareThroughInvitation(t, alice, bob, family, proto.VaultEditor)
	bobPersonal := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Personal"))
	requireValue[*item.Item](t)(bob.SaveItem(ctx, bobPersonal, &item.Item{Overview: item.Overview{Title: "Diary"}, Details: item.Details{Password: "diary-pass"}}))
	bobSecret := requireValue[*session.Vault](t)(bob.CreateVault(ctx, "Secret", proto.VaultPersonal))
	if !bobSecret.Recoverable {
		t.Error("new personal vault is not wrapped to recovery by default")
	}
	requireValue[*item.Item](t)(bob.SaveItem(ctx, bobSecret, &item.Item{Overview: item.Overview{Title: "Hidden"}, Details: item.Details{Password: "hidden-pass"}}))

	t.Run("opt-out", func(t *testing.T) {
		bobFamily := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Family"))
		if err := bob.DisableVaultRecovery(ctx, bobFamily); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("editor opted a shared vault out: %v", err)
		}
		if err := alice.DisableVaultRecovery(ctx, family); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("owner opted a shared vault out: %v", err)
		}
		if err := bob.Client.CreateVault(ctx, &proto.CreateVaultRequest{ID: uuid.NewString(), Type: proto.VaultShared, EncName: []byte{1}, WrappedKey: []byte{1}}); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("shared vault created without a recovery wrapping: %v", err)
		}
		if err := bob.DisableVaultRecovery(ctx, bobSecret); err != nil {
			t.Fatal(err)
		}
		allWithoutRecovery := requireValue[[]session.Vault](t)(bob.ListAllVaultWithoutRecovery(ctx))
		if len(allWithoutRecovery) != 1 || allWithoutRecovery[0].ID != bobSecret.ID || !allWithoutRecovery[0].RecoveryOptOut {
			t.Errorf("bob vaults without recovery: %+v", allWithoutRecovery)
		}
		if allEnabled := requireValue[[]session.Vault](t)(bob.EnableRecoveryForAllVault(ctx)); len(allEnabled) != 0 {
			t.Errorf("enabling all vaults undid the opt-out: %+v", allEnabled)
		}
	})

	t.Run("admin only", func(t *testing.T) {
		if _, err := bob.Client.CreateRecoveryLink(ctx, alice.AccountID); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("member started a recovery: %v", err)
		}
		if _, err := bob.ListAllPendingRecovery(ctx); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("member listed recoveries: %v", err)
		}
		if _, err := bob.Client.CompleteRecovery(ctx, alice.AccountID, &proto.CompleteRecoveryRequest{}); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("member completed a recovery: %v", err)
		}
		_, err := alice.Client.CreateRecoveryLink(ctx, alice.AccountID)
		if apiError, ok := err.(*client.APIError); !ok || apiError.Status != http.StatusBadRequest || !strings.Contains(apiError.Message, "another admin") {
			t.Errorf("admin recovered themselves: %v", err)
		}
		if _, err := alice.CompleteRecovery(ctx, alice.AccountID); !errors.Is(err, session.ErrRecoverSelf) {
			t.Errorf("admin completed their own recovery: %v", err)
		}
		if _, err := alice.Client.CreateRecoveryLink(ctx, "ZZZZZZ"); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("recovery of an unknown account: %v", err)
		}
	})

	newPassword := []byte("bob new password")
	recoveryLink := requireValue[*proto.RecoveryLinkResponse](t)(alice.Client.CreateRecoveryLink(ctx, bob.AccountID))
	if recoveryLink.URL != "https://hisar.test/recover/"+recoveryLink.Token {
		t.Errorf("recovery link: %s", recoveryLink.URL)
	}
	if _, err := session.StartRecovery(ctx, client.Create(baseURL), recoveryLink.Token, "alice@example.com", newPassword, testDevice); !client.IsStatus(err, http.StatusForbidden) {
		t.Errorf("recovery link used with another email: %v", err)
	}
	newSecretKey, err := session.StartRecovery(ctx, client.Create(baseURL), recoveryLink.Token, "Bob@Example.com", newPassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if newSecretKey.AccountID != bob.AccountID || newSecretKey.Encode() == bobSecretKey.Encode() {
		t.Errorf("new Secret Key %s for account %s", newSecretKey.Encode(), bob.AccountID)
	}
	_, err = session.StartRecovery(ctx, client.Create(baseURL), recoveryLink.Token, "bob@example.com", newPassword, testDevice)
	if apiError, ok := err.(*client.APIError); !ok || apiError.Status != http.StatusForbidden || apiError.Message != "invalid recovery link" {
		t.Errorf("recovery link reused: %v", err)
	}

	t.Run("old credentials work until completion", func(t *testing.T) {
		if _, err := bob.Client.ListAllVault(ctx); err != nil {
			t.Errorf("bob's session stopped before completion: %v", err)
		}
		requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "bob@example.com", bobPassword, bobSecretKey, testDevice))
		if _, err := session.LogIn(ctx, client.Create(baseURL), "bob@example.com", newPassword, newSecretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
			t.Errorf("new credentials before completion: %v", err)
		}
	})

	pending := findPendingRecovery(t, alice, bob.AccountID)
	if pending == nil || pending.Email != "bob@example.com" || pending.Name != "Bob" || pending.PublicKeys.Verify(bob.AccountID) != nil || pending.SubmittedAt.IsZero() {
		t.Fatalf("pending recovery: %+v", pending)
	}
	allRecoveryWrappedKeyByVaultID := map[string][]byte{}
	for _, recoveryVault := range pending.AllVault {
		allRecoveryWrappedKeyByVaultID[recoveryVault.VaultID] = recoveryVault.WrappedKey
	}
	if len(pending.AllVault) != 3 || allRecoveryWrappedKeyByVaultID[family.ID] == nil || allRecoveryWrappedKeyByVaultID[bobPersonal.ID] == nil || allRecoveryWrappedKeyByVaultID[bobSecret.ID] != nil {
		t.Errorf("pending vaults: %+v", pending.AllVault)
	}

	allUnrecoverableVaultID, err := alice.CompleteRecovery(ctx, bob.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(allUnrecoverableVaultID, []string{bobSecret.ID}) {
		t.Errorf("unrecoverable vaults: %v, want the opted-out vault %s", allUnrecoverableVaultID, bobSecret.ID)
	}

	if _, err := bob.Client.ListAllVault(ctx); !client.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("old session after completion: %v", err)
	}
	if _, err := session.LogIn(ctx, client.Create(baseURL), "bob@example.com", bobPassword, bobSecretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("old credentials after completion: %v", err)
	}
	recoveredBob := requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "bob@example.com", newPassword, newSecretKey, testDevice))
	if password := readItemPassword(t, recoveredBob, "Family", "Netflix"); password != "family-pass" {
		t.Errorf("shared item after recovery: %q", password)
	}
	if password := readItemPassword(t, recoveredBob, "Personal", "Diary"); password != "diary-pass" {
		t.Errorf("personal item after recovery: %q", password)
	}
	if _, err := recoveredBob.FindVault(ctx, "Secret"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("opted-out vault after recovery: %v", err)
	}
	if allDevice := requireValue[[]proto.Device](t)(recoveredBob.Client.ListAllDevice(ctx)); len(allDevice) != 1 {
		t.Errorf("devices after recovery: %d", len(allDevice))
	}
	if findPendingRecovery(t, alice, bob.AccountID) != nil {
		t.Error("pending recovery left after completion")
	}

	t.Run("recovery still works after a key rotation", func(t *testing.T) {
		recoveredBobMember := requireValue[*proto.Member](t)(alice.GetVerifiedMember(ctx, "bob@example.com"))
		if !bytes.Equal(recoveredBobMember.PublicKeys.KEM, recoveredBob.Keyset.GetAllPublicKey(bob.AccountID).KEM) {
			t.Fatal("members list does not show bob's new key")
		}
		requireValue[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Bank"}, Details: item.Details{Password: "bank-pass"}}))
		oldVaultKey := append([]byte(nil), family.Key...)
		if err := alice.RotateVaultKey(ctx, family, []session.AccessGrant{{AccountID: alice.AccountID, Role: proto.VaultOwner}, {AccountID: bob.AccountID, Role: proto.VaultEditor}}); err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(oldVaultKey, family.Key) || !family.Recoverable {
			t.Fatalf("rotation: key changed %v, recoverable %v", !bytes.Equal(oldVaultKey, family.Key), family.Recoverable)
		}
		staleRequest := &proto.RotateVaultRequest{EncName: []byte{1}, AllAccess: []proto.RotatedAccess{{AccountID: alice.AccountID, Role: proto.VaultOwner, WrappedKey: []byte{1}}}}
		for _, protoItem := range requireValue[*proto.ChangesResponse](t)(alice.Client.ListAllChange(ctx, family.ID, 0, 0)).Items {
			staleRequest.AllItem = append(staleRequest.AllItem, proto.RotatedItem{ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}})
		}
		if err := alice.Client.RotateVault(ctx, family.ID, staleRequest); !client.IsStatus(err, http.StatusConflict) {
			t.Errorf("rotation dropping the recovery wrapping: %v", err)
		}

		thirdPassword := []byte("bob third password")
		secondLink := requireValue[*proto.RecoveryLinkResponse](t)(alice.Client.CreateRecoveryLink(ctx, bob.AccountID))
		thirdSecretKey := requireValue[crypt.SecretKey](t)(session.StartRecovery(ctx, client.Create(baseURL), secondLink.Token, "bob@example.com", thirdPassword, testDevice))
		if allUnrecoverable := requireValue[[]string](t)(alice.CompleteRecovery(ctx, bob.AccountID)); len(allUnrecoverable) != 0 {
			t.Errorf("unrecoverable after rotation: %v", allUnrecoverable)
		}
		secondRecoveredBob := requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "bob@example.com", thirdPassword, thirdSecretKey, testDevice))
		if password := readItemPassword(t, secondRecoveredBob, "Family", "Bank"); password != "bank-pass" {
			t.Errorf("item written before the rotation, after second recovery: %q", password)
		}
		if password := readItemPassword(t, secondRecoveredBob, "Personal", "Diary"); password != "diary-pass" {
			t.Errorf("personal item after second recovery: %q", password)
		}
	})

	t.Run("expired link", func(t *testing.T) {
		expiredLink := requireValue[*proto.RecoveryLinkResponse](t)(alice.Client.CreateRecoveryLink(ctx, bob.AccountID))
		if _, err := connection.Exec(ctx, `UPDATE recovery_tokens SET expires_at = now() - interval '1 minute'`); err != nil {
			t.Fatal(err)
		}
		_, err := session.StartRecovery(ctx, client.Create(baseURL), expiredLink.Token, "bob@example.com", newPassword, testDevice)
		if apiError, ok := err.(*client.APIError); !ok || apiError.Status != http.StatusForbidden || apiError.Message != "invalid recovery link" {
			t.Errorf("expired recovery link: %v", err)
		}
		finishRequest := &proto.RecoveryFinishRequest{Token: expiredLink.Token, Email: "bob@example.com"}
		if err := client.Create(baseURL).FinishRecovery(ctx, finishRequest); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("finish with expired link: %v", err)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		defer func(limiter *ipLimiter) { server.limiter = limiter }(server.limiter)
		server.limiter = createIPLimiter(rate.Every(time.Hour), 1)
		request := &proto.RecoveryStartRequest{Token: "x", Email: "bob@example.com"}
		client.Create(baseURL).StartRecovery(ctx, request)
		if _, err := client.Create(baseURL).StartRecovery(ctx, request); !client.IsStatus(err, http.StatusTooManyRequests) {
			t.Errorf("recovery start not rate limited: %v", err)
		}
		if err := client.Create(baseURL).FinishRecovery(ctx, &proto.RecoveryFinishRequest{}); !client.IsStatus(err, http.StatusTooManyRequests) {
			t.Errorf("recovery finish not rate limited: %v", err)
		}
	})

	t.Run("recovery key signed by the wrong key is rejected", func(t *testing.T) {
		attackerKeyset, _ := crypt.GenerateKeyset()
		substituteKeyset, _ := crypt.GenerateKeyset()
		substitute := attackerKeyset.SignRecoveryKeyset(substituteKeyset)
		if _, err := connection.Exec(ctx, `UPDATE recovery_keyset SET kem_pub = $1, kem_sig = $2`, substitute.KEM, substitute.KEMSig); err != nil {
			t.Fatal(err)
		}
		if _, err := alice.GetRecoveryPublicKey(ctx); !errors.Is(err, session.ErrBadRecoveryKey) {
			t.Errorf("recovery key signed by another key: %v", err)
		}
		if _, err := connection.Exec(ctx, `UPDATE recovery_keyset SET sign_pub = $1`, substitute.Sign); err != nil {
			t.Fatal(err)
		}
		if _, err := alice.GetRecoveryPublicKey(ctx); !errors.Is(err, session.ErrBadRecoveryKey) {
			t.Errorf("recovery key with the signer replaced: %v", err)
		}
		if _, err := alice.CreateVault(ctx, "Wrapped to attacker", proto.VaultShared); !errors.Is(err, session.ErrBadRecoveryKey) {
			t.Errorf("vault created with a substituted recovery key: %v", err)
		}
		if _, err := alice.EnableRecoveryForAllVault(ctx); !errors.Is(err, session.ErrBadRecoveryKey) {
			t.Errorf("vaults wrapped to a substituted recovery key: %v", err)
		}
	})

	requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "alice@example.com", alicePassword, aliceSecretKey, testDevice))
}
