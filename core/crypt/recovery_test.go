package crypt

import (
	"bytes"
	"testing"
)

func TestRecoveryKeyset(t *testing.T) {
	adminKeyset, _ := GenerateKeyset()
	recoveryKeyset, _ := GenerateKeyset()

	publicKeys := adminKeyset.SignRecoveryKeyset(recoveryKeyset)
	if err := publicKeys.Verify(RecoveryRecipientID); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(publicKeys.Sign, adminKeyset.GetAllPublicKey("ADMIN1").Sign) || !bytes.Equal(publicKeys.KEM, recoveryKeyset.GetKEMPublicKey()) {
		t.Error("recovery public keys do not name the admin signer and the recovery KEM key")
	}
	if publicKeys.Verify("ADMIN1") == nil {
		t.Error("recovery signature verified as an account key")
	}
	otherKeyset, _ := GenerateKeyset()
	substituted := otherKeyset.SignRecoveryKeyset(recoveryKeyset)
	substituted.Sign = publicKeys.Sign
	if substituted.Verify(RecoveryRecipientID) == nil {
		t.Error("signature by another key accepted")
	}

	encrypted, err := recoveryKeyset.EncryptRecoveryKeyset(adminKeyset.UserKey, "ADMIN1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptRecoveryKeyset(adminKeyset.UserKey, "OTHER1", encrypted); err == nil {
		t.Error("decrypted with another admin account ID")
	}
	if _, err := DecryptKeyset(adminKeyset.UserKey, "ADMIN1", encrypted); err == nil {
		t.Error("recovery keyset decrypted as an account keyset")
	}
	decrypted, err := DecryptRecoveryKeyset(adminKeyset.UserKey, "ADMIN1", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	vaultKey := GenerateKey()
	wrappedKey, _ := WrapVaultKey(publicKeys.KEM, "vault", RecoveryRecipientID, vaultKey)
	if unwrapped, err := decrypted.UnwrapVaultKey("vault", RecoveryRecipientID, wrappedKey); err != nil || !bytes.Equal(unwrapped, vaultKey) {
		t.Errorf("unwrap with recovery keyset: %v", err)
	}
}
