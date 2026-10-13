package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/proto"
)

func TestClientSendsPlatformAndVersion(t *testing.T) {
	var allPlatform, allVersion []string
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		allPlatform = append(allPlatform, request.Header.Get(proto.PlatformHeader))
		allVersion = append(allVersion, request.Header.Get(proto.VersionHeader))
		if request.URL.Path == "/api/v1/events" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	if err := fakeClient.CheckVersion(context.Background()); err != nil {
		t.Fatal(err)
	}
	fakeClient.Platform, fakeClient.Version = proto.PlatformDesktopMacOS, "0.1.6"
	if err := fakeClient.CheckVersion(context.Background()); err != nil {
		t.Fatal(err)
	}
	fakeClient.StreamEvents(context.Background(), func(Event) {})
	if allPlatform[0] != "" || allVersion[0] != "" {
		t.Errorf("headers without a version: %q %q", allPlatform[0], allVersion[0])
	}
	for index := 1; index < 3; index++ {
		if allPlatform[index] != "darwin/desktop" || allVersion[index] != "0.1.6" {
			t.Errorf("request %d sent %q %q", index, allPlatform[index], allVersion[index])
		}
	}
}

func TestUpgradeRequired(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUpgradeRequired)
		json.NewEncoder(writer).Encode(proto.UpgradeRequired{Error: "update required", MinimumVersion: "0.1.6", UpdateURL: "https://tryhisar.com/download"})
	})
	fakeClient.Platform, fakeClient.Version = "linux/cli", "0.1.5"
	var allHeard []*UpgradeRequiredError
	fakeClient.OnUpgradeRequired = func(upgradeRequired *UpgradeRequiredError) { allHeard = append(allHeard, upgradeRequired) }

	_, err := fakeClient.GetAccount(context.Background())
	upgradeRequired, ok := AsUpgradeRequired(err)
	if !ok || upgradeRequired.MinimumVersion != "0.1.6" || upgradeRequired.UpdateURL != "https://tryhisar.com/download" {
		t.Fatalf("GetAccount: %v", err)
	}
	if IsUnreachable(err) {
		t.Error("426 counted as unreachable, which would hide it behind the offline cache")
	}
	streamErr := fakeClient.StreamEvents(context.Background(), func(Event) {})
	if _, ok := AsUpgradeRequired(streamErr); !ok {
		t.Errorf("event stream: %v", streamErr)
	}
	if len(allHeard) != 2 || allHeard[0].MinimumVersion != "0.1.6" {
		t.Errorf("OnUpgradeRequired heard %d", len(allHeard))
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
		"darwin/desktop":    proto.ClientDesktopMacOS,
		"ios/mobile":        proto.ClientIOS,
		"darwin/cli":        proto.ClientCLI,
		"linux/cli":         proto.ClientCLI,
		"windows/cli":       proto.ClientCLI,
		"browser/extension": proto.ClientExtension,
		"linux/desktop":     "",
		"/cli":              "",
		"go":                "",
		"":                  "",
	} {
		if client := proto.GetClient(platform); client != expected {
			t.Errorf("GetClient(%q) = %q, want %q", platform, client, expected)
		}
	}
}
