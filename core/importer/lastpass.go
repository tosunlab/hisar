package importer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
)

// LastPass exports a typed secure note (credit card, address, server…) as an http://sn row whose extra column is
// "NoteType:<type>" followed by "<name>:<value>" lines, with "Notes:" and the note's own text last.
const (
	lastPassNoteTypePrefix = "NoteType:"
	lastPassNotesName      = "Notes"
)

type lastPassValue struct {
	name   string
	value  string
	isUsed bool
}

// lastPassNote is a typed note's values in the order of the export.
type lastPassNote struct {
	noteType string
	values   []*lastPassValue
	notes    string
}

// isLastPassTypedNote reports whether a secure note's text is a typed note.
func isLastPassTypedNote(text string) bool {
	return strings.HasPrefix(strings.TrimLeft(text, "\ufeff \r\n"), lastPassNoteTypePrefix)
}

// parseLastPassNote reads a typed note's text. A line that does not start with a name and a colon continues the value
// before it, so values with line breaks, such as a private key, stay whole; everything after "Notes:" is the notes.
func parseLastPassNote(text string) lastPassNote {
	note := lastPassNote{}
	lines := strings.Split(strings.ReplaceAll(strings.TrimLeft(text, "\ufeff \r\n"), "\r\n", "\n"), "\n")
	var current *lastPassValue
	for index, line := range lines {
		name, value, isNamed := cutLastPassLine(line)
		switch {
		case isNamed && name == lastPassNotesName:
			note.notes = strings.Join(append([]string{value}, lines[index+1:]...), "\n")
			return note
		case isNamed && name == "NoteType":
			note.noteType = strings.TrimSpace(value)
			current = nil
		case isNamed && name == "Language":
			current = nil
		case isNamed:
			current = &lastPassValue{name: name, value: value}
			note.values = append(note.values, current)
		case current != nil:
			current.value += "\n" + line
		}
	}
	return note
}

// cutLastPassLine splits "Name:value". LastPass writes no space after the colon, so a line such as "Proc-Type: 4,ENCRYPTED"
// in a private key is part of the value before it.
func cutLastPassLine(line string) (name, value string, isNamed bool) {
	name, value, isFound := strings.Cut(line, ":")
	if !isFound || name == "" || len(name) > 64 || strings.TrimSpace(name) != name || strings.HasPrefix(value, " ") || strings.Contains(name, "-----") {
		return "", "", false
	}
	return name, value, true
}

// take returns the first of names with a value and marks it used.
func (note *lastPassNote) take(names ...string) string {
	for _, name := range names {
		for _, value := range note.values {
			if !value.isUsed && strings.EqualFold(value.name, name) && strings.TrimSpace(value.value) != "" {
				value.isUsed = true
				return strings.TrimSpace(value.value)
			}
		}
	}
	return ""
}

// has reports whether any of names has a value, without using it.
func (note *lastPassNote) has(names ...string) bool {
	for _, name := range names {
		for _, value := range note.values {
			if !value.isUsed && strings.EqualFold(value.name, name) && strings.TrimSpace(value.value) != "" {
				return true
			}
		}
	}
	return false
}

// remainingFields returns the values no mapping used as custom fields, secrets concealed.
func (note *lastPassNote) remainingFields() []item.Field {
	fields := []item.Field{}
	for _, value := range note.values {
		if value.isUsed || strings.TrimSpace(value.value) == "" {
			continue
		}
		fields = append(fields, item.Field{Name: value.name, Value: formatLastPassValue(value.value), Concealed: isLastPassSecretName(value.name)})
	}
	return fields
}

// formatLastPassValue turns LastPass's phone JSON ({"num":"5551234","ext":"12","cc3l":"USA"}) and dates
// ("January,2024") into text; other values are trimmed.
func formatLastPassValue(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{") {
		return convertLastPassDate(value)
	}
	var phone struct {
		Number    string `json:"num"`
		Extension string `json:"ext"`
	}
	if err := json.Unmarshal([]byte(value), &phone); err != nil || phone.Number == "" {
		return value
	}
	if phone.Extension != "" {
		return phone.Number + " ext. " + phone.Extension
	}
	return phone.Number
}

// isLastPassSecretName reports whether a typed note's value is a secret to conceal.
func isLastPassSecretName(name string) bool {
	name = strings.ToLower(name)
	if strings.Contains(name, "phone") || strings.HasPrefix(name, "number of") {
		return false
	}
	for _, secret := range []string{"password", "passphrase", "pin", "security code", "cvv", "ssn", "social security", "secret", "private key", "license key", "iban"} {
		if strings.Contains(name, secret) {
			return true
		}
	}
	return name == "number" || strings.HasSuffix(name, " number")
}

// convertLastPassDate writes LastPass's "January,15,1990" as 1990-01-15 and "January,2027" as 01/2027; anything else
// stays as it is.
func convertLastPassDate(text string) string {
	parts := strings.Split(strings.TrimSpace(text), ",")
	if len(parts) < 2 {
		return strings.TrimSpace(text)
	}
	month, err := time.Parse("January", strings.TrimSpace(parts[0]))
	if err != nil {
		return strings.TrimSpace(text)
	}
	year, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil || year < 1000 || year > 9999 || len(parts) > 3 {
		return strings.TrimSpace(text)
	}
	if len(parts) == 2 {
		return fmt.Sprintf("%02d/%d", int(month.Month()), year)
	}
	day, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || day < 1 || day > 31 {
		return strings.TrimSpace(text)
	}
	return fmt.Sprintf("%d-%02d-%02d", year, int(month.Month()), day)
}

// convertLastPassNote turns a typed note into the matching item type; a type Hisar has no item for becomes a secure note
// whose values are custom fields. The values a type does not use are kept as custom fields.
func convertLastPassNote(title, text string) (item.Item, string) {
	note := parseLastPassNote(text)
	vaultItem := item.Item{Overview: item.Overview{Title: title}, Details: item.Details{Notes: note.notes}}
	switch strings.ToLower(note.noteType) {
	case "credit card":
		vaultItem.Overview.Category = item.CategoryCreditCard
		card := item.Card{
			Cardholder: note.take("Name on Card"), Type: note.take("Type"), Number: note.take("Number"),
			SecurityCode: note.take("Security Code"), Expiry: convertLastPassDate(note.take("Expiration Date")),
		}
		item.WriteCard(&vaultItem, card, note.remainingFields())
	case "address":
		address := item.Address{
			Street: joinNonEmpty("\n", note.take("Address 1"), note.take("Address 2"), note.take("Address 3")),
			City:   note.take("City / Town", "City"), Region: note.take("State", "County"), PostalCode: note.take("Zip / Postal Code", "Zip"),
			Country: note.take("Country"),
		}
		if !note.has("First Name", "Middle Name", "Last Name", "Email Address", "Phone", "Birthday") {
			vaultItem.Overview.Category = item.CategoryAddress
			item.WriteAddress(&vaultItem, address, note.remainingFields())
			break
		}
		vaultItem.Overview.Category = item.CategoryIdentity
		identity := item.Identity{
			FirstName: note.take("First Name"), MiddleName: note.take("Middle Name"), LastName: note.take("Last Name"),
			BirthDate: convertLastPassDate(note.take("Birthday")), Email: note.take("Email Address"),
			Phone: formatLastPassValue(note.take("Phone")), Address: address,
		}
		item.WriteIdentity(&vaultItem, identity, note.remainingFields())
	case "server":
		vaultItem.Overview.Category = item.CategoryServer
		vaultItem.Overview.Username, vaultItem.Details.Password = note.take("Username"), note.take("Password")
		item.WriteServer(&vaultItem, item.Server{Host: note.take("Hostname")}, note.remainingFields())
	case "database":
		vaultItem.Overview.Category = item.CategoryDatabase
		vaultItem.Overview.Username, vaultItem.Details.Password = note.take("Username"), note.take("Password")
		database := item.Database{Engine: note.take("Type"), Host: note.take("Hostname"), Port: note.take("Port"), Name: note.take("Database", "SID")}
		item.WriteDatabase(&vaultItem, database, note.remainingFields())
	case "software license":
		vaultItem.Overview.Category = item.CategorySoftwareLicense
		license := item.SoftwareLicense{
			Product: title, Version: note.take("Version"), LicenseKey: note.take("License Key"), LicensedTo: note.take("Licensee"),
			Email: note.take("Support Email"), PurchaseDate: convertLastPassDate(note.take("Purchase Date")), DownloadURL: note.take("Website"),
		}
		item.WriteSoftwareLicense(&vaultItem, license, note.remainingFields())
	case "ssh key":
		if keyItem, reason := convertLastPassSSHKey(title, &note); reason == "" {
			return keyItem, ""
		}
		fallthrough
	default:
		vaultItem.Overview.Category = item.CategorySecureNote
		fields := []item.Field{{Name: "Type", Value: note.noteType}}
		vaultItem.Details.Fields = append(fields, note.remainingFields()...)
	}
	return vaultItem, ""
}

// convertLastPassSSHKey stores the key as an ssh-key item, with the host and date in its notes; the public key, format
// and bit strength come from the key itself. It leaves the note's values unused when the key can't be stored.
func convertLastPassSSHKey(title string, note *lastPassNote) (item.Item, string) {
	privateKey, passphrase := "", ""
	for _, value := range note.values {
		switch strings.ToLower(value.name) {
		case "private key":
			privateKey = value.value
		case "passphrase":
			passphrase = strings.TrimSpace(value.value)
		}
	}
	keyItem, reason := buildProtectedSSHKeyItem(title, privateKey, passphrase, "")
	if reason != "" {
		return item.Item{}, reason
	}
	lines := []string{}
	for _, value := range note.values {
		switch strings.ToLower(value.name) {
		case "private key", "passphrase", "public key", "format", "bit strength":
			continue
		}
		if text := formatLastPassValue(value.value); text != "" {
			lines = append(lines, value.name+": "+text)
		}
	}
	keyItem.Details.Notes = strings.TrimSpace(joinNonEmpty("\n\n", strings.Join(lines, "\n"), note.notes))
	return keyItem, ""
}
