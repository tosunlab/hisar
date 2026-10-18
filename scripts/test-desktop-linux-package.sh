#!/usr/bin/env bash
# Installs a Hisar .deb, starts the app hidden on a virtual display and checks that it serves the command-line socket
# and the SSH agent socket, and that the packaged hisar reaches the app through them.
# Usage: test-desktop-linux-package.sh <Hisar-<version>-linux-<arch>.deb>     (as root on Debian or Ubuntu, as in CI)
set -euo pipefail

package=${1:?usage: test-desktop-linux-package.sh <package.deb>}
apt-get install -y -qq "$(realpath "$package")" xvfb xauth dbus-x11 openssh-client >/dev/null
for path in /usr/bin/Hisar /usr/bin/hisar /usr/lib/hisar/hisar-browser-host; do
  test -x "$path" || { echo "$path is not installed"; exit 1; }
done

HOME=$(mktemp -d)
export HOME
# A container has no user namespaces for WebKit's sandbox, nor a GPU; neither matters to the sockets checked here.
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 LIBGL_ALWAYS_SOFTWARE=1
log=$HOME/hisar.log

xvfb-run -a dbus-run-session -- bash -c '
  set -euo pipefail
  /usr/bin/Hisar --hidden >"$0" 2>&1 &
  appPID=$!
  trap "kill $appPID 2>/dev/null || true" EXIT
  for _ in $(seq 120); do
    if [ -S "$HOME/.hisar/cli.sock" ] && [ -S "$HOME/.hisar/agent.sock" ]; then break; fi
    kill -0 "$appPID" 2>/dev/null || { echo "Hisar exited:"; cat "$0"; exit 1; }
    sleep 0.5
  done
  [ -S "$HOME/.hisar/cli.sock" ] && [ -S "$HOME/.hisar/agent.sock" ] || { echo "Hisar did not serve its sockets in 60 s:"; cat "$0"; exit 1; }
  status=$(HISAR_NO_LAUNCH=1 /usr/bin/hisar status)
  echo "$status"
  grep -q "^mode:    Hisar app (" <<<"$status" || { echo "hisar did not use the app"; exit 1; }
  grep -q "the Hisar app is not signed in" <<<"$status" || { echo "unexpected app state"; exit 1; }
  SSH_AUTH_SOCK=$HOME/.hisar/agent.sock ssh-add -l >/dev/null 2>&1 || [ $? -eq 1 ] || { echo "the SSH agent did not answer"; exit 1; }
' "$log"
echo "the installed Hisar serves hisar and the SSH agent"
