package dotenv

import (
	"reflect"
	"strings"
	"testing"
)

func TestDotenvLayoutRewritesOnlyChosenValues(t *testing.T) {
	const content = "# Database\n" +
		"export DATABASE_PASSWORD=\"s3cr3t \\\"quoted\\\"\" # rotate yearly\n" +
		"PORT=3000\n" +
		"\n" +
		"  API_TOKEN = 'tok en'   # comment stays\n" +
		"PRIVATE_KEY=\"-----BEGIN KEY-----\n" +
		"line two\n" +
		"-----END KEY-----\"\n" +
		"STRIPE_KEY=sk_live_1 # first\n" +
		"STRIPE_KEY=sk_live_2\n" +
		"EMPTY=\n" +
		"ALREADY=hisar://Personal/App/token\n" +
		"NODE_ENV=production"
	layout, err := ParseLayout(".env", content)
	if err != nil {
		t.Fatal(err)
	}
	parsedValues, err := Parse(".env", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsedValues) != len(layout.entries) {
		t.Fatalf("hisar run reads %d variables, the layout %d", len(parsedValues), len(layout.entries))
	}
	for index, parsed := range parsedValues {
		if entry := layout.entries[index]; entry.name != parsed.Name || entry.value != parsed.Value {
			t.Errorf("entry %d: layout %q=%q, hisar run %q=%q", index, entry.name, entry.value, parsed.Name, parsed.Value)
		}
	}
	if layout.Render(nil) != content {
		t.Errorf("rendering without changes altered the file:\n%s", layout.Render(nil))
	}

	rendered := layout.Render(map[string]string{
		"DATABASE_PASSWORD": "hisar://Personal/app/DATABASE_PASSWORD",
		"API_TOKEN":         "hisar://Personal/app/API_TOKEN",
		"PRIVATE_KEY":       "hisar://Personal/app/PRIVATE_KEY",
		"STRIPE_KEY":        "hisar://Personal/app/STRIPE_KEY",
	})
	want := "# Database\n" +
		"export DATABASE_PASSWORD=\"hisar://Personal/app/DATABASE_PASSWORD\" # rotate yearly\n" +
		"PORT=3000\n" +
		"\n" +
		"  API_TOKEN = 'hisar://Personal/app/API_TOKEN'   # comment stays\n" +
		"PRIVATE_KEY=\"hisar://Personal/app/PRIVATE_KEY\"\n" +
		"STRIPE_KEY=hisar://Personal/app/STRIPE_KEY # first\n" +
		"STRIPE_KEY=hisar://Personal/app/STRIPE_KEY\n" +
		"EMPTY=\n" +
		"ALREADY=hisar://Personal/App/token\n" +
		"NODE_ENV=production"
	if rendered != want {
		t.Errorf("rendered:\n%s\nwant:\n%s", rendered, want)
	}
	rewritten, err := Parse(".env", rendered)
	if err != nil {
		t.Fatalf("hisar run can't read the rewritten file: %v", err)
	}
	for _, variable := range rewritten {
		if variable.Name == "DATABASE_PASSWORD" && variable.Value != "hisar://Personal/app/DATABASE_PASSWORD" {
			t.Errorf("hisar run reads %q", variable.Value)
		}
	}

	variables := layout.ListVariables()
	byName := map[string]LayoutVariable{}
	for _, variable := range variables {
		byName[variable.Name] = variable
	}
	if stripe := byName["STRIPE_KEY"]; stripe.Value != "sk_live_2" || stripe.OccurrenceCount != 2 || !stripe.HasOtherValue {
		t.Errorf("duplicate key: %+v", stripe)
	}
	if privateKey := byName["PRIVATE_KEY"]; privateKey.Value != "-----BEGIN KEY-----\nline two\n-----END KEY-----" {
		t.Errorf("multiline value: %q", privateKey.Value)
	}
	storable, skipped := StorableVariables(variables)
	if len(storable) != 6 || !reflect.DeepEqual(skipped, []string{"EMPTY (empty)", "ALREADY (already a secret reference)"}) {
		t.Errorf("storable %d, skipped %v", len(storable), skipped)
	}
}

func TestDotenvLayoutKeepsLineEndingsAndByteOrderMark(t *testing.T) {
	content := dotenvByteOrderMark + "TOKEN=abc\r\nPORT=1\r\n"
	layout, err := ParseLayout(".env", content)
	if err != nil {
		t.Fatal(err)
	}
	if rendered := layout.Render(map[string]string{"TOKEN": "hisar://V/I/TOKEN"}); rendered != dotenvByteOrderMark+"TOKEN=hisar://V/I/TOKEN\r\nPORT=1\r\n" {
		t.Errorf("rendered %q", rendered)
	}
	empty, err := ParseLayout(".env", "")
	if err != nil || empty.Render(nil) != "" {
		t.Errorf("empty file: %v", err)
	}
}

func TestDotenvLayoutErrorsNameTheLineNotTheValue(t *testing.T) {
	for content, want := range map[string]string{
		"A=1\nB=\"never closed\nC=2": ".env:2: has a quote that is never closed",
		"A=1\nnot a variable":        ".env:2: expected NAME=value",
		"1BAD=x":                     `.env:1: "1BAD" is not a valid variable name`,
		"A='x' trailing":             ".env:1: has text after its closing quote",
	} {
		_, err := ParseLayout(".env", content)
		if err == nil || err.Error() != want {
			t.Errorf("%q: %v, want %s", content, err, want)
		}
		if err != nil && strings.Contains(err.Error(), "never closed\n") {
			t.Errorf("the error shows a value: %v", err)
		}
	}
}

func TestClassifyDotenvVariable(t *testing.T) {
	for _, testCase := range []struct {
		name, value string
		isSecret    bool
	}{
		{"DATABASE_PASSWORD", "x", true},
		{"STRIPE_SECRET_KEY", "sk_live", true},
		{"GITHUB_TOKEN", "ghp", true},
		{"API_KEY", "abc", true},
		{"SESSION_SALT", "abc", true},
		{"PORT", "3000", false},
		{"NODE_ENV", "production", false},
		{"LOG_LEVEL", "debug", false},
		{"API_URL", "https://api.example.com/v1", false},
		{"REDIS_URL", "redis://localhost:6379", false},
		{"DATABASE_URL", "postgres://app:hunter2@db.internal:5432/app", true},
		{"WEBHOOK_URL", "https://hooks.example.com/x", true},
		{"CALLBACK_URL", "https://example.com/cb?token=abc", true},
		{"FEATURE_FLAG", "true", false},
		{"RETRIES", "5", false},
		{"STRIPE_ACCOUNT", "acct_1Nx", true},
		{"MAILER_FROM", "Hisar <noreply@example.com>", true},
	} {
		if isSecret, reason := classifyDotenvVariable(testCase.name, testCase.value); isSecret != testCase.isSecret || reason == "" {
			t.Errorf("%s=%s: secret %v (%s), want %v", testCase.name, testCase.value, isSecret, reason, testCase.isSecret)
		}
	}
}
