#!/usr/bin/env bash
# Signs Hisar.app inside out: Sparkle's helpers, Sparkle.framework, the browser host, the hisar CLI, then the app.
# Usage: sign-desktop-app.sh <path to Hisar.app>
# HISAR_SIGNING_IDENTITY: a team identity, by name or SHA-1 (Developer ID Application for releases; signs with the
# hardened runtime and a timestamp); default ad-hoc (-).
set -euo pipefail

application=${1:?usage: sign-desktop-app.sh <path to Hisar.app>}
identity=${HISAR_SIGNING_IDENTITY:--}
allOption=(--force --sign "$identity")
appOption=()
entitlements=""
# The entitlements file is removed without an EXIT trap, because macOS's bash 3.2 exits 0 from a script that has one
# after a set -u error.
# Ad-hoc code has no Team ID, so the hardened runtime's library validation would refuse to load Sparkle.
if [ "$identity" != "-" ]; then
  allOption+=(--options runtime --timestamp)
  profile=${HISAR_MACOS_PROVISIONING_PROFILE:?signed Mac builds require HISAR_MACOS_PROVISIONING_PROFILE for OS-protected Touch ID}
  entitlements=$(mktemp)
  python3 "$(dirname "$0")/desktop-keychain-entitlements.py" "$profile" "$application/Contents/Info.plist" "$entitlements" "$identity"
  cp "$profile" "$application/Contents/embedded.provisionprofile"
  appOption+=(--entitlements "$entitlements")
fi

framework="$application/Contents/Frameworks/Sparkle.framework"
if [ -d "$framework" ]; then
  frameworkVersion="$framework/Versions/$(readlink "$framework/Versions/Current")"
  codesign "${allOption[@]}" "$frameworkVersion/XPCServices/Installer.xpc"
  codesign "${allOption[@]}" --preserve-metadata=entitlements "$frameworkVersion/XPCServices/Downloader.xpc"
  codesign "${allOption[@]}" "$frameworkVersion/Autoupdate"
  codesign "${allOption[@]}" "$frameworkVersion/Updater.app"
  codesign "${allOption[@]}" "$framework"
fi
if [ -f "$application/Contents/MacOS/hisar-browser-host" ]; then
  codesign "${allOption[@]}" "$application/Contents/MacOS/hisar-browser-host"
fi
if [ -f "$application/Contents/Helpers/hisar" ]; then
  codesign "${allOption[@]}" --identifier com.tryhisar.hisar.cli "$application/Contents/Helpers/hisar"
fi
codesign "${allOption[@]}" ${appOption[@]+"${appOption[@]}"} "$application"
if [ -n "$entitlements" ]; then rm -f "$entitlements"; fi
codesign --verify --deep --strict "$application"
