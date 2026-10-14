package apitypes

// Server-sent events on GET /api/v1/events.
const (
	EventVaultChanged  = "vault-changed"
	EventVaultsChanged = "vaults-changed"
	// EventShareInvitationsChanged carries {}: the account's received or sent share invitations changed.
	EventShareInvitationsChanged = "share-invitations-changed"
	MaxEventSize                 = 64 << 10
)

// VaultChangedEvent says a vault reached Seq; Seq is 0 when the server does not know it, and clients sync either way.
type VaultChangedEvent struct {
	VaultID string `json:"vaultID"`
	Seq     int64  `json:"seq"`
}
