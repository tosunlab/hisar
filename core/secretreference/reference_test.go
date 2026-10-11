package secretreference

import (
	"strings"
	"testing"
)

func TestParseReference(t *testing.T) {
	allCase := []struct {
		text     string
		expected Reference
	}{
		{"hisar://Personal/GitHub/password", Reference{"Personal", "GitHub", "password"}},
		{"HISAR://Personal/GitHub/password", Reference{"Personal", "GitHub", "password"}},
		{"hisar://Work/AWS%20prod/secret%20access%20key", Reference{"Work", "AWS prod", "secret access key"}},
		{"hisar://Personal/GitHub/private key", Reference{"Personal", "GitHub", "private key"}},
		{"hisar://Team%2FOps/a%2fb%25c/field", Reference{"Team/Ops", "a/b%c", "field"}},
		{"hisar://Kişisel/Gİzli/şifre", Reference{"Kişisel", "Gİzli", "şifre"}},
		{"hisar://Ki%C5%9Fisel/x/y", Reference{"Kişisel", "x", "y"}},
		{"hisar://v/item?query#fragment/f", Reference{"v", "item?query#fragment", "f"}},
	}
	for _, testCase := range allCase {
		reference, err := Parse(testCase.text)
		if err != nil || reference != testCase.expected {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", testCase.text, reference, err, testCase.expected)
		}
	}
}

func TestParseReferenceError(t *testing.T) {
	allCase := []struct {
		text    string
		message string
	}{
		{"https://Personal/GitHub/password", "starts with hisar://"},
		{"hisar://Personal/GitHub", "has 2"},
		{"hisar://Personal/GitHub/password/extra", "has 4"},
		{"hisar:///GitHub/password", "vault of a secret reference is empty"},
		{"hisar://Personal//password", "item of a secret reference is empty"},
		{"hisar://Personal/GitHub/", "field of a secret reference is empty"},
		{"hisar://Personal/GitHub/%20", "field of a secret reference is empty"},
		{"hisar://Personal/Git%zzHub/password", `"%zz" is not a valid percent-encoding`},
		{"hisar://Personal/GitHub/pass%2", `"%2" is not a valid percent-encoding`},
		{"hisar://Personal/%FF/password", "must decode to UTF-8"},
		{"hisar://Personal/\xff/password", "must be UTF-8"},
	}
	for _, testCase := range allCase {
		_, err := Parse(testCase.text)
		if err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("Parse(%q) error = %v; want %q", testCase.text, err, testCase.message)
		}
	}
}

func TestEncodeReference(t *testing.T) {
	allCase := []struct {
		reference Reference
		expected  string
	}{
		{Reference{"Personal", "GitHub", "password"}, "hisar://Personal/GitHub/password"},
		{Reference{"Work", "AWS prod", "secret access key"}, "hisar://Work/AWS%20prod/secret%20access%20key"},
		{Reference{"Team/Ops", "a%b", "c#d?e"}, "hisar://Team%2FOps/a%25b/c%23d%3Fe"},
		{Reference{"Kişisel", "Gİzli", "şifre"}, "hisar://Kişisel/Gİzli/şifre"},
		{Reference{"$HOME", "`x`", "a'b\"c"}, "hisar://%24HOME/%60x%60/a%27b%22c"},
		{Reference{"v.1", "my_item-2~", "f"}, "hisar://v.1/my_item-2~/f"},
	}
	for _, testCase := range allCase {
		if encoded := testCase.reference.String(); encoded != testCase.expected {
			t.Errorf("%+v.String() = %q; want %q", testCase.reference, encoded, testCase.expected)
		}
	}
}

func TestReferenceRoundTrip(t *testing.T) {
	allName := []string{"Personal", "a/b", "100%", "spaces in name", "tab\there", "new\nline", "Kişisel 🔑", "?#&=+", "%2F", "'quoted\"", "\\back"}
	for _, vaultName := range allName {
		for _, itemTitle := range allName {
			reference := Reference{Vault: vaultName, Item: itemTitle, Field: "custom " + vaultName}
			encoded := reference.String()
			if strings.ContainsAny(encoded, " \t\n'\"\\$`") {
				t.Errorf("%q is not one shell word", encoded)
			}
			parsed, err := Parse(encoded)
			if err != nil || parsed != reference {
				t.Errorf("round trip of %+v through %q gave %+v, %v", reference, encoded, parsed, err)
			}
		}
	}
}

func TestIsReference(t *testing.T) {
	for text, expected := range map[string]bool{
		"hisar://a/b/c": true, "Hisar://a/b/c": true, "hisar://": true,
		"hisar:/a/b/c": false, " hisar://a/b/c": false, "postgres://hisar://a/b/c": false, "": false,
	} {
		if IsReference(text) != expected {
			t.Errorf("IsReference(%q) = %v", text, !expected)
		}
	}
}
