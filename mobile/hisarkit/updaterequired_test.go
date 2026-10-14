package hisarkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

type updateRequiredRecorder struct {
	allUpdateRequired chan struct{}
}

func (recorder *updateRequiredRecorder) OnLocked() {}

func (recorder *updateRequiredRecorder) OnApprovalEvent(eventName, requestID string) {}

func (recorder *updateRequiredRecorder) OnRemoteChange() {}

func (recorder *updateRequiredRecorder) OnUpdateRequired() { recorder.allUpdateRequired <- struct{}{} }

func TestUpdateRequiredReachesTheApp(t *testing.T) {
	allSentHeader := make(chan http.Header, 4)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		allSentHeader <- request.Header.Clone()
		writer.WriteHeader(http.StatusUpgradeRequired)
		json.NewEncoder(writer).Encode(apitypes.UpgradeRequired{Error: "update required", MinimumVersion: "0.2.0", UpdateURL: "https://tryhisar.com/download"})
	}))
	defer httpServer.Close()
	stateDirectory := t.TempDir()
	os.WriteFile(filepath.Join(stateDirectory, "hisar.json"), []byte(`{"server":"`+httpServer.URL+`"}`), 0o600)
	recorder := &updateRequiredRecorder{allUpdateRequired: make(chan struct{}, 4)}
	hisar, err := New(stateDirectory, "Test iPhone", createMemorySecretKeyStore(), nil, nil, recorder)
	if err != nil {
		t.Fatal(err)
	}
	hisar.SetAppVersion("0.1.0")

	var status struct {
		UpdateRequired *struct {
			CurrentVersion string `json:"currentVersion"`
			MinimumVersion string `json:"minimumVersion"`
			UpdateURL      string `json:"updateURL"`
		} `json:"updateRequired"`
	}
	if err := json.Unmarshal([]byte(hisar.CheckMinimumVersion()), &status); err != nil {
		t.Fatal(err)
	}
	if status.UpdateRequired == nil || status.UpdateRequired.CurrentVersion != "0.1.0" || status.UpdateRequired.MinimumVersion != "0.2.0" || status.UpdateRequired.UpdateURL != "https://tryhisar.com/download" {
		t.Fatalf("status: %+v", status.UpdateRequired)
	}
	if header := <-allSentHeader; header.Get(apitypes.PlatformHeader) != "ios/mobile" || header.Get(apitypes.VersionHeader) != "0.1.0" {
		t.Errorf("sent %q %q", header.Get(apitypes.PlatformHeader), header.Get(apitypes.VersionHeader))
	}
	select {
	case <-recorder.allUpdateRequired:
	case <-time.After(5 * time.Second):
		t.Error("OnUpdateRequired was not called")
	}
}
