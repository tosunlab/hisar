package app

import (
	"path/filepath"
	"testing"
)

func TestThemeDefaultsAndIsKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	if theme := service.GetTheme(); theme != (Theme{Appearance: "system", Color: "neutral"}) {
		t.Errorf("default theme = %+v", theme)
	}
	var allAppliedTheme []Theme
	service.config.OnThemeChange = func(theme Theme) { allAppliedTheme = append(allAppliedTheme, theme) }
	if theme, err := service.SetTheme("dark", "green"); err != nil || theme != (Theme{Appearance: "dark", Color: "green"}) {
		t.Fatalf("SetTheme = %+v, %v", theme, err)
	}
	if _, err := service.SetTheme("sepia", "green"); err == nil {
		t.Error("an unknown appearance was accepted")
	}
	if _, err := service.SetTheme("light", "orange"); err == nil {
		t.Error("an unknown colour was accepted")
	}
	if len(allAppliedTheme) != 1 || allAppliedTheme[0].Color != "green" {
		t.Errorf("theme changes heard: %+v", allAppliedTheme)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	if theme := reloaded.GetTheme(); theme != (Theme{Appearance: "dark", Color: "green"}) {
		t.Errorf("theme after signing out and relaunching = %+v", theme)
	}
}

func TestThemeIgnoresUnknownSavedValues(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	service.state.Appearance, service.state.Color = "sepia", "orange"
	if theme := service.GetTheme(); theme != (Theme{Appearance: "system", Color: "neutral"}) {
		t.Errorf("theme from unknown saved values = %+v", theme)
	}
}
