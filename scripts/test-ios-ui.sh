#!/usr/bin/env bash
# Runs the iOS XCUITests in the simulator against a freshly started hisar-server.
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database, bin/hisar-server (make build)
# and mobile/build/HisarKit.xcframework (make build-ios-framework).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d)
PORT=${IOS_UI_TEST_PORT:-18082}
export HISAR_PUBLIC_URL=http://127.0.0.1:$PORT
export HISAR_SECRETS_FILE=$T/secrets.json
export HISAR_AUTH_BURST=1000
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

"$ROOT/bin/hisar-server" serve -addr "127.0.0.1:$PORT" >"$T/server.log" 2>&1 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true; rm -rf "$T"' EXIT
curl -sf --retry 20 --retry-all-errors --retry-delay 1 "$HISAR_PUBLIC_URL/healthz" >/dev/null

create_invite() {
  local link
  link=$("$ROOT/bin/hisar-server" invite --email "$1" --role admin </dev/null | tail -1)
  [[ "$link" == "$HISAR_PUBLIC_URL/invite/"* ]] || { echo "could not create an invite for $1: $link" >&2; exit 1; }
  echo "$link"
}
INVITE_LINK=$(create_invite ios-ui@example.com)
AUTOFILL_INVITE_LINK=$(create_invite ios-autofill@example.com)
BIOMETRIC_INVITE_LINK=$(create_invite ios-face-id@example.com)
ONE_TIME_CODE_INVITE_LINK=$(create_invite ios-one-time-code@example.com)

SIMULATOR_ID=${IOS_SIMULATOR_ID:-$("$ROOT/mobile/ios/scripts/find-simulator.sh")}

if ! TEST_RUNNER_HISAR_UI_TEST_SERVER="$HISAR_PUBLIC_URL" \
  TEST_RUNNER_HISAR_UI_TEST_INVITE_LINK="$INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_AUTOFILL_INVITE_LINK="$AUTOFILL_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_BIOMETRIC_INVITE_LINK="$BIOMETRIC_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_ONE_TIME_CODE_INVITE_LINK="$ONE_TIME_CODE_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_SCREENSHOT_DIRECTORY="${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" \
  xcodebuild test -project "$ROOT/mobile/ios/Hisar.xcodeproj" -scheme Hisar -only-testing:"${IOS_UI_TESTS:-HisarUITests}" \
  -destination "id=$SIMULATOR_ID" ${IOS_DERIVED_DATA:+-derivedDataPath "$IOS_DERIVED_DATA"} ${IOS_RESULT_BUNDLE:+-resultBundlePath "$IOS_RESULT_BUNDLE"}; then
  echo "server log:" >&2
  cat "$T/server.log" >&2
  exit 1
fi
