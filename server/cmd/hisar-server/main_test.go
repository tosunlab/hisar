package main

import (
	"strings"
	"testing"
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
	setUpdateEnvironment(t, "https://github.com/tosunlab/hisarapp-core", "", "")
	if _, err := createDesktopUpdateSource("https://hisar.test"); err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Errorf("repository URL accepted: %v", err)
	}
	setUpdateEnvironment(t, "tosunlab/hisarapp-core", "", "api.github.com")
	if _, err := createDesktopUpdateSource("https://hisar.test"); err == nil {
		t.Error("API URL without a scheme accepted")
	}
}

func setMailEnvironment(t *testing.T, host, port, username, password, from string) {
	t.Setenv("HISAR_SMTP_HOST", host)
	t.Setenv("HISAR_SMTP_PORT", port)
	t.Setenv("HISAR_SMTP_USERNAME", username)
	t.Setenv("HISAR_SMTP_PASSWORD", password)
	t.Setenv("HISAR_SMTP_FROM", from)
}

func TestCreateMailer(t *testing.T) {
	setMailEnvironment(t, "", "", "", "", "")
	if mailer, err := createMailer(); mailer != nil || err != nil {
		t.Errorf("unset: %v %v", mailer, err)
	}
	setMailEnvironment(t, "smtp.example.com", "587", "hisar", "secret", "Hisar <noreply@tryhisar.com>")
	if mailer, err := createMailer(); mailer == nil || err != nil {
		t.Errorf("configured: %v %v", mailer, err)
	}
	setMailEnvironment(t, "smtp.example.com", "587", "hisar", "", "noreply@tryhisar.com")
	if _, err := createMailer(); err == nil || !strings.Contains(err.Error(), "together") {
		t.Errorf("partial configuration: %v", err)
	}
	setMailEnvironment(t, "smtp.example.com", "submission", "hisar", "secret", "noreply@tryhisar.com")
	if _, err := createMailer(); err == nil {
		t.Error("port name accepted")
	}
	setMailEnvironment(t, "smtp.example.com", "587", "hisar", "secret", "not an address")
	if _, err := createMailer(); err == nil {
		t.Error("invalid sender accepted")
	}
}

func TestReadMinimumVersions(t *testing.T) {
	t.Setenv("HISAR_MINIMUM_VERSION_DESKTOP_MACOS", " 0.1.6 ")
	t.Setenv("HISAR_MINIMUM_VERSION_IOS", "")
	t.Setenv("HISAR_MINIMUM_VERSION_CLI", "1.0.0-alpha-test")
	t.Setenv("HISAR_MINIMUM_VERSION_EXTENSION", "0.2.0")
	minimumVersions := readMinimumVersions()
	if minimumVersions.DesktopMacOS != "0.1.6" || minimumVersions.IOS != "" || minimumVersions.CLI != "1.0.0-alpha-test" || minimumVersions.Extension != "0.2.0" {
		t.Errorf("read %+v", minimumVersions)
	}
	if err := minimumVersions.Validate(); err != nil {
		t.Error(err)
	}
	t.Setenv("HISAR_MINIMUM_VERSION_IOS", "1.2")
	if err := readMinimumVersions().Validate(); err == nil || !strings.Contains(err.Error(), "ios") {
		t.Errorf("invalid minimum: %v", err)
	}
}
