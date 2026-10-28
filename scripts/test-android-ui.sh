#!/usr/bin/env bash
# Runs the Android app's instrumented tests against a freshly started hisar-server (scripts/ui-test-partner.sh), on the one
# connected emulator or device, or on ANDROID_SERIAL. Requires ANDROID_HOME, HISAR_DATABASE_URL pointing at an empty,
# disposable database, mobile/build/HisarKit.aar (make build-android-aar) and openssl.
#
# ANDROID_UI_TESTS picks the tests: comma-separated names of classes (HisarKitBackendTests), tests
# (testAccountItemLockAndSignInFlow) or both (HisarUITests#testAccountItemLockAndSignInFlow), or "smoke" for the set the
# older Android versions run (a smoke test this checkout lacks is named and left out). A name that matches
# no test, or a run in which no test ran, fails. HISAR_UI_TEST_SCREENSHOT_DIRECTORY receives the screenshots the tests save.
# ANDROID_UI_TEST_PART=1, 2 or 3 runs one of three parts of similar length, for three CI jobs, as scripts/ui-test-parts.sh
# splits the classes; part 3 also runs every class new to its table.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
ADB=$ANDROID_HOME/platform-tools/adb
SMOKE_TESTS=HisarKitBackendTests,KeystoreSecretKeyStoreTests,BiometricKeystoreTests,ClipboardAdapterTests,testAccountItemLockAndSignInFlow,testBiometricUnlockAfterTurningItOn,AutofillServiceUITests#testWebViewFormFillsAfterUnlock,testRegisterPasskeyThenSignInWithIt,HarnessServerTests,testAppSwitcherHidesAnOpenSheet
# The runner matches tests_regex against package.Class#test: parts 1 and 2 name their classes, and part 3 leaves out theirs.
source "$ROOT/scripts/ui-test-parts.sh"
TESTS_REGEX=
case "${ANDROID_UI_TEST_PART:-}" in
  "") ;;
  1 | 2)
    ui_test_part "$ANDROID_UI_TEST_PART" 3 "${ANDROID_UI_TEST_SECONDS[@]}"
    TESTS_REGEX="^com\\.tryhisar\\.hisar\\.($(IFS='|'; echo "${UI_TEST_PART_CLASSES[*]//./\\.}"))#"
    ;;
  3)
    ui_test_part 3 3 "${ANDROID_UI_TEST_SECONDS[@]}"
    TESTS_REGEX="^(?!com\\.tryhisar\\.hisar\\.($(IFS='|'; echo "${UI_TEST_PART_CLASSES[*]//./\\.}"))#)"
    ;;
  *) echo "ANDROID_UI_TEST_PART is 1, 2 or 3" >&2; exit 1 ;;
esac
if [ -n "${ANDROID_UI_TEST_PART:-}" ] && [ -n "${ANDROID_UI_TESTS:-}" ]; then
  echo "set ANDROID_UI_TESTS or ANDROID_UI_TEST_PART, not both" >&2
  exit 1
fi

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
DEVICE_LOG=$ROOT/mobile/android/app/build/outputs/androidTest-results/device-logcat.txt
SYSTEM_SERVER_AT_START=$("$ADB" shell pidof system_server | tr -d '\r')
# adb waits forever on an emulator that hung, so the calls after a failed run give up after a while instead of holding the job
# until its timeout. The timer's output goes to /dev/null so that $(...) does not wait for it.
within() {
  local seconds=$1 adb status=0
  shift
  "$ADB" "$@" &
  adb=$!
  (sleep "$seconds"; kill "$adb" 2>/dev/null) >/dev/null 2>&1 &
  wait "$adb" || status=$?
  kill $! 2>/dev/null || true
  return "$status"
}
# A failed run keeps the device's whole log in the uploaded results, and says whether Android's framework restarted.
save_device_log() {
  [ "$1" != 0 ] || return 0
  mkdir -p "$(dirname "$DEVICE_LOG")"
  within 120 logcat -d -b all >"$DEVICE_LOG" 2>&1 || true
  echo "--- system_server was process $SYSTEM_SERVER_AT_START at the start and is $(within 20 shell pidof system_server | tr -d '\r') now; the device's log is in $DEVICE_LOG" >&2
  echo "--- device crash log" >&2
  within 60 logcat -d -b crash 2>&1 | tail -n 80 >&2 || true
}

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
ui_test_add_account IMPORT_CANCEL_ "android-import-cancel-$(date +%s)-$RANDOM@example.com"
ui_test_add_account GENERATOR_SHEET_ "android-generator-sheet-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_DETAIL_ "android-item-detail-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_CATEGORIES_ "android-item-categories-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ITEM_APP_LINK_ "android-item-app-link-$(date +%s)-$RANDOM@example.com"
ui_test_add_account QR_SCANNER_ "android-qr-scanner-$(date +%s)-$RANDOM@example.com"
ui_test_add_account PASSKEY_UV_ "android-passkey-uv-$(date +%s)-$RANDOM@example.com"
ui_test_add_account IDLE_LOCK_ "android-idle-lock-$(date +%s)-$RANDOM@example.com"
ui_test_add_account VAULT_MOVE_ "android-vault-move-$(date +%s)-$RANDOM@example.com"
ui_test_add_account PASSWORD_SEARCH_ "android-password-search-$(date +%s)-$RANDOM@example.com"
ui_test_add_account PRIVACY_ "android-privacy-$(date +%s)-$RANDOM@example.com"
ui_test_add_account FILE_HANDOFF_EXPORT_ "android-file-handoff-export-$(date +%s)-$RANDOM@example.com"
ui_test_add_account FILE_HANDOFF_KIT_ "android-file-handoff-kit-$(date +%s)-$RANDOM@example.com"
ui_test_add_account FILE_HANDOFF_FAILURE_ "android-file-handoff-failure-$(date +%s)-$RANDOM@example.com"
for account in WEB CODE APP SAVE SAVE_CODE RECENTS; do
  ui_test_add_account "AUTOFILL_SERVICE_${account}_" "android-autofill-service-$(echo "$account" | tr A-Z_ a-z-)-$(date +%s)-$RANDOM@example.com"
done
# The phone approval test signs in to an account whose Mac is played by hisar-ui-test-mac: it signs up first, then sends
# the phone its requests and saves each answer it verified in the Personal vault.
PHONE_APPROVAL_EMAIL="android-phone-approval-$(date +%s)-$RANDOM@example.com"
PHONE_APPROVAL_PASSWORD=phone-approval-master-password
(cd "$ROOT/mobile" && go build -o "$ROOT/bin/hisar-ui-test-mac" ./cmd/hisar-ui-test-mac)
"$ROOT/bin/hisar-ui-test-mac" -invite "$(ui_test_create_invite "$PHONE_APPROVAL_EMAIL")" -email "$PHONE_APPROVAL_EMAIL" \
  -password "$PHONE_APPROVAL_PASSWORD" -state "$T/mac" -secret-key-file "$T/mac-secret-key" >"$T/mac.log" 2>&1 &
UI_TEST_MAC=$!
trap 'save_device_log $?; ui_test_stop "$UI_TEST_MAC"' EXIT
for _ in $(seq 60); do [ -s "$T/mac-secret-key" ] || ! kill -0 "$UI_TEST_MAC" 2>/dev/null && break; sleep 1; done
[ -s "$T/mac-secret-key" ] || { echo "hisar-ui-test-mac did not sign up:" >&2; cat "$T/mac.log" >&2; exit 1; }
UI_TEST_VALUES+=(
  "HISAR_UI_TEST_PHONE_APPROVAL_EMAIL=$PHONE_APPROVAL_EMAIL"
  "HISAR_UI_TEST_PHONE_APPROVAL_PASSWORD=$PHONE_APPROVAL_PASSWORD"
  "HISAR_UI_TEST_PHONE_APPROVAL_SECRET_KEY=$(cat "$T/mac-secret-key")"
)
# The cross-device test signs in to an account whose other device is the command line, which signs up here.
# answer_as_other_device then plays that device: each step runs once the phone did what the step waits for, through the
# server. The phone saves the notes "Ready for a favorite" and "Ready to be signed out" once it watches for those steps.
CROSS_DEVICE_EMAIL="android-cross-device-$(date +%s)-$RANDOM@example.com"
CROSS_DEVICE_PASSWORD=cross-device-master-password
CROSS_DEVICE_NEW_PASSWORD=cross-device-new-master-password
printf '%s\nCommand Line\n%s\n%s\n' "$CROSS_DEVICE_EMAIL" "$CROSS_DEVICE_PASSWORD" "$CROSS_DEVICE_PASSWORD" |
  HISAR_STATE=$T/other-device.json HISAR_SECRET_KEY=- "$ROOT/bin/hisar" signup -y -o "$T/other-device-emergency-kit.pdf" "$(ui_test_create_invite "$CROSS_DEVICE_EMAIL")" 2>"$T/other-device-signup.txt"
CROSS_DEVICE_SECRET_KEY=$(grep -oE 'H1-[0-9A-Z-]+' "$T/other-device-signup.txt")
other_device() { HISAR_STATE=$T/other-device.json HISAR_SECRET_KEY=$CROSS_DEVICE_SECRET_KEY "$ROOT/bin/hisar" "$@"; }
answer_as_other_device() {
  local password=$CROSS_DEVICE_PASSWORD items devices
  while sleep 1; do
    # The phone's new master password signs this device out; it signs in again with it, as its user would.
    if ! items=$(printf '%s\n' "$password" | other_device ls 2>&1); then
      if [[ "$items" == *"this device was signed out"* ]] && printf '\n\n%s\n' "$CROSS_DEVICE_NEW_PASSWORD" | other_device login; then
        password=$CROSS_DEVICE_NEW_PASSWORD
        touch "$T/other-device-signed-out"
      fi
      continue
    fi
    devices=$(other_device devices 2>&1) || continue
    # It edits the phone's login only after reading the password the phone gave it.
    if [ ! -e "$T/other-device-edited" ] && [[ "$devices" == *" android/mobile "* ]] &&
      [ "$(printf '%s\n' "$password" | other_device get "Cross-device login" -f password 2>/dev/null)" = android-login-password ] &&
      { [[ "$items" == *"Server lists android/mobile"* ]] || printf '%s\nServer lists android/mobile\n.\n' "$password" | other_device add --type secure-note; } &&
      printf '%s\n\ncli-user\n\ncli-login-password\n\n' "$password" | other_device edit "Cross-device login"; then
      touch "$T/other-device-edited"
    fi
    if [ ! -e "$T/other-device-starred" ] && [[ "$items" == *"Ready for a favorite"* ]] &&
      printf '%s\n' "$password" | other_device favorite "Cross-device login"; then
      touch "$T/other-device-starred"
    fi
    if [ -e "$T/other-device-signed-out" ] && [[ "$items" != *"Signed out on the command line"* ]]; then
      printf '%s\nSigned out on the command line\n.\n' "$password" | other_device add --type secure-note || true
    fi
    if [ ! -e "$T/other-device-revoked" ] && [[ "$items" == *"Ready to be signed out"* ]] &&
      other_device revoke-device "$(printf '%s\n' "$devices" | awk '/ android\/mobile / {print $NF}')"; then
      touch "$T/other-device-revoked"
    fi
  done
}
answer_as_other_device >"$T/other-device.log" 2>&1 &
UI_TEST_OTHER_DEVICE=$!
trap 'save_device_log $?; ui_test_stop "$UI_TEST_MAC" "$UI_TEST_OTHER_DEVICE"' EXIT
CROSS_DEVICE_CLI_SYSTEM=$(uname -s)
[ "$CROSS_DEVICE_CLI_SYSTEM" != Darwin ] || CROSS_DEVICE_CLI_SYSTEM=macOS
UI_TEST_VALUES+=(
  "HISAR_UI_TEST_CROSS_DEVICE_EMAIL=$CROSS_DEVICE_EMAIL"
  "HISAR_UI_TEST_CROSS_DEVICE_PASSWORD=$CROSS_DEVICE_PASSWORD"
  "HISAR_UI_TEST_CROSS_DEVICE_NEW_PASSWORD=$CROSS_DEVICE_NEW_PASSWORD"
  "HISAR_UI_TEST_CROSS_DEVICE_SECRET_KEY=$CROSS_DEVICE_SECRET_KEY"
  "HISAR_UI_TEST_CROSS_DEVICE_CLI_NAME=$(other_device devices | awk '/\(this device\)/ {print $1}')"
  "HISAR_UI_TEST_CROSS_DEVICE_CLI_SYSTEM=$CROSS_DEVICE_CLI_SYSTEM"
)
# The biometric tests enroll fingers and touch the sensor through the emulator's console.
if [[ "$ANDROID_SERIAL" == emulator-* ]] && [ -r "$HOME/.emulator_console_auth_token" ]; then
  UI_TEST_VALUES+=("HISAR_UI_TEST_EMULATOR_CONSOLE_PORT=${ANDROID_SERIAL#emulator-}" "HISAR_UI_TEST_EMULATOR_CONSOLE_TOKEN=$(cat "$HOME/.emulator_console_auth_token")")
fi
"$ADB" reverse "tcp:$UI_TEST_PORT" "tcp:$UI_TEST_PORT" >/dev/null
trap 'save_device_log $?; within 20 reverse --remove "tcp:$UI_TEST_PORT" >/dev/null 2>&1 || true; ui_test_stop "$UI_TEST_MAC" "$UI_TEST_OTHER_DEVICE"' EXIT

# TrustedCertificatesTests serves HTTPS on the device with a certificate for 127.0.0.1 from a private CA made for this run
# alone, and reads both from the test storage's input files.
PRIVATE_CA=$T/private-ca
mkdir -p "$PRIVATE_CA"
EC_KEY=(-newkey ec -pkeyopt ec_paramgen_curve:P-256 -pkeyopt ec_param_enc:named_curve -nodes)
{
  openssl req -x509 "${EC_KEY[@]}" -sha256 -keyout "$PRIVATE_CA/ca-key.pem" -out "$PRIVATE_CA/ca.pem" -days 2 -subj "/CN=Hisar Test Private CA" \
    -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,cRLSign &&
    openssl req "${EC_KEY[@]}" -keyout "$PRIVATE_CA/server-key.pem" -subj /CN=127.0.0.1 |
    openssl x509 -req -sha256 -CA "$PRIVATE_CA/ca.pem" -CAkey "$PRIVATE_CA/ca-key.pem" -set_serial 2 -days 2 -out "$PRIVATE_CA/server.pem" \
      -extfile <(printf 'subjectAltName=IP:127.0.0.1\nbasicConstraints=CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\n')
} 2>"$PRIVATE_CA/openssl.log" || { cat "$PRIVATE_CA/openssl.log" >&2; exit 1; }
# A freshly booted emulator reports boot completed before its shared storage takes writes.
for _ in $(seq 60); do
  "$ADB" shell mkdir -p /sdcard/googletest/test_runfiles/private-ca 2>/dev/null && break
  sleep 2
done
"$ADB" shell mkdir -p /sdcard/googletest/test_runfiles/private-ca || { echo "the device's shared storage never took the test CA" >&2; exit 1; }
"$ADB" push "$PRIVATE_CA/ca.pem" "$PRIVATE_CA/server.pem" "$PRIVATE_CA/server-key.pem" /sdcard/googletest/test_runfiles/private-ca/ >/dev/null

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
  # `class` would lose every name after the first comma.
  TESTS_REGEX="^(${SELECTED//./\\.})(#|\$)"
fi
RECOVERY_TEST=com.tryhisar.hisar.ui.AccountLifecycleUITests#testChangePasswordReplaceSecretKeyAndDeleteAccount
RUN_RECOVERY=false
if python3 -c 'import re, sys; sys.exit(not re.search(sys.argv[1] or ".*", sys.argv[2]))' "$TESTS_REGEX" "$RECOVERY_TEST"; then
  RUN_RECOVERY=true
fi
RECOVERY_ONLY=false
case "${ANDROID_UI_TESTS:-}" in
  AccountLifecycleUITests|AccountLifecycleUITests#testChangePasswordReplaceSecretKeyAndDeleteAccount|testChangePasswordReplaceSecretKeyAndDeleteAccount) RECOVERY_ONLY=true ;;
  "$RECOVERY_TEST"|"${RECOVERY_TEST%#*}") RECOVERY_ONLY=true ;;
esac
TESTS_REGEX="^(?!${RECOVERY_TEST//./\\.}\$)${TESTS_REGEX:-.*}"
# The quotes keep the regex whole in the device's shell.
if [ -n "$TESTS_REGEX" ]; then
  GRADLE_ARGUMENTS+=("-Pandroid.testInstrumentationRunnerArguments.tests_regex='$TESTS_REGEX'")
fi

OUTPUT=$ROOT/mobile/android/app/build/outputs/connected_android_test_additional_output
RESULTS=$ROOT/mobile/android/app/build/outputs/androidTest-results/connected
rm -rf "$OUTPUT" "$RESULTS"
STATUS=0
if "$RECOVERY_ONLY"; then
  (cd "$ROOT/mobile/android" && ./gradlew --console=plain installDebug installDebugAndroidTest) || STATUS=$?
else
  (cd "$ROOT/mobile/android" && ./gradlew --console=plain connectedDebugAndroidTest "${GRADLE_ARGUMENTS[@]}") || STATUS=$?
fi

# Instrumentation shares the app process. These phases preserve app data while force-stop discards every in-memory key.
if "$RUN_RECOVERY"; then
  mkdir -p "$RESULTS/recovery-process-restart"
  if ! "$RECOVERY_ONLY"; then
    (cd "$ROOT/mobile/android" && ./gradlew --console=plain installDebug installDebugAndroidTest) || STATUS=$?
  fi
  SERVICES_APK=$(cd "$ROOT/mobile/android" && ./gradlew --quiet :app:printTestServicesApk)
  [ -f "$SERVICES_APK" ] || { echo "Gradle did not resolve the pinned Android test services APK: $SERVICES_APK" >&2; exit 1; }
  "$ADB" install -r -g "$SERVICES_APK"
  if [ "$("$ADB" shell getprop ro.build.version.sdk | tr -d '\r')" -ge 30 ]; then
    "$ADB" shell appops set androidx.test.services MANAGE_EXTERNAL_STORAGE allow
  fi
  RECOVERY_STATUS=0
  RECOVERY_ARGUMENTS=(-e class "$RECOVERY_TEST" -e useTestStorageService true)
  for VALUE in "${UI_TEST_VALUES[@]}"; do
    RECOVERY_ARGUMENTS+=(-e "${VALUE%%=*}" "${VALUE#*=}")
  done
  "$ADB" shell pm clear com.tryhisar.hisar.debug >/dev/null
  "$ADB" shell pm clear com.tryhisar.hisar.debug.test >/dev/null
  for PHASE in prepare-replacement-kit finish-after-process-restart; do
    PHASE_LOG=$RESULTS/recovery-process-restart/$PHASE.txt
    if ! "$ADB" shell am instrument -w -r "${RECOVERY_ARGUMENTS[@]}" -e HISAR_UI_TEST_RECOVERY_PHASE "$PHASE" \
      com.tryhisar.hisar.debug.test/com.tryhisar.hisar.HisarTestRunner >"$PHASE_LOG" 2>&1 ||
      ! grep -q '^OK (1 test)' "$PHASE_LOG"; then
      cat "$PHASE_LOG" >&2
      RECOVERY_STATUS=1
      break
    fi
    cat "$PHASE_LOG"
    "$ADB" shell am force-stop com.tryhisar.hisar.debug
    if [ -n "$("$ADB" shell pidof com.tryhisar.hisar.debug | tr -d '\r')" ]; then
      echo "the recovery app process survived force-stop" >&2
      RECOVERY_STATUS=1
      break
    fi
  done
  {
    printf '<testsuite name="RecoveryAcrossProcessRestart" tests="1" failures="%s"><testcase name="%s" classname="%s">' \
      "$RECOVERY_STATUS" "${RECOVERY_TEST#*#}" "${RECOVERY_TEST%#*}"
    if [ "$RECOVERY_STATUS" != 0 ]; then
      printf '<failure message="Recovery process-restart phase failed. See prepare-replacement-kit.txt and finish-after-process-restart.txt."/>'
      STATUS=1
    fi
    printf '</testcase></testsuite>\n'
  } >"$RESULTS/recovery-process-restart/TEST-recovery-process-restart.xml"
  if [ -n "${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" ]; then
    mkdir -p "$OUTPUT/recovery-process-restart"
    "$ADB" pull /sdcard/googletest/test_outputfiles/. "$OUTPUT/recovery-process-restart" >/dev/null
  fi
fi
# Gradle passes a run whose test runner crashed before it found any test, or whose ANDROID_UI_TESTS matched none.
if [ "$STATUS" = 0 ] && ! grep -rqsE '<testsuites? [^>]*tests="[1-9]' "$RESULTS"; then
  echo "no test ran" >&2
  STATUS=1
fi
# The test orchestrator lists every test first (one logcat file each); a test whose own run never started reports nothing, and
# Gradle still passes.
MISSING=$(LC_ALL=C comm -23 \
  <(find "$RESULTS" -name 'logcat-*.txt' | sed -E 's|.*/logcat-(.*)-([^-]*)\.txt$|\1#\2|' | LC_ALL=C sort -u) \
  <(grep -rhoE '<testcase name="[^"]*" classname="[^"]*"' "$RESULTS" | sed -E 's/<testcase name="([^"]*)" classname="([^"]*)"/\2#\1/' | LC_ALL=C sort -u))
if [ -n "$MISSING" ]; then
  echo "these tests never reported a result:" $MISSING >&2
  STATUS=1
fi
if [ -n "${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" ] && [ -d "$OUTPUT" ]; then
  find "$OUTPUT" -name 'android-*.png' -exec cp {} "$HISAR_UI_TEST_SCREENSHOT_DIRECTORY" \;
fi
if [ "$STATUS" != 0 ]; then
  ui_test_print_logs
  echo "--- hisar-ui-test-mac log" >&2
  cat "$T/mac.log" >&2
  echo "--- other device (command line) log" >&2
  cat "$T/other-device.log" >&2
  exit "$STATUS"
fi
