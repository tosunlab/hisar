package biometric

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tryhisar/hisar/core/crypt"
)

type migrationKeychain interface {
	hasLegacy() bool
	loadLegacy() ([]byte, error)
	loadProtected() ([]byte, error)
	storeProtected([]byte) error
	deleteLegacy() error
	deleteProtected() error
}

// The sealed state stays unchanged, so interruption never leaves it pointing at a different key.
func migrateWrappingKey(keychain migrationKeychain, accountID string, sealedUnlockKey []byte) error {
	if !keychain.hasLegacy() {
		return nil
	}
	key, err := keychain.loadProtected()
	if err != nil {
		if !errors.Is(err, ErrKeyNotFound) {
			return fmt.Errorf("could not access the protected Touch ID wrapper; use a signed Mac app with a data-protection keychain provisioning profile: %w", err)
		}
		key, err = keychain.loadLegacy()
		if err != nil {
			return fmt.Errorf("could not read the previous Touch ID wrapper: %w", err)
		}
		defer clear(key)
		unlockKey, err := crypt.Open(key, sealedUnlockKey, []byte("hisar/v1/biometric-unlock:"+accountID))
		if err != nil {
			return fmt.Errorf("the previous Touch ID wrapper does not match; turn Touch ID off and on with your master password")
		}
		clear(unlockKey)
		if err := keychain.storeProtected(key); err != nil {
			return fmt.Errorf("Touch ID needs a signed Mac app with a data-protection keychain provisioning profile; your master password still unlocks Hisar: %w", err)
		}
	} else {
		defer clear(key)
	}
	provenKey, err := keychain.loadProtected()
	if err != nil {
		return fmt.Errorf("could not verify the new Touch ID protection; the previous wrapper was kept: %w", err)
	}
	defer clear(provenKey)
	if !bytes.Equal(key, provenKey) {
		return fmt.Errorf("the new Touch ID wrapper did not match; the previous wrapper was kept")
	}
	unlockKey, err := crypt.Open(provenKey, sealedUnlockKey, []byte("hisar/v1/biometric-unlock:"+accountID))
	if err != nil {
		return fmt.Errorf("the new Touch ID wrapper does not open this account; the previous wrapper was kept")
	}
	clear(unlockKey)
	if err := keychain.deleteLegacy(); err != nil {
		return fmt.Errorf("Touch ID protection is pending because the obsolete wrapper could not be retired: %w", err)
	}
	return nil
}

// deleteWrappingKey removes the protected wrapper and any login-keychain wrapper a migration has not retired yet, so
// turning Touch ID off or signing out leaves neither behind.
func deleteWrappingKey(keychain migrationKeychain) error {
	return errors.Join(keychain.deleteProtected(), keychain.deleteLegacy())
}
