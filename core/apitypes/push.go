package apitypes

import "time"

// PushKind names what a push notification is about. It is all a push carries: the phone shows a sentence for the kind,
// and the details only once Hisar is open and unlocked.
type PushKind string

const (
	PushKindApproval PushKind = "approval"
	PushKindSecurity PushKind = "security"
	PushKindSharing  PushKind = "sharing"
)

// ListAllPushKind lists every kind, in the order the phones' settings show them.
func ListAllPushKind() []PushKind {
	return []PushKind{PushKindApproval, PushKindSecurity, PushKindSharing}
}

func IsValidPushKind(kind PushKind) bool {
	for _, known := range ListAllPushKind() {
		if kind == known {
			return true
		}
	}
	return false
}

// GetTimeToLive is how long a push service keeps trying to deliver a push of the kind: an approval request ends within
// minutes, the others stay worth seeing for a day.
func (kind PushKind) GetTimeToLive() time.Duration {
	if kind == PushKindApproval {
		return 3 * time.Minute
	}
	return 24 * time.Hour
}

// PushPlatform is the push service an iPhone's token belongs to. Android phones use Web Push instead, through the
// UnifiedPush distributor the user installed.
type PushPlatform string

const (
	PushPlatformAPNs PushPlatform = "apns"
	// PushPlatformAPNsSandbox is the APNs development environment, for builds signed for development.
	PushPlatformAPNsSandbox PushPlatform = "apns-sandbox"
)

// PushRegistration is the body of PUT /api/v1/devices/current/push: where this phone gets its pushes, and the kinds it
// wants. An iPhone sends the push relay's handle, and the server never sees its APNs token; an Android phone sends the
// Web Push subscription of its UnifiedPush distributor, which the server sends to directly. Exactly one is set.
type PushRegistration struct {
	Handle  string               `json:"handle,omitempty"`
	WebPush *WebPushSubscription `json:"webPush,omitempty"`
	AllKind []PushKind           `json:"kinds"`
}

// WebPushSubscription is a Web Push endpoint and the keys its messages are encrypted to (RFC 8291). P256DH, the
// phone's public key, and Auth, the authentication secret, are base64url without padding, as UnifiedPush gives them.
type WebPushSubscription struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// PushConfig is GET /api/v1/push/config: the push relay this server sends iPhones' pushes through. An empty URL means
// the server sends no push notifications, to iPhones or Android phones.
type PushConfig struct {
	RelayURL string `json:"relayURL"`
}

// RelayRegisterRequest is the body of the push relay's POST /v1/register, which the phone sends with its push service
// token and gets a handle back.
type RelayRegisterRequest struct {
	Platform PushPlatform `json:"platform"`
	Token    string       `json:"token"`
}

type RelayRegisterResponse struct {
	Handle string `json:"handle"`
}

// RelayPushRequest is the body of the push relay's POST /v1/push, which a Hisar server sends. The relay answers 410
// Gone when the push service no longer knows the token, and the server then forgets the handle.
type RelayPushRequest struct {
	Handle string   `json:"handle"`
	Kind   PushKind `json:"kind"`
}
