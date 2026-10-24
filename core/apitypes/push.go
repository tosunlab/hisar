package apitypes

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

// PushPlatform is the push service a phone's token belongs to.
type PushPlatform string

const (
	PushPlatformAPNs PushPlatform = "apns"
	// PushPlatformAPNsSandbox is the APNs development environment, for builds signed for development.
	PushPlatformAPNsSandbox PushPlatform = "apns-sandbox"
	PushPlatformFCM         PushPlatform = "fcm"
)

// PushRegistration is the body of PUT /api/v1/devices/current/push: the push relay's handle for this phone and the
// kinds it wants. The server never sees the push service's own token.
type PushRegistration struct {
	Handle  string     `json:"handle"`
	AllKind []PushKind `json:"kinds"`
}

// PushConfig is GET /api/v1/push/config: the push relay this server sends through. An empty URL means the server
// sends no push notifications.
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
