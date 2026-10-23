package importer

import (
	"fmt"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestLastPassPreservesWhitespaceSecrets(t *testing.T) {
	for _, secret := range []string{" ", "\u00a0", "\t ", " secret ", "\u00a0secret\u00a0"} {
		t.Run(fmt.Sprintf("%q", secret), func(t *testing.T) {
			result := parseFixture(t, "lastpass.csv", buildLastPassExport(t, [][]string{
				lastPassNoteRow("Server", "", "NoteType:Server", "Password:"+secret),
				lastPassNoteRow("Custom", "", "NoteType:Custom", "API Key:"+secret),
			}), FormatLastPassCSV)
			if got := findTestItem(t, result, "Server").Details.Password; got != secret {
				t.Errorf("password = %q, want %q", got, secret)
			}
			if !slices.Contains(findTestItem(t, result, "Custom").Details.Fields, item.Field{Name: "API Key", Value: secret, Concealed: true}) {
				t.Error("lost custom secret")
			}
		})
	}
}
