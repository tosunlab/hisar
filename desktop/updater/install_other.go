//go:build !linux

package updater

// DetectInstallation finds no installer off Linux: the Mac has Sparkle, and elsewhere the user downloads updates.
func DetectInstallation(quit func()) Installation {
	return Installation{}
}
