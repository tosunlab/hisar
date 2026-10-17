package itemicon

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func createTestImage(width, height int) *image.NRGBA {
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			picture.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 3), uint8(x ^ y), 255})
		}
	}
	return picture
}

func encodeTestPNG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// encodeTestICO builds an ICO with a 16 px 32-bit bitmap entry and a PNG entry of side pixels.
func encodeTestICO(t *testing.T, side int) []byte {
	t.Helper()
	small := 16
	dib := make([]byte, 40+small*small*4+small*4)
	binary.LittleEndian.PutUint32(dib[0:], 40)
	binary.LittleEndian.PutUint32(dib[4:], uint32(small))
	binary.LittleEndian.PutUint32(dib[8:], uint32(small*2))
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	for index := 40; index < 40+small*small*4; index += 4 {
		copy(dib[index:], []byte{0, 0, 255, 255})
	}
	large := encodeTestPNG(t, createTestImage(side, side))
	header := make([]byte, 6+32)
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], 2)
	writeEntry := func(entry []byte, side int, bits uint16, size, offset int) {
		entry[0] = byte(side % 256)
		entry[1] = byte(side % 256)
		binary.LittleEndian.PutUint16(entry[4:], 1)
		binary.LittleEndian.PutUint16(entry[6:], bits)
		binary.LittleEndian.PutUint32(entry[8:], uint32(size))
		binary.LittleEndian.PutUint32(entry[12:], uint32(offset))
	}
	writeEntry(header[6:], small, 32, len(dib), len(header))
	writeEntry(header[22:], side, 32, len(large), len(header)+len(dib))
	return append(append(header, dib...), large...)
}

func decodeTestPNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	return decoded
}

func TestNormalizeImageMakesASmallSquarePNG(t *testing.T) {
	var jpegImage, gifImage bytes.Buffer
	if err := jpeg.Encode(&jpegImage, createTestImage(640, 480), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&gifImage, createTestImage(40, 60), nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		data []byte
		side int
	}{
		{"large PNG", encodeTestPNG(t, createTestImage(1000, 1000)), item.MaxIconImageSide},
		{"wide JPEG", jpegImage.Bytes(), item.MaxIconImageSide},
		{"small GIF is not scaled up", gifImage.Bytes(), 40},
		{"ICO uses its largest entry", encodeTestICO(t, 64), 64},
	} {
		normalized, err := NormalizeImage(test.data)
		if err != nil {
			t.Errorf("%s: %v", test.name, err)
			continue
		}
		bounds := decodeTestPNG(t, normalized).Bounds()
		if bounds.Dx() != test.side || bounds.Dy() != test.side || !item.IsIconImage(normalized) {
			t.Errorf("%s: %v, %d bytes", test.name, bounds, len(normalized))
		}
	}
}

func TestNormalizeImageReadsAnICOBitmap(t *testing.T) {
	ico := encodeTestICO(t, 64)
	// Drop the PNG entry: the 16 px red bitmap is left.
	binary.LittleEndian.PutUint16(ico[4:], 1)
	normalized, err := NormalizeImage(ico)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeTestPNG(t, normalized)
	if red, green, _, alpha := decoded.At(8, 8).RGBA(); decoded.Bounds().Dx() != 16 || red>>8 != 255 || green != 0 || alpha>>8 != 255 {
		t.Errorf("bitmap entry: %v, %v", decoded.Bounds(), decoded.At(8, 8))
	}
}

func TestNormalizeImageRefusesOtherData(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`)
	tooLarge := encodeTestPNG(t, image.NewGray(image.Rect(0, 0, MaxSourceSide+1, 1)))
	for name, data := range map[string][]byte{"SVG": svg, "empty": nil, "text": []byte("hello"), "too large": tooLarge} {
		if _, err := NormalizeImage(data); !errors.Is(err, ErrUnsupportedImage) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDomainRefusesNonPublicURLs(t *testing.T) {
	for _, rawURL := range []string{
		"http://example.com", "https://192.168.1.10/admin", "https://[::1]/", "https://localhost/", "https://router.local/",
		"https://intranet/", "https://example.com:8443/", "https://user@example.com/", "ftp://example.com", "not a url",
		"https://exa_mple.com/", "https://nas.home.arpa/",
	} {
		if domain, err := Domain(rawURL); !errors.Is(err, ErrRefusedURL) {
			t.Errorf("Domain(%q) = %q, %v", rawURL, domain, err)
		}
	}
	if domain, err := Domain(" https://GitHub.com./login?x=1 "); err != nil || domain != "github.com" {
		t.Errorf("Domain = %q, %v", domain, err)
	}
}

func TestIsPublicAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"140.82.112.3": true, "2606:4700::6810:84e5": true, "127.0.0.1": false, "10.0.0.1": false, "192.168.1.1": false,
		"172.16.5.4": false, "169.254.169.254": false, "100.64.1.1": false, "::1": false, "fe80::1": false, "fd00::1": false,
		"0.0.0.0": false, "::ffff:127.0.0.1": false, "224.0.0.1": false,
	} {
		if got := isPublicAddress(netip.MustParseAddr(address)); got != want {
			t.Errorf("isPublicAddress(%s) = %v", address, got)
		}
	}
	if err := refusePrivateAddress("tcp4", "127.0.0.1:443", nil); !errors.Is(err, ErrRefusedURL) {
		t.Errorf("dialing loopback: %v", err)
	}
}

// startTestWebsite serves handler over TLS as example.com and returns a fetcher that reaches it.
func startTestWebsite(t *testing.T, handler http.Handler) *Fetcher {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return newFetcher(transport)
}

func TestFetchPrefersTheLargestIconLink(t *testing.T) {
	small, large := encodeTestPNG(t, createTestImage(32, 32)), encodeTestPNG(t, createTestImage(192, 192))
	fetcher := startTestWebsite(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.Write([]byte(`<html><head>
				<link rel="icon" href="/small.png" sizes="32x32">
				<link rel="icon" type="image/svg+xml" href="/logo.svg">
				<link rel="apple-touch-icon" href="/static/touch.png" sizes="192x192">
				</head><body><link rel="icon" href="/late.png" sizes="512x512"></body></html>`))
		case "/small.png":
			writer.Write(small)
		case "/static/touch.png":
			writer.Write(large)
		default:
			http.NotFound(writer, request)
		}
	}))
	icon, err := fetcher.Fetch(context.Background(), "https://example.com/login")
	if err != nil {
		t.Fatal(err)
	}
	if side := decodeTestPNG(t, icon).Bounds().Dx(); side != item.MaxIconImageSide {
		t.Errorf("icon side %d, want the touch icon scaled to %d", side, item.MaxIconImageSide)
	}
}

func TestFetchFallsBackToFaviconICO(t *testing.T) {
	ico := encodeTestICO(t, 48)
	fetcher := startTestWebsite(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			writer.Header().Set("Content-Type", "text/html")
			writer.Write([]byte(`<html><head><link rel="icon" href="/missing.png"></head></html>`))
		case "/favicon.ico":
			writer.Write(ico)
		default:
			http.NotFound(writer, request)
		}
	}))
	icon, err := fetcher.Fetch(context.Background(), "https://example.com")
	if err != nil || decodeTestPNG(t, icon).Bounds().Dx() != 48 {
		t.Fatalf("favicon.ico: %v", err)
	}
}

func TestFetchStopsAtLimits(t *testing.T) {
	var requests atomic.Int32
	fetcher := startTestWebsite(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		switch {
		case request.URL.Path == "/":
			writer.Header().Set("Content-Type", "text/html")
			links := ""
			for index := range 10 {
				links += `<link rel="icon" href="/huge` + string(rune('a'+index)) + `.png">`
			}
			writer.Write([]byte("<head>" + links + "</head>"))
		case strings.HasPrefix(request.URL.Path, "/huge"):
			writer.Write(bytes.Repeat([]byte{1}, maxIconSize+10))
		case request.URL.Path == "/redirect":
			http.Redirect(writer, request, "/redirect", http.StatusFound)
		default:
			http.NotFound(writer, request)
		}
	}))
	if _, err := fetcher.Fetch(context.Background(), "https://example.com"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("huge icons: %v", err)
	}
	if got := requests.Load(); got != 1+maxIconCandidates {
		t.Errorf("%d requests, want the page and %d icons", got, maxIconCandidates)
	}
	if _, err := fetcher.fetchImage(context.Background(), "https://example.com/redirect"); err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("redirect loop: %v", err)
	}
}

func TestFetchRefusesARedirectToHTTP(t *testing.T) {
	fetcher := startTestWebsite(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "http://example.com/favicon.ico", http.StatusFound)
	}))
	if _, err := fetcher.fetchImage(context.Background(), "https://example.com/favicon.ico"); !errors.Is(err, ErrRefusedURL) {
		t.Errorf("redirect to http: %v", err)
	}
	if _, err := NewFetcher().Fetch(context.Background(), "https://10.0.0.1/"); !errors.Is(err, ErrRefusedURL) {
		t.Errorf("IP literal: %v", err)
	}
}
