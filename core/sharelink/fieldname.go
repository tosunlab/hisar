package sharelink

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/item"
)

// FindFieldKey maps a field name as people type it to its key; built-in names win over custom labels.
func FindFieldKey(vaultItem *item.Item, name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "username":
		return FieldUsername, nil
	case "password":
		return FieldPassword, nil
	case "url", "urls", "website", "websites":
		return FieldURLs, nil
	case "notes", "note":
		return FieldNotes, nil
	case "private key", "privatekey", "one-time code", "otp", "code", "passkey", "passkeys":
		return "", fmt.Errorf("%q can't be shared: SSH private keys, passkeys and one-time password secrets never leave Hisar", name)
	}
	foundKey := ""
	for index, field := range vaultItem.Details.Fields {
		if !strings.EqualFold(field.Name, strings.TrimSpace(name)) {
			continue
		}
		if foundKey != "" {
			return "", fmt.Errorf("item %q has two fields named %q", vaultItem.Overview.Title, name)
		}
		foundKey = GetCustomFieldKey(index)
	}
	if foundKey == "" {
		return "", fmt.Errorf("item %q has no field %q", vaultItem.Overview.Title, name)
	}
	return foundKey, nil
}

// FindFieldKeys returns the default keys for no names, and refuses a choice that shares nothing.
func FindFieldKeys(vaultItem *item.Item, names []string) ([]string, error) {
	if len(names) == 0 {
		keys := ListDefaultFieldKeys(vaultItem)
		if len(keys) == 0 {
			return nil, fmt.Errorf("item %q has no username or password; choose what to share with --field", vaultItem.Overview.Title)
		}
		return keys, nil
	}
	keys := []string{}
	for _, name := range names {
		key, err := FindFieldKey(vaultItem, name)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
