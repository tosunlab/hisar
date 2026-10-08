package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func findVault(t *testing.T, database *Store, accountID, vaultID string) *Vault {
	t.Helper()
	allVault, err := database.ListAllVault(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	for index := range allVault {
		if allVault[index].ID == vaultID {
			return &allVault[index]
		}
	}
	return nil
}

func TestRecoveryKeyset(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")

	if _, err := database.GetRecoveryKeyset(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("keyset before setup: %v", err)
	}
	recoveryKeyset := &RecoveryKeyset{KEMPub: []byte{1}, SignPub: []byte{2}, KEMSig: []byte{3}, EncKeyset: []byte{4}, CreatedBy: "AAAAAA"}
	if err := database.CreateRecoveryKeyset(ctx, recoveryKeyset); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateRecoveryKeyset(ctx, recoveryKeyset); !errors.Is(err, ErrExists) {
		t.Errorf("second keyset: %v", err)
	}
	stored, err := database.GetRecoveryKeyset(ctx)
	if err != nil || stored.CreatedBy != "AAAAAA" || stored.KEMPub[0] != 1 || stored.EncKeyset[0] != 4 {
		t.Errorf("stored keyset: %+v %v", stored, err)
	}
}

func TestVaultRecovery(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")

	wrappedVaultID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: wrappedVaultID, Type: "personal", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}, []byte{7}); err != nil {
		t.Fatal(err)
	}
	plainVaultID := createTestVault(t, database, "AAAAAA")
	if vault := findVault(t, database, "AAAAAA", wrappedVaultID); !vault.Recoverable || vault.RecoveryOptOut {
		t.Errorf("vault created with a recovery wrapping: %+v", vault)
	}
	if vault := findVault(t, database, "AAAAAA", plainVaultID); vault.Recoverable {
		t.Errorf("vault created without a recovery wrapping: %+v", vault)
	}

	if err := database.OptOutVaultRecovery(ctx, wrappedVaultID); err != nil {
		t.Fatal(err)
	}
	if vault := findVault(t, database, "AAAAAA", wrappedVaultID); vault.Recoverable || !vault.RecoveryOptOut {
		t.Errorf("after opt-out: %+v", vault)
	}
	if err := database.SetVaultRecovery(ctx, wrappedVaultID, []byte{8}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetVaultRecovery(ctx, wrappedVaultID, []byte{9}); err != nil {
		t.Fatal(err)
	}
	if vault := findVault(t, database, "AAAAAA", wrappedVaultID); !vault.Recoverable || vault.RecoveryOptOut {
		t.Errorf("after enabling again: %+v", vault)
	}
	if vaultType, err := database.GetVaultType(ctx, wrappedVaultID); err != nil || vaultType != "personal" {
		t.Errorf("vault type: %q %v", vaultType, err)
	}
}

func TestRecoveryTokens(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	pending := &PendingRecovery{AccountID: "BBBBBB", Argon2Params: []byte(`{}`), Salt: []byte{2}, OpaqueRecord: []byte{2}, EncKeyset: []byte{2}, KEMPub: []byte{2}, SignPub: []byte{2}, KEMSig: []byte{2}}

	if err := database.CreateRecoveryToken(ctx, "BBBBBB", []byte("first"), "AAAAAA", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateRecoveryToken(ctx, "BBBBBB", []byte("second"), "AAAAAA", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetRecoveryTokenAccount(ctx, []byte("first")); !errors.Is(err, ErrNotFound) {
		t.Errorf("replaced token: %v", err)
	}
	if account, err := database.GetRecoveryTokenAccount(ctx, []byte("second")); err != nil || account.ID != "BBBBBB" {
		t.Fatalf("token account: %+v %v", account, err)
	}
	wrongAccount := *pending
	wrongAccount.AccountID = "AAAAAA"
	if err := database.SubmitRecovery(ctx, []byte("second"), &wrongAccount); !errors.Is(err, ErrNotFound) {
		t.Errorf("submit for another account: %v", err)
	}
	if err := database.SubmitRecovery(ctx, []byte("second"), pending); err != nil {
		t.Fatal(err)
	}
	if err := database.SubmitRecovery(ctx, []byte("second"), pending); !errors.Is(err, ErrNotFound) {
		t.Errorf("token reuse: %v", err)
	}
	if _, err := database.GetRecoveryTokenAccount(ctx, []byte("second")); !errors.Is(err, ErrNotFound) {
		t.Errorf("used token: %v", err)
	}
	if allPending, _ := database.ListAllPendingRecovery(ctx); len(allPending) != 1 {
		t.Errorf("pending after submit: %d", len(allPending))
	}

	if err := database.CreateRecoveryToken(ctx, "BBBBBB", []byte("expired"), "AAAAAA", -time.Minute); err != nil {
		t.Fatal(err)
	}
	if allPending, _ := database.ListAllPendingRecovery(ctx); len(allPending) != 0 {
		t.Errorf("a new recovery link kept the earlier submission: %d", len(allPending))
	}
	if _, err := database.GetRecoveryTokenAccount(ctx, []byte("expired")); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired token: %v", err)
	}
	if err := database.SubmitRecovery(ctx, []byte("expired"), pending); !errors.Is(err, ErrNotFound) {
		t.Errorf("submit with expired token: %v", err)
	}
}

func TestCompleteRecovery(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	_, bobSession := createTestAccount(t, database, "BBBBBB", "b@example.com")
	secondDevice := &Device{ID: uuid.NewString(), AccountID: "BBBBBB", Name: "d2", Platform: "p"}
	secondSession := &Session{TokenHash: []byte("second"), AccountID: "BBBBBB", DeviceID: secondDevice.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := database.CreateDeviceSession(ctx, secondDevice, secondSession); err != nil {
		t.Fatal(err)
	}

	sharedVaultID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: sharedVaultID, Type: "shared", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}, []byte{5}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetAccess(ctx, sharedVaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	personalVaultID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: personalVaultID, Type: "personal", EncName: []byte{1}, CreatedBy: "BBBBBB"}, []byte{1}, []byte{6}); err != nil {
		t.Fatal(err)
	}
	optedOutVaultID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: optedOutVaultID, Type: "personal", EncName: []byte{1}, CreatedBy: "BBBBBB"}, []byte{1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.OptOutVaultRecovery(ctx, optedOutVaultID); err != nil {
		t.Fatal(err)
	}

	if _, err := database.CompleteRecovery(ctx, "BBBBBB", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("complete without a pending recovery: %v", err)
	}
	if err := database.CreateRecoveryToken(ctx, "BBBBBB", []byte("token"), "AAAAAA", time.Hour); err != nil {
		t.Fatal(err)
	}
	pending := &PendingRecovery{AccountID: "BBBBBB", Argon2Params: []byte(`{"t":3}`), Salt: []byte{9}, OpaqueRecord: []byte{9}, EncKeyset: []byte{9}, KEMPub: []byte{9}, SignPub: []byte{9}, KEMSig: []byte{9}}
	if err := database.SubmitRecovery(ctx, []byte("token"), pending); err != nil {
		t.Fatal(err)
	}

	allPending, err := database.ListAllPendingRecovery(ctx)
	if err != nil || len(allPending) != 1 {
		t.Fatalf("pending: %+v %v", allPending, err)
	}
	if allPending[0].Email != "b@example.com" || allPending[0].KEMPub[0] != 9 || len(allPending[0].AllVault) != 3 {
		t.Errorf("pending recovery: %+v", allPending[0])
	}
	allRecoveryWrappedKeyByVaultID := map[string][]byte{}
	for _, recoveryVault := range allPending[0].AllVault {
		allRecoveryWrappedKeyByVaultID[recoveryVault.VaultID] = recoveryVault.WrappedKey
	}
	if allRecoveryWrappedKeyByVaultID[sharedVaultID][0] != 5 || allRecoveryWrappedKeyByVaultID[personalVaultID][0] != 6 || allRecoveryWrappedKeyByVaultID[optedOutVaultID] != nil {
		t.Errorf("recovery wrapped keys: %v", allRecoveryWrappedKeyByVaultID)
	}

	foreignVaultID := createTestVault(t, database, "AAAAAA")
	if _, err := database.CompleteRecovery(ctx, "BBBBBB", []VaultKey{{VaultID: foreignVaultID, WrappedKey: []byte{1}}}); !errors.Is(err, ErrConflict) {
		t.Errorf("complete with a vault the account cannot access: %v", err)
	}
	if account, _ := database.GetAccountByID(ctx, "BBBBBB"); account.Salt[0] == 9 {
		t.Fatal("a failed completion changed the account")
	}

	allUnrecoverableVaultID, err := database.CompleteRecovery(ctx, "BBBBBB", []VaultKey{
		{VaultID: sharedVaultID, WrappedKey: []byte{10}},
		{VaultID: personalVaultID, WrappedKey: []byte{11}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(allUnrecoverableVaultID, []string{optedOutVaultID}) {
		t.Errorf("unrecoverable: %v", allUnrecoverableVaultID)
	}
	account, _ := database.GetAccountByID(ctx, "BBBBBB")
	if account.Salt[0] != 9 || account.OpaqueRecord[0] != 9 || account.EncKeyset[0] != 9 || account.KEMPub[0] != 9 || account.SignPub[0] != 9 || account.KEMSig[0] != 9 || string(account.Argon2Params) != `{"t": 3}` {
		t.Errorf("account after completion: %+v", account)
	}
	allVault, _ := database.ListAllVault(ctx, "BBBBBB")
	if len(allVault) != 2 {
		t.Fatalf("vaults after completion: %+v", allVault)
	}
	for _, vault := range allVault {
		if (vault.ID == sharedVaultID && vault.WrappedKey[0] != 10) || (vault.ID == personalVaultID && vault.WrappedKey[0] != 11) {
			t.Errorf("vault access not re-wrapped: %+v", vault)
		}
	}
	if role, _ := database.GetVaultRole(ctx, sharedVaultID, "AAAAAA"); role != "owner" {
		t.Error("completion changed another member's access")
	}
	for _, tokenHash := range [][]byte{bobSession.TokenHash, secondSession.TokenHash} {
		if _, err := database.GetSessionByHash(ctx, tokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
			t.Errorf("session survived completion: %v", err)
		}
	}
	if allDevice, _ := database.ListAllDevice(ctx, "BBBBBB"); len(allDevice) != 0 {
		t.Errorf("devices after completion: %d", len(allDevice))
	}
	if allPending, _ := database.ListAllPendingRecovery(ctx); len(allPending) != 0 {
		t.Errorf("pending after completion: %d", len(allPending))
	}
	var tokenCount int
	database.pool.QueryRow(ctx, `SELECT count(*) FROM recovery_tokens`).Scan(&tokenCount)
	if tokenCount != 0 {
		t.Errorf("recovery tokens after completion: %d", tokenCount)
	}
	if _, err := database.CompleteRecovery(ctx, "BBBBBB", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("second completion: %v", err)
	}
}

func TestCompleteRecoveryOfKeysetCreator(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: vaultID, Type: "shared", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}, []byte{5}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateRecoveryKeyset(ctx, &RecoveryKeyset{KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}, EncKeyset: []byte{1}, CreatedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	database.CreateRecoveryToken(ctx, "AAAAAA", []byte("token"), "AAAAAA", time.Hour)
	pending := &PendingRecovery{AccountID: "AAAAAA", Argon2Params: []byte(`{}`), Salt: []byte{9}, OpaqueRecord: []byte{9}, EncKeyset: []byte{9}, KEMPub: []byte{9}, SignPub: []byte{9}, KEMSig: []byte{9}}
	if err := database.SubmitRecovery(ctx, []byte("token"), pending); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CompleteRecovery(ctx, "AAAAAA", []VaultKey{{VaultID: vaultID, WrappedKey: []byte{2}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetRecoveryKeyset(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("recovery keyset sealed with the old keys survived: %v", err)
	}
	if vault := findVault(t, database, "AAAAAA", vaultID); vault.Recoverable {
		t.Error("vault still wrapped to the deleted recovery keyset")
	}
}

func TestRotateVaultCarriesRecovery(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: vaultID, Type: "shared", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}, []byte{5}); err != nil {
		t.Fatal(err)
	}
	rotation := VaultRotation{VaultID: vaultID, EncName: []byte{2}, AllAccess: []Member{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte{2}}}, RotatedBy: "AAAAAA"}
	if err := database.RotateVault(ctx, rotation); !errors.Is(err, ErrConflict) {
		t.Errorf("rotation dropping the recovery wrapping: %v", err)
	}
	rotation.RecoveryWrappedKey = []byte{6}
	if err := database.RotateVault(ctx, rotation); err != nil {
		t.Fatal(err)
	}
	allPendingVault, err := database.listAllRecoveryVault(ctx, "AAAAAA")
	if err != nil || len(allPendingVault) != 1 || allPendingVault[0].WrappedKey[0] != 6 {
		t.Errorf("recovery wrapping after rotation: %+v %v", allPendingVault, err)
	}

	plainVaultID := createTestVault(t, database, "AAAAAA")
	plainRotation := VaultRotation{VaultID: plainVaultID, EncName: []byte{2}, AllAccess: []Member{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte{2}}}, RotatedBy: "AAAAAA"}
	if err := database.RotateVault(ctx, plainRotation); err != nil {
		t.Errorf("rotation of a vault without recovery: %v", err)
	}
	if vault := findVault(t, database, "AAAAAA", plainVaultID); vault.Recoverable {
		t.Error("rotation without a recovery key made the vault recoverable")
	}
}
