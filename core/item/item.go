package item

import (
	"crypto/rand"
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"
)

const (
	CategoryLogin  = "login"
	CategorySSHKey = "ssh-key"
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
}

func UpdatePasskeyOverview(vaultItem *Item) {
	var allRelyingPartyID []string
	for _, passkey := range vaultItem.Details.Passkeys {
		if !slices.Contains(allRelyingPartyID, passkey.RelyingPartyID) {
			allRelyingPartyID = append(allRelyingPartyID, passkey.RelyingPartyID)
		}
	}
	vaultItem.Overview.PasskeyRelyingPartyIDs = allRelyingPartyID
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

var DefaultGeneratorConfig = GeneratorConfig{Length: 24, Digits: true, Symbols: true}

// GeneratePassword returns a random password containing at least one character of each enabled class.
func GeneratePassword(config GeneratorConfig) (string, error) {
	allClass := []string{lowerClass, upperClass}
	if config.Digits {
		allClass = append(allClass, digitClass)
	}
	if config.Symbols {
		allClass = append(allClass, symbolClass)
	}
	if config.Length < len(allClass) || config.Length > 256 {
		return "", errors.New("item: invalid password length")
	}
	alphabet := strings.Join(allClass, "")
	for {
		password := make([]byte, config.Length)
		for i := range password {
			password[i] = alphabet[generateRandomIndex(len(alphabet))]
		}
		if containsAllClass(string(password), allClass) {
			return string(password), nil
		}
	}
}

func containsAllClass(password string, allClass []string) bool {
	for _, class := range allClass {
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
