#!/usr/bin/env bash
# Downloads the pinned Sparkle release into desktop/build/sparkle (Sparkle.framework and bin/sign_update), checking its SHA-256.
set -euo pipefail

sparkleVersion=2.10.0
sparkleChecksum=c2bf58aa8387266ac179357b1415d6f2635f044da8be41042af32425dae6da0c

destination="$(cd "$(dirname "$0")/.." && pwd)/desktop/build/sparkle"
isDownloaded() {
  [ "$(cat "$destination/VERSION" 2>/dev/null)" = "$sparkleVersion" ] && [ -d "$destination/Sparkle.framework" ]
}
if isDownloaded; then
  exit 0
fi

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
if isDownloaded; then
  exit 0
fi
archive="$workDirectory/Sparkle-$sparkleVersion.tar.xz"
curl -fsSL --retry 3 -o "$archive" "https://github.com/sparkle-project/Sparkle/releases/download/$sparkleVersion/Sparkle-$sparkleVersion.tar.xz"
if [ "$(shasum -a 256 "$archive" | cut -d ' ' -f 1)" != "$sparkleChecksum" ]; then
  echo "Sparkle-$sparkleVersion.tar.xz does not match the pinned SHA-256; refusing to use it" >&2
  exit 1
fi
mkdir "$workDirectory/extracted"
tar -xJf "$archive" -C "$workDirectory/extracted"

rm -rf "$destination"
mkdir -p "$destination/bin"
ditto "$workDirectory/extracted/Sparkle.framework" "$destination/Sparkle.framework"
cp "$workDirectory/extracted/bin/sign_update" "$workDirectory/extracted/bin/generate_keys" "$destination/bin/"
cp "$workDirectory/extracted/LICENSE" "$destination/LICENSE"
echo "$sparkleVersion" > "$destination/VERSION"
