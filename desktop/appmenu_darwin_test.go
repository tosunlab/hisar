package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestApplicationMenuSettingsAccelerator(t *testing.T) {
	application.New(application.Options{Name: "Hisar"})
	menu := createApplicationMenu(func() {}, func() {})
	settings := menu.FindByLabel("Settings…")
	if settings == nil || settings.GetAccelerator() != "Cmd+," {
		t.Fatalf("Settings command = %v", settings)
	}
	quit := menu.FindByLabel("Quit Hisar")
	if quit == nil || quit.GetAccelerator() != "Cmd+Q" {
		t.Fatalf("background-running Quit command = %v", quit)
	}
}

func TestSettingsMenuRestoresLastSectionAndUnlocksFirst(t *testing.T) {
	for _, unlocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "locked", true: "unlocked"}[unlocked], func(t *testing.T) {
			requested := "not requested"
			mainShown := false
			openSettingsFromMenu(func(section string) bool {
				requested = section
				return true
			}, unlocked, func() { mainShown = true })
			if requested != "" {
				t.Fatalf("requested section = %q, want last section", requested)
			}
			if mainShown == unlocked {
				t.Fatalf("unlock window shown = %v, unlocked = %v", mainShown, unlocked)
			}
		})
	}
}
