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
)

var allColumnAlias = map[string]string{
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
}

// allIgnoredColumn holds export metadata that is not worth keeping as a field.
var allIgnoredColumn = map[string]bool{
	"favorite": true, "fav": true, "reprompt": true, "httprealm": true, "formactionorigin": true, "guid": true,
	"timecreated": true, "timelastused": true, "timepasswordchanged": true, "created": true, "modified": true, "last modified": true,
}

type csvLayout struct {
	allIndexByColumn map[string]int
	allExtraIndex    []int
	allColumnName    []string
}

func (layout *csvLayout) getValue(allValue []string, column string) string {
	index, found := layout.allIndexByColumn[column]
	if !found || index >= len(allValue) {
		return ""
	}
	return allValue[index]
}

func normalizeColumnName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.Trim(name, "\ufeff \t\"")), " "))
}

// buildCSVLayout assigns each role to an exactly named column first, then to the first alias.
func buildCSVLayout(allColumnName []string) (*csvLayout, error) {
	layout := &csvLayout{allIndexByColumn: map[string]int{}, allColumnName: allColumnName}
	allIsAssigned := make([]bool, len(allColumnName))
	for index, name := range allColumnName {
		normalized := normalizeColumnName(name)
		if column, found := allColumnAlias[normalized]; found && column == normalized {
			if _, taken := layout.allIndexByColumn[column]; !taken {
				layout.allIndexByColumn[column] = index
				allIsAssigned[index] = true
			}
		}
	}
	for index, name := range allColumnName {
		if allIsAssigned[index] {
			continue
		}
		normalized := normalizeColumnName(name)
		if column, found := allColumnAlias[normalized]; found {
			if _, taken := layout.allIndexByColumn[column]; !taken {
				layout.allIndexByColumn[column] = index
				continue
			}
		}
		if normalized != "" && !allIgnoredColumn[normalized] {
			layout.allExtraIndex = append(layout.allExtraIndex, index)
		}
	}
	_, hasPassword := layout.allIndexByColumn[columnPassword]
	_, hasUsername := layout.allIndexByColumn[columnUsername]
	_, hasURL := layout.allIndexByColumn[columnURL]
	_, hasTitle := layout.allIndexByColumn[columnTitle]
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
	allColumnName, err := createCSVReader(content).Read()
	if errors.Is(err, io.EOF) {
		return "", ErrEmptyFile
	}
	if err != nil {
		return "", ErrUnknownFormat
	}
	allNormalized := map[string]bool{}
	for _, name := range allColumnName {
		allNormalized[normalizeColumnName(name)] = true
	}
	switch {
	case allNormalized["login_password"] || allNormalized["login_username"] || allNormalized["login_uri"]:
		return FormatBitwardenCSV, nil
	case allNormalized["archived"] && allNormalized["otpauth"] && allNormalized["title"]:
		return FormatOnePasswordCSV, nil
	}
	if _, err := buildCSVLayout(allColumnName); err != nil {
		return "", ErrUnknownFormat
	}
	return FormatCSV, nil
}

func parseCSV(content []byte, format string) (*Result, error) {
	reader := createCSVReader(content)
	allColumnName, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, ErrEmptyFile
	}
	if err != nil {
		return nil, convertCSVError(err)
	}
	layout, err := buildCSVLayout(allColumnName)
	if err != nil {
		return nil, err
	}
	result := &Result{AllItem: []item.Item{}, AllSkipped: []SkippedEntry{}}
	for {
		allValue, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, convertCSVError(err)
		}
		vaultItem, reason := convertCSVRecord(layout, allValue, format)
		if reason != "" {
			result.AllSkipped = append(result.AllSkipped, SkippedEntry{Title: describeEntry(layout.getValue(allValue, columnTitle)), Reason: reason})
			continue
		}
		if finishItem(&vaultItem) {
			result.AllItem = append(result.AllItem, vaultItem)
		}
	}
	return result, nil
}

func convertCSVRecord(layout *csvLayout, allValue []string, format string) (item.Item, string) {
	getValue := func(column string) string { return layout.getValue(allValue, column) }
	if format == FormatOnePasswordCSV && strings.EqualFold(strings.TrimSpace(getValue(columnArchived)), "true") {
		return item.Item{}, "archived in 1Password"
	}
	if format == FormatBitwardenCSV {
		switch entryType := strings.ToLower(strings.TrimSpace(getValue(columnType))); entryType {
		case "", "login", "note":
		default:
			return item.Item{}, fmt.Sprintf("Bitwarden %s items are not supported in Hisar yet", entryType)
		}
	}
	vaultItem := item.Item{
		Overview: item.Overview{Title: getValue(columnTitle), Username: getValue(columnUsername)},
		Details:  item.Details{Password: getValue(columnPassword), Notes: getValue(columnNotes)},
	}
	vaultItem.Overview.URLs = splitAllURL(getValue(columnURL), format)
	if otp := strings.TrimSpace(getValue(columnOTP)); otp != "" {
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: "One-time password", Value: otp, Concealed: true})
	}
	if tagText := getValue(columnTags); tagText != "" {
		if format == FormatBitwardenCSV {
			vaultItem.Overview.Tags = []string{tagText}
		} else {
			vaultItem.Overview.Tags = strings.FieldsFunc(tagText, func(character rune) bool { return character == ',' || character == ';' })
		}
	}
	if format == FormatBitwardenCSV {
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, parseAllBitwardenCSVField(getValue(columnFields))...)
	}
	for _, index := range layout.allExtraIndex {
		if index < len(allValue) && strings.TrimSpace(allValue[index]) != "" {
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: layout.allColumnName[index], Value: allValue[index]})
		}
	}
	return vaultItem, ""
}

// splitAllURL: Bitwarden joins several URIs with commas; LastPass marks secure notes with the URL http://sn.
func splitAllURL(text string, format string) []string {
	text = strings.TrimSpace(text)
	if text == "" || strings.EqualFold(text, "http://sn") {
		return nil
	}
	if format == FormatBitwardenCSV {
		return strings.FieldsFunc(text, func(character rune) bool { return character == ',' || character == '\n' })
	}
	return strings.Split(text, "\n")
}

// parseAllBitwardenCSVField reads Bitwarden's "name: value" lines.
func parseAllBitwardenCSVField(text string) []item.Field {
	allField := []item.Field{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, value, found := strings.Cut(line, ": ")
		if !found {
			name, value = "Field", line
		}
		allField = append(allField, item.Field{Name: name, Value: value})
	}
	return allField
}
