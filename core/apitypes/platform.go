package apitypes

import "strings"

// Every app sends its platform and version, so the server can refuse versions below its minimum with 426.
const (
	PlatformHeader = "Hisar-Platform"
	VersionHeader  = "Hisar-Version"

	PlatformDesktopMacOS = "darwin/desktop"
	PlatformIOS          = "ios/mobile"
	PlatformAndroid      = "android/mobile"
	PlatformExtension    = "browser/extension"
	cliPlatformSuffix    = "/cli"
	phonePlatformSuffix  = "/mobile"

	ClientDesktopMacOS = "desktop-macos"
	ClientIOS          = "ios"
	ClientAndroid      = "android"
	ClientCLI          = "cli"
	ClientExtension    = "extension"
)

// UpgradeRequired is the body of 426 Upgrade Required.
type UpgradeRequired struct {
	Error          string `json:"error"`
	MinimumVersion string `json:"minimumVersion"`
	UpdateURL      string `json:"updateURL"`
}

// GetClient maps a platform header to the app it names; "" for platforms without a minimum version.
func GetClient(platform string) string {
	switch {
	case platform == PlatformDesktopMacOS:
		return ClientDesktopMacOS
	case platform == PlatformIOS:
		return ClientIOS
	case platform == PlatformAndroid:
		return ClientAndroid
	case platform == PlatformExtension:
		return ClientExtension
	case strings.HasSuffix(platform, cliPlatformSuffix) && len(platform) > len(cliPlatformSuffix):
		return ClientCLI
	}
	return ""
}

// IsPhonePlatform reports whether a device's platform is a phone app, which can answer phone approvals.
func IsPhonePlatform(platform string) bool {
	return strings.HasSuffix(platform, phonePlatformSuffix) && len(platform) > len(phonePlatformSuffix)
}
