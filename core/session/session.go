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
	ErrChangesStalled = errors.New("the server's list of vault changes did not advance")
	ErrKeyMismatch    = errors.New("server returned public keys that do not match the keyset")
	ErrNotFound       = errors.New("not found")
	// ErrKeyChanged: the server refused items sealed with a vault key another device's rotation replaced, again after the
	// vault was reloaded and the items sealed with its current key.
	ErrKeyChanged = errors.New("the vault's key changed on another device; try again")
	// ErrImportScopeChanged: the import destination's members changed since the import was reviewed, so the server refused
	// the scoped write. Every further write with that scope fails too.
	ErrImportScopeChanged = errors.New("destination membership changed; review the import destinations again")
	// ErrAccountSuspended: an admin of the server suspended the account. Sign-in says so only after a correct proof; a
	// device that was signed in keeps its session and gets it on every request until reactivation.
	ErrAccountSuspended = apiclient.ErrAccountSuspended
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
	// DeviceID is the device LogIn or SignUp signed in; Unlock leaves it empty.
	DeviceID string
	// MemberVerifier, when set, is asked to accept each member a vault key is wrapped for (e.g. key pinning).
	MemberVerifier func(member *apitypes.Member) error
	// Cache, when set, stores the server's encrypted records; reads fall back to it while the server is unreachable.
	Cache           Cache
	isOffline       atomic.Bool
	verifiedAccount *apitypes.Account

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
	KeyGeneration  int64
	AccessRevision int64
	// NeedsRotation remains set after owner removal until an owner's app commits the replacement key generation.
	NeedsRotation   bool
	NameUnreadable  bool
	IsPersonalExtra bool
	// record is the server's record Key was opened from; the cache publishes it with the items sealed with Key.
	record apitypes.Vault
}

func (s *Session) Lock() {
	if s.Keyset != nil {
		s.Keyset.Wipe()
		s.Keyset = nil
	}
}
