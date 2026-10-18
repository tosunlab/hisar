#!/usr/bin/env bash
# Installs a Hisar .deb, or takes a Hisar AppImage, starts the app hidden on a virtual display and checks that it serves
# the command-line socket and the SSH agent socket, and that its hisar reaches the app through them.
# Usage: test-desktop-linux-package.sh <Hisar-<version>-linux-<arch>.deb|.AppImage>     (as root on Debian or Ubuntu, as in CI)
set -euo pipefail

package=$(realpath "${1:?usage: test-desktop-linux-package.sh <package.deb|package.AppImage>}")
HOME=$(mktemp -d)
export HOME
case $package in
  *.deb)
    apt-get install -y -qq "$package" xvfb xauth dbus-x11 openssh-client >/dev/null
    for path in /usr/bin/Hisar /usr/bin/hisar /usr/lib/hisar/hisar-browser-host; do
      test -x "$path" || { echo "$path is not installed"; exit 1; }
    done
    appCommand=(/usr/bin/Hisar)
    commandLine=/usr/bin/hisar
    ;;
  *.AppImage)
    apt-get install -y -qq libgtk-4-1 libwebkitgtk-6.0-4 xvfb xauth dbus-x11 openssh-client >/dev/null
    # A container has no FUSE to mount the AppImage; the runtime extracts it instead.
    export APPIMAGE_EXTRACT_AND_RUN=1
    appCommand=("$package")
    # The app copies hisar out of the AppImage, with the launcher hisar starts it through (desktop/portable).
    commandLine=$HOME/.local/share/hisar/bin/hisar
    ;;
  *) echo "$package is neither a .deb nor an AppImage"; exit 1 ;;
esac

# A container has no user namespaces for WebKit's sandbox, nor a GPU; neither matters to the sockets checked here.
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 LIBGL_ALWAYS_SOFTWARE=1
log=$HOME/hisar.log

xvfb-run -a dbus-run-session -- bash -c '
  set -euo pipefail
  log=$1 commandLine=$2
  shift 2
  "$@" --hidden >"$log" 2>&1 &
  appPID=$!
  trap "kill $appPID 2>/dev/null || true" EXIT
  for _ in $(seq 120); do
    if [ -S "$HOME/.hisar/cli.sock" ] && [ -S "$HOME/.hisar/agent.sock" ] && [ -x "$commandLine" ]; then break; fi
    kill -0 "$appPID" 2>/dev/null || { echo "Hisar exited:"; cat "$log"; exit 1; }
    sleep 0.5
  done
  [ -S "$HOME/.hisar/cli.sock" ] && [ -S "$HOME/.hisar/agent.sock" ] || { echo "Hisar did not serve its sockets in 60 s:"; cat "$log"; exit 1; }
  status=$(HISAR_NO_LAUNCH=1 "$commandLine" status)
  echo "$status"
  grep -q "^mode:    Hisar app (" <<<"$status" || { echo "hisar did not use the app"; exit 1; }
  grep -q "the Hisar app is not signed in" <<<"$status" || { echo "unexpected app state"; exit 1; }
  SSH_AUTH_SOCK=$HOME/.hisar/agent.sock ssh-add -l >/dev/null 2>&1 || [ $? -eq 1 ] || { echo "the SSH agent did not answer"; exit 1; }
  case "$1" in
    *.AppImage) [ -x "$HOME/.local/share/hisar/bin/hisar-app" ] && [ -x "$HOME/.local/share/hisar/bin/hisar-browser-host" ] ||
      { echo "the AppImage did not copy out its launcher and browser host"; exit 1; } ;;
  esac
' _ "$log" "$commandLine" "${appCommand[@]}"
echo "$(basename "$package") serves hisar and the SSH agent"
