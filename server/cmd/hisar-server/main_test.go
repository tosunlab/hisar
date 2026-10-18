package main

import (
	"strings"
	"testing"

	"github.com/tryhisar/hisar/server/internal/desktopupdate"
)

func setUpdateEnvironment(t *testing.T, repository, token, apiURL string) {
	t.Setenv("HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY", repository)
	t.Setenv("HISAR_DESKTOP_UPDATE_GITHUB_TOKEN", token)
	t.Setenv("HISAR_DESKTOP_UPDATE_GITHUB_API_URL", apiURL)
}

func TestCreateDesktopUpdateSource(t *testing.T) {
	setUpdateEnvironment(t, "", "", "")
	if source, err := createDesktopUpdateSource("https://hisar.test"); source == nil || err != nil {
		t.Errorf("default repository: %v %v", source, err)
	}
	setUpdateEnvironment(t, "off", "", "")
	if source, err := createDesktopUpdateSource("https://hisar.test"); source != nil || err != nil {
		t.Errorf("off: %v %v", source, err)
	}
	setUpdateEnvironment(t, "example/fork", "github_pat_token", "https://github.example.com/api/v3")
	if source, err := createDesktopUpdateSource("https://hisar.test"); source == nil || err != nil {
		t.Errorf("configured: %v %v", source, err)
	}
	setUpdateEnvironment(t, "https://github.com/"+desktopupdate.DefaultGitHubRepository, "", "")
	if _, err := createDesktopUpdateSource("https://hisar.test"); err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Errorf("repository URL accepted: %v", err)
	}
	setUpdateEnvironment(t, desktopupdate.DefaultGitHubRepository, "", "api.github.com")
	if _, err := createDesktopUpdateSource("https://hisar.test"); err == nil {
		t.Error("API URL without a scheme accepted")
	}
}
