package main

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/tryhisar/hisar/core/appversion"
	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

func getPlatform() string { return runtime.GOOS + "/cli" }

// getClientVersion is the version the server compares with its minimum; development builds send none.
func getClientVersion() string {
	if _, err := appversion.Parse(version); err != nil {
		return ""
	}
	return version
}

func createClient(server string) *client.Client {
	apiClient := client.Create(server)
	apiClient.Platform, apiClient.Version = getPlatform(), getClientVersion()
	return apiClient
}

// describeUpdateRequired explains a refusal by the server (standalone) or by the Hisar app (app mode) and how to update.
func describeUpdateRequired(err error) (string, bool) {
	if upgradeRequired, isUpgradeRequired := client.AsUpgradeRequired(err); isUpgradeRequired {
		return fmt.Sprintf("update required: the Hisar server needs hisar %s or later, and this is %s.\n"+
			"Download the latest hisar from %s, or on a Mac install the one that comes with the Hisar app (Settings → Developer tools).",
			upgradeRequired.MinimumVersion, version, upgradeRequired.UpdateURL), true
	}
	var responseError *cliprotocol.ResponseError
	if errors.As(err, &responseError) && responseError.Code == cliprotocol.ErrorUpdateRequired {
		return "update required. " + responseError.Message, true
	}
	return "", false
}
