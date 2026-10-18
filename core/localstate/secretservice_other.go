//go:build !linux

package localstate

// checkSecretService has nothing to check: macOS, Windows and iOS always have their keychain.
func checkSecretService() error { return nil }
