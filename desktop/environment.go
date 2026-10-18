package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/tryhisar/hisar/core/appversion"
	"github.com/tryhisar/hisar/desktop/background"
)

func getDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "desktop"
	}
	if len(hostname) > 100 {
		return hostname[:100]
	}
	return hostname
}

// getSingleInstanceID lets a copy with its own HISAR_DESKTOP_STATE run next to the installed app.
func getSingleInstanceID(statePath string) string {
	if os.Getenv("HISAR_DESKTOP_STATE") == "" {
		return background.BundleID
	}
	statePathHash := sha256.Sum256([]byte(statePath))
	return background.BundleID + "." + hex.EncodeToString(statePathHash[:8])
}

// getAppVersion is the bundle's version, or the one the Linux packaging stamped; development builds send none, so no
// minimum refuses them.
func getAppVersion() string {
	version, _ := newAppUpdater().GetVersion()
	if _, err := appversion.Parse(version); err != nil {
		return ""
	}
	return version
}

func getLoginShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/zsh"
}
