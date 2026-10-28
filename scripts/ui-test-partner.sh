# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh, so both apps' UI tests meet the same server.
# Builds and starts hisar-server and an SMTP sink against HISAR_DATABASE_URL (an empty, disposable database), creates the
# invites the UI tests sign up with, and signs up a partner on the standalone CLI who answers in the background as the
# partner's own Hisar would.
#
# Needs ROOT, UI_TEST_PLATFORM_NAME (iOS or Android) and UI_TEST_PORT. Leaves the server's address in HISAR_PUBLIC_URL
# and every value the tests read in UI_TEST_VALUES, as NAME=value. On exit it stops what it started; a caller that needs
# more cleanup sets its own EXIT trap that also calls ui_test_stop. ui_test_print_logs prints the partner and server logs.
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

T=$(mktemp -d)
UI_TEST_SERVER=
UI_TEST_SINK=
UI_TEST_PARTNER_LOOP=
# Stops the server, the sink, the partner loop and any processes passed in, then deletes $T once they stopped writing.
ui_test_stop() {
  local allProcess
  allProcess=$(echo "$@" $UI_TEST_SERVER $UI_TEST_SINK $UI_TEST_PARTNER_LOOP)
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

# A partner on the command line accepts every share and organization invitation it gets and completes every share it
# sent, as their own Hisar would.
PARTNER_EMAIL="$EMAIL_PREFIX-partner-$(date +%s)-$RANDOM@example.com"
PARTNER_NAME="$UI_TEST_PLATFORM_NAME Partner"
PARTNER_PASSWORD='partner master password'
printf '%s\n%s\n%s\n%s\n' "$PARTNER_EMAIL" "$PARTNER_NAME" "$PARTNER_PASSWORD" "$PARTNER_PASSWORD" |
  HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=- "$ROOT/bin/hisar" signup -y -o "$T/partner-emergency-kit.pdf" "$(ui_test_create_invite "$PARTNER_EMAIL")" 2>"$T/partner-signup.txt"
PARTNER_SECRET_KEY=$(grep -oE 'H1-[0-9A-Z-]+' "$T/partner-signup.txt")
partner() { HISAR_STATE=$T/partner.json HISAR_SECRET_KEY=$PARTNER_SECRET_KEY "$ROOT/bin/hisar" "$@"; }
partner_unlocked() { printf '%s\n' "$PARTNER_PASSWORD" | partner "$@"; }
# check_partner fails when a partner step's output lacks what the step promises; a broken step in the loop below is in the
# partner log printed when the tests fail.
check_partner() {
  [[ "$3" == *"$2"* ]] && return 0
  echo "partner: $1 failed: $3" >&2
  return 1
}
# The Runbooks step runs once the app did what it waits for, and again on every pass until its check passes; a file in $T
# records it done.
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
# The iPhone's tests sign up with the first three addresses, written in the tests.
ui_test_add_account "" "$EMAIL_PREFIX-ui@example.com"
ui_test_add_account BIOMETRIC_ "$EMAIL_PREFIX-face-id@example.com"
ui_test_add_account PASSKEY_ "$EMAIL_PREFIX-passkey@example.com"
ui_test_add_account SHARING_ "$EMAIL_PREFIX-sharing-$(date +%s)-$RANDOM@example.com"
ui_test_add_account LIFECYCLE_ "$EMAIL_PREFIX-lifecycle-$(date +%s)-$RANDOM@example.com"
ui_test_add_account ORGANIZATION_ADMIN_ "$EMAIL_PREFIX-organization-admin-$(date +%s)-$RANDOM@example.com"
