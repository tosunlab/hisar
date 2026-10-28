//go:build !hisar_fast_test_kdf

package crypt

// NewAccountArgon2Params are the parameters an account gets when it signs up or changes its credentials.
// scripts/check-fast-test-kdf.sh keeps the hisar_fast_test_kdf build tag, which lowers them, to test builds.
func NewAccountArgon2Params() Argon2Params { return DefaultArgon2Params() }
