// Package export writes and reads Hisar's own export files: plain JSON, or that JSON sealed with a passphrase.
package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

const (
	FormatPlain     = "hisar-export"
	FormatEncrypted = "hisar-encrypted-export"
	Version         = 1

	KDFArgon2id         = "argon2id"
	CipherXChaCha20     = "xchacha20-poly1305"
	MinPassphraseLength = 10
	MaxFileSize         = 100 << 20
)

var (
	ErrNotExport             = errors.New("this is not a Hisar export")
	ErrUnsupportedVersion    = errors.New("this Hisar export was made by a newer version of Hisar; update Hisar and try again")
	ErrPassphraseRequired    = errors.New("this Hisar export is encrypted; enter its export passphrase")
	ErrWrongPassphrase       = errors.New("that passphrase doesn't open this export")
	ErrPassphraseTooShort    = fmt.Errorf("use at least %d characters for the export passphrase", MinPassphraseLength)
	ErrDamagedExport         = errors.New("this Hisar export is damaged")
	ErrUnsupportedKDF        = errors.New("this Hisar export uses a key derivation Hisar doesn't know")
	ErrUnsupportedCipher     = errors.New("this Hisar export uses a cipher Hisar doesn't know")
	ErrBadPassphraseSettings = errors.New("this Hisar export's passphrase settings are outside the allowed range")
)

// Document is the plain export, and the plaintext of an encrypted one.
type Document struct {
	Format     string      `json:"format"`
	Version    int         `json:"version"`
	ExportedAt time.Time   `json:"exportedAt"`
	Account    Account     `json:"account"`
	Vaults     []Vault     `json:"vaults"`
	Provenance *Provenance `json:"provenance,omitempty"`
}

type Account struct {
	Email  string `json:"email"`
	Server string `json:"server"`
}

// Vault is one exported vault. NameUnreadable marks a recovery container: the vault's name did not open, so Name is a
// label made from its ID, not the name its owner gave it.
type Vault struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NameUnreadable bool   `json:"nameUnreadable,omitempty"`
	Type           string `json:"type"`
	Items          []Item `json:"items"`
}

type Item struct {
	ID       string        `json:"id"`
	Overview item.Overview `json:"overview"`
	Details  item.Details  `json:"details"`
}

// EncryptedDocument: Ciphertext is version (0x01) || nonce (24) || XChaCha20-Poly1305 of the plain Document's JSON.
type EncryptedDocument struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	KDF        KDF    `json:"kdf"`
	Cipher     string `json:"cipher"`
	Ciphertext []byte `json:"ciphertext"`
}

type KDF struct {
	Algorithm   string `json:"algorithm"`
	Salt        []byte `json:"salt"`
	Iterations  uint32 `json:"iterations"`
	MemoryKiB   uint32 `json:"memoryKiB"`
	Parallelism uint8  `json:"parallelism"`
}

var byteOrderMark = []byte{0xef, 0xbb, 0xbf}

type header struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

func ConvertVault(vaultID, name, vaultType string, items []item.Item) Vault {
	vault := Vault{ID: vaultID, Name: name, Type: vaultType, Items: make([]Item, 0, len(items))}
	for _, vaultItem := range items {
		vault.Items = append(vault.Items, Item{ID: vaultItem.ID, Overview: vaultItem.Overview, Details: vaultItem.Details})
	}
	return vault
}

func Build(account Account, vaults []Vault, exportedAt time.Time) *Document {
	return &Document{Format: FormatPlain, Version: Version, ExportedAt: exportedAt.UTC(), Account: account, Vaults: vaults}
}

func CheckPassphrase(passphrase []byte) error {
	if utf8.RuneCount(passphrase) < MinPassphraseLength {
		return ErrPassphraseTooShort
	}
	return nil
}

func EncodePlain(document *Document) ([]byte, error) {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func EncodeEncrypted(document *Document, passphrase []byte) ([]byte, error) {
	return encodeEncryptedWithParams(document, passphrase, crypt.DefaultArgon2Params())
}

func encodeEncryptedWithParams(document *Document, passphrase []byte, params crypt.Argon2Params) ([]byte, error) {
	if err := CheckPassphrase(passphrase); err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	salt := crypt.GenerateSalt()
	exportKey, err := crypt.DeriveExportKey(passphrase, salt, params)
	if err != nil {
		return nil, err
	}
	defer clear(exportKey)
	ciphertext, err := crypt.SealExport(exportKey, plaintext)
	if err != nil {
		return nil, err
	}
	encrypted := EncryptedDocument{
		Format:  FormatEncrypted,
		Version: Version,
		KDF: KDF{
			Algorithm: KDFArgon2id, Salt: salt, Iterations: params.Time, MemoryKiB: params.MemoryKiB, Parallelism: params.Threads,
		},
		Cipher:     CipherXChaCha20,
		Ciphertext: ciphertext,
	}
	encoded, err := json.MarshalIndent(encrypted, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// DetectFormat returns FormatPlain or FormatEncrypted for a Hisar export, or ErrNotExport.
func DetectFormat(content []byte) (string, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(content, byteOrderMark))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", ErrNotExport
	}
	var fileHeader header
	if err := json.Unmarshal(trimmed, &fileHeader); err != nil {
		return "", ErrNotExport
	}
	switch fileHeader.Format {
	case FormatPlain, FormatEncrypted:
	default:
		return "", ErrNotExport
	}
	if fileHeader.Version != Version {
		return "", ErrUnsupportedVersion
	}
	return fileHeader.Format, nil
}

// Decode reads either format; passphrase is only used for an encrypted export.
func Decode(content, passphrase []byte) (*Document, error) {
	format, err := DetectFormat(content)
	if err != nil {
		return nil, err
	}
	content = bytes.TrimPrefix(content, byteOrderMark)
	if format == FormatEncrypted {
		if len(passphrase) == 0 {
			return nil, ErrPassphraseRequired
		}
		if content, err = decrypt(content, passphrase); err != nil {
			return nil, err
		}
		defer clear(content)
	}
	var document Document
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, ErrDamagedExport
	}
	if document.Format != FormatPlain || document.Version != Version {
		return nil, ErrDamagedExport
	}
	for index := range document.Vaults {
		if strings.TrimSpace(document.Vaults[index].Name) == "" {
			return nil, ErrDamagedExport
		}
	}
	return &document, nil
}

func decrypt(content, passphrase []byte) ([]byte, error) {
	var encrypted EncryptedDocument
	if err := json.Unmarshal(content, &encrypted); err != nil {
		return nil, ErrDamagedExport
	}
	if encrypted.KDF.Algorithm != KDFArgon2id {
		return nil, ErrUnsupportedKDF
	}
	if encrypted.Cipher != CipherXChaCha20 {
		return nil, ErrUnsupportedCipher
	}
	params := crypt.Argon2Params{Time: encrypted.KDF.Iterations, MemoryKiB: encrypted.KDF.MemoryKiB, Threads: encrypted.KDF.Parallelism}
	if params.Validate() != nil || len(encrypted.KDF.Salt) != crypt.SaltSize {
		return nil, ErrBadPassphraseSettings
	}
	exportKey, err := crypt.DeriveExportKey(passphrase, encrypted.KDF.Salt, params)
	if err != nil {
		return nil, err
	}
	defer clear(exportKey)
	plaintext, err := crypt.OpenExport(exportKey, encrypted.Ciphertext)
	if errors.Is(err, crypt.ErrDecrypt) {
		return nil, ErrWrongPassphrase
	}
	if err != nil {
		return nil, ErrDamagedExport
	}
	return plaintext, nil
}

// ConvertToItem returns an item without ID or revision, ready to be saved as a new item.
func (exported Item) ConvertToItem() item.Item {
	vaultItem := item.Item{Overview: exported.Overview, Details: exported.Details}
	item.UpdatePasskeyOverview(&vaultItem)
	return vaultItem
}

func (document *Document) CountItem() int {
	itemCount := 0
	for _, vault := range document.Vaults {
		itemCount += len(vault.Items)
	}
	return itemCount
}
