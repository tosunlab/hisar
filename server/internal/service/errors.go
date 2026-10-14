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
)
