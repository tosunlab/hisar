package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
)

func setInteractiveTerminal(t *testing.T, isInteractive bool) {
	t.Helper()
	original := isInteractiveTerminal
	isInteractiveTerminal = func() bool { return isInteractive }
	t.Cleanup(func() { isInteractiveTerminal = original })
}

func answerVaultDataRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	switch request.Type {
	case cliprotocol.TypeListVault:
		return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{
			{ID: "personal-id", Name: "Personal", Type: "personal", Role: "owner", ItemCount: 2},
			{ID: "work-id", Name: "Work", Type: "shared", Role: "owner", ItemCount: 3},
			{ID: "family-id", Name: "Family", Type: "shared", Role: "reader", ItemCount: 1},
		}}, nil
	case cliprotocol.TypeGetVaultActions:
		switch request.VaultID {
		case "personal-id":
			return cliprotocol.VaultActionsResult{Vault: cliprotocol.Vault{ID: "personal-id", Name: "Personal", ItemCount: 2}, CanRename: true,
				DeleteBlockedReason: "Your Personal vault can't be deleted.", LeaveBlockedReason: "You can't leave your Personal vault."}, nil
		case "family-id":
			return cliprotocol.VaultActionsResult{Vault: cliprotocol.Vault{ID: "family-id", Name: "Family", ItemCount: 1}, CanLeave: true,
				DeleteBlockedReason: "Only an owner of this vault can delete it. You can leave it instead."}, nil
		}
		return cliprotocol.VaultActionsResult{
			Vault: cliprotocol.Vault{ID: "work-id", Name: "Work", ItemCount: 3}, CanRename: true, CanDelete: true,
			LeaveBlockedReason:    "You are its only owner.",
			AllMemberLosingAccess: []cliprotocol.VaultMember{{Name: "Ada", Email: "ada@example.com", Role: "editor"}},
			AllTargetVault:        []cliprotocol.Vault{{ID: "personal-id", Name: "Personal", Role: "owner"}},
		}, nil
	case cliprotocol.TypeRenameVault, cliprotocol.TypeLeaveVault:
		return struct{}{}, nil
	case cliprotocol.TypeDeleteVault:
		if request.VaultDeletion.MoveToVaultID != "" {
			return cliprotocol.VaultDeletionResult{MovedCount: 3}, nil
		}
		return cliprotocol.VaultDeletionResult{}, nil
	case cliprotocol.TypeExport:
		return cliprotocol.ExportResult{Path: request.Export.Path, VaultCount: 3, ItemCount: 6}, nil
	case cliprotocol.TypeListItem:
		if request.VaultID == "work-id" {
			return cliprotocol.ItemListResult{Items: []cliprotocol.Item{{ID: "shop-id", Revision: "r1", Title: "shop"}}}, nil
		}
		return cliprotocol.ItemListResult{Items: []cliprotocol.Item{{ID: "github-id", Revision: "r1", Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}}}}, nil
	case cliprotocol.TypeGetItem:
		return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "shop-id", Revision: "r1", Title: "shop", Fields: []cliprotocol.Field{{Name: "STRIPE_KEY", Value: "old", Concealed: true}, {Name: "region", Value: "eu"}}}}, nil
	case cliprotocol.TypeSaveItem:
		return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "saved-id", Revision: "r2", Title: request.Item.Title}}, nil
	}
	return answerFakeVaultRequest(request)
}

func startVaultDataApp(t *testing.T) *fakeApp {
	t.Helper()
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = answerVaultDataRequest
	app.mutex.Unlock()
	return app
}

func TestVaultCommandsThroughApp(t *testing.T) {
	app := startVaultDataApp(t)
	setInteractiveTerminal(t, false)

	if _, stderr, err := runCapturingOutput(t, false, "", "vault", "rename", "Work", "Work 2026"); err != nil || !strings.Contains(stderr, `Renamed "Work" to "Work 2026"`) {
		t.Fatalf("rename: %v %s", err, stderr)
	}
	if renames := app.findRequest(cliprotocol.TypeRenameVault); len(renames) != 1 || renames[0].VaultID != "work-id" || renames[0].Name != "Work 2026" {
		t.Errorf("rename request: %+v", renames)
	}

	if _, _, err := runCapturingOutput(t, false, "", "vault", "delete", "Personal", "-y"); err == nil || !strings.Contains(err.Error(), "Personal vault can't be deleted") {
		t.Errorf("deleting the personal vault: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "delete", "Work", "-y"); err == nil || !strings.Contains(err.Error(), "--move-to") {
		t.Errorf("deleting a vault with items without a choice: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "delete", "Work", "--delete-items"); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Errorf("deleting items without the typed name: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "delete", "Work", "--delete-items", "--confirm", "work"); err == nil || !strings.Contains(err.Error(), "not the vault's name") {
		t.Errorf("deleting items with a wrong name: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "delete", "Work", "--move-to", "Personal"); err != errNeedsConfirmation {
		t.Errorf("moving without -y in a script: %v", err)
	}
	if len(app.findRequest(cliprotocol.TypeDeleteVault)) != 0 {
		t.Fatal("a refused deletion reached the app")
	}
	_, stderr, err := runCapturingOutput(t, false, "", "vault", "delete", "Work", "--move-to", "Personal", "-y")
	if err != nil || !strings.Contains(stderr, "These people lose access:\n  Ada <ada@example.com>, editor") || !strings.Contains(stderr, "Moved 3 items to Personal and deleted Work.") {
		t.Fatalf("move and delete: %v\n%s", err, stderr)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault", "delete", "Work", "--delete-items", "--confirm", "Work"); err != nil {
		t.Fatalf("delete with items: %v", err)
	}
	allDeletion := app.findRequest(cliprotocol.TypeDeleteVault)
	if len(allDeletion) != 2 || allDeletion[0].VaultDeletion.MoveToVaultID != "personal-id" ||
		!allDeletion[1].VaultDeletion.IsDeletingItems || allDeletion[1].VaultDeletion.ConfirmedName != "Work" {
		t.Errorf("deletion requests: %+v", allDeletion)
	}

	if _, _, err := runCapturingOutput(t, false, "", "vault", "leave", "Work", "-y"); err == nil || !strings.Contains(err.Error(), "only owner") {
		t.Errorf("only owner leaves: %v", err)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "vault", "leave", "Family", "-y"); err != nil || !strings.Contains(stderr, "You left Family.") {
		t.Errorf("leave: %v %s", err, stderr)
	}
	if leaves := app.findRequest(cliprotocol.TypeLeaveVault); len(leaves) != 1 || leaves[0].VaultID != "family-id" {
		t.Errorf("leave requests: %+v", leaves)
	}
}

func TestVaultDeleteWizardAsksForTheTypedName(t *testing.T) {
	app := startVaultDataApp(t)
	setInteractiveTerminal(t, true)
	if _, stderr, err := runCapturingOutput(t, false, "2\nWork\n", "vault", "delete", "Work"); err != nil || !strings.Contains(stderr, "1) Move them to Personal") || !strings.Contains(stderr, "Deleted Work.") {
		t.Fatalf("wizard: %v\n%s", err, stderr)
	}
	if _, _, err := runCapturingOutput(t, false, "2\nwrong\n", "vault", "delete", "Work"); err == nil {
		t.Error("deleted with a wrong typed name")
	}
	if _, stderr, err := runCapturingOutput(t, false, "\ny\n", "vault", "delete", "Work"); err != nil || !strings.Contains(stderr, "Move 3 items to Personal and delete Work?") {
		t.Fatalf("move by default: %v\n%s", err, stderr)
	}
	allDeletion := app.findRequest(cliprotocol.TypeDeleteVault)
	if len(allDeletion) != 2 || !allDeletion[0].VaultDeletion.IsDeletingItems || allDeletion[1].VaultDeletion.MoveToVaultID != "personal-id" {
		t.Errorf("deletion requests: %+v", allDeletion)
	}
}

func TestExportThroughAppNeverReplacesAFile(t *testing.T) {
	app := startVaultDataApp(t)
	setInteractiveTerminal(t, false)
	directory := t.TempDir()
	encryptedPath := filepath.Join(directory, "backup.hisar")

	if _, _, err := runCapturingOutput(t, false, "short\n", "export", "-o", encryptedPath); err == nil || !strings.Contains(err.Error(), "at least 10 characters") {
		t.Errorf("short passphrase: %v", err)
	}
	_, stderr, err := runCapturingOutput(t, false, "a long export passphrase\n", "export", "-o", encryptedPath)
	if err != nil || !strings.Contains(stderr, "Exported 6 items from 3 vaults to "+encryptedPath) {
		t.Fatalf("encrypted export: %v %s", err, stderr)
	}
	allExport := app.findRequest(cliprotocol.TypeExport)
	if len(allExport) != 1 || allExport[0].Export.Format != "encrypted" || allExport[0].Export.Passphrase != "a long export passphrase" || allExport[0].Export.Path != encryptedPath {
		t.Errorf("export request: %+v", allExport)
	}

	plainPath := filepath.Join(directory, "backup.json")
	if _, _, err := runCapturingOutput(t, false, "", "export", "--format", "plain", "-o", plainPath); err != errNeedsConfirmation {
		t.Errorf("plain export without -y in a script: %v", err)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "export", "--format", "plain", "-o", plainPath, "-y"); err != nil || !strings.Contains(stderr, "not encrypted") {
		t.Errorf("plain export: %v %s", err, stderr)
	}
	os.WriteFile(plainPath, []byte("{}"), 0o600)
	if _, _, err := runCapturingOutput(t, false, "", "export", "--format", "plain", "-o", plainPath, "-y"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("existing file: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "export", "--format", "csv"); err == nil {
		t.Error("an unknown format was accepted")
	}
	if allExport := app.findRequest(cliprotocol.TypeExport); len(allExport) != 2 || allExport[1].Export.Format != "plain" || allExport[1].Export.Passphrase != "" {
		t.Errorf("export requests: %+v", allExport)
	}
}

const testDotenv = "# shop\nexport STRIPE_KEY=\"sk_live_x\" # live key\nPORT=8080\nDATABASE_URL=postgres://shop:pw@db/shop\nAPI_URL=https://api.example.com\n"

func TestDotenvImportThroughApp(t *testing.T) {
	app := startVaultDataApp(t)
	setInteractiveTerminal(t, false)
	directory := t.TempDir()
	envPath := filepath.Join(directory, ".env")
	os.WriteFile(envPath, []byte(testDotenv), 0o640)

	_, stderr, err := runCapturingOutput(t, false, "", "import", envPath, "--vault", "Work", "--item", "shop", "--dry-run")
	if err != nil || !strings.Contains(stderr, "STRIPE_KEY=hisar://Work/shop/STRIPE_KEY") || !strings.Contains(stderr, "Kept as they are: PORT, API_URL") ||
		!strings.Contains(stderr, "Dry run: nothing was stored") {
		t.Fatalf("dry run: %v\n%s", err, stderr)
	}
	if content, _ := os.ReadFile(envPath); string(content) != testDotenv || len(app.findRequest(cliprotocol.TypeSaveItem)) != 0 || len(app.findRequest(cliprotocol.TypeListVault)) != 0 {
		t.Fatal("the dry run changed something or asked the app")
	}
	if _, _, err := runCapturingOutput(t, false, "", "import", envPath, "--vault", "Work", "--item", "shop"); err != errNeedsConfirmation {
		t.Errorf("import without -y in a script: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "import", envPath, "-y"); err == nil || !strings.Contains(err.Error(), "--vault and --item") {
		t.Errorf("import without a vault in a script: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "import", envPath, "--vault", "Family", "--item", "shop", "-y"); err == nil || !strings.Contains(err.Error(), "only read") {
		t.Errorf("import into a read-only vault: %v", err)
	}

	_, stderr, err = runCapturingOutput(t, false, "", "import", envPath, "--vault", "Work", "--item", "shop", "-y")
	if err != nil || !strings.Contains(stderr, "hisar run --env-file "+envPath+" -- <command>") || !strings.Contains(stderr, "(the existing item)") {
		t.Fatalf("import: %v\n%s", err, stderr)
	}
	allSave := app.findRequest(cliprotocol.TypeSaveItem)
	if len(allSave) != 1 || allSave[0].VaultID != "work-id" || allSave[0].Item.ID != "shop-id" || allSave[0].Item.Fields == nil {
		t.Fatalf("save requests: %+v", allSave)
	}
	allField := *allSave[0].Item.Fields
	if !slices.Contains(allField, cliprotocol.Field{Name: "STRIPE_KEY", Value: "sk_live_x", Concealed: true}) ||
		!slices.Contains(allField, cliprotocol.Field{Name: "DATABASE_URL", Value: "postgres://shop:pw@db/shop", Concealed: true}) ||
		!slices.Contains(allField, cliprotocol.Field{Name: "region", Value: "eu"}) || len(allField) != 3 {
		t.Errorf("fields: %+v", allField)
	}
	content, _ := os.ReadFile(envPath)
	want := "# shop\nexport STRIPE_KEY=\"hisar://Work/shop/STRIPE_KEY\" # live key\nPORT=8080\nDATABASE_URL=hisar://Work/shop/DATABASE_URL\nAPI_URL=https://api.example.com\n"
	if string(content) != want {
		t.Errorf("rewritten file:\n%s", content)
	}
	if fileInfo, _ := os.Stat(envPath); fileInfo.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", fileInfo.Mode().Perm())
	}
	if allEntry, _ := os.ReadDir(directory); len(allEntry) != 1 {
		t.Errorf("files next to .env: %v", allEntry)
	}

	os.WriteFile(envPath, []byte("CODE=1234\nTOKEN=abc\n"), 0o600)
	if _, _, err := runCapturingOutput(t, false, "", "import", envPath, "--vault", "Work", "--item", "new", "--keys", "CODE", "-y"); err == nil || !strings.Contains(err.Error(), "one-time code") {
		t.Errorf("a key named like the one-time code: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "import", envPath, "--vault", "Work", "--item", "new", "--keys", "MISSING", "-y"); err == nil {
		t.Error("an unknown key was accepted")
	}
}

func TestDotenvImportWizard(t *testing.T) {
	app := startVaultDataApp(t)
	setInteractiveTerminal(t, true)
	directory := t.TempDir()
	envPath := filepath.Join(directory, ".env")
	os.WriteFile(envPath, []byte("TOKEN=abc\nPORT=1\n"), 0o600)
	_, stderr, err := runCapturingOutput(t, false, "1 2\n\n1\napp\ny\n", "import", envPath)
	if err != nil || !strings.Contains(stderr, "[x]  1  TOKEN") || !strings.Contains(stderr, "[ ]  2  PORT") {
		t.Fatalf("wizard: %v\n%s", err, stderr)
	}
	allSave := app.findRequest(cliprotocol.TypeSaveItem)
	if len(allSave) != 1 || allSave[0].VaultID != "personal-id" || allSave[0].Item.Title != "app" || allSave[0].Item.ID != "" {
		t.Fatalf("save: %+v", allSave)
	}
	if fields := *allSave[0].Item.Fields; len(fields) != 1 || fields[0].Name != "PORT" {
		t.Errorf("switched fields: %+v", fields)
	}
	if content, _ := os.ReadFile(envPath); string(content) != "TOKEN=abc\nPORT=hisar://Personal/app/PORT\n" {
		t.Errorf("rewritten: %s", content)
	}
}

func TestHisarExportImportThroughApp(t *testing.T) {
	app := startVaultDataApp(t)
	setInteractiveTerminal(t, false)
	document := export.Build(export.Account{Email: "alice@example.com"}, []export.Vault{
		export.ConvertVault("old-personal", "Personal", "personal", []item.Item{
			{Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}}, Details: item.Details{Password: "x"}},
			{Overview: item.Overview{Title: "Bank"}, Details: item.Details{Password: "y", OneTimePassword: "otpauth://totp/Bank?secret=JBSWY3DPEHPK3PXP"}},
		}),
		export.ConvertVault("old-family", "Family", "shared", []item.Item{{Overview: item.Overview{Title: "Router"}, Details: item.Details{Password: "z"}}}),
		export.ConvertVault("old-side", "Side project", "shared", []item.Item{{Overview: item.Overview{Title: "Deploy", Category: item.CategorySSHKey}, Details: item.Details{PrivateKey: "KEY"}}}),
	}, time.Now())
	content, err := export.EncodeEncrypted(document, []byte("a long export passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.hisar")
	os.WriteFile(path, content, 0o600)

	if _, _, err := runCapturingOutput(t, false, "the wrong passphrase\n", "import", path, "-y"); err != export.ErrWrongPassphrase {
		t.Errorf("wrong passphrase: %v", err)
	}
	if len(app.findRequest(cliprotocol.TypeImportItem)) != 0 {
		t.Fatal("imported with a wrong passphrase")
	}
	_, stderr, err := runCapturingOutput(t, false, "a long export passphrase\n", "import", path, "-y")
	if err != nil || !strings.Contains(stderr, "Personal → Personal: 2 items, 1 already there") ||
		!strings.Contains(stderr, "Family → Family (imported) (new vault): 1 item") || !strings.Contains(stderr, "Side project → Side project (new vault)") {
		t.Fatalf("import: %v\n%s", err, stderr)
	}
	allCreate := app.findRequest(cliprotocol.TypeCreateVault)
	if len(allCreate) != 2 || allCreate[0].Name != "Family (imported)" || allCreate[1].Name != "Side project" {
		t.Errorf("created vaults: %+v", allCreate)
	}
	allImport := app.findRequest(cliprotocol.TypeImportItem)
	if len(allImport) != 3 || allImport[0].VaultID != "personal-id" || len(allImport[0].ImportedItems) != 1 || allImport[0].ImportedItems[0].Details.OneTimePassword == "" {
		t.Fatalf("import requests: %+v", allImport)
	}
	if allImport[2].ImportedItems[0].Details.PrivateKey != "KEY" {
		t.Error("the SSH private key was not imported")
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("an encrypted export was deleted")
	}
}
