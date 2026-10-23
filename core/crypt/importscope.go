package crypt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

// ImportMember identifies an account with current destination access. Wrapped keys are not part of consent.
type ImportMember struct {
	AccountID string `json:"accountId"`
	Role      string `json:"role"`
}

// ImportScope binds an import to the exact destination and its reviewed account roles.
func ImportScope(vaultID string, members []ImportMember) string {
	members = slices.Clone(members)
	slices.SortFunc(members, func(a, b ImportMember) int {
		if order := strings.Compare(a.AccountID, b.AccountID); order != 0 {
			return order
		}
		return strings.Compare(a.Role, b.Role)
	})
	encoded, _ := json.Marshal(struct {
		Purpose string
		VaultID string
		Members []ImportMember
	}{"hisar import scope v1", vaultID, members})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
