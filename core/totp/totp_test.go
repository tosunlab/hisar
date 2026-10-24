package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B: the seeds are ASCII "1234567890" repeated to the hash's block length.
func createRFCConfig(algorithm string, seedLength int) Config {
	seed := []byte(strings.Repeat("1234567890", 7)[:seedLength])
	return Config{Secret: seed, Algorithm: algorithm, Digits: 8, Period: 30}
}

func TestRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unixTime                     int64
		sha1Code, sha256Code, sha512 string
	}{
		{59, "94287082", "46119246", "90693936"},
		{1111111109, "07081804", "68084774", "25091201"},
		{1111111111, "14050471", "67062674", "99943326"},
		{1234567890, "89005924", "91819424", "93441116"},
		{2000000000, "69279037", "90698825", "38618901"},
		{20000000000, "65353130", "77737706", "47863826"},
	}
	for _, testCase := range cases {
		at := time.Unix(testCase.unixTime, 0)
		for algorithm, want := range map[string]string{"SHA1": testCase.sha1Code, "SHA256": testCase.sha256Code, "SHA512": testCase.sha512} {
			seedLength := map[string]int{"SHA1": 20, "SHA256": 32, "SHA512": 64}[algorithm]
			if code, _ := createRFCConfig(algorithm, seedLength).GenerateCode(at); code != want {
				t.Errorf("%s at %d = %s, want %s", algorithm, testCase.unixTime, code, want)
			}
		}
	}
}

func TestGenerateCodeExpiry(t *testing.T) {
	config := createRFCConfig("SHA1", 20)
	config.Digits = 6
	code, expiresAt := config.GenerateCode(time.Unix(59, 0))
	if len(code) != 6 || !expiresAt.Equal(time.Unix(60, 0)) {
		t.Errorf("code %q expires %v", code, expiresAt)
	}
	nextCode, _ := config.GenerateCode(time.Unix(60, 0))
	if nextCode == code {
		t.Error("the code did not change at the period boundary")
	}
}

func TestParse(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	config, err := Parse("otpauth://totp/GitHub:octocat?secret=" + secret + "&issuer=GitHub&algorithm=sha256&digits=8&period=60")
	if err != nil || config.Issuer != "GitHub" || config.Account != "octocat" || config.Algorithm != "SHA256" || config.Digits != 8 || config.Period != 60 {
		t.Fatalf("parsed = %+v %v", config, err)
	}
	if string(config.Secret) != "12345678901234567890" {
		t.Errorf("secret = %q", config.Secret)
	}

	spaced := strings.ToLower(secret[:8] + " " + secret[8:16] + " " + secret[16:])
	if config, err := Parse(spaced); err != nil || config.Algorithm != "SHA1" || config.Digits != 6 || config.Period != 30 {
		t.Errorf("bare secret: %+v %v", config, err)
	}
	if config, err := Parse("otpauth://totp/Example%20Corp:alice%40example.com?secret=" + secret); err != nil || config.Issuer != "Example Corp" || config.Account != "alice@example.com" {
		t.Errorf("issuer from label: %+v %v", config, err)
	}

	if config, err := Parse("otpauth://totp/x?secret=" + secret + "&image=%zz&algorithm=SHA256"); err != nil || config.Algorithm != "SHA256" {
		t.Errorf("a malformed parameter Hisar does not read: %+v %v", config, err)
	}
	if config, err := Parse("otpauth://totp/x?secret=" + secret + "&image%=unused&algorithm=SHA256"); err != nil || config.Algorithm != "SHA256" {
		t.Errorf("a malformed unknown parameter name: %+v %v", config, err)
	}

	for _, invalid := range []string{
		"", "not base32!", "AAAA",
		"otpauth://hotp/x?secret=" + secret,
		"otpauth://totp/x?secret=" + secret + "&algorithm=MD5",
		"otpauth://totp/x?secret=" + secret + "&digits=4",
		"otpauth://totp/x?secret=" + secret + "&period=0",
		"otpauth://totp/x",
		"otpauth://totp/x?secret=" + secret + "&algorithm=SHA%256",
		"otpauth://totp/x?secret=" + secret + "&algorithm=SHA256;",
		"otpauth://totp/x?secret=" + secret + "&digits=8%",
		"otpauth://totp/x?secret=" + secret + "&%61lgorithm=SHA256;",
		"otpauth://totp/x?secret=" + secret + "&%64igits=8%",
		"otpauth://totp/x?secret=" + secret + "&%70eriod=60%",
		"otpauth://totp/x?secret=" + secret + "&algorithm%=SHA256",
		"otpauth://totp/x?secret=" + secret + "&digits%zz=8",
		"otpauth://totp/x?secret=" + secret + "&%70eriod%0=60",
		"otpauth://totp/x?secret=" + secret + "&ignored=x;issuer%=Example",
		"otpauth://totp/x?secret=" + secret + "&secret%=OTHER",
		"otpauth://totp/x?secret=" + secret + "&%61lgorithm%=SHA256",
		"otpauth://totp/x?secret=" + secret + "&%64igits%=8",
		"otpauth://totp/x?secret=" + secret + "&%70eriod%=60",
		"otpauth://totp/x?secret=" + secret + "&%61lgorithm%6=SHA256",
		"otpauth://totp/x?secret=" + secret + "&%64igits%6=8",
		"otpauth://totp/x?secret=" + secret + "&%70eriod%7=60",
		"otpauth://totp/x?secret=" + secret + "&algorithm=SHA256&algorithm=SHA1",
		"otpauth://totp/x?secret=" + secret + "&period=60&period=30",
	} {
		if _, err := Parse(invalid); err != ErrInvalid {
			t.Errorf("%q: %v", invalid, err)
		}
	}
}

func TestNormalizeRoundTrip(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	uri, err := Normalize(strings.ToLower(secret))
	if err != nil || uri != "otpauth://totp/?secret="+secret {
		t.Fatalf("bare secret normalized to %q (%v)", uri, err)
	}
	original := "otpauth://totp/GitHub:octocat?secret=" + secret + "&issuer=GitHub&digits=8"
	uri, err = Normalize(original)
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(uri)
	if err != nil || reparsed.Issuer != "GitHub" || reparsed.Account != "octocat" || reparsed.Digits != 8 || string(reparsed.Secret) != "12345678901234567890" {
		t.Errorf("round trip %q: %+v %v", uri, reparsed, err)
	}
}
