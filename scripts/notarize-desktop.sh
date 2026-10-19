#!/usr/bin/env bash
# Notarizes a Developer ID-signed Hisar.app or DMG with Apple's notary service, staples the ticket to it, and checks
# that Gatekeeper accepts it.
# Usage: notarize-desktop.sh <path to Hisar.app or .dmg>
# HISAR_NOTARY_KEY: the App Store Connect API key file (.p8); HISAR_NOTARY_KEY_ID and HISAR_NOTARY_ISSUER_ID: its IDs.
set -euo pipefail

target=${1:?usage: notarize-desktop.sh <path to Hisar.app or .dmg>}
: "${HISAR_NOTARY_KEY:?set HISAR_NOTARY_KEY to the App Store Connect API key file (.p8)}"
: "${HISAR_NOTARY_KEY_ID:?set HISAR_NOTARY_KEY_ID to the API key ID}"
: "${HISAR_NOTARY_ISSUER_ID:?set HISAR_NOTARY_ISSUER_ID to the API key issuer ID}"
allCredential=(--key "$HISAR_NOTARY_KEY" --key-id "$HISAR_NOTARY_KEY_ID" --issuer "$HISAR_NOTARY_ISSUER_ID")

workDirectory=$(mktemp -d)
trap 'rm -rf "$workDirectory"' EXIT
submission=$target
# The notary service takes a zip, a DMG or a package, not a bare bundle.
if [ -d "$target" ]; then
  submission="$workDirectory/$(basename "$target").zip"
  ditto -c -k --keepParent "$target" "$submission"
fi

# notarytool exits non-zero for a rejected submission too; the status below decides.
xcrun notarytool submit "$submission" "${allCredential[@]}" --wait --timeout 1h --output-format json > "$workDirectory/result.json" || true
status=$(plutil -extract status raw -o - "$workDirectory/result.json" 2>/dev/null || echo "no answer")
if [ "$status" != Accepted ]; then
  echo "notarizing $target ended with: $status" >&2
  cat "$workDirectory/result.json" >&2
  if submissionID=$(plutil -extract id raw -o - "$workDirectory/result.json" 2>/dev/null); then
    xcrun notarytool log "$submissionID" "${allCredential[@]}" >&2 || true
  fi
  exit 1
fi

xcrun stapler staple "$target"
xcrun stapler validate "$target"
if [ -d "$target" ]; then
  spctl --assess --type execute --verbose=2 "$target"
else
  spctl --assess --type open --context context:primary-signature --verbose=2 "$target"
fi
