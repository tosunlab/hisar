package invitelink

import "testing"

func TestParse(t *testing.T) {
	server, token, ok := Parse(" https://tryhisar.com/invite/abc_DEF-123 ")
	if !ok || server != "https://tryhisar.com" || token != "abc_DEF-123" {
		t.Errorf("got %q %q %v", server, token, ok)
	}
	if server, token, ok := Parse("http://127.0.0.1:18080/invite/xyz"); !ok || server != "http://127.0.0.1:18080" || token != "xyz" {
		t.Errorf("http: %q %q %v", server, token, ok)
	}
	for _, badLink := range []string{"", "tryhisar.com/invite/x", "https://tryhisar.com/invite/", "https://tryhisar.com/other/x", "ftp://h/invite/x", "https://h/invite/a/b"} {
		if _, _, ok := Parse(badLink); ok {
			t.Errorf("%q accepted", badLink)
		}
	}
}
