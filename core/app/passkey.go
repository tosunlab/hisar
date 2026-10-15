package app

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

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/passkey"
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

func convertToItemPasskeys(passkeys []item.Passkey) []ItemPasskey {
	itemPasskeys := make([]ItemPasskey, 0, len(passkeys))
	for _, storedPasskey := range passkeys {
		itemPasskeys = append(itemPasskeys, ItemPasskey{
			CredentialID:    encodeBase64URL(storedPasskey.CredentialID),
			RelyingPartyID:  storedPasskey.RelyingPartyID,
			UserName:        storedPasskey.UserName,
			UserDisplayName: storedPasskey.UserDisplayName,
			CreatedAt:       storedPasskey.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return itemPasskeys
}

func encodeBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeBase64URL(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
}

func decodeCredentialIDs(encodedValues []string) ([][]byte, error) {
	if len(encodedValues) > maxPasskeyCredentialCount {
		return nil, passkey.ErrInvalidRequest
	}
	credentialIDs := make([][]byte, 0, len(encodedValues))
	for _, encoded := range encodedValues {
		credentialID, err := decodeBase64URL(encoded)
		if err != nil || len(credentialID) == 0 || len(credentialID) > 1023 {
			return nil, passkey.ErrInvalidRequest
		}
		credentialIDs = append(credentialIDs, credentialID)
	}
	return credentialIDs, nil
}

func containsCredentialID(credentialIDs [][]byte, credentialID []byte) bool {
	return slices.ContainsFunc(credentialIDs, func(candidate []byte) bool { return bytes.Equal(candidate, credentialID) })
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
	return convertToItemDetail(&vaultItem, s.config.Now()), nil
}

func (bridge *BrowserBridge) handlePasskeyRequest(ctx context.Context, request *browserprotocol.Request) (any, error) {
	var passkeyRequest browserprotocol.PasskeyRequest
	if len(request.Passkey) == 0 || json.Unmarshal(request.Passkey, &passkeyRequest) != nil {
		return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "the passkey request is not valid")
	}
	var result any
	var err error
	switch request.Type {
	case "passkeys.list":
		result, err = bridge.service.listPasskeysForRequest(ctx, &passkeyRequest)
	case "passkeys.prepare":
		result, err = bridge.service.preparePasskeyCreation(ctx, &passkeyRequest)
	case "passkeys.create":
		result, err = bridge.service.createPasskey(ctx, &passkeyRequest)
	case "passkeys.get":
		result, err = bridge.service.signPasskeyAssertion(ctx, &passkeyRequest)
	case PasskeySignalUnknownCredential, PasskeySignalAcceptedCredentials, PasskeySignalCurrentUserDetails:
		result, err = bridge.service.applyPasskeySignal(ctx, request.Type, &passkeyRequest)
	default:
		return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
	}
	return result, convertPasskeyError(err)
}

func convertPasskeyError(err error) error {
	switch {
	case errors.Is(err, passkey.ErrInvalidOrigin), errors.Is(err, passkey.ErrInvalidRelyingPartyID), errors.Is(err, passkey.ErrInvalidRequest), errors.Is(err, errPasskeyExcluded):
		return browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, err.Error())
	}
	return convertBrowserError(err)
}

type passkeyMatch struct {
	vaultID string
	itemID  string
	title   string
	passkey item.Passkey
}

// collectPasskeys runs under the service mutex with every vault's items loaded; an empty relyingPartyID collects every passkey.
func (s *Service) collectPasskeys(relyingPartyID string) []passkeyMatch {
	var matches []passkeyMatch
	for vaultID, items := range s.itemsByVaultID {
		if s.vaultsByID[vaultID] == nil {
			continue
		}
		for _, vaultItem := range items {
			if !vaultItem.IsCategory(item.CategoryLogin) {
				continue
			}
			for _, storedPasskey := range vaultItem.Details.Passkeys {
				if relyingPartyID == "" || storedPasskey.RelyingPartyID == relyingPartyID {
					matches = append(matches, passkeyMatch{vaultID: vaultID, itemID: vaultItem.ID, title: vaultItem.Overview.Title, passkey: storedPasskey})
				}
			}
		}
	}
	slices.SortFunc(matches, func(first, second passkeyMatch) int {
		if comparison := strings.Compare(strings.ToLower(first.title), strings.ToLower(second.title)); comparison != 0 {
			return comparison
		}
		return strings.Compare(strings.ToLower(first.passkey.UserName), strings.ToLower(second.passkey.UserName))
	})
	return matches
}

// loadPasskeys runs under the service mutex.
func (s *Service) loadPasskeys(ctx context.Context, relyingPartyID string) ([]passkeyMatch, error) {
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, err
	}
	return s.collectPasskeys(relyingPartyID), nil
}

func filterAllowedPasskeys(matches []passkeyMatch, allowedIDs [][]byte) []passkeyMatch {
	if len(allowedIDs) == 0 {
		return matches
	}
	return slices.DeleteFunc(slices.Clone(matches), func(match passkeyMatch) bool { return !containsCredentialID(allowedIDs, match.passkey.CredentialID) })
}

func isAnyPasskeyExcluded(matches []passkeyMatch, excludedIDs [][]byte) bool {
	return slices.ContainsFunc(matches, func(match passkeyMatch) bool { return containsCredentialID(excludedIDs, match.passkey.CredentialID) })
}

func (s *Service) listWritableLogins(ctx context.Context, origin string) ([]browserprotocol.Login, error) {
	logins, _, err := s.listLoginsForURL(ctx, origin)
	if err != nil {
		return nil, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return slices.DeleteFunc(logins, func(login browserprotocol.Login) bool {
		vault := s.vaultsByID[login.VaultID]
		return vault == nil || vault.Role == apitypes.VaultReader
	}), nil
}

type passkeyTarget struct {
	relyingPartyID string
	origin         string
}

func resolveBrowserPasskeyTarget(request *browserprotocol.PasskeyRequest) (passkeyTarget, error) {
	relyingPartyID, err := passkey.ResolveRelyingPartyID(request.Origin, request.RelyingPartyID)
	return passkeyTarget{relyingPartyID: relyingPartyID, origin: request.Origin}, err
}

func resolvePlatformPasskeyTarget(requestedRelyingPartyID string) (passkeyTarget, error) {
	relyingPartyID, err := passkey.ValidateRelyingPartyID(requestedRelyingPartyID)
	return passkeyTarget{relyingPartyID: relyingPartyID, origin: "https://" + relyingPartyID}, err
}

func (s *Service) preparePasskeySave(ctx context.Context, target passkeyTarget, excludedIDs [][]byte) (browserprotocol.PasskeyPrepareResult, error) {
	s.mutex.Lock()
	matches, err := s.loadPasskeys(ctx, target.relyingPartyID)
	s.mutex.Unlock()
	if err != nil {
		return browserprotocol.PasskeyPrepareResult{}, err
	}
	logins, err := s.listWritableLogins(ctx, target.origin)
	if err != nil {
		return browserprotocol.PasskeyPrepareResult{}, err
	}
	return browserprotocol.PasskeyPrepareResult{IsExcluded: isAnyPasskeyExcluded(matches, excludedIDs), Logins: logins}, nil
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
	target      passkeyTarget
	user        passkeyUser
	excludedIDs [][]byte
	vaultID     string
	itemID      string
}

type savedPasskey struct {
	creation passkey.Creation
	vaultID  string
	itemID   string
}

func (s *Service) savePasskey(ctx context.Context, save passkeySave, createCredential func() (passkey.Creation, error)) (savedPasskey, error) {
	isExistingItem := save.vaultID != "" || save.itemID != ""
	if isExistingItem {
		logins, err := s.listWritableLogins(ctx, save.target.origin)
		if err != nil {
			return savedPasskey{}, err
		}
		if !slices.ContainsFunc(logins, func(login browserprotocol.Login) bool {
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
	matches, err := s.loadPasskeys(ctx, relyingPartyID)
	if err != nil {
		return savedPasskey{}, err
	}
	if isAnyPasskeyExcluded(matches, save.excludedIDs) {
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

func (s *Service) signWithPasskey(ctx context.Context, relyingPartyID string, credentialID []byte, allowedIDs [][]byte, sign func(item.Passkey) (passkey.Assertion, error)) (item.Passkey, passkey.Assertion, error) {
	if len(credentialID) == 0 {
		return item.Passkey{}, passkey.Assertion{}, passkey.ErrInvalidRequest
	}
	if len(allowedIDs) > 0 && !containsCredentialID(allowedIDs, credentialID) {
		return item.Passkey{}, passkey.Assertion{}, fmt.Errorf("passkey %w", ErrNotFound)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	matches, err := s.loadPasskeys(ctx, relyingPartyID)
	if err != nil {
		return item.Passkey{}, passkey.Assertion{}, err
	}
	index := slices.IndexFunc(matches, func(match passkeyMatch) bool { return bytes.Equal(match.passkey.CredentialID, credentialID) })
	if index < 0 {
		return item.Passkey{}, passkey.Assertion{}, fmt.Errorf("passkey %w", ErrNotFound)
	}
	storedPasskey := matches[index].passkey
	assertion, err := sign(storedPasskey)
	return storedPasskey, assertion, err
}

func (s *Service) listPasskeysForRequest(ctx context.Context, request *browserprotocol.PasskeyRequest) (browserprotocol.PasskeyListResult, error) {
	allowedIDs, err := decodeCredentialIDs(request.AllowedCredentialIDs)
	if err != nil {
		return browserprotocol.PasskeyListResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browserprotocol.PasskeyListResult{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	matches, err := s.loadPasskeys(ctx, target.relyingPartyID)
	if err != nil {
		return browserprotocol.PasskeyListResult{}, err
	}
	result := browserprotocol.PasskeyListResult{Passkeys: []browserprotocol.PasskeySummary{}}
	for _, match := range filterAllowedPasskeys(matches, allowedIDs) {
		result.Passkeys = append(result.Passkeys, browserprotocol.PasskeySummary{
			CredentialID: encodeBase64URL(match.passkey.CredentialID), VaultID: match.vaultID, ItemID: match.itemID,
			Title: match.title, UserName: match.passkey.UserName, UserDisplayName: match.passkey.UserDisplayName,
		})
	}
	return result, nil
}

func (s *Service) preparePasskeyCreation(ctx context.Context, request *browserprotocol.PasskeyRequest) (browserprotocol.PasskeyPrepareResult, error) {
	excludedIDs, err := decodeCredentialIDs(request.ExcludedCredentialIDs)
	if err != nil {
		return browserprotocol.PasskeyPrepareResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browserprotocol.PasskeyPrepareResult{}, err
	}
	return s.preparePasskeySave(ctx, target, excludedIDs)
}

func (s *Service) createPasskey(ctx context.Context, request *browserprotocol.PasskeyRequest) (browserprotocol.PasskeyCreationResult, error) {
	user, err := decodePasskeyUser(request.UserHandle, request.UserName, request.UserDisplayName)
	if err != nil {
		return browserprotocol.PasskeyCreationResult{}, err
	}
	challenge, err := decodeBase64URL(request.Challenge)
	if err != nil {
		return browserprotocol.PasskeyCreationResult{}, passkey.ErrInvalidRequest
	}
	excludedIDs, err := decodeCredentialIDs(request.ExcludedCredentialIDs)
	if err != nil {
		return browserprotocol.PasskeyCreationResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browserprotocol.PasskeyCreationResult{}, err
	}
	var prfResult *browserprotocol.PasskeyPRFResult
	saved, err := s.savePasskey(ctx, passkeySave{target: target, user: user, excludedIDs: excludedIDs, vaultID: request.VaultID, itemID: request.ItemID}, func() (passkey.Creation, error) {
		creation, err := passkey.CreateCredential(passkey.CreationRequest{
			Origin: request.Origin, TopOrigin: request.TopOrigin, IsCrossOrigin: request.IsCrossOrigin, RelyingPartyID: target.relyingPartyID, Challenge: challenge,
		})
		if err != nil {
			return passkey.Creation{}, err
		}
		// Before the save, so an invalid PRF input saves nothing.
		prfResult, err = evaluatePasskeyCreationPRF(request.PRF, creation.PrivateKey)
		return creation, err
	})
	if err != nil {
		return browserprotocol.PasskeyCreationResult{}, err
	}
	return browserprotocol.PasskeyCreationResult{
		CredentialID:       encodeBase64URL(saved.creation.CredentialID),
		ClientDataJSON:     encodeBase64URL(saved.creation.ClientDataJSON),
		AttestationObject:  encodeBase64URL(saved.creation.AttestationObject),
		AuthenticatorData:  encodeBase64URL(saved.creation.AuthenticatorData),
		PublicKey:          encodeBase64URL(saved.creation.PublicKey),
		PublicKeyAlgorithm: passkey.AlgorithmES256,
		VaultID:            saved.vaultID,
		ItemID:             saved.itemID,
		PRF:                prfResult,
	}, nil
}

func (s *Service) signPasskeyAssertion(ctx context.Context, request *browserprotocol.PasskeyRequest) (browserprotocol.PasskeyAssertionResult, error) {
	credentialID, err := decodeBase64URL(request.CredentialID)
	if err != nil {
		return browserprotocol.PasskeyAssertionResult{}, passkey.ErrInvalidRequest
	}
	challenge, err := decodeBase64URL(request.Challenge)
	if err != nil {
		return browserprotocol.PasskeyAssertionResult{}, passkey.ErrInvalidRequest
	}
	allowedIDs, err := decodeCredentialIDs(request.AllowedCredentialIDs)
	if err != nil {
		return browserprotocol.PasskeyAssertionResult{}, err
	}
	target, err := resolveBrowserPasskeyTarget(request)
	if err != nil {
		return browserprotocol.PasskeyAssertionResult{}, err
	}
	var prfResult *browserprotocol.PasskeyPRFResult
	storedPasskey, assertion, err := s.signWithPasskey(ctx, target.relyingPartyID, credentialID, allowedIDs, func(storedPasskey item.Passkey) (passkey.Assertion, error) {
		var err error
		if prfResult, err = evaluatePasskeyAssertionPRF(request.PRF, allowedIDs, storedPasskey); err != nil {
			return passkey.Assertion{}, err
		}
		return passkey.SignAssertion(passkey.AssertionRequest{
			Origin: request.Origin, TopOrigin: request.TopOrigin, IsCrossOrigin: request.IsCrossOrigin, RelyingPartyID: target.relyingPartyID, Challenge: challenge,
		}, storedPasskey.RelyingPartyID, storedPasskey.PrivateKey)
	})
	if err != nil {
		return browserprotocol.PasskeyAssertionResult{}, err
	}
	return browserprotocol.PasskeyAssertionResult{
		CredentialID:      encodeBase64URL(storedPasskey.CredentialID),
		ClientDataJSON:    encodeBase64URL(assertion.ClientDataJSON),
		AuthenticatorData: encodeBase64URL(assertion.AuthenticatorData),
		Signature:         encodeBase64URL(assertion.Signature),
		UserHandle:        encodeBase64URL(storedPasskey.UserHandle),
		PRF:               prfResult,
	}, nil
}
