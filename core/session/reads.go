package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/snapshot"
)

var (
	ErrIncompleteRead = errors.New("some records could not be opened; retry or repair the records before this operation")
	ErrReviewChanged  = errors.New("that unreadable record changed; retry and review it again")
)

// ReadIssue identifies omitted data without carrying plaintext or cryptographic diagnostics. Revision is the reviewed
// revision of the record: an item's revision, or for a vault a digest of the record it was read from. A recovery
// action names it and is refused once the record changed.
type ReadIssue struct {
	VaultID   string `json:"vaultId"`
	ItemID    string `json:"itemId,omitempty"`
	Revision  string `json:"revision,omitempty"`
	Kind      string `json:"kind"`
	CanRename bool   `json:"canRename"`
	CanDelete bool   `json:"canDelete"`
	review    apitypes.VaultReview
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
	var verified, unverified []apitypes.Vault
	for _, record := range records {
		vault, issue := s.openVault(record)
		if vault != nil {
			result.Vaults = append(result.Vaults, *vault)
		}
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
			unverified = append(unverified, record)
		} else {
			verified = append(verified, record)
		}
	}
	if fetched && retry && len(result.Issues) != 0 {
		clearVaultKeys(result.Vaults)
		return s.readVaults(ctx, false)
	}
	if fetched && s.Cache != nil {
		if err := s.replaceCachedVaults(verified, unverified); err != nil {
			clearVaultKeys(result.Vaults)
			return VaultReadResult{}, err
		}
	}
	return result, nil
}

// openVault opens a vault record: a nil vault when its key does not open, and an issue when its key or name does not.
func (s *Session) openVault(record apitypes.Vault) (*Vault, *ReadIssue) {
	isOwner := record.Role == apitypes.VaultOwner
	issue := &ReadIssue{
		VaultID:   record.ID,
		CanDelete: isOwner && (record.Type != apitypes.VaultPersonal || record.IsPersonalExtra),
		review:    apitypes.VaultReview{EncName: record.EncName, WrappedKey: record.WrappedKey, KeyGeneration: record.KeyGeneration},
	}
	issue.Revision = describeReview(issue.review)
	key, err := s.Keyset.UnwrapVaultKey(record.ID, s.AccountID, record.WrappedKey)
	if err != nil {
		issue.Kind = "vault-key"
		return nil, issue
	}
	vault := &Vault{
		ID: record.ID, Type: record.Type, Role: record.Role, Key: key, OrganizationID: record.OrganizationID, KeyGeneration: record.KeyGeneration,
		AccessRevision: record.AccessRevision, NeedsRotation: record.NeedsRotation, IsPersonalExtra: record.IsPersonalExtra, record: record,
	}
	name, err := crypt.OpenVaultName(key, record.ID, record.EncName)
	if err != nil {
		vault.NameUnreadable = true
		issue.Kind, issue.CanRename = "vault-name", isOwner
		return vault, issue
	}
	vault.Name = string(name)
	return vault, nil
}

func describeReview(review apitypes.VaultReview) string {
	encoded, _ := json.Marshal(review)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func clearVaultKeys(vaults []Vault) {
	for index := range vaults {
		clear(vaults[index].Key)
	}
}

// ReadItems returns the vault's items that open and an issue for each one that does not. When a record does not open
// after an online acquisition, the vault is reloaded once, since another device may have rotated its key meanwhile; if
// that retry fails, the read that already succeeded stands.
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
		if retried, err := s.acquireItems(ctx, vault, true); err == nil {
			acquired = retried
			result.Items, result.Issues = acquired.open(vault)
		}
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
			issues = append(issues, ReadIssue{VaultID: vault.ID, ItemID: record.ID, Revision: record.Revision, Kind: "item", CanDelete: apitypes.CanWriteItems(vault.Role)})
			continue
		}
		items = append(items, *opened)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Overview.Title) < strings.ToLower(items[j].Overview.Title)
	})
	return items, issues
}

// DeleteUnreadableItem uses the displayed revision, never fabricated replacement plaintext.
func (s *Session) DeleteUnreadableItem(ctx context.Context, vault *Vault, itemID, revision string) error {
	record := &item.Item{ID: itemID, Revision: revision}
	_, err := s.putItem(ctx, vault, record, nil, true)
	return err
}

// RepairVaultName seals a new name for a vault whose name did not open. The server stores it only while the vault is
// still the record issue was read from, so a name another device repaired meanwhile is never overwritten.
func (s *Session) RepairVaultName(ctx context.Context, vault *Vault, issue ReadIssue, name string) error {
	if issue.VaultID != vault.ID || issue.Kind != "vault-name" || !issue.CanRename {
		return ErrReviewChanged
	}
	return s.repairVaultName(ctx, vault, issue.review, name)
}

func (s *Session) repairVaultName(ctx context.Context, vault *Vault, review apitypes.VaultReview, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrVaultNameEmpty
	}
	if review.KeyGeneration != vault.KeyGeneration {
		return ErrReviewChanged
	}
	encryptedName, err := crypt.SealVaultName(vault.Key, vault.ID, []byte(name))
	if err != nil {
		return err
	}
	err = s.Client.RepairVaultName(ctx, vault.ID, &apitypes.RepairVaultNameRequest{Review: review, EncName: encryptedName})
	if apiclient.IsStatus(err, http.StatusConflict) {
		return fmt.Errorf("%w: %w", ErrReviewChanged, err)
	}
	if err != nil {
		return err
	}
	vault.Name, vault.NameUnreadable = name, false
	vault.record.EncName = encryptedName
	return nil
}

// DeleteUnreadableVault deletes a vault that did not open, with every item in it, only while it is still the record
// issue was read from.
func (s *Session) DeleteUnreadableVault(ctx context.Context, issue ReadIssue) error {
	if issue.ItemID != "" || !issue.CanDelete {
		return ErrReviewChanged
	}
	err := s.Client.DeleteUnreadableVault(ctx, issue.VaultID, &apitypes.DeleteUnreadableVaultRequest{Review: issue.review})
	if apiclient.IsStatus(err, http.StatusConflict) {
		return fmt.Errorf("%w: %w", ErrReviewChanged, err)
	}
	if err != nil {
		return err
	}
	s.noteOwnRemoval(issue.VaultID)
	return nil
}
