package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tryhisar/hisar/core/item"
)

const (
	columnTitle    = "title"
	columnUsername = "username"
	columnPassword = "password"
	columnURL      = "url"
	columnNotes    = "notes"
	columnOTP      = "otp"
	columnTags     = "tags"
	columnFields   = "fields"
	columnType     = "type"
	columnArchived = "archived"
	columnFavorite = "favorite"
)

var columnAliases = map[string]string{
	"title": columnTitle, "name": columnTitle, "account": columnTitle, "item name": columnTitle, "entry": columnTitle,
	"username": columnUsername, "user name": columnUsername, "login_username": columnUsername, "login": columnUsername, "login name": columnUsername,
	"user": columnUsername, "email": columnUsername, "e-mail": columnUsername, "email address": columnUsername,
	"password": columnPassword, "login_password": columnPassword, "pass": columnPassword,
	"url": columnURL, "login_uri": columnURL, "website": columnURL, "web site": columnURL, "uri": columnURL, "web address": columnURL,
	"hostname": columnURL,
	"notes":    columnNotes, "note": columnNotes, "comments": columnNotes, "comment": columnNotes, "extra": columnNotes,
	"otpauth": columnOTP, "totp": columnOTP, "login_totp": columnOTP, "otp": columnOTP, "one-time password": columnOTP,
	"tags": columnTags, "folder": columnTags, "grouping": columnTags,
	"fields":   columnFields,
	"type":     columnType,
	"archived": columnArchived,
	"favorite": columnFavorite, "fav": columnFavorite,
}

// ignoredColumns holds export metadata that is not worth keeping as a field.
var ignoredColumns = map[string]bool{
	"reprompt": true, "httprealm": true, "formactionorigin": true, "guid": true,
	"timecreated": true, "timelastused": true, "timepasswordchanged": true, "created": true, "modified": true, "last modified": true,
}

type csvLayout struct {
	indexesByColumn map[string]int
	extraIndexes    []int
	columnNames     []string
}

func (layout *csvLayout) getValue(values []string, column string) string {
	index, found := layout.indexesByColumn[column]
	if !found || index >= len(values) {
		return ""
	}
	return values[index]
}

func normalizeColumnName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.Trim(name, "\ufeff \t\"")), " "))
}

// buildCSVLayout assigns each role to an exactly named column first, then to the first alias.
func buildCSVLayout(columnNames []string) (*csvLayout, error) {
	layout := &csvLayout{indexesByColumn: map[string]int{}, columnNames: columnNames}
	isAssigned := make([]bool, len(columnNames))
	for index, name := range columnNames {
		normalized := normalizeColumnName(name)
		if column, found := columnAliases[normalized]; found && column == normalized {
			if _, taken := layout.indexesByColumn[column]; !taken {
				layout.indexesByColumn[column] = index
				isAssigned[index] = true
			}
		}
	}
	for index, name := range columnNames {
		if isAssigned[index] {
			continue
		}
		normalized := normalizeColumnName(name)
		if column, found := columnAliases[normalized]; found {
			if _, taken := layout.indexesByColumn[column]; !taken {
				layout.indexesByColumn[column] = index
				continue
			}
		}
		if normalized != "" && !ignoredColumns[normalized] {
			layout.extraIndexes = append(layout.extraIndexes, index)
		}
	}
	_, hasPassword := layout.indexesByColumn[columnPassword]
	_, hasUsername := layout.indexesByColumn[columnUsername]
	_, hasURL := layout.indexesByColumn[columnURL]
	_, hasTitle := layout.indexesByColumn[columnTitle]
	if !hasPassword && !(hasUsername && (hasURL || hasTitle)) {
		return nil, errors.New("the first row of the CSV file must name the columns, for example title, username, password, url and notes")
	}
	return layout, nil
}

func createCSVReader(content []byte) *csv.Reader {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	firstLine, _, _ := bytes.Cut(content, []byte("\n"))
	if bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		reader.Comma = ';'
	} else if bytes.Count(firstLine, []byte("\t")) > bytes.Count(firstLine, []byte(",")) {
		reader.Comma = '\t'
	}
	return reader
}

// convertCSVError keeps the position of a syntax error and drops everything else, which could contain values.
func convertCSVError(err error) error {
	var parseError *csv.ParseError
	if errors.As(err, &parseError) {
		return fmt.Errorf("the CSV file is not valid near line %d; check that values with commas, quotes or line breaks are in double quotes", parseError.StartLine)
	}
	return errors.New("the CSV file could not be read")
}

func detectCSVFormat(content []byte) (string, error) {
	columnNames, err := createCSVReader(content).Read()
	if errors.Is(err, io.EOF) {
		return "", ErrEmptyFile
	}
	if err != nil {
		return "", ErrUnknownFormat
	}
	normalized := map[string]bool{}
	for _, name := range columnNames {
		normalized[normalizeColumnName(name)] = true
	}
	switch {
	case normalized["login_password"] || normalized["login_username"] || normalized["login_uri"]:
		return FormatBitwardenCSV, nil
	case normalized["archived"] && normalized["otpauth"] && normalized["title"]:
		return FormatOnePasswordCSV, nil
	case normalized["extra"] && normalized["grouping"] && normalized["url"]:
		return FormatLastPassCSV, nil
	}
	if _, err := buildCSVLayout(columnNames); err != nil {
		return "", ErrUnknownFormat
	}
	return FormatCSV, nil
}

func parseCSV(content []byte, format string) (*Result, error) {
	reader := createCSVReader(content)
	columnNames, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, ErrEmptyFile
	}
	if err != nil {
		return nil, convertCSVError(err)
	}
	layout, err := buildCSVLayout(columnNames)
	if err != nil {
		return nil, err
	}
	result := &Result{Items: []item.Item{}, Skipped: []SkippedEntry{}}
	for {
		values, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, convertCSVError(err)
		}
		vaultItem, reason := convertCSVRecord(layout, values, format)
		if reason != "" {
			result.Skipped = append(result.Skipped, SkippedEntry{Title: describeEntry(layout.getValue(values, columnTitle)), Reason: reason})
			continue
		}
		if finishItem(&vaultItem) {
			result.Items = append(result.Items, vaultItem)
		}
	}
	return result, nil
}

func convertCSVRecord(layout *csvLayout, values []string, format string) (item.Item, string) {
	getValue := func(column string) string { return layout.getValue(values, column) }
	if format == FormatOnePasswordCSV && strings.EqualFold(strings.TrimSpace(getValue(columnArchived)), "true") {
		return item.Item{}, "archived in 1Password"
	}
	entryType := strings.ToLower(strings.TrimSpace(getValue(columnType)))
	if format == FormatBitwardenCSV {
		switch entryType {
		case "", "login", "note":
		case "card":
			return item.Item{}, "Bitwarden CSV exports leave out card details; export as JSON to import cards"
		case "identity":
			return item.Item{}, "Bitwarden CSV exports leave out identity details; export as JSON to import identities"
		default:
			return item.Item{}, fmt.Sprintf("Bitwarden %s items are not supported in Hisar yet", entryType)
		}
	}
	// LastPass's vault export has an http://group row for each folder; it is not an entry, and an empty item is dropped.
	if format == FormatLastPassCSV && strings.EqualFold(strings.TrimSpace(getValue(columnURL)), "http://group") {
		return item.Item{}, ""
	}
	if format == FormatLastPassCSV && isLastPassSecureNoteURL(getValue(columnURL)) && isLastPassTypedNote(getValue(columnNotes)) {
		vaultItem, reason := convertLastPassNote(getValue(columnTitle), getValue(columnNotes))
		if reason == "" {
			vaultItem.IsFavorite = isCSVTrue(getValue(columnFavorite))
			vaultItem.Overview.Tags = splitCSVTags(getValue(columnTags), format)
		}
		return vaultItem, reason
	}
	vaultItem := item.Item{
		Overview: item.Overview{Title: getValue(columnTitle), Username: getValue(columnUsername)},
		Details:  item.Details{Password: getValue(columnPassword), Notes: getValue(columnNotes)},
	}
	// A Bitwarden note and a LastPass http://sn row are notes even without notes text; finishItem still makes one with a
	// username, password or website a login.
	if (format == FormatBitwardenCSV && entryType == "note") || isLastPassSecureNoteURL(getValue(columnURL)) {
		vaultItem.Overview.Category = item.CategorySecureNote
	}
	vaultItem.IsFavorite = isCSVTrue(getValue(columnFavorite))
	vaultItem.Overview.URLs = splitURLs(getValue(columnURL), format)
	addOneTimePassword(&vaultItem, "One-time password", strings.TrimSpace(getValue(columnOTP)))
	vaultItem.Overview.Tags = splitCSVTags(getValue(columnTags), format)
	if format == FormatBitwardenCSV {
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, parseBitwardenCSVFields(getValue(columnFields))...)
	}
	for _, index := range layout.extraIndexes {
		if index < len(values) && strings.TrimSpace(values[index]) != "" {
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: layout.columnNames[index], Value: values[index]})
		}
	}
	return vaultItem, ""
}

// splitCSVTags reads the tags column. A Bitwarden folder is one tag, and so is a LastPass folder, whose nested folders
// LastPass joins with backslashes (Work\Servers becomes Work/Servers) and older LastPass files name "(none)" for no folder;
// other exports list tags with commas or semicolons.
func splitCSVTags(text, format string) []string {
	switch {
	case strings.TrimSpace(text) == "":
		return nil
	case format == FormatBitwardenCSV:
		return []string{text}
	case format == FormatLastPassCSV && strings.EqualFold(strings.TrimSpace(text), "(none)"):
		return nil
	case format == FormatLastPassCSV:
		return []string{strings.ReplaceAll(text, "\\", "/")}
	}
	return strings.FieldsFunc(text, func(character rune) bool { return character == ',' || character == ';' })
}

// isCSVTrue reads a flag column: 1Password writes true, Bitwarden and LastPass 1.
func isCSVTrue(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

// isLastPassSecureNoteURL: LastPass marks secure notes with the URL http://sn.
func isLastPassSecureNoteURL(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "http://sn")
}

// splitURLs: Bitwarden joins several URIs with commas; a LastPass secure note has no website.
func splitURLs(text string, format string) []string {
	text = strings.TrimSpace(text)
	if text == "" || isLastPassSecureNoteURL(text) {
		return nil
	}
	if format == FormatBitwardenCSV {
		return strings.FieldsFunc(text, func(character rune) bool { return character == ',' || character == '\n' })
	}
	return strings.Split(text, "\n")
}

// parseBitwardenCSVFields reads Bitwarden's "name: value" lines.
func parseBitwardenCSVFields(text string) []item.Field {
	fields := []item.Field{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, value, found := strings.Cut(line, ": ")
		if !found {
			name, value = "Field", line
		}
		fields = append(fields, item.Field{Name: name, Value: value})
	}
	return fields
}
