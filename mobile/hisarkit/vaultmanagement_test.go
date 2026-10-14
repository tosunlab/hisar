package hisarkit

import (
	"os"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/export"
)

// checkVaultManagementAndExport runs inside TestIntegrationMobileFlow against the real server; personalID holds one item.
func checkVaultManagementAndExport(t *testing.T, hisar *Hisar, personalID, password string) {
	t.Helper()
	trip := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(hisar.CreateVault("Trip"))
	decodeJSON[struct{}](t)(hisar.SaveItem(trip.ID, `{"title":"Hotel","password":"hotel-secret","urls":[],"fields":[{"name":"booking","value":"B-42","concealed":true}]}`))
	type vaultActions struct {
		CanDelete    bool `json:"canDelete"`
		CanRename    bool `json:"canRename"`
		TargetVaults []struct {
			ID string `json:"id"`
		} `json:"targetVaults"`
		DeleteBlockedReason string `json:"deleteBlockedReason"`
	}
	if actions := decodeJSON[vaultActions](t)(hisar.GetVaultActions(personalID)); actions.CanDelete || actions.DeleteBlockedReason == "" {
		t.Errorf("personal vault actions: %+v", actions)
	}
	if actions := decodeJSON[vaultActions](t)(hisar.GetVaultActions(trip.ID)); !actions.CanDelete || !actions.CanRename || len(actions.TargetVaults) != 1 || actions.TargetVaults[0].ID != personalID {
		t.Errorf("trip actions: %+v", actions)
	}
	if renamed := decodeJSON[struct {
		Name string `json:"name"`
	}](t)(hisar.RenameVault(trip.ID, "Trip 2026")); renamed.Name != "Trip 2026" {
		t.Errorf("renamed: %+v", renamed)
	}
	if moved := decodeJSON[struct {
		MovedCount int `json:"movedCount"`
	}](t)(hisar.DeleteVault(trip.ID, `{"moveToVaultId":"`+personalID+`"}`)); moved.MovedCount != 1 {
		t.Errorf("moved: %+v", moved)
	}
	items := decodeJSON[[]struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}](t)(hisar.ListAllItem(personalID))
	hotelIndex := slices.IndexFunc(items, func(listed struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}) bool {
		return listed.Title == "Hotel"
	})
	if len(items) != 2 || hotelIndex < 0 {
		t.Fatalf("personal items after the move: %+v", items)
	}
	if hotel := decodeJSON[struct {
		Password string `json:"password"`
	}](t)(hisar.GetItem(personalID, items[hotelIndex].ID)); hotel.Password != "hotel-secret" {
		t.Errorf("moved item: %+v", hotel)
	}
	exportDirectory := t.TempDir()
	if _, err := hisar.ExportAllData("encrypted", "a long export passphrase", "not the master password", exportDirectory); err == nil {
		t.Error("exported with a wrong master password")
	}
	exported := decodeJSON[struct {
		Path      string `json:"path"`
		ItemCount int    `json:"itemCount"`
	}](t)(hisar.ExportAllData("encrypted", "a long export passphrase", password, exportDirectory))
	content, err := os.ReadFile(exported.Path)
	if err != nil || exported.ItemCount != 2 {
		t.Fatalf("export: %+v %v", exported, err)
	}
	if document, err := export.Decode(content, []byte("a long export passphrase")); err != nil || document.CountItem() != 2 {
		t.Errorf("decode the iPhone export: %v", err)
	}
}

func TestVaultManagementWhileSignedOut(t *testing.T) {
	hisar, err := New(t.TempDir(), "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hisar.GetVaultActions("vault"); err == nil {
		t.Error("vault actions while signed out")
	}
	if _, err := hisar.RenameVault("vault", "Name"); err == nil {
		t.Error("a vault renamed while signed out")
	}
	if _, err := hisar.DeleteVault("vault", `{"isDeletingItems": true, "confirmedName": "Vault"}`); err == nil {
		t.Error("a vault deleted while signed out")
	}
	if _, err := hisar.DeleteVault("vault", `not json`); err == nil {
		t.Error("a malformed deletion request was accepted")
	}
	if err := hisar.LeaveVault("vault"); err == nil {
		t.Error("a vault left while signed out")
	}
	if notices, err := hisar.ListAllRemovedVault(); err != nil || notices != "[]" {
		t.Errorf("removed vaults while signed out: %q %v", notices, err)
	}
	if options := decodeJSON[struct {
		MinPassphraseLength int `json:"minPassphraseLength"`
	}](t)(hisar.GetExportOptions()); options.MinPassphraseLength != 10 {
		t.Errorf("export options: %+v", options)
	}
	exportDirectory := t.TempDir()
	if _, err := hisar.ExportAllData("encrypted", "a long export passphrase", "password", exportDirectory); err == nil {
		t.Error("exported while signed out")
	}
	if _, err := hisar.ExportAllData("encrypted", "short", "", exportDirectory); err == nil {
		t.Error("a short export passphrase was accepted")
	}
	if entries, _ := os.ReadDir(exportDirectory); len(entries) != 0 {
		t.Errorf("files written by failed exports: %v", entries)
	}
}
