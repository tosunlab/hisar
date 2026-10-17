package api

import (
	"embed"
	"io/fs"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
)

//go:embed all:adminpage/dist
var adminPageDist embed.FS

func builtAdminPageFiles() fs.FS {
	files, err := fs.Sub(adminPageDist, "adminpage/dist")
	if err != nil {
		panic(err)
	}
	return files
}

const (
	adminPageContentSecurityPolicy = "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; connect-src 'self'; " +
		"img-src 'self' data:; worker-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	adminAssetDirectory = "assets/"
	notBuiltMessage     = "The admin dashboard was not built into this server."
)

var adminAssetContentTypes = map[string]string{
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".wasm": "application/wasm",
}

type adminAsset struct {
	contentType string
	hasBrotli   bool
	hasGzip     bool
}

type adminPage struct {
	files  fs.FS
	index  []byte
	assets map[string]adminAsset
}

func newAdminPage(files fs.FS) *adminPage {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return &adminPage{}
	}
	isFile := map[string]bool{}
	fs.WalkDir(files, strings.TrimSuffix(adminAssetDirectory, "/"), func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			isFile[name] = true
		}
		return err
	})
	assets := map[string]adminAsset{}
	for name := range isFile {
		extension := path.Ext(name)
		if extension == ".br" || extension == ".gz" {
			continue
		}
		contentType, isKnown := adminAssetContentTypes[extension]
		if !isKnown {
			contentType = fiber.MIMEOctetStream
		}
		assets[name] = adminAsset{contentType: contentType, hasBrotli: isFile[name+".br"], hasGzip: isFile[name+".gz"]}
	}
	return &adminPage{files: files, index: index, assets: assets}
}

func (page *adminPage) register(router fiber.Router) {
	router.Get("/admin", page.serve)
	router.Get("/admin/*", page.serve)
}

func (page *adminPage) serve(c fiber.Ctx) error {
	if page.index == nil {
		c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
		return c.Status(fiber.StatusNotFound).SendString(notBuiltMessage)
	}
	c.Set("Content-Security-Policy", adminPageContentSecurityPolicy)
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	c.Set("Cross-Origin-Opener-Policy", "same-origin")
	name := c.Params("*")
	if !strings.HasPrefix(name, adminAssetDirectory) {
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.Send(page.index)
	}
	asset, isAsset := page.assets[name]
	if !isAsset {
		return errNotFound
	}
	c.Vary(fiber.HeaderAcceptEncoding)
	switch chooseAdminAssetEncoding(c, asset) {
	case "br":
		name += ".br"
		c.Set(fiber.HeaderContentEncoding, "br")
	case "gzip":
		name += ".gz"
		c.Set(fiber.HeaderContentEncoding, "gzip")
	}
	file, err := page.files.Open(name)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	c.Set(fiber.HeaderContentType, asset.contentType)
	c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	return c.SendStream(file, int(info.Size()))
}

func chooseAdminAssetEncoding(c fiber.Ctx, asset adminAsset) string {
	switch {
	// fiber's AcceptsEncodings answers its first offer when Accept-Encoding is absent.
	case c.Get(fiber.HeaderAcceptEncoding) == "":
		return ""
	case asset.hasBrotli && c.AcceptsEncodings("br") == "br":
		return "br"
	case asset.hasGzip && c.AcceptsEncodings("gzip") == "gzip":
		return "gzip"
	}
	return ""
}
