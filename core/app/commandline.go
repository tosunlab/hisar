package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
)

// CommandLineBridge answers the hisar CLI on the app's socket (clisocket.Handler); it is not bound to the UI.
type CommandLineBridge struct {
	service *Service
}

// NewCommandLineBridge also ends the CLI approvals whenever Hisar locks or signs out.
func NewCommandLineBridge(service *Service, approvals *clisocket.SessionApprovals) *CommandLineBridge {
	service.mutex.Lock()
	service.lockListeners = append(service.lockListeners, approvals.ClearAll)
	service.mutex.Unlock()
	return &CommandLineBridge{service: service}
}

func (bridge *CommandLineBridge) GetHello() cliprotocol.HelloResult {
	status := bridge.service.GetStatus()
	return cliprotocol.HelloResult{State: status.State, Email: status.Email, Server: status.Server}
}

// CheckUpdateRequired refuses requests while Hisar needs an update, or while the hisar that sent them is below the server's minimum.
func (bridge *CommandLineBridge) CheckUpdateRequired(request *cliprotocol.Request) error {
	s := bridge.service
	if requirement := s.getUpdateRequirement(); requirement != nil {
		return cliprotocol.NewError(cliprotocol.ErrorUpdateRequired, fmt.Sprintf(
			"Hisar needs an update: the server requires version %s or later and this Mac has %s. Open Hisar and choose Update now.",
			requirement.MinimumVersion, describeVersion(requirement.CurrentVersion)))
	}
	if !strings.HasSuffix(request.ClientPlatform, "/cli") {
		return nil
	}
	if requirement := s.checkClientVersion(request.ClientPlatform, request.ClientVersion); requirement != nil {
		return cliprotocol.NewError(cliprotocol.ErrorUpdateRequired, fmt.Sprintf(
			"hisar needs an update: the server requires version %s or later and this is %s. Install the latest hisar from %s, or use the one that comes with the Hisar app.",
			requirement.MinimumVersion, requirement.CurrentVersion, requirement.UpdateURL))
	}
	return nil
}

func describeVersion(version string) string {
	if version == "" {
		return "a development build"
	}
	return version
}

func convertCommandLineError(err error) error {
	var responseError *cliprotocol.ResponseError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &responseError):
		return err
	case errors.Is(err, ErrLocked):
		return cliprotocol.NewError(cliprotocol.ErrorLocked, "Hisar is locked; run the command again to unlock it")
	case errors.Is(err, ErrSignedOut):
		return cliprotocol.NewError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there, or run hisar --standalone")
	case errors.Is(err, ErrOffline):
		return cliprotocol.NewError(cliprotocol.ErrorOffline, err.Error())
	case errors.Is(err, ErrNotFound):
		return cliprotocol.NewError(cliprotocol.ErrorNotFound, err.Error())
	}
	return cliprotocol.NewError(cliprotocol.ErrorInternal, err.Error())
}

func (bridge *CommandLineBridge) HandleRequest(_ context.Context, request *cliprotocol.Request) (any, error) {
	result, err := bridge.answer(request)
	return result, convertCommandLineError(err)
}

func (bridge *CommandLineBridge) answer(request *cliprotocol.Request) (any, error) {
	if cliprotocol.IsAgentRequest(request.Type) {
		return bridge.answerAgentRequest(request)
	}
	s := bridge.service
	switch request.Type {
	case cliprotocol.TypeAccount:
		return s.getCommandLineAccount()
	case cliprotocol.TypeListVault:
		summaries, err := s.ListVaults()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{}}
		for _, summary := range summaries {
			result.Vaults = append(result.Vaults, convertToCommandLineVault(summary))
		}
		return result, nil
	case cliprotocol.TypeCreateVault:
		summary, err := s.CreateVault(request.Name)
		return cliprotocol.VaultResult{Vault: convertToCommandLineVault(summary)}, err
	case cliprotocol.TypeGetVaultActions:
		return s.getCommandLineVaultActions(request.VaultID)
	case cliprotocol.TypeRenameVault:
		summary, err := s.RenameVault(request.VaultID, request.Name)
		return cliprotocol.VaultResult{Vault: convertToCommandLineVault(summary)}, err
	case cliprotocol.TypeDeleteVault:
		if request.VaultDeletion == nil {
			return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "say what happens to the vault's items")
		}
		result, err := s.DeleteVault(request.VaultID, VaultDeletionRequest{
			MoveToVaultID: request.VaultDeletion.MoveToVaultID, IsDeletingItems: request.VaultDeletion.IsDeletingItems, ConfirmedName: request.VaultDeletion.ConfirmedName,
		})
		return cliprotocol.VaultDeletionResult{MovedCount: result.MovedCount}, err
	case cliprotocol.TypeLeaveVault:
		return nil, s.LeaveVault(request.VaultID)
	case cliprotocol.TypeExport:
		if request.Export == nil {
			return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the export is missing")
		}
		result, err := s.writeExportFile(request.Export.Format, request.Export.Passphrase, request.Export.Path)
		return cliprotocol.ExportResult{Path: result.Path, VaultCount: result.VaultCount, ItemCount: result.ItemCount}, err
	case cliprotocol.TypeListItem:
		details, err := s.ListItems(request.VaultID)
		if err != nil {
			return nil, err
		}
		result := cliprotocol.ItemListResult{Items: []cliprotocol.Item{}}
		for _, detail := range details {
			result.Items = append(result.Items, convertToCommandLineSummary(detail))
		}
		return result, nil
	case cliprotocol.TypeGetItem:
		detail, err := bridge.getItem(request.VaultID, request.ItemID)
		if err != nil {
			return nil, err
		}
		return cliprotocol.ItemResult{Item: convertToCommandLineItem(detail)}, nil
	case cliprotocol.TypeGetCode:
		if _, err := bridge.getItem(request.VaultID, request.ItemID); err != nil {
			return nil, err
		}
		code, err := s.GetOneTimeCode(request.VaultID, request.ItemID)
		return cliprotocol.CodeResult{Code: code.Code, ExpiresAt: code.ExpiresAt, Period: code.Period}, err
	case cliprotocol.TypeSaveItem:
		return bridge.saveItem(request.VaultID, request.Item)
	case cliprotocol.TypeDeleteItem:
		if _, err := bridge.getItem(request.VaultID, request.ItemID); err != nil {
			return nil, err
		}
		return nil, s.DeleteItem(request.VaultID, request.ItemID)
	case cliprotocol.TypeImportItem:
		return s.saveCommandLineImports(request.VaultID, request.ImportedItems)
	case cliprotocol.TypeListPeople:
		people, err := s.ListPeople()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.PeopleListResult{People: []cliprotocol.Person{}}
		for _, person := range people {
			result.People = append(result.People, cliprotocol.Person{
				AccountID: person.AccountID, Name: person.Name, Email: person.Email, Role: person.Role,
				SecurityCode: person.SecurityCode, IsCurrentUser: person.IsCurrentUser, IsKeyChanged: person.IsKeyChanged,
			})
		}
		return result, nil
	case cliprotocol.TypeShareVault:
		return nil, s.ShareVault(request.VaultID, request.Email, request.Role)
	case cliprotocol.TypeUnshareVault:
		return nil, s.RemoveVaultMember(request.VaultID, request.AccountID)
	case cliprotocol.TypeInvite:
		return nil, s.InvitePerson(request.Email)
	case cliprotocol.TypeListInvitation:
		invitations, err := s.ListShareInvitations()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.InvitationListResult{Received: []cliprotocol.ReceivedInvitation{}, Sent: []cliprotocol.SentInvitation{}, CompletionError: invitations.CompletionError}
		for _, received := range invitations.Received {
			result.Received = append(result.Received, cliprotocol.ReceivedInvitation{
				ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail, ExpiresAt: received.ExpiresAt,
			})
		}
		for _, sent := range invitations.Sent {
			result.Sent = append(result.Sent, cliprotocol.SentInvitation{
				ID: sent.ID, VaultID: sent.VaultID, VaultName: sent.VaultName, Email: sent.Email, Role: sent.Role, State: sent.State, ExpiresAt: sent.ExpiresAt,
			})
		}
		return result, nil
	case cliprotocol.TypeAcceptInvitation:
		return nil, s.AcceptShareInvitation(request.InvitationID)
	case cliprotocol.TypeDeclineInvitation:
		return nil, s.DeclineShareInvitation(request.InvitationID)
	case cliprotocol.TypeCancelInvitation:
		return nil, s.CancelShareInvitation(request.InvitationID)
	case cliprotocol.TypeListDevice:
		devices, err := s.ListDevices()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.DeviceListResult{Devices: []cliprotocol.Device{}}
		for _, device := range devices {
			result.Devices = append(result.Devices, cliprotocol.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, LastSeenAt: device.LastSeenAt, IsCurrent: device.IsCurrent})
		}
		return result, nil
	case cliprotocol.TypeRevokeDevice:
		return nil, s.RevokeDevice(request.DeviceID)
	case cliprotocol.TypeResolveSecret:
		return s.resolveSecrets(request.References)
	case cliprotocol.TypeCreateShareLink:
		return bridge.createShareLink(request)
	case cliprotocol.TypeListShareLink:
		return bridge.listShareLinks()
	case cliprotocol.TypeRevokeShareLink:
		return nil, s.RevokeShareLink(request.ShareLinkID)
	}
	return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
}

func (s *Service) getCommandLineAccount() (cliprotocol.AccountResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return cliprotocol.AccountResult{}, err
	}
	return cliprotocol.AccountResult{
		AccountID: s.state.AccountID, Name: s.name, Email: s.state.Email, Role: s.unlockedSession.Role, Server: s.state.Server,
	}, nil
}

// getItem loads the vault first when the item is not cached yet, e.g. after the UI showed another vault.
func (bridge *CommandLineBridge) getItem(vaultID, itemID string) (ItemDetail, error) {
	detail, err := bridge.service.GetItem(vaultID, itemID)
	if !errors.Is(err, ErrNotFound) {
		return detail, err
	}
	if _, err := bridge.service.ListItems(vaultID); err != nil {
		return ItemDetail{}, err
	}
	return bridge.service.GetItem(vaultID, itemID)
}

func (bridge *CommandLineBridge) saveItem(vaultID string, change *cliprotocol.ItemChange) (any, error) {
	if change == nil {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the item is missing")
	}
	detail := ItemDetail{URLs: []string{}, Fields: []ItemField{}}
	if change.ID != "" {
		existing, err := bridge.getItem(vaultID, change.ID)
		if err != nil {
			return nil, err
		}
		detail = existing
		if change.Revision != "" {
			detail.Revision = change.Revision
		}
	}
	detail.Title, detail.Username, detail.URLs, detail.Password, detail.Notes = change.Title, change.Username, change.URLs, change.Password, change.Notes
	if change.Fields != nil {
		detail.Fields = []ItemField{}
		for _, field := range *change.Fields {
			detail.Fields = append(detail.Fields, ItemField{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
		}
	}
	saved, err := bridge.service.SaveItem(vaultID, detail)
	if err != nil {
		return nil, err
	}
	return cliprotocol.ItemResult{Item: convertToCommandLineItem(saved)}, nil
}

// saveCommandLineImports only creates items: IDs and revisions from the CLI are dropped.
func (s *Service) saveCommandLineImports(vaultID string, importedItems []item.Item) (cliprotocol.ImportResult, error) {
	result := cliprotocol.ImportResult{Failures: []cliprotocol.ImportFailure{}}
	if len(importedItems) > cliprotocol.MaxImportBatch {
		return result, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send at most %d items at a time", cliprotocol.MaxImportBatch))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return result, err
	}
	if vault.Role == apitypes.VaultReader {
		return result, errReadOnlyVault
	}
	if s.itemsByVaultID[vault.ID] == nil {
		existingItems, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.itemsByVaultID[vault.ID] = existingItems
	}
	for _, vaultItem := range importedItems {
		vaultItem.ID, vaultItem.Revision = "", ""
		_, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem)
		if isImportStoppingError(err) {
			return result, convertError(err)
		}
		if err != nil {
			result.Failures = append(result.Failures, cliprotocol.ImportFailure{Title: vaultItem.Overview.Title, Message: convertError(err).Error()})
			continue
		}
		result.SavedCount++
		s.replaceCachedItem(vault.ID, vaultItem)
	}
	return result, nil
}

func (s *Service) getCommandLineVaultActions(vaultID string) (cliprotocol.VaultActionsResult, error) {
	actions, err := s.GetVaultActions(vaultID)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	result := cliprotocol.VaultActionsResult{
		Vault: convertToCommandLineVault(actions.Vault), CanRename: actions.CanRename,
		CanDelete: actions.CanDelete, DeleteBlockedReason: actions.DeleteBlockedReason,
		CanLeave: actions.CanLeave, LeaveBlockedReason: actions.LeaveBlockedReason,
		MembersLosingAccess: []cliprotocol.VaultMember{}, TargetVaults: []cliprotocol.Vault{},
	}
	for _, member := range actions.MembersLosingAccess {
		result.MembersLosingAccess = append(result.MembersLosingAccess, cliprotocol.VaultMember{Name: member.Name, Email: member.Email, Role: member.Role})
	}
	for _, target := range actions.TargetVaults {
		result.TargetVaults = append(result.TargetVaults, convertToCommandLineVault(target))
	}
	return result, nil
}

func convertToCommandLineVault(summary VaultSummary) cliprotocol.Vault {
	return cliprotocol.Vault{ID: summary.ID, Name: summary.Name, Type: summary.Type, Role: summary.Role, ItemCount: summary.ItemCount}
}

func convertToCommandLineSummary(detail ItemDetail) cliprotocol.Item {
	return cliprotocol.Item{
		ID: detail.ID, Revision: detail.Revision, Category: detail.Category, Title: detail.Title, Username: detail.Username,
		URLs: append([]string{}, detail.URLs...), HasOneTimePassword: detail.HasOneTimePassword,
	}
}

// convertToCommandLineItem leaves out the one-time password secret; the CLI asks items.code for the code.
func convertToCommandLineItem(detail ItemDetail) cliprotocol.Item {
	commandLineItem := convertToCommandLineSummary(detail)
	commandLineItem.Password, commandLineItem.Notes = detail.Password, detail.Notes
	commandLineItem.PublicKey, commandLineItem.Fingerprint = detail.PublicKey, detail.Fingerprint
	for _, field := range detail.Fields {
		commandLineItem.Fields = append(commandLineItem.Fields, cliprotocol.Field{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	for _, passkey := range detail.Passkeys {
		commandLineItem.Passkeys = append(commandLineItem.Passkeys, cliprotocol.Passkey{RelyingPartyID: passkey.RelyingPartyID, UserName: passkey.UserName})
	}
	return commandLineItem
}
