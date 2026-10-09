//go:build darwin && cgo

package appinfo

import (
	"strings"
	"testing"
)

func TestDescribeBundleOnMacOS(t *testing.T) {
	app := describeBundle("/System/Applications/Utilities/Terminal.app")
	if app.Name != "Terminal" || !strings.HasPrefix(app.IconURL, "data:image/png;base64,") || len(app.IconURL) < 1000 {
		t.Errorf("Terminal: name %q, icon %d bytes", app.Name, len(app.IconURL))
	}
}
