package pake

import (
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/bytemare/opaque"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// WithReauthenticationScope leaves generic proofs unchanged and binds deletion intent inside OPAQUE's transcript.
// The base must already contain the client's intended or the server's configured origin and reauthentication purpose.
func WithReauthenticationScope(base *opaque.Configuration, scope apitypes.ReauthenticationScope) (*opaque.Configuration, error) {
	if scope == (apitypes.ReauthenticationScope{}) {
		return base, nil
	}
	if scope.Action != "account-delete" && scope.Action != "admin-account-delete" {
		return nil, errors.New("pake: unsupported reauthentication scope")
	}
	if !crypt.IsValidAccountID(scope.Target) || len(scope.PlanVersion) != 66 || scope.PlanVersion[:2] != "1:" {
		return nil, errors.New("pake: invalid account deletion scope")
	}
	digest, err := hex.DecodeString(scope.PlanVersion[2:])
	if err != nil || hex.EncodeToString(digest) != scope.PlanVersion[2:] {
		return nil, errors.New("pake: invalid deletion plan digest")
	}
	context := []byte("hisar/v1/scoped-reauthentication")
	for _, field := range [][]byte{base.Context, []byte(scope.Action), []byte(scope.Target), digest} {
		context = binary.BigEndian.AppendUint32(context, uint32(len(field)))
		context = append(context, field...)
	}
	configuration := *base
	configuration.Context = context
	return &configuration, nil
}
