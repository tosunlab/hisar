#!/bin/bash
# Builds HisarKit.aar (arm64-v8a and x86_64) from ./hisarkit with gomobile, then checks its libraries and Java classes.
# No armeabi-v7a: github.com/bytemare/ksf does not compile for 32-bit targets.
set -euo pipefail
cd "$(dirname "$0")"
GOTOOLCHAIN=$(bash ../scripts/go-toolchain.sh)
export GOTOOLCHAIN
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to the Android NDK, such as \$ANDROID_HOME/ndk/30.0.16248370}"
PATH="$(go env GOPATH)/bin:$PATH"
export PATH
export GOWORK=off

GOOS=android GOARCH=arm64 go vet ./hisarkit
mkdir -p build
aar=build/HisarKit.aar
gomobile bind -target=android/arm64,android/amd64 -androidapi 29 -javapkg com.tryhisar -trimpath -ldflags="-s -w" -o "$aar" ./hisarkit

workDirectory=$(mktemp -d)
trap 'rm -rf "$workDirectory"' EXIT
unzip -p "$aar" classes.jar > "$workDirectory/classes.jar"
# grep reads the whole listing: with -q it would stop at the match, and pipefail would then report unzip's SIGPIPE.
unzip -Z1 "$workDirectory/classes.jar" | grep -x com/tryhisar/hisarkit/Hisar.class >/dev/null || { echo "$aar has no com/tryhisar/hisarkit/Hisar.class"; exit 1; }

readelf=$(echo "$ANDROID_NDK_HOME"/toolchains/llvm/prebuilt/*/bin/llvm-readelf)
for abi in arm64-v8a x86_64; do
	library=jni/$abi/libgojni.so
	unzip -p "$aar" "$library" > "$workDirectory/$abi.so" || { echo "$aar has no $library"; exit 1; }
	# Devices with 16 KB pages refuse a library whose LOAD segments are aligned to less.
	if "$readelf" -lW "$workDirectory/$abi.so" | awk '$1 == "LOAD" && $NF != "0x4000" { found = 1 } END { exit !found }'; then
		echo "$library has LOAD segments that are not aligned to 16 KB"
		exit 1
	fi
done
echo "built $(pwd)/$aar"
