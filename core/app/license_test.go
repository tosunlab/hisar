package app

import (
	"reflect"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestSoftwareLicenseIsStoredAsNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategorySoftwareLicense, Title: "Sketch",
		SoftwareLicense: &ItemSoftwareLicense{
			Product: " Sketch ", Version: "100", LicenseKey: "TEST-LICENSE-KEY", LicensedTo: "Alice Example", Email: "alice@example.com",
			PurchaseDate: "2024/3/9", DownloadURL: "https://www.sketch.com/download",
		},
		Fields:   []ItemField{{Name: "Seats", Value: "5"}},
		Username: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldProduct, Value: "Sketch"},
		{Name: item.FieldVersion, Value: "100"},
		{Name: item.FieldLicenseKey, Value: "TEST-LICENSE-KEY", Concealed: true},
		{Name: item.FieldLicensedTo, Value: "Alice Example"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldPurchaseDate, Value: "2024-03-09"},
		{Name: item.FieldDownloadURL, Value: "https://www.sketch.com/download"},
		{Name: "Seats", Value: "5"},
		{Name: "Username", Value: "alice"},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if stored.Overview.Category != item.CategorySoftwareLicense || stored.Overview.Username != "" || len(stored.Overview.URLs) != 0 {
		t.Errorf("stored license = %+v", stored.Overview)
	}
	if saved.SoftwareLicense == nil || saved.SoftwareLicense.LicenseKey != "TEST-LICENSE-KEY" || len(saved.Fields) != 2 {
		t.Errorf("saved license = %+v, fields %+v", saved.SoftwareLicense, saved.Fields)
	}

	saved.SoftwareLicense.Version = "101"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil || edited.SoftwareLicense.Version != "101" || edited.SoftwareLicense.LicenseKey != "TEST-LICENSE-KEY" || len(edited.Fields) != 2 {
		t.Errorf("edited license = %+v, fields %+v, %v", edited.SoftwareLicense, edited.Fields, err)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil || len(summaries) != 1 || summaries[0].SoftwareLicense == nil {
		t.Fatalf("summaries = %+v, %v", summaries, err)
	}
	want := ItemSoftwareLicense{Product: "Sketch", Version: "101", LicensedTo: "Alice Example", Email: "alice@example.com"}
	if *summaries[0].SoftwareLicense != want || summaries[0].Fields != nil {
		t.Errorf("summary license = %+v", summaries[0].SoftwareLicense)
	}
}

func TestCommandLineSoftwareLicenseTravelsAsNamedFieldsAndResolves(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{{Name: "license key", Value: "TEST-LICENSE-KEY"}, {Name: "Product", Value: "Sketch"}}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategorySoftwareLicense, Title: "Sketch", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{{Name: item.FieldProduct, Value: "Sketch"}, {Name: item.FieldLicenseKey, Value: "TEST-LICENSE-KEY", Concealed: true}}
	if created.Item.Category != item.CategorySoftwareLicense || !reflect.DeepEqual(created.Item.Fields, wantFields) {
		t.Errorf("created %+v", created.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureSoftwareLicenses) || cliprotocol.GetCategoryFeature(item.CategorySoftwareLicense) != cliprotocol.FeatureSoftwareLicenses {
		t.Errorf("hello features = %v", hello.Features)
	}

	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Sketch/license%20key"}, CommandLine: []string{"env"}}, &result)
	if !reflect.DeepEqual(result.Values, []string{"TEST-LICENSE-KEY"}) {
		t.Errorf("hisar://Personal/Sketch/license%%20key = %q", result.Values)
	}
}

func TestCodingAgentsFindSoftwareLicensesAsTheAppsDo(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	if _, err := service.SetVaultAvailableToAgents(vaultID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategorySoftwareLicense, Title: "Design app",
		SoftwareLicense: &ItemSoftwareLicense{Product: "Sketch", Version: "v100", LicenseKey: "TEST-LICENSE-KEY", LicensedTo: "Alice Example", Email: "bob@example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"sketch", "v100", "alice", "bob@"} {
		if result, err := service.searchAgentItems(query, ""); err != nil || result.TotalCount != 1 {
			t.Errorf("search %q = %+v, %v", query, result, err)
		}
	}
	if result, err := service.searchAgentItems("test-license", ""); err != nil || result.TotalCount != 0 {
		t.Errorf("a search for the license key found %+v, %v", result, err)
	}
}
