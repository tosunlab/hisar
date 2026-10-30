#!/usr/bin/env bash
# Signs in to hisar-sign-in-site-fixture with desktop/axfill's Linux driver in Debian's Chromium, on Xvfb with openbox
# for the window manager and a session bus for the accessibility bus. The browser runs with ACCESSIBILITY_ENABLED=1 and
# --force-renderer-accessibility, as Hisar tells users to start it.
# Usage: test-agent-sign-in-linux.sh     (as root on Debian, as in CI)
set -euo pipefail

bash "$(dirname "$0")/ci-apt-install.sh" chromium xvfb xauth openbox dbus dbus-x11 at-spi2-core libatk-bridge2.0-0t64
cd "$(dirname "$0")/../desktop"

export HISAR_TEST_SIGN_IN_BROWSERS=${HISAR_TEST_SIGN_IN_BROWSERS:-/usr/lib/chromium/chromium}
# The window manager sets _NET_ACTIVE_WINDOW, the window Hisar signs in to.
xvfb-run -a -s "-screen 0 1280x1000x24" dbus-run-session -- sh -c '
  openbox >/dev/null 2>&1 &
  set -o pipefail
  go test -count=1 -v -run "^TestIntegrationSignIn$" ./axfill 2>&1 | tee /tmp/agent-sign-in.log
  status=$?
  if grep -- "--- SKIP" /tmp/agent-sign-in.log; then exit 1; fi
  exit $status'
