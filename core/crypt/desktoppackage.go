package crypt

import (
	"crypto/ed25519"
	"crypto/sha256"
)

// desktopPackageLabel signs a Linux desktop package (.deb, .rpm, AppImage, Flatpak) of a Hisar release with the key
// that signs the Mac's Sparkle updates, so the app installs only a package Hisar's release made. The file name, which
// names the version, the architecture and the format, is signed with the content, so a signature for one file cannot
// pass for another.
const desktopPackageLabel = "hisar/v1/desktop-package"

func encodeDesktopPackage(fileName string, digest [sha256.Size]byte) []byte {
	return encodeContext(desktopPackageLabel, fileName, string(digest[:]))
}

// SignDesktopPackage signs a release file's name and SHA-256 with the update signing key.
func SignDesktopPackage(privateKey ed25519.PrivateKey, fileName string, digest [sha256.Size]byte) []byte {
	return ed25519.Sign(privateKey, encodeDesktopPackage(fileName, digest))
}

// VerifyDesktopPackage checks a downloaded release file against the update signing key the app was built with.
func VerifyDesktopPackage(publicKey ed25519.PublicKey, fileName string, digest [sha256.Size]byte, signature []byte) error {
	if len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(publicKey, encodeDesktopPackage(fileName, digest), signature) {
		return ErrBadSignature
	}
	return nil
}
