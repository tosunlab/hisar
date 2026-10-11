package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

const recoveryPinKey = "recovery"

var (
	errRecoveryAdminOnly   = errors.New("only admins can manage account recovery")
	errRecoverSelf         = errors.New("you cannot recover your own account; ask another admin")
	errRecoveryNotSetUp    = errors.New("account recovery is not set up; ask your admin to set it up in Settings")
	errRecoveryKeyChanged  = errors.New("the recovery key changed since this device last used it; contact your Hisar admin")
	errInvalidRecoveryLink = errors.New("this recovery link is invalid, expired or already used, or it was sent to a different email")
	errSignedInRecovery    = errors.New("sign out of Hisar on this device before recovering an account")
)

func parseRecoveryLink(link string) (server, token string, err error) {
	return parseLinkToken(link, "/recover/", errors.New("paste the full recovery link you received from your admin"))
}

// checkRecoveryPin trusts the recovery KEM key on first use and refuses it if it changes later; the caller saves the state.
func (s *Service) checkRecoveryPin(recoveryKEMPublicKey []byte) error {
	if pinned, found := s.state.AllPin[recoveryPinKey]; found && !bytes.Equal(pinned, recoveryKEMPublicKey) {
		return errRecoveryKeyChanged
	}
	s.state.AllPin[recoveryPinKey] = append([]byte(nil), recoveryKEMPublicKey...)
	return nil
}

func convertRecoveryError(err error) error {
	switch {
	case errors.Is(err, session.ErrNoRecoveryKeyset):
		return errRecoveryNotSetUp
	case errors.Is(err, session.ErrRecoverSelf):
		return errRecoverSelf
	case client.IsStatus(err, http.StatusForbidden):
		return errRecoveryAdminOnly
	}
	return convertError(err)
}

// The role from unlocking can be stale (promoted since, or unlocked offline), so the server decides; this only refreshes it.
func (s *Service) refreshRole(ctx context.Context) {
	if account, err := s.unlockedSession.Client.GetAccount(ctx); err == nil && account.AccountID == s.unlockedSession.AccountID {
		s.unlockedSession.Role = account.Role
	}
}

// GetRecoveryStatus reports whether an admin set up account recovery and whether this account can complete recoveries.
func (s *Service) GetRecoveryStatus() (RecoveryStatus, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return RecoveryStatus{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.getRecoveryStatus(ctx)
}

func (s *Service) getRecoveryStatus(ctx context.Context) (RecoveryStatus, error) {
	s.refreshRole(ctx)
	status := RecoveryStatus{IsAdmin: s.unlockedSession.Role == proto.RoleAdmin}
	recoveryKeyset, err := s.unlockedSession.Client.GetRecoveryKeyset(ctx)
	if client.IsStatus(err, http.StatusNotFound) {
		return status, nil
	}
	if err != nil {
		return RecoveryStatus{}, convertError(err)
	}
	status.IsSetUp = true
	status.IsCreatedByCurrentUser = recoveryKeyset.CreatedBy == s.unlockedSession.AccountID
	if creator := recoveryKeyset.Creator; creator != nil && creator.AccountID == recoveryKeyset.CreatedBy {
		status.CreatedByName = creator.Name
		if status.CreatedByName == "" {
			status.CreatedByName = creator.Email
		}
	}
	return status, nil
}

// SetUpRecovery creates the recovery keyset unless an admin already did, then turns recovery on for the vaults this
// account owns. It returns the names of vaults that could not be turned on.
func (s *Service) SetUpRecovery() ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := s.unlockedSession.CreateRecoveryKeyset(ctx)
	if err != nil && !client.IsStatus(err, http.StatusConflict) {
		return nil, convertRecoveryError(err)
	}
	return s.enableRecoveryForAllOwnedVault(ctx)
}

// EnableRecoveryForAllVault turns recovery on for every vault this account owns, except personal vaults it opted out.
// It returns the names of vaults that could not be turned on.
func (s *Service) EnableRecoveryForAllVault() ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.enableRecoveryForAllOwnedVault(ctx)
}

func (s *Service) enableRecoveryForAllOwnedVault(ctx context.Context) ([]string, error) {
	if _, err := s.unlockedSession.GetRecoveryPublicKey(ctx); err != nil {
		return nil, convertRecoveryError(err)
	}
	if err := s.state.Save(); err != nil {
		return nil, err
	}
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	allFailedVaultName := []string{}
	for _, vault := range s.allVaultByID {
		if vault.Role != proto.VaultOwner || vault.Recoverable || vault.RecoveryOptOut {
			continue
		}
		if err := s.unlockedSession.EnableVaultRecovery(ctx, vault); err != nil {
			allFailedVaultName = append(allFailedVaultName, vault.Name)
		}
	}
	slices.Sort(allFailedVaultName)
	return allFailedVaultName, nil
}

func (s *Service) findOwnedVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	vault, found := s.allVaultByID[vaultID]
	if !found {
		return nil, fmt.Errorf("vault %w", ErrNotFound)
	}
	if vault.Role != proto.VaultOwner {
		return nil, errors.New("only owners of this vault can change its recovery")
	}
	return vault, nil
}

func (s *Service) countCachedItem(vaultID string) int {
	return len(s.allItemByVaultID[vaultID])
}

// EnableVaultRecovery lets the recovery admin recover this vault again, also undoing an opt-out.
func (s *Service) EnableVaultRecovery(vaultID string) (VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSummary{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findOwnedVault(ctx, vaultID)
	if err != nil {
		return VaultSummary{}, err
	}
	enableErr := s.unlockedSession.EnableVaultRecovery(ctx, vault)
	if err := s.state.Save(); enableErr == nil && err != nil {
		return VaultSummary{}, err
	}
	if enableErr != nil {
		return VaultSummary{}, convertRecoveryError(enableErr)
	}
	return convertToVaultSummary(vault, s.countCachedItem(vaultID)), nil
}

// DisableVaultRecovery opts a personal vault out of recovery: if the owner loses access, nobody can recover it.
func (s *Service) DisableVaultRecovery(vaultID string) (VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSummary{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findOwnedVault(ctx, vaultID)
	if err != nil {
		return VaultSummary{}, err
	}
	if vault.Type != proto.VaultPersonal {
		return VaultSummary{}, errors.New("shared vaults always stay recoverable, so their members can be recovered")
	}
	if err := s.unlockedSession.DisableVaultRecovery(ctx, vault); err != nil {
		return VaultSummary{}, convertError(err)
	}
	return convertToVaultSummary(vault, s.countCachedItem(vaultID)), nil
}

// StartAccountRecovery creates a one-time recovery link for someone, valid for 7 days.
func (s *Service) StartAccountRecovery(accountID string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return "", err
	}
	if accountID == s.unlockedSession.AccountID {
		return "", errRecoverSelf
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := s.unlockedSession.Client.CreateRecoveryLink(ctx, accountID)
	if client.IsStatus(err, http.StatusNotFound) {
		return "", errors.New("this person no longer has an account on the server")
	}
	if err != nil {
		return "", convertRecoveryError(err)
	}
	return response.URL, nil
}

// ListAllPendingRecovery returns the submitted recoveries, oldest first.
func (s *Service) ListAllPendingRecovery() ([]PendingRecovery, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allProtoPending, err := s.unlockedSession.ListAllPendingRecovery(ctx)
	if err != nil {
		return nil, convertRecoveryError(err)
	}
	slices.SortStableFunc(allProtoPending, func(first, second proto.PendingRecovery) int {
		return first.SubmittedAt.Compare(second.SubmittedAt)
	})
	allPending := make([]PendingRecovery, 0, len(allProtoPending))
	for _, protoPending := range allProtoPending {
		pending := PendingRecovery{
			AccountID:   protoPending.AccountID,
			Name:        protoPending.Name,
			Email:       protoPending.Email,
			SubmittedAt: protoPending.SubmittedAt.UTC().Format(time.RFC3339),
		}
		for _, recoveryVault := range protoPending.AllVault {
			if recoveryVault.WrappedKey != nil {
				pending.RecoverableVaultCount++
			} else {
				pending.UnrecoverableVaultCount++
			}
		}
		allPending = append(allPending, pending)
	}
	return allPending, nil
}

// CompleteAccountRecovery gives the member their recoverable vaults under their new keys, pins their new signing key
// and returns the names of the vaults they lost.
func (s *Service) CompleteAccountRecovery(accountID string) ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if accountID == s.unlockedSession.AccountID {
		return nil, errRecoverSelf
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	allProtoPending, err := s.unlockedSession.ListAllPendingRecovery(ctx)
	if err != nil {
		return nil, convertRecoveryError(err)
	}
	index := slices.IndexFunc(allProtoPending, func(pending proto.PendingRecovery) bool { return pending.AccountID == accountID })
	if index < 0 {
		return nil, errors.New("this recovery is no longer pending; reload the list")
	}
	pending := allProtoPending[index]
	if err := pending.PublicKeys.Verify(accountID); err != nil {
		return nil, fmt.Errorf("the new keys of %s are not valid; start the recovery again", pending.Email)
	}

	allUnrecoverableVaultID, err := s.unlockedSession.CompleteRecovery(ctx, accountID)
	if err != nil {
		return nil, convertRecoveryError(err)
	}
	allMember, err := s.unlockedSession.Client.ListAllMember(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	memberIndex := slices.IndexFunc(allMember, func(member proto.Member) bool { return member.AccountID == accountID })
	if memberIndex >= 0 && !bytes.Equal(allMember[memberIndex].PublicKeys.Sign, pending.PublicKeys.Sign) {
		return nil, fmt.Errorf("the server returned different keys for %s than the ones they submitted; contact your Hisar admin", pending.Email)
	}
	s.state.AllPin[accountID] = append([]byte(nil), pending.PublicKeys.Sign...)
	if err := s.state.Save(); err != nil {
		return nil, err
	}
	return s.getAllUnrecoverableVaultName(pending, allUnrecoverableVaultID), nil
}

// getAllUnrecoverableVaultName names the vaults the admin can open and describes the others.
func (s *Service) getAllUnrecoverableVaultName(pending proto.PendingRecovery, allUnrecoverableVaultID []string) []string {
	allVaultTypeByID := map[string]string{}
	for _, recoveryVault := range pending.AllVault {
		allVaultTypeByID[recoveryVault.VaultID] = recoveryVault.Type
	}
	firstName := strings.Fields(pending.Name + " " + pending.Email)[0]
	allVaultName := make([]string, 0, len(allUnrecoverableVaultID))
	for _, vaultID := range allUnrecoverableVaultID {
		switch vault, found := s.allVaultByID[vaultID]; {
		case found:
			allVaultName = append(allVaultName, vault.Name)
		case allVaultTypeByID[vaultID] == proto.VaultPersonal:
			allVaultName = append(allVaultName, fmt.Sprintf("%s's personal vault", firstName))
		default:
			allVaultName = append(allVaultName, fmt.Sprintf("A shared vault you are not a member of (%s)", vaultID))
		}
	}
	return allVaultName
}

// RecoverAccount submits a new Secret Key and master password with the recovery link an admin sent. The account keeps
// its old credentials until the admin completes the recovery; then the user signs in with the returned Secret Key.
func (s *Service) RecoverAccount(recoveryLink, email, newPassword string) (RecoverAccountResult, error) {
	server, token, err := parseRecoveryLink(recoveryLink)
	if err != nil {
		return RecoverAccountResult{}, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return RecoverAccountResult{}, errors.New("enter the email address of your Hisar account")
	}
	if len(newPassword) < 10 {
		return RecoverAccountResult{}, errors.New("use a master password with at least 10 characters")
	}
	s.mutex.Lock()
	isSignedIn := s.state.IsLoggedIn()
	s.mutex.Unlock()
	if isSignedIn {
		return RecoverAccountResult{}, errSignedInRecovery
	}
	passwordBytes := []byte(newPassword)
	defer clear(passwordBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	secretKey, err := session.StartRecovery(ctx, client.Create(server), token, email, passwordBytes, s.getDeviceInfo())
	if client.IsStatus(err, http.StatusForbidden) {
		return RecoverAccountResult{}, errInvalidRecoveryLink
	}
	if err != nil {
		return RecoverAccountResult{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.state.IsLoggedIn() {
		return RecoverAccountResult{}, errSignedInRecovery
	}
	s.name = ""
	s.state.Reset()
	s.state.Server, s.state.Email = server, email
	s.recoveredAccountSecretKey = &secretKey
	result := RecoverAccountResult{Status: s.buildStatus(), SecretKey: secretKey.Encode()}
	if err := s.state.Save(); err != nil {
		return result, fmt.Errorf("recovery submitted, but this device could not save the server address (%v); write down your new Secret Key: %s", err, result.SecretKey)
	}
	return result, nil
}
