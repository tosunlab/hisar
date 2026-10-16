package importer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
)

const maxOnePasswordDataSize = 64 << 20

const (
	onePasswordLogin           = "001"
	onePasswordCreditCard      = "002"
	onePasswordSecureNote      = "003"
	onePasswordIdentity        = "004"
	onePasswordPassword        = "005"
	onePasswordSoftwareLicense = "100"
	onePasswordLicense         = "103"
	onePasswordPassport        = "106"
	onePasswordSSN             = "108"
	onePasswordServer          = "110"
	onePasswordAPICredential   = "112"
	onePasswordSSHKey          = "114"
)

var onePasswordCategoryNames = map[string]string{
	"001": "Login", "002": "Credit Card", "003": "Secure Note", "004": "Identity", "005": "Password", "006": "Document",
	"100": "Software License", "101": "Bank Account", "102": "Database", "103": "Driver License", "104": "Outdoor License",
	"105": "Membership", "106": "Passport", "107": "Reward Program", "108": "Social Security Number", "109": "Wireless Router",
	"110": "Server", "111": "Email Account", "112": "API Credential", "113": "Medical Record", "114": "SSH Key", "115": "Crypto Wallet",
}

// onePasswordCredentialCategories become logins with their fields; other categories hold personal records Hisar has no type for yet.
var onePasswordCredentialCategories = map[string]bool{"102": true, "109": true, "111": true}

type onePasswordExport struct {
	Accounts []struct {
		Vaults []struct {
			Items []onePasswordItem `json:"items"`
		} `json:"vaults"`
	} `json:"accounts"`
}

type onePasswordItem struct {
	State string `json:"state"`
	// FavIndex is the item's place in 1Password's Favorites; 0 means it is not a favorite.
	FavIndex     float64 `json:"favIndex"`
	CategoryUUID string  `json:"categoryUuid"`
	Details      struct {
		LoginFields []struct {
			Value       string `json:"value"`
			Name        string `json:"name"`
			FieldType   string `json:"fieldType"`
			Designation string `json:"designation"`
		} `json:"loginFields"`
		NotesPlain string `json:"notesPlain"`
		Password   string `json:"password"`
		Sections   []struct {
			Title  string             `json:"title"`
			Fields []onePasswordField `json:"fields"`
		} `json:"sections"`
	} `json:"details"`
	Overview struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		URLs  []struct {
			URL string `json:"url"`
		} `json:"urls"`
		Tags []string `json:"tags"`
	} `json:"overview"`
}

type onePasswordField struct {
	Title string                     `json:"title"`
	ID    string                     `json:"id"`
	Value map[string]json.RawMessage `json:"value"`
}

func readOnePasswordData(content []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, errors.New("this .1pux file is damaged; export it from 1Password again")
	}
	for _, file := range archive.File {
		if file.Name != "export.data" {
			continue
		}
		if file.UncompressedSize64 > maxOnePasswordDataSize {
			return nil, errors.New("this 1Password export is too large to import")
		}
		reader, err := file.Open()
		if err != nil {
			return nil, errors.New("this .1pux file is damaged; export it from 1Password again")
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, maxOnePasswordDataSize+1))
		if err != nil || len(data) > maxOnePasswordDataSize {
			return nil, errors.New("this .1pux file is damaged; export it from 1Password again")
		}
		return data, nil
	}
	return nil, errors.New("this ZIP file is not a 1Password export; it has no export.data")
}

func parseOnePasswordExport(content []byte) (*Result, error) {
	data, err := readOnePasswordData(content)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	var export onePasswordExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, errors.New("the 1Password export could not be read; export it from 1Password again")
	}
	result := &Result{Items: []item.Item{}, Skipped: []SkippedEntry{}}
	for _, account := range export.Accounts {
		for _, vault := range account.Vaults {
			for _, onePasswordEntry := range vault.Items {
				vaultItem, reason := convertOnePasswordItem(onePasswordEntry)
				if reason != "" {
					result.Skipped = append(result.Skipped, SkippedEntry{Title: describeEntry(onePasswordEntry.Overview.Title), Reason: reason})
					continue
				}
				vaultItem.IsFavorite = onePasswordEntry.FavIndex > 0
				if finishItem(&vaultItem) {
					result.Items = append(result.Items, vaultItem)
				}
			}
		}
	}
	return result, nil
}

// onePasswordCardFields maps the ids of a 1Password Credit Card's fields to the card's values; its other fields (bank,
// valid from, phone numbers…) stay custom fields.
var onePasswordCardFields = map[string]func(*item.Card) *string{
	"cardholder": func(card *item.Card) *string { return &card.Cardholder },
	"type":       func(card *item.Card) *string { return &card.Type },
	"ccnum":      func(card *item.Card) *string { return &card.Number },
	"expiry":     func(card *item.Card) *string { return &card.Expiry },
	"cvv":        func(card *item.Card) *string { return &card.SecurityCode },
	"pin":        func(card *item.Card) *string { return &card.PIN },
	"address":    func(card *item.Card) *string { return &card.BillingAddressText },
}

// onePasswordAPICredentialFields maps the ids of a 1Password API Credential's fields to the credential's values; its other
// fields (type, filename, valid from…) stay custom fields. It has no service field: the title names the service.
var onePasswordAPICredentialFields = map[string]func(*item.APICredential) *string{
	"username":   func(credential *item.APICredential) *string { return &credential.KeyID },
	"credential": func(credential *item.APICredential) *string { return &credential.Secret },
	"hostname":   func(credential *item.APICredential) *string { return &credential.Host },
	"expires":    func(credential *item.APICredential) *string { return &credential.Expiry },
}

// onePasswordIdentityFields maps the ids of the fields of a 1Password Identity, Passport, Driver License or Social
// Security Number to an identity's values; their other fields (sex, occupation, expiry date, issuing country…) stay
// custom fields. The first of the phone numbers becomes the phone, and a document's "number" is its passport, license or
// national ID number.
var onePasswordIdentityFields = map[string]func(*item.Identity) *string{
	"firstname": func(identity *item.Identity) *string { return &identity.FirstName },
	"initial":   func(identity *item.Identity) *string { return &identity.MiddleName },
	"lastname":  func(identity *item.Identity) *string { return &identity.LastName },
	"birthdate": func(identity *item.Identity) *string { return &identity.BirthDate },
	"email":     func(identity *item.Identity) *string { return &identity.Email },
	"defphone":  func(identity *item.Identity) *string { return &identity.Phone },
	"cellphone": func(identity *item.Identity) *string { return &identity.Phone },
	"homephone": func(identity *item.Identity) *string { return &identity.Phone },
	"busphone":  func(identity *item.Identity) *string { return &identity.Phone },
	"address":   func(identity *item.Identity) *string { return &identity.AddressText },
}

// onePasswordLicenseFieldNames renames the fields of a 1Password Driver License that name where it was issued, which would
// otherwise be read as the identity's address, such as "country".
var onePasswordLicenseFieldNames = map[string]string{"country": "Issuing country", "state": "Issuing state"}

// getOnePasswordAddress points at the address an "address" field of a card or identity fills, or returns nil.
func getOnePasswordAddress(card *item.Card, identity *item.Identity, category, fieldID string) *item.Address {
	switch {
	case fieldID != "address":
		return nil
	case category == onePasswordCreditCard:
		return &card.BillingAddress
	case isOnePasswordIdentityCategory(category):
		return &identity.Address
	}
	return nil
}

// readOnePasswordAddress reads a typed address value; 1Password calls the region "state" and the postal code "zip".
func readOnePasswordAddress(valuesByType map[string]json.RawMessage) (item.Address, bool) {
	rawAddress, found := valuesByType["address"]
	if !found {
		return item.Address{}, false
	}
	var address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		State   string `json:"state"`
		Zip     string `json:"zip"`
		Country string `json:"country"`
	}
	if json.Unmarshal(rawAddress, &address) != nil {
		return item.Address{}, false
	}
	return item.Address{Street: address.Street, City: address.City, Region: address.State, PostalCode: address.Zip, Country: address.Country}, true
}

// getOnePasswordIdentityValue points at the identity value a field fills, or returns nil for a field that stays a custom
// field.
func getOnePasswordIdentityValue(identity *item.Identity, category, fieldID string) *string {
	switch {
	case fieldID == "number" && category == onePasswordPassport:
		return &identity.PassportNumber
	case fieldID == "number" && category == onePasswordLicense:
		return &identity.DriversLicenseNumber
	case fieldID == "number" && category == onePasswordSSN:
		return &identity.NationalIDNumber
	}
	if value, found := onePasswordIdentityFields[fieldID]; found {
		return value(identity)
	}
	return nil
}

// onePasswordSoftwareLicenseFields maps the ids of a 1Password Software License's fields to the license's values; its other
// fields (company, publisher, retail price, order number…) stay custom fields. It has no product field: the title names the
// product.
var onePasswordSoftwareLicenseFields = map[string]func(*item.SoftwareLicense) *string{
	"product_version": func(license *item.SoftwareLicense) *string { return &license.Version },
	"reg_code":        func(license *item.SoftwareLicense) *string { return &license.LicenseKey },
	"reg_name":        func(license *item.SoftwareLicense) *string { return &license.LicensedTo },
	"reg_email":       func(license *item.SoftwareLicense) *string { return &license.Email },
	"order_date":      func(license *item.SoftwareLicense) *string { return &license.PurchaseDate },
	"download_link":   func(license *item.SoftwareLicense) *string { return &license.DownloadURL },
}

func convertOnePasswordItem(entry onePasswordItem) (item.Item, string) {
	if entry.State == "archived" {
		return item.Item{}, "archived in 1Password"
	}
	category := entry.CategoryUUID
	switch {
	case category == onePasswordSSHKey:
		return convertOnePasswordSSHKey(entry)
	case category == onePasswordLogin, category == onePasswordPassword, category == onePasswordSecureNote, category == onePasswordCreditCard,
		category == onePasswordAPICredential, category == onePasswordSoftwareLicense, category == onePasswordServer, isOnePasswordIdentityCategory(category), onePasswordCredentialCategories[category]:
	default:
		categoryName, found := onePasswordCategoryNames[category]
		if !found {
			categoryName = "this kind of"
		}
		return item.Item{}, fmt.Sprintf("1Password %s items are not supported in Hisar yet", categoryName)
	}
	vaultItem := item.Item{
		Overview: item.Overview{Title: entry.Overview.Title, Tags: entry.Overview.Tags},
		Details:  item.Details{Password: entry.Details.Password, Notes: entry.Details.NotesPlain},
	}
	if category == onePasswordSecureNote {
		vaultItem.Overview.Category = item.CategorySecureNote
	}
	var card item.Card
	var identity item.Identity
	var credential item.APICredential
	var license item.SoftwareLicense
	var server item.Server
	var serverURL string
	// consoleFieldIndexes holds where a server's admin console URL, username and password landed among the custom fields.
	consoleFieldIndexes := map[string]int{}
	var fullName item.Field
	isIdentity := isOnePasswordIdentityCategory(category)
	if entry.Overview.URL != "" {
		vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, entry.Overview.URL)
	}
	for _, link := range entry.Overview.URLs {
		vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, link.URL)
	}
	for _, loginField := range entry.Details.LoginFields {
		switch {
		case loginField.Designation == "username" && vaultItem.Overview.Username == "":
			vaultItem.Overview.Username = loginField.Value
		case loginField.Designation == "password" && vaultItem.Details.Password == "":
			vaultItem.Details.Password = loginField.Value
		case loginField.FieldType == "C" || loginField.FieldType == "B" || loginField.FieldType == "R":
		default:
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: loginField.Name, Value: loginField.Value, Concealed: loginField.FieldType == "P"})
		}
	}
	for _, section := range entry.Details.Sections {
		for _, sectionField := range section.Fields {
			if _, isFile := sectionField.Value["file"]; isFile {
				continue
			}
			if address := getOnePasswordAddress(&card, &identity, category, sectionField.ID); address != nil && address.IsEmpty() {
				// A typed address fills the card's or identity's address fields; a plain one is kept as one line below.
				if typed, isTyped := readOnePasswordAddress(sectionField.Value); isTyped {
					*address = typed
					continue
				}
			}
			value, isConcealed := convertOnePasswordFieldValue(sectionField.Value)
			if category == onePasswordServer {
				// A server's URL is its host, which NormalizeServer splits into host and port, and its username and password
				// are its own. Its admin console stays a custom field until promoteOnePasswordAdminConsole decides.
				switch {
				case sectionField.ID == "url" && server.Host == "":
					server.Host, serverURL = value, value
					continue
				case sectionField.ID != "url" && promoteOnePasswordField(&vaultItem, sectionField.ID, value):
					continue
				}
			}
			if onePasswordCredentialCategories[category] && promoteOnePasswordField(&vaultItem, sectionField.ID, value) {
				continue
			}
			if cardValue, isCardField := onePasswordCardFields[sectionField.ID]; category == onePasswordCreditCard && isCardField && *cardValue(&card) == "" {
				*cardValue(&card) = value
				continue
			}
			if credentialValue, isCredentialField := onePasswordAPICredentialFields[sectionField.ID]; category == onePasswordAPICredential && isCredentialField && *credentialValue(&credential) == "" {
				*credentialValue(&credential) = value
				continue
			}
			if licenseValue, isLicenseField := onePasswordSoftwareLicenseFields[sectionField.ID]; category == onePasswordSoftwareLicense && isLicenseField && *licenseValue(&license) == "" {
				*licenseValue(&license) = value
				continue
			}
			if identityValue := getOnePasswordIdentityValue(&identity, category, sectionField.ID); isIdentity && identityValue != nil && *identityValue == "" {
				*identityValue = value
				continue
			}
			name := sectionField.Title
			if name == "" {
				name = getFirstNonEmpty(section.Title, sectionField.ID)
			}
			if issuingName, isRenamed := onePasswordLicenseFieldNames[sectionField.ID]; category == onePasswordLicense && isRenamed {
				name = issuingName
			}
			// A Social Security Number calls its full name "name".
			isFullName := sectionField.ID == "fullname" || category == onePasswordSSN && sectionField.ID == "name"
			if isIdentity && isFullName && fullName.Value == "" {
				fullName = item.Field{Name: name, Value: value}
				continue
			}
			if isOnePasswordOneTimePassword(sectionField, value) {
				addOneTimePassword(&vaultItem, name, value)
				continue
			}
			if _, isSeen := consoleFieldIndexes[sectionField.ID]; category == onePasswordServer && strings.HasPrefix(sectionField.ID, "admin_console_") && !isSeen {
				consoleFieldIndexes[sectionField.ID] = len(vaultItem.Details.Fields)
			}
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: name, Value: value, Concealed: isConcealed})
		}
	}
	if category == onePasswordCreditCard {
		vaultItem.Overview.Category = item.CategoryCreditCard
		item.WriteCard(&vaultItem, card, vaultItem.Details.Fields)
	}
	if category == onePasswordAPICredential {
		// Some exports keep the key ID and secret in login fields instead of the section; those fill the credential. A
		// different value stays, as a custom field.
		foldOnePasswordLoginValue(&credential.KeyID, &vaultItem.Overview.Username)
		foldOnePasswordLoginValue(&credential.Secret, &vaultItem.Details.Password)
		vaultItem.Overview.Category = item.CategoryAPICredential
		item.WriteAPICredential(&vaultItem, credential, vaultItem.Details.Fields)
	}
	if category == onePasswordSoftwareLicense {
		license.Product = strings.TrimSpace(entry.Overview.Title)
		// A license saved with a website but no download link keeps the website as its download URL; any other website
		// becomes a custom field.
		if strings.TrimSpace(license.DownloadURL) == "" && len(vaultItem.Overview.URLs) > 0 {
			license.DownloadURL, vaultItem.Overview.URLs = vaultItem.Overview.URLs[0], vaultItem.Overview.URLs[1:]
		}
		vaultItem.Overview.Category = item.CategorySoftwareLicense
		item.WriteSoftwareLicense(&vaultItem, license, vaultItem.Details.Fields)
	}
	if category == onePasswordServer {
		// 1Password repeats the server's URL as the item's website; the host keeps it.
		websites := slices.DeleteFunc(vaultItem.Overview.URLs, func(link string) bool {
			return serverURL != "" && strings.TrimSpace(link) == strings.TrimSpace(serverURL)
		})
		vaultItem.Overview.URLs = append(promoteOnePasswordAdminConsole(&vaultItem, consoleFieldIndexes), websites...)
		vaultItem.Overview.Category = item.CategoryServer
		item.WriteServer(&vaultItem, server, vaultItem.Details.Fields)
	}
	if isIdentity {
		vaultItem.Overview.Category = item.CategoryIdentity
		otherFields := vaultItem.Details.Fields
		// A Passport, Driver License or Social Security Number has one full name; it fills the name unless the item has name parts already.
		if fullName.Value != "" {
			if identity.FirstName == "" && identity.LastName == "" {
				identity.FirstName, identity.LastName = splitFullName(fullName.Value)
			} else {
				otherFields = append([]item.Field{fullName}, otherFields...)
			}
		}
		item.WriteIdentity(&vaultItem, identity, otherFields)
	}
	return vaultItem, ""
}

// promoteOnePasswordAdminConsole moves a server's admin console URL out of its custom fields and returns it as the server's
// website, along with console credentials that repeat the server's own. A console with a username or password of its own
// stays custom fields: as the website, the extension would fill the server's password on the console and a browser save
// there would overwrite it.
func promoteOnePasswordAdminConsole(vaultItem *item.Item, consoleFieldIndexes map[string]int) []string {
	fields := vaultItem.Details.Fields
	urlIndex, hasURL := consoleFieldIndexes["admin_console_url"]
	if !hasURL || strings.TrimSpace(fields[urlIndex].Value) == "" {
		return nil
	}
	ownValuesByID := map[string]string{"admin_console_username": vaultItem.Overview.Username, "admin_console_password": vaultItem.Details.Password}
	droppedIndexes := map[int]bool{urlIndex: true}
	for fieldID, ownValue := range ownValuesByID {
		index, found := consoleFieldIndexes[fieldID]
		if !found {
			continue
		}
		if value := fields[index].Value; value != "" && value != ownValue {
			return nil
		}
		droppedIndexes[index] = true
	}
	consoleURL := fields[urlIndex].Value
	keptFields := []item.Field{}
	for index, field := range fields {
		if !droppedIndexes[index] {
			keptFields = append(keptFields, field)
		}
	}
	vaultItem.Details.Fields = keptFields
	return []string{consoleURL}
}

// foldOnePasswordLoginValue moves a login value into an empty credential value, and drops it when the credential has the
// same value already.
func foldOnePasswordLoginValue(credentialValue, loginValue *string) {
	switch {
	case strings.TrimSpace(*credentialValue) == "":
		*credentialValue, *loginValue = *loginValue, ""
	case *loginValue == *credentialValue:
		*loginValue = ""
	}
}

// isOnePasswordIdentityCategory tells whether items of a 1Password category become identities.
func isOnePasswordIdentityCategory(category string) bool {
	return category == onePasswordIdentity || category == onePasswordPassport || category == onePasswordLicense || category == onePasswordSSN
}

// splitFullName takes the last word of a full name as the last name and the rest as the first name.
func splitFullName(fullName string) (firstName, lastName string) {
	words := strings.Fields(fullName)
	if len(words) < 2 {
		return strings.Join(words, " "), ""
	}
	return strings.Join(words[:len(words)-1], " "), words[len(words)-1]
}

func isOnePasswordOneTimePassword(sectionField onePasswordField, value string) bool {
	_, isTOTP := sectionField.Value["totp"]
	return isTOTP || strings.HasPrefix(sectionField.ID, "TOTP_") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "otpauth://")
}

// promoteOnePasswordField moves a credential category's username, password and URL fields into the item itself.
func promoteOnePasswordField(vaultItem *item.Item, fieldID, value string) bool {
	switch {
	case fieldID == "username" && vaultItem.Overview.Username == "":
		vaultItem.Overview.Username = value
	case fieldID == "password" && vaultItem.Details.Password == "":
		vaultItem.Details.Password = value
	case fieldID == "url" && len(vaultItem.Overview.URLs) == 0:
		vaultItem.Overview.URLs = []string{value}
	default:
		return false
	}
	return true
}

func convertOnePasswordSSHKey(entry onePasswordItem) (item.Item, string) {
	for _, section := range entry.Details.Sections {
		for _, sectionField := range section.Fields {
			rawKey, found := sectionField.Value["sshKey"]
			if !found {
				continue
			}
			var sshKey struct {
				PrivateKey string `json:"privateKey"`
				Metadata   struct {
					PrivateKey string `json:"privateKey"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(rawKey, &sshKey); err != nil {
				return item.Item{}, "the SSH private key could not be read"
			}
			return buildSSHKeyItem(entry.Overview.Title, getFirstNonEmpty(sshKey.PrivateKey, sshKey.Metadata.PrivateKey), entry.Details.NotesPlain)
		}
	}
	return item.Item{}, "the SSH key has no private key"
}

// convertOnePasswordFieldValue reads the single typed value 1Password stores per field.
func convertOnePasswordFieldValue(valuesByType map[string]json.RawMessage) (string, bool) {
	for valueType, rawValue := range valuesByType {
		isConcealed := valueType == "concealed" || valueType == "totp" || valueType == "creditCardNumber"
		var text string
		if json.Unmarshal(rawValue, &text) == nil {
			return text, isConcealed
		}
		switch valueType {
		case "date":
			var seconds int64
			if json.Unmarshal(rawValue, &seconds) == nil && seconds != 0 {
				return time.Unix(seconds, 0).UTC().Format("2006-01-02"), false
			}
		case "monthYear":
			var monthYear int
			if json.Unmarshal(rawValue, &monthYear) == nil && monthYear > 0 {
				return fmt.Sprintf("%02d/%04d", monthYear%100, monthYear/100), false
			}
		case "email":
			var email struct {
				Address string `json:"email_address"`
			}
			if json.Unmarshal(rawValue, &email) == nil {
				return email.Address, false
			}
		case "address":
			var address struct {
				Street  string `json:"street"`
				City    string `json:"city"`
				State   string `json:"state"`
				Zip     string `json:"zip"`
				Country string `json:"country"`
			}
			if json.Unmarshal(rawValue, &address) == nil {
				return joinNonEmpties([]string{address.Street, address.City, address.State, address.Zip, address.Country}, ", "), false
			}
		}
		var number json.Number
		if json.Unmarshal(rawValue, &number) == nil {
			return number.String(), isConcealed
		}
		var flag bool
		if json.Unmarshal(rawValue, &flag) == nil {
			return strconv.FormatBool(flag), false
		}
	}
	return "", false
}

func getFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinNonEmpties(values []string, separator string) string {
	nonEmpties := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			nonEmpties = append(nonEmpties, value)
		}
	}
	return strings.Join(nonEmpties, separator)
}
