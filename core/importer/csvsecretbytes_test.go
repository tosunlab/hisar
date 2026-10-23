package importer

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestCSVPreservesWhitespaceSecrets(t *testing.T) {
	for _, secret := range []string{" ", "\u00a0", "\t ", " secret ", "\u00a0secret\u00a0"} {
		result := parseFixture(t, "generic.csv", []byte("name,password,password,API key\nExample,primary,"+secret+","+secret+","+secret+"\n"), FormatCSV)
		fields := result.Items[0].Details.Fields
		for _, name := range []string{"password", "API key", "Column 5"} {
			if !slices.Contains(fields, item.Field{Name: name, Value: secret, Concealed: true}) {
				t.Errorf("lost %q secret %q in %+v", name, secret, fields)
			}
		}
	}
}

func TestCSVDiscardsWhitespaceOnlyNonsecretExtras(t *testing.T) {
	for _, space := range []string{" ", "\u00a0", "\t "} {
		result := parseFixture(t, "generic.csv", []byte("name,password,nickname\nExample,primary,"+space+"\n,,"+space+"\n"), FormatCSV)
		if len(result.Items) != 1 || len(result.Items[0].Details.Fields) != 0 {
			t.Errorf("nonsecret whitespace %q created content: %+v", space, result.Items)
		}
		result = parseFixture(t, "generic.csv", []byte("name,password,nickname\nExample,primary,"+space+"Alice"+space+"\n"), FormatCSV)
		if len(result.Items) != 1 || !slices.Contains(result.Items[0].Details.Fields, item.Field{Name: "nickname", Value: space + "Alice" + space}) {
			t.Errorf("nonblank display field lost bytes: %+v", result.Items)
		}
	}
}
