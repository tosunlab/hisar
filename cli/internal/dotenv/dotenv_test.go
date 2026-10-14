package dotenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseEnvironmentFile(t *testing.T) {
	content := "\uFEFF# database settings\r\n" +
		"DATABASE_URL=postgres://localhost/app\r\n" +
		"\n" +
		"   # indented comment\n" +
		"export GITHUB_TOKEN=hisar://Personal/GitHub/token\n" +
		"export\tTABBED=1\n" +
		"exported_name=keeps the word export\n" +
		"SPACED = value with spaces   # trailing comment\n" +
		"HASH_INSIDE=abc#def\n" +
		"ONLY_COMMENT= # nothing\n" +
		"EMPTY=\n" +
		"SINGLE='$HOME \\n # not a comment'\n" +
		"DOUBLE=\"line\\none\\ttab \\\"quoted\\\" \\\\ \\$HOME \\x\" # comment\n" +
		"MULTILINE=\"-----BEGIN KEY-----\n" +
		"abc\n" +
		"-----END KEY-----\"\n" +
		"SINGLE_MULTILINE='first\n" +
		"second'\n" +
		"REFERENCE=\"hisar://Work/AWS prod/secret key\"\n" +
		"DUPLICATE=first\n" +
		"DUPLICATE=second\n"
	variables, err := Parse(".env", content)
	if err != nil {
		t.Fatal(err)
	}
	expectedValues := []Variable{
		{"DATABASE_URL", "postgres://localhost/app"},
		{"GITHUB_TOKEN", "hisar://Personal/GitHub/token"},
		{"TABBED", "1"},
		{"exported_name", "keeps the word export"},
		{"SPACED", "value with spaces"},
		{"HASH_INSIDE", "abc#def"},
		{"ONLY_COMMENT", ""},
		{"EMPTY", ""},
		{"SINGLE", "$HOME \\n # not a comment"},
		{"DOUBLE", "line\none\ttab \"quoted\" \\ $HOME \\x"},
		{"MULTILINE", "-----BEGIN KEY-----\nabc\n-----END KEY-----"},
		{"SINGLE_MULTILINE", "first\nsecond"},
		{"REFERENCE", "hisar://Work/AWS prod/secret key"},
		{"DUPLICATE", "first"},
		{"DUPLICATE", "second"},
	}
	if !slices.Equal(variables, expectedValues) {
		t.Errorf("parsed:\n%q\nwant:\n%q", variables, expectedValues)
	}
}

func TestParseEnvironmentFileSingleQuotesAreLiteral(t *testing.T) {
	variables, err := Parse(".env", `LITERAL='a\nb $HOME "x" # y'`)
	if err != nil || len(variables) != 1 || variables[0].Value != `a\nb $HOME "x" # y` {
		t.Errorf("parsed %q, %v", variables, err)
	}
}

func TestParseEnvironmentFileErrors(t *testing.T) {
	cases := []struct {
		content string
		message string
	}{
		{"JUST_A_NAME\n", ".env:1: expected NAME=value"},
		{"OK=1\n1BAD=secret-value\n", `.env:2: "1BAD" is not a valid variable name`},
		{"BAD-NAME=secret-value", `.env:1: "BAD-NAME" is not a valid variable name`},
		{"=secret-value", `.env:1: "" is not a valid variable name`},
		{"A=1\nOPEN=\"secret-value\nstill open\n", ".env:2: OPEN has a quote that is never closed"},
		{"AFTER='secret-value' trailing", ".env:1: AFTER has text after its closing quote"},
	}
	for _, testCase := range cases {
		_, err := Parse(".env", testCase.content)
		if err == nil || err.Error() != testCase.message {
			t.Errorf("%q: %v; want %q", testCase.content, err, testCase.message)
		}
		if err != nil && strings.Contains(err.Error(), "secret-value") {
			t.Errorf("the error shows the value: %v", err)
		}
	}
}

func TestReadEnvironmentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.env")
	os.WriteFile(path, []byte("A=1\n"), 0o600)
	if variables, err := ReadFile(path); err != nil || len(variables) != 1 {
		t.Errorf("read %q, %v", variables, err)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.env")); err == nil {
		t.Error("a missing file was read")
	}
}

