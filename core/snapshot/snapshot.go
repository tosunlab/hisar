// Package snapshot names a vault's published ciphertext snapshot for core/session without importing core/cache, so
// what imports the session (the admin dashboard's wasm build) never links SQLite.
package snapshot

import "github.com/tryhisar/hisar/core/apitypes"

// Checkpoint describes the published ciphertext snapshot, not the latest server state.
type Checkpoint struct {
	HasCompletedSync  bool   `json:"hasCompletedSync"`
	CompletedSequence int64  `json:"completedSequence"`
	CompletedAt       string `json:"completedAt"`
	KeyGeneration     int64  `json:"keyGeneration"`
}

// Staging owns a vault's unpublished changes. Closing or process death rolls them back.
type Staging interface {
	Apply(items []apitypes.Item) error
	// Commit publishes the staged items, the vault record whose key they are sealed with and the completion checkpoint
	// together.
	Commit(sequence int64, vault *apitypes.Vault) error
	Rollback()
}
