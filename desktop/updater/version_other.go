//go:build !darwin || !cgo

package updater

func getBundleVersion() (version, buildNumber string) { return "", "" }
