// Package browserprotocol is the native-messaging protocol between the browser extension and the Hisar app:
// framing, request and result types, error codes, passkey messages and URL matching.
package browserprotocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ProtocolVersion = 1
	HostName        = "com.tryhisar.hisar"
	ExtensionID     = "mmipiffoogblblocokimpemdopmdjghf"
	AllowedOrigin   = "chrome-extension://" + ExtensionID + "/"
	MaxMessageSize  = 1 << 20
)

const (
	ErrorInvalidRequest     = "invalid-request"
	ErrorNotPaired          = "not-paired"
	ErrorPairingFailed      = "pairing-failed"
	ErrorSignedOut          = "signed-out"
	ErrorLocked             = "locked"
	ErrorOffline            = "offline"
	ErrorNotFound           = "not-found"
	ErrorNoMatch            = "no-match"
	ErrorDesktopUnavailable = "desktop-unavailable"
	ErrorInternal           = "internal"
	ErrorUpdateRequired     = "update-required"
	// ErrorWrongPassword: unlock.password got a wrong master password, or too many wrong ones in a row.
	ErrorWrongPassword = "wrong-password"
)

var ErrMessageTooLarge = errors.New("browser: message larger than 1 MiB")

type Request struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Token            string `json:"token,omitempty"`
	InstanceID       string `json:"instanceID,omitempty"`
	Browser          string `json:"browser,omitempty"`
	ExtensionVersion string `json:"extensionVersion,omitempty"`
	Code             string `json:"code,omitempty"`
	URL              string `json:"url,omitempty"`
	VaultID          string `json:"vaultID,omitempty"`
	ItemID           string `json:"itemID,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	Length           int    `json:"length,omitempty"`
	Digits           *bool  `json:"digits,omitempty"`
	Symbols          *bool  `json:"symbols,omitempty"`
	// Query, Category and IsFavoriteOnly narrow items.list; FieldID names the field items.reveal returns.
	Query          string `json:"query,omitempty"`
	Category       string `json:"category,omitempty"`
	IsFavoriteOnly bool   `json:"isFavoriteOnly,omitempty"`
	FieldID        string `json:"fieldID,omitempty"`
	IsFavorite     *bool  `json:"isFavorite,omitempty"`
	// Title names a login logins.save saves new (FeatureSaveOptions); Item is what items.create saves (FeatureCreateItems).
	Title string   `json:"title,omitempty"`
	Item  *NewItem `json:"item,omitempty"`

	Passkey json.RawMessage `json:"passkey,omitempty"`
}

type Response struct {
	ID     string         `json:"id"`
	OK     bool           `json:"ok"`
	Result any            `json:"result,omitempty"`
	Error  *ResponseError `json:"error,omitempty"`
}

type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ResponseError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func NewError(code, message string) *ResponseError {
	return &ResponseError{Code: code, Message: message}
}

type ConnectMessage struct {
	Type            string `json:"type"`
	ExtensionOrigin string `json:"extensionOrigin"`
}

// HelloResult: Features lists what this app answers beyond the first version of the protocol, so the extension offers it
// only to an app that has it; an older app sends none.
type HelloResult struct {
	ProtocolVersion int                `json:"protocolVersion"`
	IsPaired        bool               `json:"isPaired"`
	State           string             `json:"state"`
	UpdateRequired  *UpdateRequirement `json:"updateRequired,omitempty"`
	Features        []string           `json:"features,omitempty"`
}

// FeatureAddresses: addresses.list and addresses.fill fill a page's address form from address items and identities.
const FeatureAddresses = "addresses"

// FeatureVault: items.list, items.get, items.reveal, items.code, items.icon and items.favorite let the popup browse
// every vault, not only the logins of the current site.
const FeatureVault = "vault"

// FeatureCards: cards.list and cards.fill fill a page's payment form from credit-card items.
const FeatureCards = "cards"

// FeatureIdentities: identities.list and identities.fill fill a page's name, email, phone and birth date fields, and
// its address form, from identity items.
const FeatureIdentities = "identities"

// FeatureSaveOptions: vaults.list lists the vaults a new login may go into, and logins.save takes a title and a vault
// for a new login.
const FeatureSaveOptions = "save-options"

// FeatureCreateItems: items.create saves a new login, secure note, card or identity from the popup.
const FeatureCreateItems = "create-items"

// FeatureUnlockPassword: unlock.password unlocks Hisar with the master password typed in the popup.
const FeatureUnlockPassword = "unlock-password"

// ListFeatures returns the features this app announces in hello.
func ListFeatures() []string {
	return []string{FeatureAddresses, FeatureVault, FeatureUnlockPassword, FeatureCards, FeatureIdentities, FeatureSaveOptions, FeatureCreateItems}
}

const (
	UpdateTargetApp       = "app"
	UpdateTargetExtension = "extension"
)

// UpdateRequirement names what is below the server's minimum version: the Hisar app or this extension.
type UpdateRequirement struct {
	Target         string `json:"target"`
	CurrentVersion string `json:"currentVersion"`
	MinimumVersion string `json:"minimumVersion"`
	UpdateURL      string `json:"updateURL"`
}

type TokenResult struct {
	Token string `json:"token"`
}

type StatusResult struct {
	State     string `json:"state"`
	Email     string `json:"email"`
	IsOffline bool   `json:"isOffline"`
}

type Login struct {
	VaultID            string `json:"vaultID"`
	ItemID             string `json:"itemID"`
	Title              string `json:"title"`
	Username           string `json:"username"`
	VaultName          string `json:"vaultName"`
	HasOneTimePassword bool   `json:"hasOneTimePassword"`
}

type MatchResult struct {
	Logins []Login `json:"logins"`
}

type FillResult struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	HasOneTimePassword bool   `json:"hasOneTimePassword"`
}

type CheckResult struct {
	Status  string `json:"status"`
	VaultID string `json:"vaultID,omitempty"`
	ItemID  string `json:"itemID,omitempty"`
	Title   string `json:"title,omitempty"`
}

// AddressSource tells where an address offered for filling comes from.
const (
	AddressSourceAddress  = "address"
	AddressSourceIdentity = "identity"
)

// AddressSummary is an address offered for filling: Summary is only its city and country, never the street or postal
// code, which leave the app only in addresses.fill.
type AddressSummary struct {
	VaultID   string `json:"vaultID"`
	ItemID    string `json:"itemID"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Source    string `json:"source"`
	VaultName string `json:"vaultName"`
}

type AddressListResult struct {
	Addresses []AddressSummary `json:"addresses"`
}

// AddressFillResult is the address to fill a form with; Street may hold several lines.
type AddressFillResult struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	Region     string `json:"region"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
}

// ItemSummary is one row of the popup's item list: metadata only. Subtitle is what the app's lists show under the
// title (a login's username, a card's brand and last four digits), never a secret.
type ItemSummary struct {
	VaultID            string `json:"vaultID"`
	ItemID             string `json:"itemID"`
	Category           string `json:"category"`
	Title              string `json:"title"`
	Subtitle           string `json:"subtitle"`
	VaultName          string `json:"vaultName"`
	URL                string `json:"url,omitempty"`
	IsFavorite         bool   `json:"isFavorite"`
	HasOneTimePassword bool   `json:"hasOneTimePassword"`
}

type ItemListResult struct {
	Items []ItemSummary `json:"items"`
}

// The kinds of an item's fields in items.get: a secret's value is left out and read with items.reveal.
const (
	FieldKindText   = "text"
	FieldKindSecret = "secret"
	FieldKindURL    = "url"
	FieldKindNotes  = "notes"
	FieldKindCode   = "one-time-code"
)

// ItemField is one field of an item as the popup shows it. ID names it in items.reveal.
type ItemField struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type ItemResult struct {
	ItemSummary
	Fields []ItemField `json:"fields"`
}

type RevealResult struct {
	Value string `json:"value"`
}

// IconResult: PNG is empty when the website has no icon or website icons are off.
type IconResult struct {
	PNG []byte `json:"png,omitempty"`
}

// CardSummary is a card offered for filling: its brand, last four digits and expiry, never the number or security code,
// which leave the app only in cards.fill.
type CardSummary struct {
	VaultID   string `json:"vaultID"`
	ItemID    string `json:"itemID"`
	Title     string `json:"title"`
	Brand     string `json:"brand"`
	Last4     string `json:"last4"`
	Expiry    string `json:"expiry"`
	VaultName string `json:"vaultName"`
}

type CardListResult struct {
	Cards []CardSummary `json:"cards"`
}

// CardFillResult is the card to fill a payment form with: Number holds only digits, ExpiryMonth two digits and
// ExpiryYear four, either empty when the card has no valid expiry.
type CardFillResult struct {
	Cardholder   string `json:"cardholder"`
	Number       string `json:"number"`
	ExpiryMonth  string `json:"expiryMonth"`
	ExpiryYear   string `json:"expiryYear"`
	SecurityCode string `json:"securityCode"`
	Brand        string `json:"brand"`
}

// IdentitySummary is an identity offered for filling: its name and email, never its phone, birth date or address,
// which leave the app only in identities.fill.
type IdentitySummary struct {
	VaultID   string `json:"vaultID"`
	ItemID    string `json:"itemID"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	VaultName string `json:"vaultName"`
}

type IdentityListResult struct {
	Identities []IdentitySummary `json:"identities"`
}

// IdentityFillResult is the identity to fill a form with; Address is nil when the identity has none worth filling.
// BirthDate is YYYY-MM-DD when the identity's is written year first, else as saved.
type IdentityFillResult struct {
	FirstName  string             `json:"firstName"`
	MiddleName string             `json:"middleName"`
	LastName   string             `json:"lastName"`
	Email      string             `json:"email"`
	Phone      string             `json:"phone"`
	BirthDate  string             `json:"birthDate"`
	Address    *AddressFillResult `json:"address,omitempty"`
}

// VaultSummary is a vault a new item may be saved into; IsPersonal marks the user's own vault, which comes first.
type VaultSummary struct {
	VaultID    string `json:"vaultID"`
	Name       string `json:"name"`
	IsPersonal bool   `json:"isPersonal"`
}

type VaultListResult struct {
	Vaults []VaultSummary `json:"vaults"`
}

// NewItem is an item items.create saves: a login, a secure note, a credit card or an identity, with only the fields
// that category has.
type NewItem struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	URL      string `json:"url,omitempty"`
	Notes    string `json:"notes,omitempty"`

	Cardholder   string `json:"cardholder,omitempty"`
	CardNumber   string `json:"cardNumber,omitempty"`
	Expiry       string `json:"expiry,omitempty"`
	SecurityCode string `json:"securityCode,omitempty"`

	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type SaveResult struct {
	VaultID string `json:"vaultID"`
	ItemID  string `json:"itemID"`
}

type PasswordResult struct {
	Password string `json:"password"`
}

// ReadMessage reads one native messaging frame: a 32-bit length in native byte order, then the JSON.
func ReadMessage(reader io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.NativeEndian, &length); err != nil {
		return nil, err
	}
	if length > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}
	message := make([]byte, length)
	if _, err := io.ReadFull(reader, message); err != nil {
		return nil, err
	}
	return message, nil
}

func WriteMessage(writer io.Writer, message []byte) error {
	if len(message) > MaxMessageSize {
		return ErrMessageTooLarge
	}
	frame := make([]byte, 4+len(message))
	binary.NativeEndian.PutUint32(frame, uint32(len(message)))
	copy(frame[4:], message)
	_, err := writer.Write(frame)
	return err
}
