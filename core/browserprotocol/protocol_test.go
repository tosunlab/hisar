package browserprotocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestFraming(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteMessage(&buffer, []byte(`{"id":"1"}`)); err != nil {
		t.Fatal(err)
	}
	if binary.NativeEndian.Uint32(buffer.Bytes()[:4]) != 10 {
		t.Errorf("length prefix = %v", buffer.Bytes()[:4])
	}
	message, err := ReadMessage(&buffer)
	if err != nil || string(message) != `{"id":"1"}` {
		t.Fatalf("read: %q %v", message, err)
	}
	if _, err := ReadMessage(&buffer); !errors.Is(err, io.EOF) {
		t.Errorf("empty input: %v", err)
	}
	if err := WriteMessage(io.Discard, make([]byte, MaxMessageSize+1)); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("oversized write: %v", err)
	}
	oversized := binary.NativeEndian.AppendUint32(nil, MaxMessageSize+1)
	if _, err := ReadMessage(bytes.NewReader(oversized)); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("oversized read: %v", err)
	}
	truncated := append(binary.NativeEndian.AppendUint32(nil, 10), []byte("abc")...)
	if _, err := ReadMessage(bytes.NewReader(truncated)); err == nil {
		t.Error("truncated message accepted")
	}
}

func TestIsURLMatch(t *testing.T) {
	allCase := []struct {
		itemURL, pageURL string
		isExactHost      bool
		want             bool
	}{
		{"https://github.com", "https://github.com/login", false, true},
		{"github.com", "https://github.com/login", false, true},
		{"https://google.com", "https://accounts.google.com/signin", false, true},
		{"https://google.com", "https://accounts.google.com/signin", true, false},
		{"https://www.example.com", "https://example.com", false, true},
		{"https://Example.COM.", "https://example.com", true, true},
		{"https://alice.github.io", "https://mallory.github.io", false, false},
		{"https://example.com", "https://example.com.evil.net", false, false},
		{"https://example.com", "https://evilexample.com", false, false},
		{"http://router.home.arpa", "https://router.home.arpa", false, true},
		{"https://example.com", "http://example.com", false, false},
		{"https://example.com:8443", "https://example.com:8443/", false, true},
		{"https://example.com:8443", "https://example.com", false, false},
		{"https://example.com:443", "https://example.com", false, true},
		{"http://192.168.1.1", "http://192.168.1.1/admin", false, true},
		{"http://192.168.1.1", "http://192.168.1.2", false, false},
		{"http://localhost:3000", "http://localhost:3000/login", false, true},
		{"https://example.com", "file:///etc/passwd", false, false},
		{"https://example.com", "javascript:alert(1)", false, false},
		{"", "https://example.com", false, false},
		{"https://co.uk", "https://bank.co.uk", false, false},
	}
	for _, testCase := range allCase {
		if got := IsURLMatch(testCase.itemURL, testCase.pageURL, testCase.isExactHost); got != testCase.want {
			t.Errorf("IsURLMatch(%q, %q, exact=%v) = %v", testCase.itemURL, testCase.pageURL, testCase.isExactHost, got)
		}
	}
}

func TestGetSiteNameAndOrigin(t *testing.T) {
	if name := GetSiteName("https://www.GitHub.com/login"); name != "github.com" {
		t.Errorf("site name = %q", name)
	}
	if origin := GetOrigin("https://example.com:8443/a?b"); origin != "https://example.com:8443" {
		t.Errorf("origin = %q", origin)
	}
}
