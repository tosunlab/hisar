#!/bin/bash
# Builds HisarKit.xcframework (iPhone and simulator) from ./hisarkit with gomobile.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$(go env GOPATH)/bin:$PATH"
export GOWORK=off
mkdir -p build
gomobile bind -target=ios,iossimulator -iosversion=17.0 -trimpath -ldflags="-s -w" -o build/HisarKit.xcframework ./hisarkit
echo "built $(pwd)/build/HisarKit.xcframework"
