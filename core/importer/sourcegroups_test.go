package importer

import (
	"reflect"
	"testing"
)

func TestOnePasswordKeepsPrivateAndSharedVaultsDistinct(t *testing.T) {
	data := []byte(`{"accounts":[{"attrs":{"uuid":"account"},"vaults":[{"attrs":{"uuid":"private","name":"Private","type":"P"},"items":[{"categoryUuid":"003","overview":{"title":"Same"},"details":{"notesPlain":"same secret"}}]},{"attrs":{"uuid":"shared","name":"Private","type":"U"},"items":[{"categoryUuid":"003","overview":{"title":"Same"},"details":{"notesPlain":"same secret"}}]}]}]}`)
	result, err := Parse("source.1pux", buildOnePasswordExport(t, data), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 2 || len(result.Items) != 2 {
		t.Fatalf("lost source boundaries: %+v", result)
	}
	if result.Groups[0].ID != "1password/account/private" || !result.Groups[0].IsPrivate || result.Groups[1].IsPrivate || result.Groups[0].ID == result.Groups[1].ID {
		t.Fatalf("source identities: %+v", result.Groups)
	}
	for _, group := range result.Groups {
		if len(group.Items) != 1 || group.Items[0].Details.Notes != "same secret" {
			t.Fatalf("group lost its item: %+v", group)
		}
	}
}

func TestBitwardenKeepsExactOrganizationCollectionSets(t *testing.T) {
	result, err := Parse("source.json", []byte(`{"encrypted":false,"collections":[{"id":"a","organizationId":"org","name":"Ops"},{"id":"b","organizationId":"org","name":"Finance"}],"items":[{"type":2,"name":"Private","notes":"personal"},{"type":2,"name":"Ops","organizationId":"org","collectionIds":["a"],"notes":"ops"},{"type":2,"name":"Both","organizationId":"org","collectionIds":["b","a"],"notes":"both"},{"type":2,"name":"Both again","organizationId":"org","collectionIds":["a","b"],"notes":"both2"},{"type":2,"name":"Uncollected","organizationId":"org","notes":"org"},{"type":2,"name":"Empty collection set","organizationId":"org","collectionIds":[],"notes":"also org"}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 4 || len(result.Items) != 6 || len(result.Groups[3].Items) != 2 {
		t.Fatalf("source grouping: %+v", result)
	}
	if !result.Groups[0].IsPrivate || result.Groups[1].IsPrivate || len(result.Groups[2].Items) != 2 || result.Groups[3].IsPrivate {
		t.Fatalf("private and org access mixed: %+v", result.Groups)
	}
	if result.Groups[1].ID == result.Groups[2].ID || result.Groups[1].ID == result.Groups[3].ID {
		t.Fatal("different collection sets share an identity")
	}
}

func TestCredentialExchangeKeepsAccountsAndExactCollectionSets(t *testing.T) {
	note := func(id, title, text string) string {
		return `{"id":"` + id + `","title":"` + title + `","credentials":[{"type":"note","content":{"fieldType":"string","value":"` + text + `"}}]}`
	}
	content := `{"version":{"major":1,"minor":0},"exporterRpId":"example.com","exporterDisplayName":"Provider","timestamp":1,"accounts":[` +
		`{"id":"YQ","username":"alice","email":"alice@example.com","collections":[` +
		`{"id":"work","title":"Work","items":[{"item":"w"},{"item":"both"}]},` +
		`{"id":"family","title":"Family","items":[{"item":"both"}],"subCollections":[{"id":"kids","title":"Kids","items":[{"item":"k"}]}]}],` +
		`"items":[` + note("w", "Work note", "work") + `,` + note("p", "Personal note", "same") + `,` + note("both", "Both", "both") + `,` +
		note("k", "Kids note", "kids") + `,` + note("p2", "Personal note", "same") + `,` + note("linked", "Linked", "linked") + `]},` +
		`{"id":"Yg","email":"bob@example.com","collections":[{"id":"team","title":"Team","items":[{"item":"linked","account":"YQ"}]}],` +
		`"items":[` + note("b", "Personal note", "same") + `]}]}`
	result, err := ParseCredentialExchange([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	type group struct {
		name   string
		titles []string
	}
	want := []group{
		{"alice / Work", []string{"Work note"}},
		{"alice", []string{"Personal note"}},
		{"alice / Family, Work", []string{"Both"}},
		{"alice / Family / Kids", []string{"Kids note"}},
		{"alice / Team", []string{"Linked"}},
		{"bob@example.com", []string{"Personal note"}},
	}
	var got []group
	identities := map[string]bool{}
	for _, sourceGroup := range result.SourceGroups() {
		titles := []string{}
		for _, vaultItem := range sourceGroup.Items {
			titles = append(titles, vaultItem.Overview.Title)
		}
		got = append(got, group{sourceGroup.Name, titles})
		identities[sourceGroup.ID] = true
		if !sourceGroup.IsPrivate {
			t.Errorf("%s claims the other app shared it; a collection grants no access", sourceGroup.Name)
		}
	}
	if !reflect.DeepEqual(got, want) || len(identities) != len(want) {
		t.Errorf("groups = %+v, want %+v", got, want)
	}
	if len(result.Items) != 6 || len(result.Skipped) != 1 || result.Skipped[0].Reason != reasonRepeated {
		t.Errorf("only the repeat within one group is dropped: %d items, skipped %+v", len(result.Items), result.Skipped)
	}
}
