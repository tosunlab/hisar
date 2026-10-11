#!/usr/bin/env bash
# Prints the CFBundleVersion for a MAJOR.MINOR.PATCH release: MAJOR*1000000 + MINOR*1000 + PATCH.
# A custom build (MAJOR.MINOR.PATCH-<label>) gets one less, so the release it leads up to updates it.
set -euo pipefail

version=${1:?usage: get-build-number.sh <MAJOR.MINOR.PATCH[-label]>}
if ! [[ "$version" =~ ^(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "version $version is not MAJOR.MINOR.PATCH (parts up to 999) with an optional -label" >&2
  exit 1
fi
buildNumber=$((BASH_REMATCH[1] * 1000000 + BASH_REMATCH[2] * 1000 + BASH_REMATCH[3]))
if [[ -n "${BASH_REMATCH[4]}" ]]; then
  [[ "$buildNumber" -gt 1 ]] || { echo "a custom build needs a version above 0.0.1" >&2; exit 1; }
  buildNumber=$((buildNumber - 1))
fi
echo "$buildNumber"
