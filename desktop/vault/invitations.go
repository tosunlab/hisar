package vault

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

// completeAllAcceptedShare wraps vault keys for people who accepted this user's share invitations, pinning their
// signing keys. It returns whether a share was completed; a failure is kept for ListAllShareInvitation to show.
func (s *Service) completeAllAcceptedShare(ctx context.Context) bool {
	allCompleted, completeErr := s.unlockedSession.CompleteAllAcceptedShare(ctx)
	if apiclient.IsUnreachable(completeErr) || apiclient.IsStatus(completeErr, http.StatusNotFound) {
		return false
	}
	s.shareCompletionError = ""
	if completeErr != nil {
		s.shareCompletionError = completeErr.Error()
	}
	if len(allCompleted) > 0 {
		s.state.Save()
		s.loadAllVault(ctx)
	}
	return len(allCompleted) > 0
}

// ListAllShareInvitation completes accepted shares first, then returns the invitations this user received and sent.
func (s *Service) ListAllShareInvitation() (ShareInvitations, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ShareInvitations{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s.completeAllAcceptedShare(ctx)
	invitationList, err := s.unlockedSession.Client.ListAllShareInvitation(ctx)
	if err != nil {
		return ShareInvitations{}, convertError(err)
	}
	result := ShareInvitations{
		AllReceived:     make([]ReceivedShareInvitation, 0, len(invitationList.AllReceived)),
		AllSent:         make([]SentShareInvitation, 0, len(invitationList.AllSent)),
		AllWaiting:      make([]WaitingShareInvitation, 0, len(invitationList.AllWaiting)),
		CompletionError: s.shareCompletionError,
	}
	for _, waiting := range invitationList.AllWaiting {
		result.AllWaiting = append(result.AllWaiting, WaitingShareInvitation{
			ID: waiting.ID, VaultID: waiting.VaultID, Role: waiting.Role, InvitedByName: waiting.InvitedByName, InvitedByEmail: waiting.InvitedByEmail,
			AcceptedAt: formatTime(waiting.AcceptedAt),
		})
	}
	for _, received := range invitationList.AllReceived {
		result.AllReceived = append(result.AllReceived, ReceivedShareInvitation{
			ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail,
			ExpiresAt: received.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	for _, sent := range invitationList.AllSent {
		vaultName := ""
		if vault, found := s.allVaultByID[sent.VaultID]; found {
			vaultName = vault.Name
		}
		result.AllSent = append(result.AllSent, convertToSentShareInvitation(sent, vaultName))
	}
	return result, nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339) }

func convertToSentShareInvitation(sent apitypes.SentShareInvitation, vaultName string) SentShareInvitation {
	invitation := SentShareInvitation{
		ID: sent.ID, VaultID: sent.VaultID, VaultName: vaultName, Email: sent.Email, Role: sent.Role, State: sent.State,
		ExpiresAt: formatTime(sent.ExpiresAt),
	}
	if sent.Recipient != nil {
		invitation.RecipientName = sent.Recipient.Name
	}
	if sent.AcceptedAt != nil {
		invitation.AcceptedAt = formatTime(*sent.AcceptedAt)
	}
	return invitation
}

// ListAllShareAwaitingUnlock lists, while locked, the shares people accepted that this app finishes once unlocked.
// It needs only the saved session; unlocked, signed out or offline, the list is empty.
func (s *Service) ListAllShareAwaitingUnlock() ([]ShareAwaitingUnlock, error) {
	s.mutex.Lock()
	isLocked := s.unlockedSession == nil && s.state.IsLoggedIn()
	server, token := s.state.Server, s.state.Token
	s.mutex.Unlock()
	allAwaiting := []ShareAwaitingUnlock{}
	if !isLocked {
		return allAwaiting, nil
	}
	apiClient := s.createClient(server)
	apiClient.Token = token
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	invitationList, err := apiClient.ListAllShareInvitation(ctx)
	if apiclient.IsUnreachable(err) {
		return allAwaiting, nil
	}
	if err != nil {
		return allAwaiting, convertError(err)
	}
	for _, sent := range invitationList.AllSent {
		if sent.State != apitypes.ShareInvitationAccepted || sent.Recipient == nil {
			continue
		}
		awaiting := ShareAwaitingUnlock{InvitationID: sent.ID, RecipientName: sent.Recipient.Name, RecipientEmail: sent.Recipient.Email}
		if sent.AcceptedAt != nil {
			awaiting.AcceptedAt = formatTime(*sent.AcceptedAt)
		}
		allAwaiting = append(allAwaiting, awaiting)
	}
	return allAwaiting, nil
}

func (s *Service) answerShareInvitation(invitationID string, send func(context.Context, string) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if strings.TrimSpace(invitationID) == "" {
		return errInvitationNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return convertInvitationError(send(ctx, invitationID))
}

// AcceptShareInvitation tells the owner's app it may share the vault; nothing secret is sent.
func (s *Service) AcceptShareInvitation(invitationID string) error {
	return s.answerShareInvitation(invitationID, func(ctx context.Context, invitationID string) error {
		return s.unlockedSession.Client.AcceptShareInvitation(ctx, invitationID)
	})
}

func (s *Service) DeclineShareInvitation(invitationID string) error {
	return s.answerShareInvitation(invitationID, func(ctx context.Context, invitationID string) error {
		return s.unlockedSession.Client.DeclineShareInvitation(ctx, invitationID)
	})
}

// CancelShareInvitation withdraws an invitation to a vault this user owns.
func (s *Service) CancelShareInvitation(invitationID string) error {
	return s.answerShareInvitation(invitationID, func(ctx context.Context, invitationID string) error {
		return s.unlockedSession.Client.CancelShareInvitation(ctx, invitationID)
	})
}
