package hisarkit

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tryhisar/hisar/core/app"
)

// PasskeyVerifier performs fresh device-owner authentication in the native ceremony.
type PasskeyVerifier interface {
	AuthenticatePasskey(reason string) error
}

type nativePasskeyVerifier struct{ verifier PasskeyVerifier }

func (adapter nativePasskeyVerifier) VerifyPasskey(ctx context.Context, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := adapter.verifier.AuthenticatePasskey(reason); err != nil {
		return err
	}
	return ctx.Err()
}

// SetPasskeyVerifier must run immediately after New, before any passkey ceremony.
func (hisar *Hisar) SetPasskeyVerifier(verifier PasskeyVerifier) {
	if verifier == nil {
		app.SetPasskeyVerifier(hisar.service, nil)
		return
	}
	app.SetPasskeyVerifier(hisar.service, nativePasskeyVerifier{verifier})
}

// AssertPasskeyWithPolicy accepts the complete policy-aware assertion request as JSON.
func (hisar *Hisar) AssertPasskeyWithPolicy(requestJSON string) (string, error) {
	var request app.PasskeyAssertionRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		return "", errors.New("hisarkit: the passkey assertion is not valid JSON")
	}
	return encodePasskeyJSON(app.SignPasskeyWithPolicy(hisar.service, request))
}

// CancelPasskeyCeremonies is safe on the main thread; it only invalidates in-flight authorization.
func (hisar *Hisar) CancelPasskeyCeremonies() {
	app.CancelPasskeyCeremonies(hisar.service)
}
