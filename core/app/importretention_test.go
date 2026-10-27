package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/importer"
)

func TestImportRetentionUsesContentAndIgnoresExactRepeats(t *testing.T) {
	for _, kind := range []string{"identical", "different password", "different recovery field", "in-file repeat"} {
		t.Run(kind, func(t *testing.T) {
			test := createUnlockedTestService(t)
			csv := "name,url,username,password,Recovery code\nGitHub,https://github.com,alice,old-password,\n"
			if kind != "in-file repeat" {
				if _, err := test.service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Password: "old-password"}); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "different password":
				csv = strings.Replace(csv, "old-password", "new-password", 1)
			case "different recovery field":
				csv = strings.TrimSuffix(csv, ",\n") + ",recovery-secret\n"
			case "in-file repeat":
				csv += "GitHub,https://github.com,alice,old-password,\n"
			}
			path := writeImportFile(t, "source.csv", csv)
			setImportPath(test.service, path)
			if _, err := test.service.ChooseImportFile(test.personalID, "csv"); err != nil {
				t.Fatal(err)
			}
			if _, err := test.service.StartImport(test.personalID, true); err != nil {
				t.Fatal(err)
			}
			progress, _ := runImportToEnd(t, test.service)
			encoded, _ := json.Marshal(progress)
			var counts struct {
				LeftBehindCount int `json:"leftBehindCount"`
			}
			if err := json.Unmarshal(encoded, &counts); err != nil {
				t.Fatal(err)
			}
			want := 0
			if strings.HasPrefix(kind, "different") {
				want = 1
			}
			if counts.LeftBehindCount != want {
				t.Errorf("left behind = %d, want %d", counts.LeftBehindCount, want)
			}
			if kind != "in-file repeat" {
				if len(progress.DuplicateOutcomes) != 1 || progress.DuplicateOutcomes[0].Title != "GitHub" {
					t.Fatalf("duplicate report = %+v", progress.DuplicateOutcomes)
				}
				if want > 0 {
					if len(progress.Skipped) != 1 || !strings.Contains(progress.Skipped[0].Reason, "only in the source") {
						t.Errorf("late omission report = %+v", progress.Skipped)
					}
				} else if !strings.Contains(progress.DuplicateOutcomes[0].Reason, "Identical duplicate") {
					t.Errorf("identical duplicate report = %+v", progress.DuplicateOutcomes)
				}
			}
			err := test.service.DeleteImportFile()
			if want > 0 {
				if err == nil {
					t.Error("deleted source with distinct content")
				}
				if _, err := os.Stat(path); err != nil {
					t.Error(err)
				}
			} else {
				if err != nil {
					t.Errorf("cannot delete fully imported source: %v", err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("source still exists: %v", err)
				}
			}
		})
	}
}

func TestImportReportIncludesDuplicatesChosenForInclusion(t *testing.T) {
	test := createUnlockedTestService(t)
	if _, err := test.service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Password: "password"}); err != nil {
		t.Fatal(err)
	}
	path := writeImportFile(t, "source.csv", "name,url,username,password\nGitHub,https://github.com,alice,password\n")
	setImportPath(test.service, path)
	if _, err := test.service.ChooseImportFile(test.personalID, "csv"); err != nil {
		t.Fatal(err)
	}
	started, err := test.service.StartImport(test.personalID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(started.DuplicateOutcomes) != 0 {
		t.Fatal("a queued duplicate was reported as saved")
	}
	progress, _ := runImportToEnd(t, test.service)
	if progress.SavedCount != 1 || len(progress.DuplicateOutcomes) != 1 ||
		!strings.Contains(progress.DuplicateOutcomes[0].Reason, "Included the duplicate") {
		t.Fatalf("included duplicate report = %+v", progress)
	}
}

func TestImportProgressRetainsOmissionAndFailureReasons(t *testing.T) {
	service := &Service{serviceState: &serviceState{pendingImport: &pendingImport{
		sourceName: "Passwords", savedCount: 1, isSourceKept: true,
		skipped:           []importer.SkippedEntry{{Title: "GitHub", Reason: "Unsupported extension data from a retained login."}},
		failed:            []ImportSkippedEntry{{Title: "Figma", Reason: "The destination refused this save."}},
		duplicateOutcomes: []ImportSkippedEntry{{Title: "Slack", Reason: "Identical duplicate already present in Hisar."}},
	}}}
	progress := service.buildImportProgress()
	if !progress.IsDone || !progress.IsSourceKept || progress.LeftBehindCount != 2 ||
		len(progress.Skipped) != 1 || progress.Skipped[0].Reason != "Unsupported extension data from a retained login." ||
		len(progress.Failed) != 1 || progress.Failed[0].Reason != "The destination refused this save." ||
		len(progress.DuplicateOutcomes) != 1 {
		t.Fatalf("completion report = %+v", progress)
	}
	progress.Skipped[0].Reason = "changed"
	progress.Failed[0].Reason = "changed"
	progress.DuplicateOutcomes[0].Reason = "changed"
	again := service.buildImportProgress()
	if again.Skipped[0].Reason == "changed" || again.Failed[0].Reason == "changed" || again.DuplicateOutcomes[0].Reason == "changed" {
		t.Fatal("editing a returned report changed the retained report")
	}
}
