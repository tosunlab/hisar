package vault

import (
	"context"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/passkey"
)

// Platform passkey functions serve the iOS credential provider, which passes an RP ID and a clientDataHash; Wails does not bind functions.

type StoredPasskey struct {
	CredentialID    string `json:"credentialId"`
	RelyingPartyID  string `json:"relyingPartyId"`
	UserName        string `json:"userName"`
	UserDisplayName string `json:"userDisplayName"`
	UserHandle      string `json:"userHandle"`
	VaultID         string `json:"vaultID"`
	ItemID          string `json:"itemID"`
	Title           string `json:"title"`
}

type PasskeyRegistration struct {
	RelyingPartyID          string   `json:"relyingPartyId"`
	UserName                string   `json:"userName"`
	UserDisplayName         string   `json:"userDisplayName"`
	UserHandle              string   `json:"userHandle"`
	ClientDataHash          string   `json:"clientDataHash"`
	AllExcludedCredentialID []string `json:"excludeCredentials"`
	VaultID                 string   `json:"vaultID"`
	ItemID                  string   `json:"itemID"`
}

type PasskeyRegistrationResult struct {
	CredentialID      string `json:"credentialId"`
	AttestationObject string `json:"attestationObject"`
	AuthenticatorData string `json:"authenticatorData"`
	VaultID           string `json:"vaultID"`
	ItemID            string `json:"itemID"`
}

type PasskeyAssertion struct {
	CredentialID      string `json:"credentialId"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle"`
}

func convertToAllStoredPasskey(allMatch []passkeyMatch) []StoredPasskey {
	allStoredPasskey := make([]StoredPasskey, 0, len(allMatch))
	for _, match := range allMatch {
		allStoredPasskey = append(allStoredPasskey, StoredPasskey{
			CredentialID: encodeBase64URL(match.passkey.CredentialID), RelyingPartyID: match.passkey.RelyingPartyID,
			UserName: match.passkey.UserName, UserDisplayName: match.passkey.UserDisplayName, UserHandle: encodeBase64URL(match.passkey.UserHandle),
			VaultID: match.vaultID, ItemID: match.itemID, Title: match.title,
		})
	}
	return allStoredPasskey
}

func ListAllPasskey(s *Service) ([]StoredPasskey, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allMatch, err := s.loadAllPasskey(context.Background(), "")
	if err != nil {
		return nil, convertPasskeyError(err)
	}
	return convertToAllStoredPasskey(allMatch), nil
}

func ListAllPasskeyForRelyingParty(s *Service, relyingPartyID string, allAllowedCredentialID []string) ([]StoredPasskey, error) {
	allAllowedID, err := decodeAllCredentialID(allAllowedCredentialID)
	if err != nil {
		return nil, convertPasskeyError(err)
	}
	target, err := resolvePlatformPasskeyTarget(relyingPartyID)
	if err != nil {
		return nil, convertPasskeyError(err)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allMatch, err := s.loadAllPasskey(context.Background(), target.relyingPartyID)
	if err != nil {
		return nil, convertPasskeyError(err)
	}
	return convertToAllStoredPasskey(filterAllAllowedPasskey(allMatch, allAllowedID)), nil
}

func PreparePasskeyRegistration(s *Service, relyingPartyID string, allExcludedCredentialID []string) (browserprotocol.PasskeyPrepareResult, error) {
	allExcludedID, err := decodeAllCredentialID(allExcludedCredentialID)
	if err != nil {
		return browserprotocol.PasskeyPrepareResult{}, convertPasskeyError(err)
	}
	target, err := resolvePlatformPasskeyTarget(relyingPartyID)
	if err != nil {
		return browserprotocol.PasskeyPrepareResult{}, convertPasskeyError(err)
	}
	result, err := s.preparePasskeySave(context.Background(), target, allExcludedID)
	return result, convertPasskeyError(err)
}

func CreatePasskeyForRelyingParty(s *Service, registration PasskeyRegistration) (PasskeyRegistrationResult, error) {
	result, err := s.createPlatformPasskey(context.Background(), registration)
	return result, convertPasskeyError(err)
}

func (s *Service) createPlatformPasskey(ctx context.Context, registration PasskeyRegistration) (PasskeyRegistrationResult, error) {
	user, err := decodePasskeyUser(registration.UserHandle, registration.UserName, registration.UserDisplayName)
	if err != nil {
		return PasskeyRegistrationResult{}, err
	}
	clientDataHash, err := decodeBase64URL(registration.ClientDataHash)
	if err != nil || len(clientDataHash) != passkey.ClientDataHashLength {
		return PasskeyRegistrationResult{}, passkey.ErrInvalidRequest
	}
	allExcludedID, err := decodeAllCredentialID(registration.AllExcludedCredentialID)
	if err != nil {
		return PasskeyRegistrationResult{}, err
	}
	target, err := resolvePlatformPasskeyTarget(registration.RelyingPartyID)
	if err != nil {
		return PasskeyRegistrationResult{}, err
	}
	saved, err := s.savePasskey(ctx, passkeySave{target: target, user: user, allExcludedID: allExcludedID, vaultID: registration.VaultID, itemID: registration.ItemID}, func() (passkey.Creation, error) {
		return passkey.CreateCredentialWithClientDataHash(target.relyingPartyID, clientDataHash)
	})
	if err != nil {
		return PasskeyRegistrationResult{}, err
	}
	return PasskeyRegistrationResult{
		CredentialID:      encodeBase64URL(saved.creation.CredentialID),
		AttestationObject: encodeBase64URL(saved.creation.AttestationObject),
		AuthenticatorData: encodeBase64URL(saved.creation.AuthenticatorData),
		VaultID:           saved.vaultID,
		ItemID:            saved.itemID,
	}, nil
}

func SignPasskeyForRelyingParty(s *Service, relyingPartyID, credentialID, clientDataHash string) (PasskeyAssertion, error) {
	result, err := s.signPlatformPasskey(context.Background(), relyingPartyID, credentialID, clientDataHash)
	return result, convertPasskeyError(err)
}

func (s *Service) signPlatformPasskey(ctx context.Context, relyingPartyID, encodedCredentialID, encodedClientDataHash string) (PasskeyAssertion, error) {
	credentialID, err := decodeBase64URL(encodedCredentialID)
	if err != nil {
		return PasskeyAssertion{}, passkey.ErrInvalidRequest
	}
	clientDataHash, err := decodeBase64URL(encodedClientDataHash)
	if err != nil || len(clientDataHash) != passkey.ClientDataHashLength {
		return PasskeyAssertion{}, passkey.ErrInvalidRequest
	}
	target, err := resolvePlatformPasskeyTarget(relyingPartyID)
	if err != nil {
		return PasskeyAssertion{}, err
	}
	storedPasskey, assertion, err := s.signWithPasskey(ctx, target.relyingPartyID, credentialID, nil, func(storedPasskey item.Passkey) (passkey.Assertion, error) {
		return passkey.SignAssertionWithClientDataHash(target.relyingPartyID, storedPasskey.RelyingPartyID, clientDataHash, storedPasskey.PrivateKey)
	})
	if err != nil {
		return PasskeyAssertion{}, err
	}
	return PasskeyAssertion{
		CredentialID:      encodeBase64URL(storedPasskey.CredentialID),
		AuthenticatorData: encodeBase64URL(assertion.AuthenticatorData),
		Signature:         encodeBase64URL(assertion.Signature),
		UserHandle:        encodeBase64URL(storedPasskey.UserHandle),
	}, nil
}
