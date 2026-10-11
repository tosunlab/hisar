package appversion

import (
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	for text, expected := range map[string]Version{
		"0.1.6":                {Major: 0, Minor: 1, Patch: 6},
		"1.0.0-alpha-test":     {Major: 1, PreRelease: []string{"alpha-test"}},
		"1.2.3-rc.1+build.5":   {Major: 1, Minor: 2, Patch: 3, PreRelease: []string{"rc", "1"}},
		"10.20.30+metadata":    {Major: 10, Minor: 20, Patch: 30},
		"999.999.999":          {Major: 999, Minor: 999, Patch: 999},
		"1.2.3-0":              {Major: 1, Minor: 2, Patch: 3, PreRelease: []string{"0"}},
		"1.2.3-x-y-z.--":       {Major: 1, Minor: 2, Patch: 3, PreRelease: []string{"x-y-z", "--"}},
		"0.0.0-label.with.dot": {PreRelease: []string{"label", "with", "dot"}},
	} {
		version, err := Parse(text)
		if err != nil || version.Major != expected.Major || version.Minor != expected.Minor || version.Patch != expected.Patch || !slices.Equal(version.PreRelease, expected.PreRelease) {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", text, version, err, expected)
		}
	}
	for _, text := range []string{"", "development", "1", "1.2", "1.2.3.4", "v1.2.3", "01.2.3", "1.02.3", "1.2.-3", "1.2.3-", "1.2.3-a..b", "1.2.3-01", "1.2.3-a_b", " 1.2.3", "1.2.x"} {
		if version, err := Parse(text); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", text, version)
		}
	}
}

func TestCompareFollowsSemanticVersioning(t *testing.T) {
	allOrdered := []string{
		"0.1.5", "0.1.6-alpha", "0.1.6", "0.2.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-alpha-test",
		"1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.10.0", "2.0.0",
	}
	for firstIndex, firstText := range allOrdered {
		for secondIndex, secondText := range allOrdered {
			first, _ := Parse(firstText)
			second, _ := Parse(secondText)
			expected := 0
			if firstIndex < secondIndex {
				expected = -1
			} else if firstIndex > secondIndex {
				expected = 1
			}
			if result := Compare(first, second); result != expected {
				t.Errorf("Compare(%s, %s) = %d, want %d", firstText, secondText, result, expected)
			}
		}
	}
	withBuild, _ := Parse("1.0.0+build.7")
	withoutBuild, _ := Parse("1.0.0")
	if Compare(withBuild, withoutBuild) != 0 {
		t.Error("build metadata changed the order")
	}
}

func TestIsOlder(t *testing.T) {
	for _, testCase := range []struct {
		current, minimum string
		isOlder          bool
	}{
		{"0.1.5", "0.1.6", true},
		{"0.1.6", "0.1.6", false},
		{"0.1.7", "0.1.6", false},
		{"1.0.0-alpha-test", "1.0.0", true},
		{"1.0.0", "1.0.0-alpha-test", false},
		{"1.2.3-label", "1.2.3", true},
	} {
		isOlder, err := IsOlder(testCase.current, testCase.minimum)
		if err != nil || isOlder != testCase.isOlder {
			t.Errorf("IsOlder(%s, %s) = %v, %v", testCase.current, testCase.minimum, isOlder, err)
		}
	}
	if _, err := IsOlder("development", "1.0.0"); err == nil {
		t.Error("a development build compared as a version")
	}
}

func TestString(t *testing.T) {
	for _, text := range []string{"0.1.6", "1.0.0-alpha-test", "1.2.3-rc.1"} {
		if version, _ := Parse(text); version.String() != text {
			t.Errorf("String() = %q, want %q", version.String(), text)
		}
	}
	if version, _ := Parse("1.2.3+build"); version.String() != "1.2.3" {
		t.Errorf("String() kept build metadata: %q", version.String())
	}
}
