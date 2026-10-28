package apitypes

import "time"

// PushKind names what a push notification is about. Apple, Google, the push relay and a UnifiedPush distributor see
// only the kind: the phone shows a sentence for it, or the PushMessage the server encrypted to the phone.
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

// PushMessage is what a push says, encrypted to the phone (RFC 8291): an Android phone's Web Push message, or an
// iPhone's RelayPushRequest.Payload, which its notification service extension opens. Invitation is set for a sharing
// push, Approval for an approval push a Mac wrote; an older server sends the kind alone.
type PushMessage struct {
	Kind       PushKind        `json:"kind"`
	Invitation *PushInvitation `json:"invitation,omitempty"`
	Approval   *PushApproval   `json:"approval,omitempty"`
}

// PushApproval is the preview of an approval request (approval.PushPreview as JSON, base64url) and the account's
// signature over it (crypt.SignApprovalPreview, base64url). The Mac that asks writes and encrypts it to each phone;
// the server only passes it on.
type PushApproval struct {
	Preview   string `json:"preview"`
	Signature string `json:"signature"`
}

// PushInvitation names who invited the phone's account to what. A shared vault's name is encrypted, so the server
// cannot name the vault; an organization's name is not (§2.3).
type PushInvitation struct {
	Type             string `json:"type"`
	InviterName      string `json:"inviterName"`
	InviterEmail     string `json:"inviterEmail"`
	OrganizationName string `json:"organizationName,omitempty"`
	Role             string `json:"role"`
}

const (
	PushInvitationOrganization = "organization"
	PushInvitationVault        = "vault"
)

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
// PayloadKey, with a handle only, is the key an iPhone opens a push's details with; only a server whose PushConfig
// says AcceptsPayloadKey takes it. KeySignature, base64url, is the account's signature over the key messages are
// encrypted to (crypt.SignPushKey): the payload key, or the Web Push subscription's; a phone sends it once unlocked,
// and only to a server whose PushConfig says AcceptsKeySignature.
type PushRegistration struct {
	Handle       string               `json:"handle,omitempty"`
	WebPush      *WebPushSubscription `json:"webPush,omitempty"`
	PayloadKey   *PushPayloadKey      `json:"payloadKey,omitempty"`
	KeySignature string               `json:"keySignature,omitempty"`
	AllKind      []PushKind           `json:"kinds"`
}

// DevicePushKey is a phone's key for push messages with the account's signature over it, as GET /api/v1/push/keys
// lists them: a Mac checks the signature, then encrypts an approval's preview to the key.
type DevicePushKey struct {
	DeviceID  string `json:"deviceId"`
	P256DH    string `json:"p256dh"`
	Auth      string `json:"auth"`
	Signature string `json:"signature"`
}

type DevicePushKeysResponse struct {
	AllKey []DevicePushKey `json:"keys"`
}

// PushPayloadKey is an iPhone's public key and authentication secret for push details, as a Web Push subscription's
// (base64url): the server encrypts a PushMessage to it as it does to an Android phone.
type PushPayloadKey struct {
	P256DH string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// WebPushSubscription is a Web Push endpoint and the keys its messages are encrypted to (RFC 8291). P256DH, the
// phone's public key, and Auth, the authentication secret, are base64url without padding, as UnifiedPush gives them.
type WebPushSubscription struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// PushConfig is GET /api/v1/push/config: the push relay this server sends iPhones' pushes through. An empty URL means
// the server sends no push notifications, to iPhones or Android phones. AcceptsPayloadKey and AcceptsKeySignature mean
// PushRegistration may carry a PayloadKey and a KeySignature; an older server rejects the fields.
type PushConfig struct {
	RelayURL            string `json:"relayURL"`
	AcceptsPayloadKey   bool   `json:"acceptsPayloadKey,omitempty"`
	AcceptsKeySignature bool   `json:"acceptsKeySignature,omitempty"`
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
// Gone when the push service no longer knows the token, and the server then forgets the handle. Payload is the
// PushMessage encrypted to the phone's PushPayloadKey (aes128gcm, base64url): the relay passes it on unread. An older
// relay rejects the field, and the server then sends the kind alone.
type RelayPushRequest struct {
	Handle  string   `json:"handle"`
	Kind    PushKind `json:"kind"`
	Payload string   `json:"payload,omitempty"`
}

// MaximumRelayPushPayload bounds RelayPushRequest.Payload, so the APNs payload stays under its 4 KB.
const MaximumRelayPushPayload = 3072
