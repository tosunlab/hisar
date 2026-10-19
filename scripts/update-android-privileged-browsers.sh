#!/bin/bash
# Rebuilds the Android app's list of browsers allowed to pass a web origin to Hisar's passkey provider: Google's list
# merged with the community lists that Bitwarden and KeePassDX ship, one entry per package with every listed signature.
set -euo pipefail
cd "$(dirname "$0")/.."

output=mobile/android/app/src/main/assets/privileged_browsers.json
workDirectory=$(mktemp -d)
trap 'rm -rf "$workDirectory"' EXIT

curl -sSfL -o "$workDirectory/google.json" https://www.gstatic.com/gpm-passkeys-privileged-apps/apps.json
curl -sSfL -o "$workDirectory/bitwarden.json" https://raw.githubusercontent.com/bitwarden/android/main/app/src/main/assets/fido2_privileged_community.json
curl -sSfL https://raw.githubusercontent.com/Kunzisoft/KeePassDX/master/app/src/free/assets/passkeys_privileged_apps_community.json |
	# KeePassDX's file has trailing commas, which jq rejects.
	perl -0pe 's/,(\s*[\]}])/$1/g' > "$workDirectory/keepassdx.json"

jq -s '{apps: ([.[].apps[] | select(.type == "android")]
	| group_by(.info.package_name)
	| map({type: "android", info: {package_name: .[0].info.package_name, signatures: ([.[].info.signatures[]] | unique_by(.cert_fingerprint_sha256))}}))}' \
	"$workDirectory/google.json" "$workDirectory/bitwarden.json" "$workDirectory/keepassdx.json" > "$output"
echo "wrote $output with $(jq '.apps | length' "$output") browsers"
