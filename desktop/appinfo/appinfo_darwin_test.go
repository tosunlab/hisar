//go:build darwin && cgo

package appinfo

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestDescribeBundleOnMacOS(t *testing.T) {
	app := DescribeBundle("/System/Applications/Utilities/Terminal.app")
	if app.Name != "Terminal" || !strings.HasPrefix(app.IconURL, "data:image/png;base64,") || len(app.IconURL) < 1000 {
		t.Errorf("Terminal: name %q, icon %d bytes", app.Name, len(app.IconURL))
	}
}

func TestReadThisProcessOnMacOS(t *testing.T) {
	allArgument, err := ListAllProcessArgument(os.Getpid())
	if err != nil || !slices.Equal(allArgument, os.Args) {
		t.Errorf("arguments %q, want %q: %v", allArgument, os.Args, err)
	}
	workingDirectory, err := GetProcessWorkingDirectory(os.Getpid())
	expected, _ := os.Getwd()
	if err != nil || workingDirectory != expected {
		t.Errorf("working directory %q, want %q: %v", workingDirectory, expected, err)
	}
	if _, err := GetProcessWorkingDirectory(999999); err == nil {
		t.Error("a missing process has a working directory")
	}
}
