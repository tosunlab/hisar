#!/usr/bin/env bash
# Turns desktop/bin/Hisar.app (built by package-desktop-dmg.sh) into a Sparkle update:
# desktop/bin/Hisar-<version>.zip, signed with the EdDSA private key, and desktop/bin/update.json.
# Usage: package-desktop-update.sh <version> <private key file> [release notes file]
set -euo pipefail

version=${1:?usage: package-desktop-update.sh <version> <private key file> [release notes file]}
privateKeyFile=${2:-}
releaseNotesFile=${3:-}
if [ -z "$privateKeyFile" ] || [ ! -s "$privateKeyFile" ]; then
  echo "the Sparkle private key file is missing; in CI, set the protected File variable SPARKLE_PRIVATE_KEY" >&2
  exit 1
fi

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
cd "$scriptDirectory/../desktop"
application=bin/Hisar.app
signUpdate=build/sparkle/bin/sign_update
buildNumber=$("$scriptDirectory/get-build-number.sh" "$version")

test "$(plutil -extract CFBundleShortVersionString raw "$application/Contents/Info.plist")" = "$version"
test "$(plutil -extract CFBundleVersion raw "$application/Contents/Info.plist")" = "$buildNumber"
minimumSystemVersion=$(plutil -extract LSMinimumSystemVersion raw "$application/Contents/Info.plist")
minimumSystemVersion=${minimumSystemVersion%.0}
publicKey=$(plutil -extract SUPublicEDKey raw "$application/Contents/Info.plist")

# Signing with a key that does not belong to the app's SUPublicEDKey would publish an update no install accepts.
derivedPublicKey=$(go run ./cmd/hisar-update-key "$privateKeyFile")
if [ "$derivedPublicKey" != "$publicKey" ]; then
  echo "the private key in $privateKeyFile does not belong to SUPublicEDKey in Info.plist" >&2
  exit 1
fi

archive=bin/Hisar-$version.zip
rm -f "$archive"
ditto -c -k --sequesterRsrc --keepParent "$application" "$archive"
signature=$("$signUpdate" --ed-key-file "$privateKeyFile" -p "$archive")
"$signUpdate" --verify --ed-key-file "$privateKeyFile" "$archive" "$signature" >/dev/null
length=$(stat -f %z "$archive")

releaseNotes="Hisar $version"
if [ -n "$releaseNotesFile" ] && [ -s "$releaseNotesFile" ]; then
  releaseNotes=$(cat "$releaseNotesFile")
fi

VERSION="$version" BUILD_NUMBER="$buildNumber" ARCHIVE="$(basename "$archive")" SIGNATURE="$signature" LENGTH="$length" \
  MINIMUM_SYSTEM_VERSION="$minimumSystemVersion" RELEASE_NOTES="$releaseNotes" PUBLISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  /usr/bin/python3 -I -c '
import json, os
metadata = {
    "version": os.environ["VERSION"],
    "buildNumber": int(os.environ["BUILD_NUMBER"]),
    "archive": os.environ["ARCHIVE"],
    "edSignature": os.environ["SIGNATURE"],
    "length": int(os.environ["LENGTH"]),
    "minimumSystemVersion": os.environ["MINIMUM_SYSTEM_VERSION"],
    "releaseNotes": os.environ["RELEASE_NOTES"].strip(),
    "releaseNotesFormat": "markdown",
    "publishedAt": os.environ["PUBLISHED_AT"],
}
print(json.dumps(metadata, indent=2, ensure_ascii=False))
' > bin/update.json
echo "$PWD/$archive"
echo "$PWD/bin/update.json"
