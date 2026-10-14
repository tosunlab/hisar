package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
)

func writeImportFile(t *testing.T, fileName, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildImportCSV(loginCount int) string {
	var builder strings.Builder
	builder.WriteString("name,url,username,password,note\n")
	builder.WriteString("GitHub,https://github.com,alice,imported-github-pass,\n")
	for index := 1; index <= loginCount; index++ {
		fmt.Fprintf(&builder, "Site %02d,https://site%02d.example.com,user%02d,pass-%02d,\n", index, index, index, index)
	}
	builder.WriteString("Wi-Fi,,,,\"Network: Home\nKey: under the mat\"\n")
	return builder.String()
}

func setImportPath(service *Service, path string) {
	service.config.FileOpener.(*fakeFileOpener).path = path
}

func runImportToEnd(t *testing.T, service *Service) (ImportProgress, int) {
	t.Helper()
	callCount := 0
	for {
		progress, err := service.ContinueImport()
		if err != nil {
			t.Fatal(err)
		}
		callCount++
		if progress.IsDone {
			return progress, callCount
		}
		if callCount > 100 {
			t.Fatal("import does not finish")
		}
	}
}

func listItemsByTitle(t *testing.T, service *Service, vaultID string) map[string]ItemDetail {
	t.Helper()
	items, err := service.ListItems(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	itemsByTitle := map[string]ItemDetail{}
	for _, listed := range items {
		itemsByTitle[listed.Title] = listed
	}
	return itemsByTitle
}

func TestImportPreviewSaveAndDeleteFile(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	if _, err := service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com/"}, Password: "existing-pass"}); err != nil {
		t.Fatal(err)
	}
	path := writeImportFile(t, "Chrome Passwords.csv", buildImportCSV(12))
	setImportPath(service, path)

	preview, err := service.ChooseImportFile(test.personalID, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.FileName != "Chrome Passwords.csv" || preview.FilePath != path || preview.Format != "csv" || !strings.HasPrefix(preview.FormatName, "CSV") {
		t.Errorf("preview file = %+v", preview)
	}
	if preview.VaultName != "Personal" || preview.ItemCount != 14 || preview.LoginCount != 13 || preview.SecureNoteCount != 1 || preview.SSHKeyCount != 0 {
		t.Errorf("preview counts = %+v", preview)
	}
	if len(preview.Duplicates) != 1 || preview.Duplicates[0] != (ImportDuplicate{Title: "GitHub", Username: "alice", URL: "https://github.com"}) {
		t.Errorf("duplicates = %+v", preview.Duplicates)
	}
	if len(preview.Skipped) != 0 {
		t.Errorf("skipped = %+v", preview.Skipped)
	}

	familyPreview, err := service.PreviewImport(test.familyID)
	if err != nil || familyPreview.VaultName != "Family" || len(familyPreview.Duplicates) != 0 || familyPreview.ItemCount != 14 {
		t.Errorf("family preview = %+v %v", familyPreview, err)
	}

	started, err := service.StartImport(test.personalID, true)
	if err != nil {
		t.Fatal(err)
	}
	if started.TotalCount != 13 || started.SavedCount != 0 || started.IsDone || started.VaultName != "Personal" {
		t.Errorf("started = %+v", started)
	}
	if _, err := service.PreviewImport(test.personalID); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("preview after start: %v", err)
	}
	if err := service.DeleteImportFile(); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("delete before the import finished: %v", err)
	}
	finished, callCount := runImportToEnd(t, service)
	if finished.SavedCount != 13 || finished.TotalCount != 13 || len(finished.Failed) != 0 || callCount != 2 {
		t.Errorf("finished = %+v after %d calls", finished, callCount)
	}

	itemsByTitle := listItemsByTitle(t, service, test.personalID)
	if len(itemsByTitle) != 14 {
		t.Errorf("vault has %d items", len(itemsByTitle))
	}
	if github, _ := service.GetItem(test.personalID, itemsByTitle["GitHub"].ID); github.Password != "existing-pass" {
		t.Errorf("the duplicate replaced the existing item: %q", github.Password)
	}
	site, _ := service.GetItem(test.personalID, itemsByTitle["Site 07"].ID)
	if site.Username != "user07" || site.Password != "pass-07" || site.URLs[0] != "https://site07.example.com" || site.Category != item.CategoryLogin {
		t.Errorf("imported login = %+v", site)
	}
	if note, _ := service.GetItem(test.personalID, itemsByTitle["Wi-Fi"].ID); note.Notes != "Network: Home\nKey: under the mat" {
		t.Errorf("imported note = %+v", note)
	}

	if err := service.DeleteImportFile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("export file still exists: %v", err)
	}
	if err := service.DeleteImportFile(); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("second delete: %v", err)
	}
}

func TestImportIncludesDuplicatesWhenAsked(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	if _, err := service.SaveItem(test.familyID, ItemDetail{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}}); err != nil {
		t.Fatal(err)
	}
	setImportPath(service, writeImportFile(t, "export.csv", buildImportCSV(1)))
	if _, err := service.ChooseImportFile(test.familyID, "csv"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartImport(test.familyID, false); err != nil {
		t.Fatal(err)
	}
	if finished, _ := runImportToEnd(t, service); finished.SavedCount != 3 {
		t.Errorf("finished = %+v", finished)
	}
	items, _ := service.ListItems(test.familyID)
	if len(items) != 4 {
		t.Errorf("family vault has %d items, want both GitHub items", len(items))
	}
	if _, err := os.Stat(service.config.FileOpener.(*fakeFileOpener).path); err != nil {
		t.Errorf("export file removed without asking: %v", err)
	}
}

func TestImportCancelledFileDialog(t *testing.T) {
	test := createUnlockedTestService(t)
	preview, err := test.service.ChooseImportFile(test.personalID, "")
	if err != nil || preview.FileName != "" {
		t.Errorf("cancelled dialog = %+v %v", preview, err)
	}
	if _, err := test.service.StartImport(test.personalID, true); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("start without a file: %v", err)
	}
	if _, err := test.service.ContinueImport(); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("continue without a file: %v", err)
	}
}

func TestImportRejectsReadOnlyVaultAndUnknownFormat(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	setImportPath(service, writeImportFile(t, "export.csv", buildImportCSV(1)))
	if _, err := service.ChooseImportFile(test.personalID, "keepass"); err == nil || !strings.Contains(err.Error(), "supported import formats") {
		t.Errorf("unknown format: %v", err)
	}
	if _, err := service.ChooseImportFile("missing", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing vault: %v", err)
	}
	if _, err := service.ChooseImportFile(test.personalID, ""); err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	service.vaultsByID[test.familyID].Role = apitypes.VaultReader
	service.mutex.Unlock()
	if _, err := service.PreviewImport(test.familyID); !errors.Is(err, errReadOnlyVault) {
		t.Errorf("preview for a read-only vault: %v", err)
	}
	if _, err := service.StartImport(test.familyID, true); !errors.Is(err, errReadOnlyVault) {
		t.Errorf("import into a read-only vault: %v", err)
	}
	if _, err := service.ChooseImportFile(test.familyID, ""); !errors.Is(err, errReadOnlyVault) {
		t.Errorf("choose for a read-only vault: %v", err)
	}
}

func TestImportReportsInvalidFileWithoutValues(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	setImportPath(service, writeImportFile(t, "broken.csv", "title,username,password\nBank,alice,\"Hunter2-SecretValue\n"))
	_, err := service.ChooseImportFile(test.personalID, "")
	if err == nil || strings.Contains(err.Error(), "Hunter2") {
		t.Errorf("invalid file: %v", err)
	}
	setImportPath(service, filepath.Join(t.TempDir(), "missing.csv"))
	if _, err := service.ChooseImportFile(test.personalID, ""); err == nil || !strings.Contains(err.Error(), "missing.csv") {
		t.Errorf("missing file: %v", err)
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.pendingImport != nil {
		t.Error("a failed file left a pending import")
	}
}

func TestLockDropsPendingImport(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	setImportPath(service, writeImportFile(t, "export.csv", buildImportCSV(2)))
	if _, err := service.ChooseImportFile(test.personalID, ""); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	service.mutex.Lock()
	pending := service.pendingImport
	service.mutex.Unlock()
	if pending != nil {
		t.Error("the parsed export survived locking")
	}
	if _, err := service.StartImport(test.personalID, true); !errors.Is(err, ErrLocked) {
		t.Errorf("start after lock: %v", err)
	}
}

func TestCancelImport(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	setImportPath(service, writeImportFile(t, "export.csv", buildImportCSV(2)))
	if _, err := service.ChooseImportFile(test.personalID, ""); err != nil {
		t.Fatal(err)
	}
	service.CancelImport()
	if _, err := service.StartImport(test.personalID, true); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("start after cancel: %v", err)
	}
}

func TestImportPausesWhileOffline(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	setImportPath(service, writeImportFile(t, "export.csv", buildImportCSV(14)))
	if _, err := service.ChooseImportFile(test.personalID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartImport(test.personalID, true); err != nil {
		t.Fatal(err)
	}
	if progress, err := service.ContinueImport(); err != nil || progress.SavedCount != 10 {
		t.Fatalf("first batch = %+v %v", progress, err)
	}
	test.apiServer.setUnreachable(true)
	if _, err := service.ContinueImport(); !errors.Is(err, ErrOffline) {
		t.Errorf("offline batch: %v", err)
	}
	test.apiServer.setUnreachable(false)
	finished, _ := runImportToEnd(t, service)
	if finished.SavedCount != 16 || finished.TotalCount != 16 || len(finished.Failed) != 0 {
		t.Errorf("finished = %+v", finished)
	}
	if items, _ := service.ListItems(test.personalID); len(items) != 16 {
		t.Errorf("vault has %d items after resuming", len(items))
	}
}

func TestListAllImportFormat(t *testing.T) {
	test := createUnlockedTestService(t)
	formats := test.service.ListImportFormats()
	if len(formats) != 5 || formats[0].ID != "1pux" || formats[4].ID != "csv" {
		t.Errorf("formats = %+v", formats)
	}
}
