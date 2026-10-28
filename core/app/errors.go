package app

import (
	"errors"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

var (
	ErrLocked    = errors.New("Hisar is locked")
	ErrSignedOut = errors.New("not signed in")
	ErrNotFound  = errors.New("not found")
	ErrOffline   = errors.New("cannot reach the Hisar server; changes can be saved once you are back online")
)

// ErrNoAgentGrantHead and ErrAgentGrantHeadUnavailable are what an AgentGrantHeadStore returns when it keeps no head,
// and when it cannot be read or written.
var (
	ErrNoAgentGrantHead          = errors.New("no head of the saved agent grants")
	ErrAgentGrantHeadUnavailable = errors.New("Hisar cannot keep agent grants on this device")
)

// errBadCredentials is a wrong master password, or email or Secret Key, as the user reads it.
var errBadCredentials = errors.New("incorrect email, master password or Secret Key")

var errUnlockPassword = errors.New("The master password is incorrect.")

func convertError(err error) error {
	switch {
	case errors.Is(err, session.ErrBadCredentials):
		return errBadCredentials
	case errors.Is(err, crypt.ErrArgon2ParamsOutOfRange):
		return errors.New("the server asked for key-stretching settings Hisar does not accept; contact the server's admin")
	case errors.Is(err, session.ErrConflict):
		return errors.New("this item was changed on another device; reload it and try again")
	case errors.Is(err, session.ErrRollback):
		return errors.New("the server returned older data than this device has seen; contact Hisar support")
	case errors.Is(err, session.ErrKeyMismatch):
		return errors.New("the server returned keys that do not match your account; contact Hisar support")
	case errors.Is(err, apiclient.ErrSignUpClosed):
		return err
	case apiclient.IsStatus(err, http.StatusForbidden):
		return errors.New("you do not have permission to do that")
	case apiclient.IsStatus(err, http.StatusTooManyRequests):
		return errors.New("too many attempts; wait a minute and try again")
	case apiclient.IsUnreachable(err):
		return ErrOffline
	}
	return err
}
