//go:build !unix

package signin

// isSystemExecutable is false where Hisar does not sign in to websites.
func isSystemExecutable(string) bool { return false }
