#!/usr/bin/env bash
# End-to-end test of the built hisar-server and hisar CLI with two users.
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
BIN=$ROOT/bin
T=$(mktemp -d)
PORT=${END_TO_END_PORT:-18081}
SMTP_PORT=${END_TO_END_SMTP_PORT:-18091}
MAIL=$T/mail
export HISAR_PUBLIC_URL=http://127.0.0.1:$PORT
export HISAR_SECRETS_FILE=$T/secrets.json
# Many sign-ins come from one IP during this test.
export HISAR_AUTH_BURST=1000
# The standalone CLI is tested here; a Hisar app running on this machine must not answer it.
export HISAR_STANDALONE=1
# Invites and share invitations go to a local SMTP sink that writes each email to $MAIL.
export HISAR_SMTP_HOST=127.0.0.1 HISAR_SMTP_PORT=$SMTP_PORT HISAR_SMTP_USERNAME=hisar HISAR_SMTP_PASSWORD=end-to-end HISAR_SMTP_FROM='Hisar <noreply@hisar.test>'
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

"$BIN/hisar-smtp-sink" -addr "127.0.0.1:$SMTP_PORT" -dir "$MAIL" >"$T/smtp.log" 2>&1 &
SINK=$!
"$BIN/hisar-server" serve -addr "127.0.0.1:$PORT" >"$T/server.log" 2>&1 &
SERVER=$!
trap 'kill $SERVER $SINK 2>/dev/null || true; rm -rf "$T"' EXIT
curl -sf --retry 20 --retry-all-errors --retry-delay 1 "http://127.0.0.1:$PORT/healthz" >/dev/null

fail() { echo "FAIL: $*" >&2; cat "$T/server.log" >&2; exit 1; }
expect() { [[ "$2" == *"$3"* ]] || fail "$1: expected '$3', got '$2'"; echo "ok   $1"; }

PW='correct horse battery'
hisar() { "$BIN/hisar" "$@"; }

count_mail() { find "$MAIL" -name "*-$1.txt" 2>/dev/null | wc -l | tr -d ' '; }
# wait_for_mail prints the newest email to $1 once there are more than $2.
wait_for_mail() {
  for _ in $(seq 1 100); do
    if (( $(count_mail "$1") > $2 )); then
      cat "$(find "$MAIL" -name "*-$1.txt" | sort | tail -1)"
      return
    fi
    sleep 0.1
  done
  fail "no email to $1"
}
get_invite_code() { grep -oE '/invite/[A-Za-z0-9_-]+' | head -1 | cut -d/ -f3; }

ALICE_INV=$("$BIN/hisar-server" invite --email alice@example.com </dev/null | tail -1)
expect "admin invite" "$ALICE_INV" "$HISAR_PUBLIC_URL/invite/"

printf 'alice@example.com\nAlice\n%s\n%s\n' "$PW" "$PW" | HISAR_STATE=$T/alice.json HISAR_SECRET_KEY=- hisar signup "$ALICE_INV" 2>"$T/alice-signup.txt"
ALICE_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/alice-signup.txt")
expect "alice signup" "$ALICE_SK" "H1-"
A() { HISAR_STATE=$T/alice.json HISAR_SECRET_KEY=$ALICE_SK hisar "$@"; }

expect "whoami" "$(A whoami)" "role:    admin"

printf '%s\nGitHub\nalice\nhttps://github.com\n\n\n' "$PW" | A add 2>/dev/null
GEN=$(printf '%s\n' "$PW" | A get github -f password 2>/dev/null)
[[ ${#GEN} -eq 24 ]] || fail "generated password length ${#GEN}"
echo "ok   generated password"

printf '%s\n' "$PW" | A vault-create Family 2>/dev/null
printf '%s\nNetflix\nfamily@example.com\nhttps://netflix.com\nhunter2hunter2\nshared login\n' "$PW" | A add -v Family 2>/dev/null

INVITE_OUTPUT=$(A invite bob@example.com 2>&1)
expect "invite by email" "$INVITE_OUTPUT" "Invitation sent to bob@example.com."
[[ "$INVITE_OUTPUT" != *"/invite/"* ]] || fail "the invite response shows the link: $INVITE_OUTPUT"
echo "ok   the invite response has no link or code"
BOB_MAIL=$(wait_for_mail bob@example.com 0)
expect "invite email" "$BOB_MAIL" "Alice (alice@example.com) invited you to Hisar"
BOB_CODE=$(printf '%s' "$BOB_MAIL" | get_invite_code)
expect "invite email carries the link and the code" "$BOB_MAIL" "$HISAR_PUBLIC_URL/invite/$BOB_CODE"$'\n\nOr enter this invite code in the Hisar app:\n'"$BOB_CODE"
printf 'bob@example.com\nBob\n%s\n%s\n' "$PW" "$PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=- hisar signup "$HISAR_PUBLIC_URL/invite/$BOB_CODE" 2>"$T/bob-signup.txt"
BOB_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/bob-signup.txt")
expect "bob signs up from the emailed code" "$BOB_SK" "H1-"
B() { HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=$BOB_SK hisar "$@"; }

BOB_MAIL_COUNT=$(count_mail bob@example.com)
SHARE_OUTPUT=$(printf '%s\n' "$PW" | A share Family bob@example.com 2>&1)
expect "share sends an invitation" "$SHARE_OUTPUT" 'Invitation sent to bob@example.com for "Family" as editor.'
UNKNOWN_SHARE_OUTPUT=$(printf '%s\n' "$PW" | A share Family nobody@example.com 2>&1)
[[ "${UNKNOWN_SHARE_OUTPUT//nobody@example.com/someone}" == "${SHARE_OUTPUT//bob@example.com/someone}" ]] || fail "share to an unknown email answered differently: $UNKNOWN_SHARE_OUTPUT"
echo "ok   a share to an email without an account gives the same response"
expect "share email to bob" "$(wait_for_mail bob@example.com "$BOB_MAIL_COUNT")" "Alice (alice@example.com) shared a vault with you in Hisar as an editor."
expect "share email to an unknown address carries a sign-up invite" "$(wait_for_mail nobody@example.com 0)" "$HISAR_PUBLIC_URL/invite/"
expect "bob does not see Family before accepting" "$(printf '%s\n' "$PW" | B vaults 2>/dev/null)" "Personal"
[[ "$(printf '%s\n' "$PW" | B vaults 2>/dev/null)" != *Family* ]] || fail "bob sees Family before accepting"
expect "bob's invitations" "$(B invitations 2>&1)" "Alice <alice@example.com>  editor"
expect "bob accepts" "$(printf '\n' | B invitations accept 2>&1)" "Accepted."
[[ "$(printf '%s\n' "$PW" | B vaults 2>/dev/null)" != *Family* ]] || fail "bob sees Family before alice completed the share"
echo "ok   accepting alone gives no access"
expect "alice's app completes the share" "$(printf '%s\n' "$PW" | A invitations 2>&1)" 'Shared "Family" with bob@example.com as editor. Their security code:'
expect "bob sees Family" "$(printf '%s\n' "$PW" | B vaults 2>/dev/null)" "Family    shared    editor"
expect "bob reads item" "$(printf '%s\n' "$PW" | B get netflix -v Family -f password 2>/dev/null)" "hunter2hunter2"
expect "bob edits item" "$(printf '%s\nNetflix\n\n\n\nedited by bob\n' "$PW" | B edit netflix -v Family 2>&1)" "Saved"
expect "alice sees edit" "$(printf '%s\n' "$PW" | A get netflix -v Family -f notes 2>/dev/null)" "edited by bob"
expect "bob invites by email too" "$(B invite eve@example.com 2>&1)" "Invitation sent to eve@example.com."

CAROL_INV=$("$BIN/hisar-server" invite --email carol@example.com </dev/null | tail -1)
printf 'carol@example.com\nCarol\n%s\n%s\n' "$PW" "$PW" | HISAR_STATE=$T/carol.json HISAR_SECRET_KEY=- hisar signup "$CAROL_INV" 2>"$T/carol-signup.txt"
CAROL_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/carol-signup.txt")
C() { HISAR_STATE=$T/carol.json HISAR_SECRET_KEY=$CAROL_SK hisar "$@"; }
printf '%s\n' "$PW" | A vault-create Work 2>/dev/null
ALICE_TOKEN=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$T/alice.json")
WORK_ID=$(printf '%s\n' "$PW" | A vaults 2>/dev/null | awk '/^Work/{print $4}')
share_raw() {
  curl -s -w ' %{http_code}' -H "Authorization: Bearer $ALICE_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"role\":\"reader\"}" "http://127.0.0.1:$PORT/api/v1/vaults/$WORK_ID/share-invitations"
}
EXISTING_RESPONSE=$(share_raw carol@example.com)
UNKNOWN_RESPONSE=$(share_raw ghost@example.com)
expect "share API response" "$EXISTING_RESPONSE" '{"message":"Invitation sent"} 202'
[[ "$EXISTING_RESPONSE" == "$UNKNOWN_RESPONSE" ]] || fail "the share API answers differently for an unknown email: $UNKNOWN_RESPONSE"
echo "ok   the share API answers the same for an existing and an unknown email"
expect "carol accepts" "$(C invitations accept "$(C invitations 2>/dev/null | awk '/alice@example.com/{print $1}')" 2>&1)" "Accepted."
expect "alice completes carol's share" "$(printf '%s\n' "$PW" | A invitations 2>&1)" 'Shared "Work" with carol@example.com as reader.'
expect "alice sees carol" "$(A people)" "carol@example.com"
BOB_PEOPLE=$(B people)
expect "bob sees alice" "$BOB_PEOPLE" "alice@example.com"
[[ "$BOB_PEOPLE" != *carol@example.com* ]] || fail "bob can list alice's other contacts: $BOB_PEOPLE"
echo "ok   bob cannot list alice's other contacts"
BOB_TOKEN=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$T/bob.json")
[[ "$(curl -s -H "Authorization: Bearer $BOB_TOKEN" "http://127.0.0.1:$PORT/api/v1/members")" != *carol@example.com* ]] || fail "GET /members shows carol to bob"
echo "ok   GET /members only lists people who share a vault"
expect "bob cannot read alice personal" "$(printf '%s\n' "$PW" | B ls -v "$(printf '%s\n' "$PW" | A vaults 2>/dev/null | awk '/Personal/{print $4}')" 2>&1 || true)" "not found"

expect "second device login" "$(printf 'http://127.0.0.1:%s\nalice@example.com\n%s\n%s\n' "$PORT" "$ALICE_SK" "$PW" | HISAR_STATE=$T/alice2.json HISAR_SECRET_KEY=- hisar login 2>&1)" "Signed in as Alice"
expect "wrong password" "$(printf 'nope\n' | A ls 2>&1 || true)" "incorrect email, password or Secret Key"
expect "devices" "$(A devices)" "(this device)"
expect "emergency kit" "$(printf '%s\n' "$PW" | A emergency-kit -o "$T/kit.pdf" 2>&1)" "Saved your Emergency Kit"
expect "emergency kit is a private PDF" "$(head -c 5 "$T/kit.pdf") $(stat -c %a "$T/kit.pdf" 2>/dev/null || stat -f %Lp "$T/kit.pdf")" "%PDF- 600"
expect "emergency kit never overwrites" "$(printf '%s\n' "$PW" | A emergency-kit -o "$T/kit.pdf" 2>&1 || true)" "already exists"

printf '%s\n' 'name,url,username,password,note' \
  'Imported Bank,https://bank.example.com,alice,"bank,pass""word",' \
  'Imported Mail,https://mail.example.com,alice@example.com,mail-pass-1,"First line' \
  'Second line"' \
  'GitHub,https://github.com,alice,should-not-replace,' >"$T/import.csv"
expect "CLI import of a generic CSV skips the GitHub item already in Personal" "$(printf '%s\n' "$PW" | A import "$T/import.csv" -y 2>&1)" "Imported 2 items into Personal"
expect "imported password keeps its comma and quote" "$(printf '%s\n' "$PW" | A get "Imported Bank" -f password 2>/dev/null)" 'bank,pass"word'
expect "imported notes keep their line break" "$(printf '%s\n' "$PW" | A get "Imported Mail" -f notes 2>/dev/null)" $'First line\nSecond line'
GITHUB_COUNT=$(printf '%s\n' "$PW" | A ls 2>/dev/null | grep -c '^GitHub ' || true)
[[ "$GITHUB_COUNT" == 1 ]] || fail "import added a second GitHub item ($GITHUB_COUNT)"
echo "ok   import did not duplicate GitHub"
[[ -f "$T/import.csv" ]] || fail "import -y deleted the export file"
echo "ok   import -y keeps the export file"
expect "interactive import declines the items already in the vault" "$(printf '%s\nn\n' "$PW" | A import "$T/import.csv" 2>&1)" "Nothing to import."
printf '%s\n' "$PW" | A vault-create Imports 2>/dev/null
expect "interactive import deletes the export file when asked" "$(printf '%s\ny\ny\n' "$PW" | A import "$T/import.csv" -v Imports 2>&1)" "Deleted $T/import.csv"
[[ ! -e "$T/import.csv" ]] || fail "export file still exists after the import deleted it"
echo "ok   export file deleted"

expect "delete" "$(printf '%s\ny\n' "$PW" | A rm netflix -v Family 2>&1)" "Deleted"
LEFT=$(printf '%s\n' "$PW" | B ls -v Family 2>/dev/null | tail -n +2)
[[ -z "$LEFT" ]] || fail "items left after delete: $LEFT"
echo "ok   delete propagates"

expect "unshare rotates the vault key" "$(printf '%s\n' "$PW" | A unshare Family bob@example.com 2>&1)" "rotated the vault key"
expect "removed member cannot read the vault" "$(printf '%s\n' "$PW" | B ls -v Family 2>&1 || true)" "not found"

expect "recovery setup" "$(printf '%s\n' "$PW" | A recovery-setup 2>&1)" "Recovery is set up"
expect "bob enables recovery" "$(printf '%s\n' "$PW" | B recovery-enable 2>&1)" 'Enabled recovery for "Personal"'
printf '%s\n' "$PW" | A vault-create Household 2>/dev/null
printf '%s\nWifi\n\n\nwifi-password-1\n\n' "$PW" | A add -v Household 2>/dev/null
expect "share recoverable vault" "$(printf '%s\n' "$PW" | A share Household bob@example.com 2>&1)" "as editor."
expect "bob accepts the household invitation" "$(B invitations accept "$(B invitations 2>/dev/null | awk '/alice@example.com/{print $1}')" 2>&1)" "Accepted."
expect "alice completes the household share" "$(printf '%s\n' "$PW" | A invitations 2>&1)" 'Shared "Household" with bob@example.com as editor.'
expect "member cannot start a recovery" "$(B recovery-start alice@example.com 2>&1 || true)" "admin only"

RECOVERY_LINK=$(A recovery-start bob@example.com 2>/dev/null)
expect "recovery link" "$RECOVERY_LINK" "$HISAR_PUBLIC_URL/recover/"
NEW_PW='new correct horse battery'
printf 'bob@example.com\n%s\n%s\n' "$NEW_PW" "$NEW_PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=- hisar recover "$RECOVERY_LINK" 2>"$T/bob-recover.txt"
BOB_NEW_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/bob-recover.txt" || true)
[[ "$BOB_NEW_SK" == H1-* && "$BOB_NEW_SK" != "$BOB_SK" ]] || fail "recover did not print a new Secret Key: $(cat "$T/bob-recover.txt")"
echo "ok   recover prints a new Secret Key"
expect "recovery link is single use" "$(printf 'bob@example.com\n%s\n%s\n' "$NEW_PW" "$NEW_PW" | HISAR_STATE=$T/bob-again.json HISAR_SECRET_KEY=- hisar recover "$RECOVERY_LINK" 2>&1 || true)" "invalid recovery link"
expect "recovery pending" "$(A recovery-list 2>&1)" "bob@example.com"
expect "recovery complete" "$(printf 'y\n%s\n' "$PW" | A recovery-complete bob@example.com 2>&1)" "Recovered bob@example.com"
expect "old Secret Key stops working" "$(printf 'http://127.0.0.1:%s\nbob@example.com\n%s\n%s\n' "$PORT" "$BOB_SK" "$PW" | HISAR_STATE=$T/bob-old.json HISAR_SECRET_KEY=- hisar login 2>&1 || true)" "incorrect email, password or Secret Key"
expect "login with the new Secret Key" "$(printf 'http://127.0.0.1:%s\nbob@example.com\n%s\n%s\n' "$PORT" "$BOB_NEW_SK" "$NEW_PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=- hisar login 2>&1)" "Signed in as Bob"
RB() { HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=$BOB_NEW_SK hisar "$@"; }
expect "recovered bob reads item" "$(printf '%s\n' "$NEW_PW" | RB get wifi -v Household -f password 2>/dev/null)" "wifi-password-1"

APPCAST_URL=http://127.0.0.1:$PORT/api/v1/desktop/updates/appcast.xml
expect "desktop updates need a session" "$(curl -s "$APPCAST_URL")" "unauthorized"
expect "desktop updates not configured" "$(curl -s -H "Authorization: Bearer $ALICE_TOKEN" "$APPCAST_URL")" "updates are not configured on this server"

expect "logout" "$(A logout && A whoami 2>&1 || true)" "not logged in"

DESKTOP_INVITE=$("$BIN/hisar-server" invite --email desktop@example.com --role admin </dev/null | tail -1)
DESKTOP_MEMBER_INVITE=$("$BIN/hisar-server" invite --email desktop-member@example.com </dev/null | tail -1)
DESKTOP_RECOVERED_INVITE=$("$BIN/hisar-server" invite --email desktop-recovered@example.com </dev/null | tail -1)
DESKTOP_RESULT=$(HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_INVITE_LINK="$DESKTOP_INVITE" \
  HISAR_TEST_MEMBER_INVITE_LINK="$DESKTOP_MEMBER_INVITE" HISAR_TEST_RECOVERED_MEMBER_INVITE_LINK="$DESKTOP_RECOVERED_INVITE" \
  HISAR_TEST_RECOVERY_ADMIN_EMAIL=alice@example.com HISAR_TEST_RECOVERY_ADMIN_SECRET_KEY="$ALICE_SK" HISAR_TEST_RECOVERY_ADMIN_PASSWORD="$PW" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationDesktopFlow ./vault/ 2>&1 || true)
expect "desktop app backend against the server" "$DESKTOP_RESULT" "--- PASS: TestIntegrationDesktopFlow"

COMMAND_LINE_INVITE=$("$BIN/hisar-server" invite --email cli@example.com </dev/null | tail -1)
COMMAND_LINE_RESULT=$(HISAR_STANDALONE= HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_CLI_INVITE_LINK="$COMMAND_LINE_INVITE" HISAR_TEST_CLI_BINARY="$BIN/hisar" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationCommandLineThroughApp ./vault/ 2>&1 || true)
expect "hisar through the desktop app backend" "$COMMAND_LINE_RESULT" "--- PASS: TestIntegrationCommandLineThroughApp"

MOBILE_INVITE=$("$BIN/hisar-server" invite --email mobile@example.com </dev/null | tail -1)
MOBILE_RESULT=$(HISAR_TEST_MOBILE_INVITE_LINK="$MOBILE_INVITE" go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationMobileFlow ./hisarkit/ 2>&1 || true)
expect "iOS app backend against the server" "$MOBILE_RESULT" "--- PASS: TestIntegrationMobileFlow"

PHONE_APPROVAL_INVITE=$("$BIN/hisar-server" invite --email phone-approval@example.com </dev/null | tail -1)
PHONE_APPROVAL_RESULT=$(HISAR_TEST_PHONE_APPROVAL_INVITE_LINK="$PHONE_APPROVAL_INVITE" go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationPhoneApproval ./hisarkit/ 2>&1 || true)
expect "the iPhone approves and denies the Mac's SSH requests, and a forged answer is refused" "$PHONE_APPROVAL_RESULT" "--- PASS: TestIntegrationPhoneApproval"
echo "end-to-end test passed"
