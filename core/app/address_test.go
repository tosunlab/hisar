package app

import (
	"reflect"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestAddressIsStoredAsNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryAddress, Title: "Home",
		Address:  &ItemAddress{Street: " 1 Main St \n\nApt 4", City: "Springfield", Region: "IL", PostalCode: "12345", Country: "us"},
		Fields:   []ItemField{{Name: "Gate code", Value: "1234", Concealed: true}},
		Username: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldStreet, Value: "1 Main St\nApt 4"},
		{Name: item.FieldCity, Value: "Springfield"},
		{Name: item.FieldRegion, Value: "IL"},
		{Name: item.FieldPostalCode, Value: "12345"},
		{Name: item.FieldCountry, Value: "US"},
		{Name: "Gate code", Value: "1234", Concealed: true},
		{Name: "Username", Value: "alice"},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) || stored.Overview.Username != "" {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if saved.Address == nil || saved.Address.Country != "US" || len(saved.Fields) != 2 {
		t.Errorf("saved address = %+v, fields %+v", saved.Address, saved.Fields)
	}

	saved.Address.City = "Chicago"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil || edited.Address.City != "Chicago" || edited.Address.Street != "1 Main St\nApt 4" || len(edited.Fields) != 2 {
		t.Errorf("edited address = %+v, fields %+v, %v", edited.Address, edited.Fields, err)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil || len(summaries) != 1 || summaries[0].Address == nil {
		t.Fatalf("summaries = %+v, %v", summaries, err)
	}
	if *summaries[0].Address != (ItemAddress{Street: "1 Main St", City: "Chicago", Country: "US"}) || summaries[0].Fields != nil {
		t.Errorf("summary address = %+v", summaries[0].Address)
	}
}

func TestIdentityAndCardKeepTheirAddressInTheAddressFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	address := ItemAddress{Street: "1 Main St", City: "Springfield", PostalCode: "12345", Country: "tr"}
	identity, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryIdentity, Title: "Alice",
		Identity: &ItemIdentity{FirstName: "Alice", ItemAddress: address, Address: "1 Main St, Springfield"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantAddress := ItemAddress{Street: "1 Main St", City: "Springfield", PostalCode: "12345", Country: "TR"}
	if identity.Identity.ItemAddress != wantAddress || identity.Identity.Address != "1 Main St, Springfield" {
		t.Errorf("identity = %+v", identity.Identity)
	}
	if fields := getCachedItem(t, service, vaultID, identity.ID).Details.Fields; fields[1].Name != item.FieldStreet || fields[5].Name != item.FieldAddress {
		t.Errorf("stored identity fields = %+v", fields)
	}

	card, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryCreditCard, Title: "Visa",
		Card: &ItemCard{Number: "4111111111111111", ItemAddress: address},
	})
	if err != nil || card.Card.ItemAddress != wantAddress || card.Card.BillingAddress != "" {
		t.Errorf("card = %+v, %v", card.Card, err)
	}
}

func TestCommandLineAddressTravelsAsNamedFields(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{{Name: "city", Value: "Springfield"}, {Name: "Street", Value: "1 Main St"}}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategoryAddress, Title: "Home", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{{Name: item.FieldStreet, Value: "1 Main St"}, {Name: item.FieldCity, Value: "Springfield"}}
	if created.Item.Category != item.CategoryAddress || !reflect.DeepEqual(created.Item.Fields, wantFields) {
		t.Errorf("created %+v", created.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureAddresses) || cliprotocol.GetCategoryFeature(item.CategoryAddress) != cliprotocol.FeatureAddresses {
		t.Errorf("hello features = %v", hello.Features)
	}

	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Home/city"}, CommandLine: []string{"env"}}, &result)
	if !reflect.DeepEqual(result.Values, []string{"Springfield"}) {
		t.Errorf("hisar://Personal/Home/city = %q", result.Values)
	}
}
