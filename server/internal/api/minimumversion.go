package api

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/appversion"
)

const extensionStoreURL = "https://chromewebstore.google.com/detail/mmipiffoogblblocokimpemdopmdjghf"

var updateURLsByClient = map[string]string{
	apitypes.ClientDesktopMacOS: downloadURL,
	apitypes.ClientIOS:          downloadURL,
	apitypes.ClientAndroid:      downloadURL,
	apitypes.ClientCLI:          downloadURL,
	apitypes.ClientExtension:    extensionStoreURL,
}

// requireMinimumVersion refuses only apps that say they are too old; callers without headers or with an unparsable version pass.
// The desktop update feed stays open so a refused Mac app can still update itself.
func (s *Server) requireMinimumVersion(c fiber.Ctx) error {
	if strings.HasPrefix(c.Path(), "/api/v1/desktop/updates/") {
		return c.Next()
	}
	client := apitypes.GetClient(c.Get(apitypes.PlatformHeader))
	minimumVersion := s.instanceSettings.Load().MinimumVersions.Get(client)
	version := c.Get(apitypes.VersionHeader)
	if minimumVersion == "" || version == "" {
		return c.Next()
	}
	if isOlder, err := appversion.IsOlder(version, minimumVersion); err != nil || !isOlder {
		return c.Next()
	}
	return c.Status(fiber.StatusUpgradeRequired).JSON(apitypes.UpgradeRequired{
		Error:          "update required",
		MinimumVersion: minimumVersion,
		UpdateURL:      updateURLsByClient[client],
	})
}

func (s *Server) limitVersionCheckRate(c fiber.Ctx) error {
	if !s.versionCheckLimiter.Allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	return c.Next()
}

// handleCheckVersion answers 204 to apps the minimum version lets through; requireMinimumVersion answers the others.
func handleCheckVersion(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }
