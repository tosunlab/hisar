#!/bin/bash
# Runs the Android app's instrumented tests on the one booted emulator or device, or on ANDROID_SERIAL when several are
# connected. Needs ANDROID_HOME and mobile/build/HisarKit.aar (make build-android-aar).
set -euo pipefail
cd "$(dirname "$0")/../mobile/android"
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
./gradlew --console=plain connectedDebugAndroidTest
