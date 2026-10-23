package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestClientSendsPlatformAndVersion(t *testing.T) {
	var platforms, versions []string
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		platforms = append(platforms, request.Header.Get(apitypes.PlatformHeader))
		versions = append(versions, request.Header.Get(apitypes.VersionHeader))
		if request.URL.Path == "/api/v1/events" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	if err := fakeClient.CheckVersion(context.Background()); err != nil {
		t.Fatal(err)
	}
	fakeClient.Platform, fakeClient.Version = apitypes.PlatformDesktopMacOS, "0.1.6"
	if err := fakeClient.CheckVersion(context.Background()); err != nil {
		t.Fatal(err)
	}
	fakeClient.StreamEvents(context.Background(), nil, func(Event) {})
	if platforms[0] != "" || versions[0] != "" {
		t.Errorf("headers without a version: %q %q", platforms[0], versions[0])
	}
	for index := 1; index < 3; index++ {
		if platforms[index] != "darwin/desktop" || versions[index] != "0.1.6" {
			t.Errorf("request %d sent %q %q", index, platforms[index], versions[index])
		}
	}
}

func TestUpgradeRequired(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUpgradeRequired)
		json.NewEncoder(writer).Encode(apitypes.UpgradeRequired{Error: "update required", MinimumVersion: "0.1.6", UpdateURL: "https://tryhisar.com/download"})
	})
	fakeClient.Platform, fakeClient.Version = "linux/cli", "0.1.5"
	var heards []*UpgradeRequiredError
	fakeClient.OnUpgradeRequired = func(upgradeRequired *UpgradeRequiredError) { heards = append(heards, upgradeRequired) }

	_, err := fakeClient.GetAccount(context.Background())
	upgradeRequired, ok := AsUpgradeRequired(err)
	if !ok || upgradeRequired.MinimumVersion != "0.1.6" || upgradeRequired.UpdateURL != "https://tryhisar.com/download" {
		t.Fatalf("GetAccount: %v", err)
	}
	if IsUnreachable(err) {
		t.Error("426 counted as unreachable, which would hide it behind the offline cache")
	}
	streamErr := fakeClient.StreamEvents(context.Background(), nil, func(Event) {})
	if _, ok := AsUpgradeRequired(streamErr); !ok {
		t.Errorf("event stream: %v", streamErr)
	}
	if len(heards) != 2 || heards[0].MinimumVersion != "0.1.6" {
		t.Errorf("OnUpgradeRequired heard %d", len(heards))
	}
	if _, ok := AsUpgradeRequired(errors.New("other")); ok {
		t.Error("another error read as upgrade required")
	}
	if err.Error() != "update required: the server needs version 0.1.6 or later" {
		t.Errorf("message %q", err.Error())
	}
}

func TestGetClient(t *testing.T) {
	for platform, expected := range map[string]string{
		"darwin/desktop":    apitypes.ClientDesktopMacOS,
		"ios/mobile":        apitypes.ClientIOS,
		"android/mobile":    apitypes.ClientAndroid,
		"darwin/cli":        apitypes.ClientCLI,
		"linux/cli":         apitypes.ClientCLI,
		"windows/cli":       apitypes.ClientCLI,
		"browser/extension": apitypes.ClientExtension,
		"linux/desktop":     "",
		"/cli":              "",
		"go":                "",
		"":                  "",
	} {
		if client := apitypes.GetClient(platform); client != expected {
			t.Errorf("GetClient(%q) = %q, want %q", platform, client, expected)
		}
	}
}

func TestIsPhonePlatform(t *testing.T) {
	for platform, expected := range map[string]bool{
		"ios/mobile":        true,
		"android/mobile":    true,
		"darwin/desktop":    false,
		"linux/cli":         false,
		"browser/extension": false,
		"/mobile":           false,
		"":                  false,
	} {
		if isPhone := apitypes.IsPhonePlatform(platform); isPhone != expected {
			t.Errorf("IsPhonePlatform(%q) = %v, want %v", platform, isPhone, expected)
		}
	}
}
