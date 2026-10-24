package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScannerFindsPasswordInAgentOutput(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "agent.log")
	password := "HISAR-CANARY-test-password"
	if err := os.WriteFile(path, []byte("tool output "+password), 0o600); err != nil {
		t.Fatal(err)
	}
	scan := newScanner(canaries{Passwords: []string{password}}, "", filepath.Join(directory, "ocr"))
	if err := scan.scanPath(directory); err != nil {
		t.Fatal(err)
	}
	if len(scan.report.Scanned) != 1 || scan.report.Scanned[0].Bytes != int64(len("tool output "+password)) {
		t.Fatalf("scanned = %+v", scan.report.Scanned)
	}
	if len(scan.report.Matches) != 1 || scan.report.Matches[0].Kind != "login password" || scan.report.Matches[0].isAllowed() {
		t.Fatalf("matches = %+v", scan.report.Matches)
	}
}

func TestScannerLabelsOnlyNamedFixtureRecords(t *testing.T) {
	directory := t.TempDir()
	fixture := filepath.Join(directory, "site.jsonl")
	agent := filepath.Join(directory, "agent.log")
	password := "HISAR-CANARY-test-password"
	for _, path := range []string{fixture, agent} {
		if err := os.WriteFile(path, []byte(password), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scan := newScanner(canaries{Passwords: []string{password}}, "", filepath.Join(directory, "ocr"))
	scan.references = map[string]string{fixture: "fixture POST"}
	if err := scan.scanPath(directory); err != nil {
		t.Fatal(err)
	}
	if len(scan.report.Matches) != 2 {
		t.Fatalf("matches = %+v", scan.report.Matches)
	}
	for _, match := range scan.report.Matches {
		want := "file"
		if match.Path == fixture {
			want = "fixture POST"
		}
		if match.Medium != want {
			t.Errorf("%s medium = %q, want %q", match.Path, match.Medium, want)
		}
	}
}

func TestAgentChannelCodesFailOutsideImages(t *testing.T) {
	for _, medium := range []string{"file", "sqlite dump", "image", "embedded image", "video frame"} {
		found := match{Kind: "one-time code", Medium: medium}
		want := medium == "image" || medium == "embedded image" || medium == "video frame"
		if found.isAllowed() != want {
			t.Errorf("%s allowed = %v, want %v", medium, found.isAllowed(), want)
		}
		found.Kind = "login password"
		if found.isAllowed() {
			t.Errorf("password allowed in %s", medium)
		}
	}
}

func TestScannerDistinguishesCodeTokensFromNumericData(t *testing.T) {
	for _, test := range []struct {
		text string
		want bool
	}{
		{`{"code":"12345678"}`, true},
		{`{"code":12345678}`, true},
		{"Your code is 12345678.", true},
		{"87654321 12345678", true},
		{"0.12345678", false},
		{"12345678.25", false},
		{`X'ab12345678cd'`, false},
		{"item_12345678", false},
	} {
		t.Run(test.text, func(t *testing.T) {
			scan := &scanner{codes: map[string]bool{"12345678": true}}
			scan.find("output", "file", test.text)
			if found := len(scan.report.Matches) != 0; found != test.want {
				t.Errorf("found = %v, want %v", found, test.want)
			}
		})
	}
}

func TestBrowserProfileCodesAreReportedSeparately(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	profile := filepath.Join(root, "edge", "Web Data") + " (.dump)"
	agent := root + "-other/agent.log"
	report := scanReport{
		Scanned: []scanned{{Path: profile}, {Path: agent}},
		Matches: []match{
			{Path: profile, Medium: "sqlite dump", Kind: "one-time code"},
			{Path: profile, Medium: "sqlite dump", Kind: "login password"},
			{Path: agent, Medium: "file", Kind: "one-time code"},
		},
	}
	report.markBrowserProfiles(root)
	if report.Scanned[0].Category != "browser profile storage" || report.Scanned[1].Category != "" {
		t.Fatalf("scanned categories = %+v", report.Scanned)
	}
	if report.Matches[0].Category != "browser profile storage" || !report.Matches[0].isAllowed() {
		t.Errorf("browser code residual = %+v", report.Matches[0])
	}
	if report.Matches[1].isAllowed() {
		t.Error("a password in browser storage must fail")
	}
	if report.Matches[2].Category != "" || report.Matches[2].isAllowed() {
		t.Error("an adjacent folder must not bypass the agent-channel code gate")
	}
}

func TestScannerRedactsCodeContext(t *testing.T) {
	scan := &scanner{
		needles: map[string]string{"HISAR-CANARY-test": "login password"},
		codes:   map[string]bool{"12345678": true},
	}
	scan.find("output", "file", `INSERT INTO autofill VALUES('code','12345678','HISAR-CANARY-test');`)
	for _, found := range scan.report.Matches {
		if strings.Contains(found.Context, "12345678") || strings.Contains(found.Context, "HISAR-CANARY-test") {
			t.Fatal("the scanner report disclosed a canary")
		}
		if found.Kind == "one-time code" && !strings.Contains(found.Context, "autofill") {
			t.Error("the redacted context omitted the table that retained the code")
		}
	}
}
