#!/usr/bin/env bash
# Builds a universal (Apple silicon + Intel) Hisar.app with Sparkle, the hisar CLI and the omp plugin, and wraps it in
# desktop/bin/Hisar-<version>.dmg.
# Usage: package-desktop-dmg.sh [version]     (default: the version in desktop/build/darwin/Info.plist)
# Needs macOS with Xcode command line tools, Go, Node, wails3 (make install-wails) and dmgbuild (pip install dmgbuild).
# HISAR_SIGNING_IDENTITY signs with a team identity instead of ad-hoc (scripts/sign-desktop-app.sh). With HISAR_NOTARY_KEY
# as well (and HISAR_NOTARY_KEY_ID, HISAR_NOTARY_ISSUER_ID), the app is notarized and stapled before the DMG and the
# Sparkle zip are made from it, and the DMG is signed, notarized and stapled too (scripts/notarize-desktop.sh).
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
GOTOOLCHAIN=$(bash "$scriptDirectory/go-toolchain.sh")
export GOTOOLCHAIN
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

for architecture in amd64 arm64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$architecture go build -trimpath -ldflags="-s -w -X main.version=$version" -o "bin/hisar-$architecture" ../cli/cmd/hisar
done
mkdir -p "$application/Contents/Helpers"
lipo -create -output "$application/Contents/Helpers/hisar" bin/hisar-amd64 bin/hisar-arm64
rm bin/hisar-amd64 bin/hisar-arm64
test "$("$application/Contents/Helpers/hisar" --version)" = "hisar $version"

# Settings → Coding agents links omp to this folder. The plugin imports only types from omp, so it needs no node_modules.
# omp starts mcp.json's server with bin/run-hisar, which runs this app's hisar when it sits in Contents/Resources/omp-plugin.
ompPlugin=$application/Contents/Resources/omp-plugin
rm -rf "$ompPlugin"
mkdir -p "$ompPlugin"
cp -R ../omp/package.json ../omp/mcp.json ../omp/bin ../omp/src ../omp/skills "$ompPlugin/"
test -f "$ompPlugin/package.json"
test -f "$ompPlugin/mcp.json"
test -x "$ompPlugin/bin/run-hisar"

/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" -c "Set :CFBundleVersion $buildNumber" "$application/Contents/Info.plist"
test -f "$application/Contents/Frameworks/Sparkle.framework/Versions/Current/Sparkle"
otool -L "$application/Contents/MacOS/Hisar" | grep "@rpath/Sparkle.framework/" >/dev/null
otool -l "$application/Contents/MacOS/Hisar" | grep "path @executable_path/../Frameworks " >/dev/null
"$scriptDirectory/sign-desktop-app.sh" "$application"
for binary in MacOS/Hisar MacOS/hisar-browser-host Helpers/hisar Frameworks/Sparkle.framework/Versions/Current/Sparkle; do
  test "$(lipo -archs "$application/Contents/$binary")" = "x86_64 arm64"
done
isNotarizing=false
if [ -n "${HISAR_NOTARY_KEY:-}" ]; then
  test "${HISAR_SIGNING_IDENTITY:--}" != - || { echo "notarizing needs HISAR_SIGNING_IDENTITY (a Developer ID Application identity)" >&2; exit 1; }
  isNotarizing=true
  "$scriptDirectory/notarize-desktop.sh" "$application"
fi

output=bin/Hisar-$version.dmg
rm -f "$output"
dmgbuild -s build/darwin/dmg-settings.py \
  -D application="$application" -D background=build/darwin/dmg-background.png -D volume_icon=build/darwin/icons.icns \
  "Hisar $version" "$output"
hdiutil verify "$output" >/dev/null
if $isNotarizing; then
  codesign --force --sign "$HISAR_SIGNING_IDENTITY" --timestamp "$output"
  "$scriptDirectory/notarize-desktop.sh" "$output"
fi
echo "$PWD/$output"
