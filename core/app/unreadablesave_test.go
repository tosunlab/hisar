package app

import "testing"

func TestJudgeUnreadablePlaceholderCannotOverwriteCiphertext(t *testing.T) {
	f := createUnlockedTestService(t)
	injectJudgeBadItem(t, f)
	records, err := f.service.ListItems(f.personalID)
	if err != nil {
		t.Fatal(err)
	}
	var placeholder ItemDetail
	for _, r := range records {
		if r.ID == "judge-bad" {
			placeholder = r
		}
	}
	if placeholder.ReadIssue == nil {
		t.Fatal("missing reviewed issue")
	}
	saved, err := f.service.SaveItem(f.personalID, placeholder)
	if err == nil {
		t.Fatalf("unreadable issue became default-filled plaintext and overwrote ciphertext, title=%q password=%q revision=%s", saved.Title, saved.Password, saved.Revision)
	}
}
