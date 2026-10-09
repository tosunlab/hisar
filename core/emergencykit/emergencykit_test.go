package emergencykit

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
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

func TestBuildSignInURI(t *testing.T) {
	signInURI := BuildSignInURI(createTestKit())
	parsedURI, err := url.Parse(signInURI)
	if err != nil {
		t.Fatal(err)
	}
	if parsedURI.Scheme != "hisar" || parsedURI.Host != "sign-in" {
		t.Errorf("uri = %s", signInURI)
	}
	query := parsedURI.Query()
	if query.Get("server") != "https://tryhisar.com" || query.Get("email") != "alper@tryhisar.com" || query.Get("secretKey") != "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M" {
		t.Errorf("query = %v", query)
	}
	if len(query) != 3 {
		t.Errorf("unexpected fields in the QR payload: %v", query)
	}
}
