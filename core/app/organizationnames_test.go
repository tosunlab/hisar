package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
)

type organizationNamesTransport struct {
	list   apitypes.OrganizationList
	status int
}

func (transport *organizationNamesTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/api/v1/organizations" {
		return http.DefaultTransport.RoundTrip(request)
	}
	if transport.status == 0 {
		return nil, errors.New("server unreachable")
	}
	data, err := json.Marshal(transport.list)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: transport.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: request}, nil
}

func TestOrganizationNamesSurviveOfflineRestartAndReconcileOnReconnect(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	prepareTestApprovalUnlock(t, fixture, testMasterPassword)
	service.config.IsEventStreamDisabled = true
	service.Lock()
	if _, err := service.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	transport := &organizationNamesTransport{status: http.StatusOK, list: apitypes.OrganizationList{
		Organizations: []apitypes.OrganizationSummary{{ID: "acme", Name: "Acme", Role: "admin", MemberCount: 2}, {ID: "globex", Name: "Globex", Role: "member", MemberCount: 3}},
		Invitations:   []apitypes.ReceivedOrganizationInvitation{{ID: "invitation", OrganizationName: "Never cache an invitation"}},
	}}
	service.unlockedSession.Client.HTTP = &http.Client{Transport: transport}
	live, err := service.ListOrganizations()
	if err != nil || live.IsCached || len(live.Organizations) != 2 || len(live.Invitations) != 1 {
		t.Fatalf("live organizations: %+v %v", live, err)
	}
	sealed, err := service.localCache.LoadOrganizationNames(fixture.accountID)
	if err != nil || len(sealed) == 0 || bytes.Contains(sealed, []byte("Globex")) {
		t.Fatalf("names not encrypted: %x %v", sealed, err)
	}
	service.Lock()
	service.localCache.Close()
	service.localCache = nil
	fixture.apiServer.setUnreachable(true)
	transport.status = 0

	reopened, _, _ := createTestService(t, service.config.StatePath, service.config.SecretKeyStore)
	reopened.config.IsEventStreamDisabled = true
	reopened.serverTransport = transport
	t.Cleanup(func() { reopened.Lock() })
	if _, err := reopened.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	cached, err := reopened.ListOrganizations()
	want := []OrganizationSummary{{ID: "acme", Name: "Acme"}, {ID: "globex", Name: "Globex"}}
	if err != nil || !cached.IsCached || !reflect.DeepEqual(cached.Organizations, want) || len(cached.Invitations) != 0 {
		t.Fatalf("offline names contain live authority or lost a name: %+v %v", cached, err)
	}
	if _, err := reopened.CreateOrganization("Offline mutation"); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline mutation was permitted: %v", err)
	}
	transport.status = http.StatusForbidden
	if _, err := reopened.ListOrganizations(); err == nil || strings.Contains(err.Error(), "Acme") {
		t.Fatalf("authorization refusal used cached summaries: %v", err)
	}

	fixture.apiServer.setUnreachable(false)
	transport.status = http.StatusOK
	transport.list = apitypes.OrganizationList{Organizations: []apitypes.OrganizationSummary{{ID: "acme", Name: "Renamed Acme", Role: "member", MemberCount: 1}}}
	live, err = reopened.ListOrganizations()
	if err != nil || live.IsCached || len(live.Organizations) != 1 || live.Organizations[0].Name != "Renamed Acme" {
		t.Fatalf("reconnected names: %+v %v", live, err)
	}
	transport.status = 0
	cached, err = reopened.ListOrganizations()
	if err != nil || len(cached.Organizations) != 1 || cached.Organizations[0].Name != "Renamed Acme" || cached.Organizations[0].Role != "" {
		t.Fatalf("renames or removed memberships were not reconciled: %+v %v", cached, err)
	}
	if err := reopened.localCache.CheckAccount("another-account"); !errors.Is(err, cache.ErrAccountMismatch) {
		t.Fatalf("another account accepted these names: %v", err)
	}
	other := createUnlockedTestService(t)
	other.service.config.CachePath = reopened.config.CachePath
	other.service.unlockedSession.Client.HTTP = &http.Client{Transport: transport}
	if names, err := other.service.ListOrganizations(); !errors.Is(err, ErrOffline) || len(names.Organizations) != 0 {
		t.Fatalf("account switch showed the previous account's names: %+v %v", names, err)
	}
	if _, err := reopened.SignOut(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(reopened.config.CachePath); !os.IsNotExist(err) {
		t.Fatalf("forgotten account retained metadata: %v", err)
	}
}
