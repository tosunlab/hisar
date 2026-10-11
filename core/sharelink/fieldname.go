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

// FindAllFieldKey returns the default keys for no names, and refuses a choice that shares nothing.
func FindAllFieldKey(vaultItem *item.Item, allName []string) ([]string, error) {
	if len(allName) == 0 {
		allKey := ListAllDefaultFieldKey(vaultItem)
		if len(allKey) == 0 {
			return nil, fmt.Errorf("item %q has no username or password; choose what to share with --field", vaultItem.Overview.Title)
		}
		return allKey, nil
	}
	allKey := []string{}
	for _, name := range allName {
		key, err := FindFieldKey(vaultItem, name)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(allKey, key) {
			allKey = append(allKey, key)
		}
	}
	return allKey, nil
}
