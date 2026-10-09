// Package appinfo finds the app a process belongs to, with its name and icon, for approval prompts.
package appinfo

import (
	"encoding/base64"
	"path/filepath"
	"strings"
)

type App struct {
	Name    string
	Path    string
	IconURL string
}

const maximumAncestorCount = 16

// FindApp walks from a process up its parents to the first one inside an app bundle; it returns false for none.
func FindApp(pid int, getProcessPath func(int) (string, error), getParentPID func(int) (int, error)) (App, bool) {
	for range maximumAncestorCount {
		if pid <= 1 {
			return App{}, false
		}
		if path, err := getProcessPath(pid); err == nil {
			if bundlePath := getOutermostBundlePath(path); bundlePath != "" {
				return describeBundle(bundlePath), true
			}
		}
		parentPID, err := getParentPID(pid)
		if err != nil || parentPID == pid {
			return App{}, false
		}
		pid = parentPID
	}
	return App{}, false
}

// getOutermostBundlePath returns the first ".app" directory in a path, so helper apps resolve to the app that ships them.
func getOutermostBundlePath(path string) string {
	index := strings.Index(path, ".app/")
	if index < 0 {
		return ""
	}
	return path[:index+len(".app")]
}

func describeBundle(bundlePath string) App {
	app := App{Path: bundlePath, Name: getBundleDisplayName(bundlePath)}
	if app.Name == "" {
		app.Name = strings.TrimSuffix(filepath.Base(bundlePath), ".app")
	}
	if icon, err := getBundleIconPNG(bundlePath, 128); err == nil {
		app.IconURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(icon)
	}
	return app
}
