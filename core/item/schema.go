package item

import "strings"

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
