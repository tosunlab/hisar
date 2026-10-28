# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh, so both apps' UI tests meet the same server.
# Builds and starts hisar-server and an SMTP sink against HISAR_DATABASE_URL (an empty, disposable database), creates the
# invites the UI tests sign up with, and signs up a partner on the standalone CLI who owns the shared vault and the
# organization the tests join, and who answers in the background as the partner's own Hisar would.
#
# Needs ROOT, UI_TEST_PLATFORM_NAME (iOS or Android) and UI_TEST_PORT; with UI_TEST_OPEN_PORT it also starts a second
# server there, on the same database, with open registration. Leaves the server's address in HISAR_PUBLIC_URL
# and every value the tests read in UI_TEST_VALUES, as NAME=value. On exit it stops what it started; a caller that needs
# more cleanup sets its own EXIT trap that also calls ui_test_stop. ui_test_print_logs prints the partner and server logs.
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

T=$(mktemp -d)
UI_TEST_SERVER=
UI_TEST_OPEN_SERVER=
UI_TEST_SINK=
UI_TEST_PARTNER_LOOP=
# Stops the server, the sink, the partner loop and any processes passed in, then deletes $T once they stopped writing.
ui_test_stop() {
  local allProcess
  allProcess=$(echo "$@" $UI_TEST_SERVER $UI_TEST_OPEN_SERVER $UI_TEST_SINK $UI_TEST_PARTNER_LOOP)
  if [ -n "$allProcess" ]; then
    kill $allProcess 2>/dev/null || true
    wait $allProcess 2>/dev/null || true
  fi
  # A partner command started just before the kill can still be writing into $T for a moment.
  for _ in 1 2 3 4 5; do
    rm -rf "$T" 2>/dev/null && return 0
    sleep 1
  done
  rm -rf "$T" || true
}
trap ui_test_stop EXIT
ui_test_print_logs() {
  echo "partner log:" >&2
  cat "$T/partner.log" >&2
  echo "server log:" >&2
  cat "$T/server.log" >&2
  if [ -n "$UI_TEST_OPEN_SERVER" ]; then
    echo "open registration server log:" >&2
    cat "$T/open-server.log" >&2
  fi
}

(cd "$ROOT" && go build -o bin/hisar-server ./server/cmd/hisar-server && go build -o bin/hisar ./cli/cmd/hisar &&
  go build -o bin/hisar-smtp-sink ./server/cmd/hisar-smtp-sink)

UI_TEST_SMTP_PORT=$((UI_TEST_PORT + 1))
export HISAR_PUBLIC_URL=http://127.0.0.1:$UI_TEST_PORT
export HISAR_SECRETS_FILE=$T/secrets.json
export HISAR_AUTH_BURST=1000
# The partner account below uses the standalone CLI; a Hisar app running on this machine must not answer it or start.
export HISAR_STANDALONE=1 HISAR_NO_LAUNCH=1
# Invitations to people without an account are sent by email, so the server needs one; the sink keeps every email in $T/mail.
export HISAR_SMTP_HOST=127.0.0.1 HISAR_SMTP_PORT=$UI_TEST_SMTP_PORT HISAR_SMTP_USERNAME=hisar HISAR_SMTP_PASSWORD=ui-test HISAR_SMTP_FROM='Hisar <noreply@hisar.test>'

"$ROOT/bin/hisar-smtp-sink" -addr "127.0.0.1:$UI_TEST_SMTP_PORT" -dir "$T/mail" >"$T/smtp.log" 2>&1 &
UI_TEST_SINK=$!
"$ROOT/bin/hisar-server" serve -addr "127.0.0.1:$UI_TEST_PORT" >"$T/server.log" 2>&1 &
UI_TEST_SERVER=$!
curl -sf --retry 20 --retry-all-errors --retry-delay 1 "$HISAR_PUBLIC_URL/healthz" >/dev/null

ui_test_create_invite() {
  local link
  link=$("$ROOT/bin/hisar-server" invite --email "$1" --role admin </dev/null | tail -1)
  [[ "$link" == "$HISAR_PUBLIC_URL/invite/"* ]] || { echo "could not create an invite for $1: $link" >&2; exit 1; }
  echo "$link"
}
EMAIL_PREFIX=$(printf '%s' "$UI_TEST_PLATFORM_NAME" | tr '[:upper:]' '[:lower:]')
SHARING_EMAIL="$EMAIL_PREFIX-sharing-$(date +%s)-$RANDOM@example.com"
BANNER_EMAIL="$EMAIL_PREFIX-banner-$(date +%s)-$RANDOM@example.com"
ORGANIZATION_JOIN_EMAIL="$EMAIL_PREFIX-organization-join-$(date +%s)-$RANDOM@example.com"

# A partner on the command line owns "Infrastructure", shared with the banner account before it exists, and
# accepts every share invitation it gets and completes every share it sent, as their own Hisar would.
PARTNER_EMAIL="$EMAIL_PREFIX-partner-$(date +%s)-$RANDOM@example.com"
PARTNER_NAME="$UI_TEST_PLATFORM_NAME Partner"
PARTNER_PASSWORD='partner master password'
printf '%s\n%s\n%s\n%s\n' "$PARTNER_EMAIL" "$PARTNER_NAME" "$PARTNER_PASSWORD" "$PARTNER_PASSWORD" |
  HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=- "$ROOT/bin/hisar" signup -y -o "$T/partner-emergency-kit.pdf" "$(ui_test_create_invite "$PARTNER_EMAIL")" 2>"$T/partner-signup.txt"
PARTNER_SECRET_KEY=$(grep -oE 'H1-[0-9A-Z-]+' "$T/partner-signup.txt")
partner() { HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=$PARTNER_SECRET_KEY "$ROOT/bin/hisar" "$@"; }
partner_unlocked() { printf '%s\n' "$PARTNER_PASSWORD" | partner "$@"; }
# Started once the partner exists, so this process reads a setup that is past its first admin and answers "open".
if [ -n "${UI_TEST_OPEN_PORT:-}" ]; then
  HISAR_OPEN_REGISTRATION=true HISAR_PUBLIC_URL=http://127.0.0.1:$UI_TEST_OPEN_PORT \
    "$ROOT/bin/hisar-server" serve -addr "127.0.0.1:$UI_TEST_OPEN_PORT" >"$T/open-server.log" 2>&1 &
  UI_TEST_OPEN_SERVER=$!
  curl -sf --retry 20 --retry-all-errors --retry-delay 1 "http://127.0.0.1:$UI_TEST_OPEN_PORT/healthz" >/dev/null
fi
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

# Each organization step of the loop runs once the app did what it waits for, and again on every pass until its check
# passes; the files in $T record the steps done. Every `orgs` gives team members the keys of the vaults the partner owns and
# gives the vaults people lost a new key, so a change runs it again at once and the key steps check the latest one.
answer_as_partner() {
  while true; do
    for INVITATION_ID in $(printf '%s\n' "$PARTNER_PASSWORD" | partner invitations 2>/dev/null | awk '/^RECEIVED/{isReceived=1; next} /^(SENT|$)/{isReceived=0} isReceived{print $1}'); do
      partner invitations accept "$INVITATION_ID" </dev/null >/dev/null 2>&1 || true
    done
    for INVITATION_ID in $(partner_unlocked invitations 2>/dev/null | awk '/needs-review/{print $1}'); do
      partner_unlocked invitations confirm "$INVITATION_ID" --yes >/dev/null 2>&1 || true
    done
    # Listing the organizations also gives team members the keys of the vaults the partner owns, and rotates them.
    ORGANIZATIONS=$(partner_unlocked orgs 2>&1 || true)
    for INVITATION_ID in $(printf '%s\n' "$ORGANIZATIONS" | awk '/^INVITATION/{isInvitation=1; next} /^(Answer|$)/{isInvitation=0} isInvitation{print $1}'); do
      check_partner "accepting an organization invitation" "You joined" "$(partner org accept "$INVITATION_ID" </dev/null 2>&1)" || true
    done
    # The joining account accepted: Northwind has two members. It joins Platform and gets the keys of Deploy and Audit.
    if [ ! -e "$T/joined-platform" ] && printf '%s\n' "$ORGANIZATIONS" | awk '$1 == "Northwind" && $3 >= 2 {isFound=1} END {exit !isFound}' &&
      check_partner "adding the joining account to Platform" "Added $ORGANIZATION_JOIN_EMAIL to team \"Platform\"." \
        "$(partner team add Northwind Platform "$ORGANIZATION_JOIN_EMAIL" 2>&1)"; then
      touch "$T/joined-platform"
      ORGANIZATIONS=$(partner_unlocked orgs 2>&1 || true)
    fi
    if [ -e "$T/joined-platform" ] && [ ! -e "$T/gave-platform-keys" ] &&
      check_partner "giving the joining account the keys of Deploy and Audit" "Gave 2 vault keys to team members." "$ORGANIZATIONS"; then
      touch "$T/gave-platform-keys"
    fi
    # The joining account saved "Remove me" in Deploy: it leaves Platform, and both vaults get a new key.
    if [ -e "$T/joined-platform" ] && [ ! -e "$T/left-platform" ] && [[ "$(partner_unlocked ls -v Deploy 2>/dev/null || true)" == *"Remove me"* ]] &&
      check_partner "removing the joining account from Platform" "Removed $ORGANIZATION_JOIN_EMAIL from team \"Platform\"." \
        "$(partner team remove Northwind Platform "$ORGANIZATION_JOIN_EMAIL" 2>&1)"; then
      touch "$T/left-platform"
      ORGANIZATIONS=$(partner_unlocked orgs 2>&1 || true)
    fi
    if [ -e "$T/left-platform" ] && [ ! -e "$T/rotated-platform-vaults" ] &&
      check_partner "rotating Deploy and Audit" "Rotated the keys of 2 vaults people lost access to." "$ORGANIZATIONS"; then
      touch "$T/rotated-platform-vaults"
    fi
    # The admin account made the partner a member of Contoso's team Ops and created Runbooks for it with Pager in it.
    if [ ! -e "$T/saw-runbooks" ] && printf '%s\n' "$ORGANIZATIONS" | awk '$1 == "Contoso" {isFound=1} END {exit !isFound}' &&
      [ "$(partner_unlocked get Pager -v Runbooks -f username 2>/dev/null || true)" = "pager-oncall" ] &&
      check_partner "writing into Runbooks" 'Saved "Seen by partner" to Runbooks.' \
        "$(printf '%s\nSeen by partner\npartner\nhttps://runbooks.example.com\nseen-by-partner-password\n\n' "$PARTNER_PASSWORD" | partner add -v Runbooks 2>&1)"; then
      touch "$T/saw-runbooks"
    fi
    sleep 2
  done
}
answer_as_partner >"$T/partner.log" 2>&1 &
UI_TEST_PARTNER_LOOP=$!

UI_TEST_VALUES=(
  "HISAR_UI_TEST_SERVER=$HISAR_PUBLIC_URL"
  "HISAR_UI_TEST_SHARING_PARTNER_EMAIL=$PARTNER_EMAIL"
)
# ui_test_add_account invites an account and passes its invite link and email as HISAR_UI_TEST_<$1>INVITE_LINK and
# HISAR_UI_TEST_<$1>EMAIL.
ui_test_add_account() {
  local link
  link=$(ui_test_create_invite "$2")
  UI_TEST_VALUES+=("HISAR_UI_TEST_${1}INVITE_LINK=$link" "HISAR_UI_TEST_${1}EMAIL=$2")
}
# The iPhone's tests sign up with the first five addresses, written in the tests.
ui_test_add_account "" "$EMAIL_PREFIX-ui@example.com"
ui_test_add_account BIOMETRIC_ "$EMAIL_PREFIX-face-id@example.com"
ui_test_add_account ONE_TIME_CODE_ "$EMAIL_PREFIX-one-time-code@example.com"
ui_test_add_account PASSKEY_ "$EMAIL_PREFIX-passkey@example.com"
ui_test_add_account FAVORITES_ "$EMAIL_PREFIX-favorites@example.com"
ui_test_add_account WIDE_LAYOUT_ "$EMAIL_PREFIX-wide-layout@example.com"
ui_test_add_account SHARING_ "$SHARING_EMAIL"
ui_test_add_account BANNER_ "$BANNER_EMAIL"
ui_test_add_account LIFECYCLE_ "$EMAIL_PREFIX-lifecycle-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ORGANIZATION_JOIN_ "$ORGANIZATION_JOIN_EMAIL"
ui_test_add_account ORGANIZATION_ADMIN_ "$EMAIL_PREFIX-organization-admin-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ONBOARDING_POSTPONED_ "$EMAIL_PREFIX-onboarding-postponed-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ONBOARDING_AUTOLOCK_ "$EMAIL_PREFIX-onboarding-autolock-$(date +%s)-$RANDOM@example.com"
ui_test_add_account EMERGENCY_KIT_BANNER_ "$EMAIL_PREFIX-emergency-kit-banner-$(date +%s)-$RANDOM@example.com"
if [ -n "$UI_TEST_OPEN_SERVER" ]; then
  UI_TEST_VALUES+=(
    "HISAR_UI_TEST_OPEN_SERVER=http://127.0.0.1:$UI_TEST_OPEN_PORT"
    "HISAR_UI_TEST_OPEN_EMAIL=$EMAIL_PREFIX-open-$(date +%s)-$RANDOM@example.com"
  )
fi
