#!/bin/bash
# Installs the pinned Android SDK packages the Android build and tests use into $ANDROID_HOME, and creates the emulators
# hisar-api29, hisar-api34 and hisar-api37 for this machine's architecture when they are missing. Safe to run again.
# --build-only installs only what building and the JVM tests need, as CI does before its own emulator step.
set -euo pipefail
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
bin=$ANDROID_HOME/cmdline-tools/latest/bin
case "$(uname -m)" in
	arm64 | aarch64) abi=arm64-v8a ;;
	*) abi=x86_64 ;;
esac

allEmulator=(
	"hisar-api29 system-images;android-29;google_apis;$abi"
	"hisar-api34 system-images;android-34;google_apis_playstore;$abi"
	"hisar-api37 system-images;android-37.0;google_apis_playstore;$abi"
)
allPackage=("platforms;android-37.0" "build-tools;37.0.0" platform-tools "ndk;30.0.16248370")
if [ "${1:-}" = --build-only ]; then
	allEmulator=()
else
	allPackage+=(emulator)
fi
for emulator in ${allEmulator[@]+"${allEmulator[@]}"}; do
	allPackage+=("${emulator#* }")
done

(yes || true) | "$bin/sdkmanager" --licenses > /dev/null
"$bin/sdkmanager" --install "${allPackage[@]}"

allExisting=$("$bin/avdmanager" list avd -c)
for emulator in ${allEmulator[@]+"${allEmulator[@]}"}; do
	name=${emulator%% *}
	if ! grep -qx "$name" <<<"$allExisting"; then
		echo no | "$bin/avdmanager" create avd -n "$name" -k "${emulator#* }" -d pixel_9
	fi
done
