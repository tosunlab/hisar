package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

const testExportPassphrase = "orange cactus harbor"

func buildTestHisarExport(vaults ...export.Vault) *export.Document {
	return export.Build(export.Account{Email: "old@example.com", Server: "https://old.example.com"}, vaults, time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC))
}

func exportLogin(title, password string) item.Item {
	return item.Item{Overview: item.Overview{Category: item.CategoryLogin, Title: title, Username: "alice"}, Details: item.Details{Password: password}}
}

func groupVaultIDs(preview ImportPreview) map[string]string {
	vaultIDs := map[string]string{}
	for _, group := range preview.Groups {
		vaultIDs[group.Name] = group.VaultID
	}
	return vaultIDs
}

// An encrypted export from this account goes back into the vaults it came from, by their IDs; a vault the account no
// longer has goes to Personal, and the file is kept.
func TestImportRestoresAnEncryptedHisarExportIntoTheVaultsItCameFrom(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	document := buildTestHisarExport(
		export.ConvertVault(test.personalID, "Personal", apitypes.VaultPersonal, []item.Item{exportLogin("Mail", "mail-pass")}),
		export.ConvertVault(test.familyID, "Family", apitypes.VaultShared, []item.Item{exportLogin("Router", "router-pass")}),
		export.ConvertVault("vault-gone", "Old team", apitypes.VaultShared, []item.Item{exportLogin("Wiki", "wiki-pass")}),
	)
	content, err := export.EncodeEncrypted(document, []byte(testExportPassphrase))
	if err != nil {
		t.Fatal(err)
	}
	path := writeImportFile(t, "Hisar export 2026-10-20.hisar", string(content))
	setImportPath(service, path)

	waiting, err := service.ChooseImportFile(test.personalID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !waiting.IsPassphraseRequired || waiting.FormatName != "Hisar encrypted export" || waiting.FileName != "Hisar export 2026-10-20.hisar" || waiting.ItemCount != 0 || len(waiting.Groups) != 0 {
		t.Fatalf("before the passphrase = %+v", waiting)
	}
	if _, err := service.StartImport(test.personalID, true); !errors.Is(err, export.ErrPassphraseRequired) {
		t.Errorf("start before the passphrase: %v", err)
	}
	if _, err := service.OpenImportFile("not the passphrase"); !errors.Is(err, export.ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}

	preview, err := service.OpenImportFile(testExportPassphrase)
	if err != nil {
		t.Fatalf("the right passphrase after a wrong one: %v", err)
	}
	want := map[string]string{"Personal": test.personalID, "Family": test.familyID, "Old team": test.personalID}
	if got := groupVaultIDs(preview); len(got) != 3 || got["Personal"] != want["Personal"] || got["Family"] != want["Family"] || got["Old team"] != want["Old team"] {
		t.Errorf("destinations = %v, want %v", got, want)
	}
	if preview.IsPassphraseRequired || preview.ItemCount != 3 || !preview.IsSourceKept || preview.ExportedFrom != "old@example.com" || preview.ExportedAt != "2026-10-20T09:30:00Z" || preview.Notice != "" {
		t.Errorf("preview = %+v", preview)
	}

	if _, err := service.StartImport(preview.VaultID, true); err != nil {
		t.Fatal(err)
	}
	finished, _ := runImportToEnd(t, service)
	if finished.SavedCount != 3 || !finished.IsSourceKept {
		t.Errorf("finished = %+v", finished)
	}
	if personal := listItemsByTitle(t, service, test.personalID); personal["Mail"].ID == "" || personal["Wiki"].ID == "" {
		t.Errorf("Personal = %v", personal)
	}
	if family := listItemsByTitle(t, service, test.familyID); family["Router"].ID == "" {
		t.Errorf("Family = %v", family)
	}
	if err := service.DeleteImportFile(); err == nil {
		t.Error("deleted an encrypted export")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the export is gone: %v", err)
	}
}

// A vault in the file restores only into a vault with its ID and type that this account may write to: an ID alone, as
// a file anyone can write may claim, never sends items into a vault of another kind.
func TestImportOfAHisarExportMatchesTheVaultTypeAndNeverTheName(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	document := buildTestHisarExport(
		export.ConvertVault(test.familyID, "Personal", apitypes.VaultPersonal, []item.Item{exportLogin("Mail", "mail-pass")}),
		export.ConvertVault("another-id", "Family", apitypes.VaultShared, []item.Item{exportLogin("Router", "router-pass")}),
	)
	content, err := export.EncodePlain(document)
	if err != nil {
		t.Fatal(err)
	}
	setImportPath(service, writeImportFile(t, "export.json", string(content)))
	preview, err := service.ChooseImportFile(test.personalID, FormatHisarExport)
	if err != nil {
		t.Fatal(err)
	}
	if got := groupVaultIDs(preview); got["Personal"] != test.personalID || got["Family"] != test.personalID {
		t.Errorf("destinations = %v", got)
	}
}

// A plain export with nothing left behind is offered for deletion like any other file.
func TestImportOfAPlainHisarExportDeletesTheFileAfterwards(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	content, err := export.EncodePlain(buildTestHisarExport(export.ConvertVault(test.personalID, "Personal", apitypes.VaultPersonal, []item.Item{exportLogin("Mail", "mail-pass")})))
	if err != nil {
		t.Fatal(err)
	}
	path := writeImportFile(t, "Hisar export (not encrypted).json", string(content))
	setImportPath(service, path)
	preview, err := service.ChooseImportFile(test.personalID, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.IsPassphraseRequired || preview.IsSourceKept || preview.FormatName != "Hisar export (plain JSON)" || preview.ItemCount != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := service.StartImport(test.personalID, true); err != nil {
		t.Fatal(err)
	}
	if finished, _ := runImportToEnd(t, service); finished.IsSourceKept || finished.LeftBehindCount != 0 {
		t.Errorf("finished = %+v", finished)
	}
	if err := service.DeleteImportFile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the plain export is still there: %v", err)
	}
}

// A phone hands the picked file's bytes over. A cached copy says so and is kept; a file that is no Hisar export is
// refused.
func TestPhoneImportOfAHisarExportShowsACachedCopy(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	document := buildTestHisarExport(export.ConvertVault(test.personalID, "Personal", apitypes.VaultPersonal, []item.Item{exportLogin("Mail", "mail-pass")}))
	document.Provenance = &export.Provenance{Source: "cached", Vaults: []export.VaultCheckpoint{}, Omissions: []session.ReadIssue{}}
	content, err := export.EncodePlain(document)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PrepareHisarExportImport(service, test.personalID, "backup.json", content)
	if err != nil {
		t.Fatal(err)
	}
	if preview.FileName != "backup.json" || preview.FilePath != "" || !preview.IsSourceKept || !strings.Contains(preview.Notice, "Cached copy") || preview.ItemCount != 1 {
		t.Errorf("preview = %+v", preview)
	}
	if _, err := PrepareHisarExportImport(service, test.personalID, "passwords.csv", []byte("name,url,username,password\n")); !errors.Is(err, export.ErrNotExport) {
		t.Errorf("a CSV: %v", err)
	}
}

func TestLockForgetsAnEncryptedExportWaitingForItsPassphrase(t *testing.T) {
	test := createLockedUnlockableService(t)
	service := test.service
	if _, err := service.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	content, err := export.EncodeEncrypted(buildTestHisarExport(), []byte(testExportPassphrase))
	if err != nil {
		t.Fatal(err)
	}
	if preview, err := PrepareHisarExportImport(service, test.personalID, "backup.hisar", content); err != nil || !preview.IsPassphraseRequired {
		t.Fatalf("prepare = %+v %v", preview, err)
	}
	service.Lock()
	if _, err := service.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenImportFile(testExportPassphrase); !errors.Is(err, ErrNoPendingImport) {
		t.Errorf("open after a lock: %v", err)
	}
}
