package service

// Kind is what went wrong, so that the API can answer with the matching HTTP status.
type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindTooManyRequests
	KindUnavailable
	KindNotImplemented
	KindGone
)

// Error is a refusal the caller may read; any other error is internal.
type Error struct {
	Kind    Kind
	Message string
}

func (err *Error) Error() string { return err.Message }

func newError(kind Kind, message string) *Error { return &Error{Kind: kind, Message: message} }

var (
	ErrNotFound  = newError(KindNotFound, "not found")
	ErrOwnerOnly = newError(KindForbidden, "owner only")
	ErrAdminOnly = newError(KindForbidden, "admin only")
	// ErrKeyChanged refuses items sealed with a key generation other than the vault's current one (§5.11). The API answers
	// it with apitypes.KeyChangedResponse, so the client tells it from an item conflict and seals the items again.
	ErrKeyChanged = newError(KindConflict, "the vault key changed since the items were encrypted; sync and try again")
	// ErrDatabaseUnavailable answers a request the server cannot serve because its database does not answer.
	ErrDatabaseUnavailable = newError(KindUnavailable, "the server cannot reach its database")
)
