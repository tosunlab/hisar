package app

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestCreditCardIsStoredAsNamedFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryCreditCard, Title: "Visa",
		Card:     &ItemCard{Cardholder: " Alice Smith ", Number: "4111111111111111", Expiry: "1229", SecurityCode: "123", PIN: "0000", BillingAddress: "1 Main St"},
		Fields:   []ItemField{{Name: "Bank", Value: "İş Bankası"}},
		Username: "hidden", Password: "hidden", URLs: []string{"https://bank.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, saved.ID)
	wantFields := []item.Field{
		{Name: item.FieldCardholder, Value: "Alice Smith"},
		{Name: item.FieldCardType, Value: "Visa"},
		{Name: item.FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true},
		{Name: item.FieldExpiry, Value: "12/29"},
		{Name: item.FieldSecurityCode, Value: "123", Concealed: true},
		{Name: item.FieldPIN, Value: "0000", Concealed: true},
		{Name: item.FieldBillingAddress, Value: "1 Main St"},
		{Name: "Bank", Value: "İş Bankası"},
	}
	if !reflect.DeepEqual(stored.Details.Fields, wantFields) {
		t.Errorf("stored fields =\n%+v\nwant\n%+v", stored.Details.Fields, wantFields)
	}
	if stored.Overview.Category != item.CategoryCreditCard || stored.Overview.Username != "" || stored.Details.Password != "" || stored.Overview.URLs != nil {
		t.Errorf("stored card = %+v", stored)
	}
	if saved.Card == nil || saved.Card.Brand != "Visa" || saved.Card.Last4 != "1111" || saved.Card.Type != "Visa" || saved.Card.Expiry != "12/29" || saved.Card.NumberWarning != "" {
		t.Errorf("saved card = %+v", saved.Card)
	}
	if !reflect.DeepEqual(saved.Fields, []ItemField{{Name: "Bank", Value: "İş Bankası"}}) {
		t.Errorf("other fields = %+v", saved.Fields)
	}

	saved.Card.SecurityCode, saved.Card.Number, saved.Card.Type = "456", "4111 1111 1111 1112", "visa electron"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Card.SecurityCode != "456" || edited.Card.NumberWarning == "" || len(edited.Fields) != 1 {
		t.Errorf("edited card = %+v, fields %+v", edited.Card, edited.Fields)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil || len(summaries) != 1 || summaries[0].Card == nil {
		t.Fatalf("summaries = %+v, %v", summaries, err)
	}
	if card := summaries[0].Card; card.Brand != "Visa" || card.Last4 != "1112" || card.Type != "Visa" || card.Number != "" || card.SecurityCode != "" || card.PIN != "" || card.Cardholder != "" {
		t.Errorf("summary card = %+v", card)
	}
	if description := service.DescribeCard("3782 822463 10005", "13/27"); description.Brand != "American Express" || description.ExpiryWarning == "" {
		t.Errorf("DescribeCard = %+v", description)
	}
}

func TestCommandLineCreditCardTravelsAsNamedFields(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	fields := []cliprotocol.Field{
		{Name: "Card number", Value: "5555555555554444"},
		{Name: "Security code", Value: "321"},
		{Name: "Note", Value: "work card"},
	}
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategoryCreditCard, Title: "Mastercard", Fields: &fields,
	}}, &created)
	wantFields := []cliprotocol.Field{
		{Name: item.FieldCardType, Value: "Mastercard"},
		{Name: item.FieldCardNumber, Value: "5555 5555 5555 4444", Concealed: true},
		{Name: item.FieldSecurityCode, Value: "321", Concealed: true},
		{Name: "Note", Value: "work card"},
	}
	if created.Item.Category != item.CategoryCreditCard || !reflect.DeepEqual(created.Item.Fields, wantFields) {
		t.Errorf("created %+v", created.Item)
	}

	edited := append([]cliprotocol.Field{}, created.Item.Fields...)
	edited[2].Value = "999"
	var saved cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: created.Item.ID, Revision: created.Item.Revision, Title: "Mastercard", Fields: &edited,
	}}, &saved)
	if saved.Item.Fields[2].Value != "999" || saved.Item.Category != item.CategoryCreditCard {
		t.Errorf("edited %+v", saved.Item)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureCreditCards) {
		t.Errorf("hello features = %v", hello.Features)
	}
}

func TestImportedCardKeepsItsWebsiteThroughAnEdit(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	fileWriter, err := writer.Create("export.data")
	if err != nil {
		t.Fatal(err)
	}
	fileWriter.Write([]byte(`{"accounts":[{"vaults":[{"items":[{"categoryUuid":"002","overview":{"title":"Amex","url":"https://americanexpress.com"},
		"details":{"sections":[{"fields":[{"id":"ccnum","value":{"creditCardNumber":"378282246310005"}}]}]}}]}]}]}`))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.1pux")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	setImportPath(service, path)
	if _, err := service.ChooseImportFile(test.personalID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartImport(test.personalID, false); err != nil {
		t.Fatal(err)
	}
	runImportToEnd(t, service)

	website := ItemField{Name: "Website", Value: "https://americanexpress.com"}
	imported, err := service.GetItem(test.personalID, listItemsByTitle(t, service, test.personalID)["Amex"].ID)
	if err != nil || imported.Card == nil || !slices.Contains(imported.Fields, website) {
		t.Fatalf("imported card = %+v %v", imported, err)
	}
	edited, err := service.SaveItem(test.personalID, imported)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(edited.Fields, website) || edited.Card.Last4 != "0005" {
		t.Errorf("after an edit the card = %+v, fields %+v", edited.Card, edited.Fields)
	}
}
