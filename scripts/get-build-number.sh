#!/usr/bin/env bash
# Prints the CFBundleVersion for a MAJOR.MINOR.PATCH release: MAJOR*1000000 + MINOR*1000 + PATCH.
set -euo pipefail

version=${1:?usage: get-build-number.sh <MAJOR.MINOR.PATCH>}
if ! [[ "$version" =~ ^(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})$ ]]; then
  echo "version $version is not MAJOR.MINOR.PATCH with parts up to 999" >&2
  exit 1
fi
echo $((BASH_REMATCH[1] * 1000000 + BASH_REMATCH[2] * 1000 + BASH_REMATCH[3]))
