#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
toolchain=$(bash "$root/scripts/go-toolchain.sh")
version=${toolchain#go}
expected=$(printf '%s\n%s\n' "$version" "$version")
dockerVersions=$(awk '/^FROM / { for (i = 2; i <= NF; i++) if ($i ~ /^golang:/) { sub(/^golang:/, "", $i); print $i } }' "$root/deploy/Dockerfile")
flatpakVersions=$(awk '$1 == "url:" && $2 ~ /^https:\/\/go.dev\/dl\/go/ { version = $2; sub(/^.*\/go/, "", version); sub(/\.linux-.*$/, "", version); print version }' "$root/desktop/build/linux/flatpak/com.tryhisar.hisar.yml")
if [ "$dockerVersions" != "$expected" ]; then
  printf 'Expected both Docker golang image tags to be %s, got:\n%s\n' "$version" "$dockerVersions" >&2
  exit 1
fi
if [ "$flatpakVersions" != "$expected" ]; then
  printf 'Expected both Flatpak Go archive versions to be %s, got:\n%s\n' "$version" "$flatpakVersions" >&2
  exit 1
fi
printf 'Docker and Flatpak toolchains match %s.\n' "$toolchain"
