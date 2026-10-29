package app

import (
	"context"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/signin"
)

const (
	StateSignedOut = "signed-out"
	StateLocked    = "locked"
	StateUnlocked  = "unlocked"
)

type Status struct {
	State  string `json:"state"`
	Server string `json:"server"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	// BiometricReady means Touch ID can unlock now: enabled, and the master password was used since launch and recently.
	BiometricAvailable  bool   `json:"biometricAvailable"`
	BiometricEnabled    bool   `json:"biometricEnabled"`
	BiometricReady      bool   `json:"biometricReady"`
	BiometricSetupError string `json:"biometricSetupError,omitempty"`
	// BiometricUnavailableReason says why biometric unlock is unavailable when the platform can tell ("No fingerprint is
	// enrolled…"); empty when it is available or the platform cannot say.
	BiometricUnavailableReason string `json:"biometricUnavailableReason,omitempty"`
	// IsOffline: the server could not be reached, so vaults and items come from this device's encrypted cache.
	IsOffline bool `json:"isOffline"`
	// AutoLockSeconds is how long Hisar stays unlocked without activity.
	AutoLockSeconds int `json:"autoLockSeconds"`
	// SignedOutReason says why the server signed this device out; empty after a sign-out on this device.
	SignedOutReason string `json:"signedOutReason"`
	// UpdateRequired is set once the server refused this version; nothing works until Hisar updates.
	UpdateRequired *UpdateRequirement `json:"updateRequired"`
	// EmergencyKit is the Emergency Kit this device still owes for the installed account; nil when none is owed.
	EmergencyKit *EmergencyKitStatus `json:"emergencyKit,omitempty"`
	// BiometricOfferAvailable: the optional biometric offer after a new account's kit screen can turn biometric unlock on
	// without the master password, because this unlock still holds the account unlock key and biometric unlock is off.
	BiometricOfferAvailable bool `json:"biometricOfferAvailable,omitempty"`
}

// EmergencyKitStatus is the Emergency Kit this device still owes for the installed account: the account's own sign-up or
// Secret Key replacement, kept in protected storage, whose kit delivery is not acknowledged yet.
type EmergencyKitStatus struct {
	OperationID string `json:"operationId"`
	// Kind is "signup" or "replace-secret-key".
	Kind string `json:"kind"`
	// SavedBy is how the platform confirmed a delivery: "file", "print" or "share"; "" before one.
	SavedBy string `json:"savedBy"`
	// PostponedAt is when the kit was first left unsaved, RFC 3339 in UTC: the first Save later, or the first unlock that
	// found it still owed; "" before either.
	PostponedAt string `json:"postponedAt"`
	// Reminder is "new", "3-days" or "14-days" after PostponedAt.
	Reminder string `json:"reminder"`
	// IsAuthorized: this unlock may show the Secret Key, save the kit and acknowledge it without asking again.
	IsAuthorized bool `json:"isAuthorized"`
	// IsReminderHidden: Not now hid the reminder until the next lock.
	IsReminderHidden bool `json:"isReminderHidden"`
}

const (
	EmergencyKitSavedByFile  = "file"
	EmergencyKitSavedByPrint = "print"
	EmergencyKitSavedByShare = "share"

	EmergencyKitReminderNew          = "new"
	EmergencyKitReminderThreeDays    = "3-days"
	EmergencyKitReminderFourteenDays = "14-days"
)

// SignUpPolicy is who may create an account on a server without an invitation.
type SignUpPolicy struct {
	// Server is the canonical server address, with its path prefix, e.g. http://127.0.0.1:19400/sub.
	Server string `json:"server"`
	// Host is the server as the forms show it, without https://.
	Host string `json:"host"`
	// Mode is "open", "invite", "setup" or "unknown" (a server without the policy route, or a mode this app does not know).
	Mode string `json:"mode"`
}

const SignUpModeUnknown = "unknown"

// Biometric is the platform's user-presence check and keychain storage for the biometric unlock key.
type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
	StoreKey(accountID string, key []byte) error
	LoadKey(accountID string) ([]byte, error)
	DeleteKey(accountID string) error
}

// BiometricKeyPreparer is for platforms whose biometric prompt must carry the key operation itself (Android's per-use
// Keystore key). Each Prepare readies one StoreKey or LoadKey for the account; the Authenticate after it authorizes it.
type BiometricKeyPreparer interface {
	PrepareStoreKey(accountID string) error
	PrepareLoadKey(accountID string) error
}

// BiometricKeyMigrator upgrades an existing OS wrapper only after password unlock.
// IsKeyProtected must not prompt and must reject any still-readable legacy wrapper.
type BiometricKeyMigrator interface {
	IsKeyProtected(accountID string) bool
	MigrateKey(accountID string, sealedUnlockKey []byte) error
}

// BiometricSessionKeyStore is a Biometric whose keys last only as long as the process, so nothing that opens the vault
// is at rest: Linux, where no keyring item can require the fingerprint. HasKey must not prompt. Until a password unlock
// seals the account unlock key with a new key, biometric unlock waits for the master password, as it does after every
// launch anyway.
type BiometricSessionKeyStore interface {
	HasKey(accountID string) bool
}

// BiometricAvailabilityReporter says why the biometric check is unavailable, in a sentence the user can act on.
type BiometricAvailabilityReporter interface {
	GetUnavailableReason() string
}

type VaultSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Role      string `json:"role"`
	ItemCount int    `json:"itemCount"`
	// OrganizationID is set for organization vaults only.
	OrganizationID  string              `json:"organizationId,omitempty"`
	ReadIssues      []session.ReadIssue `json:"readIssues,omitempty"`
	IsPersonalExtra bool                `json:"isPersonalExtra,omitempty"`
}

type OrganizationSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	MemberCount int    `json:"memberCount"`
}

// OrganizationInvitation is an invitation to this user's email; ExpiresAt is RFC 3339.
type OrganizationInvitation struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	Role             string `json:"role"`
	InvitedByName    string `json:"invitedByName"`
	InvitedByEmail   string `json:"invitedByEmail"`
	ExpiresAt        string `json:"expiresAt"`
}

type Organizations struct {
	Organizations []OrganizationSummary    `json:"organizations"`
	Invitations   []OrganizationInvitation `json:"invitations"`
	// IsCached means only saved organization IDs and names are available, not live membership or invitations.
	IsCached bool `json:"isCached,omitempty"`
}

// OrganizationMember: SecurityCode and IsKeyChanged as for Person.
type OrganizationMember struct {
	AccountID     string `json:"accountId"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

type TeamMember struct {
	AccountID     string `json:"accountId"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	IsCurrentUser bool   `json:"isCurrentUser"`
}

// TeamVault is a vault the team was given. Vault names are encrypted, so Name is empty when CanOpen is false:
// this user has no key for it (yet).
type TeamVault struct {
	VaultID string `json:"vaultId"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	CanOpen bool   `json:"canOpen"`
}

type Team struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Members []TeamMember `json:"members"`
	Vaults  []TeamVault  `json:"vaults"`
}

// SentOrganizationInvitation is a pending invitation to the organization; only admins see them. Times are RFC 3339.
type SentOrganizationInvitation struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
}

// Organization is what its members see of it; Invitations is empty unless IsAdmin.
type Organization struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Role        string                       `json:"role"`
	IsAdmin     bool                         `json:"isAdmin"`
	Members     []OrganizationMember         `json:"members"`
	Teams       []Team                       `json:"teams"`
	Invitations []SentOrganizationInvitation `json:"invitations"`
}

// WaitingVaultKey is an organization vault this user's teams give them that no owner's app has delivered yet.
type WaitingVaultKey struct {
	VaultID          string `json:"vaultId"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	Role             string `json:"role"`
}

// WaitingVaultKeys: DeliveryError and RotationError say why this app could not deliver a key to someone or rotate a
// vault it owns, e.g. a changed signing key.
type WaitingVaultKeys struct {
	Waiting       []WaitingVaultKey `json:"waiting"`
	DeliveryError string            `json:"deliveryError"`
	RotationError string            `json:"rotationError"`
}

// Person is the user or someone they share a vault with. SecurityCode is derived from their signing key; IsKeyChanged
// means it differs from the key pinned when this device first shared with them.
type Person struct {
	AccountID     string `json:"accountId"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

type ReceivedShareInvitation struct {
	ID             string `json:"id"`
	VaultID        string `json:"vaultId"`
	Role           string `json:"role"`
	InvitedByName  string `json:"invitedByName"`
	InvitedByEmail string `json:"invitedByEmail"`
	ExpiresAt      string `json:"expiresAt"`
}

// SentShareInvitation: State is "pending" until the person accepts, then "accepted" until this app completes the share;
// RecipientName and AcceptedAt (RFC 3339) are empty while pending.
type SentShareInvitation struct {
	ID                   string `json:"id"`
	VaultID              string `json:"vaultId"`
	VaultName            string `json:"vaultName"`
	Email                string `json:"email"`
	RecipientName        string `json:"recipientName"`
	Role                 string `json:"role"`
	State                string `json:"state"`
	ExpiresAt            string `json:"expiresAt"`
	AcceptedAt           string `json:"acceptedAt"`
	RequiresConfirmation bool   `json:"requiresConfirmation"`
	RecipientID          string `json:"recipientId"`
	SecurityCode         string `json:"securityCode"`
	ReviewToken          string `json:"reviewToken"`
}

// WaitingShareInvitation is an invitation this user accepted that the owner's app has not completed yet; AcceptedAt is RFC 3339.
type WaitingShareInvitation struct {
	ID             string `json:"id"`
	VaultID        string `json:"vaultId"`
	Role           string `json:"role"`
	InvitedByName  string `json:"invitedByName"`
	InvitedByEmail string `json:"invitedByEmail"`
	AcceptedAt     string `json:"acceptedAt"`
}

// ShareInvitations: CompletionError says why an accepted share could not be completed, e.g. a changed signing key.
type ShareInvitations struct {
	Received        []ReceivedShareInvitation `json:"received"`
	Sent            []SentShareInvitation     `json:"sent"`
	Waiting         []WaitingShareInvitation  `json:"waiting"`
	CompletionError string                    `json:"completionError"`
}

// ShareAwaitingUnlock is a share someone accepted that this locked app finishes once unlocked; the vault name is encrypted until then.
type ShareAwaitingUnlock struct {
	InvitationID   string `json:"invitationId"`
	RecipientName  string `json:"recipientName"`
	RecipientEmail string `json:"recipientEmail"`
	AcceptedAt     string `json:"acceptedAt"`
}

type VaultMember struct {
	AccountID     string `json:"accountId"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

// Device is a signed-in device of the current account; LastSeenAt is RFC 3339.
type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	IsCurrent  bool   `json:"isCurrent"`
}

type ItemField struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Concealed bool   `json:"concealed"`
}

// ItemDetail never carries an SSH private key; the agent reads it from the cached item.
type ItemDetail struct {
	ID          string             `json:"id"`
	Revision    string             `json:"revision"`
	ReadIssue   *session.ReadIssue `json:"readIssue,omitempty"`
	Category    string             `json:"category"`
	Title       string             `json:"title"`
	Username    string             `json:"username"`
	URLs        []string           `json:"urls"`
	Tags        []string           `json:"tags"`
	Password    string             `json:"password"`
	Notes       string             `json:"notes"`
	Fields      []ItemField        `json:"fields"`
	PublicKey   string             `json:"publicKey"`
	Fingerprint string             `json:"fingerprint"`
	KeyType     string             `json:"keyType"`
	// OneTimePassword is the otpauth:// URI; summaries leave it empty and set HasOneTimePassword instead.
	OneTimePassword    string `json:"oneTimePassword"`
	HasOneTimePassword bool   `json:"hasOneTimePassword"`

	Passkeys []ItemPasskey `json:"passkeys"`
	// AndroidApps are the Android apps a login fills into, which the item stores among its URLs; URLs holds only its
	// websites. SaveItem keeps the stored ones; AddAndroidApp and RemoveAndroidApp change them.
	AndroidApps []ItemAndroidApp `json:"androidApps"`
	// CanConvertToSecureNote: a login with only notes and custom fields, which ConvertToSecureNote turns into a secure note.
	CanConvertToSecureNote bool `json:"canConvertToSecureNote"`
	// Card holds a credit card's values, which are stored as named custom fields; Fields then holds only the others.
	// Summaries keep only the type, brand and last four digits.
	Card *ItemCard `json:"card,omitempty"`
	// Identity holds an identity's values, stored as named custom fields like a card's; summaries keep only the name and
	// email.
	Identity *ItemIdentity `json:"identity,omitempty"`
	// IsFavorite: this user starred the item. Favorites belong to the user, not the item; SaveItem ignores it.
	IsFavorite bool `json:"isFavorite"`
	// APICredential holds an API credential's values, stored as named custom fields like an identity's; summaries keep
	// only the service and host.
	APICredential *ItemAPICredential `json:"apiCredential,omitempty"`
	// Address holds an address item's values, stored as named custom fields like an identity's; summaries keep only the
	// city, country and the first line of the street.
	Address *ItemAddress `json:"address,omitempty"`
	// SoftwareLicense holds a software license's values, stored as named custom fields like an API credential's; summaries
	// keep only the product, version, licensed-to name and email.
	SoftwareLicense *ItemSoftwareLicense `json:"softwareLicense,omitempty"`
	// Server holds a server's host, port and linked SSH key, stored as named custom fields; its username, password and
	// admin console are the item's own Username, Password and URLs. Summaries keep only the host and port.
	Server *ItemServer `json:"server,omitempty"`
	// Database holds a database's engine, host, port, name and options, stored as named custom fields; its username and
	// password are the item's own. Summaries keep only the engine, host, port and name.
	Database *ItemDatabase `json:"database,omitempty"`
	// Icon is the icon the user chose, or nil for the automatic one: a login's website icon (GetWebsiteIcon), else the
	// category's icon or the initials.
	Icon *ItemIcon `json:"icon,omitempty"`
}

// ItemIcon is a preset glyph (an item.IconPresets ID) in one of item.IconColors, or a PNG image (PrepareItemIcon),
// which wins.
type ItemIcon struct {
	Preset string `json:"preset,omitempty"`
	Color  string `json:"color,omitempty"`
	Image  []byte `json:"image,omitempty"`
}

// ItemDatabase is a database's values besides its login data. Engine is the ID of a known engine (postgresql, mysql,
// mariadb, sqlserver, oracle, mongodb, redis) or any other engine as typed; EngineName names it. ConnectionString is built
// from the values and the item's login data, and is "" for another engine or without a host. PortWarning says when the
// port is not a number from 1 to 65535. SaveItem reads only Engine, Host, Port, Name and Options.
type ItemDatabase struct {
	Engine           string `json:"engine"`
	EngineName       string `json:"engineName"`
	Host             string `json:"host"`
	Port             string `json:"port"`
	Name             string `json:"name"`
	Options          string `json:"options"`
	ConnectionString string `json:"connectionString"`
	PortWarning      string `json:"portWarning"`
}

// ItemServer is a server's values besides its login data. SSHKeyID names an ssh-key item in the same vault, or is "";
// SSHKeyTitle and SSHKeyFingerprint describe that key, and are "" when it is gone. PortWarning says when the port is not a
// number from 1 to 65535. SaveItem reads only Host, Port and SSHKeyID.
type ItemServer struct {
	Host              string `json:"host"`
	Port              string `json:"port"`
	SSHKeyID          string `json:"sshKeyId"`
	SSHKeyTitle       string `json:"sshKeyTitle"`
	SSHKeyFingerprint string `json:"sshKeyFingerprint"`
	PortWarning       string `json:"portWarning"`
}

// ItemSoftwareLicense is a software license; the license key is concealed. DownloadURL is a field, not the item's
// website.
type ItemSoftwareLicense struct {
	Product      string `json:"product"`
	Version      string `json:"version"`
	LicenseKey   string `json:"licenseKey"`
	LicensedTo   string `json:"licensedTo"`
	Email        string `json:"email"`
	PurchaseDate string `json:"purchaseDate"`
	DownloadURL  string `json:"downloadUrl"`
}

// ItemAddress is a postal address; Street may hold several lines. Identities and cards hold one too.
type ItemAddress struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	Region     string `json:"region"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
}

// ItemAPICredential is an API key; the secret is concealed. ExpiryWarning says when the expiry has passed.
type ItemAPICredential struct {
	Service       string `json:"service"`
	KeyID         string `json:"keyId"`
	Secret        string `json:"secret"`
	Host          string `json:"host"`
	Expiry        string `json:"expiry"`
	ExpiryWarning string `json:"expiryWarning"`
}

// ItemIdentity is a person's details; the document numbers are optional and concealed. Its address is in the embedded
// ItemAddress; Address is a one-line address saved before identities had one, kept as it is.
type ItemIdentity struct {
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	LastName   string `json:"lastName"`
	BirthDate  string `json:"birthDate"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	ItemAddress
	Address              string `json:"address"`
	PassportNumber       string `json:"passportNumber"`
	DriversLicenseNumber string `json:"driversLicenseNumber"`
	NationalIDNumber     string `json:"nationalIdNumber"`
}

// ItemCard is a card's values. Type is the brand saved with the card, which the user may choose; Brand is the one its
// number points to. Lists show Type, or Brand when the card has no type.
// Its billing address is in the embedded ItemAddress; BillingAddress is a one-line address saved before cards had one, kept
// as it is.
type ItemCard struct {
	Cardholder   string `json:"cardholder"`
	Type         string `json:"type"`
	Number       string `json:"number"`
	Expiry       string `json:"expiry"`
	SecurityCode string `json:"securityCode"`
	PIN          string `json:"pin"`
	ItemAddress
	BillingAddress string `json:"billingAddress"`
	CardDescription
}

// CardDescription is what an editor shows about a card number and expiry; a warning never stops a save.
type CardDescription struct {
	Brand         string `json:"brand"`
	Last4         string `json:"last4"`
	NumberWarning string `json:"numberWarning"`
	ExpiryWarning string `json:"expiryWarning"`
}

// OneTimeCode is the current code of an item's one-time password; ExpiresAt is RFC 3339.
type OneTimeCode struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
	Period    int    `json:"period"`
}

type SignUpResult struct {
	Status             Status   `json:"status"`
	SecretKey          string   `json:"secretKey"`
	PendingOperationID string   `json:"pendingOperationId"`
	SetupState         string   `json:"setupState"`
	Problems           []string `json:"problems"`
}

type PasswordConfig struct {
	Length  int  `json:"length"`
	Digits  bool `json:"digits"`
	Symbols bool `json:"symbols"`
}

type Clipboard interface {
	SetText(text string) bool
	GetText() (string, bool)
}

type LockNotifier interface {
	NotifyLocked()
}

type SecretKeyStore interface {
	LoadSecretKey(accountID string) (crypt.SecretKey, error)
	StoreSecretKey(secretKey crypt.SecretKey) error
	DeleteSecretKey(accountID string) error
}

// AgentGrantHeadStore keeps a random head replaced with every sealed grant update.
// Storage must prevent another program from reading the head without user approval,
// so restoring an older state file cannot restore grants. LoadAgentGrantHead returns
// ErrNoAgentGrantHead for a missing head. Unusable storage returns ErrAgentGrantHeadUnavailable.
type AgentGrantHeadStore interface {
	LoadAgentGrantHead() ([]byte, error)
	SaveAgentGrantHead(head []byte) error
	DeleteAgentGrantHead() error
}

type KeychainSecretKeyStore struct{}

func (KeychainSecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	return localstate.LoadSecretKey(accountID)
}

func (KeychainSecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	return localstate.StoreSecretKey(secretKey)
}

func (KeychainSecretKeyStore) DeleteSecretKey(accountID string) error {
	return localstate.DeleteSecretKey(accountID)
}

type Config struct {
	StatePath string
	// CachePath is the encrypted offline cache; empty keeps Hisar online-only.
	CachePath  string
	DeviceName string
	Platform   string
	// AppVersion goes to the server with every request; empty for development builds, which no minimum refuses.
	AppVersion     string
	Clipboard      Clipboard
	LockNotifier   LockNotifier
	SecretKeyStore SecretKeyStore
	AutoLockAfter  time.Duration
	// GetSystemIdleDuration is how long the whole Mac has had no input; when set, it alone decides auto-lock.
	GetSystemIdleDuration func() (time.Duration, bool)
	ClipboardClearAfter   time.Duration
	Biometric             Biometric
	PasskeyVerifier       PasskeyVerifier
	// BiometricName names the biometric check in messages, as in "turn on Touch ID unlock" and "the Touch ID key".
	BiometricName string
	// BiometricPasswordInterval is how long biometrics may unlock before the master password is required again.
	BiometricPasswordInterval time.Duration
	// IsBiometricAllowedAfterLaunch lets biometrics unlock before the first password unlock, for platforms whose
	// keychain enforces the biometric check itself (iOS); the desktop keeps its launch rule.
	IsBiometricAllowedAfterLaunch bool
	// BiometricKeyPreparer runs before each biometric prompt that guards StoreKey or LoadKey; nil where none is needed.
	BiometricKeyPreparer BiometricKeyPreparer
	Now                  func() time.Time
	// SleepCountingClock reads, from a fixed origin, a clock that keeps counting while the device sleeps, which Go's
	// timers do not (golang/go#24595); auto-lock measures idle time on it.
	SleepCountingClock func() time.Duration
	// OnLock runs under the service mutex whenever a session ends: a lock, a sign-out or an account switch, also from a
	// locked session. It must not call the service.
	OnLock func()
	// OnRemoteChange runs after the backend synced a change another device made; the UI refreshes.
	OnRemoteChange func()
	// OnShareLinksChange runs when one of the account's share links was created, viewed, used up or revoked.
	OnShareLinksChange func()
	// OnApprovalEvent hears approval-requested, approval-answered and approval-resolved; the iOS app lists phone approvals on them.
	OnApprovalEvent func(eventName, requestID string)
	// IsEventStreamDisabled skips the server's event stream, e.g. in tests whose fake server has none.
	IsEventStreamDisabled bool
	// OnThemeChange runs after SetTheme saved a new theme, outside the service mutex; every window applies it.
	OnThemeChange func(theme Theme)
	// OnWebsiteIconsChange runs after SetWebsiteIconsEnabled saved the setting, outside the service mutex; every window
	// applies it.
	OnWebsiteIconsChange func(isEnabled bool)
	// FileSaver asks where to save a file; it returns "" when the user cancels.
	FileSaver FileSaver
	// FileOpener asks which file to open; it returns "" when the user cancels.
	FileOpener FileOpener
	// WebsiteIconFetcher fetches website icons; nil uses itemicon.NewFetcher, which asks the websites themselves.
	WebsiteIconFetcher WebsiteIconFetcher
	// OnUpdateRequired runs on its own goroutine when the server starts refusing this version.
	OnUpdateRequired func(requirement UpdateRequirement)
	// ShowItem shows an item, or its editor, in the app's window for launcher.items.open; nil, as on the iPhone, refuses
	// that request.
	ShowItem func(location ItemLocation)
	// DescribeApp names an app bundle and gives its icon as a data URL, for the coding agent sessions and agent hosts that
	// Settings lists; nil names it by its file name, without an icon.
	DescribeApp func(bundlePath string) (name string, iconURL string)
	// SignInDriver fills browser windows for agent sign-in; nil where agents cannot sign in to websites (every platform
	// but the Mac).
	SignInDriver signin.Driver
	// AgentGrantHeads keeps the head of the saved agent grants; nil keeps a harness's grants for one session only, as the
	// approval dialog then offers.
	AgentGrantHeads AgentGrantHeadStore
}

type WebsiteIconFetcher interface {
	Fetch(ctx context.Context, rawURL string) ([]byte, error)
}

type FileSaver interface {
	ChooseSavePath(defaultFileName string) (string, error)
}

type FileOpener interface {
	ChooseOpenPath() (string, error)
}
