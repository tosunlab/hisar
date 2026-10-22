package emergencykit

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func createTestKit() Kit {
	return Kit{
		Name:      "Alper Şahin",
		Email:     "alper@tryhisar.com",
		Server:    "https://tryhisar.com",
		SecretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M",
		CreatedAt: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
	}
}

func TestGenerateProducesSinglePagePDF(t *testing.T) {
	document, err := Generate(createTestKit())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(document, []byte("%PDF-")) || !bytes.Contains(document[len(document)-64:], []byte("%%EOF")) {
		t.Fatal("output is not a complete PDF")
	}
	if pageCount := len(regexp.MustCompile(`/Type\s*/Page[^s]`).FindAll(document, -1)); pageCount != 1 {
		t.Errorf("pages = %d, want 1", pageCount)
	}
	if !bytes.Contains(document, []byte("/Title")) {
		t.Error("document title missing from the PDF info")
	}
	if path := os.Getenv("HISAR_TEST_KIT_PDF"); path != "" {
		if err := os.WriteFile(path, document, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGenerateRequiresSignInDetails(t *testing.T) {
	for name, kit := range map[string]Kit{
		"no server":     {Email: "a@example.com", SecretKey: "H1-X"},
		"no email":      {Server: "https://h", SecretKey: "H1-X"},
		"no secret key": {Server: "https://h", Email: "a@example.com"},
	} {
		if _, err := Generate(kit); !errors.Is(err, ErrIncompleteKit) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestGenerateHandlesLongValuesAndNoName(t *testing.T) {
	kit := createTestKit()
	kit.Name = ""
	kit.Server = "https://passwords.a-very-long-family-domain-name.example.com/hisar/sign-in"
	kit.Email = "someone.with.a.really.long.address@a-very-long-family-domain-name.example.com"
	kit.CreatedAt = time.Time{}
	if _, err := Generate(kit); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratePrintsLongSignInDetailsWithoutOverlap(t *testing.T) {
	kit := createTestKit()
	kit.Server = "https://passwords.a-very-long-family-domain-name.example.com/hisar/a-long-path-under-the-reverse-proxy/sign-in"
	kit.Email = "someone.with.a.really.long.address.that.wraps@a-very-long-family-domain-name.example.com"
	document, err := Generate(kit)
	if err != nil {
		t.Fatal(err)
	}
	lines := readPrintedLines(t, document)
	var printed strings.Builder
	for _, line := range lines {
		printed.WriteString(line.text)
		if line.left < 0 || line.right > pageWidth || line.bottom < 0 || line.top > pageHeight {
			t.Errorf("%q is printed off the page", line.text)
		}
	}
	for _, want := range []string{"Scan only with Hisar.", "a-long-path-under-the-reverse-proxy", "that.wraps@", "Signing in on a new device"} {
		if !strings.Contains(printed.String(), want) {
			t.Errorf("%q is not printed", want)
		}
	}
	for first := range lines {
		for second := first + 1; second < len(lines); second++ {
			if lines[first].overlaps(lines[second]) {
				t.Errorf("%q is printed over %q", lines[first].text, lines[second].text)
			}
		}
	}
}

func TestQRPayloadIsPlainTextAndPreservesSignInDetails(t *testing.T) {
	kit := createTestKit()
	kit.Server = "https://hisar.example/a?x=one+two&y=%20"
	kit.Email = "alper+kit@tryhisar.com"
	payload := BuildQRPayload(kit)
	const canonical = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL2hpc2FyLmV4YW1wbGUvYT94PW9uZSt0d29cdTAwMjZ5PSUyMCIsImVtYWlsIjoiYWxwZXIra2l0QHRyeWhpc2FyLmNvbSIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
	if payload != canonical {
		t.Fatalf("canonical payload changed: %q", payload)
	}
	if !strings.HasPrefix(payload, "HISARKIT1 ") || strings.ContainsAny(payload, ":.@=/") {
		t.Fatalf("payload is not the specified non-URI text: %q", payload)
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(payload, "HISARKIT1 "))
	if err != nil {
		t.Fatal(err)
	}
	var decoded signInPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != (signInPayload{Server: kit.Server, Email: kit.Email, SecretKey: kit.SecretKey}) {
		t.Fatalf("sign-in bytes changed: %#v", decoded)
	}
}

// The phones' tests parse this exact payload, so the generator and both scanners agree on international server names.
func TestQRPayloadKeepsAnInternationalServerName(t *testing.T) {
	kit := createTestKit()
	kit.Server = "https://例え.test:8443"
	kit.Email = "李雷@例え.test"
	if _, err := Generate(kit); err != nil {
		t.Fatal(err)
	}
	const canonical = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL-S-i-OBiC50ZXN0Ojg0NDMiLCJlbWFpbCI6Iuadjumbt0DkvovjgYgudGVzdCIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
	if payload := BuildQRPayload(kit); payload != canonical {
		t.Fatalf("canonical payload changed: %q", payload)
	}
	kit.Server = "https://user@例え.test"
	if _, err := Generate(kit); err == nil {
		t.Fatal("a server address with a user name was accepted")
	}
}
