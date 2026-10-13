package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/proto"
)

func setVersion(t *testing.T, testVersion string) {
	t.Helper()
	previousVersion := version
	version = testVersion
	t.Cleanup(func() { version = previousVersion })
}

func TestClientVersionIsSentOnlyForReleases(t *testing.T) {
	var allHeader []http.Header
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		allHeader = append(allHeader, request.Header.Clone())
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer httpServer.Close()
	setVersion(t, "development")
	createClient(httpServer.URL).CheckVersion(context.Background())
	setVersion(t, "1.2.3")
	createClient(httpServer.URL).CheckVersion(context.Background())
	if allHeader[0].Get(proto.VersionHeader) != "" {
		t.Errorf("a development build sent version %q", allHeader[0].Get(proto.VersionHeader))
	}
	if allHeader[1].Get(proto.PlatformHeader) != runtime.GOOS+"/cli" || allHeader[1].Get(proto.VersionHeader) != "1.2.3" {
		t.Errorf("a release sent %q %q", allHeader[1].Get(proto.PlatformHeader), allHeader[1].Get(proto.VersionHeader))
	}
	if getDeviceInfo().Platform != runtime.GOOS+"/cli" {
		t.Errorf("device platform %q", getDeviceInfo().Platform)
	}
}

func TestDescribeUpdateRequired(t *testing.T) {
	setVersion(t, "0.9.0")
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUpgradeRequired)
		json.NewEncoder(writer).Encode(proto.UpgradeRequired{Error: "update required", MinimumVersion: "1.0.0", UpdateURL: "https://tryhisar.com/download"})
	}))
	defer httpServer.Close()
	err := createClient(httpServer.URL).CheckVersion(context.Background())
	message, isUpdateRequired := describeUpdateRequired(err)
	if !isUpdateRequired || message != "update required: the Hisar server needs hisar 1.0.0 or later, and this is 0.9.0.\n"+
		"Download the latest hisar from https://tryhisar.com/download, or on a Mac install the one that comes with the Hisar app (Settings → Developer tools)." {
		t.Errorf("server refusal: %q", message)
	}

	appError := cliprotocol.CreateError(cliprotocol.ErrorUpdateRequired, "Hisar needs an update: the server requires version 0.1.6 or later and this Mac has 0.1.5. Open Hisar and choose Update now.")
	if message, isUpdateRequired := describeUpdateRequired(appError); !isUpdateRequired || message != "update required. "+appError.Message {
		t.Errorf("app refusal: %q", message)
	}
	for _, other := range []error{nil, errors.New("other"), cliprotocol.CreateError(cliprotocol.ErrorLocked, "locked")} {
		if _, isUpdateRequired := describeUpdateRequired(other); isUpdateRequired {
			t.Errorf("%v read as update required", other)
		}
	}
}
