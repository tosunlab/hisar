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
	allVault, err := s.ListAllVault()
	if err != nil {
		return "", item.Item{}, err
	}
	vaultID := ""
	for _, vault := range allVault {
		if vault.ID == vaultNameOrID || strings.EqualFold(vault.Name, vaultNameOrID) {
			vaultID = vault.ID
			break
		}
	}
	if vaultID == "" {
		return "", item.Item{}, cliprotocol.CreateError(cliprotocol.ErrorNotFound, fmt.Sprintf("vault %q: not found", vaultNameOrID))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var allMatch []*item.Item
	for index := range s.allItemByVaultID[vaultID] {
		vaultItem := &s.allItemByVaultID[vaultID][index]
		if vaultItem.ID == query || strings.EqualFold(vaultItem.Overview.Title, query) {
			return vaultID, *vaultItem, nil
		}
		if strings.Contains(strings.ToLower(vaultItem.Overview.Title), strings.ToLower(query)) {
			allMatch = append(allMatch, vaultItem)
		}
	}
	switch len(allMatch) {
	case 0:
		return "", item.Item{}, cliprotocol.CreateError(cliprotocol.ErrorNotFound, fmt.Sprintf("no item matches %q", query))
	case 1:
		return vaultID, *allMatch[0], nil
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d items; use the exact title or ID:", query, len(allMatch))
	for _, match := range allMatch {
		fmt.Fprintf(&builder, "\n  %s  %s", match.ID, match.Overview.Title)
	}
	return "", item.Item{}, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, builder.String())
}

// DescribeShareLink names the item and fields for the approval window; while locked it shows the request as written.
func (bridge *CommandLineBridge) DescribeShareLink(request *cliprotocol.Request) (string, []string) {
	allName := append([]string{}, request.ShareLink.Fields...)
	if bridge.service.GetStatus().State != StateUnlocked {
		return request.Query, allName
	}
	_, vaultItem, err := bridge.service.resolveCommandLineItem(request.Vault, request.Query)
	if err != nil {
		return request.Query, allName
	}
	allFieldKey, err := sharelink.FindAllFieldKey(&vaultItem, request.ShareLink.Fields)
	if err != nil {
		return vaultItem.Overview.Title, allName
	}
	allLabel := make([]string, 0, len(allFieldKey))
	for _, key := range allFieldKey {
		allLabel = append(allLabel, sharelink.DescribeFieldKey(&vaultItem, key))
	}
	return vaultItem.Overview.Title, allLabel
}

func (bridge *CommandLineBridge) createShareLink(request *cliprotocol.Request) (any, error) {
	change := request.ShareLink
	if change == nil {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "the share link is missing")
	}
	shareLinkRequest := ShareLinkRequest{ExpiresInSeconds: int(change.ExpiresInSeconds), MaxViews: change.MaxViews, Passphrase: change.Passphrase, Title: change.Title, Text: change.Text}
	if request.Query != "" {
		vaultID, vaultItem, err := bridge.service.resolveCommandLineItem(request.Vault, request.Query)
		if err != nil {
			return nil, err
		}
		allFieldKey, err := sharelink.FindAllFieldKey(&vaultItem, change.Fields)
		if err != nil {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, err.Error())
		}
		shareLinkRequest.VaultID, shareLinkRequest.ItemID, shareLinkRequest.FieldKeys = vaultID, vaultItem.ID, allFieldKey
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

func (bridge *CommandLineBridge) listAllShareLink() (any, error) {
	allLink, err := bridge.service.ListAllShareLink()
	if err != nil {
		return nil, err
	}
	result := cliprotocol.ShareLinkListResult{ShareLinks: []cliprotocol.ShareLink{}}
	for _, link := range allLink {
		commandLineLink := convertToCommandLineShareLink(link)
		commandLineLink.URL = ""
		result.ShareLinks = append(result.ShareLinks, commandLineLink)
	}
	return result, nil
}
