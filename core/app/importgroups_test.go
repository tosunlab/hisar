package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/session"
)

func TestImportPreservesSourceGroupsAndDefaultsToPersonal(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	source := `{"encrypted":false,"collections":[{"id":"ops","organizationId":"org","name":"Ops"}],"items":[{"type":2,"name":"Private note","notes":"private"},{"type":2,"name":"Team note","organizationId":"org","collectionIds":["ops"],"notes":"team"}]}`
	setImportPath(service, writeImportFile(t, "bitwarden.json", source))
	preview, err := service.ChooseImportFile(test.familyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Groups) != 2 || preview.Groups[0].VaultID != test.personalID || preview.Groups[1].VaultID != test.personalID {
		t.Fatalf("not Personal-first: %+v", preview)
	}
	if _, err := service.PreviewImport(test.familyID); err == nil {
		t.Fatal("legacy destination selector flattened several source groups")
	}
	preview, err = service.MapImportGroup(preview.Groups[1].ID, test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Groups[0].VaultID != test.personalID || preview.Groups[1].VaultID != test.familyID || len(preview.Groups[1].Members) == 0 {
		t.Fatalf("mapping or membership missing: %+v", preview.Groups)
	}
	if _, err := service.StartImport(preview.VaultID, false); err != nil {
		t.Fatal(err)
	}
	progress, _ := runImportToEnd(t, service)
	if progress.SavedCount != 2 {
		t.Fatalf("saved: %+v", progress)
	}
	private := listItemsByTitle(t, service, test.personalID)
	shared := listItemsByTitle(t, service, test.familyID)
	privateItem, privateErr := service.GetItem(test.personalID, private["Private note"].ID)
	sharedItem, sharedErr := service.GetItem(test.familyID, shared["Team note"].ID)
	if privateErr != nil || sharedErr != nil || privateItem.Notes != "private" || sharedItem.Notes != "team" {
		t.Fatalf("items went to wrong destination: %+v %+v, %v %v", privateItem, sharedItem, privateErr, sharedErr)
	}
	if _, found := shared["Private note"]; found {
		t.Fatal("private source item was shared")
	}
}

func TestImportStopsWhenReviewedDestinationMembersChange(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	setImportPath(service, writeImportFile(t, "source.csv", buildImportCSV(12)))
	if _, err := service.ChooseImportFile(test.personalID, ""); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewImport(test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartImport(preview.VaultID, false); err != nil {
		t.Fatal(err)
	}
	first, err := service.ContinueImport()
	if err != nil || first.SavedCount != 10 || first.IsDone {
		t.Fatalf("first batch: %+v %v", first, err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.accessesByVaultID[test.familyID] = append(test.apiServer.accessesByVaultID[test.familyID], apitypes.RotatedAccess{AccountID: "new-reader", Role: apitypes.VaultReader})
	test.apiServer.mutex.Unlock()
	for range 2 {
		if _, err := service.ContinueImport(); !errors.Is(err, session.ErrImportScopeChanged) {
			t.Fatalf("membership change paused the import as %v", err)
		}
	}
	if count := len(listItemsByTitle(t, service, test.familyID)); count != 10 {
		t.Fatalf("wrote more after scope changed: %d", count)
	}
	if err := service.DeleteImportFile(); err == nil || !strings.Contains(err.Error(), "finish") && !strings.Contains(err.Error(), "import") {
		t.Fatalf("source deletion accepted: %v", err)
	}
	service.CancelImport()
}

func TestCommandLineScopedSaveStopsWhenMembersChange(t *testing.T) {
	test := createCommandLineTest(t)
	service, vaultID := test.testService.service, test.testService.familyID
	scope := reviewedImportScope(t, service, vaultID)
	var saved cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, ImportScope: scope, Item: &cliprotocol.ItemChange{Title: "Environment", Password: "original"}}, &saved)
	test.testService.apiServer.mutex.Lock()
	test.testService.apiServer.accessesByVaultID[vaultID] = append(test.testService.apiServer.accessesByVaultID[vaultID], apitypes.RotatedAccess{AccountID: "new-reader", Role: apitypes.VaultReader})
	test.testService.apiServer.mutex.Unlock()
	refusal := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, ImportScope: scope, Item: &cliprotocol.ItemChange{ID: saved.Item.ID, Revision: saved.Item.Revision, Title: "Environment", Password: "unreviewed-new-value"}}, nil)
	if refusal == "" {
		t.Fatal("scoped update ignored the membership change")
	}
	current, err := service.GetItem(vaultID, saved.Item.ID)
	if err != nil || current.Password != "original" {
		t.Fatalf("unreviewed update persisted: %+v %v", current, err)
	}
}
