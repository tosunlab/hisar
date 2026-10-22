package app

import (
	"fmt"
	"strings"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
)

// resolveCommandLineItem finds the item as hisar get does: the vault by ID, else by its one name; the item by ID, else by
// its one exact title, else by the one title containing the query. A name or title more than one has is refused.
func (s *Service) resolveCommandLineItem(vaultNameOrID, query string) (string, item.Item, error) {
	vaults, err := s.ListVaults()
	if err != nil {
		return "", item.Item{}, err
	}
	vault, err := selectCommandLineVault(vaults, vaultNameOrID)
	if err != nil {
		return "", item.Item{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	items := s.itemsByVaultID[vault.ID]
	for index := range items {
		if items[index].ID == query {
			return vault.ID, items[index], nil
		}
	}
	// An item that did not open may have the title asked for, so a title lookup needs every item.
	for _, issue := range vault.ReadIssues {
		if issue.ItemID != "" {
			return "", item.Item{}, session.ErrIncompleteRead
		}
	}
	var exact, partial []*item.Item
	for index := range items {
		vaultItem := &items[index]
		if strings.EqualFold(vaultItem.Overview.Title, query) {
			exact = append(exact, vaultItem)
		}
		if strings.Contains(strings.ToLower(vaultItem.Overview.Title), strings.ToLower(query)) {
			partial = append(partial, vaultItem)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	switch len(matches) {
	case 0:
		return "", item.Item{}, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("no item matches %q in %s", query, describeCommandLineVault(vault)))
	case 1:
		return vault.ID, *matches[0], nil
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d items in %s; use an ID (hisar ls -v <vault-ID> lists metadata):", query, len(matches), describeCommandLineVault(vault))
	for _, match := range matches {
		fmt.Fprintf(&builder, "\n  %s  %s", match.ID, match.Overview.Title)
	}
	return "", item.Item{}, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, builder.String())
}

// selectCommandLineVault picks the vault with this ID, else the one vault with this name ignoring case. A name lookup
// needs every vault's name: one that did not open may be the name asked for.
func selectCommandLineVault(vaults []VaultSummary, nameOrID string) (VaultSummary, error) {
	for _, vault := range vaults {
		if vault.ID == nameOrID {
			return vault, nil
		}
	}
	for _, vault := range vaults {
		for _, issue := range vault.ReadIssues {
			if issue.ItemID == "" {
				return VaultSummary{}, session.ErrIncompleteRead
			}
		}
	}
	var matches []VaultSummary
	for _, vault := range vaults {
		if strings.EqualFold(vault.Name, nameOrID) {
			matches = append(matches, vault)
		}
	}
	switch len(matches) {
	case 0:
		return VaultSummary{}, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("vault %q: not found", nameOrID))
	case 1:
		return matches[0], nil
	}
	candidates := make([]string, 0, len(matches))
	for _, vault := range matches {
		candidates = append(candidates, describeCommandLineVault(vault))
	}
	return VaultSummary{}, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("%d vaults are named %q; use an ID instead: %s", len(matches), nameOrID, strings.Join(candidates, ", ")))
}

func describeCommandLineVault(vault VaultSummary) string {
	return fmt.Sprintf("%s (name %q, type %s, role %s, organization %q)", vault.ID, vault.Name, vault.Type, vault.Role, vault.OrganizationID)
}

// findShareLinkItem finds the item a share link request names: by its ID for a launcher, by vault and query as hisar get
// does for hisar share-link. The approval window and the link both use it, so the dialog names the item that is shared.
func (bridge *CommandLineBridge) findShareLinkItem(request *cliprotocol.Request) (string, item.Item, error) {
	if request.Type != cliprotocol.TypeLauncherShareItem {
		return bridge.service.resolveCommandLineItem(request.Vault, request.Query)
	}
	if _, err := bridge.getItem(request.VaultID, request.ItemID); err != nil {
		return "", item.Item{}, err
	}
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vaultItem, err := s.findItem(request.VaultID, request.ItemID)
	if err != nil {
		return "", item.Item{}, err
	}
	return request.VaultID, *vaultItem, nil
}

// DescribeShareLink names the item and fields for the approval window; while locked it shows the request as written.
func (bridge *CommandLineBridge) DescribeShareLink(request *cliprotocol.Request) (string, []string) {
	names := append([]string{}, request.ShareLink.Fields...)
	if bridge.service.GetStatus().State != StateUnlocked {
		return request.Query, names
	}
	_, vaultItem, err := bridge.findShareLinkItem(request)
	if err != nil {
		return request.Query, names
	}
	fieldKeys, err := sharelink.FindFieldKeys(&vaultItem, request.ShareLink.Fields)
	if err != nil {
		return vaultItem.Overview.Title, names
	}
	labels := make([]string, 0, len(fieldKeys))
	for _, key := range fieldKeys {
		labels = append(labels, sharelink.DescribeFieldKey(&vaultItem, key))
	}
	return vaultItem.Overview.Title, labels
}

// createShareLink shares the fields of the item a launcher or hisar share-link names, or hisar's text.
func (bridge *CommandLineBridge) createShareLink(request *cliprotocol.Request) (any, error) {
	change := request.ShareLink
	if change == nil {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the share link is missing")
	}
	shareLinkRequest := ShareLinkRequest{ExpiresInSeconds: int(change.ExpiresInSeconds), MaxViews: change.MaxViews, Passphrase: change.Passphrase, Title: change.Title, Text: change.Text}
	if request.Type == cliprotocol.TypeLauncherShareItem || request.Query != "" {
		vaultID, vaultItem, err := bridge.findShareLinkItem(request)
		if err != nil {
			return nil, err
		}
		fieldKeys, err := sharelink.FindFieldKeys(&vaultItem, change.Fields)
		if err != nil {
			return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, err.Error())
		}
		shareLinkRequest.VaultID, shareLinkRequest.ItemID, shareLinkRequest.FieldKeys = vaultID, vaultItem.ID, fieldKeys
	}
	created, err := bridge.service.CreateShareLink(shareLinkRequest)
	if err != nil {
		return nil, err
	}
	return cliprotocol.ShareLinkResult{ShareLink: convertToCommandLineShareLink(created)}, nil
}

func convertToCommandLineShareLink(link ShareLink) cliprotocol.ShareLink {
	return cliprotocol.ShareLink{
		ID: link.ID, URL: link.URL, Title: link.Title, FieldLabels: append([]string{}, link.FieldLabels...), IsText: link.IsText,
		CreatedAt: link.CreatedAt, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews, ViewCount: link.ViewCount, HasPassphrase: link.HasPassphrase,
	}
}

func (bridge *CommandLineBridge) listShareLinks() (any, error) {
	links, err := bridge.service.ListShareLinks()
	if err != nil {
		return nil, err
	}
	result := cliprotocol.ShareLinkListResult{ShareLinks: []cliprotocol.ShareLink{}}
	for _, link := range links {
		commandLineLink := convertToCommandLineShareLink(link)
		commandLineLink.URL = ""
		result.ShareLinks = append(result.ShareLinks, commandLineLink)
	}
	return result, nil
}
