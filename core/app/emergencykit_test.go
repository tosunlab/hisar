package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/emergencykit"
)

func TestEmergencyKitFileNameIdentifiesAccountServerAndExportTime(t *testing.T) {
	kit := emergencykit.Kit{
		Email: "alice@example.com", Server: "https://vault.example.com:8443/hisar/",
		CreatedAt: time.Date(2026, 10, 27, 15, 4, 5, 123, time.FixedZone("local", 3*60*60)),
		SecretKey: "never-include-this-secret",
	}
	want := "Hisar Emergency Kit - alice@example.com - vault.example.com-8443-hisar - 2026-10-27-120405.000000123.pdf"
	if got := emergencyKitFileName(kit); got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	for _, change := range []func(*emergencykit.Kit){
		func(kit *emergencykit.Kit) { kit.Email = "bob@example.com" },
		func(kit *emergencykit.Kit) { kit.Server = "https://another.example.com" },
		func(kit *emergencykit.Kit) { kit.CreatedAt = kit.CreatedAt.Add(time.Nanosecond) },
	} {
		other := kit
		change(&other)
		if emergencyKitFileName(other) == want {
			t.Error("another account, server or export time has the same filename")
		}
	}
}

func TestEmergencyKitFileNameIsABoundedBasenameWithoutSecrets(t *testing.T) {
	for _, value := range []string{"../../\\\x00\r\n:*?\"<>|", strings.Repeat("long", 300), "你好", " .@_- "} {
		kit := emergencykit.Kit{Email: value, Server: value, SecretKey: "H1-SECRET-NEVER-IN-FILENAME"}
		name := emergencyKitFileName(kit)
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n:*?\"<>|") || len(name) > 180 || !strings.HasSuffix(name, ".pdf") {
			t.Errorf("unsafe filename %q", name)
		}
		if strings.Contains(name, kit.SecretKey) {
			t.Errorf("filename contains the Secret Key: %q", name)
		}
	}
}

type kitSuggestionSaver struct {
	path       string
	suggestion string
}

func (saver *kitSuggestionSaver) ChooseSavePath(suggestion string) (string, error) {
	saver.suggestion = suggestion
	return saver.path, nil
}

func TestEmergencyKitUsesDescriptiveSuggestionsWithoutChangingChosenPaths(t *testing.T) {
	service := createUnlockedTestService(t).service
	kit := emergencykit.Kit{
		Name: "Alice", Email: "alice@example.com", Server: "https://vault.example.com",
		SecretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M", CreatedAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
	want := "Hisar Emergency Kit - alice@example.com - vault.example.com - 2026-10-27-120000.000000000.pdf"
	saver := &kitSuggestionSaver{path: filepath.Join(t.TempDir(), "my kit.pdf")}
	service.config.FileSaver = saver
	path, err := service.writeEmergencyKit(kit, service.sessionEpoch)
	if err != nil || path != saver.path || saver.suggestion != want {
		t.Fatalf("native save = %q, suggestion = %q, error = %v", path, saver.suggestion, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	export, err := service.saveEmergencyKitToDirectory(kit, directory, service.sessionEpoch)
	if err != nil || export.Path != filepath.Join(directory, want) {
		t.Fatalf("directory export = %+v, error = %v", export, err)
	}
}
