package main

import "testing"

func setUpdateEnvironment(t *testing.T, gitLabURL, projectID, token string) {
	t.Setenv("HISAR_UPDATE_GITLAB_URL", gitLabURL)
	t.Setenv("HISAR_UPDATE_GITLAB_PROJECT_ID", projectID)
	t.Setenv("HISAR_UPDATE_GITLAB_TOKEN", token)
}

func TestCreateDesktopUpdateSource(t *testing.T) {
	setUpdateEnvironment(t, "", "", "")
	if source, err := createDesktopUpdateSource("https://hisar.test"); source != nil || err != nil {
		t.Errorf("unset: %v %v", source, err)
	}
	setUpdateEnvironment(t, "https://git.example.com", "2", "glpat-token")
	if source, err := createDesktopUpdateSource("https://hisar.test"); source == nil || err != nil {
		t.Errorf("configured: %v %v", source, err)
	}
	setUpdateEnvironment(t, "https://git.example.com", "2", "")
	if _, err := createDesktopUpdateSource("https://hisar.test"); err == nil {
		t.Error("partial configuration accepted")
	}
	setUpdateEnvironment(t, "git.example.com", "2", "glpat-token")
	if _, err := createDesktopUpdateSource("https://hisar.test"); err == nil {
		t.Error("URL without a scheme accepted")
	}
}
