// Package sharelink builds, seals and opens share links: one item or a piece of text for anyone holding the link.
package sharelink

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

const (
	PayloadVersion      = 1
	MaxPayloadSize      = 48 << 10
	MaxCiphertextSize   = apitypes.ShareLinkMaxCiphertextSize
	MaxLabelSize        = apitypes.ShareLinkMaxLabelSize
	MaxViews            = apitypes.ShareLinkMaxViews
	MinPassphraseLength = 8
	MaxTitleLength      = 200
)

const (
	KindUsername = "username"
	KindPassword = "password"
	KindURL      = "url"
	KindNotes    = "notes"
	KindCustom   = "custom"
	KindText     = "text"
)

// Field keys name what to share; a custom field is "field:<index>" into the item's fields.
const (
	FieldUsername     = "username"
	FieldPassword     = "password"
	FieldURLs         = "urls"
	FieldNotes        = "notes"
	customFieldPrefix = "field:"
)

const (
	ExpiryHour  = apitypes.ShareLinkExpiryHour
	ExpiryDay   = apitypes.ShareLinkExpiryDay
	ExpiryWeek  = apitypes.ShareLinkExpiryWeek
	ExpiryMonth = apitypes.ShareLinkExpiryMonth
)

var (
	ErrNoField            = errors.New("choose at least one field that has a value")
	ErrTooLarge           = errors.New("this is too large to share as a link")
	ErrPassphraseTooShort = fmt.Errorf("use a passphrase of at least %d characters", MinPassphraseLength)
	ErrWrongKey           = errors.New("the link's key or the passphrase doesn't open this link")
	ErrBadLink            = errors.New("this is not a complete share link")
	ErrEmptyText          = errors.New("enter the text to share")
)

type Field struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Value       string `json:"value"`
	IsConcealed bool   `json:"isConcealed"`
}

type Payload struct {
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Fields  []Field `json:"fields"`
}

func GetCustomFieldKey(index int) string { return customFieldPrefix + strconv.Itoa(index) }

func ListAvailableFieldKeys(vaultItem *item.Item) []string {
	var keys []string
	if vaultItem.Overview.Username != "" {
		keys = append(keys, FieldUsername)
	}
	if vaultItem.Details.Password != "" {
		keys = append(keys, FieldPassword)
	}
	if len(vaultItem.Websites()) > 0 {
		keys = append(keys, FieldURLs)
	}
	for index, field := range vaultItem.Details.Fields {
		if field.Value != "" && isShareableField(vaultItem, field) {
			keys = append(keys, GetCustomFieldKey(index))
		}
	}
	if vaultItem.Details.Notes != "" {
		keys = append(keys, FieldNotes)
	}
	return keys
}

func ListDefaultFieldKeys(vaultItem *item.Item) []string {
	var keys []string
	for _, key := range ListAvailableFieldKeys(vaultItem) {
		if key == FieldUsername || key == FieldPassword {
			keys = append(keys, key)
		}
	}
	return keys
}

func DescribeFieldKey(vaultItem *item.Item, key string) string {
	switch key {
	case FieldUsername:
		return "username"
	case FieldPassword:
		return "password"
	case FieldURLs:
		if len(vaultItem.Websites()) == 1 {
			return "website"
		}
		return "websites"
	case FieldNotes:
		return "notes"
	}
	if index, isCustom := parseCustomFieldKey(key, vaultItem); isCustom {
		return vaultItem.Details.Fields[index].Name
	}
	return key
}

func parseCustomFieldKey(key string, vaultItem *item.Item) (int, bool) {
	indexText, isCustom := strings.CutPrefix(key, customFieldPrefix)
	if !isCustom {
		return 0, false
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index >= len(vaultItem.Details.Fields) {
		return 0, false
	}
	return index, true
}

// isShareableField reports whether a custom field may go into a link. A server's SSH key field holds the ID of an item
// the recipient cannot open, and an otpauth:// value is a one-time password secret whatever field holds it, as one does
// after a login becomes another category.
func isShareableField(vaultItem *item.Item, field item.Field) bool {
	if vaultItem.IsCategory(item.CategoryServer) && strings.EqualFold(field.Name, item.FieldSSHKey) {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(strings.TrimSpace(field.Value)), "otpauth://")
}

// BuildItemPayload copies only the chosen fields; key material (SSH private keys, passkeys, one-time password secrets) has no key.
func BuildItemPayload(vaultItem *item.Item, fieldKeys []string) (*Payload, error) {
	payload := &Payload{Version: PayloadVersion, Title: limitText(vaultItem.Overview.Title, MaxTitleLength), Fields: []Field{}}
	isChosen := map[string]bool{}
	for _, key := range fieldKeys {
		if key == FieldUsername || key == FieldPassword || key == FieldURLs || key == FieldNotes {
			isChosen[key] = true
			continue
		}
		index, isCustom := parseCustomFieldKey(key, vaultItem)
		if !isCustom {
			return nil, fmt.Errorf("unknown field %q", key)
		}
		if field := vaultItem.Details.Fields[index]; !isShareableField(vaultItem, field) {
			return nil, fmt.Errorf("%q can't be shared: SSH private keys, passkeys and one-time password secrets never leave Hisar", field.Name)
		}
		isChosen[key] = true
	}
	if isChosen[FieldUsername] && vaultItem.Overview.Username != "" {
		payload.Fields = append(payload.Fields, Field{Kind: KindUsername, Label: "Username", Value: vaultItem.Overview.Username})
	}
	if isChosen[FieldPassword] && vaultItem.Details.Password != "" {
		payload.Fields = append(payload.Fields, Field{Kind: KindPassword, Label: "Password", Value: vaultItem.Details.Password, IsConcealed: true})
	}
	if isChosen[FieldURLs] {
		for _, link := range vaultItem.Websites() {
			payload.Fields = append(payload.Fields, Field{Kind: KindURL, Label: "Website", Value: link})
		}
	}
	for index, field := range vaultItem.Details.Fields {
		if isChosen[GetCustomFieldKey(index)] && field.Value != "" {
			payload.Fields = append(payload.Fields, Field{Kind: KindCustom, Label: field.Name, Value: field.Value, IsConcealed: field.Concealed})
		}
	}
	if isChosen[FieldNotes] && vaultItem.Details.Notes != "" {
		payload.Fields = append(payload.Fields, Field{Kind: KindNotes, Label: "Notes", Value: vaultItem.Details.Notes})
	}
	if len(payload.Fields) == 0 {
		return nil, ErrNoField
	}
	return payload, nil
}

func BuildTextPayload(title, text string) (*Payload, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmptyText
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Shared text"
	}
	return &Payload{Version: PayloadVersion, Title: limitText(title, MaxTitleLength), Fields: []Field{{Kind: KindText, Label: "Text", Value: text}}}, nil
}

func limitText(text string, maximumLength int) string {
	if utf8.RuneCountInString(text) <= maximumLength {
		return text
	}
	return string([]rune(text)[:maximumLength])
}

// Label is sealed with the user key: only the creator's devices read it.
type Label struct {
	Title       string   `json:"title"`
	VaultID     string   `json:"vaultID,omitempty"`
	ItemID      string   `json:"itemID,omitempty"`
	FieldLabels []string `json:"fieldLabels"`
	IsText      bool     `json:"isText,omitempty"`
	LinkKey     []byte   `json:"linkKey"`
}

func SealLabel(userKey []byte, accountID string, label *Label) ([]byte, error) {
	encoded, err := json.Marshal(label)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	sealed, err := crypt.SealShareLinkLabel(userKey, accountID, encoded)
	if err != nil {
		return nil, err
	}
	if len(sealed) > MaxLabelSize {
		return nil, ErrTooLarge
	}
	return sealed, nil
}

func OpenLabel(userKey []byte, accountID string, sealed []byte) (*Label, error) {
	encoded, err := crypt.OpenShareLinkLabel(userKey, accountID, sealed)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	var label Label
	if err := json.Unmarshal(encoded, &label); err != nil {
		return nil, err
	}
	return &label, nil
}

type Sealed struct {
	LinkKey    []byte
	Ciphertext []byte
	Passphrase *crypt.ShareLinkPassphraseParams
}

// Seal encrypts the payload under a fresh link key; an empty passphrase means the link alone opens it.
func Seal(payload *Payload, passphrase []byte) (*Sealed, error) {
	if len(passphrase) > 0 && utf8.RuneCount(passphrase) < MinPassphraseLength {
		return nil, ErrPassphraseTooShort
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	if len(encoded) > MaxPayloadSize {
		return nil, ErrTooLarge
	}
	sealed := &Sealed{LinkKey: make([]byte, crypt.ShareLinkKeySize)}
	rand.Read(sealed.LinkKey)
	if len(passphrase) > 0 {
		params := crypt.GenerateShareLinkPassphraseParams()
		sealed.Passphrase = &params
	}
	contentKey, err := crypt.DeriveShareLinkContentKey(sealed.LinkKey, passphrase, sealed.Passphrase)
	if err != nil {
		return nil, err
	}
	defer clear(contentKey)
	if sealed.Ciphertext, err = crypt.SealShareLink(contentKey, encoded); err != nil {
		return nil, err
	}
	return sealed, nil
}

func Open(ciphertext, linkKey, passphrase []byte, params *crypt.ShareLinkPassphraseParams) (*Payload, error) {
	contentKey, err := crypt.DeriveShareLinkContentKey(linkKey, passphrase, params)
	if err != nil {
		return nil, err
	}
	defer clear(contentKey)
	encoded, err := crypt.OpenShareLink(contentKey, ciphertext)
	if errors.Is(err, crypt.ErrDecrypt) {
		return nil, ErrWrongKey
	}
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	var payload Payload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, err
	}
	if payload.Version != PayloadVersion {
		return nil, fmt.Errorf("unsupported share link version %d", payload.Version)
	}
	return &payload, nil
}

func EncodeLinkKey(linkKey []byte) string { return base64.RawURLEncoding.EncodeToString(linkKey) }

// BuildURL puts the key in the fragment, which browsers never send to the server.
func BuildURL(serverURL, id string, linkKey []byte) string {
	return strings.TrimRight(serverURL, "/") + "/s/" + url.PathEscape(id) + "#" + EncodeLinkKey(linkKey)
}

func ParseURL(link string) (serverURL, id string, linkKey []byte, err error) {
	parsedURL, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return "", "", nil, ErrBadLink
	}
	id, isShareLink := strings.CutPrefix(parsedURL.Path, "/s/")
	if !isShareLink || id == "" || strings.Contains(id, "/") {
		return "", "", nil, ErrBadLink
	}
	linkKey, err = base64.RawURLEncoding.DecodeString(parsedURL.Fragment)
	if err != nil || len(linkKey) != crypt.ShareLinkKeySize {
		return "", "", nil, ErrBadLink
	}
	return parsedURL.Scheme + "://" + parsedURL.Host, id, linkKey, nil
}

func IsAllowedExpiry(expiry time.Duration) bool { return apitypes.IsAllowedShareLinkExpiry(expiry) }

func IsAllowedViews(views int) bool { return views >= 0 && views <= MaxViews }
