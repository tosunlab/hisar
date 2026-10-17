package hisarkit

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

// readQuotedList returns the quoted strings of the first list that follows marker in a source file.
func readQuotedList(t *testing.T, path, marker, end string) []string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, after, isFound := strings.Cut(string(source), marker)
	if !isFound {
		t.Fatalf("%s has no %q", path, marker)
	}
	list, _, _ := strings.Cut(after, end)
	values := []string{}
	for _, match := range regexp.MustCompile(`"([a-z-]*)"`).FindAllStringSubmatch(list, -1) {
		values = append(values, match[1])
	}
	return values
}

// TestIconPresetsMatchTheApps keeps the pickers of the Mac and iPhone apps on core/item's preset icons and colors.
func TestIconPresetsMatchTheApps(t *testing.T) {
	for _, source := range []struct{ path, presets, colors, end string }{
		{"../ios/Shared/Backend/Models.swift", "static let allPreset = [", "static let allColor = [", "]"},
		{"../../desktop/frontend/src/util.ts", "export const allIconPreset = [", "export const allIconColor = [", "]"},
	} {
		if got := readQuotedList(t, source.path, source.presets, source.end); !reflect.DeepEqual(got, item.IconPresets()) {
			t.Errorf("%s presets = %v, want %v", source.path, got, item.IconPresets())
		}
		if got := readQuotedList(t, source.path, source.colors, source.end); !reflect.DeepEqual(got, item.IconColors()) {
			t.Errorf("%s colors = %v, want %v", source.path, got, item.IconColors())
		}
	}
}
