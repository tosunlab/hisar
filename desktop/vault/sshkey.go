package vault

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/core/sshkey"
)

const (
	SSHKeyTypeEd25519 = "ed25519"
	SSHKeyTypeRSA     = "rsa"
)

// GenerateSSHKey creates an ssh-key item with a new Ed25519 (default) or RSA 4096 key.
func (s *Service) GenerateSSHKey(vaultID, title, keyType string) (ItemDetail, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return ItemDetail{}, errors.New("enter a title")
	}
	if err := s.checkUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	var material *sshkey.KeyMaterial
	var err error
	switch strings.ToLower(strings.TrimSpace(keyType)) {
	case "", SSHKeyTypeEd25519:
		material, err = sshkey.GenerateEd25519Key(title)
	case SSHKeyTypeRSA:
		material, err = sshkey.GenerateRSAKey(sshkey.DefaultRSABits, title)
	default:
		return ItemDetail{}, errors.New("choose an Ed25519 or RSA key")
	}
	if err != nil {
		return ItemDetail{}, err
	}
	return s.saveSSHKey(vaultID, title, material)
}

// ImportSSHKey stores an existing OpenSSH or PEM private key, decrypted with the passphrase if it has one.
func (s *Service) ImportSSHKey(vaultID, title, privateKeyText, passphrase string) (ItemDetail, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return ItemDetail{}, errors.New("enter a title")
	}
	if strings.TrimSpace(privateKeyText) == "" {
		return ItemDetail{}, errors.New("paste the private key")
	}
	if err := s.checkUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	privateKeyBytes := []byte(strings.TrimSpace(privateKeyText) + "\n")
	defer clear(privateKeyBytes)
	passphraseBytes := []byte(passphrase)
	defer clear(passphraseBytes)
	material, err := sshkey.ImportPrivateKey(privateKeyBytes, passphraseBytes, title)
	if err != nil {
		return ItemDetail{}, convertImportError(err)
	}
	return s.saveSSHKey(vaultID, title, material)
}

func convertImportError(err error) error {
	switch {
	case errors.Is(err, sshkey.ErrPassphraseRequired):
		return errors.New("this key is protected by a passphrase; enter it to import the key")
	case errors.Is(err, sshkey.ErrWrongPassphrase):
		return errors.New("the passphrase is not correct")
	case errors.Is(err, sshkey.ErrUnsupportedKey):
		return errors.New("this key type is not supported; use an Ed25519, RSA or ECDSA key")
	case errors.Is(err, sshkey.ErrInvalidKey):
		return errors.New("this is not a private key in OpenSSH or PEM format")
	}
	return err
}

func (s *Service) checkUnlocked() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.requireUnlocked()
}

func buildSSHKeyItem(title string, material *sshkey.KeyMaterial) item.Item {
	return item.Item{
		Overview: item.Overview{
			Title:       title,
			Category:    item.CategorySSHKey,
			PublicKey:   material.AuthorizedKey,
			Fingerprint: material.Fingerprint,
			KeyType:     material.KeyType,
		},
		Details: item.Details{PrivateKey: string(material.PrivateKeyPEM)},
	}
}

func (s *Service) saveSSHKey(vaultID, title string, material *sshkey.KeyMaterial) (ItemDetail, error) {
	defer clear(material.PrivateKeyPEM)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return ItemDetail{}, err
	}
	vaultItem := buildSSHKeyItem(title, material)
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return convertToItemDetail(&vaultItem), nil
}

// SSHKeySource offers the ssh-key items of every vault to the SSH agent. It is a separate type so the
// UI bindings of Service never expose private keys.
type SSHKeySource struct {
	service *Service
}

func CreateSSHKeySource(service *Service) *SSHKeySource {
	return &SSHKeySource{service: service}
}

// lockedKeySourceError is ErrLocked for the vault and sshagent.ErrLocked for the agent, which then asks to unlock.
type lockedKeySourceError struct{}

func (lockedKeySourceError) Error() string { return ErrLocked.Error() }

func (lockedKeySourceError) Is(target error) bool {
	return target == ErrLocked || target == sshagent.ErrLocked
}

// ListAllKey returns ErrLocked or ErrSignedOut unless Hisar is unlocked; it does not postpone auto-lock.
func (source *SSHKeySource) ListAllKey(ctx context.Context) ([]sshagent.Key, error) {
	s := source.service
	if err := s.requireCurrentVersion(); err != nil {
		var updateRequired *UpdateRequiredError
		if errors.As(err, &updateRequired) && s.config.OnUpdateRequired != nil {
			go s.config.OnUpdateRequired(updateRequired.Requirement)
		}
		return nil, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil {
		if s.state.IsLoggedIn() {
			return nil, lockedKeySourceError{}
		}
		return nil, ErrSignedOut
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.allVaultByID == nil {
		if err := s.loadAllVault(ctx); err != nil {
			return nil, err
		}
	}
	allVault := make([]*session.Vault, 0, len(s.allVaultByID))
	for _, vault := range s.allVaultByID {
		allVault = append(allVault, vault)
	}
	// Servers stop after about 6 attempts, so keys from the personal vault come first.
	slices.SortFunc(allVault, func(first, second *session.Vault) int {
		if (first.Type == apitypes.VaultPersonal) != (second.Type == apitypes.VaultPersonal) {
			if first.Type == apitypes.VaultPersonal {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	allKey := []sshagent.Key{}
	for _, vault := range allVault {
		allItem, isCached := s.allItemByVaultID[vault.ID]
		if !isCached {
			loadedItem, err := s.unlockedSession.ListAllItem(ctx, vault)
			if err != nil {
				return nil, convertError(err)
			}
			s.allItemByVaultID[vault.ID] = loadedItem
			allItem = loadedItem
		}
		for _, vaultItem := range allItem {
			if vaultItem.Overview.Category != item.CategorySSHKey || vaultItem.Details.PrivateKey == "" {
				continue
			}
			allKey = append(allKey, sshagent.Key{
				ID:            vaultItem.ID,
				VaultID:       vault.ID,
				Title:         vaultItem.Overview.Title,
				PrivateKeyPEM: []byte(vaultItem.Details.PrivateKey),
			})
		}
	}
	return allKey, nil
}
