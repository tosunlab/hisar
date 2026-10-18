package commandline

import (
	"testing"

	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	testRaycastPath   = "/Applications/Raycast.app"
	testRaycastTeamID = "A1B2C3D4E5"
)

func createRaycastRequester(signature appinfo.Signature) appinfo.Requester {
	return appinfo.Requester{
		Kind: appinfo.RequesterKindApp,
		Name: "Raycast",
		App:  appinfo.App{Name: "Raycast", Path: testRaycastPath, Signature: signature},
	}
}

func TestIdentifyAppNeedsAVerifiedSignature(t *testing.T) {
	terminal := appinfo.Requester{
		Kind: appinfo.RequesterKindApp,
		Name: "Terminal",
		App:  appinfo.App{Name: "Terminal", Path: "/System/Applications/Utilities/Terminal.app", Signature: appinfo.Signature{Status: appinfo.SignatureStatusApple}},
	}
	for _, testCase := range []struct {
		name         string
		requester    appinfo.Requester
		wantIdentity clisocket.AppIdentity
		wantIsFound  bool
	}{
		{
			name:         "signed by a developer",
			requester:    createRaycastRequester(appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: testRaycastTeamID, SignerName: "Raycast Technologies Inc"}),
			wantIdentity: clisocket.AppIdentity{Path: testRaycastPath, TeamID: testRaycastTeamID},
			wantIsFound:  true,
		},
		{
			name:         "signed by Apple",
			requester:    terminal,
			wantIdentity: clisocket.AppIdentity{Path: "/System/Applications/Utilities/Terminal.app"},
			wantIsFound:  true,
		},
		{name: "not signed", requester: createRaycastRequester(appinfo.Signature{Status: appinfo.SignatureStatusUnsigned})},
		{name: "signature does not verify", requester: createRaycastRequester(appinfo.Signature{Status: appinfo.SignatureStatusInvalid, TeamID: testRaycastTeamID})},
		{name: "signature not checked", requester: createRaycastRequester(appinfo.Signature{})},
		{name: "background process", requester: appinfo.Requester{Kind: appinfo.RequesterKindBackground, Name: "A background process"}},
		{name: "SSH session", requester: appinfo.Requester{Kind: appinfo.RequesterKindSSHSession, Name: "An SSH session from 192.0.2.10"}},
		{name: "process without an app", requester: appinfo.Requester{Kind: appinfo.RequesterKindProcess, Name: "A process without an app"}},
	} {
		identity, isFound := IdentifyApp(testCase.requester)
		if identity != testCase.wantIdentity || isFound != testCase.wantIsFound {
			t.Errorf("%s: %+v %v, want %+v %v", testCase.name, identity, isFound, testCase.wantIdentity, testCase.wantIsFound)
		}
	}
}
