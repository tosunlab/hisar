package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"golang.org/x/crypto/ssh"
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

// Bitwarden's URI match rules that Hisar cannot match by: a regular expression is no website, and "never" fills nowhere.
// Both are kept as fields under these names instead of websites.
const (
	bitwardenURIMatchRegularExpression = 4
	bitwardenURIMatchNever             = 5
)

var bitwardenURIFieldNames = map[int]string{bitwardenURIMatchRegularExpression: "Website pattern", bitwardenURIMatchNever: "Website (never filled)"}

// bitwardenNarrowerURIMatches says where Bitwarden filled a login for the rules that match less than Hisar, which offers
// it on every page of the website's site, as Bitwarden's default base domain rule does.
var bitwardenNarrowerURIMatches = map[int]string{1: "on that host", 2: "on addresses that start with it", 3: "on that exact address"}

// bitwardenRepromptPassword is the reprompt of an item for which Bitwarden asks for the master password again before it
// shows or fills it; Hisar has no such per-item setting, so each one is reported for the user to review.
const (
	bitwardenRepromptPassword = 1
	bitwardenRepromptReason   = "Bitwarden asked for the master password again before showing or filling it; Hisar shows and fills it while unlocked"
)

type bitwardenExport struct {
	Encrypted   bool `json:"encrypted"`
	Collections []struct {
		ID             string `json:"id"`
		OrganizationID string `json:"organizationId"`
		Name           string `json:"name"`
	} `json:"collections"`
	Folders []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"folders"`
	Items []bitwardenItem `json:"items"`
}

type bitwardenItem struct {
	OrganizationID string   `json:"organizationId"`
	CollectionIDs  []string `json:"collectionIds"`
	Type           int      `json:"type"`
	Name           string   `json:"name"`
	Notes          string   `json:"notes"`
	FolderID       string   `json:"folderId"`
	Favorite       bool     `json:"favorite"`
	Reprompt       int      `json:"reprompt"`
	Fields         []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Type  int    `json:"type"`
	} `json:"fields"`
	PasswordHistory []struct {
		LastUsedDate string `json:"lastUsedDate"`
		Password     string `json:"password"`
	} `json:"passwordHistory"`
	Login *struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
		URIs     []struct {
			URI   string `json:"uri"`
			Match *int   `json:"match"`
		} `json:"uris"`
		FIDO2Credentials []struct {
			RelyingPartyID string `json:"rpId"`
		} `json:"fido2Credentials"`
	} `json:"login"`
	SSHKey *struct {
		PrivateKey string `json:"privateKey"`
		PublicKey  string `json:"publicKey"`
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
	groupIndexes := map[string]int{}
	collectionNames := map[string]string{}
	collectionOrganizations := map[string]string{}
	for _, collection := range export.Collections {
		collectionNames[collection.ID] = collection.Name
		collectionOrganizations[collection.ID] = collection.OrganizationID
	}
	for _, entry := range export.Items {
		vaultItem, reason := convertBitwardenItem(entry, &result.Skipped)
		if reason != "" {
			result.Skipped = append(result.Skipped, SkippedEntry{Title: describeEntry(entry.Name), Reason: reason})
			continue
		}
		if folderName := folderNames[entry.FolderID]; folderName != "" {
			vaultItem.Overview.Tags = []string{folderName}
		}
		vaultItem.IsFavorite = entry.Favorite
		for _, previous := range entry.PasswordHistory {
			replacedAt, _ := time.Parse(time.RFC3339, previous.LastUsedDate)
			addPreviousPassword(&vaultItem, previous.Password, replacedAt)
		}
		if finishItem(&vaultItem) {
			collectionIDs := entry.CollectionIDs
			if collectionIDs == nil {
				collectionIDs = []string{}
			}
			slices.Sort(collectionIDs)
			collectionIDs = slices.Compact(collectionIDs)
			organizationID := entry.OrganizationID
			names := []string{}
			for _, id := range collectionIDs {
				name := collectionNames[id]
				if name == "" {
					name = id
				}
				names = append(names, name)
				if organizationID == "" {
					organizationID = collectionOrganizations[id]
				}
			}
			keyBytes, _ := json.Marshal([]any{organizationID, collectionIDs})
			key := string(keyBytes)
			groupIndex, found := groupIndexes[key]
			if !found {
				name := "Bitwarden Personal"
				isPrivate := organizationID == "" && len(collectionIDs) == 0
				if !isPrivate {
					name = "Bitwarden organization " + organizationID
					if len(names) > 0 {
						name += " / " + strings.Join(names, ", ")
					}
				}
				groupIndex = len(result.Groups)
				groupIndexes[key] = groupIndex
				result.Groups = append(result.Groups, SourceGroup{ID: "bitwarden/" + key, Name: name, IsPrivate: isPrivate})
			}
			result.Groups[groupIndex].Items = append(result.Groups[groupIndex].Items, vaultItem)
			result.Items = append(result.Items, vaultItem)
			for _, reason := range listBitwardenOmissions(entry) {
				result.Skipped = append(result.Skipped, SkippedEntry{Title: vaultItem.Overview.Title, Reason: reason})
			}
		}
	}
	return result, nil
}

func convertBitwardenItem(entry bitwardenItem, skipped *[]SkippedEntry) (item.Item, string) {
	switch entry.Type {
	case bitwardenLogin, bitwardenSecureNote, bitwardenCard, bitwardenIdentity:
	case bitwardenSSHKey:
		if entry.SSHKey == nil {
			return item.Item{}, "the SSH key has no private key"
		}
		keyItem, reason := buildSSHKeyItem(entry.Name, entry.SSHKey.PrivateKey, entry.Notes, skipped)
		if reason == "" {
			_, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(entry.SSHKey.PublicKey))
			if err == nil {
				reportSSHCommentOmission(skipped, entry.Name, comment)
			}
		}
		keyItem.Details.Fields = convertBitwardenFields(entry)
		return keyItem, reason
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
			if uri.Match != nil && bitwardenURIFieldNames[*uri.Match] != "" {
				vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: bitwardenURIFieldNames[*uri.Match], Value: uri.URI})
				continue
			}
			vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, uri.URI)
		}
		addOneTimePassword(&vaultItem, "One-time password", entry.Login.TOTP)
	}
	vaultItem.Details.Fields = append(vaultItem.Details.Fields, convertBitwardenFields(entry)...)
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
				Address: item.Address{
					Street: joinNonEmpty("\n", values.Address1, values.Address2, values.Address3),
					City:   values.City, Region: values.State, PostalCode: values.PostalCode, Country: values.Country,
				},
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

// convertBitwardenFields returns an item's custom fields, hidden ones concealed; a linked field only names another
// value of the item, so it is left out.
func convertBitwardenFields(entry bitwardenItem) []item.Field {
	fields := []item.Field{}
	for _, field := range entry.Fields {
		if field.Type != bitwardenFieldLinked {
			fields = append(fields, item.Field{Name: field.Name, Value: field.Value, Concealed: field.Type == bitwardenFieldHidden})
		}
	}
	return fields
}

// listBitwardenOmissions gives a reason for each thing of an imported item that Hisar did not keep as Bitwarden had it.
func listBitwardenOmissions(entry bitwardenItem) []string {
	reasons := []string{}
	if entry.Reprompt == bitwardenRepromptPassword {
		reasons = append(reasons, bitwardenRepromptReason)
	}
	if entry.Login != nil {
		for _, uri := range entry.Login.URIs {
			if uri.Match == nil || strings.TrimSpace(uri.URI) == "" {
				continue
			}
			link := describeEntry(uri.URI)
			switch {
			case *uri.Match == bitwardenURIMatchRegularExpression:
				reasons = append(reasons, fmt.Sprintf("its website pattern %s is a regular expression, which Hisar does not match; it was kept as a field", link))
			case *uri.Match == bitwardenURIMatchNever:
				reasons = append(reasons, fmt.Sprintf("its website %s was never filled in Bitwarden; it was kept as a field, so Hisar does not offer the login there", link))
			case bitwardenNarrowerURIMatches[*uri.Match] != "":
				reasons = append(reasons, fmt.Sprintf("its website %s was filled only %s in Bitwarden; Hisar offers the login on every page of its site", link, bitwardenNarrowerURIMatches[*uri.Match]))
			}
		}
		for _, credential := range entry.Login.FIDO2Credentials {
			reasons = append(reasons, fmt.Sprintf("its passkey for %s was not imported; keep using it in Bitwarden", describeEntry(credential.RelyingPartyID)))
		}
	}
	return reasons
}

// joinAddress puts the non-empty lines of an address one per line.
func joinNonEmpty(separator string, parts ...string) string {
	nonEmpty := []string{}
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, separator)
}
