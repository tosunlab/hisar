package importer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
)

const maxOnePasswordDataSize = 64 << 20

const (
	onePasswordLogin      = "001"
	onePasswordSecureNote = "003"
	onePasswordPassword   = "005"
	onePasswordSSHKey     = "114"
)

var onePasswordCategoryNames = map[string]string{
	"001": "Login", "002": "Credit Card", "003": "Secure Note", "004": "Identity", "005": "Password", "006": "Document",
	"100": "Software License", "101": "Bank Account", "102": "Database", "103": "Driver License", "104": "Outdoor License",
	"105": "Membership", "106": "Passport", "107": "Reward Program", "108": "Social Security Number", "109": "Wireless Router",
	"110": "Server", "111": "Email Account", "112": "API Credential", "113": "Medical Record", "114": "SSH Key", "115": "Crypto Wallet",
}

// onePasswordCredentialCategories become logins with their fields; other categories hold personal records Hisar has no type for yet.
var onePasswordCredentialCategories = map[string]bool{"100": true, "102": true, "109": true, "110": true, "111": true, "112": true}

type onePasswordExport struct {
	Accounts []struct {
		Vaults []struct {
			Items []onePasswordItem `json:"items"`
		} `json:"vaults"`
	} `json:"accounts"`
}

type onePasswordItem struct {
	State        string `json:"state"`
	CategoryUUID string `json:"categoryUuid"`
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
				if finishItem(&vaultItem) {
					result.Items = append(result.Items, vaultItem)
				}
			}
		}
	}
	return result, nil
}

func convertOnePasswordItem(entry onePasswordItem) (item.Item, string) {
	if entry.State == "archived" {
		return item.Item{}, "archived in 1Password"
	}
	category := entry.CategoryUUID
	switch {
	case category == onePasswordSSHKey:
		return convertOnePasswordSSHKey(entry)
	case category == onePasswordLogin, category == onePasswordPassword, category == onePasswordSecureNote, onePasswordCredentialCategories[category]:
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
			value, isConcealed := convertOnePasswordFieldValue(sectionField.Value)
			if onePasswordCredentialCategories[category] && promoteOnePasswordField(&vaultItem, sectionField.ID, value) {
				continue
			}
			name := sectionField.Title
			if name == "" {
				name = getFirstNonEmpty(section.Title, sectionField.ID)
			}
			if isOnePasswordOneTimePassword(sectionField, value) {
				addOneTimePassword(&vaultItem, name, value)
				continue
			}
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: name, Value: value, Concealed: isConcealed})
		}
	}
	return vaultItem, ""
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
