package app

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestDatabaseKeepsItsLoginDataAndNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryDatabase, Title: "Prod db", Username: "app", Password: "p@ss",
		Database: &ItemDatabase{Engine: "Postgres", Host: "db.example.com:5432", Name: "app", Options: "sslmode=require", ConnectionString: "ignored"},
		Fields:   []ItemField{{Name: "SID", Value: "ORCL"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldEngine, Value: "postgresql"},
		{Name: item.FieldHost, Value: "db.example.com"},
		{Name: item.FieldPort, Value: "5432"},
		{Name: item.FieldDatabaseName, Value: "app"},
		{Name: item.FieldOptions, Value: "sslmode=require"},
		{Name: "SID", Value: "ORCL"},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if stored.Overview.Category != item.CategoryDatabase || stored.Overview.Username != "app" || stored.Details.Password != "p@ss" {
		t.Errorf("stored database = %+v %+v", stored.Overview, stored.Details)
	}
	wantDatabase := ItemDatabase{
		Engine: "postgresql", EngineName: "PostgreSQL", Host: "db.example.com", Port: "5432", Name: "app", Options: "sslmode=require",
		ConnectionString: "postgresql://app:p%40ss@db.example.com:5432/app?sslmode=require",
	}
	if saved.Database == nil || *saved.Database != wantDatabase || len(saved.Fields) != 1 {
		t.Errorf("saved database = %+v, fields %+v", saved.Database, saved.Fields)
	}

	// The connection string follows the password and port.
	saved.Password = "new-password"
	saved.Database.Port = "70000"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil || edited.Database.ConnectionString != "postgresql://app:new-password@db.example.com:70000/app?sslmode=require" ||
		edited.Database.PortWarning != "The port is not a number from 1 to 65535" {
		t.Errorf("edited database = %+v, %v", edited.Database, err)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil {
		t.Fatal(err)
	}
	wantSummary := ItemDatabase{Engine: "postgresql", EngineName: "PostgreSQL", Host: "db.example.com", Port: "70000", Name: "app"}
	for _, summary := range summaries {
		if summary.ID == saved.ID && (summary.Database == nil || *summary.Database != wantSummary || summary.Password != "") {
			t.Errorf("summary database = %+v", summary.Database)
		}
	}

	if err := service.CopyField(vaultID, saved.ID, "connection string"); err != nil {
		t.Fatal(err)
	}
	if copied, _ := testService.clipboard.GetText(); copied != edited.Database.ConnectionString {
		t.Errorf("copied %q", copied)
	}
	if reference, err := service.CopySecretReference(vaultID, saved.ID, "Connection string"); err != nil || reference != "hisar://Personal/Prod%20db/connection%20string" {
		t.Errorf("reference = %q, %v", reference, err)
	}
}

func TestDatabaseWithAnotherEngineHasNoConnectionString(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryDatabase, Title: "Mainframe", Username: "app", Password: "secret",
		Database: &ItemDatabase{Engine: "DB2", Host: "db2.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Database.Engine != "DB2" || saved.Database.EngineName != "DB2" || saved.Database.ConnectionString != "" {
		t.Errorf("database = %+v", saved.Database)
	}
	if err := service.CopyField(vaultID, saved.ID, "connection string"); err == nil {
		t.Error("an empty connection string was copied")
	}
}

func TestCommandLineDatabaseTravelsAsNamedFieldsAndResolves(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{{Name: "database", Value: "app"}, {Name: "Host", Value: "db.example.com"}, {Name: "engine", Value: "mysql"}}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategoryDatabase, Title: "Shop db", Username: "root", Password: "db-password", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{{Name: item.FieldEngine, Value: "mysql"}, {Name: item.FieldHost, Value: "db.example.com"}, {Name: item.FieldDatabaseName, Value: "app"}}
	if created.Item.Category != item.CategoryDatabase || !reflect.DeepEqual(created.Item.Fields, wantFields) || created.Item.Username != "root" {
		t.Errorf("created %+v", created.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureDatabases) || cliprotocol.GetCategoryFeature(item.CategoryDatabase) != cliprotocol.FeatureDatabases {
		t.Errorf("hello features = %v", hello.Features)
	}

	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{
		"hisar://Personal/Shop db/connection string", "hisar://Personal/Shop db/host", "hisar://Personal/Shop db/database", "hisar://Personal/Shop db/password",
	}, CommandLine: []string{"env"}}, &result)
	if !reflect.DeepEqual(result.Values, []string{"mysql://root:db-password@db.example.com/app", "db.example.com", "app", "db-password"}) {
		t.Errorf("references = %q", result.Values)
	}
}

func TestCodingAgentsFindDatabasesByHostAndName(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	if _, err := service.SetVaultAvailableToAgents(vaultID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryDatabase, Title: "Prod", Username: "app", Password: "db-password",
		Database: &ItemDatabase{Engine: "postgresql", Host: "db.example.com", Name: "orders"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"db.example", "orders", "app"} {
		if result, err := service.searchAgentItems(query, ""); err != nil || result.TotalCount != 1 {
			t.Errorf("search %q = %+v, %v", query, result, err)
		}
	}
	if result, err := service.searchAgentItems("db-password", ""); err != nil || result.TotalCount != 0 {
		t.Errorf("a search for the password found %+v, %v", result, err)
	}
}

func TestCodingAgentsGetTheConnectionStringReference(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	if _, err := service.SetVaultAvailableToAgents(vaultID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryDatabase, Title: "Prod", Username: "app", Password: "p@ss",
		Database: &ItemDatabase{Engine: "sqlserver", Host: "sql.example.com", Port: "1433", Name: "Sales"},
	}); err != nil {
		t.Fatal(err)
	}
	jdbcField := []ItemField{{Name: "Connection string", Value: "jdbc:db2://db2.example.com:50000/app"}}
	if _, err := service.SaveItem(vaultID, ItemDetail{Category: item.CategoryDatabase, Title: "Mainframe", Database: &ItemDatabase{Engine: "DB2", Host: "db2.example.com"}, Fields: jdbcField}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(vaultID, ItemDetail{Category: item.CategoryLogin, Title: "Legacy", Fields: jdbcField}); err != nil {
		t.Fatal(err)
	}
	referencesByTitle := map[string][]string{}
	result, err := service.searchAgentItems("", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, agentItem := range result.Items {
		for _, field := range agentItem.Fields {
			referencesByTitle[agentItem.Title] = append(referencesByTitle[agentItem.Title], field.Reference)
		}
	}
	for _, title := range []string{"Prod", "Mainframe", "Legacy"} {
		// A custom field keeps its own spelling; references match labels ignoring case.
		want := "hisar://personal/" + strings.ToLower(title) + "/connection%20string"
		if !slices.ContainsFunc(referencesByTitle[title], func(reference string) bool { return strings.EqualFold(reference, want) }) {
			t.Errorf("%s fields = %v, want %s", title, referencesByTitle[title], want)
		}
	}
	resolved, err := service.resolveSecrets([]string{"hisar://Personal/Prod/connection%20string", "hisar://Personal/Mainframe/connection%20string"})
	if err != nil || !reflect.DeepEqual(resolved.Values, []string{"sqlserver://app:p%40ss@sql.example.com:1433?database=Sales", "jdbc:db2://db2.example.com:50000/app"}) {
		t.Errorf("resolved = %q, %v", resolved.Values, err)
	}
	mainframeID := listItemsByTitle(t, service, vaultID)["Mainframe"].ID
	if err := service.CopyField(vaultID, mainframeID, "connection string"); err != nil {
		t.Fatal(err)
	}
	if copied, _ := testService.clipboard.GetText(); copied != "jdbc:db2://db2.example.com:50000/app" {
		t.Errorf("copied %q", copied)
	}
}
