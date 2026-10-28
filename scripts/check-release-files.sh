#!/usr/bin/env bash
# Checks that <directory> holds every file of the GitHub Release of <app> <version> and nothing else, and that each
# SHA256SUMS file lists exactly the files it covers, with their bytes. The Release workflow runs it before it creates the
# release, so a release is always complete.
# Usage: check-release-files.sh <app> <version> <directory>
set -euo pipefail

app=${1:?usage: check-release-files.sh <app> <version> <directory>}
version=${2:?usage: check-release-files.sh <app> <version> <directory>}
directory=${3:?usage: check-release-files.sh <app> <version> <directory>}
isCustomBuild=false
[ "$version" = "${version%%-*}" ] || isCustomBuild=true

# requireChecksums <sum file> <file>...: the sum file lists exactly these files, and their bytes match.
requireChecksums() {
  local sumFile=$1 allListed allCovered
  shift
  allListed=$(while read -r _ fileName; do echo "${fileName#\*}"; done < "$sumFile" | sort)
  allCovered=$(printf '%s\n' "$@" | sort)
  if [ "$allListed" != "$allCovered" ]; then
    echo "$sumFile does not list exactly the files it covers:" >&2
    diff <(echo "$allCovered") <(echo "$allListed") | sed -n 's/^< /not listed: /p; s/^> /listed: /p' >&2
    exit 1
  fi
  if command -v sha256sum > /dev/null; then sha256sum -c --quiet "$sumFile"; else shasum -a 256 -c --quiet "$sumFile"; fi
}

case "$app" in
  server) allExpected=(release-evidence.json) ;;
  cli)
    allArchive=()
    for architecture in amd64 arm64; do
      allArchive+=("hisar-$version-linux-$architecture.tar.gz" "hisar-$version-darwin-$architecture.tar.gz"
        "hisar-$version-windows-$architecture.zip")
    done
    allExpected=(release-evidence.json SHA256SUMS "${allArchive[@]}")
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
      # The signatures the Linux app checks before it installs an update; a custom build is never offered as one.
      $isCustomBuild || allExpected+=("Hisar-$version-linux-$architecture".{deb,rpm,AppImage,flatpak}.sig)
    done
    ;;
  # A custom build's Firefox package is not signed: an AMO version is used up by its first signing.
  extension)
    allExpected=(release-evidence.json "hisar-extension-$version.zip")
    if $isCustomBuild; then allExpected+=("hisar-extension-firefox-$version-unsigned.zip"); else allExpected+=("hisar-extension-firefox-$version.xpi"); fi
    ;;
  docs) allExpected=(release-evidence.json) ;;
  *) echo "unknown app $app (server, cli, desktop, extension or docs)" >&2; exit 1 ;;
esac

cd "$directory"
allPresent=$(find . -type f | sed 's|^\./||' | sort)
allExpectedSorted=$(printf '%s\n' ${allExpected[@]+"${allExpected[@]}"} | sed '/^$/d' | sort)
if [ "$allPresent" != "$allExpectedSorted" ]; then
  echo "the files of $app $version are not the expected ones:" >&2
  diff <(echo "$allExpectedSorted") <(echo "$allPresent") | sed -n 's/^< /missing: /p; s/^> /unexpected: /p' >&2
  exit 1
fi
case "$app" in
  cli) requireChecksums SHA256SUMS "${allArchive[@]}" ;;
  desktop)
    for architecture in amd64 arm64; do
      requireChecksums "SHA256SUMS-linux-$architecture" "Hisar-$version-linux-$architecture".{deb,rpm,AppImage}
      requireChecksums "SHA256SUMS-flatpak-$architecture" "Hisar-$version-linux-$architecture.flatpak"
    done
    ;;
esac
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
