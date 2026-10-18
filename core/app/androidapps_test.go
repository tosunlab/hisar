package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

const (
	bankCertificate      = "14:6D:E9:83:C5:73:06:50:D8:EE:B9:95:2F:34:FC:64:16:A0:83:42:E6:1D:BE:A8:8A:04:96:B2:3F:CF:44:E5"
	bankCertificateLower = "146de983c5730650d8eeb9952f34fc6416a08342e61dbea88a0496b23fcf44e5"
	sideloadCertificate  = "0000000000000000000000000000000000000000000000000000000000000001"
)

func getBrowserErrorCode(err error) string {
	var responseError *browserprotocol.ResponseError
	if errors.As(err, &responseError) {
		return responseError.Code
	}
	return ""
}

func listLoginTitles(logins []browserprotocol.Login) []string {
	titles := []string{}
	for _, login := range logins {
		titles = append(titles, login.Title)
	}
	return titles
}

func TestAndroidAppFillsOnlyItsPackageWithItsCertificate(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service, personalID := fixture.service, fixture.personalID
	bank, err := service.SaveItem(personalID, ItemDetail{
		Title: "Bank", Username: "alice", Password: "bank-password", URLs: []string{"https://bank.example"},
		OneTimePassword: "otpauth://totp/Bank:alice?secret=JBSWY3DPEHPK3PXP",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(personalID, ItemDetail{Title: "Bank website", Username: "alice", Password: "web", URLs: []string{"https://com.bank.app"}}); err != nil {
		t.Fatal(err)
	}

	linked, err := service.AddAndroidApp(personalID, bank.ID, "com.bank.app", bankCertificateLower)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(linked.AndroidApps, []ItemAndroidApp{{PackageName: "com.bank.app", CertificateSHA256: bankCertificate}}) || !slices.Equal(linked.URLs, []string{"https://bank.example"}) {
		t.Fatalf("linked: apps %+v, websites %q", linked.AndroidApps, linked.URLs)
	}

	logins, err := ListLoginsForAndroidApp(service, "com.bank.app", bankCertificate)
	if err != nil || !slices.Equal(listLoginTitles(logins), []string{"Bank"}) {
		t.Fatalf("the linked app: %+v %v", logins, err)
	}
	if fill, err := GetLoginForAndroidApp(service, personalID, bank.ID, "com.bank.app", bankCertificate); err != nil || fill.Password != "bank-password" || !fill.HasOneTimePassword {
		t.Errorf("fill: %+v %v", fill, err)
	}
	if code, err := GetOneTimeCodeForAndroidApp(service, personalID, bank.ID, "com.bank.app", bankCertificate); err != nil || code.Code == "" {
		t.Errorf("one-time code: %+v %v", code, err)
	}

	for _, testCase := range []struct{ name, packageName, certificate string }{
		{"same package, another certificate", "com.bank.app", sideloadCertificate},
		{"longer package, same certificate", "com.bank.app.evil", bankCertificate},
		{"shorter package, same certificate", "com.bank", bankCertificate},
		{"package named after the website", "example.bank", bankCertificate},
	} {
		if logins, err := ListLoginsForAndroidApp(service, testCase.packageName, testCase.certificate); err != nil || len(logins) != 0 {
			t.Errorf("%s: listed %+v %v", testCase.name, logins, err)
		}
		if fill, err := GetLoginForAndroidApp(service, personalID, bank.ID, testCase.packageName, testCase.certificate); getBrowserErrorCode(err) != browserprotocol.ErrorNoMatch || fill.Password != "" {
			t.Errorf("%s: filled %+v %v", testCase.name, fill, err)
		}
		if code, err := GetOneTimeCodeForAndroidApp(service, personalID, bank.ID, testCase.packageName, testCase.certificate); getBrowserErrorCode(err) != browserprotocol.ErrorNoMatch || code.Code != "" {
			t.Errorf("%s: one-time code %+v %v", testCase.name, code, err)
		}
	}
	if _, err := ListLoginsForAndroidApp(service, "com.bank.app", ""); getBrowserErrorCode(err) != browserprotocol.ErrorInvalidRequest {
		t.Errorf("without a certificate: %v", err)
	}

	if logins, _ := ListLoginsForURL(service, "https://bank.example/login"); !slices.Equal(listLoginTitles(logins), []string{"Bank"}) {
		t.Errorf("the website: %+v", logins)
	}
	if logins, _ := ListLoginsForURL(service, "https://com.bank.app/"); !slices.Equal(listLoginTitles(logins), []string{"Bank website"}) {
		t.Errorf("a site named like the package: %+v", logins)
	}

	again, err := service.AddAndroidApp(personalID, bank.ID, "com.bank.app", bankCertificate)
	if err != nil || again.Revision != linked.Revision || len(again.AndroidApps) != 1 {
		t.Errorf("adding the link again: revision %q, want %q; apps %+v; %v", again.Revision, linked.Revision, again.AndroidApps, err)
	}
	unlinked, err := service.RemoveAndroidApp(personalID, bank.ID, "com.bank.app", bankCertificate)
	if err != nil || len(unlinked.AndroidApps) != 0 || !slices.Equal(unlinked.URLs, []string{"https://bank.example"}) {
		t.Fatalf("unlinked: %+v %v", unlinked, err)
	}
	if logins, _ := ListLoginsForAndroidApp(service, "com.bank.app", bankCertificate); len(logins) != 0 {
		t.Errorf("after unlinking: %+v", logins)
	}
}

func TestAddAndroidAppRefusesItemsThatDoNotFill(t *testing.T) {
	fixture := createUnlockedTestService(t)
	note, err := fixture.service.SaveItem(fixture.personalID, ItemDetail{Category: item.CategorySecureNote, Title: "Note", Notes: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.AddAndroidApp(fixture.personalID, note.ID, "com.bank.app", bankCertificate); err == nil {
		t.Error("a secure note was linked to an app")
	}
	if _, err := fixture.service.AddAndroidApp(fixture.personalID, note.ID, "bank", bankCertificate); !errors.Is(err, errInvalidAndroidApp) {
		t.Errorf("a package name without a dot: %v", err)
	}
}

func TestEditingALoginKeepsItsAndroidApps(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service, personalID := fixture.service, fixture.personalID
	bank, err := service.SaveItem(personalID, ItemDetail{Title: "Bank", Username: "alice", Password: "old", URLs: []string{"https://bank.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddAndroidApp(personalID, bank.ID, "com.bank.app", bankCertificate); err != nil {
		t.Fatal(err)
	}
	bankApp, _ := item.ParseAndroidApp("com.bank.app", bankCertificate)

	unaware, err := service.GetItem(personalID, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	unaware.AndroidApps = nil
	unaware.Password, unaware.URLs = "new", []string{"https://bank.example", "https://m.bank.example"}
	saved, err := service.SaveItem(personalID, unaware)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.AndroidApps) != 1 || !slices.Equal(saved.URLs, []string{"https://bank.example", "https://m.bank.example"}) {
		t.Errorf("after an edit by an editor that does not send apps: apps %+v, websites %q", saved.AndroidApps, saved.URLs)
	}
	if fill, err := GetLoginForAndroidApp(service, personalID, bank.ID, "com.bank.app", bankCertificate); err != nil || fill.Password != "new" {
		t.Errorf("fill after the edit: %+v %v", fill, err)
	}

	olderApp := saved
	olderApp.Title, olderApp.URLs = "My bank", append(slices.Clone(saved.URLs), bankApp.URL())
	if _, err := service.SaveItem(personalID, olderApp); err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, personalID, bank.ID)
	if want := []string{"https://bank.example", "https://m.bank.example", bankApp.URL()}; !slices.Equal(stored.Overview.URLs, want) {
		t.Errorf("after an edit that sends the link among the websites, as an older app does: %q, want %q", stored.Overview.URLs, want)
	}
}

func TestListLoginWebsites(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	for _, detail := range []ItemDetail{
		{Title: "Bank", URLs: []string{"https://login.bank.example.co.uk/sign-in", "androidapp://com.bank.app?sha256=" + bankCertificateLower}},
		{Title: "Bank again", URLs: []string{"login.bank.example.co.uk"}},
		{Title: "Router", URLs: []string{"http://192.168.1.1", "https://router.local", "https://example.org:8443"}},
		{Title: "Shop", URLs: []string{"http://shop.example.com"}},
		{Category: item.CategorySecureNote, Title: "Note", URLs: []string{"https://note.example"}, Notes: "text"},
	} {
		if _, err := service.SaveItem(fixture.personalID, detail); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.SaveItem(fixture.familyID, ItemDetail{Title: "Family bank", URLs: []string{"https://bank.example.co.uk"}}); err != nil {
		t.Fatal(err)
	}
	websites, err := ListLoginWebsites(service)
	want := []LoginWebsite{
		{Host: "bank.example.co.uk", RegistrableDomain: "example.co.uk"},
		{Host: "login.bank.example.co.uk", RegistrableDomain: "example.co.uk"},
		{Host: "shop.example.com", RegistrableDomain: "example.com"},
	}
	if err != nil || !slices.Equal(websites, want) {
		t.Errorf("websites %+v %v, want %+v", websites, err, want)
	}
}
