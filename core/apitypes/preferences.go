package apitypes

// EventAccountPreferencesChanged carries {}: another device of the account saved its preferences record.
const EventAccountPreferencesChanged = "account-preferences-changed"

// AccountPreferencesMaxSize bounds the sealed record, which holds the account's favorite item IDs.
const AccountPreferencesMaxSize = 1 << 20

// AccountPreferences is the account's preferences record, sealed with K_user; Revision 0 and no Ciphertext mean none was saved yet.
type AccountPreferences struct {
	Revision   int64  `json:"revision"`
	Ciphertext []byte `json:"ciphertext,omitempty"`
}

// PutAccountPreferencesRequest replaces the record if BaseRevision is still the current revision; otherwise 409.
type PutAccountPreferencesRequest struct {
	BaseRevision int64  `json:"baseRevision"`
	Ciphertext   []byte `json:"ciphertext"`
}

type PutAccountPreferencesResponse struct {
	Revision int64 `json:"revision"`
}
