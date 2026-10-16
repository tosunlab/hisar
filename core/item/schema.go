package item

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NamedField is one value of an item type that keeps its values as custom fields with fixed names, such as a card's
// number: its field name, whether it is concealed, and where it lives in the type's record T. A new type of this kind is
// a record struct and a list of its NamedFields.
type NamedField[T any] struct {
	Name      string
	Concealed bool
	value     func(*T) *string
}

// Value points at the field's value in record.
func (field NamedField[T]) Value(record *T) *string { return field.value(record) }

// readRecord returns the record held in the item's custom fields and the item's other custom fields. Names match
// ignoring case, since an older app may have saved them; when a name repeats, the first one is the record's and the rest
// stay other fields.
func readRecord[T any](vaultItem *Item, namedFields []NamedField[T]) (T, []Field) {
	var record T
	isRead := map[string]bool{}
	otherFields := []Field{}
	for _, field := range vaultItem.Details.Fields {
		isNamedField := false
		for _, namedField := range namedFields {
			if strings.EqualFold(field.Name, namedField.Name) && !isRead[namedField.Name] {
				*namedField.value(&record) = field.Value
				isRead[namedField.Name], isNamedField = true, true
				break
			}
		}
		if !isNamedField {
			otherFields = append(otherFields, field)
		}
	}
	return record, otherFields
}

// writeRecord stores record as the item's first custom fields, in the order of namedFields with the secret ones
// concealed, followed by otherFields; an empty value is left out.
func writeRecord[T any](vaultItem *Item, record T, namedFields []NamedField[T], otherFields []Field) {
	fields := []Field{}
	for _, namedField := range namedFields {
		if value := *namedField.value(&record); value != "" {
			fields = append(fields, Field{Name: namedField.Name, Value: value, Concealed: namedField.Concealed})
		}
	}
	fields = append(fields, otherFields...)
	vaultItem.Details.Fields = nil
	if len(fields) > 0 {
		vaultItem.Details.Fields = fields
	}
}

// trimRecord trims every value of record.
func trimRecord[T any](record *T, namedFields []NamedField[T]) {
	for _, namedField := range namedFields {
		value := namedField.value(record)
		*value = strings.TrimSpace(*value)
	}
}

var yearFirstDate = regexp.MustCompile(`^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})$`)

// NormalizeDate writes a date given year first, such as 1990/4/12, as 1990-04-12: an identity's birth date or an API
// credential's expiry. Any other date, or one with no such month or day, is kept as typed: 04/12/1990 may mean April or
// December.
func NormalizeDate(date string) string {
	date = strings.TrimSpace(date)
	if parsed, isDate := parseYearFirstDate(date); isDate {
		return parsed.Format(time.DateOnly)
	}
	return date
}

// parseYearFirstDate reads a year-first date as midnight UTC of that day; a day that does not exist is no date.
func parseYearFirstDate(date string) (time.Time, bool) {
	match := yearFirstDate.FindStringSubmatch(date)
	if match == nil {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	day, _ := strconv.Atoi(match[3])
	// time.Date carries an impossible day, such as February 31, into the next month.
	parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if parsed.Year() != year || int(parsed.Month()) != month || parsed.Day() != day {
		return time.Time{}, false
	}
	return parsed, true
}
