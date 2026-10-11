// Package hisarkit is the iOS app's backend, compiled with gomobile into HisarKit.xcframework.
// gomobile passes only simple types, so lists and items cross the boundary as JSON strings.
package hisarkit

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/desktop/browser"
	"github.com/tryhisar/hisar/desktop/vault"
)

// SecretKeyStore keeps the Secret Key in the iOS Keychain, shared with the AutoFill extension.
type SecretKeyStore interface {
	LoadSecretKey(accountID string) (string, error)
	StoreSecretKey(accountID, secretKey string) error
	DeleteSecretKey(accountID string) error
}

// Biometric checks Face ID or Touch ID and keeps the biometric unlock key in a Keychain item only biometrics open.
type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
	StoreKey(accountID string, key []byte) error
	LoadKey(accountID string) ([]byte, error)
	DeleteKey(accountID string) error
}

// Clipboard writes the general pasteboard; GetText returns "" when it holds no text.
type Clipboard interface {
	SetText(text string) bool
	GetText() string
}

// EventListener hears about locks the app did not ask for, e.g. auto-lock, about phone approvals
// (eventName is approval-requested, approval-answered or approval-resolved) and about syncs of changes made elsewhere.
type EventListener interface {
	OnLocked()
	OnApprovalEvent(eventName, requestID string)
	OnRemoteChange()
}

type Hisar struct {
	service   *vault.Service
	biometric Biometric
}

// New opens the state and offline cache in stateDirectory (the app group container, so the AutoFill extension shares it).
func New(stateDirectory, deviceName string, secretKeyStore SecretKeyStore, biometric Biometric, clipboard Clipboard, listener EventListener) (*Hisar, error) {
	if secretKeyStore == nil {
		return nil, errors.New("hisarkit: a SecretKeyStore is required")
	}
	config := vault.Config{
		StatePath:                     filepath.Join(stateDirectory, "hisar.json"),
		CachePath:                     filepath.Join(stateDirectory, "hisar-cache.db"),
		DeviceName:                    deviceName,
		Platform:                      "ios/mobile",
		SecretKeyStore:                secretKeyStoreAdapter{store: secretKeyStore},
		IsBiometricAllowedAfterLaunch: true,
		AutoLockAfter:                 5 * time.Minute,
	}
	if biometric != nil {
		config.Biometric = biometric
	}
	if clipboard != nil {
		config.Clipboard = clipboardAdapter{clipboard: clipboard}
	}
	if listener != nil {
		config.LockNotifier = lockNotifierAdapter{listener: listener}
		config.OnApprovalEvent = listener.OnApprovalEvent
		config.OnRemoteChange = listener.OnRemoteChange
	}
	service, err := vault.CreateService(config)
	if err != nil {
		return nil, err
	}
	return &Hisar{service: service, biometric: biometric}, nil
}

type secretKeyStoreAdapter struct {
	store SecretKeyStore
}

func (adapter secretKeyStoreAdapter) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	text, err := adapter.store.LoadSecretKey(accountID)
	if err != nil {
		return crypt.SecretKey{}, err
	}
	return crypt.ParseSecretKey(text)
}

func (adapter secretKeyStoreAdapter) StoreSecretKey(secretKey crypt.SecretKey) error {
	return adapter.store.StoreSecretKey(secretKey.AccountID, secretKey.Encode())
}

func (adapter secretKeyStoreAdapter) DeleteSecretKey(accountID string) error {
	return adapter.store.DeleteSecretKey(accountID)
}

type clipboardAdapter struct {
	clipboard Clipboard
}

func (adapter clipboardAdapter) SetText(text string) bool { return adapter.clipboard.SetText(text) }

func (adapter clipboardAdapter) GetText() (string, bool) {
	text := adapter.clipboard.GetText()
	return text, text != ""
}

type lockNotifierAdapter struct {
	listener EventListener
}

func (adapter lockNotifierAdapter) NotifyLocked() { adapter.listener.OnLocked() }

func encodeJSON(value any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

// releaseMemory hands Argon2's 64 MiB back to iOS right after a key derivation; the AutoFill extension has a tight memory limit.
func releaseMemory() {
	debug.FreeOSMemory()
}

// Status returns vault.Status as JSON.
func (hisar *Hisar) Status() string {
	encoded, _ := encodeJSON(hisar.service.GetStatus(), nil)
	return encoded
}

func (hisar *Hisar) SignIn(server, email, secretKey, password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.SignIn(server, email, secretKey, password))
}

// SignUp returns {"status": …, "secretKey": "H1-…"} as JSON.
func (hisar *Hisar) SignUp(inviteLink, email, name, password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.SignUp(inviteLink, email, name, password))
}

func (hisar *Hisar) Unlock(password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.Unlock(password))
}

func (hisar *Hisar) UnlockWithBiometric() (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.UnlockWithBiometric())
}

func (hisar *Hisar) EnableBiometricUnlock(password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.EnableBiometricUnlock(password))
}

func (hisar *Hisar) EnableNewAccountBiometricUnlock() (string, error) {
	return encodeJSON(hisar.service.EnableNewAccountBiometricUnlock())
}

func (hisar *Hisar) ForgetNewAccountSecretKey() {
	hisar.service.ForgetNewAccountSecretKey()
}

func (hisar *Hisar) DisableBiometricUnlock() (string, error) {
	return encodeJSON(hisar.service.DisableBiometricUnlock())
}

func (hisar *Hisar) Lock() string {
	encoded, _ := encodeJSON(hisar.service.Lock(), nil)
	return encoded
}

func (hisar *Hisar) SignOut() (string, error) {
	return encodeJSON(hisar.service.SignOut())
}

// ReportActivity postpones auto-lock; the app calls it on user interaction.
func (hisar *Hisar) ReportActivity() {
	hisar.service.ReportActivity()
}

// ListAllVault returns []vault.VaultSummary as JSON; it also syncs every vault.
func (hisar *Hisar) ListAllVault() (string, error) {
	return encodeJSON(hisar.service.ListAllVault())
}

// CreateVault creates a shared vault owned by the current user and returns its vault.VaultSummary as JSON.
func (hisar *Hisar) CreateVault(name string) (string, error) {
	return encodeJSON(hisar.service.CreateVault(name))
}

// ListAllPerson returns the user and the people they share a vault with ([]vault.Person) as JSON; isCurrentUser marks the user.
func (hisar *Hisar) ListAllPerson() (string, error) {
	return encodeJSON(hisar.service.ListAllPerson())
}

// InvitePerson asks the server to email a sign-up invite.
func (hisar *Hisar) InvitePerson(email string) error {
	return hisar.service.InvitePerson(email)
}

// ListAllShareInvitation returns vault.ShareInvitations as JSON, after completing shares that people accepted.
func (hisar *Hisar) ListAllShareInvitation() (string, error) {
	return encodeJSON(hisar.service.ListAllShareInvitation())
}

// ListAllShareAwaitingUnlock returns []vault.ShareAwaitingUnlock as JSON for the lock screen: shares people accepted that this
// app finishes once unlocked. It is empty while unlocked, offline or signed out.
func (hisar *Hisar) ListAllShareAwaitingUnlock() (string, error) {
	return encodeJSON(hisar.service.ListAllShareAwaitingUnlock())
}

func (hisar *Hisar) AcceptShareInvitation(invitationID string) error {
	return hisar.service.AcceptShareInvitation(invitationID)
}

func (hisar *Hisar) DeclineShareInvitation(invitationID string) error {
	return hisar.service.DeclineShareInvitation(invitationID)
}

func (hisar *Hisar) CancelShareInvitation(invitationID string) error {
	return hisar.service.CancelShareInvitation(invitationID)
}

// ListAllVaultMember returns who has access to a shared vault ([]vault.VaultMember) as JSON, owners first.
func (hisar *Hisar) ListAllVaultMember(vaultID string) (string, error) {
	return encodeJSON(hisar.service.ListAllVaultMember(vaultID))
}

// ShareVault emails a share invitation for a shared vault the user owns; role is "owner", "editor" or "reader".
func (hisar *Hisar) ShareVault(vaultID, email, role string) error {
	return hisar.service.ShareVault(vaultID, email, role)
}

func (hisar *Hisar) ChangeVaultMemberRole(vaultID, accountID, role string) error {
	return hisar.service.ChangeVaultMemberRole(vaultID, accountID, role)
}

// RemoveVaultMember removes a member and re-encrypts the vault with a new key for the remaining members.
func (hisar *Hisar) RemoveVaultMember(vaultID, accountID string) error {
	return hisar.service.RemoveVaultMember(vaultID, accountID)
}

// ListAllItem returns []vault.ItemDetail without passwords, notes, fields and one-time password secrets (hasOneTimePassword says whether there is one), as JSON.
func (hisar *Hisar) ListAllItem(vaultID string) (string, error) {
	return encodeJSON(hisar.service.ListAllItem(vaultID))
}

// GetItem returns one vault.ItemDetail with its secrets, as JSON.
func (hisar *Hisar) GetItem(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.GetItem(vaultID, itemID))
}

// SaveItem takes and returns a vault.ItemDetail as JSON; an empty id creates the item.
func (hisar *Hisar) SaveItem(vaultID, itemJSON string) (string, error) {
	var detail vault.ItemDetail
	if err := json.Unmarshal([]byte(itemJSON), &detail); err != nil {
		return "", errors.New("hisarkit: the item is not valid JSON")
	}
	return encodeJSON(hisar.service.SaveItem(vaultID, detail))
}

func (hisar *Hisar) DeleteItem(vaultID, itemID string) error {
	return hisar.service.DeleteItem(vaultID, itemID)
}

// GetOneTimeCode returns vault.OneTimeCode ({"code", "expiresAt", "period"}) as JSON; the app asks again once it expires.
func (hisar *Hisar) GetOneTimeCode(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.GetOneTimeCode(vaultID, itemID))
}

// CopyField copies a field to the pasteboard and clears it after 90 seconds if it still holds that value.
func (hisar *Hisar) CopyField(vaultID, itemID, fieldName string) error {
	return hisar.service.CopyField(vaultID, itemID, fieldName)
}

func (hisar *Hisar) GeneratePassword(length int, isDigitIncluded, isSymbolIncluded bool) (string, error) {
	return hisar.service.GeneratePassword(vault.PasswordConfig{Length: length, Digits: isDigitIncluded, Symbols: isSymbolIncluded})
}

// ListAllLoginForService returns the logins matching an AutoFill service identifier (a URL or a bare domain), as JSON.
func (hisar *Hisar) ListAllLoginForService(serviceIdentifier string) (string, error) {
	return encodeJSON(vault.ListAllLoginForURL(hisar.service, convertServiceIdentifierToURL(serviceIdentifier)))
}

// GetLoginForService returns {"username", "password", "hasOneTimePassword"} as JSON after checking the login matches the service again.
func (hisar *Hisar) GetLoginForService(vaultID, itemID, serviceIdentifier string) (string, error) {
	return encodeJSON(vault.GetLoginForURL(hisar.service, vaultID, itemID, convertServiceIdentifierToURL(serviceIdentifier)))
}

// GetOneTimeCodeForService returns vault.OneTimeCode as JSON after checking the login matches the service, as GetLoginForService does.
func (hisar *Hisar) GetOneTimeCodeForService(vaultID, itemID, serviceIdentifier string) (string, error) {
	return encodeJSON(vault.GetOneTimeCodeForURL(hisar.service, vaultID, itemID, convertServiceIdentifierToURL(serviceIdentifier)))
}

func convertServiceIdentifierToURL(serviceIdentifier string) string {
	serviceIdentifier = strings.TrimSpace(serviceIdentifier)
	if strings.Contains(serviceIdentifier, "://") {
		return serviceIdentifier
	}
	return "https://" + serviceIdentifier
}

// ListAllPasskey returns every passkey in a login ([]vault.StoredPasskey, binary values base64url) as JSON, for the credential identity store.
func (hisar *Hisar) ListAllPasskey() (string, error) {
	return encodePasskeyJSON(vault.ListAllPasskey(hisar.service))
}

// ListAllPasskeyForRelyingParty takes the allowed credential IDs as a JSON array of base64url strings; an empty array allows all.
func (hisar *Hisar) ListAllPasskeyForRelyingParty(relyingPartyID, allowedCredentialIDsJSON string) (string, error) {
	allAllowedCredentialID, err := decodeStringArray(allowedCredentialIDsJSON)
	if err != nil {
		return "", err
	}
	return encodePasskeyJSON(vault.ListAllPasskeyForRelyingParty(hisar.service, relyingPartyID, allAllowedCredentialID))
}

// PreparePasskeyRegistration returns {"isExcluded", "logins"} as JSON: the writable logins matching the relying party.
func (hisar *Hisar) PreparePasskeyRegistration(relyingPartyID, excludedCredentialIDsJSON string) (string, error) {
	allExcludedCredentialID, err := decodeStringArray(excludedCredentialIDsJSON)
	if err != nil {
		return "", err
	}
	return encodePasskeyJSON(vault.PreparePasskeyRegistration(hisar.service, relyingPartyID, allExcludedCredentialID))
}

// CreatePasskey takes a vault.PasskeyRegistration and returns a vault.PasskeyRegistrationResult, as JSON; the private key stays in Go.
func (hisar *Hisar) CreatePasskey(registrationJSON string) (string, error) {
	var registration vault.PasskeyRegistration
	if err := json.Unmarshal([]byte(registrationJSON), &registration); err != nil {
		return "", errors.New("hisarkit: the passkey registration is not valid JSON")
	}
	return encodePasskeyJSON(vault.CreatePasskeyForRelyingParty(hisar.service, registration))
}

// AssertPasskey signs authenticatorData || clientDataHash and returns a vault.PasskeyAssertion as JSON; IDs and the hash are base64url.
func (hisar *Hisar) AssertPasskey(relyingPartyID, credentialID, clientDataHash string) (string, error) {
	return encodePasskeyJSON(vault.SignPasskeyForRelyingParty(hisar.service, relyingPartyID, credentialID, clientDataHash))
}

// DeletePasskey returns the item without the passkey as vault.ItemDetail JSON.
func (hisar *Hisar) DeletePasskey(vaultID, itemID, credentialID string) (string, error) {
	return encodeJSON(hisar.service.DeletePasskey(vaultID, itemID, credentialID))
}

func decodeStringArray(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var allValue []string
	if err := json.Unmarshal([]byte(text), &allValue); err != nil {
		return nil, errors.New("hisarkit: the credential IDs are not a JSON array of strings")
	}
	return allValue, nil
}

// encodePasskeyJSON drops the browser protocol's error code, so the app shows only the message.
func encodePasskeyJSON(value any, err error) (string, error) {
	var responseError *browser.ResponseError
	if errors.As(err, &responseError) {
		err = errors.New(strings.TrimPrefix(responseError.Message, "passkey: "))
	}
	return encodeJSON(value, err)
}
