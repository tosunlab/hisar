package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestServerKeepsItsLoginDataAndNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	key, err := service.GenerateSSHKey(vaultID, "Deploy key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryServer, Title: "Build server", Username: "deploy", Password: "server-password",
		URLs:   []string{"https://console.example.com"},
		Server: &ItemServer{Host: "ssh://build.example.com:2222", SSHKeyID: key.ID},
		Fields: []ItemField{{Name: "Provider", Value: "Example Cloud"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldHost, Value: "build.example.com"},
		{Name: item.FieldPort, Value: "2222"},
		{Name: item.FieldSSHKey, Value: key.ID},
		{Name: "Provider", Value: "Example Cloud"},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if stored.Overview.Category != item.CategoryServer || stored.Overview.Username != "deploy" || stored.Details.Password != "server-password" ||
		!reflect.DeepEqual(stored.Overview.URLs, []string{"https://console.example.com"}) {
		t.Errorf("stored server = %+v %+v", stored.Overview, stored.Details)
	}
	wantServer := ItemServer{Host: "build.example.com", Port: "2222", SSHKeyID: key.ID, SSHKeyTitle: "Deploy key", SSHKeyFingerprint: key.Fingerprint}
	if saved.Server == nil || *saved.Server != wantServer || key.Fingerprint == "" || len(saved.Fields) != 1 {
		t.Errorf("saved server = %+v, fields %+v", saved.Server, saved.Fields)
	}

	saved.Server.Port = "70000"
	saved.Password = "new-password"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil || edited.Password != "new-password" || edited.Server.PortWarning != "The port is not a number from 1 to 65535" || edited.Server.SSHKeyID != key.ID {
		t.Errorf("edited server = %+v %q, %v", edited.Server, edited.Password, err)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		if summary.ID == saved.ID && (summary.Server == nil || *summary.Server != (ItemServer{Host: "build.example.com", Port: "70000"}) || summary.Password != "") {
			t.Errorf("summary server = %+v", summary.Server)
		}
	}
}

func TestServerLinksOnlyToAnSSHKeyOfItsVault(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, personalID, familyID := testService.service, testService.personalID, testService.familyID
	key, err := service.GenerateSSHKey(personalID, "Deploy key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	familyKey, err := service.GenerateSSHKey(familyID, "Family key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	login, err := service.SaveItem(personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	for name, sshKeyID := range map[string]string{"a login": login.ID, "an unknown item": "no-such-item", "a key of another vault": familyKey.ID} {
		_, err := service.SaveItem(personalID, ItemDetail{Category: item.CategoryServer, Title: "Build", Server: &ItemServer{Host: "build.example.com", SSHKeyID: sshKeyID}})
		if err == nil || !strings.Contains(err.Error(), "choose an SSH key from this vault") {
			t.Errorf("linking %s: %v", name, err)
		}
	}

	saved, err := service.SaveItem(personalID, ItemDetail{Category: item.CategoryServer, Title: "Build", Server: &ItemServer{Host: "build.example.com", SSHKeyID: key.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteItem(personalID, key.ID); err != nil {
		t.Fatal(err)
	}
	// The link to the deleted key stays, so the server can still be edited; a new link to it cannot be made.
	saved.Title = "Build server"
	if edited, err := service.SaveItem(personalID, saved); err != nil || edited.Server.SSHKeyID != key.ID || edited.Server.SSHKeyTitle != "" {
		t.Errorf("editing a server whose key was deleted = %+v, %v", edited.Server, err)
	}
	if _, err := service.SaveItem(personalID, ItemDetail{Category: item.CategoryServer, Title: "Other", Server: &ItemServer{SSHKeyID: key.ID}}); err == nil {
		t.Error("a new server links to a deleted key")
	}
}

func TestBrowserOffersAServerOnItsAdminConsole(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, vaultID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	server, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryServer, Title: "Build server", Username: "admin", Password: "console-password",
		URLs: []string{"https://console.example.com"}, Server: &ItemServer{Host: "build.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(vaultID, ItemDetail{Category: item.CategorySecureNote, Title: "Console notes", Notes: "n", URLs: []string{"https://console.example.com"}}); err != nil {
		t.Fatal(err)
	}
	match, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: "https://console.example.com/login"})
	logins := match.(browserprotocol.MatchResult).Logins
	if code != "" || len(logins) != 1 || logins[0].ItemID != server.ID || logins[0].Username != "admin" {
		t.Fatalf("match = %+v %s", logins, code)
	}
	fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.fill", Token: token, VaultID: vaultID, ItemID: server.ID, URL: "https://console.example.com/login"})
	if code != "" || fill.(browserprotocol.FillResult).Password != "console-password" {
		t.Errorf("fill = %+v %s", fill, code)
	}
	check, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.check", Token: token, URL: "https://console.example.com/login", Username: "admin", Password: "console-password"})
	if code != "" || check.(browserprotocol.CheckResult).Status != "unchanged" || check.(browserprotocol.CheckResult).ItemID != server.ID {
		t.Errorf("check = %+v %s", check, code)
	}
}

func TestImportedServerKeepsItsPasswordOffAConsoleWithItsOwnLogin(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, vaultID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	importOnePasswordExport(t, service, vaultID, `{"accounts":[{"vaults":[{"items":[{"categoryUuid":"110","overview":{"title":"Build server"},"details":{"sections":[
		{"title":"","fields":[
			{"title":"URL","id":"url","value":{"string":"build.example.com"}},
			{"title":"username","id":"username","value":{"string":"admin"}},
			{"title":"password","id":"password","value":{"concealed":"server-password"}}]},
		{"title":"Admin Console","fields":[
			{"title":"console URL","id":"admin_console_url","value":{"string":"https://console.example.com"}},
			{"title":"console username","id":"admin_console_username","value":{"string":"admin"}},
			{"title":"console password","id":"admin_console_password","value":{"concealed":"console-password"}}]}]}}]}]}]}`)
	server := listItemsByTitle(t, service, vaultID)["Build server"]
	if server.Category != item.CategoryServer {
		t.Fatalf("imported %+v", server)
	}

	const consoleURL = "https://console.example.com/login"
	match, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: consoleURL})
	if logins := match.(browserprotocol.MatchResult).Logins; code != "" || len(logins) != 0 {
		t.Errorf("the console is offered %+v %s", logins, code)
	}
	fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.fill", Token: token, VaultID: vaultID, ItemID: server.ID, URL: consoleURL})
	if code == "" {
		t.Errorf("the console is filled with %+v", fill)
	}
	// Saving the console's login offers a new login, not an update that would overwrite the server's password.
	check, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.check", Token: token, URL: consoleURL, Username: "admin", Password: "console-password"})
	if result := check.(browserprotocol.CheckResult); code != "" || result.ItemID == server.ID {
		t.Errorf("check = %+v %s", result, code)
	}
	if kept, err := service.GetItem(vaultID, server.ID); err != nil || kept.Password != "server-password" {
		t.Errorf("server = %+v %v", kept, err)
	}
}

func TestCommandLineServerTravelsAsNamedFieldsAndResolves(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{{Name: "port", Value: "2222"}, {Name: "Host", Value: "build.example.com"}}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategoryServer, Title: "Build", Username: "deploy", Password: "server-password", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{{Name: item.FieldHost, Value: "build.example.com"}, {Name: item.FieldPort, Value: "2222"}}
	if created.Item.Category != item.CategoryServer || !reflect.DeepEqual(created.Item.Fields, wantFields) || created.Item.Username != "deploy" {
		t.Errorf("created %+v", created.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureServers) || cliprotocol.GetCategoryFeature(item.CategoryServer) != cliprotocol.FeatureServers {
		t.Errorf("hello features = %v", hello.Features)
	}

	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Build/password", "hisar://Personal/Build/host"}, CommandLine: []string{"env"}}, &result)
	if !reflect.DeepEqual(result.Values, []string{"server-password", "build.example.com"}) {
		t.Errorf("references = %q", result.Values)
	}
}

func TestCodingAgentsFindServersByHost(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	if _, err := service.SetVaultAvailableToAgents(vaultID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(vaultID, ItemDetail{Category: item.CategoryServer, Title: "Build", Username: "deploy", Password: "server-password", Server: &ItemServer{Host: "build.example.com"}}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"build.example", "deploy"} {
		if result, err := service.searchAgentItems(query, ""); err != nil || result.TotalCount != 1 {
			t.Errorf("search %q = %+v, %v", query, result, err)
		}
	}
	if result, err := service.searchAgentItems("server-password", ""); err != nil || result.TotalCount != 0 {
		t.Errorf("a search for the password found %+v, %v", result, err)
	}
}
