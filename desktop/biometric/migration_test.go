package biometric

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
)

type migrationFixture struct {
	legacy, protected                   []byte
	storeError, proofError, deleteError error
	corruptProof                        bool
	stores, reads, deletes              int
}

func (fixture *migrationFixture) hasLegacy() bool { return fixture.legacy != nil }
func (fixture *migrationFixture) loadLegacy() ([]byte, error) {
	return bytes.Clone(fixture.legacy), nil
}
func (fixture *migrationFixture) loadProtected() ([]byte, error) {
	fixture.reads++
	if fixture.protected == nil {
		return nil, ErrKeyNotFound
	}
	if fixture.proofError != nil {
		return nil, fixture.proofError
	}
	key := bytes.Clone(fixture.protected)
	if fixture.corruptProof {
		key[0] ^= 1
	}
	return key, nil
}
func (fixture *migrationFixture) storeProtected(key []byte) error {
	fixture.stores++
	if fixture.storeError != nil {
		return fixture.storeError
	}
	fixture.protected = bytes.Clone(key)
	return nil
}
func (fixture *migrationFixture) deleteLegacy() error {
	fixture.deletes++
	if fixture.deleteError != nil {
		return fixture.deleteError
	}
	clear(fixture.legacy)
	fixture.legacy = nil
	return nil
}

func TestWrapperMigrationPreservesRecoveryUntilProtectedReadProvesTheKey(t *testing.T) {
	for _, failure := range []string{"missing profile", "cancelled proof", "wrong proof", "retirement failed", "none"} {
		t.Run(failure, func(t *testing.T) {
			key := crypt.GenerateKey()
			defer clear(key)
			sealed, err := crypt.Seal(key, []byte("account unlock key"), []byte("hisar/v1/biometric-unlock:ABC123"))
			if err != nil {
				t.Fatal(err)
			}
			originalState := bytes.Clone(sealed)
			fixture := &migrationFixture{legacy: bytes.Clone(key)}
			switch failure {
			case "missing profile":
				fixture.storeError = errors.New("missing entitlement")
			case "cancelled proof":
				fixture.proofError = errors.New("cancelled")
			case "wrong proof":
				fixture.corruptProof = true
			case "retirement failed":
				fixture.deleteError = errors.New("keychain unavailable")
			}
			err = migrateWrappingKey(fixture, "ABC123", sealed)
			if !bytes.Equal(sealed, originalState) {
				t.Fatal("migration changed the sealed recovery state")
			}
			if failure == "none" {
				if err != nil || fixture.legacy != nil || !bytes.Equal(fixture.protected, key) {
					t.Fatalf("migration: %v %#v", err, fixture)
				}
				return
			}
			if err == nil || !bytes.Equal(fixture.legacy, key) {
				t.Fatalf("failure lost the previous wrapper: %v", err)
			}
			if failure != "retirement failed" && fixture.deletes != 0 {
				t.Fatal("unproved wrapper retired")
			}
			fixture.storeError, fixture.proofError, fixture.deleteError, fixture.corruptProof = nil, nil, nil, false
			if err := migrateWrappingKey(fixture, "ABC123", sealed); err != nil {
				t.Fatal(err)
			}
			if fixture.legacy != nil || !bytes.Equal(fixture.protected, key) {
				t.Fatal("retry did not converge")
			}
			stores := fixture.stores
			if err := migrateWrappingKey(fixture, "ABC123", sealed); err != nil || fixture.stores != stores {
				t.Fatal("completed migration was not idempotent")
			}
		})
	}
}

func TestWrapperMigrationRejectsAnotherAccountsSealedState(t *testing.T) {
	key := crypt.GenerateKey()
	defer clear(key)
	sealed, err := crypt.Seal(key, []byte("unlock key"), []byte("hisar/v1/biometric-unlock:OTHER1"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &migrationFixture{legacy: bytes.Clone(key)}
	if err := migrateWrappingKey(fixture, "ABC123", sealed); err == nil {
		t.Fatal("migrated a wrapper for another account")
	}
	if fixture.protected != nil || fixture.deletes != 0 {
		t.Fatal("invalid state changed keychain recovery")
	}
}
