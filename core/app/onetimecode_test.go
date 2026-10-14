package app

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/totp"
)

func TestOneTimePassword(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	currentTime := time.Date(2026, 10, 9, 12, 0, 10, 0, time.UTC)
	service.config.Now = func() time.Time { return currentTime }
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	pasted := strings.ToLower(secret[:8] + " " + secret[8:])

	saved, err := service.SaveItem(testService.personalID, ItemDetail{Title: "GitHub", Username: "octocat", OneTimePassword: pasted, Fields: []ItemField{}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.OneTimePassword != "otpauth://totp/?secret="+secret || !saved.HasOneTimePassword {
		t.Errorf("stored = %q", saved.OneTimePassword)
	}
	summaries, err := service.ListItems(testService.personalID)
	if err != nil || len(summaries) != 1 || summaries[0].OneTimePassword != "" || !summaries[0].HasOneTimePassword {
		t.Fatalf("summary = %+v %v", summaries, err)
	}

	config, _ := totp.Parse(saved.OneTimePassword)
	wantCode, wantExpiry := config.GenerateCode(currentTime)
	code, err := service.GetOneTimeCode(testService.personalID, saved.ID)
	if err != nil || code.Code != wantCode || code.ExpiresAt != wantExpiry.UTC().Format(time.RFC3339) || code.Period != 30 {
		t.Errorf("code = %+v %v, want %s until %v", code, err, wantCode, wantExpiry)
	}
	if err := service.CopyField(testService.personalID, saved.ID, "one-time code"); err != nil {
		t.Fatal(err)
	}
	if copied, _ := testService.clipboard.GetText(); len(copied) != 6 {
		t.Errorf("copied code = %q", copied)
	}

	if _, err := service.SaveItem(testService.personalID, ItemDetail{ID: saved.ID, Revision: saved.Revision, Title: "GitHub", OneTimePassword: "not a secret", Fields: []ItemField{}}); err == nil || !strings.Contains(err.Error(), "not a valid") {
		t.Errorf("invalid secret: %v", err)
	}
	cleared, err := service.SaveItem(testService.personalID, ItemDetail{ID: saved.ID, Revision: saved.Revision, Title: "GitHub", OneTimePassword: "  ", Fields: []ItemField{}})
	if err != nil || cleared.HasOneTimePassword {
		t.Fatalf("cleared: %+v %v", cleared, err)
	}
	if _, err := service.GetOneTimeCode(testService.personalID, saved.ID); err == nil {
		t.Error("a code for an item without a one-time password")
	}
}
