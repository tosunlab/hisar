package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/totp"
)

// standaloneBackend signs in on its own: state.json, the Secret Key and the master password, unlocking only when needed.
type standaloneBackend struct {
	currentState    *localstate.State
	unlockedSession *session.Session
	vaultsByID      map[string]*session.Vault
	itemsByVaultID  map[string][]item.Item
	// signedInState is the state read for a command that needs only the device's token, not the master password.
	signedInState *localstate.State
}

// getSignedInClient returns the device's state and an API client with its token, without unlocking.
func (access *standaloneBackend) getSignedInClient() (*localstate.State, *apiclient.Client, error) {
	currentState := access.currentState
	if currentState == nil {
		if access.signedInState == nil {
			loadedState, err := loadState()
			if err != nil {
				return nil, nil, err
			}
			if err := requireLogin(loadedState); err != nil {
				return nil, nil, err
			}
			access.signedInState = loadedState
		}
		currentState = access.signedInState
	}
	return currentState, createAuthedClient(currentState), nil
}

func (access *standaloneBackend) requireSession(ctx context.Context) error {
	if access.unlockedSession != nil {
		return nil
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	access.currentState, access.unlockedSession = currentState, unlockedSession
	return nil
}

func (access *standaloneBackend) loadVaults(ctx context.Context) error {
	if err := access.requireSession(ctx); err != nil {
		return err
	}
	vaults, err := access.unlockedSession.ListVaults(ctx)
	if err != nil {
		return err
	}
	access.vaultsByID = map[string]*session.Vault{}
	for index := range vaults {
		access.vaultsByID[vaults[index].ID] = &vaults[index]
	}
	return nil
}

func (access *standaloneBackend) findVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if vault, isFound := access.vaultsByID[vaultID]; isFound {
		return vault, nil
	}
	if err := access.loadVaults(ctx); err != nil {
		return nil, err
	}
	if vault, isFound := access.vaultsByID[vaultID]; isFound {
		return vault, nil
	}
	return nil, fmt.Errorf("vault %s: not found", vaultID)
}

func (access *standaloneBackend) listVaults(ctx context.Context) ([]cliprotocol.Vault, error) {
	if err := access.loadVaults(ctx); err != nil {
		return nil, err
	}
	vaults := []cliprotocol.Vault{}
	for _, vault := range access.vaultsByID {
		vaults = append(vaults, cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role, OrganizationID: vault.OrganizationID})
	}
	slices.SortFunc(vaults, func(first, second cliprotocol.Vault) int {
		if (first.Type == apitypes.VaultPersonal) != (second.Type == apitypes.VaultPersonal) {
			if first.Type == apitypes.VaultPersonal {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return vaults, nil
}

func (access *standaloneBackend) createVault(ctx context.Context, name string) (cliprotocol.Vault, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.Vault{}, err
	}
	vault, err := access.unlockedSession.CreateVault(ctx, name, apitypes.VaultShared)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	if access.vaultsByID != nil {
		access.vaultsByID[vault.ID] = vault
	}
	return cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role}, nil
}

// listItems keeps each vault's items until a command changes them, so finding and then reading an item downloads once.
func (access *standaloneBackend) listItems(ctx context.Context, vaultID string) ([]item.Item, error) {
	if items, isFound := access.itemsByVaultID[vaultID]; isFound {
		return items, nil
	}
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	items, err := access.unlockedSession.ListItems(ctx, vault)
	if err != nil {
		return nil, err
	}
	if access.itemsByVaultID == nil {
		access.itemsByVaultID = map[string][]item.Item{}
	}
	access.itemsByVaultID[vaultID] = items
	return items, nil
}

func (access *standaloneBackend) getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error) {
	items, err := access.listItems(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if items[index].ID == itemID {
			return &items[index], nil
		}
	}
	return nil, fmt.Errorf("item %s: not found", itemID)
}

func (access *standaloneBackend) saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	var base *item.Item
	if vaultItem.Revision != "" {
		if cached, cacheErr := access.getItem(ctx, vaultID, vaultItem.ID); cacheErr == nil && cached != vaultItem {
			copied := *cached
			base = &copied
		}
	}
	delete(access.itemsByVaultID, vaultID)
	_, err = access.unlockedSession.SaveItem(ctx, vault, vaultItem, base)
	return err
}

func (access *standaloneBackend) importItems(ctx context.Context, vaultID string, items []item.Item) (int, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return 0, err
	}
	delete(access.itemsByVaultID, vaultID)
	return saveImportedItems(ctx, access.unlockedSession, vault, items)
}

func (access *standaloneBackend) listFavorites(ctx context.Context) ([]cliprotocol.Favorite, error) {
	if err := access.loadVaults(ctx); err != nil {
		return nil, err
	}
	preferences, err := access.unlockedSession.LoadPreferences(ctx)
	if err != nil {
		return nil, err
	}
	favorites := []cliprotocol.Favorite{}
	for vaultID := range access.vaultsByID {
		items, err := access.listItems(ctx, vaultID)
		if err != nil {
			return nil, err
		}
		for _, vaultItem := range items {
			if preferences.IsFavorite(vaultItem.ID) {
				favorites = append(favorites, cliprotocol.Favorite{VaultID: vaultID, ItemID: vaultItem.ID})
			}
		}
	}
	return favorites, nil
}

func (access *standaloneBackend) setItemFavorite(ctx context.Context, vaultID, itemID string, isFavorite bool) error {
	if err := access.requireSession(ctx); err != nil {
		return err
	}
	change := session.FavoriteChange{RemoveItemIDs: []string{itemID}}
	if isFavorite {
		change = session.FavoriteChange{AddItemIDs: []string{itemID}}
	}
	_, err := access.unlockedSession.UpdateFavorites(ctx, nil, change)
	return err
}

func (access *standaloneBackend) getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	items, err := access.unlockedSession.ListItems(ctx, vault)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	actions := cliprotocol.VaultActionsResult{
		Vault:               cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role, ItemCount: len(items)},
		MembersLosingAccess: []cliprotocol.VaultMember{},
		TargetVaults:        []cliprotocol.Vault{},
	}
	ownerCount := 0
	if vault.Type != apitypes.VaultPersonal {
		accesses, err := access.unlockedSession.Client.ListVaultMembers(ctx, vault.ID)
		if err != nil {
			return cliprotocol.VaultActionsResult{}, err
		}
		people, err := access.unlockedSession.Client.ListMembers(ctx)
		if err != nil {
			return cliprotocol.VaultActionsResult{}, err
		}
		for _, backend := range accesses {
			if backend.Role == apitypes.VaultOwner {
				ownerCount++
			}
			if backend.AccountID == access.unlockedSession.AccountID {
				continue
			}
			member := cliprotocol.VaultMember{Role: backend.Role}
			if index := slices.IndexFunc(people, func(person apitypes.Member) bool { return person.AccountID == backend.AccountID }); index >= 0 {
				member.Name, member.Email = people[index].Name, people[index].Email
			}
			actions.MembersLosingAccess = append(actions.MembersLosingAccess, member)
		}
	}
	permissions := session.GetVaultPermissions(vault.Type, vault.Role, len(access.vaultsByID), ownerCount)
	actions.CanRename, actions.CanDelete, actions.DeleteBlockedReason = permissions.CanRename, permissions.CanDelete, permissions.DeleteBlockedReason
	actions.CanLeave, actions.LeaveBlockedReason = permissions.CanLeave, permissions.LeaveBlockedReason
	targets, err := access.listVaults(ctx)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	for _, target := range targets {
		if target.ID != vault.ID && apitypes.CanWriteItems(target.Role) {
			actions.TargetVaults = append(actions.TargetVaults, target)
		}
	}
	return actions, nil
}

func (access *standaloneBackend) renameVault(ctx context.Context, vaultID, name string) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	return access.unlockedSession.RenameVault(ctx, vault, name)
}

func (access *standaloneBackend) deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return 0, err
	}
	sessionDeletion := session.VaultDeletion{IsDeletingItems: deletion.IsDeletingItems}
	if deletion.IsDeletingItems && deletion.ConfirmedName != vault.Name {
		return 0, errors.New("type the vault's name to delete it together with its items")
	}
	if deletion.MoveToVaultID != "" {
		if sessionDeletion.MoveTo, err = access.findVault(ctx, deletion.MoveToVaultID); err != nil {
			return 0, err
		}
	}
	movedCount, err := access.unlockedSession.DeleteVault(ctx, vault, sessionDeletion)
	if apiclient.IsStatus(err, http.StatusBadRequest) || apiclient.IsStatus(err, http.StatusForbidden) {
		var apiError *apiclient.APIError
		if errors.As(err, &apiError) {
			return 0, errors.New(apiError.Message)
		}
	}
	return movedCount, err
}

func (access *standaloneBackend) leaveVault(ctx context.Context, vaultID string) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	err = access.unlockedSession.LeaveVault(ctx, vault)
	var apiError *apiclient.APIError
	if apiclient.IsStatus(err, http.StatusBadRequest) && errors.As(err, &apiError) {
		return errors.New(apiError.Message)
	}
	return err
}

func (access *standaloneBackend) writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.ExportResult{}, err
	}
	vaults, err := export.Collect(ctx, access.unlockedSession)
	if err != nil {
		return cliprotocol.ExportResult{}, err
	}
	document := export.Build(export.Account{Email: access.currentState.Email, Server: access.currentState.Server}, vaults, time.Now())
	var content []byte
	if change.Format == cliprotocol.ExportFormatPlain {
		content, err = export.EncodePlain(document)
	} else {
		passphrase := []byte(change.Passphrase)
		content, err = export.EncodeEncrypted(document, passphrase)
		clear(passphrase)
	}
	if err != nil {
		return cliprotocol.ExportResult{}, err
	}
	defer clear(content)
	if err := writeNewFile(change.Path, content); err != nil {
		return cliprotocol.ExportResult{}, err
	}
	return cliprotocol.ExportResult{Path: change.Path, VaultCount: len(document.Vaults), ItemCount: document.CountItem()}, nil
}

func (access *standaloneBackend) finish(err error) error {
	if access.currentState == nil {
		return err
	}
	return finishSession(access.currentState, access.unlockedSession, err)
}

func (access *standaloneBackend) account(ctx context.Context) (cliprotocol.AccountResult, error) {
	currentState, authedClient, err := access.getSignedInClient()
	if err != nil {
		return cliprotocol.AccountResult{}, err
	}
	account, err := authedClient.GetAccount(ctx)
	if err != nil {
		return cliprotocol.AccountResult{}, err
	}
	return cliprotocol.AccountResult{AccountID: account.AccountID, Name: account.Name, Email: account.Email, Role: account.Role, Server: currentState.Server}, nil
}

// checkNewItemCategory: a standalone hisar saves the whole item itself, category included.
func (access *standaloneBackend) checkNewItemCategory(string) error { return nil }

// checkTags: a standalone hisar reads the whole item, tags included.
func (access *standaloneBackend) checkTags() error { return nil }

func (access *standaloneBackend) savePromptedItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	err := access.saveItem(ctx, vaultID, vaultItem)
	if errors.Is(err, session.ErrConflict) {
		return errors.New("item was changed on another device meanwhile; run edit again")
	}
	return err
}

func (access *standaloneBackend) deleteItem(ctx context.Context, vaultID, itemID string) error {
	vaultItem, err := access.getItem(ctx, vaultID, itemID)
	if err != nil {
		return err
	}
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	delete(access.itemsByVaultID, vaultID)
	_, err = access.unlockedSession.DeleteItem(ctx, vault, vaultItem)
	return err
}

// getOneTimeCode generates the code here: a standalone hisar holds the one-time password itself.
func (access *standaloneBackend) getOneTimeCode(ctx context.Context, vaultID, itemID string) (string, error) {
	vaultItem, err := access.getItem(ctx, vaultID, itemID)
	if err != nil {
		return "", err
	}
	config, err := totp.Parse(vaultItem.Details.OneTimePassword)
	if err != nil {
		return "", errNoOneTimeCode
	}
	code, _ := config.GenerateCode(time.Now())
	return code, nil
}

func (access *standaloneBackend) listPeople(ctx context.Context) ([]cliprotocol.Person, error) {
	currentState, authedClient, err := access.getSignedInClient()
	if err != nil {
		return nil, err
	}
	members, err := authedClient.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	people := make([]cliprotocol.Person, 0, len(members))
	for _, member := range members {
		people = append(people, cliprotocol.Person{
			AccountID: member.AccountID, Name: member.Name, Email: member.Email, Role: member.Role,
			SecurityCode: crypt.FormatKeyFingerprint(member.PublicKeys.Sign), IsCurrentUser: member.AccountID == currentState.AccountID,
			IsKeyChanged: currentState.IsMemberKeyChanged(member.AccountID, member.PublicKeys.Sign),
		})
	}
	return people, nil
}

func (access *standaloneBackend) shareVault(ctx context.Context, vaultID, email, role string) (string, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return "", err
	}
	if err := access.unlockedSession.InviteToVault(ctx, vault, email, role); err != nil {
		return "", err
	}
	return "then run `hisar invitations` to finish sharing.", nil
}

func (access *standaloneBackend) unshareVault(ctx context.Context, vaultID, emailOrAccountID string) (string, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return "", err
	}
	member, err := access.unlockedSession.GetVerifiedMember(ctx, emailOrAccountID)
	if err != nil {
		return "", err
	}
	if err := access.unlockedSession.RemoveVaultMember(ctx, vault, member.AccountID); err != nil {
		return "", err
	}
	return member.Email, nil
}

func (access *standaloneBackend) invite(ctx context.Context, email string) error {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return err
	}
	_, err = authedClient.Invite(ctx, &apitypes.InviteRequest{Email: email})
	return err
}

// listInvitations finishes the shares people accepted first, which needs the master password only when there are any.
func (access *standaloneBackend) listInvitations(ctx context.Context) (cliprotocol.InvitationListResult, error) {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return cliprotocol.InvitationListResult{}, err
	}
	invitationList, err := authedClient.ListShareInvitations(ctx)
	if err != nil {
		return cliprotocol.InvitationListResult{}, err
	}
	vaultNamesByID, completeErr := access.completeAcceptedShares(ctx, invitationList)
	if vaultNamesByID != nil {
		if invitationList, err = authedClient.ListShareInvitations(ctx); err != nil {
			return cliprotocol.InvitationListResult{}, err
		}
	}
	result := convertToCommandLineInvitationList(invitationList, vaultNamesByID)
	if completeErr != nil {
		result.CompletionError = completeErr.Error()
	}
	return result, nil
}

func (access *standaloneBackend) completeAcceptedShares(ctx context.Context, invitationList *apitypes.ShareInvitationList) (map[string]string, error) {
	isAnyAccepted := false
	for _, sent := range invitationList.Sent {
		isAnyAccepted = isAnyAccepted || sent.State == apitypes.ShareInvitationAccepted
	}
	if !isAnyAccepted {
		return nil, nil
	}
	fmt.Fprintln(standardError(), "Someone accepted your invitation; unlock to finish sharing.")
	if err := access.requireSession(ctx); err != nil {
		return nil, err
	}
	vaultNamesByID := map[string]string{}
	completedShares, completeErr := access.unlockedSession.CompleteAcceptedShares(ctx)
	for _, completed := range completedShares {
		fmt.Fprintf(standardError(), "Shared %q with %s as %s. Their security code: %s\n", completed.VaultName, completed.Recipient.Email, completed.Role,
			crypt.FormatKeyFingerprint(completed.Recipient.PublicKeys.Sign))
	}
	if vaults, err := access.unlockedSession.ListVaults(ctx); err == nil {
		for _, vault := range vaults {
			vaultNamesByID[vault.ID] = vault.Name
		}
	}
	return vaultNamesByID, completeErr
}

func (access *standaloneBackend) answerInvitation(ctx context.Context, action, invitationID string) error {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return err
	}
	switch action {
	case "accept":
		return authedClient.AcceptShareInvitation(ctx, invitationID)
	case "decline":
		return authedClient.DeclineShareInvitation(ctx, invitationID)
	}
	return authedClient.CancelShareInvitation(ctx, invitationID)
}

func (access *standaloneBackend) listShareLinks(ctx context.Context) ([]cliprotocol.ShareLink, error) {
	if err := access.requireSession(ctx); err != nil {
		return nil, err
	}
	links, err := access.unlockedSession.ListShareLinks(ctx)
	if err != nil {
		return nil, err
	}
	shareLinks := make([]cliprotocol.ShareLink, 0, len(links))
	for index := range links {
		shareLinks = append(shareLinks, convertToCommandLineShareLink(&links[index], false))
	}
	return shareLinks, nil
}

func (access *standaloneBackend) listDevices(ctx context.Context) ([]cliprotocol.Device, error) {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return nil, err
	}
	storedDevices, err := authedClient.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]cliprotocol.Device, 0, len(storedDevices))
	for _, device := range storedDevices {
		devices = append(devices, cliprotocol.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, LastSeenAt: device.LastSeenAt.Format(time.RFC3339), IsCurrent: device.Current})
	}
	return devices, nil
}

func (access *standaloneBackend) revokeDevice(ctx context.Context, deviceID string) error {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return err
	}
	return authedClient.RevokeDevice(ctx, deviceID)
}

var (
	errOrganizationNotFound  = errors.New("that organization, team, person or vault was not found, or you are no longer in the organization")
	errOrganizationAdminOnly = errors.New("only an admin of this organization can do that")
	errNotInTeam             = errors.New("only members of the team can create a vault for it")
)

// convertOrganizationError says what the server refused as the app does; 400 and 409 carry the server's own sentence.
func convertOrganizationError(err error) error {
	var apiError *apiclient.APIError
	switch {
	case apiclient.IsStatus(err, http.StatusNotFound):
		return errOrganizationNotFound
	case apiclient.IsStatus(err, http.StatusForbidden):
		return errOrganizationAdminOnly
	case (apiclient.IsStatus(err, http.StatusBadRequest) || apiclient.IsStatus(err, http.StatusConflict)) && errors.As(err, &apiError):
		return errors.New(apiError.Message)
	}
	return err
}

func (access *standaloneBackend) listOrganizations(ctx context.Context) (cliprotocol.OrganizationListResult, error) {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return cliprotocol.OrganizationListResult{}, err
	}
	organizationList, err := authedClient.ListOrganizations(ctx)
	if err != nil {
		return cliprotocol.OrganizationListResult{}, convertOrganizationError(err)
	}
	result := cliprotocol.OrganizationListResult{Organizations: []cliprotocol.OrganizationSummary{}, Invitations: []cliprotocol.OrganizationInvitation{}}
	for _, organization := range organizationList.Organizations {
		result.Organizations = append(result.Organizations, convertToCommandLineOrganizationSummary(organization))
	}
	slices.SortFunc(result.Organizations, func(first, second cliprotocol.OrganizationSummary) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	for _, invitation := range organizationList.Invitations {
		result.Invitations = append(result.Invitations, cliprotocol.OrganizationInvitation{
			ID: invitation.ID, OrganizationID: invitation.OrganizationID, OrganizationName: invitation.OrganizationName, Role: invitation.Role,
			InvitedByName: invitation.InvitedByName, InvitedByEmail: invitation.InvitedByEmail, ExpiresAt: invitation.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	return result, nil
}

func convertToCommandLineOrganizationSummary(organization apitypes.OrganizationSummary) cliprotocol.OrganizationSummary {
	return cliprotocol.OrganizationSummary{ID: organization.ID, Name: organization.Name, Role: organization.Role, MemberCount: organization.MemberCount}
}

// getOrganization names the team vaults this user can open only once a command unlocked (`org show` does), so changes
// that name an organization never ask for the master password.
func (access *standaloneBackend) getOrganization(ctx context.Context, organizationID string) (cliprotocol.Organization, error) {
	currentState, authedClient, err := access.getSignedInClient()
	if err != nil {
		return cliprotocol.Organization{}, err
	}
	protoOrganization, err := authedClient.GetOrganization(ctx, organizationID)
	if err != nil {
		return cliprotocol.Organization{}, convertOrganizationError(err)
	}
	if access.unlockedSession != nil {
		if err := access.loadVaults(ctx); err != nil {
			return cliprotocol.Organization{}, err
		}
	}
	organization := cliprotocol.Organization{
		ID: protoOrganization.ID, Name: protoOrganization.Name, Role: protoOrganization.Role,
		Members: []cliprotocol.OrganizationMember{}, Teams: []cliprotocol.Team{}, Invitations: []cliprotocol.SentOrganizationInvitation{},
	}
	membersByID := map[string]apitypes.OrganizationMember{}
	for _, protoMember := range protoOrganization.Members {
		membersByID[protoMember.AccountID] = protoMember
		member := cliprotocol.OrganizationMember{
			AccountID: protoMember.AccountID, Name: protoMember.Name, Email: protoMember.Email, Role: protoMember.Role,
			IsCurrentUser: protoMember.AccountID == currentState.AccountID,
		}
		if len(protoMember.PublicKeys.Sign) > 0 {
			member.SecurityCode = crypt.FormatKeyFingerprint(protoMember.PublicKeys.Sign)
			member.IsKeyChanged = currentState.IsMemberKeyChanged(protoMember.AccountID, protoMember.PublicKeys.Sign)
		}
		organization.Members = append(organization.Members, member)
	}
	slices.SortStableFunc(organization.Members, func(first, second cliprotocol.OrganizationMember) int {
		if (first.Role == apitypes.OrganizationRoleAdmin) != (second.Role == apitypes.OrganizationRoleAdmin) {
			if first.Role == apitypes.OrganizationRoleAdmin {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name+first.Email), strings.ToLower(second.Name+second.Email))
	})
	for _, protoTeam := range protoOrganization.Teams {
		organization.Teams = append(organization.Teams, access.convertToCommandLineTeam(protoTeam, membersByID))
	}
	slices.SortFunc(organization.Teams, func(first, second cliprotocol.Team) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	for _, invitation := range protoOrganization.Invitations {
		organization.Invitations = append(organization.Invitations, cliprotocol.SentOrganizationInvitation{
			ID: invitation.ID, Email: invitation.Email, Role: invitation.Role, ExpiresAt: invitation.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	return organization, nil
}

func (access *standaloneBackend) convertToCommandLineTeam(protoTeam apitypes.Team, membersByID map[string]apitypes.OrganizationMember) cliprotocol.Team {
	team := cliprotocol.Team{ID: protoTeam.ID, Name: protoTeam.Name, Members: []cliprotocol.TeamMember{}, Vaults: []cliprotocol.TeamVault{}}
	for _, memberID := range protoTeam.MemberIDs {
		member := membersByID[memberID]
		team.Members = append(team.Members, cliprotocol.TeamMember{AccountID: memberID, Name: member.Name, Email: member.Email})
	}
	slices.SortFunc(team.Members, func(first, second cliprotocol.TeamMember) int {
		return strings.Compare(strings.ToLower(first.Name+first.Email), strings.ToLower(second.Name+second.Email))
	})
	for _, protoVault := range protoTeam.Vaults {
		teamVault := cliprotocol.TeamVault{VaultID: protoVault.VaultID, Role: protoVault.Role}
		if vault, isFound := access.vaultsByID[protoVault.VaultID]; isFound {
			teamVault.Name, teamVault.CanOpen = vault.Name, true
		}
		team.Vaults = append(team.Vaults, teamVault)
	}
	slices.SortStableFunc(team.Vaults, func(first, second cliprotocol.TeamVault) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return team
}

func (access *standaloneBackend) createOrganization(ctx context.Context, name string) (cliprotocol.OrganizationSummary, error) {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return cliprotocol.OrganizationSummary{}, err
	}
	created, err := authedClient.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: name})
	if err != nil {
		return cliprotocol.OrganizationSummary{}, convertOrganizationError(err)
	}
	return convertToCommandLineOrganizationSummary(*created), nil
}

func (access *standaloneBackend) createTeam(ctx context.Context, organizationID, name string) (cliprotocol.Team, error) {
	_, authedClient, err := access.getSignedInClient()
	if err != nil {
		return cliprotocol.Team{}, err
	}
	created, err := authedClient.CreateTeam(ctx, organizationID, &apitypes.CreateTeamRequest{Name: name})
	if err != nil {
		return cliprotocol.Team{}, convertOrganizationError(err)
	}
	return access.convertToCommandLineTeam(*created, nil), nil
}

func (access *standaloneBackend) createOrganizationVault(ctx context.Context, organizationID, teamID, name string) (cliprotocol.Vault, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.Vault{}, err
	}
	vault, err := access.unlockedSession.CreateOrganizationVault(ctx, name, organizationID, teamID)
	if apiclient.IsStatus(err, http.StatusForbidden) {
		return cliprotocol.Vault{}, errNotInTeam
	}
	if err != nil {
		return cliprotocol.Vault{}, convertOrganizationError(err)
	}
	if access.vaultsByID != nil {
		access.vaultsByID[vault.ID] = vault
	}
	return cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role, OrganizationID: vault.OrganizationID}, nil
}

func (access *standaloneBackend) changeOrganization(ctx context.Context, change cliprotocol.Request) error {
	currentState, authedClient, err := access.getSignedInClient()
	if err != nil {
		return err
	}
	organizationID, teamID := change.OrganizationID, change.TeamID
	switch change.Type {
	case cliprotocol.TypeRenameOrganization:
		err = authedClient.RenameOrganization(ctx, organizationID, &apitypes.RenameOrganizationRequest{Name: change.Name})
	case cliprotocol.TypeDeleteOrganization:
		err = authedClient.DeleteOrganization(ctx, organizationID)
	case cliprotocol.TypeInviteToOrganization:
		_, err = authedClient.InviteToOrganization(ctx, organizationID, &apitypes.OrganizationInvitationRequest{Email: change.Email, Role: change.Role})
	case cliprotocol.TypeCancelOrganizationInvitation:
		err = authedClient.CancelOrganizationInvitation(ctx, organizationID, change.InvitationID)
	case cliprotocol.TypeAcceptOrganizationInvitation:
		err = authedClient.AcceptOrganizationInvitation(ctx, change.InvitationID)
	case cliprotocol.TypeDeclineOrganizationInvitation:
		err = authedClient.DeclineOrganizationInvitation(ctx, change.InvitationID)
	case cliprotocol.TypeSetOrganizationRole:
		err = authedClient.SetOrganizationMemberRole(ctx, organizationID, change.AccountID, &apitypes.SetOrganizationRoleRequest{Role: change.Role})
	case cliprotocol.TypeRemoveOrganizationMember:
		err = authedClient.RemoveOrganizationMember(ctx, organizationID, change.AccountID)
	case cliprotocol.TypeLeaveOrganization:
		err = authedClient.RemoveOrganizationMember(ctx, organizationID, currentState.AccountID)
	case cliprotocol.TypeRenameTeam:
		err = authedClient.RenameTeam(ctx, organizationID, teamID, &apitypes.RenameTeamRequest{Name: change.Name})
	case cliprotocol.TypeDeleteTeam:
		err = authedClient.DeleteTeam(ctx, organizationID, teamID)
	case cliprotocol.TypeAddTeamMember:
		err = authedClient.AddTeamMember(ctx, organizationID, teamID, change.AccountID)
	case cliprotocol.TypeRemoveTeamMember:
		err = authedClient.RemoveTeamMember(ctx, organizationID, teamID, change.AccountID)
	case cliprotocol.TypeGiveVaultToTeam:
		err = authedClient.GiveVaultToTeam(ctx, organizationID, teamID, change.VaultID, &apitypes.GrantTeamVaultRequest{Role: change.Role})
	case cliprotocol.TypeTakeVaultFromTeam:
		err = authedClient.TakeVaultFromTeam(ctx, organizationID, teamID, change.VaultID)
	default:
		return fmt.Errorf("unknown organization change %q", change.Type)
	}
	if change.InvitationID != "" && apiclient.IsStatus(err, http.StatusNotFound) {
		return errors.New("this invitation no longer exists")
	}
	return convertOrganizationError(err)
}

// completeOrganizationKeys does what the app does on every sync: deliver the keys team members wait for, rotate the
// vaults someone lost, and list what this user waits for. A server without organizations has nothing to do.
func (access *standaloneBackend) completeOrganizationKeys(ctx context.Context) (cliprotocol.OrganizationKeysResult, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.OrganizationKeysResult{}, err
	}
	result := cliprotocol.OrganizationKeysResult{Waiting: []cliprotocol.WaitingVaultKey{}}
	deliveredKeys, err := access.unlockedSession.CompleteVaultKeyRequests(ctx)
	if apiclient.IsStatus(err, http.StatusNotFound) {
		return result, nil
	}
	if err != nil {
		result.DeliveryError = err.Error()
	}
	rotatedVaults, err := access.unlockedSession.RotatePendingVaults(ctx)
	if err != nil {
		result.RotationError = err.Error()
	}
	result.DeliveredCount, result.RotatedCount = len(deliveredKeys), len(rotatedVaults)
	if len(rotatedVaults) > 0 {
		access.vaultsByID = nil
		for _, rotatedVault := range rotatedVaults {
			delete(access.itemsByVaultID, rotatedVault.ID)
		}
	}
	requestList, err := access.unlockedSession.Client.ListVaultKeyRequests(ctx)
	if err != nil {
		return result, err
	}
	if len(requestList.Waiting) == 0 {
		return result, nil
	}
	organizationNamesByID := map[string]string{}
	if organizationList, err := access.unlockedSession.Client.ListOrganizations(ctx); err == nil {
		for _, organization := range organizationList.Organizations {
			organizationNamesByID[organization.ID] = organization.Name
		}
	}
	for _, waiting := range requestList.Waiting {
		result.Waiting = append(result.Waiting, cliprotocol.WaitingVaultKey{
			VaultID: waiting.VaultID, OrganizationID: waiting.OrganizationID, OrganizationName: organizationNamesByID[waiting.OrganizationID], Role: waiting.Role,
		})
	}
	return result, nil
}
