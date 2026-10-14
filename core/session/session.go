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

	removalMutex    sync.Mutex
	allRemovedVault []RemovedVault
	allOwnRemovalID map[string]bool
}

type Vault struct {
	ID   string
	Type string
	Name string
	Role string
	Key  []byte
}

func (s *Session) Lock() {
	if s.Keyset != nil {
		s.Keyset.Wipe()
		s.Keyset = nil
	}
}
