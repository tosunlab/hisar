package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
)

func TestCLIImportMapsEachSourceAndShowsSharedMembers(t *testing.T) {
	app := startVaultDataApp(t)
	path := filepath.Join(t.TempDir(), "source.json")
	source := `{"encrypted":false,"collections":[{"id":"ops","organizationId":"org","name":"Ops"}],"items":[{"type":2,"name":"Private note","notes":"private"},{"type":2,"name":"Team note","organizationId":"org","collectionIds":["ops"],"notes":"team"}]}`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := runCapturingOutput(t, false, "", "import", path, "--dry-run")
	if err != nil || !strings.Contains(preview, "Private note") && !strings.Contains(preview, "Bitwarden Personal") || !strings.Contains(preview, `bitwarden/["org",["ops"]]`) {
		t.Fatalf("preview: %s %v", preview, err)
	}
	if len(app.findRequest(cliprotocol.TypeImportItem)) != 0 {
		t.Fatal("dry run wrote")
	}
	_, output, err := runCapturingOutput(t, false, "", "import", path, "-y", "--map", `bitwarden/["org",["ops"]]=work-id`)
	if err != nil || !strings.Contains(output, "ada@example.com") {
		t.Fatalf("explicit shared mapping omitted scope: %s %v", output, err)
	}
	imports := app.findRequest(cliprotocol.TypeImportItem)
	if len(imports) != 2 || imports[0].VaultID != "personal-id" || imports[0].ImportedItems[0].Overview.Title != "Private note" || imports[1].VaultID != "work-id" || imports[1].ImportedItems[0].Overview.Title != "Team note" || imports[0].ImportScope == "" || imports[1].ImportScope == "" {
		t.Fatalf("flattened or unprotected writes: %+v", imports)
	}
	if _, _, err := runCapturingOutput(t, false, "", "import", path, "-y", "-v", "Work"); err == nil {
		t.Fatal("single selector flattened several groups")
	}
}

func TestHisarRestorePrefersIDsNeverSameNames(t *testing.T) {
	app := startVaultDataApp(t)
	mode := chooseMode(false)
	if mode.client == nil {
		t.Fatal("no app backend")
	}
	defer mode.client.Close()
	access := appBackend{client: mode.client}
	document := export.Build(export.Account{}, []export.Vault{export.ConvertVault("work-id", "Renamed at export", "shared", []item.Item{{Overview: item.Overview{Title: "Deploy"}}})}, time.Now())
	plans, err := planHisarImport(context.Background(), access, document, hisarImportOption{})
	if err != nil || len(plans) != 1 || plans[0].target.ID != "work-id" {
		t.Fatalf("stable ID ignored: %+v %v", plans, err)
	}
	document.Vaults[0].ID = "different-source-id"
	document.Vaults[0].Name = "Work"
	plans, err = planHisarImport(context.Background(), access, document, hisarImportOption{})
	if err != nil || plans[0].target.ID != "personal-id" {
		t.Fatalf("same name silently shared: %+v %v", plans, err)
	}
	document.Vaults[0].ID = "work-id"
	plans, err = planHisarImport(context.Background(), access, document, hisarImportOption{isConfirmed: true})
	if err != nil || plans[0].target.ID != "personal-id" {
		t.Fatalf("-y implicitly shared: %+v %v", plans, err)
	}
	plans, err = planHisarImport(context.Background(), access, document, hisarImportOption{isConfirmed: true, mappings: []string{"work-id=work-id"}})
	if err != nil || plans[0].target.ID != "work-id" {
		t.Fatalf("explicit shared restore unusable: %+v %v", plans, err)
	}
	if len(app.findRequest(cliprotocol.TypeCreateVault)) != 0 {
		t.Fatal("restore auto-created a vault")
	}
}

func TestHisarRestoreOfAnExportWithoutVaultsImportsNothing(t *testing.T) {
	app := startVaultDataApp(t)
	source, err := export.EncodePlain(&export.Document{Format: export.FormatPlain, Version: export.Version})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, output, err := runCapturingOutput(t, false, "", "import", path, "-y"); err != nil || !strings.Contains(output, "Nothing to import") {
		t.Fatalf("empty restore: %s %v", output, err)
	}
	if len(app.findRequest(cliprotocol.TypeImportItem)) != 0 {
		t.Fatal("empty restore wrote")
	}
}
