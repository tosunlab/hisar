package dotenv

import (
	"slices"
	"testing"
)

func TestBuildEnvironment(t *testing.T) {
	environ := []string{
		"PATH=/usr/bin",
		"PLAIN_IN_BOTH=from the environment",
		"REFERENCE_IN_BOTH=hisar://Personal/Environment/password",
		"REFERENCE_ONLY_HERE=hisar://Personal/Only/password",
		"BROKEN_ENTRY",
	}
	fileVariables := []Variable{
		{Name: "PLAIN_IN_BOTH", Value: "from the first file"},
		{Name: "REFERENCE_IN_BOTH", Value: "hisar://Personal/First/password"},
		{Name: "FILE_ONLY", Value: "first"},
		{Name: "REFERENCE_IN_BOTH", Value: "hisar://Personal/Second/password"},
		{Name: "FILE_ONLY", Value: "second"},
	}
	variables := BuildEnvironment(environ, fileVariables)
	expectedValues := []Variable{
		{Name: "PATH", Value: "/usr/bin"},
		{Name: "PLAIN_IN_BOTH", Value: "from the environment"},
		{Name: "REFERENCE_IN_BOTH", Value: "hisar://Personal/Second/password"},
		{Name: "REFERENCE_ONLY_HERE", Value: "hisar://Personal/Only/password"},
		{Name: "FILE_ONLY", Value: "second"},
	}
	if !slices.Equal(variables, expectedValues) {
		t.Errorf("environment:\n%q\nwant:\n%q", variables, expectedValues)
	}
	references := ListReferences(append(variables, Variable{Name: "AGAIN", Value: "hisar://Personal/Only/password"}))
	if !slices.Equal(references, []string{"hisar://Personal/Second/password", "hisar://Personal/Only/password"}) {
		t.Errorf("references %q", references)
	}
}
