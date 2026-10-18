#!/usr/bin/env bash
# Runs the Android app's instrumented tests against a freshly started hisar-server (scripts/ui-test-partner.sh), on the one
# connected emulator or device, or on ANDROID_SERIAL. Requires ANDROID_HOME, HISAR_DATABASE_URL pointing at an empty,
# disposable database, and mobile/build/HisarKit.aar (make build-android-aar).
#
# ANDROID_UI_TESTS picks the tests: comma-separated names of classes (HisarKitBackendTests), tests
# (testAccountItemLockAndSignInFlow) or both (HisarUITests#testAccountItemLockAndSignInFlow), or "smoke" for the set the
# older Android versions run (a smoke test this checkout lacks is named and left out). A name that matches
# no test, or a run in which no test ran, fails. HISAR_UI_TEST_SCREENSHOT_DIRECTORY receives the screenshots the tests save.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
ADB=$ANDROID_HOME/platform-tools/adb
SMOKE_TESTS=HisarKitBackendTests,KeystoreSecretKeyStoreTests,BiometricKeystoreTests,ClipboardAdapterTests,testAccountItemLockAndSignInFlow,testBiometricUnlockAfterTurningItOn,AutofillServiceUITests#testWebViewFormFillsAfterUnlock,testRegisterPasskeyThenSignInWithIt,HarnessServerTests

if [ -z "${ANDROID_SERIAL:-}" ]; then
  SERIALS=$("$ADB" devices | awk 'NR > 1 && $2 == "device" {print $1}')
  if [ "$(printf '%s\n' "$SERIALS" | grep -c .)" != 1 ]; then
    echo "connect one emulator or device, or set ANDROID_SERIAL to one of: $(printf '%s ' $SERIALS)" >&2
    exit 1
  fi
  ANDROID_SERIAL=$SERIALS
fi
export ANDROID_SERIAL
"$ADB" wait-for-device shell 'while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 1; done'

UI_TEST_PLATFORM_NAME=Android
UI_TEST_PORT=${ANDROID_UI_TEST_PORT:-18084}
source "$ROOT/scripts/ui-test-partner.sh"
ui_test_add_account SIGN_IN_AGAIN_ "android-sign-in-again-$(date +%s)-$RANDOM@example.com"
ui_test_add_account PLATFORM_BACKEND_ "android-platform-backend-$(date +%s)-$RANDOM@example.com"
ui_test_add_account PLATFORM_BIOMETRIC_ "android-platform-biometric-$(date +%s)-$RANDOM@example.com"
ui_test_add_account PLATFORM_TIME_ZONE_ "android-platform-time-zone-$(date +%s)-$RANDOM@example.com"
ui_test_add_account CREDENTIAL_CHANGE_ "android-credential-change-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_EDITOR_ "android-item-editor-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_EDITOR_EDIT_ "android-item-editor-edit-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_DETAIL_ "android-item-detail-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_CATEGORIES_ "android-item-categories-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_APP_LINK_ "android-item-app-link-$(date +%s)-$RANDOM@example.com"
ui_test_add_account QR_SCANNER_ "android-qr-scanner-$(date +%s)-$RANDOM@example.com"
# The phone approval test signs in to an account whose Mac is played by hisar-ui-test-mac: it signs up first, then sends
# the phone its requests and saves each answer it verified in the Personal vault.
PHONE_APPROVAL_EMAIL="android-phone-approval-$(date +%s)-$RANDOM@example.com"
PHONE_APPROVAL_PASSWORD=phone-approval-master-password
(cd "$ROOT/mobile" && go build -o "$ROOT/bin/hisar-ui-test-mac" ./cmd/hisar-ui-test-mac)
"$ROOT/bin/hisar-ui-test-mac" -invite "$(ui_test_create_invite "$PHONE_APPROVAL_EMAIL")" -email "$PHONE_APPROVAL_EMAIL" \
  -password "$PHONE_APPROVAL_PASSWORD" -state "$T/mac" -secret-key-file "$T/mac-secret-key" >"$T/mac.log" 2>&1 &
UI_TEST_MAC=$!
trap 'kill "$UI_TEST_MAC" 2>/dev/null || true; ui_test_stop' EXIT
for _ in $(seq 60); do [ -s "$T/mac-secret-key" ] || ! kill -0 "$UI_TEST_MAC" 2>/dev/null && break; sleep 1; done
[ -s "$T/mac-secret-key" ] || { echo "hisar-ui-test-mac did not sign up:" >&2; cat "$T/mac.log" >&2; exit 1; }
UI_TEST_VALUES+=(
  "HISAR_UI_TEST_PHONE_APPROVAL_EMAIL=$PHONE_APPROVAL_EMAIL"
  "HISAR_UI_TEST_PHONE_APPROVAL_PASSWORD=$PHONE_APPROVAL_PASSWORD"
  "HISAR_UI_TEST_PHONE_APPROVAL_SECRET_KEY=$(cat "$T/mac-secret-key")"
)
# The biometric tests enroll fingers and touch the sensor through the emulator's console.
if [[ "$ANDROID_SERIAL" == emulator-* ]] && [ -r "$HOME/.emulator_console_auth_token" ]; then
  UI_TEST_VALUES+=("HISAR_UI_TEST_EMULATOR_CONSOLE_PORT=${ANDROID_SERIAL#emulator-}" "HISAR_UI_TEST_EMULATOR_CONSOLE_TOKEN=$(cat "$HOME/.emulator_console_auth_token")")
fi
"$ADB" reverse "tcp:$UI_TEST_PORT" "tcp:$UI_TEST_PORT" >/dev/null
trap '"$ADB" reverse --remove "tcp:$UI_TEST_PORT" >/dev/null 2>&1 || true; kill "$UI_TEST_MAC" 2>/dev/null || true; ui_test_stop' EXIT

# The Android Gradle plugin cuts each value at its first comma and hands the rest to the device's shell unquoted, inside
# `am instrument -e NAME value`.
for VALUE in "${UI_TEST_VALUES[@]}"; do
  [[ "$VALUE" =~ ^[A-Za-z0-9_]+=[A-Za-z0-9@._:/+=-]*$ ]] || { echo "cannot pass $VALUE as an instrumentation argument" >&2; exit 1; }
done
GRADLE_ARGUMENTS=("${UI_TEST_VALUES[@]/#/-Pandroid.testInstrumentationRunnerArguments.}")

# resolve_test prints package.Class, or package.Class#test for each class with that test, separated by |, for a class name,
# a test name or both, from the instrumented test sources.
resolve_test() {
  local class=${1%%#*} test= file files
  case "$1" in
    *#*) test=${1#*#} ;;
    test*) class= test=$1 ;;
  esac
  if [ -n "$class" ]; then
    files=$(find "$ROOT"/mobile/android/*/src/androidTest -name "${class##*.}.kt")
  else
    files=$(grep -rlE "fun $test\(" "$ROOT"/mobile/android/*/src/androidTest || true)
  fi
  for file in $files; do
    printf '%s.%s%s\n' "$(sed -n 's/^package //p' "$file")" "$(basename "$file" .kt)" "${test:+#$test}"
  done | paste -sd'|' -
}
TESTS=${ANDROID_UI_TESTS:-}
if [ "$TESTS" = smoke ]; then TESTS=$SMOKE_TESTS; fi
if [ -n "$TESTS" ]; then
  SELECTED=
  for NAME in $(printf '%s' "$TESTS" | tr ',' ' '); do
    RESOLVED=$(resolve_test "$NAME")
    if [ -z "$RESOLVED" ] && [ "$ANDROID_UI_TESTS" = smoke ]; then
      echo "the smoke test $NAME is not in this checkout" >&2
      continue
    fi
    [ -n "$RESOLVED" ] || { echo "no instrumented test is named $NAME" >&2; exit 1; }
    SELECTED=$SELECTED${SELECTED:+|}$RESOLVED
  done
  [ -n "$SELECTED" ] || { echo "none of $TESTS is in this checkout" >&2; exit 1; }
  # The runner matches tests_regex against package.Class#test; `class` would lose every name after the first comma. The
  # quotes keep the regex whole in the device's shell.
  GRADLE_ARGUMENTS+=("-Pandroid.testInstrumentationRunnerArguments.tests_regex='^(${SELECTED//./\\.})(#|\$)'")
fi

OUTPUT=$ROOT/mobile/android/app/build/outputs/connected_android_test_additional_output
RESULTS=$ROOT/mobile/android/app/build/outputs/androidTest-results/connected
rm -rf "$OUTPUT" "$RESULTS"
STATUS=0
(cd "$ROOT/mobile/android" && ./gradlew --console=plain connectedDebugAndroidTest "${GRADLE_ARGUMENTS[@]}") || STATUS=$?
# Gradle passes a run whose test runner crashed before it found any test, or whose ANDROID_UI_TESTS matched none.
if [ "$STATUS" = 0 ] && ! grep -rqsE '<testsuites? [^>]*tests="[1-9]' "$RESULTS"; then
  echo "no test ran; see the device's logcat" >&2
  STATUS=1
fi
if [ -n "${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" ] && [ -d "$OUTPUT" ]; then
  find "$OUTPUT" -name 'android-*.png' -exec cp {} "$HISAR_UI_TEST_SCREENSHOT_DIRECTORY" \;
fi
if [ "$STATUS" != 0 ]; then
  ui_test_print_logs
  echo "--- hisar-ui-test-mac log" >&2
  cat "$T/mac.log" >&2
  exit "$STATUS"
fi
