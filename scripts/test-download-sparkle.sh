#!/usr/bin/env bash
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
workDirectory=$(mktemp -d)
trap 'rm -rf "$workDirectory"' EXIT
mkdir -p "$workDirectory/scripts" "$workDirectory/desktop/build/sparkle/Sparkle.framework"
cp "$scriptDirectory/download-sparkle.sh" "$workDirectory/scripts/"
download="$workDirectory/scripts/download-sparkle.sh"
cache="$workDirectory/desktop/build/sparkle"
printf '2.10.0\n' > "$cache/VERSION"
printf 'poison\n' > "$cache/Sparkle.framework/poison"
bash "$download"
test ! -e "$cache/Sparkle.framework/poison"
file "$cache/bin/sign_update"
original=$(shasum -a 256 "$cache/bin/sign_update")
printf '#!/bin/sh\nexit 0\n' > "$cache/bin/sign_update"
bash "$download"
test "$(shasum -a 256 "$cache/bin/sign_update")" = "$original"
rm "$cache/bin/sign_update"
ln -s /bin/true "$cache/bin/sign_update"
printf 'extra\n' > "$cache/extra"
bash "$download"
test ! -L "$cache/bin/sign_update"
test ! -e "$cache/extra"
test "$(shasum -a 256 "$cache/bin/sign_update")" = "$original"

inode=$(stat -f %i "$cache/Sparkle.framework")
bash "$download" &
first=$!
bash "$download" &
second=$!
wait "$first"
wait "$second"
test "$(stat -f %i "$cache/Sparkle.framework")" = "$inode"
printf '%s\n' 'Poisoned framework, modified signer, symlink and extra file repaired.' \
  'Concurrent verified-cache consumers preserve the framework inode.'

sed "s|/usr/bin/python3|$workDirectory/unavailable-python3|" "$download" > "$workDirectory/scripts/unavailable-python-download.sh"
for status in 1 126 127; do
  printf '#!/bin/sh\nexit %s\n' "$status" > "$workDirectory/unavailable-python3"
  chmod +x "$workDirectory/unavailable-python3"
  if bash "$workDirectory/scripts/unavailable-python-download.sh"; then
    echo "Unavailable Python with status $status unexpectedly accepted" >&2
    exit 1
  else
    actual=$?
    if [ "$actual" != "$status" ]; then
      echo "Expected Python failure status $status, got $actual" >&2
      exit 1
    fi
  fi
  if [ "$(stat -f %i "$cache/Sparkle.framework")" != "$inode" ]; then
    echo "Python failure replaced the verified framework" >&2
    exit 1
  fi
done
printf '%s\n' 'Python startup failures leave the verified cache unchanged and fail closed.'

printf 'corrupt archive\n' > "$cache.tar.xz"
mkdir "$workDirectory/no-network"
printf '#!/bin/sh\nexit 99\n' > "$workDirectory/no-network/curl"
chmod +x "$workDirectory/no-network/curl"
if PATH="$workDirectory/no-network:$PATH" bash "$download"; then
  echo 'Corrupt archive unexpectedly accepted with the network unavailable' >&2
  exit 1
fi
printf '%s\n' 'Corrupt cached archive fails closed when its replacement cannot be downloaded.'
