package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
)

// maxAccountQueryLength bounds the search text and the cursor of the account list, in characters.
const maxAccountQueryLength = 320

var (
	errOwnAccount      = newError(KindConflict, "Use Settings → Account in the Hisar app for your own account.")
	errLastServerAdmin = newError(KindConflict, "The Hisar server needs at least one admin.")
)

func describeAdminAccount(account store.AdminAccount) apitypes.AdminAccount {
	state := apitypes.AccountStateActive
	if account.SuspendedAt != nil {
		state = apitypes.AccountStateSuspended
	}
	return apitypes.AdminAccount{
		ID: account.ID, Email: account.Email, Name: account.Name, Role: account.Role, State: state,
		CreatedAt: account.CreatedAt, LastSeenAt: account.LastSeenAt, DeviceCount: account.DeviceCount, VaultCount: account.VaultCount,
	}
}

// ListAdminAccounts returns one page of accounts sorted by email, after the cursor, whose email or name contains query.
// It does not check the role: the route requires an admin.
func (s *Service) ListAdminAccounts(ctx context.Context, query, cursor string) (apitypes.AdminAccountPage, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > maxAccountQueryLength || utf8.RuneCountInString(cursor) > maxAccountQueryLength {
		return apitypes.AdminAccountPage{}, newError(KindInvalid, "the search is too long")
	}
	if !isValidText(query) || !isValidText(cursor) {
		return apitypes.AdminAccountPage{}, newError(KindInvalid, "the search is not valid text")
	}
	accounts, err := s.store.ListAdminAccounts(ctx, query, cursor, apitypes.AdminAccountPageSize+1)
	if err != nil {
		return apitypes.AdminAccountPage{}, err
	}
	page := apitypes.AdminAccountPage{Accounts: make([]apitypes.AdminAccount, 0, min(len(accounts), apitypes.AdminAccountPageSize))}
	for index, account := range accounts {
		if index == apitypes.AdminAccountPageSize {
			page.NextCursor = page.Accounts[index-1].Email
			break
		}
		page.Accounts = append(page.Accounts, describeAdminAccount(account))
	}
	return page, nil
}

// isValidText reports whether Postgres can take text as it is: UTF-8 without a NUL character.
func isValidText(text string) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}

// GetAdminAccount returns the account and every device it signed in with. It does not check the role: the route requires
// an admin.
func (s *Service) GetAdminAccount(ctx context.Context, accountID string) (apitypes.AdminAccountDetail, error) {
	if !crypt.IsValidAccountID(accountID) {
		return apitypes.AdminAccountDetail{}, ErrNotFound
	}
	account, err := s.store.GetAdminAccount(ctx, accountID)
	if errors.Is(err, store.ErrNotFound) {
		return apitypes.AdminAccountDetail{}, ErrNotFound
	}
	if err != nil {
		return apitypes.AdminAccountDetail{}, err
	}
	devices, err := s.store.ListAdminDevices(ctx, accountID)
	if err != nil {
		return apitypes.AdminAccountDetail{}, err
	}
	detail := apitypes.AdminAccountDetail{Account: describeAdminAccount(*account), Devices: make([]apitypes.AdminDevice, 0, len(devices))}
	for _, device := range devices {
		detail.Devices = append(detail.Devices, apitypes.AdminDevice{
			ID: device.ID, Name: device.Name, Platform: device.Platform, CreatedAt: device.CreatedAt, LastSeenAt: device.LastSeenAt, RevokedAt: device.RevokedAt,
		})
	}
	return detail, nil
}

// checkOtherAccount refuses an account ID that cannot exist, and the admin's own account, which the apps manage.
func checkOtherAccount(actor Actor, accountID string) error {
	if !crypt.IsValidAccountID(accountID) {
		return ErrNotFound
	}
	if accountID == actor.AccountID {
		return errOwnAccount
	}
	return nil
}

func convertAdminAccountError(err error) error {
	switch {
	case errors.Is(err, store.ErrDeletionPlanChanged):
		return ErrDeletionPlanChanged
	case errors.Is(err, store.ErrDeletionBusy):
		return errAdminDeletionBusy
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrLastServerAdmin):
		return errLastServerAdmin
	case errors.Is(err, store.ErrActorNotAdmin):
		return ErrAdminOnly
	case errors.Is(err, store.ErrConflict):
		return errCredentialsChanged
	case errors.Is(err, store.ErrSessionEnded):
		return ErrUnauthorized
	}
	return err
}

// provenAdmin is the admin of a change that needed a re-authentication: the store checks inside the change's transaction
// that the session is still live and the OPAQUE record is the one the proof was checked against, as DeleteAccount does.
func provenAdmin(actor Actor, pending *pendingLogin) store.AdminActor {
	return store.AdminActor{AccountID: actor.AccountID, TokenHash: actor.TokenHash, ProvenRecordHash: pending.recordHash}
}

// recordAdminAudit records an admin's change to another account, which detail.account names.
func (s *Service) recordAdminAudit(ctx context.Context, actor Actor, eventType auditType, accountID string, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["account"] = accountID
	s.recordAudit(ctx, actor, eventType, detail)
}

// endAccountSessions closes the account's event streams and drops its phone approvals, after its sessions were deleted.
func (s *Service) endAccountSessions(accountID string) {
	s.events.EndStreams(accountID, nil)
	s.Approvals.removeForAccount(accountID)
}

// SuspendAccount stops another account from signing in and refuses its devices' sessions until reactivation, ending
// its event streams and phone approvals now; its devices, vaults and organizations stay as they are. Suspending a
// suspended account changes nothing.
func (s *Service) SuspendAccount(ctx context.Context, actor Actor, accountID string) error {
	if err := checkOtherAccount(actor, accountID); err != nil {
		return err
	}
	suspended, err := s.store.SuspendAccount(ctx, actor.AccountID, accountID)
	if err != nil || !suspended {
		return convertAdminAccountError(err)
	}
	s.endAccountSessions(accountID)
	s.recordAdminAudit(ctx, actor, auditAdminAccountSuspended, accountID, nil)
	return nil
}

// ReactivateAccount lets a suspended account sign in again; reactivating an active account changes nothing.
func (s *Service) ReactivateAccount(ctx context.Context, actor Actor, accountID string) error {
	if !crypt.IsValidAccountID(accountID) {
		return ErrNotFound
	}
	reactivated, err := s.store.ReactivateAccount(ctx, actor.AccountID, accountID)
	if err != nil || !reactivated {
		return convertAdminAccountError(err)
	}
	s.recordAdminAudit(ctx, actor, auditAdminAccountReactivated, accountID, nil)
	return nil
}

// ChangeAccountRole makes another account an admin or a member, after the admin proves their master password.
func (s *Service) ChangeAccountRole(ctx context.Context, actor Actor, accountID string, request *apitypes.AdminRoleChangeRequest) error {
	if err := checkOtherAccount(actor, accountID); err != nil {
		return err
	}
	if request.Role != apitypes.RoleAdmin && request.Role != apitypes.RoleMember {
		return newError(KindInvalid, "the role must be admin or member")
	}
	pending, err := s.finishReauthentication(ctx, actor, request.Proof.ReauthenticationID, request.Proof.KE3)
	if err != nil {
		return err
	}
	defer pending.erase()
	if err := s.store.SetAccountRole(ctx, provenAdmin(actor, pending), accountID, request.Role); err != nil {
		return convertAdminAccountError(err)
	}
	s.recordAdminAudit(ctx, actor, auditAdminRoleChanged, accountID, map[string]any{"role": request.Role})
	return nil
}

// DeleteAccountAsAdmin deletes another account exactly as the account's own deletion does (§5.7.1), after the admin
// types its email and proves their own master password.
func (s *Service) DeleteAccountAsAdmin(ctx context.Context, actor Actor, accountID string, request *apitypes.AdminAccountDeleteRequest) error {
	if request.PlanVersion == "" {
		return newError(KindInvalid, "Update Hisar to review the account deletion plan before deleting.")
	}
	if err := checkOtherAccount(actor, accountID); err != nil {
		return err
	}
	account, err := s.store.GetAccountByID(ctx, accountID)
	if err != nil {
		return convertAdminAccountError(err)
	}
	if apitypes.NormalizeEmail(request.Email) != account.Email {
		return newError(KindInvalid, "the email does not match this account")
	}
	pending, err := s.finishScopedReauthentication(ctx, actor, request.Proof.ReauthenticationID, request.Proof.KE3, apitypes.ReauthenticationScope{Action: "admin-account-delete", Target: accountID, PlanVersion: request.PlanVersion})
	if err != nil {
		return err
	}
	defer pending.erase()
	deleted, err := s.store.DeleteAccountAsAdmin(ctx, provenAdmin(actor, pending), accountID, request.PlanVersion)
	if err != nil {
		return convertAdminAccountError(err)
	}
	s.recordAdminAudit(ctx, actor, auditAdminAccountDeleted, accountID, map[string]any{"deletedVaults": deleted.DeletedVaultCount})
	s.announceAccountDeletion(accountID, deleted, true)
	return nil
}

// SignOutAccount revokes every device of an account, the admin's own included.
func (s *Service) SignOutAccount(ctx context.Context, actor Actor, accountID string) error {
	if !crypt.IsValidAccountID(accountID) {
		return ErrNotFound
	}
	revokedDeviceCount, err := s.store.SignOutAccount(ctx, actor.AccountID, accountID)
	if err != nil {
		return convertAdminAccountError(err)
	}
	s.endAccountSessions(accountID)
	s.recordAdminAudit(ctx, actor, auditAdminSignedOut, accountID, map[string]any{"signedOutDevices": revokedDeviceCount})
	return nil
}

// RevokeAccountDevice revokes one device of an account and closes its event streams.
func (s *Service) RevokeAccountDevice(ctx context.Context, actor Actor, accountID, deviceID string) error {
	if !crypt.IsValidAccountID(accountID) || !IsValidUUID(deviceID) {
		return ErrNotFound
	}
	endedTokenHashes, err := s.store.RevokeAccountDevice(ctx, actor.AccountID, accountID, deviceID)
	if err != nil {
		return convertAdminAccountError(err)
	}
	s.events.EndSessionStreams(accountID, endedTokenHashes)
	s.recordAdminAudit(ctx, actor, auditAdminDeviceRevoked, accountID, map[string]any{"device": deviceID})
	return nil
}
