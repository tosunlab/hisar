package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tryhisar/hisar/core/item"
)

const (
	bitwardenLogin      = 1
	bitwardenSecureNote = 2
	bitwardenCard       = 3
	bitwardenIdentity   = 4
	bitwardenSSHKey     = 5
)

const (
	bitwardenFieldHidden = 1
	bitwardenFieldLinked = 3
)

type bitwardenExport struct {
	Encrypted bool `json:"encrypted"`
	Folders   []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"folders"`
	Items []bitwardenItem `json:"items"`
}

type bitwardenItem struct {
	Type     int    `json:"type"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	FolderID string `json:"folderId"`
	Favorite bool   `json:"favorite"`
	Fields   []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Type  int    `json:"type"`
	} `json:"fields"`
	Login *struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
		URIs     []struct {
			URI string `json:"uri"`
		} `json:"uris"`
	} `json:"login"`
	SSHKey *struct {
		PrivateKey string `json:"privateKey"`
	} `json:"sshKey"`
	Card *struct {
		CardholderName string `json:"cardholderName"`
		Brand          string `json:"brand"`
		Number         string `json:"number"`
		ExpMonth       string `json:"expMonth"`
		ExpYear        string `json:"expYear"`
		Code           string `json:"code"`
	} `json:"card"`
	Identity *bitwardenIdentityValues `json:"identity"`
}

type bitwardenIdentityValues struct {
	Title          string `json:"title"`
	FirstName      string `json:"firstName"`
	MiddleName     string `json:"middleName"`
	LastName       string `json:"lastName"`
	Address1       string `json:"address1"`
	Address2       string `json:"address2"`
	Address3       string `json:"address3"`
	City           string `json:"city"`
	State          string `json:"state"`
	PostalCode     string `json:"postalCode"`
	Country        string `json:"country"`
	Company        string `json:"company"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	SSN            string `json:"ssn"`
	Username       string `json:"username"`
	PassportNumber string `json:"passportNumber"`
	LicenseNumber  string `json:"licenseNumber"`
}

func parseBitwardenJSON(content []byte) (*Result, error) {
	var export bitwardenExport
	if err := json.Unmarshal(content, &export); err != nil {
		var syntaxError *json.SyntaxError
		if errors.As(err, &syntaxError) {
			return nil, fmt.Errorf("the Bitwarden export is not valid JSON near byte %d", syntaxError.Offset)
		}
		return nil, errors.New("the Bitwarden export could not be read; export it from Bitwarden again as .json")
	}
	if export.Encrypted {
		return nil, ErrEncryptedExport
	}
	folderNames := map[string]string{}
	for _, folder := range export.Folders {
		folderNames[folder.ID] = folder.Name
	}
	result := &Result{Items: []item.Item{}, Skipped: []SkippedEntry{}}
	for _, entry := range export.Items {
		vaultItem, reason := convertBitwardenItem(entry)
		if reason != "" {
			result.Skipped = append(result.Skipped, SkippedEntry{Title: describeEntry(entry.Name), Reason: reason})
			continue
		}
		if folderName := folderNames[entry.FolderID]; folderName != "" {
			vaultItem.Overview.Tags = []string{folderName}
		}
		vaultItem.IsFavorite = entry.Favorite
		if finishItem(&vaultItem) {
			result.Items = append(result.Items, vaultItem)
		}
	}
	return result, nil
}

func convertBitwardenItem(entry bitwardenItem) (item.Item, string) {
	switch entry.Type {
	case bitwardenLogin, bitwardenSecureNote, bitwardenCard, bitwardenIdentity:
	case bitwardenSSHKey:
		if entry.SSHKey == nil {
			return item.Item{}, "the SSH key has no private key"
		}
		return buildSSHKeyItem(entry.Name, entry.SSHKey.PrivateKey, entry.Notes)
	default:
		return item.Item{}, "this kind of Bitwarden item is not supported in Hisar yet"
	}
	vaultItem := item.Item{Overview: item.Overview{Title: entry.Name}, Details: item.Details{Notes: entry.Notes}}
	if entry.Type == bitwardenSecureNote {
		vaultItem.Overview.Category = item.CategorySecureNote
	}
	if entry.Login != nil {
		vaultItem.Overview.Username = entry.Login.Username
		vaultItem.Details.Password = entry.Login.Password
		for _, uri := range entry.Login.URIs {
			vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, uri.URI)
		}
		addOneTimePassword(&vaultItem, "One-time password", entry.Login.TOTP)
	}
	for _, field := range entry.Fields {
		if field.Type == bitwardenFieldLinked {
			continue
		}
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: field.Name, Value: field.Value, Concealed: field.Type == bitwardenFieldHidden})
	}
	if entry.Type == bitwardenCard {
		vaultItem.Overview.Category = item.CategoryCreditCard
		if entry.Card != nil {
			card := item.Card{Cardholder: entry.Card.CardholderName, Type: entry.Card.Brand, Number: entry.Card.Number, SecurityCode: entry.Card.Code}
			if entry.Card.ExpMonth != "" && entry.Card.ExpYear != "" {
				card.Expiry = entry.Card.ExpMonth + "/" + entry.Card.ExpYear
			}
			item.WriteCard(&vaultItem, card, vaultItem.Details.Fields)
		}
	}
	if entry.Type == bitwardenIdentity {
		vaultItem.Overview.Category = item.CategoryIdentity
		if values := entry.Identity; values != nil {
			identity := item.Identity{
				FirstName: values.FirstName, MiddleName: values.MiddleName, LastName: values.LastName, Email: values.Email, Phone: values.Phone,
				Address:        joinAddress(values.Address1, values.Address2, values.Address3, joinNonEmpty(", ", values.City, values.State, values.PostalCode), values.Country),
				PassportNumber: values.PassportNumber, DriversLicenseNumber: values.LicenseNumber, NationalIDNumber: values.SSN,
			}
			// The username moves into a custom field with the rest of the login data when the import finishes.
			vaultItem.Overview.Username = values.Username
			otherFields := append([]item.Field{{Name: "Title", Value: values.Title}, {Name: "Company", Value: values.Company}}, vaultItem.Details.Fields...)
			item.WriteIdentity(&vaultItem, identity, otherFields)
		}
	}
	return vaultItem, ""
}

// joinAddress puts the non-empty lines of an address one per line.
func joinAddress(lines ...string) string { return joinNonEmpty("\n", lines...) }

func joinNonEmpty(separator string, parts ...string) string {
	nonEmpty := []string{}
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, separator)
}
