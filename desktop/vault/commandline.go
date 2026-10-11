package vault

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/desktop/commandline"
)

// CommandLineBridge answers the hisar CLI on the app's socket (commandline.Handler); it is not bound to the UI.
type CommandLineBridge struct {
	service *Service
}

// CreateCommandLineBridge also ends the CLI approvals whenever Hisar locks or signs out.
func CreateCommandLineBridge(service *Service, approvals *commandline.SessionApprovals) *CommandLineBridge {
	service.mutex.Lock()
	service.allLockListener = append(service.allLockListener, approvals.ClearAll)
	service.mutex.Unlock()
	return &CommandLineBridge{service: service}
}

func (bridge *CommandLineBridge) GetHello() cliprotocol.HelloResult {
	status := bridge.service.GetStatus()
	return cliprotocol.HelloResult{State: status.State, Email: status.Email, Server: status.Server}
}

func convertCommandLineError(err error) error {
	var responseError *cliprotocol.ResponseError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &responseError):
		return err
	case errors.Is(err, ErrLocked):
		return cliprotocol.CreateError(cliprotocol.ErrorLocked, "Hisar is locked; run the command again to unlock it")
	case errors.Is(err, ErrSignedOut):
		return cliprotocol.CreateError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there, or run hisar --standalone")
	case errors.Is(err, ErrOffline):
		return cliprotocol.CreateError(cliprotocol.ErrorOffline, err.Error())
	case errors.Is(err, ErrNotFound):
		return cliprotocol.CreateError(cliprotocol.ErrorNotFound, err.Error())
	}
	return cliprotocol.CreateError(cliprotocol.ErrorInternal, err.Error())
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
		allSummary, err := s.ListAllVault()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{}}
		for _, summary := range allSummary {
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
			return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "say what happens to the vault's items")
		}
		result, err := s.DeleteVault(request.VaultID, VaultDeletionRequest{
			MoveToVaultID: request.VaultDeletion.MoveToVaultID, IsDeletingItems: request.VaultDeletion.IsDeletingItems, ConfirmedName: request.VaultDeletion.ConfirmedName,
		})
		return cliprotocol.VaultDeletionResult{MovedCount: result.MovedCount}, err
	case cliprotocol.TypeLeaveVault:
		return nil, s.LeaveVault(request.VaultID)
	case cliprotocol.TypeExport:
		if request.Export == nil {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "the export is missing")
		}
		result, err := s.writeExportFile(request.Export.Format, request.Export.Passphrase, request.Export.Path)
		return cliprotocol.ExportResult{Path: result.Path, VaultCount: result.VaultCount, ItemCount: result.ItemCount}, err
	case cliprotocol.TypeListItem:
		allDetail, err := s.ListAllItem(request.VaultID)
		if err != nil {
			return nil, err
		}
		result := cliprotocol.ItemListResult{Items: []cliprotocol.Item{}}
		for _, detail := range allDetail {
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
		return s.saveAllCommandLineImport(request.VaultID, request.ImportedItems)
	case cliprotocol.TypeListPeople:
		allPerson, err := s.ListAllPerson()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.PeopleListResult{People: []cliprotocol.Person{}}
		for _, person := range allPerson {
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
		invitations, err := s.ListAllShareInvitation()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.InvitationListResult{Received: []cliprotocol.ReceivedInvitation{}, Sent: []cliprotocol.SentInvitation{}, CompletionError: invitations.CompletionError}
		for _, received := range invitations.AllReceived {
			result.Received = append(result.Received, cliprotocol.ReceivedInvitation{
				ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail, ExpiresAt: received.ExpiresAt,
			})
		}
		for _, sent := range invitations.AllSent {
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
		allDevice, err := s.ListAllDevice()
		if err != nil {
			return nil, err
		}
		result := cliprotocol.DeviceListResult{Devices: []cliprotocol.Device{}}
		for _, device := range allDevice {
			result.Devices = append(result.Devices, cliprotocol.Device{ID: device.ID, Name: device.Name, Platform: device.Platform, LastSeenAt: device.LastSeenAt, IsCurrent: device.IsCurrent})
		}
		return result, nil
	case cliprotocol.TypeRevokeDevice:
		return nil, s.RevokeDevice(request.DeviceID)
	case cliprotocol.TypeResolveSecret:
		return s.resolveAllSecret(request.References)
	case cliprotocol.TypeCreateShareLink:
		return bridge.createShareLink(request)
	case cliprotocol.TypeListShareLink:
		return bridge.listAllShareLink()
	case cliprotocol.TypeRevokeShareLink:
		return nil, s.RevokeShareLink(request.ShareLinkID)
	}
	return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
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
	if _, err := bridge.service.ListAllItem(vaultID); err != nil {
		return ItemDetail{}, err
	}
	return bridge.service.GetItem(vaultID, itemID)
}

func (bridge *CommandLineBridge) saveItem(vaultID string, change *cliprotocol.ItemChange) (any, error) {
	if change == nil {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "the item is missing")
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

// saveAllCommandLineImport only creates items: IDs and revisions from the CLI are dropped.
func (s *Service) saveAllCommandLineImport(vaultID string, allImportedItem []item.Item) (cliprotocol.ImportResult, error) {
	result := cliprotocol.ImportResult{Failures: []cliprotocol.ImportFailure{}}
	if len(allImportedItem) > cliprotocol.MaxImportBatch {
		return result, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send at most %d items at a time", cliprotocol.MaxImportBatch))
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
	if vault.Role == proto.VaultReader {
		return result, errReadOnlyVault
	}
	if s.allItemByVaultID[vault.ID] == nil {
		allExisting, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.allItemByVaultID[vault.ID] = allExisting
	}
	for _, vaultItem := range allImportedItem {
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
		AllMemberLosingAccess: []cliprotocol.VaultMember{}, AllTargetVault: []cliprotocol.Vault{},
	}
	for _, member := range actions.AllMemberLosingAccess {
		result.AllMemberLosingAccess = append(result.AllMemberLosingAccess, cliprotocol.VaultMember{Name: member.Name, Email: member.Email, Role: member.Role})
	}
	for _, target := range actions.AllTargetVault {
		result.AllTargetVault = append(result.AllTargetVault, convertToCommandLineVault(target))
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
	for _, passkey := range detail.AllPasskey {
		commandLineItem.Passkeys = append(commandLineItem.Passkeys, cliprotocol.Passkey{RelyingPartyID: passkey.RelyingPartyID, UserName: passkey.UserName})
	}
	return commandLineItem
}

// ApprovalUnlocker lets the approval window unlock Hisar, or check the master password, for an SSH or command-line request (agent.Unlocker).
type ApprovalUnlocker struct {
	service  *Service
	onUnlock func()
}

// CreateApprovalUnlocker calls onUnlock after an unlock from the approval window, so the main window can follow.
func CreateApprovalUnlocker(service *Service, onUnlock func()) *ApprovalUnlocker {
	return &ApprovalUnlocker{service: service, onUnlock: onUnlock}
}

func (unlocker *ApprovalUnlocker) IsLocked() bool {
	return unlocker.service.GetStatus().State == StateLocked
}

func (unlocker *ApprovalUnlocker) IsTouchIDUnlockReady() bool {
	s := unlocker.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.isBiometricReady()
}

func (unlocker *ApprovalUnlocker) notifyUnlock() {
	if unlocker.onUnlock != nil {
		unlocker.onUnlock()
	}
}

func (unlocker *ApprovalUnlocker) UnlockWithTouchID(authenticate func(reason string) error) error {
	if _, err := unlocker.service.unlockWithBiometric(func() error { return authenticate("unlock Hisar") }); err != nil {
		return err
	}
	unlocker.notifyUnlock()
	return nil
}

var errIncorrectMasterPassword = errors.New("Incorrect master password.")

// VerifyPassword checks the master password while Hisar is unlocked, without locking or unlocking it.
func (unlocker *ApprovalUnlocker) VerifyPassword(password string) error {
	s := unlocker.service
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return err
	}
	apiClient, accountCache, accountID := s.unlockedSession.Client, s.unlockedSession.Cache, s.state.AccountID
	s.mutex.Unlock()
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accountUnlockKey, err := session.DeriveAccountUnlockKey(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if errors.Is(err, session.ErrBadCredentials) {
		return errIncorrectMasterPassword
	}
	if err != nil {
		return convertError(err)
	}
	clear(accountUnlockKey)
	return nil
}

func (unlocker *ApprovalUnlocker) UnlockWithPassword(password string) error {
	_, err := unlocker.service.Unlock(password)
	if err != nil && err.Error() == convertError(session.ErrBadCredentials).Error() {
		return errIncorrectMasterPassword
	}
	if err != nil {
		return err
	}
	unlocker.notifyUnlock()
	return nil
}
