package signin

import "testing"

func TestParseOrigin(t *testing.T) {
	for rawURL, expected := range map[string]string{
		"https://Example.COM:443/x?y#z":             "https://example.com",
		"https://example.com:8443/login":            "https://example.com:8443",
		"http://127.0.0.1:8765/plain":               "http://127.0.0.1:8765",
		"http://localhost/":                         "http://localhost",
		"HTTP://LOCALHOST:80":                       "http://localhost",
		"https://xn--bcher-kva.example/":            "https://xn--bcher-kva.example",
		"https://bücher.example/":                   "https://xn--bcher-kva.example",
		"https://[::1]:8443/":                       "https://[::1]:8443",
		"https://user:secret@accounts.example.com/": "https://accounts.example.com",
	} {
		origin, err := ParseOrigin(rawURL)
		if err != nil || origin.String() != expected {
			t.Errorf("ParseOrigin(%q) = %q, %v; want %q", rawURL, origin.String(), err, expected)
		}
	}
	for _, rawURL := range []string{
		"", "about:blank", "about:srcdoc", "data:text/html,<input type=password>", "blob:https://example.com/0f6e",
		"file:///Users/developer/login.html", "javascript:alert(1)", "chrome://settings", "https://", "https://example.com:0/",
		"https://example.com:99999/", "ftp://example.com/",
	} {
		if origin, err := ParseOrigin(rawURL); err == nil {
			t.Errorf("ParseOrigin(%q) = %q, want a refusal", rawURL, origin)
		}
	}
}

func TestOriginsDifferInSchemeHostAndPort(t *testing.T) {
	parse := func(rawURL string) Origin {
		origin, err := ParseOrigin(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		return origin
	}
	approved := parse("https://tosunlab.com/sign-in")
	if approved != parse("https://TOSUNLAB.com:443/other") {
		t.Error("the same origin compares unequal")
	}
	for _, other := range []string{"http://tosunlab.com", "https://www.tosunlab.com", "https://tosunlab.com.evil.example", "https://tosunlab.com:8443"} {
		if approved == parse(other) {
			t.Errorf("%s equals %s", other, approved)
		}
	}
}

func TestSecureOrigins(t *testing.T) {
	for rawURL, isSecure := range map[string]bool{
		"https://example.com":       true,
		"http://127.0.0.1:8765":     true,
		"http://127.8.9.1":          true,
		"http://localhost:3000":     true,
		"http://[::1]:3000":         true,
		"http://example.com":        false,
		"http://localhost.evil.com": false,
		"http://192.168.1.10":       false,
		"http://10.0.0.1":           false,
	} {
		origin, err := ParseOrigin(rawURL)
		if err != nil || origin.isSecure() != isSecure {
			t.Errorf("%s: secure %v (%v), want %v", rawURL, origin.isSecure(), err, isSecure)
		}
	}
}
