package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

// completeAcceptedShares wraps vault keys for people who accepted this user's share invitations, pinning their
// signing keys. It returns whether a share was completed. A failure is kept for ListShareInvitations to show; an
// unreachable server, or one without share invitations, keeps the previous one unless the pins could not be saved.
func (s *Service) completeAcceptedShares(ctx context.Context) bool {
	pinCount := len(s.state.Pins)
	completedShares, completeErr := s.unlockedSession.CompleteAcceptedShares(ctx)
	isErrorShown := !apiclient.IsUnreachable(completeErr) && !apiclient.IsStatus(completeErr, http.StatusNotFound)
	// The verifier pins signing keys whatever the server answers afterwards, so they are saved after every attempt
	// that added one.
	if len(completedShares) > 0 || len(s.state.Pins) != pinCount {
		if err := s.state.Save(); err != nil {
			completeErr, isErrorShown = errors.Join(completeErr, fmt.Errorf("could not save the pinned signing keys: %w", err)), true
		}
	}
	if len(completedShares) > 0 {
		s.loadVaults(ctx)
	}
	if isErrorShown {
		s.shareCompletionError = describeError(completeErr)
	}
	return len(completedShares) > 0
}

// ListShareInvitations completes accepted shares first, then returns the invitations this user received and sent.
func (s *Service) ListShareInvitations() (ShareInvitations, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ShareInvitations{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s.completeAcceptedShares(ctx)
	invitationList, err := s.unlockedSession.Client.ListShareInvitations(ctx)
	if err != nil {
		return ShareInvitations{}, convertError(err)
	}
	result := ShareInvitations{
		Received:        make([]ReceivedShareInvitation, 0, len(invitationList.Received)),
		Sent:            make([]SentShareInvitation, 0, len(invitationList.Sent)),
		Waiting:         make([]WaitingShareInvitation, 0, len(invitationList.Waiting)),
		CompletionError: s.shareCompletionError,
	}
	for _, waiting := range invitationList.Waiting {
		result.Waiting = append(result.Waiting, WaitingShareInvitation{
			ID: waiting.ID, VaultID: waiting.VaultID, Role: waiting.Role, InvitedByName: waiting.InvitedByName, InvitedByEmail: waiting.InvitedByEmail,
			AcceptedAt: formatTime(waiting.AcceptedAt),
		})
	}
	for _, received := range invitationList.Received {
		result.Received = append(result.Received, ReceivedShareInvitation{
			ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail,
			ExpiresAt: received.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	for _, sent := range invitationList.Sent {
		vaultName := ""
		if vault, found := s.vaultsByID[sent.VaultID]; found {
			vaultName = vault.Name
		}
		result.Sent = append(result.Sent, convertToSentShareInvitation(sent, vaultName))
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

// ListSharesAwaitingUnlock lists, while locked, the shares people accepted that this app finishes once unlocked.
// It needs only the saved session; unlocked, signed out or offline, the list is empty.
func (s *Service) ListSharesAwaitingUnlock() ([]ShareAwaitingUnlock, error) {
	s.mutex.Lock()
	isLocked := s.unlockedSession == nil && s.state.IsLoggedIn()
	server, token := s.state.Server, s.state.Token
	s.mutex.Unlock()
	awaitingShares := []ShareAwaitingUnlock{}
	if !isLocked {
		return awaitingShares, nil
	}
	apiClient := s.createClient(server)
	apiClient.Token = token
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	invitationList, err := apiClient.ListShareInvitations(ctx)
	if apiclient.IsUnreachable(err) {
		return awaitingShares, nil
	}
	if err != nil {
		return awaitingShares, convertError(err)
	}
	for _, sent := range invitationList.Sent {
		if sent.State != apitypes.ShareInvitationAccepted || sent.Recipient == nil {
			continue
		}
		awaiting := ShareAwaitingUnlock{InvitationID: sent.ID, RecipientName: sent.Recipient.Name, RecipientEmail: sent.Recipient.Email}
		if sent.AcceptedAt != nil {
			awaiting.AcceptedAt = formatTime(*sent.AcceptedAt)
		}
		awaitingShares = append(awaitingShares, awaiting)
	}
	return awaitingShares, nil
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
