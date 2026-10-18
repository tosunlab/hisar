#!/usr/bin/env bash
# Builds the Hisar Flatpak (desktop/build/linux/flatpak/com.tryhisar.hisar.yml) for the machine's own architecture into
# desktop/bin/Hisar-<version>-linux-<arch>.flatpak, a single-file bundle: flatpak install --user Hisar-….flatpak.
# Usage: package-desktop-flatpak.sh <version>
# Needs flatpak, flatpak-builder (with permission to build sandboxes, as in Flathub's builder container), Node and network
# access: Flathub provides the GNOME 51 runtime and SDK, and go.dev the Go toolchain the manifest pins.
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
repositoryDirectory="$(cd "$scriptDirectory/.." && pwd)"
cd "$repositoryDirectory/desktop"
version=${1:?usage: package-desktop-flatpak.sh <version>}
case $(uname -m) in
  x86_64) architecture=amd64 ;;
  aarch64) architecture=arm64 ;;
  *) echo "no Flatpak for $(uname -m)"; exit 1 ;;
esac

(cd frontend && npm ci --no-audit --no-fund && npm run build)

buildDirectory=build/linux/flatpak/build
rm -rf "$buildDirectory"
mkdir -p "$buildDirectory" bin
manifest=$buildDirectory/com.tryhisar.hisar.yml
sed -e "s|@VERSION@|$version|g" -e "s|@SOURCE@|$repositoryDirectory|g" build/linux/flatpak/com.tryhisar.hisar.yml > "$manifest"

flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
flatpak-builder --user --install-deps-from=flathub --disable-rofiles-fuse --force-clean \
  --repo="$buildDirectory/repo" "$buildDirectory/app" "$manifest"
output=bin/Hisar-$version-linux-$architecture.flatpak
flatpak build-bundle --runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo "$buildDirectory/repo" "$output" com.tryhisar.hisar
echo "$PWD/$output"
