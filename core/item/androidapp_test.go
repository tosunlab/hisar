package item

import (
	"errors"
	"slices"
	"testing"
	"time"
)

const (
	bankCertificate      = "14:6D:E9:83:C5:73:06:50:D8:EE:B9:95:2F:34:FC:64:16:A0:83:42:E6:1D:BE:A8:8A:04:96:B2:3F:CF:44:E5"
	bankCertificateLower = "146de983c5730650d8eeb9952f34fc6416a08342e61dbea88a0496b23fcf44e5"
)

func TestParseAndroidApp(t *testing.T) {
	colons, err := ParseAndroidApp("com.bank.app", bankCertificate)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := ParseAndroidApp("com.bank.app", bankCertificateLower)
	if err != nil || plain != colons {
		t.Errorf("hex without colons: %+v %v, want %+v", plain, err, colons)
	}
	if colons.CertificateFingerprint() != bankCertificate {
		t.Errorf("fingerprint %q", colons.CertificateFingerprint())
	}
	if colons.URL() != "androidapp://com.bank.app?sha256="+bankCertificateLower {
		t.Errorf("URL %q", colons.URL())
	}
	for _, testCase := range []struct{ name, packageName, certificate string }{
		{"one segment", "bank", bankCertificate},
		{"segment starting with a digit", "com.1bank", bankCertificate},
		{"host-like name", "bank.example.com:443", bankCertificate},
		{"no certificate", "com.bank.app", ""},
		{"SHA-1 certificate", "com.bank.app", "14:6D:E9:83:C5:73:06:50:D8:EE:B9:95:2F:34:FC:64:16:A0:83:42"},
		{"not hex", "com.bank.app", "zz" + bankCertificateLower[2:]},
	} {
		if _, err := ParseAndroidApp(testCase.packageName, testCase.certificate); !errors.Is(err, ErrInvalidAndroidApp) {
			t.Errorf("%s: %v", testCase.name, err)
		}
	}
}

func TestAndroidAppsAndWebsitesSplitTheURLs(t *testing.T) {
	bank, _ := ParseAndroidApp("com.bank.app", bankCertificate)
	vaultItem := Item{Overview: Overview{URLs: []string{
		"https://bank.example",
		"ANDROIDAPP://com.bank.app/?sha256=" + bankCertificate,
		"androidapp://com.bank.lite",
		"androidapp://com.bank.app?sha256=" + bankCertificateLower + "&extra=1",
	}}}
	if apps := vaultItem.AndroidApps(); !slices.Equal(apps, []AndroidApp{bank}) {
		t.Errorf("apps %+v", apps)
	}
	wantWebsites := []string{"https://bank.example", "androidapp://com.bank.lite", "androidapp://com.bank.app?sha256=" + bankCertificateLower + "&extra=1"}
	if websites := vaultItem.Websites(); !slices.Equal(websites, wantWebsites) {
		t.Errorf("websites %q", websites)
	}
}

func TestAddAndRemoveAndroidApp(t *testing.T) {
	bank, _ := ParseAndroidApp("com.bank.app", bankCertificate)
	sideloaded, _ := ParseAndroidApp("com.bank.app", "00"+bankCertificateLower[2:])
	vaultItem := Item{Overview: Overview{URLs: []string{"https://bank.example"}}}
	if !vaultItem.AddAndroidApp(bank) || vaultItem.AddAndroidApp(bank) {
		t.Fatal("adding twice")
	}
	if !slices.Equal(vaultItem.Overview.URLs, []string{"https://bank.example", bank.URL()}) {
		t.Errorf("URLs %q", vaultItem.Overview.URLs)
	}
	if !vaultItem.IsForAndroidApp(bank) || vaultItem.IsForAndroidApp(sideloaded) {
		t.Error("an app with the same package and another certificate matches")
	}
	if vaultItem.RemoveAndroidApp(sideloaded) || !vaultItem.RemoveAndroidApp(bank) || vaultItem.IsForAndroidApp(bank) {
		t.Error("removing")
	}
	if !slices.Equal(vaultItem.Overview.URLs, []string{"https://bank.example"}) {
		t.Errorf("URLs after removing %q", vaultItem.Overview.URLs)
	}
}

func TestLookupURLSkipsAndroidApps(t *testing.T) {
	bank, _ := ParseAndroidApp("com.bank.app", bankCertificate)
	names := FieldNames{"url": FieldURL}
	appOnly := Item{Overview: Overview{URLs: []string{bank.URL()}}}
	if value, found := LookupField(&appOnly, "url", names, time.Now()); found {
		t.Errorf("an app-only login's url = %q", value)
	}
	appFirst := Item{Overview: Overview{URLs: []string{bank.URL(), "https://bank.example"}}}
	if value, found := LookupField(&appFirst, "url", names, time.Now()); !found || value != "https://bank.example" {
		t.Errorf("url = %q %v", value, found)
	}
}
