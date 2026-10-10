// Package proto defines the JSON wire types of the /api/v1 HTTP API.
package proto

import (
	"time"

	"github.com/tryhisar/hisar/core/crypt"
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	VaultOwner  = "owner"
	VaultEditor = "editor"
	VaultReader = "reader"

	VaultPersonal = "personal"
	VaultShared   = "shared"
)

type Error struct {
	Error string `json:"error"`
}

type PreloginResponse struct {
	Salt   []byte             `json:"salt"`
	Argon2 crypt.Argon2Params `json:"argon2"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
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

type Account struct {
	AccountID  string             `json:"accountId"`
	Email      string             `json:"email"`
	Name       string             `json:"name"`
	Role       string             `json:"role"`
	Argon2     crypt.Argon2Params `json:"argon2"`
	Salt       []byte             `json:"salt"`
	EncKeyset  []byte             `json:"encKeyset"`
	PublicKeys crypt.PublicKeys   `json:"publicKeys"`
}

type Member struct {
	AccountID  string           `json:"accountId"`
	Email      string           `json:"email"`
	Name       string           `json:"name"`
	Role       string           `json:"role"`
	PublicKeys crypt.PublicKeys `json:"publicKeys"`
}

type InviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type InviteResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	Current    bool      `json:"current"`
}

type Vault struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	EncName    []byte `json:"encName"`
	Seq        int64  `json:"seq"`
	Role       string `json:"role"`
	WrappedKey []byte `json:"wrappedKey"`
	// Recoverable is true when the vault key is wrapped to the recovery keyset.
	Recoverable    bool `json:"recoverable"`
	RecoveryOptOut bool `json:"recoveryOptOut"`
}

type CreateVaultRequest struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	EncName            []byte `json:"encName"`
	WrappedKey         []byte `json:"wrappedKey"`
	RecoveryWrappedKey []byte `json:"recoveryWrappedKey,omitempty"`
}

type VaultMember struct {
	AccountID string `json:"accountId"`
	Role      string `json:"role"`
	GrantedBy string `json:"grantedBy"`
}

type SetAccessRequest struct {
	WrappedKey []byte `json:"wrappedKey"`
	Role       string `json:"role"`
}

type Item struct {
	ID        string    `json:"id"`
	Revision  string    `json:"revision"`
	Seq       int64     `json:"seq"`
	Deleted   bool      `json:"deleted"`
	Overview  []byte    `json:"overview"`
	Details   []byte    `json:"details"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

type ChangesResponse struct {
	Items   []Item `json:"items"`
	NextSeq int64  `json:"nextSeq"`
	More    bool   `json:"more"`
}

// PutItemRequest is sent with an If-Match header carrying the base revision (absent for new items).
type PutItemRequest struct {
	Revision string `json:"revision"`
	Deleted  bool   `json:"deleted"`
	Overview []byte `json:"overview"`
	Details  []byte `json:"details"`
}

type PutItemResponse struct {
	Seq int64 `json:"seq"`
}

// Server-sent events on GET /api/v1/events.
const (
	EventVaultChanged  = "vault-changed"
	EventVaultsChanged = "vaults-changed"
	MaxEventSize       = 64 << 10
)

// VaultChangedEvent says a vault reached Seq; Seq is 0 when the server does not know it, and clients sync either way.
type VaultChangedEvent struct {
	VaultID string `json:"vaultID"`
	Seq     int64  `json:"seq"`
}

type ConflictResponse struct {
	Current *Item `json:"current"`
}

type RotatedAccess struct {
	AccountID  string `json:"accountId"`
	Role       string `json:"role"`
	WrappedKey []byte `json:"wrappedKey"`
}

type RotatedItem struct {
	ID           string `json:"id"`
	BaseRevision string `json:"baseRevision"`
	Revision     string `json:"revision"`
	Deleted      bool   `json:"deleted"`
	Overview     []byte `json:"overview"`
	Details      []byte `json:"details"`
}

// RotateVaultRequest replaces the vault key: every member's wrapped key and every item, re-encrypted.
// RecoveryWrappedKey is required when the vault has a recovery wrapping.
type RotateVaultRequest struct {
	EncName            []byte          `json:"encName"`
	AllAccess          []RotatedAccess `json:"access"`
	AllItem            []RotatedItem   `json:"items"`
	RecoveryWrappedKey []byte          `json:"recoveryWrappedKey,omitempty"`
}

// RecoveryKeyset is the admin's recovery keyset: PublicKeys.KEM is the recovery KEM key, signed by
// PublicKeys.Sign (the creating admin's signing key) for recipient "recovery". EncKeyset is sent only to its creator.
type RecoveryKeyset struct {
	PublicKeys crypt.PublicKeys `json:"publicKeys"`
	EncKeyset  []byte           `json:"encKeyset,omitempty"`
	CreatedBy  string           `json:"createdBy"`
}

type CreateRecoveryKeysetRequest struct {
	PublicKeys crypt.PublicKeys `json:"publicKeys"`
	EncKeyset  []byte           `json:"encKeyset"`
}

type SetVaultRecoveryRequest struct {
	WrappedKey []byte `json:"wrappedKey"`
}

type RecoveryLinkResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

type RecoveryAccountRequest struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

type RecoveryAccountResponse struct {
	AccountID string `json:"accountId"`
}

type RecoveryStartRequest struct {
	Token   string `json:"token"`
	Email   string `json:"email"`
	Request []byte `json:"request"`
}

type RecoveryStartResponse struct {
	Response []byte `json:"response"`
}

type RecoveryFinishRequest struct {
	Token      string             `json:"token"`
	Email      string             `json:"email"`
	Record     []byte             `json:"record"`
	Salt       []byte             `json:"salt"`
	Argon2     crypt.Argon2Params `json:"argon2"`
	EncKeyset  []byte             `json:"encKeyset"`
	PublicKeys crypt.PublicKeys   `json:"publicKeys"`
	Device     DeviceInfo         `json:"device"`
}

// RecoveryVault is a vault of an account being recovered; WrappedKey is its recovery wrapping, nil when it has none.
type RecoveryVault struct {
	VaultID    string `json:"vaultId"`
	Type       string `json:"type"`
	WrappedKey []byte `json:"wrappedKey"`
}

type PendingRecovery struct {
	AccountID   string           `json:"accountId"`
	Email       string           `json:"email"`
	Name        string           `json:"name"`
	PublicKeys  crypt.PublicKeys `json:"publicKeys"`
	SubmittedAt time.Time        `json:"submittedAt"`
	AllVault    []RecoveryVault  `json:"vaults"`
}

type VaultKey struct {
	VaultID    string `json:"vaultId"`
	WrappedKey []byte `json:"wrappedKey"`
}

type CompleteRecoveryRequest struct {
	AllVaultKey []VaultKey `json:"vaultKeys"`
}

type CompleteRecoveryResponse struct {
	AllUnrecoverableVaultID []string `json:"unrecoverableVaults"`
}
