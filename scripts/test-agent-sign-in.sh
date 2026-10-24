#!/usr/bin/env bash
# Local Mac only. The terminal needs Accessibility and Screen Recording permission.
# This drives throwaway browser windows and Hisar's real approval UI. Do not run in CI.
# Needs Go, Node, Wails, Swift, Docker's hisar-pg container, sqlite3 and yabai; the built-in display alone with two Spaces.
# Official Chrome and Edge copies default to /tmp/hisar-browsers. No user profile is used.
# Password canaries fail in every output. Codes fail in agent channels; capture and
# browser-profile code residuals are listed separately, never labeled PASS.
set -euo pipefail

[[ $(uname -s) == Darwin && -z ${CI:-} ]] || { echo 'Local Mac only, not CI.' >&2; exit 1; }
ROOT=$(cd "$(dirname "$0")/.." && pwd)
export PATH="$HOME/go/bin:/opt/homebrew/bin:$PATH"
T=$(mktemp -d /tmp/hisar-asi.XXXXXX)
chmod 700 "$T"
DB="hisar_agent_sign_in_${T##*.}_$$"
DB_CREATED=0
SERVER=
RUNNER=
DRIVER=
MOUSE_FOLLOWS_FOCUS=
STATE="$T/state/desktop.json"
HASH=$(printf '%s' "$STATE" | shasum -a 256)
GRANT_SERVICE="Hisar agent grants.${HASH:0:16}"
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n $RUNNER ]]; then kill -TERM "$RUNNER" 2>/dev/null || true; wait "$RUNNER" 2>/dev/null || true; fi
  if [[ -n $SERVER ]]; then kill -TERM "$SERVER" 2>/dev/null || true; wait "$SERVER" 2>/dev/null || true; fi
  if [[ -n $DRIVER ]]; then kill -TERM "$DRIVER" 2>/dev/null || true; wait "$DRIVER" 2>/dev/null || true; fi
  if [[ -n $MOUSE_FOLLOWS_FOCUS ]]; then
    local restored=
    if ! yabai -m config mouse_follows_focus "$MOUSE_FOLLOWS_FOCUS"; then
      echo "FAILED TO RESTORE yabai mouse_follows_focus=$MOUSE_FOLLOWS_FOCUS" >&2
      status=1
    fi
    restored=$(yabai -m config mouse_follows_focus) || status=1
    if [[ $restored != "$MOUSE_FOLLOWS_FOCUS" ]]; then
      echo "FAILED TO VERIFY yabai mouse_follows_focus restore: expected=$MOUSE_FOLLOWS_FOCUS actual=$restored" >&2
      status=1
    else
      echo "Restored and verified yabai mouse_follows_focus=$restored."
    fi
  fi
  if (( status )) && [[ -f "$T/logs/harness.log" ]]; then cat "$T/logs/harness.log"; fi
  if [[ -f "$T/canaries.json" && ! -f "$T/scan-report.json" ]]; then
    local fixture
    local scan_args=()
    for fixture in "$T"/logs/site-*.jsonl; do scan_args+=(-fixture-log "$fixture"); done
    "$T/tools/test" scan -canaries "$T/canaries.json" -ocr "$T/tools/ocr" -report "$T/scan-report.json" \
      -browser-profiles "$T/profiles" "${scan_args[@]}" "$T" || status=1
  fi
  local keychain_status=0
  security delete-generic-password -s "$GRANT_SERVICE" >/dev/null 2>&1 || keychain_status=$?
  if (( keychain_status != 0 && keychain_status != 44 )); then
    echo "Could not delete $GRANT_SERVICE, security exit $keychain_status" >&2
    status=1
  fi
  if (( DB_CREATED )); then
    docker exec hisar-pg dropdb -U postgres --if-exists --force "$DB" || status=1
  fi
  rm -rf "$T" || status=1
  echo "Cleaned run $T, database $DB and Keychain service $GRANT_SERVICE."
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
MOUSE_FOLLOWS_FOCUS=$(yabai -m config mouse_follows_focus)
case "$MOUSE_FOLLOWS_FOCUS" in
  on|off) ;;
  *) echo "Unexpected yabai mouse_follows_focus=$MOUSE_FOLLOWS_FOCUS" >&2; MOUSE_FOLLOWS_FOCUS=; exit 1;;
esac
echo "Recorded yabai mouse_follows_focus=$MOUSE_FOLLOWS_FOCUS; disabling it only for this run."
yabai -m config mouse_follows_focus off
[[ $(yabai -m config mouse_follows_focus) == off ]]

CUA=${AGENT_SIGN_IN_CUA_DRIVER:-"$HOME/.hermes/tools/cua-driver-0.21.0-darwin-arm64/CuaDriver.app/Contents/MacOS/cua-driver"}
BRAVE=${AGENT_SIGN_IN_BRAVE:-/Applications/Brave Browser.app}
CHROME=${AGENT_SIGN_IN_CHROME:-/tmp/hisar-browsers/Google Chrome.app}
EDGE=${AGENT_SIGN_IN_EDGE:-/tmp/hisar-browsers/Microsoft Edge.app}
UNSUPPORTED=${AGENT_SIGN_IN_UNSUPPORTED_BROWSER:-}
if [[ -z $UNSUPPORTED ]]; then
  for app in "$HOME"/Library/Caches/ms-playwright/chromium-*/chrome-mac-arm64/Google\ Chrome\ for\ Testing.app; do
    [[ ! -d $app ]] || UNSUPPORTED=$app
  done
fi
[[ -x $CUA && -d $UNSUPPORTED ]] || { echo 'Set AGENT_SIGN_IN_CUA_DRIVER and AGENT_SIGN_IN_UNSUPPORTED_BROWSER.' >&2; exit 1; }
for tool in go npm wails3 swiftc docker sqlite3 yabai plutil; do command -v "$tool" >/dev/null; done

[[ ! -f "$ROOT/admin/wasm/wasm_exec.js" ]] || chmod u+w "$ROOT/admin/wasm/wasm_exec.js"
mkdir -p "$T/tools" "$T/run" "$T/logs" "$T/Helpers"
make -C "$ROOT" build-admin
(cd "$ROOT" && go build -o "$T/tools/hisar-server" ./server/cmd/hisar-server &&
  go build -o "$T/tools/hisar" ./cli/cmd/hisar &&
  go build -o "$T/tools/site" ./server/cmd/hisar-sign-in-site-fixture &&
  go build -o "$T/tools/test" ./desktop/cmd/hisar-agent-sign-in-test)
cp "$T/tools/hisar" "$T/Helpers/hisar"
(cd "$ROOT/desktop" && wails3 task build DEV=true OUTPUT="$T/run/Hisar")
swiftc "$ROOT/scripts/agent-sign-in/ax.swift" -o "$T/tools/ax"
swiftc "$ROOT/scripts/agent-sign-in/ocr.swift" -o "$T/tools/ocr"
swiftc "$ROOT/scripts/agent-sign-in/display.swift" -o "$T/tools/display"
export HISAR_TEST_YABAI
HISAR_TEST_YABAI=$(command -v yabai)
export HISAR_TEST_DISPLAY_LAYOUT
HISAR_TEST_DISPLAY_LAYOUT=$("$T/tools/display" layout)
echo "DISPLAY PREFLIGHT PASS layout=$HISAR_TEST_DISPLAY_LAYOUT; foreground and mouse constraints disabled."
export HISAR_TEST_CUA_SOCKET="$T/s/cua.sock"
mkdir -p "$T/s" "$T/driver-home"
HOME="$T/driver-home" "$CUA" serve --socket "$HISAR_TEST_CUA_SOCKET" --no-overlay --no-permissions-gate >"$T/logs/cua-daemon.log" 2>&1 &
DRIVER=$!
for ((attempt=0; attempt<100; attempt++)); do
  kill -0 "$DRIVER"
  [[ ! -S $HISAR_TEST_CUA_SOCKET ]] || break
  sleep 0.1
done
[[ -S $HISAR_TEST_CUA_SOCKET ]]

docker exec hisar-pg createdb -U postgres "$DB"
DB_CREATED=1
PORT=${AGENT_SIGN_IN_PORT:-18183}
export HISAR_DATABASE_URL="postgres://postgres:hisar@127.0.0.1:55432/$DB?sslmode=disable"
export HISAR_PUBLIC_URL="http://127.0.0.1:$PORT"
export HISAR_SECRETS_FILE="$T/server-secrets.json" HISAR_AUTH_BURST=1000 HISAR_NO_LAUNCH=1
"$T/tools/hisar-server" serve -addr "127.0.0.1:$PORT" >"$T/logs/server.log" 2>&1 &
SERVER=$!
for ((attempt=0; attempt<60; attempt++)); do
  kill -0 "$SERVER" || { cat "$T/logs/server.log" >&2; exit 1; }
  if curl -fsS "$HISAR_PUBLIC_URL/healthz" >/dev/null 2>&1; then break; fi
  sleep 1
done
curl -fsS "$HISAR_PUBLIC_URL/healthz" >/dev/null
INVITE_OUTPUT=$("$T/tools/hisar-server" invite --email agent-sign-in@example.com </dev/null)
INVITE=${INVITE_OUTPUT##*$'\n'}
unset INVITE_OUTPUT
ARGS=()
for browser in ${AGENT_SIGN_IN_BROWSERS:-Brave Chrome Edge}; do
  case "$browser" in
    Brave) ARGS+=(-browser "Brave=$BRAVE");;
    Chrome) ARGS+=(-browser "Chrome=$CHROME");;
    Edge) ARGS+=(-browser "Edge=$EDGE");;
    *) echo "Unknown browser $browser" >&2; exit 1;;
  esac
done
"$T/tools/test" run -dir "$T" -invite "$INVITE" -hisar "$T/tools/hisar" -desktop "$T/run/Hisar" \
  -site "$T/tools/site" -ax "$T/tools/ax" -ocr "$T/tools/ocr" -cua-driver "$CUA" \
  -unsupported-browser "$UNSUPPORTED" -scenarios "${AGENT_SIGN_IN_SCENARIOS:-}" "${ARGS[@]}" >"$T/logs/harness.log" 2>&1 &
RUNNER=$!
RESULT=0
wait "$RUNNER" || RESULT=$?
RUNNER=
unset INVITE
cat "$T/logs/harness.log"
# The reference and fixture POST records deliberately contain test secrets, never agent-visible output.
SCAN_ARGS=()
for fixture in "$T"/logs/site-*.jsonl; do SCAN_ARGS+=(-fixture-log "$fixture"); done
"$T/tools/test" scan -canaries "$T/canaries.json" -ocr "$T/tools/ocr" -report "$T/scan-report.json" \
  -browser-profiles "$T/profiles" "${SCAN_ARGS[@]}" "$T" || RESULT=1
exit "$RESULT"
