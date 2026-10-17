package session

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

var (
	ErrBadCredentials = errors.New("incorrect email, password or Secret Key")
	ErrConflict       = errors.New("item was changed on another device")
	ErrRollback       = errors.New("server returned older vault state than previously seen")
	ErrKeyMismatch    = errors.New("server returned public keys that do not match the keyset")
	ErrNotFound       = errors.New("not found")
	// ErrKeyChanged: the server refused items sealed with a vault key another device's rotation replaced, again after the
	// vault was reloaded and the items sealed with its current key.
	ErrKeyChanged = errors.New("the vault's key changed on another device; try again")
)

// SeqStore remembers the highest vault seq seen, to detect server rollbacks.
type SeqStore interface {
	GetSeq(vaultID string) int64
	SetSeq(vaultID string, seq int64)
}

type Session struct {
	Client    *apiclient.Client
	AccountID string
	Email     string
	Name      string
	Role      string
	Keyset    *crypt.Keyset
	SeqStore  SeqStore
	// MemberVerifier, when set, is asked to accept each member a vault key is wrapped for (e.g. key pinning).
	MemberVerifier func(member *apitypes.Member) error
	// Cache, when set, stores the server's encrypted records; reads fall back to it while the server is unreachable.
	Cache     Cache
	isOffline atomic.Bool

	removalMutex  sync.Mutex
	removedVaults []RemovedVault
	ownRemovalIDs map[string]bool
}

type Vault struct {
	ID   string
	Type string
	Name string
	Role string
	Key  []byte
	// OrganizationID is set for organization vaults only.
	OrganizationID string
	// KeyGeneration is the generation of Key; 0 when the server does not count them.
	KeyGeneration int64
	// NeedsRotation: someone lost access through a team and no owner's app has rotated the key since.
	NeedsRotation bool
}

func (s *Session) Lock() {
	if s.Keyset != nil {
		s.Keyset.Wipe()
		s.Keyset = nil
	}
}
