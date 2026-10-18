package commandline

import (
	"errors"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	testRaycastPath   = "/Applications/Raycast.app"
	testRaycastTeamID = "A1B2C3D4E5"
)

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
		resolver := appinfo.NewResolver(appinfo.System{GetStrictSignature: func(string) appinfo.Signature { return testCase.requester.App.Signature }})
		identity, isFound := IdentifyApp(testCase.requester, resolver)
		if identity != testCase.wantIdentity || isFound != testCase.wantIsFound {
			t.Errorf("%s: %+v %v, want %+v %v", testCase.name, identity, isFound, testCase.wantIdentity, testCase.wantIsFound)
		}
	}
}

func TestIdentifyAppRequiresIntactResources(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		signature    appinfo.Signature
		wantIdentity clisocket.AppIdentity
	}{
		{name: "broken resource seal", signature: appinfo.Signature{Status: appinfo.SignatureStatusInvalid, TeamID: testRaycastTeamID}},
		{name: "not checked"},
		{name: "intact developer bundle", signature: appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: testRaycastTeamID}, wantIdentity: clisocket.AppIdentity{Path: testRaycastPath, TeamID: testRaycastTeamID}},
		{name: "intact Apple bundle", signature: appinfo.Signature{Status: appinfo.SignatureStatusApple}, wantIdentity: clisocket.AppIdentity{Path: testRaycastPath}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resolver := appinfo.NewResolver(appinfo.System{
				GetSignature: func(string) appinfo.Signature {
					return appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: "OLDTEAM"}
				},
				GetStrictSignature: func(string) appinfo.Signature { return testCase.signature },
			})
			requester := appinfo.Requester{Kind: appinfo.RequesterKindApp, App: resolver.DescribeApp(testRaycastPath)}
			identity, isFound := IdentifyApp(requester, resolver)
			if identity != testCase.wantIdentity || isFound != (testCase.wantIdentity.Path != "") {
				t.Errorf("identity %+v, found %v; want %+v", identity, isFound, testCase.wantIdentity)
			}
			if requester.App.Signature.Status != appinfo.SignatureStatusDeveloper {
				t.Errorf("display signature %+v", requester.App.Signature)
			}
		})
	}
}

func TestIdentifyAppRechecksChangedBundles(t *testing.T) {
	modificationTime := time.Unix(1, 0)
	signature := appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: testRaycastTeamID}
	resolver := appinfo.NewResolver(appinfo.System{
		GetStrictSignature:        func(string) appinfo.Signature { return signature },
		GetBundleModificationTime: func(string) (time.Time, error) { return modificationTime, nil },
	})
	requester := createRaycastRequester(signature)
	wantIdentity := clisocket.AppIdentity{Path: testRaycastPath, TeamID: testRaycastTeamID}
	for range 2 {
		if identity, isFound := IdentifyApp(requester, resolver); !isFound || identity != wantIdentity {
			t.Fatalf("intact bundle: %+v, %v", identity, isFound)
		}
	}
	signature.Status = appinfo.SignatureStatusInvalid
	if identity, isFound := IdentifyApp(requester, resolver); !isFound || identity != wantIdentity {
		t.Fatalf("unchanged bundle lost its cached identity: %+v, %v", identity, isFound)
	}
	modificationTime = modificationTime.Add(time.Second)
	if identity, isFound := IdentifyApp(requester, resolver); isFound || identity != (clisocket.AppIdentity{}) {
		t.Fatalf("damaged bundle kept its identity: %+v, %v", identity, isFound)
	}
	signature.Status = appinfo.SignatureStatusDeveloper
	modificationTime = modificationTime.Add(time.Second)
	if identity, isFound := IdentifyApp(requester, resolver); !isFound || identity != wantIdentity {
		t.Fatalf("repaired bundle: %+v, %v", identity, isFound)
	}
}

func TestIdentifyAppDoesNotCacheAnUnreadableBundle(t *testing.T) {
	signature := appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: testRaycastTeamID}
	resolver := appinfo.NewResolver(appinfo.System{
		GetStrictSignature:        func(string) appinfo.Signature { return signature },
		GetBundleModificationTime: func(string) (time.Time, error) { return time.Time{}, errors.New("appinfo: bundle unavailable") },
	})
	requester := createRaycastRequester(signature)
	if _, isFound := IdentifyApp(requester, resolver); !isFound {
		t.Fatal("intact bundle has no identity")
	}
	signature.Status = appinfo.SignatureStatusInvalid
	if identity, isFound := IdentifyApp(requester, resolver); isFound || identity != (clisocket.AppIdentity{}) {
		t.Fatalf("damaged bundle kept its identity: %+v, %v", identity, isFound)
	}
}
