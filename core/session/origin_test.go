package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

func TestLegacyContextRelaysButBoundContextDoesNot(t *testing.T) {
	fixture := createCredentialTestAccount(t, []byte("relay comparison password"))
	ctx := context.Background()
	intended := fixture.session.Client
	target, err := url.Parse(intended.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var finishes atomic.Int32
	phishing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login/v2/finish" {
			finishes.Add(1)
		}
		proxy.ServeHTTP(w, r)
	}))
	defer phishing.Close()
	client := apiclient.New(phishing.URL)
	derived, err := crypt.DeriveKeys(fixture.password, fixture.server.account.Salt, fixture.server.account.Argon2, fixture.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer derived.Wipe()

	fixture.server.mutex.Lock()
	boundServer := fixture.server.loginServer
	fixture.server.loginServer = fixture.server.opaque
	fixture.server.mutex.Unlock()
	legacy, err := pake.NewConfig().Client()
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.ClearState()
	ke1, err := legacy.GenerateKE1(derived.AuthKey)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.StartLogin(ctx, &apitypes.LoginStartRequest{Email: fixture.session.Email, KE1: ke1.Serialize()})
	if err != nil {
		t.Fatal(err)
	}
	ke2, err := legacy.Deserialize.KE2(started.KE2)
	if err != nil {
		t.Fatal(err)
	}
	ke3, _, _, err := legacy.GenerateKE3(ke2, []byte(fixture.session.AccountID), pake.ServerIdentity())
	if err != nil {
		t.Fatal(err)
	}
	issued, err := client.FinishLogin(ctx, &apitypes.LoginFinishRequest{LoginID: started.LoginID, KE3: ke3.Serialize(), Device: apitypes.DeviceInfo{Name: "relay probe", Platform: "test"}})
	if err != nil || issued.Token == "" {
		t.Fatalf("legacy relay did not reproduce bearer issuance: %v", err)
	}
	if finishes.Load() != 1 {
		t.Fatal("legacy probe did not traverse phishing origin")
	}

	fixture.server.mutex.Lock()
	fixture.server.loginServer = boundServer
	fixture.server.mutex.Unlock()
	finishes.Store(0)
	if _, err := LogIn(ctx, client, fixture.session.Email, fixture.password, fixture.secretKey, apitypes.DeviceInfo{Name: "bound probe", Platform: "test"}); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("bound relay: %v", err)
	}
	if finishes.Load() != 0 {
		t.Fatal("origin mismatch reached login finish")
	}
	signedIn, err := LogIn(ctx, apiclient.New(intended.BaseURL), fixture.session.Email, fixture.password, fixture.secretKey, apitypes.DeviceInfo{Name: "intended", Platform: "test"})
	if err != nil {
		t.Fatalf("intended-origin login after upgrade: %v", err)
	}
	signedIn.Lock()
}
