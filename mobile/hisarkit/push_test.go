package hisarkit

import (
	"encoding/json"
	"testing"
)

func TestPushSettingsAreJSONWithEveryKindOnAtFirst(t *testing.T) {
	hisar, err := New(t.TempDir(), "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := hisar.GetPushSettings()
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		IsAvailable    bool     `json:"isAvailable"`
		AllEnabledKind []string `json:"enabledKinds"`
	}
	if err := json.Unmarshal([]byte(text), &settings); err != nil || settings.IsAvailable || len(settings.AllEnabledKind) != 3 {
		t.Fatalf("settings = %s %v", text, err)
	}
	if err := hisar.SetPushToken("apns", "token"); err != nil {
		t.Fatalf("a token while signed out: %v", err)
	}
	if err := hisar.SetPushKindEnabled("marketing", true); err == nil {
		t.Fatal("an unknown kind was taken")
	}
}
