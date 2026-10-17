package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
)

var builtAdminPageFixture = fstest.MapFS{
	".gitkeep":                     {},
	"index.html":                   {Data: []byte(`<!doctype html><script type="module" src="/admin/assets/index-a1b2.js"></script>`)},
	"assets/index-a1b2.js":         {Data: []byte("console.log(1)")},
	"assets/index-a1b2.js.br":      {Data: []byte("brotli js")},
	"assets/index-a1b2.js.gz":      {Data: []byte("gzip js")},
	"assets/index-c3d4.css":        {Data: []byte("body{}")},
	"assets/hisar-e5f6.wasm":       {Data: []byte("\x00asm")},
	"assets/hisar-e5f6.wasm.br":    {Data: []byte("brotli wasm")},
	"assets/worker-g7h8.js":        {Data: []byte("self.onmessage=null")},
	"assets/worker-g7h8.js.gz":     {Data: []byte("gzip worker")},
	"assets/unknown-i9j0.bin":      {Data: []byte{1, 2, 3}},
	"assets/nested/font-k1l2.woff": {Data: []byte("font")},
}

func serveAdminPage(t *testing.T, files fstest.MapFS, target, acceptEncoding string) (*http.Response, string) {
	t.Helper()
	app := fiber.New()
	app.Use(setSecurityHeaders)
	newAdminPage(files).register(app)
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptEncoding != "" {
		request.Header.Set("Accept-Encoding", acceptEncoding)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(body)
}

func checkAdminPageHeaders(t *testing.T, response *http.Response, cacheControl, contentType string) {
	t.Helper()
	want := map[string]string{
		"Content-Security-Policy":    "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; connect-src 'self'; img-src 'self' data:; worker-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
		"Referrer-Policy":            "no-referrer",
		"X-Robots-Tag":               "noindex, nofollow",
		"Cross-Origin-Opener-Policy": "same-origin",
		"Cache-Control":              cacheControl,
		"Content-Type":               contentType,
	}
	for name, value := range want {
		if got := response.Header.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestAdminPageServesIndexForEveryPathButAssets(t *testing.T) {
	index := string(builtAdminPageFixture["index.html"].Data)
	for _, target := range []string{"/admin", "/admin/", "/admin/accounts/42", "/admin/index.html", "/admin/.gitkeep", "/admin/assetsx/index-a1b2.js"} {
		response, body := serveAdminPage(t, builtAdminPageFixture, target, "br")
		if response.StatusCode != http.StatusOK || body != index {
			t.Errorf("%s: %d %q", target, response.StatusCode, body)
			continue
		}
		checkAdminPageHeaders(t, response, "no-store", "text/html; charset=utf-8")
		if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
			t.Errorf("%s: Content-Encoding %q", target, encoding)
		}
	}
}

func TestAdminPageServesHashedAssets(t *testing.T) {
	immutable := "public, max-age=31536000, immutable"
	cases := []struct {
		target, acceptEncoding, contentType, contentEncoding, body string
	}{
		{"/admin/assets/index-a1b2.js", "", "text/javascript; charset=utf-8", "", "console.log(1)"},
		{"/admin/assets/index-a1b2.js", "gzip, deflate, br", "text/javascript; charset=utf-8", "br", "brotli js"},
		{"/admin/assets/index-a1b2.js", "gzip", "text/javascript; charset=utf-8", "gzip", "gzip js"},
		{"/admin/assets/index-a1b2.js", "br;q=0, gzip", "text/javascript; charset=utf-8", "gzip", "gzip js"},
		{"/admin/assets/index-a1b2.js", "identity", "text/javascript; charset=utf-8", "", "console.log(1)"},
		{"/admin/assets/index-c3d4.css", "br, gzip", "text/css; charset=utf-8", "", "body{}"},
		{"/admin/assets/hisar-e5f6.wasm", "", "application/wasm", "", "\x00asm"},
		{"/admin/assets/hisar-e5f6.wasm", "br", "application/wasm", "br", "brotli wasm"},
		{"/admin/assets/hisar-e5f6.wasm", "gzip", "application/wasm", "", "\x00asm"},
		{"/admin/assets/worker-g7h8.js", "br, gzip", "text/javascript; charset=utf-8", "gzip", "gzip worker"},
		{"/admin/assets/unknown-i9j0.bin", "", "application/octet-stream", "", "\x01\x02\x03"},
		{"/admin/assets/nested/font-k1l2.woff", "", "application/octet-stream", "", "font"},
	}
	for _, testCase := range cases {
		response, body := serveAdminPage(t, builtAdminPageFixture, testCase.target, testCase.acceptEncoding)
		if response.StatusCode != http.StatusOK || body != testCase.body {
			t.Errorf("%s with %q: %d %q", testCase.target, testCase.acceptEncoding, response.StatusCode, body)
			continue
		}
		checkAdminPageHeaders(t, response, immutable, testCase.contentType)
		if encoding := response.Header.Get("Content-Encoding"); encoding != testCase.contentEncoding {
			t.Errorf("%s with %q: Content-Encoding %q, want %q", testCase.target, testCase.acceptEncoding, encoding, testCase.contentEncoding)
		}
		if vary := response.Header.Get("Vary"); vary != "Accept-Encoding" {
			t.Errorf("%s: Vary %q", testCase.target, vary)
		}
	}
}

func TestAdminPageRefusesUnknownAndCompressedAssetPaths(t *testing.T) {
	for _, target := range []string{"/admin/assets/missing.js", "/admin/assets/index-a1b2.js.br", "/admin/assets/nested"} {
		if response, body := serveAdminPage(t, builtAdminPageFixture, target, ""); response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d %q", target, response.StatusCode, body)
		}
	}
}

func TestAdminPageAnswers404WhenNotBuilt(t *testing.T) {
	for _, target := range []string{"/admin", "/admin/", "/admin/assets/index-a1b2.js"} {
		response, body := serveAdminPage(t, fstest.MapFS{".gitkeep": {}}, target, "")
		if response.StatusCode != http.StatusNotFound || body != "The admin dashboard was not built into this server." {
			t.Errorf("%s: %d %q", target, response.StatusCode, body)
		}
		if contentType := response.Header.Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
			t.Errorf("%s: Content-Type %q", target, contentType)
		}
	}
}

func TestAdminPageIsEmbedded(t *testing.T) {
	if _, err := builtAdminPageFiles().Open(".gitkeep"); err != nil {
		t.Errorf("adminpage/dist is not embedded: %v", err)
	}
}
