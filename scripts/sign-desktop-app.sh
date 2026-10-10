#!/usr/bin/env bash
# Signs Hisar.app inside out: Sparkle's helpers, Sparkle.framework, the browser host, the hisar CLI, then the app.
# Usage: sign-desktop-app.sh <path to Hisar.app>
# HISAR_SIGNING_IDENTITY: a Developer ID identity (signs with the hardened runtime and a timestamp); default ad-hoc (-).
set -euo pipefail

application=${1:?usage: sign-desktop-app.sh <path to Hisar.app>}
identity=${HISAR_SIGNING_IDENTITY:--}
allOption=(--force --sign "$identity")
# Ad-hoc code has no Team ID, so the hardened runtime's library validation would refuse to load Sparkle.
if [ "$identity" != "-" ]; then
  allOption+=(--options runtime --timestamp)
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
codesign "${allOption[@]}" "$application"
codesign --verify --deep --strict "$application"
