package export

import (
	"context"
	"fmt"
	"strings"

	"github.com/tryhisar/hisar/core/session"
)

// Provenance travels with a portable copy even after the exporting device is gone.
type Provenance struct {
	Source    string              `json:"source"`
	Vaults    []VaultCheckpoint   `json:"vaults"`
	Omissions []session.ReadIssue `json:"omissions"`
}

type VaultCheckpoint struct {
	VaultID           string `json:"vaultId"`
	HasCompletedSync  bool   `json:"hasCompletedSync"`
	CompletedAt       string `json:"completedAt"`
	CompletedSequence int64  `json:"completedSequence"`
}

type Collection struct {
	Vaults     []Vault
	Provenance Provenance
}

// Collect decrypts every record the session can open. A vault whose name did not open is exported as a recovery
// container, its name issue kept among the omissions; a vault never acquired to the end is a "sync-unknown" omission,
// never an empty vault.
func Collect(ctx context.Context, unlocked *session.Session) (Collection, error) {
	collection := Collection{Vaults: []Vault{}, Provenance: Provenance{Source: "online", Vaults: []VaultCheckpoint{}, Omissions: []session.ReadIssue{}}}
	inventory, err := unlocked.ReadVaults(ctx)
	if err != nil {
		return collection, err
	}
	collection.Provenance.Source = inventory.Source
	collection.Provenance.Omissions = append(collection.Provenance.Omissions, inventory.Issues...)
	for index := range inventory.Vaults {
		vault := &inventory.Vaults[index]
		defer clear(vault.Key)
		result, err := unlocked.ReadItems(ctx, vault)
		if err != nil {
			return collection, err
		}
		source := result.Provenance.Source
		if collection.Provenance.Source != source {
			collection.Provenance.Source = "mixed"
		}
		collection.Provenance.Vaults = append(collection.Provenance.Vaults, VaultCheckpoint{VaultID: vault.ID, HasCompletedSync: result.Provenance.HasCompletedSync, CompletedAt: result.Provenance.CompletedAt, CompletedSequence: result.Provenance.CompletedSequence})
		collection.Provenance.Omissions = append(collection.Provenance.Omissions, result.Issues...)
		if !result.Provenance.HasCompletedSync {
			collection.Provenance.Omissions = append(collection.Provenance.Omissions, session.ReadIssue{VaultID: vault.ID, Kind: "sync-unknown"})
			if len(result.Items) == 0 {
				continue
			}
		}
		exported := ConvertVault(vault.ID, vault.Name, vault.Type, result.Items)
		if vault.NameUnreadable {
			exported.Name, exported.NameUnreadable = "Unreadable vault "+vault.ID, true
		}
		collection.Vaults = append(collection.Vaults, exported)
	}
	return collection, nil
}

func (p *Provenance) IsCachedCopy() bool { return p != nil && p.Source != "online" }
func (p *Provenance) IsPartial() bool    { return p != nil && len(p.Omissions) != 0 }

func (p *Provenance) Message() string {
	if p == nil {
		return ""
	}
	var lines []string
	if p.IsCachedCopy() {
		lines = append(lines, "Cached copy. Changes since this device's completed syncs may be absent.")
	}
	if p.IsPartial() {
		lines = append(lines, fmt.Sprintf("Partial copy. %d known omissions.", len(p.Omissions)))
	}
	for _, checkpoint := range p.Vaults {
		if p.IsCachedCopy() && checkpoint.HasCompletedSync {
			lines = append(lines, fmt.Sprintf("Vault %s last completed sync %s.", checkpoint.VaultID, checkpoint.CompletedAt))
		}
	}
	for _, issue := range p.Omissions {
		lines = append(lines, fmt.Sprintf("%s vault %s item %s revision %s", issue.Kind, issue.VaultID, issue.ItemID, issue.Revision))
	}
	return strings.Join(lines, "\n")
}
