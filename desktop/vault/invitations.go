package vault

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/client"
)

// completeAllAcceptedShare wraps vault keys for people who accepted this user's share invitations, pinning their
// signing keys. It returns whether a share was completed; a failure is kept for ListAllShareInvitation to show.
func (s *Service) completeAllAcceptedShare(ctx context.Context) bool {
	allCompleted, completeErr := s.unlockedSession.CompleteAllAcceptedShare(ctx)
	if client.IsUnreachable(completeErr) || client.IsStatus(completeErr, http.StatusNotFound) {
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
		CompletionError: s.shareCompletionError,
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
		result.AllSent = append(result.AllSent, SentShareInvitation{
			ID: sent.ID, VaultID: sent.VaultID, VaultName: vaultName, Email: sent.Email, Role: sent.Role, State: sent.State,
			ExpiresAt: sent.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	return result, nil
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
