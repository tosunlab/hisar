package main

import (
	"strings"
	"testing"
)

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
