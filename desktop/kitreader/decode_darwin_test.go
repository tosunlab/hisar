//go:build darwin && cgo

package kitreader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/signintech/gopdf"
	"github.com/skip2/go-qrcode"
	"github.com/tryhisar/hisar/core/emergencykit"
)

func TestReadGeneratedEmergencyKit(t *testing.T) {
	kit := emergencykit.Kit{Server: "https://hisar.example", Email: "kit@example.com", SecretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"}
	data, err := emergencykit.Generate(kit)
	if err != nil {
		t.Fatal("could not generate test kit")
	}
	path := filepath.Join(t.TempDir(), "kit.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	fields, err := New(func() (string, error) { return path, nil }).Open()
	if err != nil || fields == nil || fields.Server != kit.Server || fields.Email != kit.Email || fields.SecretKey != kit.SecretKey {
		t.Fatal("the native PDF decoder did not recover the kit account fields")
	}
}

func TestReadLegacyKitAndRejectOtherCodes(t *testing.T) {
	for _, valid := range []bool{false, true} {
		text := "https://example.com/not-a-kit"
		if valid {
			text = "hisar://sign-in?server=https%3A%2F%2Fhisar.example&email=kit%40example.com&secretKey=H1-ABC123"
		}
		png, err := qrcode.Encode(text, qrcode.Medium, 512)
		if err != nil {
			t.Fatal(err)
		}
		image, err := gopdf.ImageHolderByBytes(png)
		if err != nil {
			t.Fatal(err)
		}
		pdf := &gopdf.GoPdf{}
		pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
		pdf.AddPage()
		if err := pdf.ImageByHolder(image, 72, 72, &gopdf.Rect{W: 200, H: 200}); err != nil {
			t.Fatal(err)
		}
		data := pdf.GetBytesPdf()
		path := filepath.Join(t.TempDir(), "kit.pdf")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		fields, err := readPDF(path)
		if valid && (err != nil || fields == nil || fields.Email != "kit@example.com") {
			t.Fatal("legacy PDF kit was not read")
		}
		if !valid && (err == nil || fields != nil) {
			t.Fatal("unrelated QR code was accepted")
		}
	}
}

func TestReadPDFBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.pdf")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxPDFBytes + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if fields, err := readPDF(path); fields != nil || err == nil {
		t.Fatal("oversized PDF was accepted")
	}
	if fields, err := readPDF(t.TempDir()); fields != nil || err == nil {
		t.Fatal("directory was accepted")
	}
	if err := os.WriteFile(path, []byte("not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fields, err := readPDF(path); fields != nil || err == nil {
		t.Fatal("non-PDF file was accepted")
	}
}
