package app

import (
	"reflect"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestIdentityIsStoredAsNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryIdentity, Title: "Alice",
		Identity: &ItemIdentity{FirstName: " Alice ", LastName: "Example", BirthDate: "1990/4/12", Email: "alice@example.com", PassportNumber: "U1234567"},
		Fields:   []ItemField{{Name: "Employer", Value: "Hisar"}},
		Username: "alice", Password: "hidden",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldBirthDate, Value: "1990-04-12"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldPassportNumber, Value: "U1234567", Concealed: true},
		{Name: "Employer", Value: "Hisar"},
		{Name: "Username", Value: "alice"},
		{Name: "Password", Value: "hidden", Concealed: true},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if stored.Overview.Category != item.CategoryIdentity || stored.Overview.Username != "" || stored.Details.Password != "" {
		t.Errorf("stored identity = %+v", stored)
	}
	if saved.Identity == nil || saved.Identity.FirstName != "Alice" || saved.Identity.PassportNumber != "U1234567" || len(saved.Fields) != 3 {
		t.Errorf("saved identity = %+v, fields %+v", saved.Identity, saved.Fields)
	}

	saved.Identity.Phone = "+1 555 0100"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Identity.Phone != "+1 555 0100" || edited.Identity.PassportNumber != "U1234567" || len(edited.Fields) != 3 {
		t.Errorf("edited identity = %+v, fields %+v", edited.Identity, edited.Fields)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil || len(summaries) != 1 || summaries[0].Identity == nil {
		t.Fatalf("summaries = %+v, %v", summaries, err)
	}
	want := ItemIdentity{FirstName: "Alice", LastName: "Example", Email: "alice@example.com"}
	if *summaries[0].Identity != want || summaries[0].Fields != nil {
		t.Errorf("summary identity = %+v", summaries[0].Identity)
	}
}

func TestCommandLineIdentityTravelsAsNamedFields(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{
		{Name: "first name", Value: "Alice"},
		{Name: "Passport number", Value: "U1234567"},
		{Name: "Employer", Value: "Hisar"},
	}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategoryIdentity, Title: "Alice", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldPassportNumber, Value: "U1234567", Concealed: true},
		{Name: "Employer", Value: "Hisar"},
	}
	if created.Item.Category != item.CategoryIdentity || !reflect.DeepEqual(created.Item.Fields, wantFields) {
		t.Errorf("created %+v", created.Item)
	}

	edited := append([]cliprotocol.Field{}, created.Item.Fields...)
	edited[1].Value = "U7654321"
	var saved cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: created.Item.ID, Revision: created.Item.Revision, Title: "Alice", Fields: &edited,
	}}, &saved)
	if saved.Item.Fields[1].Value != "U7654321" || saved.Item.Category != item.CategoryIdentity {
		t.Errorf("edited %+v", saved.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureIdentities) {
		t.Errorf("hello features = %v", hello.Features)
	}
	if cliprotocol.GetCategoryFeature(item.CategoryIdentity) != cliprotocol.FeatureIdentities {
		t.Error("an identity needs no feature")
	}
}
