//go:build darwin && !ios && cgo

package appinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createSignedTestBundle(t *testing.T) (string, string) {
	t.Helper()
	bundlePath := filepath.Join(t.TempDir(), "Fixture.app")
	resourcePath := filepath.Join(bundlePath, "Contents", "Resources", "nested", "resource")
	executablePath := filepath.Join(bundlePath, "Contents", "MacOS", "fixture")
	for _, directory := range []string{filepath.Dir(resourcePath), filepath.Dir(executablePath)} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string][]byte{
		executablePath: executable,
		resourcePath:   []byte("original"),
		filepath.Join(bundlePath, "Contents", "Info.plist"): []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>com.example.signature-fixture</string>
<key>CFBundleExecutable</key><string>fixture</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>`),
	} {
		if err := os.WriteFile(path, contents, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", "--timestamp=none", bundlePath).CombinedOutput(); err != nil {
		t.Fatalf("sign fixture: %v: %s", err, output)
	}
	return bundlePath, resourcePath
}
