#!/usr/bin/env bash
# Builds a universal (Apple silicon + Intel) Hisar.app and wraps it in desktop/bin/Hisar-<version>.dmg.
# Usage: package-desktop-dmg.sh [version]     (default: the version in desktop/build/darwin/Info.plist)
# Needs macOS with Xcode command line tools, Go, Node, wails3 (make install-wails) and dmgbuild (pip install dmgbuild).
set -euo pipefail

cd "$(dirname "$0")/../desktop"
version=${1:-$(/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" build/darwin/Info.plist)}
application=bin/Hisar.app

rm -rf "$application"
wails3 task darwin:package:universal

for architecture in amd64 arm64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$architecture go build -trimpath -ldflags="-s -w" -o "bin/hisar-browser-host-$architecture" ./cmd/hisar-browser-host
done
lipo -create -output "$application/Contents/MacOS/hisar-browser-host" bin/hisar-browser-host-amd64 bin/hisar-browser-host-arm64
rm bin/hisar-browser-host-amd64 bin/hisar-browser-host-arm64

/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" -c "Set :CFBundleVersion $version" "$application/Contents/Info.plist"
codesign --force --deep --sign - "$application"
codesign --verify --deep --strict "$application"
for binary in Hisar hisar-browser-host; do
  test "$(lipo -archs "$application/Contents/MacOS/$binary")" = "x86_64 arm64"
done

output=bin/Hisar-$version.dmg
rm -f "$output"
dmgbuild -s build/darwin/dmg-settings.py \
  -D application="$application" -D background=build/darwin/dmg-background.png -D volume_icon=build/darwin/icons.icns \
  "Hisar $version" "$output"
hdiutil verify "$output" >/dev/null
echo "$PWD/$output"
