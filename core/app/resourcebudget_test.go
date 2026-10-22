package app

import (
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestSaveItemShowsTheServersGrowthBudgetSentence(t *testing.T) {
	test := createUnlockedTestService(t)
	saved, err := test.service.SaveItem(test.personalID, ItemDetail{Title: "Bank", Category: item.CategorySecureNote, Notes: "codes"})
	if err != nil {
		t.Fatal(err)
	}
	const sentence = "This vault's item history is full. Delete items you no longer need, then try again."
	test.apiServer.mutex.Lock()
	test.apiServer.historyFullMessage = sentence
	test.apiServer.mutex.Unlock()

	saved.Notes = "new codes"
	if _, err := test.service.SaveItem(test.personalID, saved); err == nil || err.Error() != sentence {
		t.Fatalf("editing an item in a full vault: %v", err)
	}
	if _, err := test.service.SaveItem(test.personalID, ItemDetail{Title: "Card", Category: item.CategorySecureNote}); err == nil || err.Error() != sentence {
		t.Fatalf("adding an item to a full vault: %v", err)
	}
	if getCachedItem(t, test.service, test.personalID, saved.ID).Details.Notes != "codes" {
		t.Error("the refused edit replaced the cached item")
	}
}
