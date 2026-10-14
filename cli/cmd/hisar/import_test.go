package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
)

const testImportCSV = "name,url,username,password,note\n" +
	"GitHub,https://github.com,alice,imported-pass,\n" +
	"Bank,https://bank.example.com,alice,bank-pass,\"Branch: Main St\nCard in the safe\"\n" +
	"Recipe,,,,Secret sauce\n"

func TestPrintImportPreview(t *testing.T) {
	result, err := importer.Parse("export.csv", []byte(testImportCSV+"GitHub,https://github.com,alice,imported-pass,\n"), importer.FormatAutomatic)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	printImportPreview(&buffer, "export.csv", result, "Personal", []int{0})
	output := buffer.String()
	for _, want := range []string{
		"File:          export.csv",
		"Format:        CSV (Chrome, Firefox, Safari, Edge and others)",
		"Vault:         Personal",
		"Logins:        2",
		"Secure notes:  1",
		"SSH keys:      0",
		"Already in Personal (same title, username and website): 1\n  - GitHub, alice, https://github.com\n",
		"Not imported: 1\n  - GitHub: the same entry appears earlier in this file\n",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("preview misses %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "imported-pass") || strings.Contains(output, "bank-pass") {
		t.Errorf("preview shows a password:\n%s", output)
	}
}

func TestSelectAllQueuedItem(t *testing.T) {
	items := []item.Item{{Overview: item.Overview{Title: "A"}}, {Overview: item.Overview{Title: "B"}}, {Overview: item.Overview{Title: "C"}}}
	getTitles := func(queuedItems []item.Item) []string {
		titles := []string{}
		for _, vaultItem := range queuedItems {
			titles = append(titles, vaultItem.Overview.Title)
		}
		return titles
	}
	if titles := getTitles(selectQueuedItems(items, []int{0, 2}, true)); !reflect.DeepEqual(titles, []string{"B"}) {
		t.Errorf("skipping duplicates: %v", titles)
	}
	if titles := getTitles(selectQueuedItems(items, []int{0, 2}, false)); !reflect.DeepEqual(titles, []string{"A", "B", "C"}) {
		t.Errorf("including duplicates: %v", titles)
	}
}

func TestRunImportChecksTheFileBeforeUnlocking(t *testing.T) {
	t.Setenv("HISAR_STATE", filepath.Join(t.TempDir(), "state.json"))
	directory := t.TempDir()
	invalidPath := filepath.Join(directory, "passwords.csv")
	os.WriteFile(invalidPath, []byte("foo,bar\nHunter2-SecretValue,1\n"), 0o600)
	validPath := filepath.Join(directory, "export.csv")
	os.WriteFile(validPath, []byte(testImportCSV), 0o600)
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no file", []string{}, "usage"},
		{"two files", []string{validPath, invalidPath}, "usage"},
		{"missing file", []string{filepath.Join(directory, "missing.csv")}, "could not open"},
		{"directory", []string{directory}, "is a directory"},
		{"unknown format", []string{validPath, "-format", "keepass"}, "unknown format"},
		{"invalid file", []string{invalidPath}, "not a 1Password, Bitwarden or CSV export"},
		{"wrong explicit format", []string{validPath, "-format", "bitwarden-json"}, "Bitwarden"},
		{"valid file needs a login", []string{validPath, "-y"}, "not logged in"},
	}
	for _, testCase := range cases {
		flags := flag.NewFlagSet("import", flag.ContinueOnError)
		flags.SetOutput(&bytes.Buffer{})
		err := runImport(context.Background(), &standaloneBackend{}, flags, testCase.args)
		if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
			t.Errorf("%s: err = %v, want %q", testCase.name, err, testCase.wantErr)
		}
		if err != nil && strings.Contains(err.Error(), "Hunter2") {
			t.Errorf("%s: the error shows a value from the file: %v", testCase.name, err)
		}
	}
	if err := runImport(context.Background(), &standaloneBackend{}, flag.NewFlagSet("import", flag.ContinueOnError), []string{}); !errors.Is(err, errUsage) {
		t.Errorf("usage: %v", err)
	}
}

func TestOfferToDeleteImportFileWhenConfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.csv")
	os.WriteFile(path, []byte(testImportCSV), 0o600)
	if err := offerToDeleteImportFile(path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("-y deleted the export without asking: %v", err)
	}
}

func TestDotenvFileNames(t *testing.T) {
	for path, want := range map[string]bool{
		".env": true, "app/.env": true, ".env.production": true, "staging.env": true,
		"export.csv": false, "environment.json": false, ".envrc": false,
	} {
		if isDotenvFile(path, "") != want {
			t.Errorf("%s: %v", path, !want)
		}
	}
	if !isDotenvFile("settings.txt", dotenvFormat) || isDotenvFile(".env", "csv") {
		t.Error("an explicit format did not win")
	}
	if title := buildDefaultItemTitle("/work/shop/.env.production"); title != "shop production" {
		t.Errorf("default item name %q", title)
	}
	if title := buildDefaultItemTitle("/work/shop/.env"); title != "shop" {
		t.Errorf("default item name %q", title)
	}
}
