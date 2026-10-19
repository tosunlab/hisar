package importer

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/passkey"
)

// FormatCredentialExchange is the FIDO Credential Exchange Format (CXF) 1.x, which another password manager hands over
// through the platform (ASCredentialImportManager on iOS) instead of a file. Binary values are base64url, a TOTP secret
// is base32, and times are seconds since 1970.
const FormatCredentialExchange = "credential-exchange"

const credentialExchangeName = "Credential Exchange"

var ErrCredentialExchangeVersion = errors.New("these credentials use a newer Credential Exchange version; update Hisar and try again")

type credentialExchangeHeader struct {
	Version struct {
		Major int `json:"major"`
		Minor int `json:"minor"`
	} `json:"version"`
	ExporterDisplayName string                      `json:"exporterDisplayName"`
	Timestamp           float64                     `json:"timestamp"`
	Accounts            []credentialExchangeAccount `json:"accounts"`
}

type credentialExchangeAccount struct {
	Items []credentialExchangeItem `json:"items"`
}

type credentialExchangeItem struct {
	Title      string  `json:"title"`
	CreationAt float64 `json:"creationAt"`
	Favorite   bool    `json:"favorite"`
	Scope      *struct {
		URLs []string `json:"urls"`
	} `json:"scope"`
	Tags        []string          `json:"tags"`
	Credentials []json.RawMessage `json:"credentials"`
}

// credentialExchangeField is CXF's EditableField.
type credentialExchangeField struct {
	FieldType string `json:"fieldType"`
	Value     string `json:"value"`
	Label     string `json:"label"`
}

func (field *credentialExchangeField) read() string {
	if field == nil {
		return ""
	}
	return strings.TrimSpace(field.Value)
}

type credentialExchangeBasicAuth struct {
	Username *credentialExchangeField `json:"username"`
	Password *credentialExchangeField `json:"password"`
}

type credentialExchangePasskey struct {
	CredentialID    string `json:"credentialId"`
	RelyingPartyID  string `json:"rpId"`
	Username        string `json:"username"`
	UserDisplayName string `json:"userDisplayName"`
	UserHandle      string `json:"userHandle"`
	Key             string `json:"key"`
	FIDO2Extensions *struct {
		HMACCredentials *struct {
			Algorithm        string `json:"algorithm"`
			CredentialWithUV string `json:"credWithUV"`
		} `json:"hmacCredentials"`
	} `json:"fido2Extensions"`
}

type credentialExchangeTOTP struct {
	Secret    string `json:"secret"`
	Period    int    `json:"period"`
	Digits    int    `json:"digits"`
	Username  string `json:"username"`
	Algorithm string `json:"algorithm"`
	Issuer    string `json:"issuer"`
}

type credentialExchangeCustomFields struct {
	Label  string                    `json:"label"`
	Fields []credentialExchangeField `json:"fields"`
}

type credentialExchangeSSHKey struct {
	PrivateKey string `json:"privateKey"`
	KeyComment string `json:"keyComment"`
}

// credentialExchangeRecordFields names the editable fields of the CXF credentials that become a card, an API credential,
// an identity, an address or a Wi-Fi note, in the order Hisar shows the ones it keeps as custom fields.
var credentialExchangeRecordFields = map[string][][2]string{
	"credit-card": {{"number", "Number"}, {"fullName", "Cardholder"}, {"cardType", "Type"}, {"verificationNumber", "Security code"}, {"pin", "PIN"},
		{"expiryDate", "Expiry"}, {"validFrom", "Valid from"}},
	"api-key": {{"key", "Key"}, {"username", "Username"}, {"keyType", "Key type"}, {"url", "URL"}, {"validFrom", "Valid from"}, {"expiryDate", "Expiry"}},
	"address": {{"streetAddress", "Street"}, {"city", "City"}, {"territory", "Region"}, {"postalCode", "Postal code"}, {"country", "Country"},
		{"tel", "Phone"}},
	"person-name": {{"title", "Title"}, {"given", "First name"}, {"givenInformal", "Informal name"}, {"given2", "Middle name"},
		{"surnamePrefix", "Surname prefix"}, {"surname", "Last name"}, {"surname2", "Second surname"}, {"credentials", "Credentials"},
		{"generation", "Generation"}},
	"identity-document": {{"fullName", "ID full name"}, {"identificationNumber", "National ID number"}, {"documentNumber", "ID number"},
		{"issuingCountry", "ID issuing country"}, {"nationality", "Nationality"}, {"birthDate", "Birth date"}, {"birthPlace", "Birth place"},
		{"sex", "Sex"}, {"issueDate", "ID issue date"}, {"expiryDate", "ID expiry date"}, {"issuingAuthority", "ID issuing authority"}},
	"passport": {{"fullName", "Passport full name"}, {"passportNumber", "Passport number"}, {"passportType", "Passport type"},
		{"nationalIdentificationNumber", "National ID number"}, {"issuingCountry", "Passport issuing country"}, {"nationality", "Nationality"},
		{"birthDate", "Birth date"}, {"birthPlace", "Birth place"}, {"sex", "Sex"}, {"issueDate", "Passport issue date"},
		{"expiryDate", "Passport expiry date"}, {"issuingAuthority", "Passport issuing authority"}},
	"drivers-license": {{"fullName", "License full name"}, {"licenseNumber", "Driver's license number"}, {"licenseClass", "License class"},
		{"birthDate", "Birth date"}, {"issueDate", "License issue date"}, {"expiryDate", "License expiry date"},
		{"issuingAuthority", "License issuing authority"}, {"territory", "License region"}, {"country", "License country"}},
	"wifi": {{"ssid", "Network name"}, {"networkSecurityType", "Security"}, {"passphrase", "Password"}, {"hidden", "Hidden network"}},
}

var credentialExchangeIdentityTypes = []string{"person-name", "identity-document", "passport", "drivers-license"}

// credentialExchangeRecord holds one credential's editable fields; take removes a value, and the rest become custom fields.
type credentialExchangeRecord struct {
	kind   string
	values []credentialExchangeValue
}

type credentialExchangeValue struct {
	key, name, value string
	isConcealed      bool
}

func readCredentialExchangeRecord(kind string, raw json.RawMessage) (*credentialExchangeRecord, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	record := &credentialExchangeRecord{kind: kind}
	for _, names := range credentialExchangeRecordFields[kind] {
		encoded, isFound := fields[names[0]]
		if !isFound {
			continue
		}
		var field credentialExchangeField
		if err := json.Unmarshal(encoded, &field); err != nil {
			return nil, err
		}
		if value := field.read(); value != "" {
			record.values = append(record.values, credentialExchangeValue{key: names[0], name: names[1], value: value, isConcealed: field.FieldType == "concealed-string"})
		}
	}
	return record, nil
}

func (record *credentialExchangeRecord) take(key string) string {
	for index, value := range record.values {
		if value.key == key {
			record.values = append(record.values[:index], record.values[index+1:]...)
			return value.value
		}
	}
	return ""
}

func (record *credentialExchangeRecord) remainingFields() []item.Field {
	fields := make([]item.Field, 0, len(record.values))
	for _, value := range record.values {
		fields = append(fields, item.Field{Name: value.name, Value: value.value, Concealed: value.isConcealed})
	}
	record.values = nil
	return fields
}

// decodeCredentialExchangeBinary reads base64url, as CXF requires, and also the padded standard base64 that Apple's
// encoder writes for an SSH key.
func decodeCredentialExchangeBinary(text string) ([]byte, error) {
	text = strings.TrimRight(strings.TrimSpace(text), "=")
	return base64.RawURLEncoding.DecodeString(strings.NewReplacer("+", "-", "/", "_").Replace(text))
}

func readCredentialExchangeTime(seconds float64) time.Time {
	if seconds <= 0 || math.IsInf(seconds, 0) || math.IsNaN(seconds) || seconds > 1e11 {
		return time.Time{}
	}
	return time.Unix(int64(seconds), 0).UTC()
}

// ParseCredentialExchange reads the CXF JSON another password manager exported. Result.Source names that app.
func ParseCredentialExchange(content []byte) (*Result, error) {
	var header credentialExchangeHeader
	if err := json.Unmarshal(content, &header); err != nil {
		return nil, errors.New("the credentials from the other app could not be read")
	}
	if header.Version.Major != 1 {
		return nil, ErrCredentialExchangeVersion
	}
	result := &Result{Format: FormatCredentialExchange, Source: strings.TrimSpace(header.ExporterDisplayName)}
	exportedAt := readCredentialExchangeTime(header.Timestamp)
	for _, account := range header.Accounts {
		for _, entry := range account.Items {
			items, skipped := convertCredentialExchangeItem(entry, exportedAt, result.Source)
			result.Items = append(result.Items, items...)
			result.Skipped = append(result.Skipped, skipped...)
		}
	}
	removeRepeatedItems(result)
	return result, nil
}

// credentialExchangeItemBuilder gathers one CXF item, which may hold several credentials, into Hisar items.
type credentialExchangeItemBuilder struct {
	title       string
	source      string
	createdAt   time.Time
	vaultItem   item.Item
	records     []*credentialExchangeRecord
	sshKeys     []credentialExchangeSSHKey
	sshKeyItems []item.Item
	skipped     []SkippedEntry
}

func (builder *credentialExchangeItemBuilder) skip(reason string) {
	builder.skipped = append(builder.skipped, SkippedEntry{Title: builder.title, Reason: reason})
}

// convertCredentialExchangeItem returns a login, a secure note, a card, an API credential, an identity or an address for
// the item, and an SSH key item for each SSH key it holds. A login keeps any other record's values as custom fields.
func convertCredentialExchangeItem(entry credentialExchangeItem, exportedAt time.Time, source string) ([]item.Item, []SkippedEntry) {
	builder := &credentialExchangeItemBuilder{title: describeEntry(entry.Title), source: source, createdAt: readCredentialExchangeTime(entry.CreationAt)}
	if builder.createdAt.IsZero() {
		builder.createdAt = exportedAt
	}
	builder.vaultItem = item.Item{Overview: item.Overview{Title: strings.TrimSpace(entry.Title), Tags: entry.Tags}, IsFavorite: entry.Favorite}
	if entry.Scope != nil {
		builder.vaultItem.Overview.URLs = entry.Scope.URLs
	}
	for _, raw := range entry.Credentials {
		if err := builder.addCredential(raw); err != nil {
			builder.skip("one of its credentials could not be read")
		}
	}
	builder.applyRecords()
	// SSH keys are built last, so they take the item's notes wherever the note credential stands.
	for _, sshKey := range builder.sshKeys {
		builder.addSSHKey(sshKey)
	}
	items := builder.sshKeyItems
	vaultItem := builder.vaultItem
	if len(vaultItem.Details.Passkeys) > 0 {
		if vaultItem.Overview.Username == "" {
			vaultItem.Overview.Username = vaultItem.Details.Passkeys[0].UserName
		}
		if len(vaultItem.Overview.URLs) == 0 {
			vaultItem.Overview.URLs = []string{"https://" + vaultItem.Details.Passkeys[0].RelyingPartyID}
		}
		item.UpdatePasskeyOverview(&vaultItem)
	}
	isOnlySSHKeys := len(items) > 0 && !item.HasLoginData(&vaultItem) && len(vaultItem.Details.Fields) == 0 && vaultItem.Overview.Category == ""
	if isOnlySSHKeys {
		// The item was an SSH key; its notes went into the key.
		return items, builder.skipped
	}
	if finishItem(&vaultItem) {
		items = append([]item.Item{vaultItem}, items...)
	}
	return items, builder.skipped
}

func (builder *credentialExchangeItemBuilder) addCredential(raw json.RawMessage) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return err
	}
	details := &builder.vaultItem.Details
	switch probe.Type {
	case "basic-auth":
		var credential credentialExchangeBasicAuth
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		username, password := credential.Username.read(), credential.Password.read()
		if builder.vaultItem.Overview.Username == "" && details.Password == "" {
			builder.vaultItem.Overview.Username, details.Password = username, password
			return nil
		}
		details.Fields = append(details.Fields, item.Field{Name: "Username", Value: username}, item.Field{Name: "Password", Value: password, Concealed: true})
	case "generated-password":
		var credential struct {
			Password string `json:"password"`
		}
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		if details.Password == "" {
			details.Password = credential.Password
			return nil
		}
		details.Fields = append(details.Fields, item.Field{Name: "Generated password", Value: credential.Password, Concealed: true})
	case "passkey":
		var credential credentialExchangePasskey
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		builder.addPasskey(credential)
	case "totp":
		var credential credentialExchangeTOTP
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		addOneTimePassword(&builder.vaultItem, "One-time password", buildCredentialExchangeTOTPURI(credential))
	case "note":
		var credential struct {
			Content *credentialExchangeField `json:"content"`
		}
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		details.Notes = joinNonEmpties([]string{details.Notes, credential.Content.read()}, "\n\n")
	case "custom-fields":
		var credential credentialExchangeCustomFields
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		for _, field := range credential.Fields {
			name := getFirstNonEmpty(strings.TrimSpace(field.Label), strings.TrimSpace(credential.Label), "Field")
			details.Fields = append(details.Fields, item.Field{Name: name, Value: field.Value, Concealed: field.FieldType == "concealed-string"})
		}
	case "ssh-key":
		var credential credentialExchangeSSHKey
		if err := json.Unmarshal(raw, &credential); err != nil {
			return err
		}
		builder.sshKeys = append(builder.sshKeys, credential)
	case "item-reference":
		// A link to another item of the export, which Hisar has no place for; the other item is imported on its own.
	default:
		if _, isRecord := credentialExchangeRecordFields[probe.Type]; !isRecord {
			builder.skip(fmt.Sprintf("its %s was not imported; Hisar has no place for it", describeCredentialExchangeType(probe.Type)))
			return nil
		}
		record, err := readCredentialExchangeRecord(probe.Type, raw)
		if err != nil {
			return err
		}
		builder.records = append(builder.records, record)
	}
	return nil
}

func describeCredentialExchangeType(credentialType string) string {
	if credentialType = strings.TrimSpace(credentialType); credentialType == "" {
		return "credential of an unknown type"
	}
	return strings.ReplaceAll(credentialType, "-", " ") + " credential"
}

func (builder *credentialExchangeItemBuilder) addPasskey(credential credentialExchangePasskey) {
	relyingPartyID := strings.TrimSpace(credential.RelyingPartyID)
	credentialID, credentialIDErr := decodeCredentialExchangeBinary(credential.CredentialID)
	userHandle, userHandleErr := decodeCredentialExchangeBinary(credential.UserHandle)
	privateKey, privateKeyErr := decodeCredentialExchangeBinary(credential.Key)
	if credentialIDErr != nil || userHandleErr != nil || privateKeyErr != nil {
		builder.skip(fmt.Sprintf("its passkey for %s could not be read", describeEntry(relyingPartyID)))
		return
	}
	normalizedID, normalizedKey, err := passkey.CheckImportedPasskey(relyingPartyID, credentialID, userHandle, privateKey)
	clear(privateKey)
	switch {
	case errors.Is(err, passkey.ErrUnsupportedKey):
		builder.skip(fmt.Sprintf("its passkey for %s uses a kind of key Hisar cannot sign with; keep using it in %s", relyingPartyID, getFirstNonEmpty(builder.source, "the other app")))
		return
	case err != nil:
		builder.skip(fmt.Sprintf("its passkey for %s is not valid", describeEntry(relyingPartyID)))
		return
	}
	storedPasskey := item.Passkey{
		CredentialID: credentialID, RelyingPartyID: normalizedID, UserHandle: userHandle, UserName: strings.TrimSpace(credential.Username),
		UserDisplayName: strings.TrimSpace(credential.UserDisplayName), PrivateKey: normalizedKey, CreatedAt: builder.createdAt,
	}
	if extensions := credential.FIDO2Extensions; extensions != nil && extensions.HMACCredentials != nil && extensions.HMACCredentials.Algorithm == "hmac-sha256" {
		if secret, err := decodeCredentialExchangeBinary(extensions.HMACCredentials.CredentialWithUV); err == nil && len(secret) == passkey.PRFSecretLength {
			storedPasskey.PRFSecret = secret
		}
	}
	builder.vaultItem.Details.Passkeys = append(builder.vaultItem.Details.Passkeys, storedPasskey)
}

func buildCredentialExchangeTOTPURI(credential credentialExchangeTOTP) string {
	label := strings.TrimSpace(credential.Username)
	if issuer := strings.TrimSpace(credential.Issuer); issuer != "" {
		label = issuer + ":" + label
	}
	query := url.Values{"secret": {strings.TrimSpace(credential.Secret)}}
	if issuer := strings.TrimSpace(credential.Issuer); issuer != "" {
		query.Set("issuer", issuer)
	}
	if algorithm := strings.ToUpper(strings.TrimSpace(credential.Algorithm)); algorithm != "" {
		query.Set("algorithm", algorithm)
	}
	if credential.Digits > 0 {
		query.Set("digits", strconv.Itoa(credential.Digits))
	}
	if credential.Period > 0 {
		query.Set("period", strconv.Itoa(credential.Period))
	}
	return (&url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + label, RawQuery: query.Encode()}).String()
}

func (builder *credentialExchangeItemBuilder) addSSHKey(credential credentialExchangeSSHKey) {
	privateKey, err := decodeCredentialExchangeBinary(credential.PrivateKey)
	if err != nil || len(privateKey) == 0 {
		builder.skip("its SSH key could not be read")
		return
	}
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}))
	clear(privateKey)
	keyItem, reason := buildSSHKeyItem(builder.title, privateKeyPEM, builder.vaultItem.Details.Notes)
	if reason != "" {
		builder.skip(reason)
		return
	}
	keyItem.Overview.Tags = item.NormalizeTags(builder.vaultItem.Overview.Tags)
	builder.sshKeyItems = append(builder.sshKeyItems, keyItem)
}

func (builder *credentialExchangeItemBuilder) takeRecord(kinds ...string) *credentialExchangeRecord {
	for index, record := range builder.records {
		if slices.Contains(kinds, record.kind) {
			builder.records = append(builder.records[:index], builder.records[index+1:]...)
			return record
		}
	}
	return nil
}

// applyRecords makes the item an identity when it holds a name or a document, and otherwise a card, an API credential,
// an address or a Wi-Fi note after its first record, when it has no login data; a login, and any record left over, keeps
// the values as custom fields.
func (builder *credentialExchangeItemBuilder) applyRecords() {
	vaultItem := &builder.vaultItem
	if len(builder.records) > 0 && !item.HasLoginData(vaultItem) {
		isIdentity := slices.ContainsFunc(builder.records, func(record *credentialExchangeRecord) bool {
			return slices.Contains(credentialExchangeIdentityTypes, record.kind)
		})
		switch {
		case isIdentity:
			builder.applyIdentity()
		case builder.records[0].kind == "credit-card":
			builder.applyCard(builder.takeRecord("credit-card"))
		case builder.records[0].kind == "api-key":
			builder.applyAPICredential(builder.takeRecord("api-key"))
		case builder.records[0].kind == "address":
			builder.applyAddress(builder.takeRecord("address"))
		default:
			record := builder.takeRecord("wifi")
			vaultItem.Overview.Category = item.CategorySecureNote
			vaultItem.Details.Fields = append(record.remainingFields(), vaultItem.Details.Fields...)
		}
	}
	for _, record := range builder.records {
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, record.remainingFields()...)
	}
	builder.records = nil
}

func (builder *credentialExchangeItemBuilder) applyCard(record *credentialExchangeRecord) {
	card := item.Card{
		Number: record.take("number"), Cardholder: record.take("fullName"), Type: record.take("cardType"), SecurityCode: record.take("verificationNumber"),
		PIN: record.take("pin"), Expiry: record.take("expiryDate"),
	}
	builder.vaultItem.Overview.Category = item.CategoryCreditCard
	item.WriteCard(&builder.vaultItem, card, append(record.remainingFields(), builder.vaultItem.Details.Fields...))
}

func (builder *credentialExchangeItemBuilder) applyAPICredential(record *credentialExchangeRecord) {
	credential := item.APICredential{Secret: record.take("key"), KeyID: record.take("username"), Host: record.take("url"), Expiry: record.take("expiryDate")}
	builder.vaultItem.Overview.Category = item.CategoryAPICredential
	item.WriteAPICredential(&builder.vaultItem, credential, append(record.remainingFields(), builder.vaultItem.Details.Fields...))
}

func takeCredentialExchangeAddress(record *credentialExchangeRecord) item.Address {
	return item.Address{
		Street: record.take("streetAddress"), City: record.take("city"), Region: record.take("territory"), PostalCode: record.take("postalCode"),
		Country: record.take("country"),
	}
}

func (builder *credentialExchangeItemBuilder) applyAddress(record *credentialExchangeRecord) {
	address := takeCredentialExchangeAddress(record)
	builder.vaultItem.Overview.Category = item.CategoryAddress
	item.WriteAddress(&builder.vaultItem, address, append(record.remainingFields(), builder.vaultItem.Details.Fields...))
}

// applyIdentity joins a person's name, documents and first address into one identity.
func (builder *credentialExchangeItemBuilder) applyIdentity() {
	identity := item.Identity{}
	var otherFields []item.Field
	if name := builder.takeRecord("person-name"); name != nil {
		identity.FirstName, identity.MiddleName = name.take("given"), name.take("given2")
		identity.LastName = joinNonEmpties([]string{name.take("surnamePrefix"), name.take("surname"), name.take("surname2")}, " ")
		otherFields = append(otherFields, name.remainingFields()...)
	}
	fullName := ""
	for record := builder.takeRecord(credentialExchangeIdentityTypes...); record != nil; record = builder.takeRecord(credentialExchangeIdentityTypes...) {
		fullName = getFirstNonEmpty(fullName, record.take("fullName"))
		identity.BirthDate = getFirstNonEmpty(identity.BirthDate, record.take("birthDate"))
		switch record.kind {
		case "passport":
			identity.PassportNumber = getFirstNonEmpty(identity.PassportNumber, record.take("passportNumber"))
			identity.NationalIDNumber = getFirstNonEmpty(identity.NationalIDNumber, record.take("nationalIdentificationNumber"))
		case "drivers-license":
			identity.DriversLicenseNumber = getFirstNonEmpty(identity.DriversLicenseNumber, record.take("licenseNumber"))
		case "identity-document":
			identity.NationalIDNumber = getFirstNonEmpty(identity.NationalIDNumber, record.take("identificationNumber"), record.take("documentNumber"))
		}
		otherFields = append(otherFields, record.remainingFields()...)
	}
	if identity.FirstName == "" && identity.LastName == "" {
		identity.FirstName, identity.LastName = splitFullName(fullName)
	}
	if address := builder.takeRecord("address"); address != nil {
		identity.Phone = address.take("tel")
		identity.Address = takeCredentialExchangeAddress(address)
		otherFields = append(otherFields, address.remainingFields()...)
	}
	builder.vaultItem.Overview.Category = item.CategoryIdentity
	item.WriteIdentity(&builder.vaultItem, identity, append(otherFields, builder.vaultItem.Details.Fields...))
}
