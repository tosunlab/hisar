// Package hisarkit is the backend of the iPhone and Android apps, compiled with gomobile into HisarKit.xcframework and HisarKit.aar.
// gomobile passes only simple types, so lists and items cross the boundary as JSON strings.
package hisarkit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

// SecretKeyStore keeps the Secret Key in the phone's keychain, shared with the AutoFill extension.
type SecretKeyStore interface {
	LoadSecretKey(accountID string) (string, error)
	StoreSecretKey(accountID, secretKey string) error
	DeleteSecretKey(accountID string) error
}

// Biometric checks the phone's biometrics and keeps the biometric unlock key where only biometrics open it.
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
// (eventName is approval-requested, approval-answered or approval-resolved), about syncs of changes made elsewhere,
// and about the server refusing this version (Status then has updateRequired).
type EventListener interface {
	OnLocked()
	OnApprovalEvent(eventName, requestID string)
	OnRemoteChange()
	OnUpdateRequired()
}

type Hisar struct {
	service   *app.Service
	biometric Biometric
}

// New opens the state and offline cache in stateDirectory (the app group container, so the AutoFill extension shares it).
func New(stateDirectory, deviceName string, secretKeyStore SecretKeyStore, biometric Biometric, clipboard Clipboard, listener EventListener) (*Hisar, error) {
	if secretKeyStore == nil {
		return nil, errors.New("hisarkit: a SecretKeyStore is required")
	}
	config := app.Config{
		StatePath:                     filepath.Join(stateDirectory, "hisar.json"),
		CachePath:                     filepath.Join(stateDirectory, "hisar-cache.db"),
		DeviceName:                    deviceName,
		Platform:                      platform,
		BiometricName:                 biometricName,
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
		config.OnUpdateRequired = func(app.UpdateRequirement) { listener.OnUpdateRequired() }
	}
	service, err := app.NewService(config)
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

// releaseMemory hands Argon2's 64 MiB back to the OS right after a key derivation; the AutoFill extension has a tight memory limit.
func releaseMemory() {
	debug.FreeOSMemory()
}

// Status returns app.Status as JSON.
func (hisar *Hisar) Status() string {
	encoded, _ := encodeJSON(hisar.service.GetStatus(), nil)
	return encoded
}

// SetAppVersion must run right after New, before any request: the server compares it with its minimum for this platform.
func (hisar *Hisar) SetAppVersion(version string) {
	app.SetAppVersion(hisar.service, version)
}

// CheckMinimumVersion asks the server whether it still accepts this version and returns app.Status as JSON.
func (hisar *Hisar) CheckMinimumVersion() string {
	encoded, _ := encodeJSON(hisar.service.CheckMinimumVersion(), nil)
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

// ListAllVault returns []app.VaultSummary as JSON; it also syncs every vault.
func (hisar *Hisar) ListAllVault() (string, error) {
	return encodeJSON(hisar.service.ListVaults())
}

// CreateVault creates a shared vault owned by the current user and returns its app.VaultSummary as JSON.
func (hisar *Hisar) CreateVault(name string) (string, error) {
	return encodeJSON(hisar.service.CreateVault(name))
}

// ListAllPerson returns the user and the people they share a vault with ([]app.Person) as JSON; isCurrentUser marks the user.
func (hisar *Hisar) ListAllPerson() (string, error) {
	return encodeJSON(hisar.service.ListPeople())
}

// InvitePerson asks the server to email a sign-up invite.
func (hisar *Hisar) InvitePerson(email string) error {
	return hisar.service.InvitePerson(email)
}

// ListAllShareInvitation returns app.ShareInvitations as JSON, after completing shares that people accepted.
func (hisar *Hisar) ListAllShareInvitation() (string, error) {
	return encodeJSON(hisar.service.ListShareInvitations())
}

// ListAllShareAwaitingUnlock returns []app.ShareAwaitingUnlock as JSON for the lock screen: shares people accepted that this
// app finishes once unlocked. It is empty while unlocked, offline or signed out.
func (hisar *Hisar) ListAllShareAwaitingUnlock() (string, error) {
	return encodeJSON(hisar.service.ListSharesAwaitingUnlock())
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

// ListAllVaultMember returns who has access to a shared vault ([]app.VaultMember) as JSON, owners first.
func (hisar *Hisar) ListAllVaultMember(vaultID string) (string, error) {
	return encodeJSON(hisar.service.ListVaultMembers(vaultID))
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

// ListAllItem returns []app.ItemDetail without passwords, notes, fields and one-time password secrets (hasOneTimePassword says whether there is one), as JSON.
func (hisar *Hisar) ListAllItem(vaultID string) (string, error) {
	return encodeJSON(hisar.service.ListItems(vaultID))
}

// GetItem returns one app.ItemDetail with its secrets, as JSON.
func (hisar *Hisar) GetItem(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.GetItem(vaultID, itemID))
}

// SaveItem takes and returns a app.ItemDetail as JSON; an empty id creates the item.
func (hisar *Hisar) SaveItem(vaultID, itemJSON string) (string, error) {
	var detail app.ItemDetail
	if err := json.Unmarshal([]byte(itemJSON), &detail); err != nil {
		return "", errors.New("hisarkit: the item is not valid JSON")
	}
	return encodeJSON(hisar.service.SaveItem(vaultID, detail))
}

func (hisar *Hisar) DeleteItem(vaultID, itemID string) error {
	return hisar.service.DeleteItem(vaultID, itemID)
}

// ListAllFavoriteItem returns []app.FavoriteItem ({"vaultId", "vaultName", "vaultType", "item"}, the item without secrets)
// as JSON: the user's starred items in every vault they can open.
func (hisar *Hisar) ListAllFavoriteItem() (string, error) {
	return encodeJSON(hisar.service.ListFavoriteItems())
}

// SetItemFavorite stars or unstars an item for this user on all their devices.
func (hisar *Hisar) SetItemFavorite(vaultID, itemID string, isFavorite bool) error {
	return hisar.service.SetItemFavorite(vaultID, itemID, isFavorite)
}

// DescribeCard returns app.CardDescription ({"brand", "last4", "numberWarning", "expiryWarning"}) as JSON, for the card
// editor to show while the user types.
func (hisar *Hisar) DescribeCard(number, expiry string) (string, error) {
	return encodeJSON(hisar.service.DescribeCard(number, expiry), nil)
}

// ConvertToSecureNote turns a login with nothing but notes and custom fields into a secure note and returns it as app.ItemDetail JSON.
func (hisar *Hisar) ConvertToSecureNote(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.ConvertToSecureNote(vaultID, itemID))
}

// GetOneTimeCode returns app.OneTimeCode ({"code", "expiresAt", "period"}) as JSON; the app asks again once it expires.
func (hisar *Hisar) GetOneTimeCode(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.GetOneTimeCode(vaultID, itemID))
}

// CopyField copies a field to the pasteboard and clears it after 90 seconds if it still holds that value.
func (hisar *Hisar) CopyField(vaultID, itemID, fieldName string) error {
	return hisar.service.CopyField(vaultID, itemID, fieldName)
}

func (hisar *Hisar) GeneratePassword(length int, isDigitIncluded, isSymbolIncluded bool) (string, error) {
	return hisar.service.GeneratePassword(app.PasswordConfig{Length: length, Digits: isDigitIncluded, Symbols: isSymbolIncluded})
}

// ListAllLoginForService returns the logins matching an AutoFill service identifier (a URL or a bare domain), as JSON.
func (hisar *Hisar) ListAllLoginForService(serviceIdentifier string) (string, error) {
	return encodeJSON(app.ListLoginsForURL(hisar.service, convertServiceIdentifierToURL(serviceIdentifier)))
}

// GetLoginForService returns {"username", "password", "hasOneTimePassword"} as JSON after checking the login matches the service again.
func (hisar *Hisar) GetLoginForService(vaultID, itemID, serviceIdentifier string) (string, error) {
	return encodeJSON(app.GetLoginForURL(hisar.service, vaultID, itemID, convertServiceIdentifierToURL(serviceIdentifier)))
}

// GetOneTimeCodeForService returns app.OneTimeCode as JSON after checking the login matches the service, as GetLoginForService does.
func (hisar *Hisar) GetOneTimeCodeForService(vaultID, itemID, serviceIdentifier string) (string, error) {
	return encodeJSON(app.GetOneTimeCodeForURL(hisar.service, vaultID, itemID, convertServiceIdentifierToURL(serviceIdentifier)))
}

func convertServiceIdentifierToURL(serviceIdentifier string) string {
	serviceIdentifier = strings.TrimSpace(serviceIdentifier)
	if strings.Contains(serviceIdentifier, "://") {
		return serviceIdentifier
	}
	return "https://" + serviceIdentifier
}

// ListAllPasskey returns every passkey in a login ([]app.StoredPasskey, binary values base64url) as JSON, for the credential identity store.
func (hisar *Hisar) ListAllPasskey() (string, error) {
	return encodePasskeyJSON(app.ListPasskeys(hisar.service))
}

// ListAllPasskeyForRelyingParty takes the allowed credential IDs as a JSON array of base64url strings; an empty array allows all.
func (hisar *Hisar) ListAllPasskeyForRelyingParty(relyingPartyID, allowedCredentialIDsJSON string) (string, error) {
	allowedCredentialIDs, err := decodeStringArray(allowedCredentialIDsJSON)
	if err != nil {
		return "", err
	}
	return encodePasskeyJSON(app.ListPasskeysForRelyingParty(hisar.service, relyingPartyID, allowedCredentialIDs))
}

// PreparePasskeyRegistration returns {"isExcluded", "logins"} as JSON: the writable logins matching the relying party.
func (hisar *Hisar) PreparePasskeyRegistration(relyingPartyID, excludedCredentialIDsJSON string) (string, error) {
	excludedCredentialIDs, err := decodeStringArray(excludedCredentialIDsJSON)
	if err != nil {
		return "", err
	}
	return encodePasskeyJSON(app.PreparePasskeyRegistration(hisar.service, relyingPartyID, excludedCredentialIDs))
}

// CreatePasskey takes a app.PasskeyRegistration and returns a app.PasskeyRegistrationResult, as JSON; the private key stays in Go.
func (hisar *Hisar) CreatePasskey(registrationJSON string) (string, error) {
	var registration app.PasskeyRegistration
	if err := json.Unmarshal([]byte(registrationJSON), &registration); err != nil {
		return "", errors.New("hisarkit: the passkey registration is not valid JSON")
	}
	return encodePasskeyJSON(app.CreatePasskeyForRelyingParty(hisar.service, registration))
}

// AssertPasskey signs authenticatorData || clientDataHash and returns a app.PasskeyAssertion as JSON; IDs and the hash are base64url.
func (hisar *Hisar) AssertPasskey(relyingPartyID, credentialID, clientDataHash string) (string, error) {
	return encodePasskeyJSON(app.SignPasskeyForRelyingParty(hisar.service, relyingPartyID, credentialID, clientDataHash))
}

// DeletePasskey returns the item without the passkey as app.ItemDetail JSON.
func (hisar *Hisar) DeletePasskey(vaultID, itemID, credentialID string) (string, error) {
	return encodeJSON(hisar.service.DeletePasskey(vaultID, itemID, credentialID))
}

func decodeStringArray(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return nil, errors.New("hisarkit: the credential IDs are not a JSON array of strings")
	}
	return values, nil
}

// encodePasskeyJSON drops the browser protocol's error code, so the app shows only the message.
func encodePasskeyJSON(value any, err error) (string, error) {
	var responseError *browserprotocol.ResponseError
	if errors.As(err, &responseError) {
		err = errors.New(strings.TrimPrefix(responseError.Message, "passkey: "))
	}
	return encodeJSON(value, err)
}

// PrepareItemIcon turns a photo or image into the PNG an item's icon keeps and returns it as base64, as icon.image is in
// the item JSON.
func (hisar *Hisar) PrepareItemIcon(image []byte) (string, error) {
	prepared, err := hisar.service.PrepareItemIcon(image)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(prepared), nil
}

// GetWebsiteIcon returns a login website's icon as a base64 PNG, or "" when there is none, website icons are off or the
// app is locked. It may ask the website, so the app calls it off the main thread.
func (hisar *Hisar) GetWebsiteIcon(rawURL string) (string, error) {
	icon, err := hisar.service.GetWebsiteIcon(rawURL)
	if err != nil || len(icon) == 0 {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(icon), nil
}

// IsWebsiteIconsEnabled reports whether logins show their website's icon.
func (hisar *Hisar) IsWebsiteIconsEnabled() bool {
	return hisar.service.IsWebsiteIconsEnabled()
}

// SetWebsiteIconsEnabled turns website icons on or off; off forgets the icons this phone cached.
func (hisar *Hisar) SetWebsiteIconsEnabled(isEnabled bool) error {
	return hisar.service.SetWebsiteIconsEnabled(isEnabled)
}

// ListAllIconPreset returns the preset icon IDs ([]string) as JSON, in the order the picker shows them.
func (hisar *Hisar) ListAllIconPreset() (string, error) {
	return encodeJSON(item.IconPresets(), nil)
}

// ListAllIconColor returns the preset icon color IDs ([]string) as JSON.
func (hisar *Hisar) ListAllIconColor() (string, error) {
	return encodeJSON(item.IconColors(), nil)
}
