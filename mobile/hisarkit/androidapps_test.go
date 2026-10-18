package hisarkit

import (
	"slices"
	"strings"
	"testing"
)

const (
	testAppCertificate     = "14:6D:E9:83:C5:73:06:50:D8:EE:B9:95:2F:34:FC:64:16:A0:83:42:E6:1D:BE:A8:8A:04:96:B2:3F:CF:44:E5"
	sideloadAppCertificate = "0000000000000000000000000000000000000000000000000000000000000001"
)

type androidAppItem struct {
	URLs        []string `json:"urls"`
	AndroidApps []struct {
		PackageName       string `json:"packageName"`
		CertificateSHA256 string `json:"certificateSha256"`
	} `json:"androidApps"`
}

func TestAndroidAppArguments(t *testing.T) {
	hisar, err := New(t.TempDir(), "Pixel", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hisar.ListAllLoginForAndroidApp("com.github.android", "not hex"); err == nil || !strings.HasPrefix(err.Error(), "invalid-request:") {
		t.Errorf("a broken certificate: %v", err)
	}
	if _, err := hisar.ListAllLoginForAndroidApp("com.github.android", testAppCertificate); err == nil || !strings.HasPrefix(err.Error(), "signed-out:") {
		t.Errorf("signed out: %v", err)
	}
	if _, err := hisar.AddAndroidApp("vault", "item", "com.github.android", testAppCertificate); err == nil {
		t.Error("linked while signed out")
	}
}

// checkAndroidAppFlow runs inside TestIntegrationMobileFlow against the real server; itemID is the GitHub login in the
// Personal vault, saved for https://github.com.
func checkAndroidAppFlow(t *testing.T, hisar *Hisar, personalID, itemID string) {
	t.Helper()
	type login struct {
		ItemID string `json:"itemID"`
	}
	if logins := decodeJSON[[]login](t)(hisar.ListAllLoginForAndroidApp("com.github.android", testAppCertificate)); len(logins) != 0 {
		t.Fatalf("logins before linking: %+v", logins)
	}
	linked := decodeJSON[androidAppItem](t)(hisar.AddAndroidApp(personalID, itemID, "com.github.android", testAppCertificate))
	if len(linked.AndroidApps) != 1 || linked.AndroidApps[0].PackageName != "com.github.android" || linked.AndroidApps[0].CertificateSHA256 != testAppCertificate ||
		len(linked.URLs) != 1 || linked.URLs[0] != "https://github.com" {
		t.Fatalf("linked item: %+v", linked)
	}
	if logins := decodeJSON[[]login](t)(hisar.ListAllLoginForAndroidApp("com.github.android", strings.ToLower(strings.ReplaceAll(testAppCertificate, ":", "")))); len(logins) != 1 || logins[0].ItemID != itemID {
		t.Errorf("logins for the linked app: %+v", logins)
	}
	if logins := decodeJSON[[]login](t)(hisar.ListAllLoginForAndroidApp("com.github.android", sideloadAppCertificate)); len(logins) != 0 {
		t.Errorf("logins for a sideloaded app with the same package: %+v", logins)
	}
	fill := decodeJSON[struct {
		Password string `json:"password"`
	}](t)(hisar.GetLoginForAndroidApp(personalID, itemID, "com.github.android", testAppCertificate))
	if fill.Password != "mobile-secret" {
		t.Errorf("fill: %+v", fill)
	}
	if _, err := hisar.GetLoginForAndroidApp(personalID, itemID, "com.github.android", sideloadAppCertificate); err == nil || !strings.HasPrefix(err.Error(), "no-match:") {
		t.Errorf("filled a sideloaded app: %v", err)
	}
	if code := decodeJSON[struct {
		Code string `json:"code"`
	}](t)(hisar.GetOneTimeCodeForAndroidApp(personalID, itemID, "com.github.android", testAppCertificate)); len(code.Code) != 6 {
		t.Errorf("one-time code: %+v", code)
	}
	type website struct {
		Host              string `json:"host"`
		RegistrableDomain string `json:"registrableDomain"`
	}
	if websites := decodeJSON[[]website](t)(hisar.ListAllLoginWebsite()); !slices.Contains(websites, website{Host: "github.com", RegistrableDomain: "github.com"}) {
		t.Errorf("websites: %+v", websites)
	}
	unlinked := decodeJSON[androidAppItem](t)(hisar.RemoveAndroidApp(personalID, itemID, "com.github.android", testAppCertificate))
	if len(unlinked.AndroidApps) != 0 {
		t.Errorf("unlinked item: %+v", unlinked)
	}
}
