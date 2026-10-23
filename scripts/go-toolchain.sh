#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
while read -r directive value; do
  if [ "$directive" = toolchain ]; then
    printf '%s\n' "$value"
    exit 0
  fi
done < "$root/go.work"
echo "go.work must name the release toolchain" >&2
exit 1
