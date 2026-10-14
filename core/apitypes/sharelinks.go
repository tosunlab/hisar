package apitypes

import (
	"time"
)

const EventShareLinksChanged = "share-links-changed"

type ShareLinkPassphrase struct {
	Salt       []byte `json:"salt"`
	Iterations int    `json:"iterations"`
}

type CreateShareLinkRequest struct {
	Ciphertext       []byte               `json:"ciphertext"`
	EncryptedLabel   []byte               `json:"encryptedLabel"`
	ExpiresInSeconds int64                `json:"expiresInSeconds"`
	MaxViews         int                  `json:"maxViews"`
	Passphrase       *ShareLinkPassphrase `json:"passphrase"`
}

// ShareLink is the creator's view of a link; MaxViews 0 means unlimited.
type ShareLink struct {
	ID             string    `json:"id"`
	EncryptedLabel []byte    `json:"encryptedLabel,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	MaxViews       int       `json:"maxViews"`
	ViewCount      int       `json:"viewCount"`
	HasPassphrase  bool      `json:"hasPassphrase"`
}

// PublicShareLink is what anyone holding the ID gets; every fetch counts a view.
type PublicShareLink struct {
	Ciphertext []byte               `json:"ciphertext"`
	Passphrase *ShareLinkPassphrase `json:"passphrase"`
	ExpiresAt  time.Time            `json:"expiresAt"`
	MaxViews   int                  `json:"maxViews"`
	ViewCount  int                  `json:"viewCount"`
}
