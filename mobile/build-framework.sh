#!/bin/bash
# Builds HisarKit.xcframework (iPhone and simulator) from ./hisarkit with gomobile.
set -euo pipefail
cd "$(dirname "$0")"
GOTOOLCHAIN=$(bash ../scripts/go-toolchain.sh)
export GOTOOLCHAIN
PATH="$(go env GOPATH)/bin:$PATH"
export PATH
export GOWORK=off
# shellcheck source=mobile/fast-test-kdf.sh
source fast-test-kdf.sh
mkdir -p build
gomobile bind ${TAGS[@]+"${TAGS[@]}"} -target=ios,iossimulator -iosversion=26.0 -trimpath -ldflags="-s -w" -o build/HisarKit.xcframework ./hisarkit
echo "built $(pwd)/build/HisarKit.xcframework"
