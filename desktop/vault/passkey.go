package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/desktop/browser"
	"github.com/tryhisar/hisar/desktop/passkey"
)

const (
	maxPasskeyNameLength      = 256
	maxPasskeyCredentialCount = 64
)

var errPasskeyExcluded = errors.New("Hisar already has a passkey for this account")

// ItemPasskey describes a stored passkey without its private key.
type ItemPasskey struct {
	CredentialID    string `json:"credentialId"`
	RelyingPartyID  string `json:"relyingPartyId"`
	UserName        string `json:"userName"`
	UserDisplayName string `json:"userDisplayName"`
	CreatedAt       string `json:"createdAt"`
}

func convertToAllItemPasskey(allPasskey []item.Passkey) []ItemPasskey {
	allItemPasskey := make([]ItemPasskey, 0, len(allPasskey))
	for _, storedPasskey := range allPasskey {
		allItemPasskey = append(allItemPasskey, ItemPasskey{
			CredentialID:    encodeBase64URL(storedPasskey.CredentialID),
			RelyingPartyID:  storedPasskey.RelyingPartyID,
			UserName:        storedPasskey.UserName,
			UserDisplayName: storedPasskey.UserDisplayName,
			CreatedAt:       storedPasskey.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return allItemPasskey
}

func encodeBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeBase64URL(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
}

func decodeAllCredentialID(allEncoded []string) ([][]byte, error) {
	if len(allEncoded) > maxPasskeyCredentialCount {
		return nil, passkey.ErrInvalidRequest
	}
	allCredentialID := make([][]byte, 0, len(allEncoded))
	for _, encoded := range allEncoded {
		credentialID, err := decodeBase64URL(encoded)
		if err != nil || len(credentialID) == 0 || len(credentialID) > 1023 {
			return nil, passkey.ErrInvalidRequest
		}
		allCredentialID = append(allCredentialID, credentialID)
	}
	return allCredentialID, nil
}

func containsCredentialID(allCredentialID [][]byte, credentialID []byte) bool {
	return slices.ContainsFunc(allCredentialID, func(candidate []byte) bool { return bytes.Equal(candidate, credentialID) })
}

// DeletePasskey removes one passkey from an item; credentialID is base64url.
func (s *Service) DeletePasskey(vaultID, itemID, credentialID string) (ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	decodedID, err := decodeBase64URL(credentialID)
	if err != nil {
		return ItemDetail{}, fmt.Errorf("passkey %w", ErrNotFound)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return ItemDetail{}, err
	}
	cachedItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	vaultItem := *cachedItem
	vaultItem.Details.Passkeys = slices.DeleteFunc(slices.Clone(cachedItem.Details.Passkeys), func(storedPasskey item.Passkey) bool {
		return bytes.Equal(storedPasskey.CredentialID, decodedID)
	})
	if len(vaultItem.Details.Passkeys) == len(cachedItem.Details.Passkeys) {
		return ItemDetail{}, fmt.Errorf("passkey %w", ErrNotFound)
	}
	item.UpdatePasskeyOverview(&vaultItem)
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return convertToItemDetail(&vaultItem), nil
}

func (bridge *BrowserBridge) handlePasskeyRequest(ctx context.Context, request *browser.Request) (any, error) {
	var passkeyRequest browser.PasskeyRequest
	if len(request.Passkey) == 0 || json.Unmarshal(request.Passkey, &passkeyRequest) != nil {
		return nil, browser.CreateError(browser.ErrorInvalidRequest, "the passkey request is not valid")
	}
	var result any
	var err error
	switch request.Type {
	case "passkeys.list":
		result, err = bridge.service.listAllPasskeyForRequest(ctx, &passkeyRequest)
	case "passkeys.prepare":
		result, err = bridge.service.preparePasskeyCreation(ctx, &passkeyRequest)
	case "passkeys.create":
		result, err = bridge.service.createPasskey(ctx, &passkeyRequest)
	case "passkeys.get":
		result, err = bridge.service.signPasskeyAssertion(ctx, &passkeyRequest)
	default:
		return nil, browser.CreateError(browser.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
	}
	return result, convertPasskeyError(err)
}

func convertPasskeyError(err error) error {
	switch {
	case errors.Is(err, passkey.ErrInvalidOrigin), errors.Is(err, passkey.ErrInvalidRelyingPartyID), errors.Is(err, passkey.ErrInvalidRequest), errors.Is(err, errPasskeyExcluded):
		return browser.CreateError(browser.ErrorInvalidRequest, err.Error())
	}
	return convertBrowserError(err)
}

type passkeyMatch struct {
	vaultID string
	itemID  string
	title   string
	passkey item.Passkey
}

// collectAllPasskey runs under the service mutex with every vault's items loaded; an empty relyingPartyID collects every passkey.
func (s *Service) collectAllPasskey(relyingPartyID string) []passkeyMatch {
	var allMatch []passkeyMatch
	for vaultID, allItem := range s.allItemByVaultID {
		if s.allVaultByID[vaultID] == nil {
			continue
		}
		for _, vaultItem := range allItem {
			if !isLoginItem(&vaultItem) {
				continue
			}
			for _, storedPasskey := range vaultItem.Details.Passkeys {
				if relyingPartyID == "" || storedPasskey.RelyingPartyID == relyingPartyID {
					allMatch = append(allMatch, passkeyMatch{vaultID: vaultID, itemID: vaultItem.ID, title: vaultItem.Overview.Title, passkey: storedPasskey})
				}
			}
		}
	}
	slices.SortFunc(allMatch, func(first, second passkeyMatch) int {
		if comparison := strings.Compare(strings.ToLower(first.title), strings.ToLower(second.title)); comparison != 0 {
			return comparison
		}
		return strings.Compare(strings.ToLower(first.passkey.UserName), strings.ToLower(second.passkey.UserName))
	})
	return allMatch
}

// loadAllPasskey runs under the service mutex.
func (s *Service) loadAllPasskey(ctx context.Context, relyingPartyID string) ([]passkeyMatch, error) {
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, err
	}
	if err := s.loadAllItemOfAllVault(ctx); err != nil {
		return nil, err
	}
	return s.collectAllPasskey(relyingPartyID), nil
}

func filterAllAllowedPasskey(allMatch []passkeyMatch, allAllowedID [][]byte) []passkeyMatch {
	if len(allAllowedID) == 0 {
		return allMatch
	}
	return slices.DeleteFunc(slices.Clone(allMatch), func(match passkeyMatch) bool { return !containsCredentialID(allAllowedID, match.passkey.CredentialID) })
}

func isAnyPasskeyExcluded(allMatch []passkeyMatch, allExcludedID [][]byte) bool {
	return slices.ContainsFunc(allMatch, func(match passkeyMatch) bool { return containsCredentialID(allExcludedID, match.passkey.CredentialID) })
}

func (s *Service) listAllWritableLogin(ctx context.Context, origin string) ([]browser.Login, error) {
	allLogin, _, err := s.listAllLoginForURL(ctx, origin)
	if err != nil {
		return nil, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return slices.DeleteFunc(allLogin, func(login browser.Login) bool {
		vault := s.allVaultByID[login.VaultID]
		return vault == nil || vault.Role == proto.VaultReader
	}), nil
}

type passkeyTarget struct {
	relyingPartyID string
	origin         string
}

func resolveBrowserPasskeyTarget(request *browser.PasskeyRequest) (passkeyTarget, error) {
	relyingPartyID, err := passkey.ResolveRelyingPartyID(request.Origin, request.RelyingPartyID)
	return passkeyTarget{relyingPartyID: relyingPartyID, origin: request.Origin}, err
}

func resolvePlatformPasskeyTarget(requestedRelyingPartyID string) (passkeyTarget, error) {
	relyingPartyID, err := passkey.ValidateRelyingPartyID(requestedRelyingPartyID)
	return passkeyTarget{relyingPartyID: relyingPartyID, origin: "https://" + relyingPartyID}, err
}

func (s *Service) preparePasskeySave(ctx context.Context, target passkeyTarget, allExcludedID [][]byte) (browser.PasskeyPrepareResult, error) {
	s.mutex.Lock()
	allMatch, err := s.loadAllPasskey(ctx, target.relyingPartyID)
	s.mutex.Unlock()
	if err != nil {
		return browser.PasskeyPrepareResult{}, err
	}
	allLogin, err := s.listAllWritableLogin(ctx, target.origin)
	if err != nil {
		return browser.PasskeyPrepareResult{}, err
	}
	return browser.PasskeyPrepareResult{IsExcluded: isAnyPasskeyExcluded(allMatch, allExcludedID), AllLogin: allLogin}, nil
}

type passkeyUser struct {
	userHandle      []byte
	userName        string
	userDisplayName string
}

func decodePasskeyUser(encodedUserHandle, userName, userDisplayName string) (passkeyUser, error) {
	userHandle, err := decodeBase64URL(encodedUserHandle)
	if err != nil || len(userHandle) == 0 || len(userHandle) > passkey.MaxUserHandleLength {
		return passkeyUser{}, passkey.ErrInvalidRequest
	}
	userName, userDisplayName = strings.TrimSpace(userName), strings.TrimSpace(userDisplayName)
	if utf8.RuneCountInString(userName) > maxPasskeyNameLength || utf8.RuneCountInString(userDisplayName) > maxPasskeyNameLength {
		return passkeyUser{}, passkey.ErrInvalidRequest
	}
	return passkeyUser{userHandle: userHandle, userName: userName, userDisplayName: userDisplayName}, nil
}

func buildPasskeyLoginURL(origin, relyingPartyID string) string {
	if parsedOrigin, err := url.Parse(origin); err == nil && strings.EqualFold(parsedOrigin.Hostname(), relyingPartyID) {
		return origin
	}
	return "https://" + relyingPartyID
}

type passkeySave struct {
	target        passkeyTarget
	user          passkeyUser
	allExcludedID [][]byte
	vaultID       string
	itemID        string
}

type savedPasskey struct {
	creation passkey.Creation
	vaultID  string
	itemID   string
}

func (s *Service) savePasskey(ctx context.Context, save passkeySave, createCredential func() (passkey.Creation, error)) (savedPasskey, error) {
	isExistingItem := save.vaultID != "" || save.itemID != ""
	if isExistingItem {
		allLogin, err := s.listAllWritableLogin(ctx, save.target.origin)
		if err != nil {
			return savedPasskey{}, err
		}
		if !slices.ContainsFunc(allLogin, func(login browser.Login) bool {
			return login.VaultID == save.vaultID && login.ItemID == save.itemID
		}) {
			return savedPasskey{}, errNoURLMatch
		}
	}
	vaultID := save.vaultID
	if !isExistingItem {
		personalVaultID, err := s.findPersonalVaultID(ctx)
		if err != nil {
			return savedPasskey{}, err
		}
		vaultID = personalVaultID
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	relyingPartyID := save.target.relyingPartyID
	allMatch, err := s.loadAllPasskey(ctx, relyingPartyID)
	if err != nil {
		return savedPasskey{}, err
	}
	if isAnyPasskeyExcluded(allMatch, save.allExcludedID) {
		return savedPasskey{}, errPasskeyExcluded
	}
	creation, err := createCredential()
	if err != nil {
		return savedPasskey{}, err
	}
	newPasskey := item.Passkey{
		CredentialID: creation.CredentialID, RelyingPartyID: relyingPartyID, UserHandle: save.user.userHandle,
		UserName: save.user.userName, UserDisplayName: save.user.userDisplayName, PrivateKey: creation.PrivateKey, CreatedAt: s.config.Now().UTC().Truncate(time.Second),
	}

	var vaultItem item.Item
	if isExistingItem {
		cachedItem, err := s.findItem(vaultID, save.itemID)
		if err != nil {
			return savedPasskey{}, err
		}
		vaultItem = *cachedItem
		vaultItem.Details.Passkeys = slices.DeleteFunc(slices.Clone(cachedItem.Details.Passkeys), func(storedPasskey item.Passkey) bool {
			return storedPasskey.RelyingPartyID == relyingPartyID && bytes.Equal(storedPasskey.UserHandle, save.user.userHandle)
		})
		vaultItem.Details.Passkeys = append(vaultItem.Details.Passkeys, newPasskey)
	} else {
		vaultItem = item.Item{
			Overview: item.Overview{Title: strings.TrimPrefix(relyingPartyID, "www."), Category: item.CategoryLogin, Username: save.user.userName, URLs: []string{buildPasskeyLoginURL(save.target.origin, relyingPartyID)}},
			Details:  item.Details{Passkeys: []item.Passkey{newPasskey}},
		}
	}
	item.UpdatePasskeyOverview(&vaultItem)
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return savedPasskey{}, err
	}
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return savedPasskey{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return savedPasskey{creation: creation, vaultID: vaultID, itemID: vaultItem.ID}, nil
}

func (s *Service) signWithPasskey(ctx context.Context, relyingPartyID string, credentialID []byte, allAllowedID [][]byte, sign func(item.Passkey) (passkey.Assertion, error)) (item.Passkey, passkey.Assertion, error) {
	if len(credentialID) == 0 {
		return item.Passkey{}, passkey.Assertion{}, passkey.ErrInvalidRequest
	}
	if len(allAllowedID) > 0 && !containsCredentialID(allAllowedID, credentialID) {
		return item.Passkey{}, passkey.Assertion{}, fmt.Errorf("passkey %w", ErrNotFound)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allMatch, err := s.loadAllPasskey(ctx, relyingPartyID)
	if err != nil {
		return item.Passkey{}, passkey.Assertion{}, err
	}
	index := slices.IndexFunc(allMatch, func(match passkeyMatch) bool { return bytes.Equal(match.passkey.CredentialID, credentialID) })
	if index < 0 {
		return item.Passkey{}, passkey.Assertion{}, fmt.Errorf("passkey %w", ErrNotFound)
	}
	storedPasskey := allMatch[index].passkey
	assertion, err := sign(storedPasskey)
	return storedPasskey, assertion, err
}

func (s *Service) listAllPasskeyForRequest(ctx context.Context, request *browser.PasskeyRequest) (browser.PasskeyListResult, error) {
	allAllowedID, err := decodeAllCredentialID(request.AllAllowedCredentialID)
	if err != nil {
		return browser.PasskeyListResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browser.PasskeyListResult{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allMatch, err := s.loadAllPasskey(ctx, target.relyingPartyID)
	if err != nil {
		return browser.PasskeyListResult{}, err
	}
	result := browser.PasskeyListResult{AllPasskey: []browser.PasskeySummary{}}
	for _, match := range filterAllAllowedPasskey(allMatch, allAllowedID) {
		result.AllPasskey = append(result.AllPasskey, browser.PasskeySummary{
			CredentialID: encodeBase64URL(match.passkey.CredentialID), VaultID: match.vaultID, ItemID: match.itemID,
			Title: match.title, UserName: match.passkey.UserName, UserDisplayName: match.passkey.UserDisplayName,
		})
	}
	return result, nil
}

func (s *Service) preparePasskeyCreation(ctx context.Context, request *browser.PasskeyRequest) (browser.PasskeyPrepareResult, error) {
	allExcludedID, err := decodeAllCredentialID(request.AllExcludedCredentialID)
	if err != nil {
		return browser.PasskeyPrepareResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browser.PasskeyPrepareResult{}, err
	}
	return s.preparePasskeySave(ctx, target, allExcludedID)
}

func (s *Service) createPasskey(ctx context.Context, request *browser.PasskeyRequest) (browser.PasskeyCreationResult, error) {
	user, err := decodePasskeyUser(request.UserHandle, request.UserName, request.UserDisplayName)
	if err != nil {
		return browser.PasskeyCreationResult{}, err
	}
	challenge, err := decodeBase64URL(request.Challenge)
	if err != nil {
		return browser.PasskeyCreationResult{}, passkey.ErrInvalidRequest
	}
	allExcludedID, err := decodeAllCredentialID(request.AllExcludedCredentialID)
	if err != nil {
		return browser.PasskeyCreationResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browser.PasskeyCreationResult{}, err
	}
	saved, err := s.savePasskey(ctx, passkeySave{target: target, user: user, allExcludedID: allExcludedID, vaultID: request.VaultID, itemID: request.ItemID}, func() (passkey.Creation, error) {
		return passkey.CreateCredential(passkey.CreationRequest{
			Origin: request.Origin, TopOrigin: request.TopOrigin, IsCrossOrigin: request.IsCrossOrigin, RelyingPartyID: target.relyingPartyID, Challenge: challenge,
		})
	})
	if err != nil {
		return browser.PasskeyCreationResult{}, err
	}
	return browser.PasskeyCreationResult{
		CredentialID:       encodeBase64URL(saved.creation.CredentialID),
		ClientDataJSON:     encodeBase64URL(saved.creation.ClientDataJSON),
		AttestationObject:  encodeBase64URL(saved.creation.AttestationObject),
		AuthenticatorData:  encodeBase64URL(saved.creation.AuthenticatorData),
		PublicKey:          encodeBase64URL(saved.creation.PublicKey),
		PublicKeyAlgorithm: passkey.AlgorithmES256,
		VaultID:            saved.vaultID,
		ItemID:             saved.itemID,
	}, nil
}

func (s *Service) signPasskeyAssertion(ctx context.Context, request *browser.PasskeyRequest) (browser.PasskeyAssertionResult, error) {
	credentialID, err := decodeBase64URL(request.CredentialID)
	if err != nil {
		return browser.PasskeyAssertionResult{}, passkey.ErrInvalidRequest
	}
	challenge, err := decodeBase64URL(request.Challenge)
	if err != nil {
		return browser.PasskeyAssertionResult{}, passkey.ErrInvalidRequest
	}
	allAllowedID, err := decodeAllCredentialID(request.AllAllowedCredentialID)
	if err != nil {
		return browser.PasskeyAssertionResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browser.PasskeyAssertionResult{}, err
	}
	storedPasskey, assertion, err := s.signWithPasskey(ctx, target.relyingPartyID, credentialID, allAllowedID, func(storedPasskey item.Passkey) (passkey.Assertion, error) {
		return passkey.SignAssertion(passkey.AssertionRequest{
			Origin: request.Origin, TopOrigin: request.TopOrigin, IsCrossOrigin: request.IsCrossOrigin, RelyingPartyID: target.relyingPartyID, Challenge: challenge,
		}, storedPasskey.RelyingPartyID, storedPasskey.PrivateKey)
	})
	if err != nil {
		return browser.PasskeyAssertionResult{}, err
	}
	return browser.PasskeyAssertionResult{
		CredentialID:      encodeBase64URL(storedPasskey.CredentialID),
		ClientDataJSON:    encodeBase64URL(assertion.ClientDataJSON),
		AuthenticatorData: encodeBase64URL(assertion.AuthenticatorData),
		Signature:         encodeBase64URL(assertion.Signature),
		UserHandle:        encodeBase64URL(storedPasskey.UserHandle),
	}, nil
}
