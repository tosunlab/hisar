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
}

type CreateVaultRequest struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	EncName    []byte `json:"encName"`
	WrappedKey []byte `json:"wrappedKey"`
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
type RotateVaultRequest struct {
	EncName   []byte          `json:"encName"`
	AllAccess []RotatedAccess `json:"access"`
	AllItem   []RotatedItem   `json:"items"`
}
