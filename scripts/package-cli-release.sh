#!/usr/bin/env bash
# Builds the standalone hisar CLI for every release platform into <output directory>:
# hisar-<version>-<os>-<arch>.tar.gz (hisar-<version>-windows-<arch>.zip with hisar.exe for Windows) and SHA256SUMS.
# Usage: package-cli-release.sh <version> <output directory>
set -euo pipefail

version=${1:?usage: package-cli-release.sh <version> <output directory>}
outputDirectory=${2:?usage: package-cli-release.sh <version> <output directory>}
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "the version must look like 1.2.3 (got $version)" >&2
  exit 1
fi
allTarget=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64)

root="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$outputDirectory"
outputDirectory="$(cd "$outputDirectory" && pwd)"
rm -f "$outputDirectory"/hisar-"$version"-* "$outputDirectory/SHA256SUMS"
workDirectory="$(mktemp -d)"
trap 'rm -rf "$workDirectory"' EXIT

for target in "${allTarget[@]}"; do
  os=${target%/*}
  architecture=${target#*/}
  name="hisar-$version-$os-$architecture"
  binary=hisar
  if [ "$os" = windows ]; then binary=hisar.exe; fi
  mkdir -p "$workDirectory/$name"
  CGO_ENABLED=0 GOOS=$os GOARCH=$architecture go -C "$root/cli" build -trimpath -ldflags="-s -w -X main.version=$version" -o "$workDirectory/$name/$binary" ./cmd/hisar
  if [ "$os" = windows ]; then
    (cd "$workDirectory/$name" && zip -q -X "$outputDirectory/$name.zip" "$binary")
  else
    tar -C "$workDirectory/$name" -czf "$outputDirectory/$name.tar.gz" "$binary"
  fi
  echo "$name"
done

cd "$outputDirectory"
if command -v sha256sum >/dev/null; then
  sha256sum hisar-"$version"-* > SHA256SUMS
else
  shasum -a 256 hisar-"$version"-* > SHA256SUMS
fi
cat SHA256SUMS
