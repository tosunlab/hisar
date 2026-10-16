package item

import (
	"crypto/rand"
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"
)

// An empty category is a login: items saved before categories existed have none.
const (
	CategoryLogin      = "login"
	CategorySSHKey     = "ssh-key"
	CategorySecureNote = "secure-note"
)

type Overview struct {
	Title    string   `json:"title"`
	Category string   `json:"category,omitempty"`
	Username string   `json:"username,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	// PublicKey is the authorized_keys line of an ssh-key item.
	PublicKey   string `json:"publicKey,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	KeyType     string `json:"keyType,omitempty"`
	// PasskeyRelyingPartyIDs marks an item with passkeys without the details; UpdatePasskeyOverview keeps it in step.
	PasskeyRelyingPartyIDs []string `json:"passkeyRelyingPartyIds,omitempty"`
}

type Field struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Concealed bool   `json:"concealed,omitempty"`
}

type Details struct {
	Password string  `json:"password,omitempty"`
	Notes    string  `json:"notes,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
	// PrivateKey is an unencrypted OpenSSH PEM key; the vault encryption protects it.
	PrivateKey string `json:"privateKey,omitempty"`
	// OneTimePassword is an otpauth://totp URI (core/totp normalizes pasted secrets into one).
	OneTimePassword string `json:"oneTimePassword,omitempty"`

	Passkeys []Passkey `json:"passkeys,omitempty"`
}

// Passkey is a WebAuthn credential; PrivateKey is the PKCS#8 DER of an ES256 key.
type Passkey struct {
	CredentialID    []byte    `json:"credentialId"`
	RelyingPartyID  string    `json:"relyingPartyId"`
	UserHandle      []byte    `json:"userHandle"`
	UserName        string    `json:"userName,omitempty"`
	UserDisplayName string    `json:"userDisplayName,omitempty"`
	PrivateKey      []byte    `json:"privateKey"`
	CreatedAt       time.Time `json:"createdAt"`
}

type Item struct {
	ID       string
	Revision string
	Overview Overview
	Details  Details
	// IsFavorite is set by importers for an item the other password manager starred. It is never sealed into the item:
	// favorites belong to the user's account.
	IsFavorite bool `json:",omitempty"`
}

// Category returns the item's category; an item saved without one is a login.
func (vaultItem *Item) Category() string {
	if vaultItem.Overview.Category == "" {
		return CategoryLogin
	}
	return vaultItem.Overview.Category
}

// IsCategory reports whether the item has category, counting an item saved without one as a login.
func (vaultItem *Item) IsCategory(category string) bool {
	return vaultItem.Category() == category
}

// HasLoginData reports whether the item holds anything only a login shows: a username, password, website, one-time
// password, private key or passkey.
func HasLoginData(vaultItem *Item) bool {
	return vaultItem.Overview.Username != "" || vaultItem.Details.Password != "" || len(vaultItem.Overview.URLs) > 0 ||
		vaultItem.Details.OneTimePassword != "" || vaultItem.Details.PrivateKey != "" || len(vaultItem.Details.Passkeys) > 0
}

// OmitsLoginData reports whether the item's category is one Hisar knows has no place for login data: a secure note, a
// credit card or an identity. A login keeps its login data, and so does a category from a newer app, which this app edits
// as a login.
func (vaultItem *Item) OmitsLoginData() bool {
	return slices.Contains([]string{CategorySecureNote, CategoryCreditCard, CategoryIdentity, CategoryAPICredential}, vaultItem.Category())
}

// MoveLoginDataToFields turns what only a login shows (username, password, websites and one-time password) into custom
// fields, for an item whose form has no place for them: it then holds nothing its form does not show, and loses nothing.
func (vaultItem *Item) MoveLoginDataToFields() {
	var fields []Field
	if vaultItem.Overview.Username != "" {
		fields = append(fields, Field{Name: "Username", Value: vaultItem.Overview.Username})
	}
	if vaultItem.Details.Password != "" {
		fields = append(fields, Field{Name: "Password", Value: vaultItem.Details.Password, Concealed: true})
	}
	for _, link := range vaultItem.Overview.URLs {
		fields = append(fields, Field{Name: "Website", Value: link})
	}
	if vaultItem.Details.OneTimePassword != "" {
		fields = append(fields, Field{Name: "One-time password", Value: vaultItem.Details.OneTimePassword, Concealed: true})
	}
	vaultItem.Details.Fields = append(vaultItem.Details.Fields, fields...)
	vaultItem.Overview.Username, vaultItem.Overview.URLs = "", nil
	vaultItem.Details.Password, vaultItem.Details.OneTimePassword = "", ""
}

// HasOnlyNotes reports whether a secure note can hold everything the item has: notes, custom fields or both.
func HasOnlyNotes(vaultItem *Item) bool {
	return !HasLoginData(vaultItem) && (vaultItem.Details.Notes != "" || len(vaultItem.Details.Fields) > 0)
}

func UpdatePasskeyOverview(vaultItem *Item) {
	var relyingPartyIDs []string
	for _, passkey := range vaultItem.Details.Passkeys {
		if !slices.Contains(relyingPartyIDs, passkey.RelyingPartyID) {
			relyingPartyIDs = append(relyingPartyIDs, passkey.RelyingPartyID)
		}
	}
	vaultItem.Overview.PasskeyRelyingPartyIDs = relyingPartyIDs
}

// NormalizeTags trims the tags, drops empty ones and repeats, and returns nil when none is left, so an item without
// tags stores no tags field.
func NormalizeTags(tags []string) []string {
	var normalized []string
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" && !slices.Contains(normalized, tag) {
			normalized = append(normalized, tag)
		}
	}
	return normalized
}

// HasTag reports whether the item carries tag, ignoring case.
func (vaultItem *Item) HasTag(tag string) bool {
	return slices.ContainsFunc(vaultItem.Overview.Tags, func(candidate string) bool { return strings.EqualFold(candidate, tag) })
}

const (
	lowerClass  = "abcdefghijkmnopqrstuvwxyz"
	upperClass  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digitClass  = "23456789"
	symbolClass = "!@#$%^&*-_=+?"
)

type GeneratorConfig struct {
	Length  int
	Digits  bool
	Symbols bool
}

func DefaultGeneratorConfig() GeneratorConfig {
	return GeneratorConfig{Length: 24, Digits: true, Symbols: true}
}

// GeneratePassword returns a random password containing at least one character of each enabled class.
func GeneratePassword(config GeneratorConfig) (string, error) {
	classes := []string{lowerClass, upperClass}
	if config.Digits {
		classes = append(classes, digitClass)
	}
	if config.Symbols {
		classes = append(classes, symbolClass)
	}
	if config.Length < len(classes) || config.Length > 256 {
		return "", errors.New("item: invalid password length")
	}
	alphabet := strings.Join(classes, "")
	for {
		password := make([]byte, config.Length)
		for i := range password {
			password[i] = alphabet[generateRandomIndex(len(alphabet))]
		}
		if containsClasses(string(password), classes) {
			return string(password), nil
		}
	}
}

func containsClasses(password string, classes []string) bool {
	for _, class := range classes {
		if !strings.ContainsAny(password, class) {
			return false
		}
	}
	return true
}

func generateRandomIndex(limit int) int {
	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		panic(err)
	}
	return int(randomIndex.Int64())
}
