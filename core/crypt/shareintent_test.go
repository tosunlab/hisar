package crypt

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestShareIntentVectorAndEveryField(t *testing.T) {
	keyHash := make([]byte, 32)
	for i := range keyHash {
		keyHash[i] = byte(i)
	}
	intent := ShareIntent{Version: 1, Origin: "https://hisar.test", ID: "01890000-0000-7000-8000-000000000001", VaultID: "01890000-0000-7000-8000-000000000002", InvitedBy: "owner", Email: "recipient@example.com", Role: "reader", KeyGeneration: 3, AccessRevision: 7, IssuedAt: 1700000000, ExpiresAt: 1700003600, RecipientID: "recipient", RecipientKeyHash: keyHash}
	if got := hex.EncodeToString(HashShareIntent(intent)); got != "d89b52b9e5d35d5eff11eb62feeedaf12d31628c9bb0b4cc050ba5e95ec00e2a" {
		t.Fatal(got)
	}
	private := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	keyset := &Keyset{signingPrivateKey: private}
	signature := keyset.SignShareIntent(intent)
	if err := VerifyShareIntent(private.Public().(ed25519.PublicKey), intent, signature); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ShareIntent){
		"version": func(i *ShareIntent) { i.Version++ }, "origin": func(i *ShareIntent) { i.Origin = "https://other.test" },
		"operation": func(i *ShareIntent) { i.ID += "x" }, "vault": func(i *ShareIntent) { i.VaultID += "x" },
		"inviter": func(i *ShareIntent) { i.InvitedBy += "x" }, "email": func(i *ShareIntent) { i.Email += "x" }, "role": func(i *ShareIntent) { i.Role = "owner" },
		"generation": func(i *ShareIntent) { i.KeyGeneration++ }, "revision": func(i *ShareIntent) { i.AccessRevision++ },
		"issued": func(i *ShareIntent) { i.IssuedAt++ }, "expiry": func(i *ShareIntent) { i.ExpiresAt++ },
		"recipient": func(i *ShareIntent) { i.RecipientID += "x" }, "recipientKeys": func(i *ShareIntent) { i.RecipientKeyHash = []byte("other") },
		"parent": func(i *ShareIntent) { i.ParentHash = []byte("other") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			altered := intent
			mutate(&altered)
			if VerifyShareIntent(private.Public().(ed25519.PublicKey), altered, signature) == nil {
				t.Fatal("changed intent verified")
			}
		})
	}
}
