package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/passkey"
)

const passkeyAuthorizationLifetime = 2 * time.Minute

// PasskeyVerifier is installed by the native ceremony owner, never by a browser request.
// Success means a fresh device-owner or master-password check completed for this invocation.
type PasskeyVerifier interface {
	VerifyPasskey(context.Context, string) error
}

// SetPasskeyVerifier installs the native adapter without making it a Wails method.
func SetPasskeyVerifier(s *Service, verifier PasskeyVerifier) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.config.PasskeyVerifier = verifier
}

// CancelPasskeyCeremonies invalidates in-flight native authorization without waiting on the UI thread.
func CancelPasskeyCeremonies(s *Service) {
	s.passkeyEpoch.Add(1)
}

type passkeyAuthorization struct {
	ctx         context.Context
	epoch       uint64
	nativeEpoch uint64
	expiresAt   time.Time
	verified    bool
	consumed    bool
}

func (s *Service) authorizePasskey(ctx context.Context, policy, ceremonyType, relyingPartyID string, needsPRF bool) (*Service, *passkeyAuthorization, error) {
	if policy != "required" && policy != "preferred" && policy != "discouraged" {
		return nil, nil, browserprotocol.NewError(browserprotocol.ErrorUpdateRequired, "update Hisar and its extension to use policy-aware passkeys")
	}
	s.mutex.Lock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		s.mutex.Unlock()
		return nil, nil, err
	}
	if epoch, browser := ctx.Value(passkeyEpochContextKey{}).(uint64); browser && epoch != s.sessionEpoch {
		s.mutex.Unlock()
		return nil, nil, errSessionEnded
	}
	authorization := &passkeyAuthorization{ctx: ctx, epoch: s.sessionEpoch, nativeEpoch: s.passkeyEpoch.Load(), expiresAt: s.config.Now().Add(passkeyAuthorizationLifetime)}
	verifier := s.config.PasskeyVerifier
	s.mutex.Unlock()
	action := "use your passkey for"
	if ceremonyType == passkey.TypeCreate {
		action = "create a passkey for"
	}
	if policy != "discouraged" || needsPRF {
		if verifier == nil {
			return nil, nil, errors.New("Hisar cannot verify you on this device; no passkey was used or saved")
		}
		verificationContext, cancel := context.WithTimeout(ctx, passkeyAuthorizationLifetime)
		defer cancel()
		if err := verifier.VerifyPasskey(verificationContext, fmt.Sprintf("%s %s", action, relyingPartyID)); err != nil {
			return nil, nil, browserprotocol.NewError(browserprotocol.ErrorPasskeyVerification, fmt.Sprintf("passkey verification did not complete: %s", err))
		}
		if err := verificationContext.Err(); err != nil {
			return nil, nil, err
		}
		authorization.verified = true
	}
	bound := &Service{serviceState: s.serviceState, requestEpoch: &authorization.epoch}
	return bound, authorization, nil
}

// consume runs under the signing/save mutex. Evidence never leaves this request's stack.
func (authorization *passkeyAuthorization) consume(s *Service) error {
	if authorization.consumed || !s.config.Now().Before(authorization.expiresAt) {
		return passkey.ErrInvalidRequest
	}
	authorization.consumed = true
	if err := authorization.ctx.Err(); err != nil {
		return err
	}
	if authorization.nativeEpoch != s.passkeyEpoch.Load() {
		return errors.New("this passkey ceremony has ended; try again")
	}
	if err := s.requirePasskeyPairing(authorization.ctx); err != nil {
		return err
	}
	return s.requireSessionEpoch(authorization.epoch)
}
