package app

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestSaveItemKeepsTagsOnEveryCategory(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	saved, err := service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", Tags: []string{" work ", "", "dev", "work"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(saved.Tags, []string{"work", "dev"}) {
		t.Errorf("saved tags = %q", saved.Tags)
	}
	untagged, err := service.SaveItem(test.personalID, ItemDetail{Title: "Bank", Category: item.CategorySecureNote, Notes: "codes"})
	if err != nil {
		t.Fatal(err)
	}
	if untagged.Tags == nil || len(untagged.Tags) != 0 {
		t.Errorf("an item without tags lists %#v", untagged.Tags)
	}
	if getCachedItem(t, service, test.personalID, untagged.ID).Overview.Tags != nil {
		t.Error("an item without tags stores a tags field")
	}

	summaries, err := service.ListItems(test.personalID)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		if summary.ID == saved.ID && !slices.Equal(summary.Tags, []string{"work", "dev"}) {
			t.Errorf("summary tags = %q", summary.Tags)
		}
	}

	edited := saved
	edited.Tags = []string{"personal"}
	if saved, err = service.SaveItem(test.personalID, edited); err != nil || !slices.Equal(saved.Tags, []string{"personal"}) {
		t.Errorf("edited tags = %q %v", saved.Tags, err)
	}
	edited = saved
	edited.Tags = nil
	if saved, err = service.SaveItem(test.personalID, edited); err != nil || len(saved.Tags) != 0 {
		t.Errorf("cleared tags = %q %v", saved.Tags, err)
	}

	generated, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	generated.Tags = []string{"servers"}
	savedKey, err := service.SaveItem(test.personalID, generated)
	if err != nil || !slices.Equal(savedKey.Tags, []string{"servers"}) || savedKey.PublicKey != generated.PublicKey {
		t.Errorf("ssh key = %+v %v", savedKey, err)
	}
}
