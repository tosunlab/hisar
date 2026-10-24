package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
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
