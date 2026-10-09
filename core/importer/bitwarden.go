package importer

import (
	"encoding/json"
	"errors"
	"fmt"

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
	AllFolder []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"folders"`
	AllItem []bitwardenItem `json:"items"`
}

type bitwardenItem struct {
	Type     int    `json:"type"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	FolderID string `json:"folderId"`
	AllField []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Type  int    `json:"type"`
	} `json:"fields"`
	Login *struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
		AllURI   []struct {
			URI string `json:"uri"`
		} `json:"uris"`
	} `json:"login"`
	SSHKey *struct {
		PrivateKey string `json:"privateKey"`
	} `json:"sshKey"`
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
	allFolderName := map[string]string{}
	for _, folder := range export.AllFolder {
		allFolderName[folder.ID] = folder.Name
	}
	result := &Result{AllItem: []item.Item{}, AllSkipped: []SkippedEntry{}}
	for _, entry := range export.AllItem {
		vaultItem, reason := convertBitwardenItem(entry)
		if reason != "" {
			result.AllSkipped = append(result.AllSkipped, SkippedEntry{Title: describeEntry(entry.Name), Reason: reason})
			continue
		}
		if folderName := allFolderName[entry.FolderID]; folderName != "" {
			vaultItem.Overview.Tags = []string{folderName}
		}
		if finishItem(&vaultItem) {
			result.AllItem = append(result.AllItem, vaultItem)
		}
	}
	return result, nil
}

func convertBitwardenItem(entry bitwardenItem) (item.Item, string) {
	switch entry.Type {
	case bitwardenLogin, bitwardenSecureNote:
	case bitwardenSSHKey:
		if entry.SSHKey == nil {
			return item.Item{}, "the SSH key has no private key"
		}
		return buildSSHKeyItem(entry.Name, entry.SSHKey.PrivateKey, entry.Notes)
	case bitwardenCard:
		return item.Item{}, "Bitwarden cards are not supported in Hisar yet"
	case bitwardenIdentity:
		return item.Item{}, "Bitwarden identities are not supported in Hisar yet"
	default:
		return item.Item{}, "this kind of Bitwarden item is not supported in Hisar yet"
	}
	vaultItem := item.Item{Overview: item.Overview{Title: entry.Name}, Details: item.Details{Notes: entry.Notes}}
	if entry.Login != nil {
		vaultItem.Overview.Username = entry.Login.Username
		vaultItem.Details.Password = entry.Login.Password
		for _, uri := range entry.Login.AllURI {
			vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, uri.URI)
		}
		addOneTimePassword(&vaultItem, "One-time password", entry.Login.TOTP)
	}
	for _, field := range entry.AllField {
		if field.Type == bitwardenFieldLinked {
			continue
		}
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: field.Name, Value: field.Value, Concealed: field.Type == bitwardenFieldHidden})
	}
	return vaultItem, ""
}
