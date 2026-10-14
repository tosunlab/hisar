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
# Steps that clear HISAR_STANDALONE must never start the Hisar app installed on this machine.
export HISAR_NO_LAUNCH=1
# Invites and share invitations go to a local SMTP sink that writes each email to $MAIL.
export HISAR_SMTP_HOST=127.0.0.1 HISAR_SMTP_PORT=$SMTP_PORT HISAR_SMTP_USERNAME=hisar HISAR_SMTP_PASSWORD=end-to-end HISAR_SMTP_FROM='Hisar <noreply@hisar.test>'
: "${HISAR_DATABASE_URL:?set HISAR_DATABASE_URL to an empty database}"
# Desktop updates come from a local fake of GitHub Releases, never from github.com.
GITHUB_PORT=${END_TO_END_GITHUB_PORT:-18092}
export HISAR_DESKTOP_UPDATE_GITHUB_API_URL=http://127.0.0.1:$GITHUB_PORT HISAR_DESKTOP_UPDATE_GITHUB_TOKEN=end-to-end-github-token

"$BIN/hisar-smtp-sink" -addr "127.0.0.1:$SMTP_PORT" -dir "$MAIL" >"$T/smtp.log" 2>&1 &
SINK=$!
"$BIN/hisar-fake-github" -addr "127.0.0.1:$GITHUB_PORT" -token "$HISAR_DESKTOP_UPDATE_GITHUB_TOKEN" >"$T/github.log" 2>&1 &
GITHUB=$!
"$BIN/hisar-server" serve -addr "127.0.0.1:$PORT" >"$T/server.log" 2>&1 &
SERVER=$!
trap 'kill $SERVER $SINK $GITHUB 2>/dev/null || true; rm -rf "$T"' EXIT
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

expect "standalone read of a secret reference" "[$(printf '%s\n' "$PW" | A read hisar://Personal/GitHub/password 2>/dev/null)]" "[$GEN]"
RUN_EXIT=0
RUN_OUTPUT=$(printf '%s\n' "$PW" | GITHUB_PASSWORD=hisar://Personal/GitHub/password A run -- sh -c 'echo "password is $GITHUB_PASSWORD"; exit 3' 2>&1) || RUN_EXIT=$?
expect "standalone run masks the secret" "$RUN_OUTPUT" "password is <concealed by hisar>"
[[ $RUN_EXIT -eq 3 && "$RUN_OUTPUT" != *"$GEN"* ]] || fail "standalone run: exit code $RUN_EXIT"
expect "standalone run names an unknown reference" "$(printf '%s\n' "$PW" | GITHUB_PASSWORD=hisar://Personal/Nothing/password A run -- sh -c 'echo ran' 2>&1 || true)" 'hisar://Personal/Nothing/password: no item in vault "Personal" is titled "Nothing"'

SHARE_URL=$(printf '%s\n' "$PW" | A share-link github --field password --views 2 --expires 1d 2>"$T/share-link.txt")
expect "share link is printed with its key in the fragment" "$SHARE_URL" "$HISAR_PUBLIC_URL/s/"
[[ "$SHARE_URL" == *"#"* ]] || fail "share link without a key: $SHARE_URL"
expect "share link summary" "$(cat "$T/share-link.txt")" 'Share link for "GitHub" (password)'
SHARE_ID=${SHARE_URL##*/s/}
SHARE_ID=${SHARE_ID%%#*}
expect "share link listed" "$(printf '%s\n' "$PW" | A share-links 2>/dev/null)" "$SHARE_ID"
expect "first open decrypts the password from the public endpoint" "$(hisar share-link open "$SHARE_URL" 2>&1)" "password:  $GEN"
expect "second open is the last view" "$(hisar share-link open "$SHARE_URL" 2>&1)" "That was the last view"
expect "third open is refused" "$(hisar share-link open "$SHARE_URL" 2>&1 || true)" "this link has expired or was already viewed"
USED_UP=$(curl -s -w ' %{http_code}' "http://127.0.0.1:$PORT/api/v1/public/share-links/$SHARE_ID")
NEVER_EXISTED=$(curl -s -w ' %{http_code}' "http://127.0.0.1:$PORT/api/v1/public/share-links/AAAAAAAAAAAAAAAAAAAAAA")
expect "used-up link is not found" "$USED_UP" '{"error":"not found"} 404'
[[ "$USED_UP" == "$NEVER_EXISTED" ]] || fail "a used-up link answers differently from one that never existed: $NEVER_EXISTED"
echo "ok   a used-up link and an unknown ID answer the same"

TEXT_URL=$(printf '%s\nlink passphrase\nlink passphrase\ndeploy token: abc123\n' "$PW" | A share-link --text --title Deploy --passphrase --views 3 --expires 1h 2>/dev/null)
expect "text share link" "$TEXT_URL" "$HISAR_PUBLIC_URL/s/"
TEXT_ID=${TEXT_URL##*/s/}
TEXT_ID=${TEXT_ID%%#*}
expect "wrong passphrase is refused" "$(printf 'wrong passphrase\n' | hisar share-link open "$TEXT_URL" 2>&1 || true)" "doesn't open this link"
expect "passphrase opens the text" "$(printf 'link passphrase\n' | hisar share-link open "$TEXT_URL" 2>&1)" "deploy token: abc123"
expect "text link listed with its views" "$(printf '%s\n' "$PW" | A share-links 2>/dev/null)" "2/3"
expect "revoke" "$(printf '%s\n' "$PW" | A share-link revoke "$TEXT_ID" 2>&1)" "Revoked the share link"
expect "revoked link no longer opens" "$(printf 'link passphrase\n' | hisar share-link open "$TEXT_URL" 2>&1 || true)" "this link has expired or was already viewed"
[[ "$(printf '%s\n' "$PW" | A share-links 2>&1)" != *"$TEXT_ID"* ]] || fail "the revoked link is still listed"
echo "ok   the revoked link is gone from the list"

printf '%s\n' "$PW" | A vault-create Family 2>/dev/null
printf '%s\nNetflix\nfamily@example.com\nhttps://netflix.com\nhunter2hunter2\nshared login\n' "$PW" | A add -v Family 2>/dev/null

INVITE_OUTPUT=$(A invite bob@example.com 2>&1)
expect "invite by email" "$INVITE_OUTPUT" "Invitation sent to bob@example.com."
[[ "$INVITE_OUTPUT" != *"/invite/"* ]] || fail "the invite response shows the link: $INVITE_OUTPUT"
echo "ok   the invite response has no link or code"
BOB_MAIL=$(wait_for_mail bob@example.com 0)
expect "invite email" "$BOB_MAIL" "Alice (alice@example.com) invited you to Hisar"
BOB_CODE=$(printf '%s' "$BOB_MAIL" | get_invite_code)
expect "invite email carries the link and the code" "$BOB_MAIL" "$HISAR_PUBLIC_URL/invite/$BOB_CODE"$'\n\nOr paste this invite code under Create account in Hisar:\n'"$BOB_CODE"
BOB_INVITE_PAGE=$(curl -s "$HISAR_PUBLIC_URL/invite/$BOB_CODE")
expect "the emailed invite link opens a page naming the inviter" "$BOB_INVITE_PAGE" "Alice (alice@example.com)</span> invited you to Hisar"
expect "the invite page shows the code to paste" "$BOB_INVITE_PAGE" "data-testid=\"invite-code\">$BOB_CODE</code>"
expect "the invite page opens the app with the invite" "$BOB_INVITE_PAGE" "href=\"hisar://invite/$BOB_CODE?server=http%3A%2F%2F127.0.0.1%3A$PORT\""
printf 'bob@example.com\nBob\n%s\n%s\n' "$PW" "$PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=- hisar signup "$HISAR_PUBLIC_URL/invite/$BOB_CODE" 2>"$T/bob-signup.txt"
BOB_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/bob-signup.txt")
expect "bob signs up from the emailed code" "$BOB_SK" "H1-"
expect "a used invite's page says it isn't valid" "$(curl -s -w ' %{http_code}' "$HISAR_PUBLIC_URL/invite/$BOB_CODE")" "This invite isn't valid</h1>"
expect "a used invite's page answers like an unknown one" "$(curl -s -o /dev/null -w '%{http_code}' "$HISAR_PUBLIC_URL/invite/$BOB_CODE") $(curl -s -o /dev/null -w '%{http_code}' "$HISAR_PUBLIC_URL/invite/never-issued")" "404 404"
B() { HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=$BOB_SK hisar "$@"; }
# Bob keeps the first master password; Alice changes hers below.
BOB_PW=$PW

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

for REMOVED_COMMAND in recover recovery-setup recovery-enable recovery-start recovery-list recovery-complete; do
  expect "hisar $REMOVED_COMMAND was removed" "$(A "$REMOVED_COMMAND" bob@example.com 2>&1 || true)" "account recovery was removed; sign in with your Emergency Kit"
done
expect "the recovery API is gone" "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $ALICE_TOKEN" "http://127.0.0.1:$PORT/api/v1/recovery/keyset")" "404"
expect "the admin recovery API is gone" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $ALICE_TOKEN" "http://127.0.0.1:$PORT/api/v1/admin/recovery/$(sed -n 's/.*"accountId": *"\([^"]*\)".*/\1/p' "$T/bob.json")")" "404"
printf '%s\n' "$PW" | A vault-create Household 2>/dev/null
expect "shared vault without a recovery key" "$(printf '%s\n' "$PW" | A vaults 2>&1)" "Household"
expect "Emergency Kit saved" "$(printf '%s\n' "$PW" | B emergency-kit -o "$T/bob-kit.pdf" 2>&1)" "Saved your Emergency Kit"
[[ "$(head -c 5 "$T/bob-kit.pdf")" == "%PDF-" ]] || fail "the Emergency Kit is not a PDF"
expect "sign in on a new device with the Emergency Kit" "$(printf 'http://127.0.0.1:%s\nbob@example.com\n%s\n%s\n' "$PORT" "$BOB_SK" "$PW" | HISAR_STATE=$T/bob-new-device.json HISAR_SECRET_KEY=- hisar login 2>&1)" "Signed in as Bob"
expect "wrong master password with the Emergency Kit" "$(printf 'http://127.0.0.1:%s\nbob@example.com\n%s\n%s\n' "$PORT" "$BOB_SK" "forgotten password" | HISAR_STATE=$T/bob-forgotten.json HISAR_SECRET_KEY=- hisar login 2>&1 || true)" "incorrect email, password or Secret Key"

printf '%s\nDeploy\nci\nhttps://ci.example.com\ndeploy-pass-1\n\n' "$PW" | A add -v Work 2>/dev/null
NEW_PW='a brand new master password'
ALICE_MAIL_COUNT=$(count_mail alice@example.com)
expect "change master password" "$(printf '%s\nshort\n%s\n%s\ny\n' "$PW" "$NEW_PW" "$NEW_PW" | A account change-password 2>&1)" "Changed your master password. 1 other device was signed out."
expect "the other device is signed out after the change" "$(printf '%s\n' "$NEW_PW" | HISAR_STATE=$T/alice2.json HISAR_SECRET_KEY=$ALICE_SK hisar ls 2>&1 || true)" "this device was signed out"
expect "the old master password no longer unlocks" "$(printf '%s\n' "$PW" | A ls 2>&1 || true)" "incorrect email, password or Secret Key"
expect "the new master password unlocks" "$(printf '%s\n' "$NEW_PW" | A get github -f password 2>/dev/null)" "$GEN"
expect "password change email" "$(wait_for_mail alice@example.com "$ALICE_MAIL_COUNT")" "Your other device was signed out."
expect "the other device signs in with the new master password" "$(printf 'http://127.0.0.1:%s\nalice@example.com\n%s\n%s\n' "$PORT" "$ALICE_SK" "$NEW_PW" | HISAR_STATE=$T/alice3.json HISAR_SECRET_KEY=- hisar login 2>&1)" "Signed in as Alice"

expect "replace Secret Key" "$(printf '%s\ny\n' "$NEW_PW" | A account replace-secret-key -o "$T/new-kit.pdf" 2>"$T/replace.txt"; cat "$T/replace.txt")" "Saved your new Emergency Kit to $T/new-kit.pdf"
NEW_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/replace.txt" | head -1)
[[ -n "$NEW_SK" && "$NEW_SK" != "$ALICE_SK" ]] || fail "no new Secret Key: $(cat "$T/replace.txt")"
expect "the new Emergency Kit is a private PDF" "$(head -c 5 "$T/new-kit.pdf") $(stat -c %a "$T/new-kit.pdf" 2>/dev/null || stat -f %Lp "$T/new-kit.pdf")" "%PDF- 600"
expect "the old Emergency Kit no longer signs in" "$(printf 'http://127.0.0.1:%s\nalice@example.com\n%s\n%s\n' "$PORT" "$ALICE_SK" "$NEW_PW" | HISAR_STATE=$T/alice-old-kit.json HISAR_SECRET_KEY=- hisar login 2>&1 || true)" "incorrect email, password or Secret Key"
ALICE_SK=$NEW_SK
expect "the new Secret Key unlocks" "$(printf '%s\n' "$NEW_PW" | A get github -f password 2>/dev/null)" "$GEN"
expect "the shared vault still decrypts for the owner" "$(printf '%s\n' "$NEW_PW" | A get deploy -v Work -f password 2>/dev/null)" "deploy-pass-1"
expect "and for the other member" "$(printf '%s\n' "$PW" | C get deploy -v Work -f password 2>/dev/null)" "deploy-pass-1"
expect "the new Emergency Kit signs in on a new device" "$(printf 'http://127.0.0.1:%s\nalice@example.com\n%s\n%s\n' "$PORT" "$NEW_SK" "$NEW_PW" | HISAR_STATE=$T/alice-new-kit.json HISAR_SECRET_KEY=- hisar login 2>&1)" "Signed in as Alice"
PW=$NEW_PW

DORA_INV=$("$BIN/hisar-server" invite --email dora@example.com </dev/null | tail -1)
printf 'dora@example.com\nDora\n%s\n%s\n' "$PW" "$PW" | HISAR_STATE=$T/dora.json HISAR_SECRET_KEY=- hisar signup "$DORA_INV" 2>"$T/dora-signup.txt"
DORA_SK=$(grep -oE 'H1-[0-9A-Z-]+' "$T/dora-signup.txt")
D() { HISAR_STATE=$T/dora.json HISAR_SECRET_KEY=$DORA_SK hisar "$@"; }
printf '%s\n' "$PW" | D vault-create Garden 2>/dev/null
expect "account deletion shows what is deleted" "$(printf '%s\nwrong@example.com\n' "$PW" | D account delete 2>&1 || true)" 'Deleted with its items: "Garden"'
expect "a wrong email deletes nothing" "$(printf '%s\nwrong@example.com\n' "$PW" | D account delete 2>&1 || true)" "nothing was deleted"
expect "a wrong master password deletes nothing" "$(printf 'wrong password\n' | D account delete --email dora@example.com 2>&1 || true)" "the master password is not correct"
expect "delete account" "$(printf '%s\ndora@example.com\n' "$PW" | D account delete 2>&1)" "Deleted your Hisar account."
expect "account deletion email" "$(wait_for_mail dora@example.com 0)" "Your Hisar account dora@example.com was deleted"
expect "the deleted account no longer signs in" "$(printf 'http://127.0.0.1:%s\ndora@example.com\n%s\n%s\n' "$PORT" "$DORA_SK" "$PW" | HISAR_STATE=$T/dora-again.json HISAR_SECRET_KEY=- hisar login 2>&1 || true)" "incorrect email, password or Secret Key"
expect "the deleted account's device is signed out" "$(D whoami 2>&1 || true)" "not logged in"

APPCAST_URL=http://127.0.0.1:$PORT/api/v1/desktop/updates/appcast.xml
expect "desktop updates need a session" "$(curl -s "$APPCAST_URL")" "unauthorized"
APPCAST=$(curl -s -H "Authorization: Bearer $ALICE_TOKEN" "$APPCAST_URL")
expect "the appcast offers the published desktop release from GitHub" "$APPCAST" "<enclosure url=\"$HISAR_PUBLIC_URL/api/v1/desktop/updates/0.2.0/Hisar-0.2.0.zip\" length=\"27\""
[[ "$APPCAST" != *"0.3.0"* ]] || fail "the appcast offers the unpublished 0.3.0"
echo "ok   the appcast leaves out the unpublished release"
expect "the update archive is proxied from GitHub" "$(curl -s -H "Authorization: Bearer $ALICE_TOKEN" "http://127.0.0.1:$PORT/api/v1/desktop/updates/0.2.0/Hisar-0.2.0.zip")" "end-to-end archive of 0.2.0"
expect "an unpublished release can't be downloaded" "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $ALICE_TOKEN" "http://127.0.0.1:$PORT/api/v1/desktop/updates/0.3.0/Hisar-0.3.0.zip")" "404"

expect "logout" "$(A logout && A whoami 2>&1 || true)" "not logged in"

DESKTOP_INVITE=$("$BIN/hisar-server" invite --email desktop@example.com --role admin </dev/null | tail -1)
DESKTOP_MEMBER_INVITE=$("$BIN/hisar-server" invite --email desktop-member@example.com </dev/null | tail -1)
DESKTOP_RESULT=$(HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_INVITE_LINK="$DESKTOP_INVITE" \
  HISAR_TEST_MEMBER_INVITE_LINK="$DESKTOP_MEMBER_INVITE" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationDesktopFlow ./vault/ 2>&1 || true)
expect "desktop app backend against the server" "$DESKTOP_RESULT" "--- PASS: TestIntegrationDesktopFlow"

LIFECYCLE_INVITE=$("$BIN/hisar-server" invite --email lifecycle@example.com </dev/null | tail -1)
LIFECYCLE_RESULT=$(HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_LIFECYCLE_INVITE_LINK="$LIFECYCLE_INVITE" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationAccountLifecycle ./vault/ 2>&1 || true)
expect "the Mac changes the master password and replaces the Secret Key, signing out its other device, then deletes the account" "$LIFECYCLE_RESULT" "--- PASS: TestIntegrationAccountLifecycle"

COMMAND_LINE_INVITE=$("$BIN/hisar-server" invite --email cli@example.com </dev/null | tail -1)
COMMAND_LINE_RESULT=$(HISAR_STANDALONE= HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_CLI_INVITE_LINK="$COMMAND_LINE_INVITE" HISAR_TEST_CLI_BINARY="$BIN/hisar" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationCommandLineThroughApp ./vault/ 2>&1 || true)
expect "hisar through the desktop app backend" "$COMMAND_LINE_RESULT" "--- PASS: TestIntegrationCommandLineThroughApp"

AGENT_INVITE=$("$BIN/hisar-server" invite --email agent@example.com </dev/null | tail -1)
AGENT_RESULT=$(HISAR_STANDALONE= HISAR_TEST_SERVER_URL="http://127.0.0.1:$PORT" HISAR_TEST_AGENT_INVITE_LINK="$AGENT_INVITE" HISAR_TEST_CLI_BINARY="$BIN/hisar" \
  go -C "$ROOT/desktop" test -count=1 -v -run TestIntegrationAgentThroughApp ./vault/ 2>&1 || true)
expect "hisar mcp speaks MCP over stdio through the desktop app backend and shows agents only their vaults" "$AGENT_RESULT" "--- PASS: TestIntegrationAgentThroughApp"

MOBILE_INVITE=$("$BIN/hisar-server" invite --email mobile@example.com </dev/null | tail -1)
MOBILE_RESULT=$(HISAR_TEST_MOBILE_INVITE_LINK="$MOBILE_INVITE" go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationMobileFlow ./hisarkit/ 2>&1 || true)
expect "iOS app backend against the server" "$MOBILE_RESULT" "--- PASS: TestIntegrationMobileFlow"

MOBILE_SHARING_OWNER_INVITE=$("$BIN/hisar-server" invite --email mobile-sharing-owner@example.com </dev/null | tail -1)
MOBILE_SHARING_RECIPIENT_INVITE=$("$BIN/hisar-server" invite --email mobile-sharing-recipient@example.com </dev/null | tail -1)
MOBILE_SHARING_RESULT=$(HISAR_TEST_MOBILE_SHARING_OWNER_INVITE_LINK="$MOBILE_SHARING_OWNER_INVITE" HISAR_TEST_MOBILE_SHARING_RECIPIENT_INVITE_LINK="$MOBILE_SHARING_RECIPIENT_INVITE" \
  go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationMobileSharing ./hisarkit/ 2>&1 || true)
expect "the iPhone shares a vault, completes the accepted share on its own, and the recipient reads the item" "$MOBILE_SHARING_RESULT" "--- PASS: TestIntegrationMobileSharing"

PHONE_APPROVAL_INVITE=$("$BIN/hisar-server" invite --email phone-approval@example.com </dev/null | tail -1)
PHONE_APPROVAL_RESULT=$(HISAR_TEST_PHONE_APPROVAL_INVITE_LINK="$PHONE_APPROVAL_INVITE" go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationPhoneApproval ./hisarkit/ 2>&1 || true)
expect "the iPhone approves and denies the Mac's SSH requests, refuses a forged answer, and approves after opening late" "$PHONE_APPROVAL_RESULT" "--- PASS: TestIntegrationPhoneApproval"
MOBILE_LIFECYCLE_INVITE=$("$BIN/hisar-server" invite --email mobile-lifecycle@example.com </dev/null | tail -1)
MOBILE_LIFECYCLE_RESULT=$(HISAR_TEST_MOBILE_LIFECYCLE_INVITE_LINK="$MOBILE_LIFECYCLE_INVITE" go -C "$ROOT/mobile" test -count=1 -v -run TestIntegrationMobileAccountLifecycle ./hisarkit/ 2>&1 || true)
expect "the iPhone changes the master password, replaces the Secret Key with a new Emergency Kit and deletes the account" "$MOBILE_LIFECYCLE_RESULT" "--- PASS: TestIntegrationMobileAccountLifecycle"

# Raise the minimum versions: older apps are refused with 426, current ones and unversioned callers pass.
kill "$SERVER"
wait "$SERVER" 2>/dev/null || true
HISAR_MINIMUM_VERSION_CLI=1.0.0 HISAR_MINIMUM_VERSION_DESKTOP_MACOS=0.2.0 "$BIN/hisar-server" serve -addr "127.0.0.1:$PORT" >>"$T/server.log" 2>&1 &
SERVER=$!
curl -sf --retry 20 --retry-all-errors --retry-delay 1 "http://127.0.0.1:$PORT/healthz" >/dev/null
go -C "$ROOT/cli" build -ldflags "-X main.version=0.9.0" -o "$T/hisar-0.9.0" ./cmd/hisar
go -C "$ROOT/cli" build -ldflags "-X main.version=1.0.0" -o "$T/hisar-1.0.0" ./cmd/hisar
OLD_EXIT=0
OLD_OUTPUT=$(printf '%s\n' "$BOB_PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=$BOB_SK "$T/hisar-0.9.0" vaults 2>&1) || OLD_EXIT=$?
expect "an old hisar is refused and told how to update" "$OLD_OUTPUT" "update required: the Hisar server needs hisar 1.0.0 or later, and this is 0.9.0."
[[ $OLD_EXIT -ne 0 ]] || fail "an old hisar exited with 0"
echo "ok   an old hisar exits with $OLD_EXIT"
expect "a current hisar passes" "$(printf '%s\n' "$BOB_PW" | HISAR_STATE=$T/bob.json HISAR_SECRET_KEY=$BOB_SK "$T/hisar-1.0.0" vaults 2>&1)" "Personal"
expect "a development hisar sends no version and passes" "$(printf '%s\n' "$BOB_PW" | B vaults 2>&1)" "Personal"
VERSION_CHECK_URL=http://127.0.0.1:$PORT/api/v1/version-check
expect "an old Mac app is refused before sign-in" "$(curl -s -w ' %{http_code}' -H 'Hisar-Platform: darwin/desktop' -H 'Hisar-Version: 0.1.9' "http://127.0.0.1:$PORT/api/v1/auth/prelogin?email=bob@example.com")" '{"error":"update required","minimumVersion":"0.2.0","updateURL":"https://tryhisar.com/download"} 426'
expect "a current Mac app passes the version check" "$(curl -s -o /dev/null -w '%{http_code}' -H 'Hisar-Platform: darwin/desktop' -H 'Hisar-Version: 0.2.0' "$VERSION_CHECK_URL")" "204"
expect "a pre-release of the minimum is older than the release" "$(curl -s -o /dev/null -w '%{http_code}' -H 'Hisar-Platform: linux/cli' -H 'Hisar-Version: 1.0.0-alpha-test' "$VERSION_CHECK_URL")" "426"
expect "the update feed stays open to an old Mac app" "$(curl -s -H 'Hisar-Platform: darwin/desktop' -H 'Hisar-Version: 0.1.9' "$APPCAST_URL")" "unauthorized"
expect "an old Mac app gets the appcast" "$(curl -s -H 'Hisar-Platform: darwin/desktop' -H 'Hisar-Version: 0.1.9' -H "Authorization: Bearer $BOB_TOKEN" "$APPCAST_URL")" "<sparkle:shortVersionString>0.2.0</sparkle:shortVersionString>"
expect "an old Mac app downloads the update" "$(curl -s -H 'Hisar-Platform: darwin/desktop' -H 'Hisar-Version: 0.1.9' -H "Authorization: Bearer $BOB_TOKEN" "http://127.0.0.1:$PORT/api/v1/desktop/updates/0.2.0/Hisar-0.2.0.zip")" "end-to-end archive of 0.2.0"
echo "end-to-end test passed"
