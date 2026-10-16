#!/usr/bin/env bash
# Runs the iOS XCUITests in the simulator against a freshly started hisar-server.
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database, bin/hisar-server and bin/hisar (make build)
# and mobile/build/HisarKit.xcframework (make build-ios-framework).
#
# The test classes run one after another on the simulator by default. IOS_UI_TEST_WORKERS=2 (or more) runs them on that many
# clones of the simulator at once, each class signing up its own account; that is a loss on the GitHub macOS runners, where two
# clones starve each other and tests time out, but can be quicker on a Mac with cores to spare. IOS_TESTS_PREBUILT=1 runs the
# tests make build-ios built instead of building them again.
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
FAVORITES_INVITE_LINK=$(create_invite ios-favorites@example.com)
SHARING_EMAIL="ios-sharing-$(date +%s)-$RANDOM@example.com"
SHARING_INVITE_LINK=$(create_invite "$SHARING_EMAIL")
BANNER_EMAIL="ios-banner-$(date +%s)-$RANDOM@example.com"
BANNER_INVITE_LINK=$(create_invite "$BANNER_EMAIL")
LIFECYCLE_EMAIL="ios-lifecycle-$(date +%s)-$RANDOM@example.com"
LIFECYCLE_INVITE_LINK=$(create_invite "$LIFECYCLE_EMAIL")
ORGANIZATION_JOIN_EMAIL="ios-organization-join-$(date +%s)-$RANDOM@example.com"
ORGANIZATION_JOIN_INVITE_LINK=$(create_invite "$ORGANIZATION_JOIN_EMAIL")
ORGANIZATION_ADMIN_EMAIL="ios-organization-admin-$(date +%s)-$RANDOM@example.com"
ORGANIZATION_ADMIN_INVITE_LINK=$(create_invite "$ORGANIZATION_ADMIN_EMAIL")

# A partner on the command line owns "Infrastructure", shared with the banner account before it exists, and
# accepts every share invitation it gets and completes every share it sent, as their own Hisar would.
PARTNER_EMAIL="ios-partner-$(date +%s)-$RANDOM@example.com"
PARTNER_PASSWORD='partner master password'
printf '%s\niOS Partner\n%s\n%s\n' "$PARTNER_EMAIL" "$PARTNER_PASSWORD" "$PARTNER_PASSWORD" |
  HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=- "$ROOT/bin/hisar" signup "$(create_invite "$PARTNER_EMAIL")" 2>"$T/partner-signup.txt"
PARTNER_SECRET_KEY=$(grep -oE 'H1-[0-9A-Z-]+' "$T/partner-signup.txt")
partner() { HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=$PARTNER_SECRET_KEY "$ROOT/bin/hisar" "$@"; }
partner_unlocked() { printf '%s\n' "$PARTNER_PASSWORD" | partner "$@"; }
# check_partner fails when a partner step's output lacks what the step promises, so a broken setup stops here and a
# broken step in the loop below is in the partner log printed when the tests fail.
check_partner() {
  [[ "$3" == *"$2"* ]] && return 0
  echo "partner: $1 failed: $3" >&2
  return 1
}
printf '%s\n' "$PARTNER_PASSWORD" | partner vault-create Infrastructure 2>/dev/null
printf '%s\nStaging database\ndeploy\nhttps://staging.example.com\nstaging-database-password\n\n' "$PARTNER_PASSWORD" | partner add -v Infrastructure 2>/dev/null
printf '%s\n' "$PARTNER_PASSWORD" | partner share Infrastructure "$BANNER_EMAIL" 2>/dev/null

# Organizations (§5.11): the partner is the admin of Northwind and invites the joining account before it exists. Northwind's
# team Platform has Deploy as editor and Audit as reader; the team Owners, the partner alone, owns both, so the partner's
# Hisar gives their keys.
check_partner "creating Northwind" 'Created organization "Northwind".' "$(partner org create Northwind 2>&1)"
check_partner "creating the team Owners" 'Created team "Owners" in Northwind.' "$(partner team create Northwind Owners 2>&1)"
check_partner "creating the team Platform" 'Created team "Platform" in Northwind.' "$(partner team create Northwind Platform 2>&1)"
check_partner "joining the team Owners" "Added $PARTNER_EMAIL to team \"Owners\"." "$(partner team add Northwind Owners "$PARTNER_EMAIL" 2>&1)"
for VAULT in Deploy Audit; do
  check_partner "creating $VAULT" "Created vault \"$VAULT\" in Northwind for team \"Owners\"." "$(partner_unlocked vault create "$VAULT" --org Northwind --team Owners 2>&1)"
done
check_partner "adding an item to Deploy" 'Saved "Release signing" to Deploy.' \
  "$(printf '%s\nRelease signing\nrelease\nhttps://deploy.example.com\nrelease-signing-password\n\n' "$PARTNER_PASSWORD" | partner add -v Deploy 2>&1)"
check_partner "adding an item to Audit" 'Saved "Audit log" to Audit.' \
  "$(printf '%s\nAudit log\nauditor\nhttps://audit.example.com\naudit-log-password\n\n' "$PARTNER_PASSWORD" | partner add -v Audit 2>&1)"
check_partner "giving Deploy to Platform" 'Gave "Deploy" to team "Platform" as editor.' "$(partner_unlocked vault give Deploy --org Northwind --team Platform -r editor 2>&1)"
check_partner "giving Audit to Platform" 'Gave "Audit" to team "Platform" as reader.' "$(partner_unlocked vault give Audit --org Northwind --team Platform -r reader 2>&1)"
check_partner "inviting the joining account to Northwind" "Invitation sent to $ORGANIZATION_JOIN_EMAIL to join Northwind as member." \
  "$(partner org invite Northwind "$ORGANIZATION_JOIN_EMAIL" 2>&1)"

# Each organization step of the loop runs once, after the app did what it waits for; the files in $T record what is done.
answer_as_partner() {
  while true; do
    for INVITATION_ID in $(printf '%s\n' "$PARTNER_PASSWORD" | partner invitations 2>/dev/null | awk '/^RECEIVED/{isReceived=1; next} /^(SENT|$)/{isReceived=0} isReceived{print $1}'); do
      partner invitations accept "$INVITATION_ID" </dev/null >/dev/null 2>&1 || true
    done
    # Listing the organizations also gives team members the keys of the vaults the partner owns, and rotates them.
    ORGANIZATIONS=$(partner_unlocked orgs 2>&1 || true)
    for INVITATION_ID in $(printf '%s\n' "$ORGANIZATIONS" | awk '/^INVITATION/{isInvitation=1; next} /^(Answer|$)/{isInvitation=0} isInvitation{print $1}'); do
      check_partner "accepting an organization invitation" "You joined" "$(partner org accept "$INVITATION_ID" </dev/null 2>&1)" || true
    done
    # The joining account accepted: Northwind has two members. It joins Platform and gets the keys of Deploy and Audit.
    if [ ! -e "$T/joined-platform" ] && printf '%s\n' "$ORGANIZATIONS" | awk '$1 == "Northwind" && $3 >= 2 {isFound=1} END {exit !isFound}'; then
      touch "$T/joined-platform"
      check_partner "adding the joining account to Platform" "Added $ORGANIZATION_JOIN_EMAIL to team \"Platform\"." \
        "$(partner team add Northwind Platform "$ORGANIZATION_JOIN_EMAIL" 2>&1)" || true
      check_partner "giving the joining account the keys of Deploy and Audit" "Gave 2 vault keys to team members." "$(partner_unlocked orgs 2>&1)" || true
    fi
    # The joining account saved "Remove me" in Deploy: it leaves Platform, and both vaults get a new key.
    if [ -e "$T/joined-platform" ] && [ ! -e "$T/left-platform" ] && [[ "$(partner_unlocked ls -v Deploy 2>/dev/null || true)" == *"Remove me"* ]]; then
      touch "$T/left-platform"
      check_partner "removing the joining account from Platform" "Removed $ORGANIZATION_JOIN_EMAIL from team \"Platform\"." \
        "$(partner team remove Northwind Platform "$ORGANIZATION_JOIN_EMAIL" 2>&1)" || true
      check_partner "rotating Deploy and Audit" "Rotated the keys of 2 vaults people lost access to." "$(partner_unlocked orgs 2>&1)" || true
    fi
    # The admin account made the partner a member of Contoso's team Ops and created Runbooks for it with Pager in it.
    if [ ! -e "$T/saw-runbooks" ] && printf '%s\n' "$ORGANIZATIONS" | awk '$1 == "Contoso" {isFound=1} END {exit !isFound}' &&
      [ "$(partner_unlocked get Pager -v Runbooks -f username 2>/dev/null || true)" = "pager-oncall" ]; then
      touch "$T/saw-runbooks"
      check_partner "writing into Runbooks" 'Saved "Seen by partner" to Runbooks.' \
        "$(printf '%s\nSeen by partner\npartner\nhttps://runbooks.example.com\nseen-by-partner-password\n\n' "$PARTNER_PASSWORD" | partner add -v Runbooks 2>&1)" || true
    fi
    sleep 2
  done
}
answer_as_partner >"$T/partner.log" 2>&1 &
PARTNER_LOOP=$!

SIMULATOR_ID=${IOS_SIMULATOR_ID:-$("$ROOT/mobile/ios/scripts/find-simulator.sh")}
ACTION=test
if [ -n "${IOS_TESTS_PREBUILT:-}" ]; then ACTION=test-without-building; fi
WORKERS=${IOS_UI_TEST_WORKERS:-1}
PARALLEL=(-parallel-testing-enabled NO)
if [ "$WORKERS" -gt 1 ]; then PARALLEL=(-parallel-testing-enabled YES -parallel-testing-worker-count "$WORKERS"); fi

if ! TEST_RUNNER_HISAR_UI_TEST_SERVER="$HISAR_PUBLIC_URL" \
  TEST_RUNNER_HISAR_UI_TEST_INVITE_LINK="$INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_AUTOFILL_INVITE_LINK="$AUTOFILL_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_BIOMETRIC_INVITE_LINK="$BIOMETRIC_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_ONE_TIME_CODE_INVITE_LINK="$ONE_TIME_CODE_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_PASSKEY_INVITE_LINK="$PASSKEY_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_FAVORITES_INVITE_LINK="$FAVORITES_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_SHARING_INVITE_LINK="$SHARING_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_SHARING_EMAIL="$SHARING_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_SHARING_PARTNER_EMAIL="$PARTNER_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_BANNER_INVITE_LINK="$BANNER_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_BANNER_EMAIL="$BANNER_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_LIFECYCLE_INVITE_LINK="$LIFECYCLE_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_LIFECYCLE_EMAIL="$LIFECYCLE_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_ORGANIZATION_JOIN_INVITE_LINK="$ORGANIZATION_JOIN_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_ORGANIZATION_JOIN_EMAIL="$ORGANIZATION_JOIN_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_ORGANIZATION_ADMIN_INVITE_LINK="$ORGANIZATION_ADMIN_INVITE_LINK" \
  TEST_RUNNER_HISAR_UI_TEST_ORGANIZATION_ADMIN_EMAIL="$ORGANIZATION_ADMIN_EMAIL" \
  TEST_RUNNER_HISAR_UI_TEST_SCREENSHOT_DIRECTORY="${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" \
  xcodebuild "$ACTION" -project "$ROOT/mobile/ios/Hisar.xcodeproj" -scheme Hisar -only-testing:"${IOS_UI_TESTS:-HisarUITests}" \
  -destination "id=$SIMULATOR_ID" "${PARALLEL[@]}" ${IOS_DERIVED_DATA:+-derivedDataPath "$IOS_DERIVED_DATA"} ${IOS_RESULT_BUNDLE:+-resultBundlePath "$IOS_RESULT_BUNDLE"}; then
  echo "partner log:" >&2
  cat "$T/partner.log" >&2
  echo "server log:" >&2
  cat "$T/server.log" >&2
  exit 1
fi
