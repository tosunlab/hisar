#!/usr/bin/env bash
# Tests desktop/secretclipboard against real display servers: Xvfb for X11 and a headless sway for Wayland data control,
# reading the clipboard with xclip and wl-paste. Needs the GTK 4 and WebKitGTK dev packages the desktop builds with.
# Usage: test-secret-clipboard.sh     (as root on Debian or Ubuntu, as in CI)
set -euo pipefail

bash "$(dirname "$0")/ci-apt-install.sh" xvfb xauth xclip sway wl-clipboard
cd "$(dirname "$0")/../desktop"

xvfb-run -a go test -count=1 -run '^TestX11' ./secretclipboard

runtimeDirectory=$(mktemp -d)
chmod 700 "$runtimeDirectory"
export XDG_RUNTIME_DIR=$runtimeDirectory
WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 WLR_RENDERER=pixman sway -c /dev/null >"$runtimeDirectory/sway.log" 2>&1 &
swayPID=$!
trap 'kill $swayPID 2>/dev/null || true' EXIT
for _ in $(seq 100); do
  socket=$(find "$runtimeDirectory" -maxdepth 1 -name 'wayland-*' ! -name '*.lock' -print -quit)
  [ -n "$socket" ] && break
  sleep 0.1
done
[ -n "${socket:-}" ] || { echo "sway did not start:"; cat "$runtimeDirectory/sway.log"; exit 1; }
WAYLAND_DISPLAY=$(basename "$socket") go test -count=1 -run '^TestWayland' ./secretclipboard
