#!/usr/bin/env bash
# Checks that <directory> holds every file of the GitHub Release of <app> <version> and nothing else, and that the files
# match their SHA256SUMS. The Release workflow runs it before it creates the release, so a release is always complete.
# Usage: check-release-files.sh <app> <version> <directory>
set -euo pipefail

app=${1:?usage: check-release-files.sh <app> <version> <directory>}
version=${2:?usage: check-release-files.sh <app> <version> <directory>}
directory=${3:?usage: check-release-files.sh <app> <version> <directory>}
isCustomBuild=false
[ "$version" = "${version%%-*}" ] || isCustomBuild=true

case "$app" in
  server) allExpected=(release-evidence.json) ;;
  cli)
    allExpected=(release-evidence.json SHA256SUMS)
    for architecture in amd64 arm64; do
      allExpected+=("hisar-$version-linux-$architecture.tar.gz" "hisar-$version-darwin-$architecture.tar.gz"
        "hisar-$version-windows-$architecture.zip")
    done
    ;;
  desktop)
    allExpected=(release-evidence.json "Hisar-$version.dmg")
    $isCustomBuild || allExpected+=("Hisar-$version.zip" update-unpublished.json)
    if ! $isCustomBuild && [ -f "$directory/update.json" ]; then
      allExpected=(release-evidence.json "Hisar-$version.dmg" "Hisar-$version.zip" update.json)
    fi
    for architecture in amd64 arm64; do
      allExpected+=("Hisar-$version-linux-$architecture".{deb,rpm,AppImage,flatpak}
        "SHA256SUMS-linux-$architecture" "SHA256SUMS-flatpak-$architecture")
    done
    ;;
  extension) allExpected=(release-evidence.json "hisar-extension-$version.zip") ;;
  *) echo "unknown app $app (server, cli, desktop or extension)" >&2; exit 1 ;;
esac

cd "$directory"
allPresent=$(find . -type f | sed 's|^\./||' | sort)
allExpectedSorted=$(printf '%s\n' ${allExpected[@]+"${allExpected[@]}"} | sed '/^$/d' | sort)
if [ "$allPresent" != "$allExpectedSorted" ]; then
  echo "the files of $app $version are not the expected ones:" >&2
  diff <(echo "$allExpectedSorted") <(echo "$allPresent") | sed -n 's/^< /missing: /p; s/^> /unexpected: /p' >&2
  exit 1
fi
for sumFile in SHA256SUMS*; do
  [ -f "$sumFile" ] || continue
  if command -v sha256sum > /dev/null; then sha256sum -c --quiet "$sumFile"; else shasum -a 256 -c --quiet "$sumFile"; fi
done
# omp links to the plugin inside Hisar.app and starts its MCP server with bin/run-hisar, so an update without them breaks
# every connected omp.
if [ "$app" = desktop ] && ! $isCustomBuild; then
  ompPlugin=Hisar.app/Contents/Resources/omp-plugin
  for pluginFile in package.json mcp.json bin/run-hisar; do
    if ! unzip -l "Hisar-$version.zip" "$ompPlugin/$pluginFile" > /dev/null; then
      echo "Hisar-$version.zip has no $ompPlugin/$pluginFile" >&2
      exit 1
    fi
  done
  runHisarMode=$(zipinfo "Hisar-$version.zip" "$ompPlugin/bin/run-hisar")
  if [[ $runHisarMode != -rwx* ]]; then
    echo "Hisar-$version.zip has $ompPlugin/bin/run-hisar without its executable bit" >&2
    exit 1
  fi
fi
echo "$app $version: ${#allExpected[@]} files"
