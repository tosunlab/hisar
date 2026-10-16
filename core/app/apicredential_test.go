package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestAPICredentialIsStoredAsNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	service.config.Now = func() time.Time { return time.Date(2030, 1, 15, 12, 0, 0, 0, time.UTC) }
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryAPICredential, Title: "Stripe",
		APICredential: &ItemAPICredential{Service: " Stripe ", KeyID: "key_1", Secret: "sk_live_123", Host: "api.stripe.com", Expiry: "2029/12/31"},
		Fields:        []ItemField{{Name: "Scope", Value: "read"}},
		Username:      "alice", URLs: []string{"https://dashboard.stripe.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldService, Value: "Stripe"},
		{Name: item.FieldKeyID, Value: "key_1"},
		{Name: item.FieldSecret, Value: "sk_live_123", Concealed: true},
		{Name: item.FieldHost, Value: "api.stripe.com"},
		{Name: item.FieldExpiry, Value: "2029-12-31"},
		{Name: "Scope", Value: "read"},
		{Name: "Username", Value: "alice"},
		{Name: "Website", Value: "https://dashboard.stripe.com"},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if stored.Overview.Category != item.CategoryAPICredential || stored.Overview.Username != "" || len(stored.Overview.URLs) != 0 {
		t.Errorf("stored credential = %+v", stored.Overview)
	}
	if saved.APICredential == nil || saved.APICredential.Secret != "sk_live_123" || saved.APICredential.ExpiryWarning != "This credential has expired" || len(saved.Fields) != 3 {
		t.Errorf("saved credential = %+v, fields %+v", saved.APICredential, saved.Fields)
	}

	saved.APICredential.Expiry = "2031-01-01"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil {
		t.Fatal(err)
	}
	if edited.APICredential.Expiry != "2031-01-01" || edited.APICredential.ExpiryWarning != "" || edited.APICredential.Secret != "sk_live_123" || len(edited.Fields) != 3 {
		t.Errorf("edited credential = %+v, fields %+v", edited.APICredential, edited.Fields)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil || len(summaries) != 1 || summaries[0].APICredential == nil {
		t.Fatalf("summaries = %+v, %v", summaries, err)
	}
	if *summaries[0].APICredential != (ItemAPICredential{Service: "Stripe", Host: "api.stripe.com"}) || summaries[0].Fields != nil {
		t.Errorf("summary credential = %+v", summaries[0].APICredential)
	}
}

func TestCommandLineAPICredentialTravelsAsNamedFields(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{
		{Name: "service", Value: "Stripe"},
		{Name: "Secret", Value: "sk_live_123"},
		{Name: "Scope", Value: "read"},
	}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategoryAPICredential, Title: "Stripe", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{
		{Name: item.FieldService, Value: "Stripe"},
		{Name: item.FieldSecret, Value: "sk_live_123", Concealed: true},
		{Name: "Scope", Value: "read"},
	}
	if created.Item.Category != item.CategoryAPICredential || !reflect.DeepEqual(created.Item.Fields, wantFields) {
		t.Errorf("created %+v", created.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureAPICredentials) {
		t.Errorf("hello features = %v", hello.Features)
	}
	if cliprotocol.GetCategoryFeature(item.CategoryAPICredential) != cliprotocol.FeatureAPICredentials {
		t.Error("an API credential needs no feature")
	}
}

func TestAPICredentialFieldsResolveAsSecretReferences(t *testing.T) {
	test := createCommandLineTest(t)
	service, personalID := test.testService.service, test.testService.personalID
	if _, err := service.SaveItem(personalID, ItemDetail{
		Category: item.CategoryAPICredential, Title: "Stripe",
		APICredential: &ItemAPICredential{Service: "Stripe", KeyID: "key_1", Secret: "sk_live_123", Host: "api.stripe.com", Expiry: "2031-01-01"},
	}); err != nil {
		t.Fatal(err)
	}
	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{
		"hisar://Personal/Stripe/secret",
		"hisar://Personal/Stripe/key%20ID",
		"hisar://Personal/Stripe/host",
		"hisar://Personal/Stripe/expiry",
		"hisar://Personal/Stripe/service",
	}, CommandLine: []string{"npm", "test"}}, &result)
	if want := []string{"sk_live_123", "key_1", "api.stripe.com", "2031-01-01", "Stripe"}; !reflect.DeepEqual(result.Values, want) {
		t.Errorf("values = %q, want %q", result.Values, want)
	}
}
