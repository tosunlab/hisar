# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh, so both apps' UI tests meet the same server.
# Builds and starts hisar-server and an SMTP sink against HISAR_DATABASE_URL (an empty, disposable database), creates the
# invites the UI tests sign up with, and starts hisar-ui-test-partner, which signs up a partner and answers in the
# background as the partner's own Hisar would.
#
# Needs ROOT, UI_TEST_PLATFORM_NAME (iOS or Android) and UI_TEST_PORT. Leaves the server's address in HISAR_PUBLIC_URL
# and every value the tests read in UI_TEST_VALUES, as NAME=value. On exit it stops what it started; a caller that needs
# more cleanup sets its own EXIT trap that also calls ui_test_stop. ui_test_print_logs prints the partner and server logs.
# ui_test_restart empties the database with psql and starts all of it again, with new invites and a new partner.
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

T=
UI_TEST_SERVER=
UI_TEST_SINK=
UI_TEST_PARTNER=
# Stops the server, the sink, the partner and any processes passed in, then deletes $T once they stopped writing.
ui_test_stop() {
  local allProcess
  allProcess=$(echo "$@" $UI_TEST_SERVER $UI_TEST_SINK $UI_TEST_PARTNER)
  if [ -n "$allProcess" ]; then
    kill $allProcess 2>/dev/null || true
    wait $allProcess 2>/dev/null || true
  fi
  UI_TEST_SERVER='' UI_TEST_SINK='' UI_TEST_PARTNER=''
  [ -n "$T" ] || return 0
  # A child of a stopped process can still be writing into $T for a moment.
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

(cd "$ROOT" && go build -o bin/hisar-server ./server/cmd/hisar-server && go build -o bin/hisar-smtp-sink ./server/cmd/hisar-smtp-sink &&
  go build -o bin/hisar-ui-test-partner ./mobile/cmd/hisar-ui-test-partner)

UI_TEST_SMTP_PORT=$((UI_TEST_PORT + 1))
export HISAR_PUBLIC_URL=http://127.0.0.1:$UI_TEST_PORT
export HISAR_AUTH_BURST=1000
# Invitations to people without an account are sent by email, so the server needs one; the sink keeps every email in $T/mail.
export HISAR_SMTP_HOST=127.0.0.1 HISAR_SMTP_PORT=$UI_TEST_SMTP_PORT HISAR_SMTP_USERNAME=hisar HISAR_SMTP_PASSWORD=ui-test HISAR_SMTP_FROM='Hisar <noreply@hisar.test>'

ui_test_create_invite() {
  local link
  link=$("$ROOT/bin/hisar-server" invite --email "$1" --role admin </dev/null | tail -1)
  [[ "$link" == "$HISAR_PUBLIC_URL/invite/"* ]] || { echo "could not create an invite for $1: $link" >&2; exit 1; }
  echo "$link"
}
# ui_test_add_account invites an account and passes its invite link and email as HISAR_UI_TEST_<$1>INVITE_LINK and
# HISAR_UI_TEST_<$1>EMAIL.
ui_test_add_account() {
  local link
  link=$(ui_test_create_invite "$2")
  UI_TEST_VALUES+=("HISAR_UI_TEST_${1}INVITE_LINK=$link" "HISAR_UI_TEST_${1}EMAIL=$2")
}

ui_test_start() {
  T=$(mktemp -d)
  export HISAR_SECRETS_FILE=$T/secrets.json
  "$ROOT/bin/hisar-smtp-sink" -addr "127.0.0.1:$UI_TEST_SMTP_PORT" -dir "$T/mail" >"$T/smtp.log" 2>&1 &
  UI_TEST_SINK=$!
  "$ROOT/bin/hisar-server" serve -addr "127.0.0.1:$UI_TEST_PORT" >"$T/server.log" 2>&1 &
  UI_TEST_SERVER=$!
  curl -sf --retry 20 --retry-all-errors --retry-delay 1 "$HISAR_PUBLIC_URL/healthz" >/dev/null

  EMAIL_PREFIX=$(printf '%s' "$UI_TEST_PLATFORM_NAME" | tr '[:upper:]' '[:lower:]')
  PARTNER_EMAIL="$EMAIL_PREFIX-partner-$(date +%s)-$RANDOM@example.com"
  "$ROOT/bin/hisar-ui-test-partner" -invite "$(ui_test_create_invite "$PARTNER_EMAIL")" -email "$PARTNER_EMAIL" \
    -name "$UI_TEST_PLATFORM_NAME Partner" -password 'partner master password' \
    -ready-file "$T/partner-ready" >"$T/partner.log" 2>&1 &
  UI_TEST_PARTNER=$!
  for _ in $(seq 120); do [ -e "$T/partner-ready" ] || ! kill -0 "$UI_TEST_PARTNER" 2>/dev/null && break; sleep 1; done
  [ -e "$T/partner-ready" ] || { echo "hisar-ui-test-partner did not sign up the partner:" >&2; cat "$T/partner.log" >&2; exit 1; }

  UI_TEST_VALUES=(
    "HISAR_UI_TEST_SERVER=$HISAR_PUBLIC_URL"
    "HISAR_UI_TEST_SHARING_PARTNER_EMAIL=$PARTNER_EMAIL"
  )
  # The iPhone's tests sign up with the first three addresses, written in the tests.
  ui_test_add_account "" "$EMAIL_PREFIX-ui@example.com"
  ui_test_add_account BIOMETRIC_ "$EMAIL_PREFIX-face-id@example.com"
  ui_test_add_account PASSKEY_ "$EMAIL_PREFIX-passkey@example.com"
  ui_test_add_account SHARING_ "$EMAIL_PREFIX-sharing-$(date +%s)-$RANDOM@example.com"
  ui_test_add_account LIFECYCLE_ "$EMAIL_PREFIX-lifecycle-$(date +%s)-$RANDOM@example.com"
  ui_test_add_account ORGANIZATION_ADMIN_ "$EMAIL_PREFIX-organization-admin-$(date +%s)-$RANDOM@example.com"
}

# Each test signs up with a single-use invite, most with an address written in the test, so a test can only run a second
# time against an emptied database.
ui_test_restart() {
  command -v psql >/dev/null || { echo "psql is not on PATH, so the database cannot be emptied for a second run" >&2; return 1; }
  ui_test_stop
  PGOPTIONS=--client-min-messages=warning psql "$HISAR_DATABASE_URL" -q -v ON_ERROR_STOP=1 -c 'DROP SCHEMA public CASCADE' -c 'CREATE SCHEMA public'
  ui_test_start
}

ui_test_start
