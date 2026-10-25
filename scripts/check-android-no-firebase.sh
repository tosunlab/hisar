#!/bin/bash
# Fails when the Android app's release build depends on Firebase. Its notifications come through the UnifiedPush
# distributor the user installed, and the Hisar server sends them by Web Push, so no Google push service is involved.
set -euo pipefail
cd "$(dirname "$0")/../mobile/android"
dependencies=$(./gradlew --console=plain -q :app:dependencies --configuration releaseRuntimeClasspath)
if grep -n 'com\.google\.firebase' <<<"$dependencies"; then
	echo "check-android-no-firebase: the Android app depends on Firebase (above); Hisar's notifications use UnifiedPush" >&2
	exit 1
fi
echo "check-android-no-firebase: no Firebase in the release build"
