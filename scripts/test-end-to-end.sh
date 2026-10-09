#!/usr/bin/env bash
# End-to-end test of the built hisar-server and hisar CLI with two users.
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
BIN=$ROOT/bin
T=$(mktemp -d)
PORT=${END_TO_END_PORT:-18081}
export HISAR_PUBLIC_URL=http://127.0.0.1:$PORT
export HISAR_SECRETS_FILE=$T/secrets.json
# Many sign-ins come from one IP during this test.
export HISAR_AUTH_BURST=1000
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"

"$BIN/hisar-server" serve -addr "127.0.0.1:$PORT" >"$T/server.log" 2>&1 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true; rm -rf "$T"' EXIT
curl -sf --retry 20 --retry-all-errors --retry-delay 1 "http://127.0.0.1:$PORT/healthz" >/dev/null

fail() { echo "FAIL: $*" >&2; cat "$T/server.log" >&2; exit 1; }
expect() { [[ "$2" == *"$3"* ]] || fail "$1: expected '$3', got '$2'"; echo "ok   $1"; }

PW='correct horse battery'
hisar() { "$BIN/hisar" "$@"; }

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

BOB_INV=$(A invite bob@example.com 2>/dev/null)
printf 'bob@example.com\nBob\n%s\n%s\n' "$PW" "$PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=- hisar signup "$BOB_INV" 2>"$T/bob-signup.txt"
BOB_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/bob-signup.txt")
B() { HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=$BOB_SK hisar "$@"; }

expect "share" "$(printf '%s\n' "$PW" | A share Family bob@example.com 2>&1)" "as editor"
expect "bob sees Family" "$(printf '%s\n' "$PW" | B vaults 2>/dev/null)" "Family    shared    editor"
expect "bob reads item" "$(printf '%s\n' "$PW" | B get netflix -v Family -f password 2>/dev/null)" "hunter2hunter2"
expect "bob edits item" "$(printf '%s\nNetflix\n\n\n\nedited by bob\n' "$PW" | B edit netflix -v Family 2>&1)" "Saved"
expect "alice sees edit" "$(printf '%s\n' "$PW" | A get netflix -v Family -f notes 2>/dev/null)" "edited by bob"
expect "bob cannot invite" "$(B invite eve@example.com 2>&1 || true)" "admin only"
expect "bob cannot read alice personal" "$(printf '%s\n' "$PW" | B ls -v "$(printf '%s\n' "$PW" | A vaults 2>/dev/null | awk '/Personal/{print $4}')" 2>&1 || true)" "not found"

expect "second device login" "$(printf 'http://127.0.0.1:%s\nalice@example.com\n%s\n%s\n' "$PORT" "$ALICE_SK" "$PW" | HISAR_STATE=$T/alice2.json HISAR_SECRET_KEY=- hisar login 2>&1)" "Signed in as Alice"
expect "wrong password" "$(printf 'nope\n' | A ls 2>&1 || true)" "incorrect email, password or Secret Key"
expect "devices" "$(A devices)" "(this device)"
expect "emergency kit" "$(printf '%s\n' "$PW" | A emergency-kit -o "$T/kit.pdf" 2>&1)" "Saved your Emergency Kit"
expect "emergency kit is a private PDF" "$(head -c 5 "$T/kit.pdf") $(stat -c %a "$T/kit.pdf" 2>/dev/null || stat -f %Lp "$T/kit.pdf")" "%PDF- 600"
expect "emergency kit never overwrites" "$(printf '%s\n' "$PW" | A emergency-kit -o "$T/kit.pdf" 2>&1 || true)" "already exists"

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
expect "share recoverable vault" "$(printf '%s\n' "$PW" | A share Household bob@example.com 2>&1)" "as editor"
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

expect "logout" "$(A logout && A whoami 2>&1 || true)" "not logged in"

DESKTOP_INVITE=$("$BIN/hisar-server" invite --email desktop@example.com --role admin </dev/null | tail -1)
DESKTOP_RESULT=$(HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_INVITE_LINK="$DESKTOP_INVITE" \
  HISAR_TEST_RECOVERY_ADMIN_EMAIL=alice@example.com HISAR_TEST_RECOVERY_ADMIN_SECRET_KEY="$ALICE_SK" HISAR_TEST_RECOVERY_ADMIN_PASSWORD="$PW" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationDesktopFlow ./vault/ 2>&1 || true)
expect "desktop app backend against the server" "$DESKTOP_RESULT" "--- PASS: TestIntegrationDesktopFlow"

MOBILE_INVITE=$("$BIN/hisar-server" invite --email mobile@example.com </dev/null | tail -1)
MOBILE_RESULT=$(HISAR_TEST_MOBILE_INVITE_LINK="$MOBILE_INVITE" go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationMobileFlow ./hisarkit/ 2>&1 || true)
expect "iOS app backend against the server" "$MOBILE_RESULT" "--- PASS: TestIntegrationMobileFlow"
echo "end-to-end test passed"
