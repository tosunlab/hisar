package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sshkey"
)

type retentionBackend struct {
	backend
	items []item.Item
}

func (*retentionBackend) listVaults(context.Context) ([]cliprotocol.Vault, error) {
	return []cliprotocol.Vault{{ID: "personal", Name: "Personal", Type: "personal", Role: "owner"}}, nil
}
func (access *retentionBackend) listItems(context.Context, string) ([]item.Item, error) {
	if access.items != nil {
		return access.items, nil
	}
	return []item.Item{{Overview: item.Overview{Category: item.CategoryLogin, Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}}, Details: item.Details{Password: "old-password"}}}, nil
}
func (*retentionBackend) listsItemSummaries() bool { return false }
func (*retentionBackend) importItems(_ context.Context, _ string, items []item.Item, _ ...string) (int, error) {
	return len(items), nil
}

func (*retentionBackend) getVaultActions(context.Context, string) (cliprotocol.VaultActionsResult, error) {
	return cliprotocol.VaultActionsResult{ImportScope: "reviewed-personal-scope"}, nil
}

func TestImportRetainsDifferentPasswordDuplicateInCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.csv")
	if err := os.WriteFile(path, []byte("name,url,username,password\nGitHub,https://github.com,alice,new-password\nOther,https://other.example,bob,another-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	original := os.Stderr
	os.Stderr = output
	defer func() { os.Stderr = original }()
	err = runImport(context.Background(), &retentionBackend{}, flag.NewFlagSet("import", flag.ContinueOnError), []string{"-y", path})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "1 of its entries are not in Hisar") || strings.Contains(string(content), "Delete it and any other copies") {
		t.Fatalf("unsafe deletion advice: %s", content)
	}
}

// Nothing is queued, but an entry of the file is not in Hisar: a duplicate whose content differs, or an entry Hisar
// cannot import beside an identical duplicate. The CLI still says to keep the file.
func TestImportWithNothingToImportStillSaysToKeepTheFile(t *testing.T) {
	for name, source := range map[string]string{
		"different.csv":    "name,url,username,password\nGitHub,https://github.com,alice,new-password\n",
		"unsupported.json": `{"encrypted":false,"items":[{"type":1,"name":"GitHub","login":{"username":"alice","password":"old-password","uris":[{"uri":"https://github.com"}]}},{"type":99,"name":"Unknown"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			original := os.Stderr
			os.Stderr = output
			defer func() { os.Stderr = original }()
			if err := runImport(context.Background(), &retentionBackend{}, flag.NewFlagSet("import", flag.ContinueOnError), []string{"-y", path}); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), "Nothing to import.") || !strings.Contains(string(content), "Keep "+path+": 1 of its entries are not in Hisar.") {
				t.Fatalf("no keep-the-file warning:\n%s", content)
			}
		})
	}
}

func TestImportIdenticalStarredAndSSHContentInCLI(t *testing.T) {
	key, err := sshkey.GenerateEd25519Key("")
	if err != nil {
		t.Fatal(err)
	}
	encodedKey, err := json.Marshal(string(key.PrivateKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"starred.csv": "name,url,username,password,favorite\nGitHub,https://github.com,alice,password,1\nOther,https://other.example,bob,another-password,\n",
		"ssh.json":    `{"encrypted":false,"items":[{"type":5,"name":"Deploy","sshKey":{"privateKey":` + string(encodedKey) + `}},{"type":1,"name":"Other","login":{"password":"another-password"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := importer.Parse(name, []byte(source), "")
			if err != nil || len(parsed.Items) != 2 {
				t.Fatalf("parsed source = %+v, %v", parsed, err)
			}
			existing := parsed.Items[:1]
			existing[0].IsFavorite = false
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			original := os.Stderr
			os.Stderr = output
			defer func() { os.Stderr = original }()
			if err := runImport(context.Background(), &retentionBackend{items: existing}, flag.NewFlagSet("import", flag.ContinueOnError), []string{"-y", path}); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(content), "are not in Hisar") || !strings.Contains(string(content), "Imported 1 item") {
				t.Fatalf("identical duplicate reported as unsaved: %s", content)
			}
		})
	}
}

type summaryRetentionBackend struct {
	appBackend
	items []cliprotocol.Item
	saved int
}

func (*summaryRetentionBackend) listVaults(context.Context) ([]cliprotocol.Vault, error) {
	return []cliprotocol.Vault{{ID: "personal", Name: "Personal", Type: "personal", Role: "owner"}}, nil
}

func (access *summaryRetentionBackend) listItems(context.Context, string) ([]item.Item, error) {
	items := make([]item.Item, 0, len(access.items))
	for _, summary := range access.items {
		items = append(items, convertFromAppItem(summary))
	}
	return items, nil
}

func (access *summaryRetentionBackend) importItems(_ context.Context, _ string, items []item.Item, _ ...string) (int, error) {
	access.saved += len(items)
	return len(items), nil
}

func (*summaryRetentionBackend) getVaultActions(context.Context, string) (cliprotocol.VaultActionsResult, error) {
	return cliprotocol.VaultActionsResult{ImportScope: "reviewed-personal-scope"}, nil
}

func TestImportAppSummariesKeepUncomparedDuplicates(t *testing.T) {
	for _, test := range []struct {
		name        string
		extraRow    string
		include     bool
		interactive bool
		hisarExport bool
		wantSaved   int
		wantRetain  bool
	}{
		{"duplicate only", "", false, false, false, 0, true},
		{"duplicate and new item", "Other,https://other.example,bob,another-password\n", false, false, false, 1, true},
		{"include duplicates", "", true, false, false, 1, false},
		{"interactive skipped duplicate", "Other,https://other.example,bob,another-password\n", false, true, false, 1, true},
		{"Hisar duplicate only", "", false, false, true, 0, true},
		{"Hisar duplicate and new item", "Other,https://other.example,bob,another-password\n", false, false, true, 1, true},
		{"Hisar include duplicates", "", true, false, true, 1, false},
		{"Hisar interactive skipped duplicate", "Other,https://other.example,bob,another-password\n", false, true, true, 1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("name,url,username,password\nGitHub,https://github.com,alice,password\n" + test.extraRow)
			path := filepath.Join(t.TempDir(), "source.csv")
			if test.hisarExport {
				parsed, err := importer.Parse("source.csv", source, importer.FormatAutomatic)
				if err != nil {
					t.Fatal(err)
				}
				source, err = export.EncodePlain(&export.Document{
					Format: export.FormatPlain, Version: export.Version,
					Vaults: []export.Vault{export.ConvertVault("source", "Personal", "personal", parsed.Items)},
				})
				if err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(t.TempDir(), "source.json")
			}
			if err := os.WriteFile(path, source, 0o600); err != nil {
				t.Fatal(err)
			}
			var summaries []cliprotocol.Item
			if err := json.Unmarshal([]byte(`[{"id":"github","revision":"r1","category":"login","title":"GitHub","username":"alice","urls":["https://github.com"],"hasOneTimePassword":true}]`), &summaries); err != nil {
				t.Fatal(err)
			}
			access := &summaryRetentionBackend{items: summaries}
			output, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			original := os.Stderr
			os.Stderr = output
			defer func() { os.Stderr = original }()
			args := []string{"-y", path}
			if test.interactive {
				args = []string{path}
				originalReader := stdinReader
				stdinReader = bufio.NewReader(strings.NewReader("n\ny\ny\n"))
				defer func() { stdinReader = originalReader }()
			}
			if test.include {
				args = append(args, "-include-duplicates")
			}
			if err := runImport(context.Background(), access, flag.NewFlagSet("import", flag.ContinueOnError), args); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			text := string(content)
			if strings.Contains(text, "different password") || strings.Contains(text, "are not in Hisar") ||
				!strings.Contains(text, "could not be compared") {
				t.Errorf("misleading summary comparison: %s", text)
			}
			if access.saved != test.wantSaved {
				t.Errorf("saved %d items, want %d", access.saved, test.wantSaved)
			}
			if test.wantRetain {
				if !strings.Contains(text, "Keep "+path) || strings.Contains(text, "Delete it") {
					t.Errorf("unverified duplicate source not retained: %s", text)
				}
				if !test.hisarExport && !strings.Contains(text, "1 item could not be saved or verified in Hisar") {
					t.Errorf("retention message has incorrect item count: %s", text)
				}
			} else if !strings.Contains(text, "Delete it and any other copies") {
				t.Errorf("fully imported source has no completion advice: %s", text)
			}
			if retained, err := os.ReadFile(path); err != nil || string(retained) != string(source) {
				t.Errorf("source changed: %q, %v", retained, err)
			}
		})
	}
}

type shortImportBackend struct {
	retentionBackend
}

func (*shortImportBackend) importItems(context.Context, string, []item.Item, ...string) (int, error) {
	return 1, nil
}

func TestHisarPlainImportKeepsEntriesLeftBehind(t *testing.T) {
	for _, test := range []struct {
		name    string
		access  backend
		answers string
		source  string
	}{
		{"changed duplicate", &retentionBackend{}, "n\ny\ny\n", "GitHub,https://github.com,alice,new-password\nOther,https://other.example,bob,password\n"},
		{"short save", &shortImportBackend{}, "y\ny\n", "First,https://first.example,alice,password\nOther,https://other.example,bob,password\n"},
		{"identical duplicate", &retentionBackend{}, "n\ny\ny\n", "GitHub,https://github.com,alice,old-password\nOther,https://other.example,bob,password\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := importer.Parse("source.csv", []byte("name,url,username,password\n"+test.source), importer.FormatAutomatic)
			if err != nil {
				t.Fatal(err)
			}
			source, err := export.EncodePlain(&export.Document{
				Format: export.FormatPlain, Version: export.Version,
				Vaults: []export.Vault{export.ConvertVault("source", "Personal", "personal", parsed.Items)},
			})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source.json")
			if err := os.WriteFile(path, source, 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			originalStderr, originalReader := os.Stderr, stdinReader
			os.Stderr, stdinReader = output, bufio.NewReader(strings.NewReader(test.answers))
			defer func() { os.Stderr, stdinReader = originalStderr, originalReader }()
			if err := runImport(context.Background(), test.access, flag.NewFlagSet("import", flag.ContinueOnError), []string{path}); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "identical duplicate" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("complete import did not offer deletion: %s, %v", content, err)
				}
				return
			}
			if retained, err := os.ReadFile(path); err != nil || string(retained) != string(source) {
				t.Errorf("partial import removed or changed source: %v\n%s", err, content)
			}
			if strings.Contains(string(content), "Delete this file") || !strings.Contains(string(content), "Keep "+path) {
				t.Errorf("partial import offered deletion: %s", content)
			}
		})
	}
}
