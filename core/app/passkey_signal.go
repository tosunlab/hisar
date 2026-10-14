package app

import (
	"bytes"
	"context"
	"strings"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/passkey"
)

const (
	PasskeySignalUnknownCredential      = "passkeys.signalUnknownCredential"
	PasskeySignalAllAcceptedCredentials = "passkeys.signalAllAcceptedCredentials"
	PasskeySignalCurrentUserDetails     = "passkeys.signalCurrentUserDetails"
)

// passkeySignalChange returns the passkey to keep (nil removes it) and whether it changed.
type passkeySignalChange func(storedPasskey item.Passkey) (*item.Passkey, bool)

type pendingPasskeySignalSave struct {
	vaultID     string
	vaultItem   item.Item
	changeCount int
}

func decodeSignalUserHandle(encoded string) ([]byte, error) {
	userHandle, err := decodeBase64URL(encoded)
	if err != nil || len(userHandle) == 0 || len(userHandle) > passkey.MaxUserHandleLength {
		return nil, passkey.ErrInvalidRequest
	}
	return userHandle, nil
}

func buildPasskeySignalChange(signalType string, request *browserprotocol.PasskeyRequest) (passkeySignalChange, error) {
	switch signalType {
	case PasskeySignalUnknownCredential:
		allCredentialID, err := decodeAllCredentialID([]string{request.CredentialID})
		if err != nil {
			return nil, err
		}
		return func(storedPasskey item.Passkey) (*item.Passkey, bool) {
			if bytes.Equal(storedPasskey.CredentialID, allCredentialID[0]) {
				return nil, true
			}
			return &storedPasskey, false
		}, nil
	case PasskeySignalAllAcceptedCredentials:
		userHandle, err := decodeSignalUserHandle(request.UserHandle)
		if err != nil {
			return nil, err
		}
		allAcceptedID, err := decodeAllCredentialID(request.AllAcceptedCredentialID)
		if err != nil {
			return nil, err
		}
		return func(storedPasskey item.Passkey) (*item.Passkey, bool) {
			if bytes.Equal(storedPasskey.UserHandle, userHandle) && !containsCredentialID(allAcceptedID, storedPasskey.CredentialID) {
				return nil, true
			}
			return &storedPasskey, false
		}, nil
	case PasskeySignalCurrentUserDetails:
		userHandle, err := decodeSignalUserHandle(request.UserHandle)
		if err != nil {
			return nil, err
		}
		userName, userDisplayName := strings.TrimSpace(request.UserName), strings.TrimSpace(request.UserDisplayName)
		if utf8.RuneCountInString(userName) > maxPasskeyNameLength || utf8.RuneCountInString(userDisplayName) > maxPasskeyNameLength {
			return nil, passkey.ErrInvalidRequest
		}
		return func(storedPasskey item.Passkey) (*item.Passkey, bool) {
			if !bytes.Equal(storedPasskey.UserHandle, userHandle) || (storedPasskey.UserName == userName && storedPasskey.UserDisplayName == userDisplayName) {
				return &storedPasskey, false
			}
			storedPasskey.UserName, storedPasskey.UserDisplayName = userName, userDisplayName
			return &storedPasskey, true
		}, nil
	}
	return nil, passkey.ErrInvalidRequest
}

func applyPasskeySignalChange(vaultItem *item.Item, relyingPartyID string, change passkeySignalChange) ([]item.Passkey, int) {
	allPasskey := make([]item.Passkey, 0, len(vaultItem.Details.Passkeys))
	changeCount := 0
	for _, storedPasskey := range vaultItem.Details.Passkeys {
		if storedPasskey.RelyingPartyID != relyingPartyID {
			allPasskey = append(allPasskey, storedPasskey)
			continue
		}
		kept, isChanged := change(storedPasskey)
		if isChanged {
			changeCount++
		}
		if kept != nil {
			allPasskey = append(allPasskey, *kept)
		}
	}
	return allPasskey, changeCount
}

// applyPasskeySignal is best effort: locked or signed out, it changes nothing and reports no error.
func (s *Service) applyPasskeySignal(ctx context.Context, signalType string, request *browserprotocol.PasskeyRequest) (browserprotocol.PasskeySignalResult, error) {
	change, err := buildPasskeySignalChange(signalType, request)
	if err != nil {
		return browserprotocol.PasskeySignalResult{}, err
	}
	if strings.TrimSpace(request.RelyingPartyID) == "" {
		return browserprotocol.PasskeySignalResult{}, passkey.ErrInvalidRelyingPartyID
	}
	relyingPartyID, err := passkey.ResolveRelyingPartyID(request.Origin, request.RelyingPartyID)
	if err != nil {
		return browserprotocol.PasskeySignalResult{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.requireUnlockedWithoutActivity() != nil {
		return browserprotocol.PasskeySignalResult{}, nil
	}
	if err := s.loadAllItemOfAllVault(ctx); err != nil {
		return browserprotocol.PasskeySignalResult{}, err
	}
	var allPendingSave []pendingPasskeySignalSave
	for vaultID, allItem := range s.allItemByVaultID {
		if vault := s.allVaultByID[vaultID]; vault == nil || vault.Role == apitypes.VaultReader {
			continue
		}
		for _, vaultItem := range allItem {
			if !isLoginItem(&vaultItem) {
				continue
			}
			allPasskey, changeCount := applyPasskeySignalChange(&vaultItem, relyingPartyID, change)
			if changeCount == 0 {
				continue
			}
			vaultItem.Details.Passkeys = allPasskey
			item.UpdatePasskeyOverview(&vaultItem)
			allPendingSave = append(allPendingSave, pendingPasskeySignalSave{vaultID: vaultID, vaultItem: vaultItem, changeCount: changeCount})
		}
	}
	result := browserprotocol.PasskeySignalResult{}
	for _, pendingSave := range allPendingSave {
		vault, err := s.findVault(ctx, pendingSave.vaultID)
		if err != nil {
			return result, err
		}
		if _, err := s.unlockedSession.SaveItem(ctx, vault, &pendingSave.vaultItem); err != nil {
			return result, convertError(err)
		}
		s.replaceCachedItem(pendingSave.vaultID, pendingSave.vaultItem)
		result.ChangedPasskeyCount += pendingSave.changeCount
	}
	return result, nil
}
