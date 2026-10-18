package apitypes

import (
	"github.com/tryhisar/hisar/core/crypt"
)

type PreloginResponse struct {
	Salt   []byte             `json:"salt"`
	Argon2 crypt.Argon2Params `json:"argon2"`
}

type RegisterStartRequest struct {
	Invite    string `json:"invite"`
	Email     string `json:"email"`
	AccountID string `json:"accountId"`
	Request   []byte `json:"request"`
}

type RegisterStartResponse struct {
	Response []byte `json:"response"`
}

type RegisterFinishRequest struct {
	Invite     string             `json:"invite"`
	Email      string             `json:"email"`
	Name       string             `json:"name"`
	AccountID  string             `json:"accountId"`
	Record     []byte             `json:"record"`
	Salt       []byte             `json:"salt"`
	Argon2     crypt.Argon2Params `json:"argon2"`
	EncKeyset  []byte             `json:"encKeyset"`
	PublicKeys crypt.PublicKeys   `json:"publicKeys"`
	Device     DeviceInfo         `json:"device"`
}

type LoginStartRequest struct {
	Email string `json:"email"`
	KE1   []byte `json:"ke1"`
}

type LoginStartResponse struct {
	LoginID string `json:"loginId"`
	KE2     []byte `json:"ke2"`
}

type LoginFinishRequest struct {
	LoginID string     `json:"loginId"`
	KE3     []byte     `json:"ke3"`
	Device  DeviceInfo `json:"device"`
}

type SessionResponse struct {
	Token     string `json:"token"`
	AccountID string `json:"accountId"`
	DeviceID  string `json:"deviceId"`
}

const (
	CredentialChangeMasterPassword = "master-password"
	CredentialChangeSecretKey      = "secret-key"
)

// ReauthenticateRequest carries RegistrationRequest only when the credentials are about to change.
type ReauthenticateRequest struct {
	KE1                 []byte `json:"ke1"`
	RegistrationRequest []byte `json:"registrationRequest,omitempty"`
}

type ReauthenticateResponse struct {
	ReauthenticationID   string `json:"reauthenticationId"`
	KE2                  []byte `json:"ke2"`
	RegistrationResponse []byte `json:"registrationResponse,omitempty"`
}

// ReauthenticationProof is a finished re-authentication: the OPAQUE KE3 for the server's ReauthenticationID. The server
// accepts it once, for the one change that carries it.
type ReauthenticationProof struct {
	ReauthenticationID string `json:"reauthenticationId"`
	KE3                []byte `json:"ke3"`
}

type ChangeCredentialsRequest struct {
	ReauthenticationID string             `json:"reauthenticationId"`
	KE3                []byte             `json:"ke3"`
	Change             string             `json:"change"`
	Record             []byte             `json:"record"`
	Salt               []byte             `json:"salt"`
	Argon2             crypt.Argon2Params `json:"argon2"`
	EncKeyset          []byte             `json:"encKeyset"`
}

type ChangeCredentialsResponse struct {
	SignedOutDeviceCount int `json:"signedOutDeviceCount"`
}
