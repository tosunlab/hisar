package vault

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/session"
)

func (server *fakeAPIServer) addAllVaultManagementRoute(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/name", func(writer http.ResponseWriter, request *http.Request) {
		var renameRequest apitypes.RenameVaultRequest
		json.NewDecoder(request.Body).Decode(&renameRequest)
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) != apitypes.VaultOwner {
			writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: "owner only"})
			return
		}
		for index := range server.allVault {
			if server.allVault[index].ID == vaultID {
				server.allVault[index].EncName = renameRequest.EncName
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/delete", func(writer http.ResponseWriter, request *http.Request) {
		var deleteRequest apitypes.DeleteVaultRequest
		json.NewDecoder(request.Body).Decode(&deleteRequest)
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if server.getVaultRole(vaultID) != apitypes.VaultOwner {
			writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: "owner only"})
			return
		}
		liveCount := 0
		for _, storedItem := range server.allItemByVaultID[vaultID] {
			if !storedItem.Deleted {
				liveCount++
			}
		}
		if deleteRequest.MoveToVaultID != "" {
			if len(deleteRequest.AllMovedItem) != liveCount {
				writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "the vault changed"})
				return
			}
			for _, movedItem := range deleteRequest.AllMovedItem {
				server.seq++
				server.allItemByVaultID[deleteRequest.MoveToVaultID][movedItem.ID] = apitypes.Item{
					ID: movedItem.ID, Revision: movedItem.Revision, Seq: server.seq, Overview: movedItem.Overview, Details: movedItem.Details,
				}
			}
		} else if liveCount > 0 && !deleteRequest.IsDeletingItems {
			writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "the vault has items"})
			return
		}
		server.removeVault(vaultID)
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/vaults/{vaultID}/access/{accountID}", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		vaultID := request.PathValue("vaultID")
		if request.PathValue("accountID") != server.accountID || server.getVaultRole(vaultID) == "" {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "not found"})
			return
		}
		if server.getVaultRole(vaultID) == apitypes.VaultOwner {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "the only owner can't leave a vault"})
			return
		}
		server.removeVault(vaultID)
		writer.WriteHeader(http.StatusNoContent)
	})
}

func (server *fakeAPIServer) removeVault(vaultID string) {
	server.allVault = slices.DeleteFunc(server.allVault, func(vault apitypes.Vault) bool { return vault.ID == vaultID })
	delete(server.allItemByVaultID, vaultID)
	delete(server.allAccessByVaultID, vaultID)
}

func (server *fakeAPIServer) hasVault(vaultID string) bool {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return slices.ContainsFunc(server.allVault, func(vault apitypes.Vault) bool { return vault.ID == vaultID })
}

func TestVaultActions(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "bob@example.com")
	test.apiServer.mutex.Lock()
	test.apiServer.allAccessByVaultID[test.familyID] = append(test.apiServer.allAccessByVaultID[test.familyID], apitypes.RotatedAccess{AccountID: bob.AccountID, Role: apitypes.VaultEditor})
	test.apiServer.mutex.Unlock()

	personal, err := service.GetVaultActions(test.personalID)
	if err != nil {
		t.Fatal(err)
	}
	if personal.CanDelete || !strings.Contains(personal.DeleteBlockedReason, "Personal vault can't be deleted") || personal.CanLeave || !personal.CanRename {
		t.Errorf("personal vault actions: %+v", personal)
	}
	family, err := service.GetVaultActions(test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	if !family.CanDelete || family.CanLeave || family.LeaveBlockedReason != session.OnlyOwnerLeaveReason {
		t.Errorf("family actions: %+v", family)
	}
	if len(family.AllMemberLosingAccess) != 1 || family.AllMemberLosingAccess[0].Email != "bob@example.com" {
		t.Errorf("members losing access: %+v", family.AllMemberLosingAccess)
	}
	if len(family.AllTargetVault) != 1 || family.AllTargetVault[0].ID != test.personalID {
		t.Errorf("target vaults: %+v", family.AllTargetVault)
	}

	test.apiServer.setVaultRole(test.familyID, apitypes.VaultEditor)
	asEditor, err := service.GetVaultActions(test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	if asEditor.CanDelete || asEditor.DeleteBlockedReason != session.NotOwnerDeleteReason || !asEditor.CanLeave || asEditor.CanRename {
		t.Errorf("editor actions: %+v", asEditor)
	}
}

func TestRenameVault(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	renamed, err := service.RenameVault(test.familyID, "  Home  ")
	if err != nil || renamed.Name != "Home" {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	allVault, err := service.ListAllVault()
	if err != nil || !slices.ContainsFunc(allVault, func(vault VaultSummary) bool { return vault.ID == test.familyID && vault.Name == "Home" }) {
		t.Errorf("after rename: %+v %v", allVault, err)
	}
	if _, err := service.RenameVault(test.familyID, " "); err == nil || !strings.Contains(err.Error(), "enter a vault name") {
		t.Errorf("blank name: %v", err)
	}
	test.apiServer.setVaultRole(test.familyID, apitypes.VaultReader)
	service.ListAllVault()
	if _, err := service.RenameVault(test.familyID, "Mine"); err == nil || !strings.Contains(err.Error(), "only an owner") {
		t.Errorf("reader renames: %v", err)
	}
}

func TestDeleteVaultMovesItemsIntoAnotherVault(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	login, err := service.SaveItem(test.familyID, ItemDetail{Title: "GitHub", Username: "alice", Password: "hunter2", URLs: []string{"https://github.com"}, Fields: []ItemField{{Name: "API_TOKEN", Value: "ghp_x", Concealed: true}}})
	if err != nil {
		t.Fatal(err)
	}
	sshKey, err := service.GenerateSSHKey(test.familyID, "Deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	privateKey := getCachedItem(t, service, test.familyID, sshKey.ID).Details.PrivateKey
	if _, err := service.SetVaultAvailableToAgents(test.familyID, true); err != nil {
		t.Fatal(err)
	}

	if _, err := service.DeleteVault(test.familyID, VaultDeletionRequest{}); !errors.Is(err, errChooseItemAction) {
		t.Errorf("delete without a choice: %v", err)
	}
	result, err := service.DeleteVault(test.familyID, VaultDeletionRequest{MoveToVaultID: test.personalID})
	if err != nil || result.MovedCount != 2 || result.TargetVaultName != "Personal" {
		t.Fatalf("move and delete: %+v %v", result, err)
	}
	if test.apiServer.hasVault(test.familyID) {
		t.Error("the vault is still on the server")
	}
	allVault, err := service.ListAllVault()
	if err != nil || len(allVault) != 1 || allVault[0].ItemCount != 2 {
		t.Fatalf("vaults after the move: %+v %v", allVault, err)
	}
	moved, err := service.GetItem(test.personalID, login.ID)
	if err != nil || moved.Password != "hunter2" || len(moved.Fields) != 1 || moved.Fields[0].Value != "ghp_x" {
		t.Errorf("moved login: %+v %v", moved, err)
	}
	if movedKey := getCachedItem(t, service, test.personalID, sshKey.ID); movedKey.Details.PrivateKey != privateKey {
		t.Error("the moved SSH key lost its private key")
	}
	if slices.Contains(service.ListAllAgentVaultID(), test.familyID) {
		t.Error("the deleted vault is still available to agents")
	}
	if notices := service.ListAllRemovedVault(); len(notices) != 0 {
		t.Errorf("own deletion became a notice: %+v", notices)
	}
}

func TestDeleteVaultWithItemsNeedsItsName(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	if _, err := service.SaveItem(test.familyID, ItemDetail{Title: "Router"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteVault(test.familyID, VaultDeletionRequest{IsDeletingItems: true, ConfirmedName: "family"}); !errors.Is(err, errConfirmVaultName) {
		t.Errorf("wrong confirmation: %v", err)
	}
	if !test.apiServer.hasVault(test.familyID) {
		t.Fatal("deleted without the typed name")
	}
	if _, err := service.DeleteVault(test.familyID, VaultDeletionRequest{IsDeletingItems: true, ConfirmedName: "Family"}); err != nil {
		t.Fatal(err)
	}
	if test.apiServer.hasVault(test.familyID) {
		t.Error("the vault is still on the server")
	}
	if _, err := service.DeleteVault(test.personalID, VaultDeletionRequest{IsDeletingItems: true, ConfirmedName: "Personal"}); err == nil || !strings.Contains(err.Error(), "Personal vault can't be deleted") {
		t.Errorf("deleting the personal vault: %v", err)
	}
}

func TestLeaveVaultAndRemovedVaultNotice(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	service.mutex.Lock()
	service.unlockedSession.Cache = service.getCache()
	service.mutex.Unlock()
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	if err := service.LeaveVault(test.familyID); err == nil || err.Error() != session.OnlyOwnerLeaveReason {
		t.Errorf("only owner leaves: %v", err)
	}
	test.apiServer.setVaultRole(test.familyID, apitypes.VaultEditor)
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	if err := service.LeaveVault(test.familyID); err != nil {
		t.Fatal(err)
	}
	if allVault, _ := service.ListAllVault(); len(allVault) != 1 {
		t.Errorf("vaults after leaving: %+v", allVault)
	}
	if notices := service.ListAllRemovedVault(); len(notices) != 0 {
		t.Errorf("leaving became a notice: %+v", notices)
	}

	shared, err := service.CreateVault("Team")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.removeVault(shared.ID)
	test.apiServer.mutex.Unlock()
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	notices := service.ListAllRemovedVault()
	if len(notices) != 1 || notices[0].Name != "Team" || !strings.Contains(notices[0].Message, "“Team” is no longer available") {
		t.Errorf("notices: %+v", notices)
	}
	if again := service.ListAllRemovedVault(); len(again) != 0 {
		t.Errorf("notice shown twice: %+v", again)
	}
}

func TestExportAllData(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	service.mutex.Lock()
	service.unlockedSession.Cache = service.getCache()
	service.mutex.Unlock()
	if _, err := service.SaveItem(test.familyID, ItemDetail{Title: "GitHub", Password: "hunter2", OneTimePassword: "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateSSHKey(test.personalID, "Laptop", ""); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	saver := service.config.FileSaver.(*fakeFileSaver)

	if _, err := service.ExportAllData(ExportRequest{Format: ExportFormatEncrypted, Passphrase: "short"}); !errors.Is(err, export.ErrPassphraseTooShort) {
		t.Errorf("short passphrase: %v", err)
	}
	if _, err := service.ExportAllData(ExportRequest{Format: "csv"}); !errors.Is(err, errUnknownExportFormat) {
		t.Errorf("unknown format: %v", err)
	}
	biometric := service.config.Biometric.(*fakeBiometric)
	biometric.setAuthenticateError(errors.New("cancelled"))
	saver.path = filepath.Join(directory, "denied.hisar")
	if _, err := service.ExportAllData(ExportRequest{Format: ExportFormatEncrypted, Passphrase: "a long export passphrase"}); err == nil {
		t.Error("exported after Touch ID failed")
	}
	if _, err := os.Stat(saver.path); !errors.Is(err, os.ErrNotExist) {
		t.Error("a file was written after Touch ID failed")
	}
	biometric.setAuthenticateError(nil)

	saver.path = filepath.Join(directory, "export.hisar")
	result, err := service.ExportAllData(ExportRequest{Format: ExportFormatEncrypted, Passphrase: "a long export passphrase"})
	if err != nil || result.VaultCount != 2 || result.ItemCount != 2 || result.Path != saver.path {
		t.Fatalf("encrypted export: %+v %v", result, err)
	}
	content, _ := os.ReadFile(saver.path)
	if fileInfo, _ := os.Stat(saver.path); fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("export mode %v", fileInfo.Mode().Perm())
	}
	if strings.Contains(string(content), "hunter2") {
		t.Error("encrypted export shows a password")
	}
	document, err := export.Decode(content, []byte("a long export passphrase"))
	if err != nil || document.CountItem() != 2 || document.Account.Email != "a@example.com" {
		t.Fatalf("decode: %+v %v", document, err)
	}
	foundSecrets := 0
	for _, vault := range document.Vaults {
		for _, exported := range vault.Items {
			if exported.Details.PrivateKey != "" || strings.HasPrefix(exported.Details.OneTimePassword, "otpauth://") {
				foundSecrets++
			}
		}
	}
	if foundSecrets != 2 {
		t.Errorf("export lacks the SSH private key or the one-time code secret: %+v", document.Vaults)
	}

	test.apiServer.setUnreachable(true)
	saver.path = filepath.Join(directory, "offline.json")
	offline, err := service.ExportAllData(ExportRequest{Format: ExportFormatPlain})
	test.apiServer.setUnreachable(false)
	if err != nil || offline.ItemCount != 2 {
		t.Errorf("offline export from the cache: %+v %v", offline, err)
	}
	plainContent, _ := os.ReadFile(saver.path)
	if plainDocument, err := export.Decode(plainContent, nil); err != nil || plainDocument.CountItem() != 2 {
		t.Errorf("plain export: %v", err)
	}

	if _, err := service.writeExportFile(ExportFormatPlain, "", saver.path+".again"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.writeExportFile(ExportFormatPlain, "", saver.path+".again"); !errors.Is(err, errExportFileExists) {
		t.Errorf("the command-line export replaced a file: %v", err)
	}
	if _, err := service.writeExportFile(ExportFormatPlain, "", "relative.json"); err == nil {
		t.Error("a relative path was accepted")
	}
}

func TestVaultManagementAndExportThroughTheCommandLine(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	familyID, personalID := test.testService.familyID, test.testService.personalID
	if _, err := service.SaveItem(familyID, ItemDetail{Title: "Router", Password: "router-password"}); err != nil {
		t.Fatal(err)
	}

	var actions cliprotocol.VaultActionsResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeGetVaultActions, VaultID: familyID}, &actions)
	if !actions.CanDelete || actions.Vault.ItemCount != 1 || len(actions.AllTargetVault) != 1 || actions.AllTargetVault[0].ID != personalID {
		t.Errorf("actions: %+v", actions)
	}
	var renamed cliprotocol.VaultResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeRenameVault, VaultID: familyID, Name: "Home"}, &renamed)
	if renamed.Vault.Name != "Home" {
		t.Errorf("renamed: %+v", renamed)
	}

	var saved cliprotocol.ItemResult
	allField := []cliprotocol.Field{{Name: "API_TOKEN", Value: "token-value", Concealed: true}}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: personalID, Item: &cliprotocol.ItemChange{Title: "shop", Fields: &allField}}, &saved)
	if detail, err := service.GetItem(personalID, saved.Item.ID); err != nil || len(detail.Fields) != 1 || detail.Fields[0].Value != "token-value" {
		t.Errorf("saved with fields: %+v %v", detail, err)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: personalID, Item: &cliprotocol.ItemChange{ID: saved.Item.ID, Revision: saved.Item.Revision, Title: "shop", Password: "p"}}, nil)
	if detail, _ := service.GetItem(personalID, saved.Item.ID); len(detail.Fields) != 1 || detail.Password != "p" {
		t.Errorf("an update without fields dropped them: %+v", detail)
	}

	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeDeleteVault, VaultID: familyID, VaultDeletion: &cliprotocol.VaultDeletion{IsDeletingItems: true, ConfirmedName: "Family"}}, nil); !strings.Contains(failure, "type the vault's name") {
		t.Errorf("deletion with the old name: %q", failure)
	}
	var deletion cliprotocol.VaultDeletionResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeDeleteVault, VaultID: familyID, VaultDeletion: &cliprotocol.VaultDeletion{MoveToVaultID: personalID}}, &deletion)
	if deletion.MovedCount != 1 || test.testService.apiServer.hasVault(familyID) {
		t.Errorf("deletion: %+v", deletion)
	}

	exportPath := filepath.Join(t.TempDir(), "export.hisar")
	var exported cliprotocol.ExportResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &cliprotocol.ExportChange{Format: ExportFormatEncrypted, Passphrase: "a long export passphrase", Path: exportPath}}, &exported)
	if exported.ItemCount != 2 || exported.VaultCount != 1 || exported.Path != exportPath {
		t.Errorf("export: %+v", exported)
	}
	content, _ := os.ReadFile(exportPath)
	if document, err := export.Decode(content, []byte("a long export passphrase")); err != nil || document.CountItem() != 2 {
		t.Errorf("decode: %v", err)
	}
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &cliprotocol.ExportChange{Format: ExportFormatPlain, Path: exportPath}}, nil); !strings.Contains(failure, "already exists") {
		t.Errorf("the export replaced a file: %q", failure)
	}
}
