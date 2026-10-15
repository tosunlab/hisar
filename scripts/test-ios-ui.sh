#!/usr/bin/env bash
# Runs the iOS XCUITests in the simulator against a freshly started hisar-server.
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database, bin/hisar-server and bin/hisar (make build)
# and mobile/build/HisarKit.xcframework (make build-ios-framework).
#
# The test classes run on IOS_UI_TEST_WORKERS clones of the simulator at once (2 by default; 1 runs them one by one): each
# class signs up its own account. IOS_TESTS_PREBUILT=1 runs the tests make build-ios built instead of building them again.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d)
PORT=${IOS_UI_TEST_PORT:-18082}
export HISAR_PUBLIC_URL=http://127.0.0.1:$PORT
export HISAR_SECRETS_FILE=$T/secrets.json
export HISAR_AUTH_BURST=1000
# The partner account below uses the standalone CLI; a Hisar app running on this machine must not answer it or start.
export HISAR_STANDALONE=1 HISAR_NO_LAUNCH=1
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

"$ROOT/bin/hisar-server" serve -addr "127.0.0.1:$PORT" >"$T/server.log" 2>&1 &
SERVER=$!
PARTNER_LOOP=
trap 'kill $SERVER $PARTNER_LOOP 2>/dev/null || true; rm -rf "$T"' EXIT
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
PASSKEY_INVITE_LINK=$(create_invite ios-passkey@example.com)
SHARING_EMAIL="ios-sharing-$(date +%s)-$RANDOM@example.com"
SHARING_INVITE_LINK=$(create_invite "$SHARING_EMAIL")
BANNER_EMAIL="ios-banner-$(date +%s)-$RANDOM@example.com"
BANNER_INVITE_LINK=$(create_invite "$BANNER_EMAIL")
LIFECYCLE_EMAIL="ios-lifecycle-$(date +%s)-$RANDOM@example.com"
LIFECYCLE_INVITE_LINK=$(create_invite "$LIFECYCLE_EMAIL")

# A partner on the command line owns "Infrastructure", shared with the banner account before it exists, and
# accepts every share invitation it gets and completes every share it sent, as their own Hisar would.
PARTNER_EMAIL="ios-partner-$(date +%s)-$RANDOM@example.com"
PARTNER_PASSWORD='partner master password'
printf '%s\niOS Partner\n%s\n%s\n' "$PARTNER_EMAIL" "$PARTNER_PASSWORD" "$PARTNER_PASSWORD" |
  HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=- "$ROOT/bin/hisar" signup "$(create_invite "$PARTNER_EMAIL")" 2>"$T/partner-signup.txt"
PARTNER_SECRET_KEY=$(grep -oE 'H1-[0-9A-Z-]+' "$T/partner-signup.txt")
partner() { HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=$PARTNER_SECRET_KEY "$ROOT/bin/hisar" "$@"; }
printf '%s\n' "$PARTNER_PASSWORD" | partner vault-create Infrastructure 2>/dev/null
printf '%s\nStaging database\ndeploy\nhttps://staging.example.com\nstaging-database-password\n\n' "$PARTNER_PASSWORD" | partner add -v Infrastructure 2>/dev/null
printf '%s\n' "$PARTNER_PASSWORD" | partner share Infrastructure "$BANNER_EMAIL" 2>/dev/null
answer_as_partner() {
  while true; do
    for INVITATION_ID in $(printf '%s\n' "$PARTNER_PASSWORD" | partner invitations 2>/dev/null | awk '/^RECEIVED/{isReceived=1; next} /^(SENT|$)/{isReceived=0} isReceived{print $1}'); do
      partner invitations accept "$INVITATION_ID" </dev/null >/dev/null 2>&1 || true
    done
    sleep 2
  done
}
answer_as_partner >"$T/partner.log" 2>&1 &
PARTNER_LOOP=$!

SIMULATOR_ID=${IOS_SIMULATOR_ID:-$("$ROOT/mobile/ios/scripts/find-simulator.sh")}
ACTION=test
if [ -n "${IOS_TESTS_PREBUILT:-}" ]; then ACTION=test-without-building; fi
WORKERS=${IOS_UI_TEST_WORKERS:-2}
PARALLEL=(-parallel-testing-enabled NO)
if [ "$WORKERS" -gt 1 ]; then PARALLEL=(-parallel-testing-enabled YES -parallel-testing-worker-count "$WORKERS"); fi

if ! TEST_RUNNER_HISAR_UI_TEST_SERVER="$HISAR_PUBLIC_URL" \
  TEST_RUNNER_HISAR_UI_TEST_INVITE_LINK="$INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_AUTOFILL_INVITE_LINK="$AUTOFILL_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_BIOMETRIC_INVITE_LINK="$BIOMETRIC_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_ONE_TIME_CODE_INVITE_LINK="$ONE_TIME_CODE_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_PASSKEY_INVITE_LINK="$PASSKEY_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_SHARING_INVITE_LINK="$SHARING_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_SHARING_EMAIL="$SHARING_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_SHARING_PARTNER_EMAIL="$PARTNER_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_BANNER_INVITE_LINK="$BANNER_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_BANNER_EMAIL="$BANNER_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_LIFECYCLE_INVITE_LINK="$LIFECYCLE_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_LIFECYCLE_EMAIL="$LIFECYCLE_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_SCREENSHOT_DIRECTORY="${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" \
  xcodebuild "$ACTION" -project "$ROOT/mobile/ios/Hisar.xcodeproj" -scheme Hisar -only-testing:"${IOS_UI_TESTS:-HisarUITests}" \
  -destination "id=$SIMULATOR_ID" "${PARALLEL[@]}" ${IOS_DERIVED_DATA:+-derivedDataPath "$IOS_DERIVED_DATA"} ${IOS_RESULT_BUNDLE:+-resultBundlePath "$IOS_RESULT_BUNDLE"}; then
  echo "server log:" >&2
  cat "$T/server.log" >&2
  exit 1
fi
