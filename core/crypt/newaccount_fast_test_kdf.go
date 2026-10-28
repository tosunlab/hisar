//go:build hisar_fast_test_kdf

package crypt

// NewAccountArgon2Params, in test builds only (make test, and the HisarKit the CI phone UI tests run), gives new accounts
// the minimum parameters: every test signs up and unlocks accounts, and 24 passes over 64 MiB made the hashing a large
// share of the tests' time. Accounts keep the parameters they were created with, so this never touches a real account.
// scripts/check-fast-test-kdf.sh fails when the tag appears outside the files allowed to use it.
func NewAccountArgon2Params() Argon2Params { return MinArgon2Params() }
