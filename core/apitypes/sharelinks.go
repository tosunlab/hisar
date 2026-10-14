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

// Share link limits the server enforces and the apps respect.
const (
	ShareLinkMaxCiphertextSize = 64 << 10
	ShareLinkMaxLabelSize      = 4 << 10
	ShareLinkMaxViews          = 100
)

// The only lifetimes a share link may have.
const (
	ShareLinkExpiryHour  = time.Hour
	ShareLinkExpiryDay   = 24 * time.Hour
	ShareLinkExpiryWeek  = 7 * 24 * time.Hour
	ShareLinkExpiryMonth = 30 * 24 * time.Hour
)

// IsAllowedShareLinkExpiry reports whether expiry is one of the allowed lifetimes.
func IsAllowedShareLinkExpiry(expiry time.Duration) bool {
	switch expiry {
	case ShareLinkExpiryHour, ShareLinkExpiryDay, ShareLinkExpiryWeek, ShareLinkExpiryMonth:
		return true
	}
	return false
}
