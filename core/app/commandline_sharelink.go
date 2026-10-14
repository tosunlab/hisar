package app

import (
	"fmt"
	"strings"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sharelink"
)

// resolveCommandLineItem finds the item as hisar get does: its ID or exact title, else one title containing the query.
func (s *Service) resolveCommandLineItem(vaultNameOrID, query string) (string, item.Item, error) {
	vaults, err := s.ListVaults()
	if err != nil {
		return "", item.Item{}, err
	}
	vaultID := ""
	for _, vault := range vaults {
		if vault.ID == vaultNameOrID || strings.EqualFold(vault.Name, vaultNameOrID) {
			vaultID = vault.ID
			break
		}
	}
	if vaultID == "" {
		return "", item.Item{}, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("vault %q: not found", vaultNameOrID))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var matches []*item.Item
	for index := range s.itemsByVaultID[vaultID] {
		vaultItem := &s.itemsByVaultID[vaultID][index]
		if vaultItem.ID == query || strings.EqualFold(vaultItem.Overview.Title, query) {
			return vaultID, *vaultItem, nil
		}
		if strings.Contains(strings.ToLower(vaultItem.Overview.Title), strings.ToLower(query)) {
			matches = append(matches, vaultItem)
		}
	}
	switch len(matches) {
	case 0:
		return "", item.Item{}, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("no item matches %q", query))
	case 1:
		return vaultID, *matches[0], nil
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d items; use the exact title or ID:", query, len(matches))
	for _, match := range matches {
		fmt.Fprintf(&builder, "\n  %s  %s", match.ID, match.Overview.Title)
	}
	return "", item.Item{}, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, builder.String())
}

// DescribeShareLink names the item and fields for the approval window; while locked it shows the request as written.
func (bridge *CommandLineBridge) DescribeShareLink(request *cliprotocol.Request) (string, []string) {
	names := append([]string{}, request.ShareLink.Fields...)
	if bridge.service.GetStatus().State != StateUnlocked {
		return request.Query, names
	}
	_, vaultItem, err := bridge.service.resolveCommandLineItem(request.Vault, request.Query)
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

func (bridge *CommandLineBridge) createShareLink(request *cliprotocol.Request) (any, error) {
	change := request.ShareLink
	if change == nil {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the share link is missing")
	}
	shareLinkRequest := ShareLinkRequest{ExpiresInSeconds: int(change.ExpiresInSeconds), MaxViews: change.MaxViews, Passphrase: change.Passphrase, Title: change.Title, Text: change.Text}
	if request.Query != "" {
		vaultID, vaultItem, err := bridge.service.resolveCommandLineItem(request.Vault, request.Query)
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
