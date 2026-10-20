#!/usr/bin/env bash
# Downloads the pinned Sparkle release into desktop/build/sparkle (Sparkle.framework and bin/sign_update), checking its SHA-256.
set -euo pipefail

sparkleVersion=2.10.0
sparkleChecksum=c2bf58aa8387266ac179357b1415d6f2635f044da8be41042af32425dae6da0c

destination="$(cd "$(dirname "$0")/.." && pwd)/desktop/build/sparkle"

# The universal build runs this for both architectures at once.
lockDirectory="$destination.lock"
mkdir -p "$(dirname "$destination")"
for attempt in $(seq 300); do
  mkdir "$lockDirectory" 2>/dev/null && break
  if [ "$attempt" = 300 ]; then
    echo "timed out waiting for $lockDirectory; remove it if no other build is running" >&2
    exit 1
  fi
  sleep 1
done
workDirectory=$(mktemp -d)
trap 'rm -rf "$workDirectory"; rmdir "$lockDirectory"' EXIT
archive="$destination.tar.xz"
hasVerifiedArchive() {
  [ -f "$archive" ] && [ "$(shasum -a 256 "$archive" | cut -d ' ' -f 1)" = "$sparkleChecksum" ]
}
if ! hasVerifiedArchive; then
  curl -fsSL --retry 3 -o "$workDirectory/download.tar.xz" "https://github.com/sparkle-project/Sparkle/releases/download/$sparkleVersion/Sparkle-$sparkleVersion.tar.xz"
  if [ "$(shasum -a 256 "$workDirectory/download.tar.xz" | cut -d ' ' -f 1)" != "$sparkleChecksum" ]; then
    echo "Sparkle-$sparkleVersion.tar.xz does not match the pinned SHA-256; refusing to use it" >&2
    exit 1
  fi
  mv "$workDirectory/download.tar.xz" "$archive"
fi
mkdir "$workDirectory/extracted"
tar -xJf "$archive" -C "$workDirectory/extracted"
expected="$workDirectory/expected"
mkdir -p "$expected/bin"
ditto "$workDirectory/extracted/Sparkle.framework" "$expected/Sparkle.framework"
cp "$workDirectory/extracted/bin/sign_update" "$workDirectory/extracted/bin/generate_keys" "$expected/bin/"
cp "$workDirectory/extracted/LICENSE" "$expected/LICENSE"
echo "$sparkleVersion" > "$expected/VERSION"

# Compare with the verified archive, not a manifest stored beside the writable cache.
# An unchanged cache stays in place while the other architecture is linking against it.
if /usr/bin/python3 -I - "$expected" "$destination" <<'PY'
import hashlib
import os
import stat
import sys

def snapshot(path):
    metadata = os.lstat(path)
    mode = metadata.st_mode
    if stat.S_ISLNK(mode):
        return mode, os.readlink(path)
    if stat.S_ISDIR(mode):
        return mode, {entry.name: snapshot(entry.path) for entry in os.scandir(path)}
    if stat.S_ISREG(mode):
        digest = hashlib.sha256()
        with open(path, "rb") as source:
            for block in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(block)
        return mode, digest.digest()
    raise ValueError("unsupported cached file type")

try:
    matches = snapshot(sys.argv[1]) == snapshot(sys.argv[2])
except (OSError, ValueError):
    matches = False
sys.exit(0 if matches else 10)
PY
then
  exit 0
else
  status=$?
  if [ "$status" != 10 ]; then
    echo "Sparkle cache comparison failed with status $status; leaving the cache unchanged" >&2
    exit "$status"
  fi
fi
rm -rf "$destination"
mv "$expected" "$destination"
