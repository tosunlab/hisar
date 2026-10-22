package session

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/snapshot"
)

var ErrIncompleteRead = errors.New("some records could not be opened; retry or repair the records before this operation")

// ReadIssue identifies omitted data without carrying plaintext or cryptographic diagnostics. Revision is an item's
// revision.
type ReadIssue struct {
	VaultID  string `json:"vaultId"`
	ItemID   string `json:"itemId,omitempty"`
	Revision string `json:"revision,omitempty"`
	Kind     string `json:"kind"`
}

// ReadProvenance says where one vault's items came from: the server ("online") or, while it is unreachable, the
// cache ("cached"), and the completed acquisition they are complete up to.
type ReadProvenance struct {
	Source string `json:"source"`
	snapshot.Checkpoint
}

type ItemReadResult struct {
	Items      []item.Item
	Issues     []ReadIssue
	Provenance ReadProvenance
}

type VaultReadResult struct {
	Vaults []Vault
	Issues []ReadIssue
	Source string
}

func (s *Session) ReadVaults(ctx context.Context) (VaultReadResult, error) {
	return s.readVaults(ctx, true)
}

func (s *Session) readVaults(ctx context.Context, retry bool) (VaultReadResult, error) {
	result := VaultReadResult{Vaults: []Vault{}, Issues: []ReadIssue{}, Source: "online"}
	records, fetched, err := s.listProtoVaults(ctx)
	if err != nil {
		return result, err
	}
	if !fetched {
		result.Source = "cached"
	}
	for _, record := range records {
		vault, issue := s.openVault(record)
		if vault != nil {
			result.Vaults = append(result.Vaults, *vault)
		}
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
		}
	}
	if fetched && retry && len(result.Issues) != 0 {
		clearVaultKeys(result.Vaults)
		return s.readVaults(ctx, false)
	}
	// Only a list in which every record opened replaces the cache.
	if fetched && s.Cache != nil && len(result.Issues) == 0 {
		if err := s.replaceCachedVaults(records); err != nil {
			clearVaultKeys(result.Vaults)
			return VaultReadResult{}, err
		}
	}
	return result, nil
}

// openVault opens a vault record: a nil vault when its key does not open, and an issue when its key or name does not.
func (s *Session) openVault(record apitypes.Vault) (*Vault, *ReadIssue) {
	issue := &ReadIssue{VaultID: record.ID}
	key, err := s.Keyset.UnwrapVaultKey(record.ID, s.AccountID, record.WrappedKey)
	if err != nil {
		issue.Kind = "vault-key"
		return nil, issue
	}
	vault := &Vault{
		ID: record.ID, Type: record.Type, Role: record.Role, Key: key, OrganizationID: record.OrganizationID, KeyGeneration: record.KeyGeneration,
		AccessRevision: record.AccessRevision, NeedsRotation: record.NeedsRotation, record: record,
	}
	name, err := crypt.OpenVaultName(key, record.ID, record.EncName)
	if err != nil {
		vault.NameUnreadable = true
		issue.Kind = "vault-name"
		return vault, issue
	}
	vault.Name = string(name)
	return vault, nil
}

func clearVaultKeys(vaults []Vault) {
	for index := range vaults {
		clear(vaults[index].Key)
	}
}

// ReadItems returns the vault's items that open and an issue for each one that does not. When a record does not open
// after an online acquisition, the vault is reloaded once, since another device may have rotated its key meanwhile.
func (s *Session) ReadItems(ctx context.Context, vault *Vault) (ItemReadResult, error) {
	result := ItemReadResult{Items: []item.Item{}, Issues: []ReadIssue{}}
	acquired, err := s.acquireItems(ctx, vault, false)
	if errors.Is(err, ErrKeyChanged) {
		if err = s.reloadVaults(ctx, vault); err == nil {
			acquired, err = s.acquireItems(ctx, vault, false)
		}
	}
	if err != nil {
		return result, err
	}
	result.Items, result.Issues = acquired.open(vault)
	if len(result.Issues) != 0 && acquired.provenance.Source == "online" && s.reloadVaults(ctx, vault) == nil {
		if acquired, err = s.acquireItems(ctx, vault, true); err != nil {
			return result, err
		}
		result.Items, result.Issues = acquired.open(vault)
	}
	result.Provenance = acquired.provenance
	return result, nil
}

// acquisition is one vault's records and where they came from. sealedWith, when set, is the vault as published with a
// cached snapshot of another key generation, whose key opens those records.
type acquisition struct {
	records    []apitypes.Item
	provenance ReadProvenance
	sealedWith *Vault
}

func (acquired acquisition) open(vault *Vault) ([]item.Item, []ReadIssue) {
	if acquired.sealedWith == nil {
		return readItems(vault, acquired.records)
	}
	defer clear(acquired.sealedWith.Key)
	return readItems(acquired.sealedWith, acquired.records)
}

func (s *Session) acquireItems(ctx context.Context, vault *Vault, reset bool) (acquisition, error) {
	if s.Cache == nil {
		return s.downloadItems(ctx, vault)
	}
	return s.syncVaultSnapshot(ctx, vault, reset)
}

func readItems(vault *Vault, records []apitypes.Item) ([]item.Item, []ReadIssue) {
	items := make([]item.Item, 0, len(records))
	issues := []ReadIssue{}
	for index := range records {
		record := &records[index]
		opened, err := decryptItem(vault, record)
		if err != nil {
			issues = append(issues, ReadIssue{VaultID: vault.ID, ItemID: record.ID, Revision: record.Revision, Kind: "item"})
			continue
		}
		items = append(items, *opened)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Overview.Title) < strings.ToLower(items[j].Overview.Title)
	})
	return items, issues
}
