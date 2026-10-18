#!/usr/bin/env bash
# Builds Hisar for Linux with hisar-browser-host and the hisar CLI, and packs desktop/bin/Hisar-<version>-linux-<arch>.{deb,rpm}.
# Usage: package-desktop-linux.sh <version>     (for the machine's own architecture; the app needs cgo, so no cross builds)
# Needs Linux with GTK 4 and WebKitGTK 6.0 headers (libgtk-4-dev libwebkitgtk-6.0-dev), a C compiler, Go, Node and
# wails3 (make install-wails), which packs the .deb and .rpm (nfpm, build/linux/nfpm/nfpm.yaml).
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
cd "$scriptDirectory/../desktop"
version=${1:?usage: package-desktop-linux.sh <version>}
architecture=$(go env GOARCH)
test "$(go env GOOS)" = linux || { echo "build the Linux packages on Linux"; exit 1; }

rm -f bin/Hisar bin/hisar-browser-host bin/hisar
(cd frontend && npm ci --no-audit --no-fund && npm run build)
# The flags of wails3 build, plus the version the update notifier compares.
CGO_ENABLED=1 go build -tags production -trimpath -buildvcs=false \
  -ldflags="-w -s -X github.com/tryhisar/hisar/desktop/updater.stampedVersion=$version" -o bin/Hisar .
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/hisar-browser-host ./cmd/hisar-browser-host
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o bin/hisar ../cli/cmd/hisar
test "$(bin/hisar --version)" = "hisar $version"

packageDirectory=$(mktemp -d)
trap 'rm -rf "$packageDirectory"' EXIT
for format in deb rpm; do
  VERSION=$version GOARCH=$architecture wails3 tool package -name Hisar -format "$format" -config build/linux/nfpm/nfpm.yaml -out "$packageDirectory"
  packages=("$packageDirectory"/*."$format")
  test "${#packages[@]}" = 1
  output=bin/Hisar-$version-linux-$architecture.$format
  mv "${packages[0]}" "$output"
  echo "$PWD/$output"
done
