package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

var ErrShareReviewRequired = errors.New("Review the accepted recipient in Sharing before giving them the vault key.")

func (s *Session) currentSharingVault(ctx context.Context, vaultID string) (*Vault, error) {
	vaults, err := s.ListVaults(ctx)
	if err != nil {
		return nil, err
	}
	for i := range vaults {
		vault := &vaults[i]
		if vault.ID == vaultID {
			if vault.Role != apitypes.VaultOwner {
				return nil, errors.New("Only owners of this vault can share it.")
			}
			if vault.Type == apitypes.VaultOrganization {
				return nil, errors.New("Organization vaults are shared through their teams.")
			}
			if vault.Type != apitypes.VaultShared {
				return nil, errors.New("Personal vaults cannot be shared. Create a shared vault instead.")
			}
			if vault.NeedsRotation {
				return nil, errors.New("Finish the vault's pending key rotation before sharing it.")
			}
			if vault.KeyGeneration <= 0 || vault.AccessRevision <= 0 {
				return nil, errors.New("Update the Hisar server before sharing this vault.")
			}
			return vault, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Session) trustedShareMember(member *apitypes.Member) error {
	if err := member.PublicKeys.Verify(member.AccountID); err != nil {
		return err
	}
	if s.MemberVerifier != nil {
		return s.MemberVerifier(member)
	}
	return nil
}

func (s *Session) newShareIntent(vault *Vault, email, role string) (*apitypes.SignedShareIntent, error) {
	origin, err := pake.CanonicalOrigin(s.Client.BaseURL)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	intent := crypt.ShareIntent{Version: 1, Origin: origin, ID: uuid.NewString(), VaultID: vault.ID, InvitedBy: s.AccountID,
		Email: apitypes.NormalizeEmail(email), Role: role, KeyGeneration: vault.KeyGeneration, AccessRevision: vault.AccessRevision, IssuedAt: now.Unix(), ExpiresAt: now.Add(7 * 24 * time.Hour).Unix()}
	return &apitypes.SignedShareIntent{Context: intent}, nil
}

// verifyInvitation runs before any key wrapping, independently of server signature validation.
func (s *Session) verifyInvitation(ctx context.Context, vault *Vault, invitation apitypes.SentShareInvitation) error {
	if invitation.Intent == nil {
		return ErrShareReviewRequired
	}
	if vault.Type != apitypes.VaultShared || vault.Role != apitypes.VaultOwner || vault.NeedsRotation {
		return errors.New("Finish the vault's pending key rotation before sharing it.")
	}
	intent := invitation.Intent.Context
	origin, err := pake.CanonicalOrigin(s.Client.BaseURL)
	if err != nil {
		return err
	}
	if intent.Version != 1 || intent.Origin != origin || intent.ID != invitation.ID || intent.VaultID != vault.ID ||
		intent.InvitedBy != invitation.InvitedBy || intent.Email != invitation.Email || intent.Role != invitation.Role ||
		intent.KeyGeneration != vault.KeyGeneration || intent.AccessRevision <= 0 || intent.AccessRevision > vault.AccessRevision || intent.ExpiresAt != invitation.ExpiresAt.Unix() || intent.ExpiresAt <= time.Now().Unix() ||
		intent.IssuedAt > time.Now().Unix()+60 || intent.ExpiresAt <= intent.IssuedAt || len(intent.ParentHash) != 0 {
		return errors.New("This invitation changed or expired. Cancel it and send a new invitation.")
	}
	var signerKey []byte
	if intent.InvitedBy == s.AccountID {
		signerKey = s.Keyset.GetSigningPublicKey()
	} else {
		inviter, err := s.GetVerifiedMember(ctx, intent.InvitedBy)
		if err != nil {
			return err
		}
		if err := s.trustedShareMember(inviter); err != nil {
			return err
		}
		signerKey = inviter.PublicKeys.Sign
	}
	if err := crypt.VerifyShareIntent(signerKey, intent, invitation.Intent.Signature); err != nil {
		return fmt.Errorf("The owner's sharing authorization is invalid: %w", err)
	}
	members, err := s.Client.ListVaultMembers(ctx, vault.ID)
	if err != nil {
		return err
	}
	for _, member := range members {
		if member.AccountID == intent.InvitedBy && member.Role == apitypes.VaultOwner {
			return nil
		}
	}
	return errors.New("The person who sent this invitation no longer owns the vault.")
}

func recipientMatches(intent crypt.ShareIntent, member *apitypes.Member) bool {
	return intent.RecipientID == member.AccountID && intent.Email == apitypes.NormalizeEmail(member.Email) && bytes.Equal(intent.RecipientKeyHash, crypt.ShareRecipientKeyHash(member.PublicKeys.Sign, member.PublicKeys.KEM))
}

func (s *Session) completeInvitation(ctx context.Context, vault *Vault, invitation apitypes.SentShareInvitation, authorization apitypes.SignedShareIntent) error {
	recipient := invitation.Recipient
	if recipient == nil || !recipientMatches(authorization.Context, recipient) {
		return errors.New("The recipient identity changed. Review a new invitation before sharing.")
	}
	if err := s.trustedShareMember(recipient); err != nil {
		return err
	}
	wrappedKey, err := crypt.WrapVaultKey(recipient.PublicKeys.KEM, vault.ID, recipient.AccountID, vault.Key)
	if err != nil {
		return err
	}
	return s.Client.CompleteSignedShare(ctx, &apitypes.CompleteShareRequest{Intent: authorization, WrappedKey: wrappedKey})
}

// ShareInvitationReviewHash binds every displayed sharing field and the concrete recipient keys.
func ShareInvitationReviewHash(invitation apitypes.SentShareInvitation) []byte {
	context := crypt.ShareIntent{ID: invitation.ID, VaultID: invitation.VaultID, InvitedBy: invitation.InvitedBy, Email: invitation.Email, Role: invitation.Role, ExpiresAt: invitation.ExpiresAt.Unix()}
	if invitation.Intent != nil {
		context.ParentHash = crypt.HashShareIntent(invitation.Intent.Context)
	}
	if invitation.Recipient != nil {
		context.RecipientID = invitation.Recipient.AccountID
		context.RecipientKeyHash = crypt.ShareRecipientKeyHash(invitation.Recipient.PublicKeys.Sign, invitation.Recipient.PublicKeys.KEM)
	}
	return crypt.HashShareIntent(context)
}

// ConfirmShareRecipient signs the concrete identity displayed by a native confirmation screen, never an ID-only approval.
func (s *Session) ConfirmShareRecipient(ctx context.Context, invitationID string, reviewHash []byte) error {
	invitations, err := s.Client.ListShareInvitations(ctx)
	if err != nil {
		return err
	}
	for _, invitation := range invitations.Sent {
		if invitation.ID != invitationID || invitation.State != apitypes.ShareInvitationAccepted || invitation.Recipient == nil {
			continue
		}
		if invitation.ExpiresAt.Unix() <= time.Now().Unix() {
			return errors.New("This invitation expired. Cancel it and send a new invitation.")
		}
		vault, err := s.currentSharingVault(ctx, invitation.VaultID)
		if err != nil {
			return err
		}
		recipient := invitation.Recipient
		if !bytes.Equal(reviewHash, ShareInvitationReviewHash(invitation)) {
			return errors.New("The invitation changed while you were reviewing. Review the vault, role and recipient identity again.")
		}
		if err := s.trustedShareMember(recipient); err != nil {
			return err
		}
		authorization, err := s.newShareIntent(vault, invitation.Email, invitation.Role)
		if err != nil {
			return err
		}
		authorization.Context.ID = invitation.ID
		authorization.Context.RecipientID = recipient.AccountID
		authorization.Context.RecipientKeyHash = crypt.ShareRecipientKeyHash(recipient.PublicKeys.Sign, recipient.PublicKeys.KEM)
		authorization.Context.ExpiresAt = invitation.ExpiresAt.Unix()
		if invitation.Intent != nil {
			if err := s.verifyInvitation(ctx, vault, invitation); err != nil {
				return err
			}
			if invitation.Intent.Context.RecipientID != "" {
				return errors.New("This invitation already binds a different recipient. Cancel it and send a new invitation.")
			}
			authorization.Context.ParentHash = crypt.HashShareIntent(invitation.Intent.Context)
		}
		authorization.Signature = s.Keyset.SignShareIntent(authorization.Context)
		return s.completeInvitation(ctx, vault, invitation, *authorization)
	}
	return ErrNotFound
}
