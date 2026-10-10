#!/usr/bin/env bash
# Builds a universal (Apple silicon + Intel) Hisar.app with Sparkle and wraps it in desktop/bin/Hisar-<version>.dmg.
# Usage: package-desktop-dmg.sh [version]     (default: the version in desktop/build/darwin/Info.plist)
# Needs macOS with Xcode command line tools, Go, Node, wails3 (make install-wails) and dmgbuild (pip install dmgbuild).
# HISAR_SIGNING_IDENTITY signs with a Developer ID instead of ad-hoc (scripts/sign-desktop-app.sh).
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
cd "$scriptDirectory/../desktop"
version=${1:-$(/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" build/darwin/Info.plist)}
buildNumber=$("$scriptDirectory/get-build-number.sh" "$version")
application=bin/Hisar.app

"$scriptDirectory/download-sparkle.sh"
rm -rf "$application"
wails3 task darwin:package:universal

for architecture in amd64 arm64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$architecture go build -trimpath -ldflags="-s -w" -o "bin/hisar-browser-host-$architecture" ./cmd/hisar-browser-host
done
lipo -create -output "$application/Contents/MacOS/hisar-browser-host" bin/hisar-browser-host-amd64 bin/hisar-browser-host-arm64
rm bin/hisar-browser-host-amd64 bin/hisar-browser-host-arm64

/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" -c "Set :CFBundleVersion $buildNumber" "$application/Contents/Info.plist"
test -f "$application/Contents/Frameworks/Sparkle.framework/Versions/Current/Sparkle"
otool -L "$application/Contents/MacOS/Hisar" | grep -q "@rpath/Sparkle.framework/"
otool -l "$application/Contents/MacOS/Hisar" | grep -q "path @executable_path/../Frameworks "
"$scriptDirectory/sign-desktop-app.sh" "$application"
for binary in MacOS/Hisar MacOS/hisar-browser-host Frameworks/Sparkle.framework/Versions/Current/Sparkle; do
  test "$(lipo -archs "$application/Contents/$binary")" = "x86_64 arm64"
done

output=bin/Hisar-$version.dmg
rm -f "$output"
dmgbuild -s build/darwin/dmg-settings.py \
  -D application="$application" -D background=build/darwin/dmg-background.png -D volume_icon=build/darwin/icons.icns \
  "Hisar $version" "$output"
hdiutil verify "$output" >/dev/null
echo "$PWD/$output"
