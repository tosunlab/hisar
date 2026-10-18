#!/usr/bin/env bash
# Plans one release for the Release workflow: the next <app>-<version> tag, its title and its changes.
#
# The previous release is the newest plain <app>-MAJOR.MINOR.PATCH tag (custom builds with a -label don't count). patch,
# minor and major bump it; an app's first release is 0.1.0 (1.0.0 for major). custom takes CUSTOM_VERSION as it is:
# MAJOR.MINOR.PATCH (each part at most 999, as scripts/get-build-number.sh requires) with an optional -label, which makes
# it a custom build that can't be published, and newer than the previous release.
#
# It refuses a release whose app has no change since the previous release (scripts/app-paths.sh, without tests and docs),
# a tag that exists, and a plain release from any branch but main. It warns when a patch release has a breaking change: a
# commit subject like "feat(core)!: …" or a "BREAKING CHANGE:" footer.
#
# Environment: APP (server, cli, desktop or extension), RELEASE_TYPE (patch, minor, major or custom), CUSTOM_VERSION (only
# for custom) and GITHUB_REF (the branch released from; defaults to the checked-out branch). Writes tag, version, title,
# previousTag, isCustomBuild, commitCount and changes to $GITHUB_OUTPUT and a summary to $GITHUB_STEP_SUMMARY.
set -euo pipefail

scriptDirectory=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=scripts/app-paths.sh
source "$scriptDirectory/app-paths.sh"

fail() {
  if [ "${GITHUB_ACTIONS:-}" = true ]; then echo "::error::$*"; else echo "$*" >&2; fi
  exit 1
}
warn() {
  if [ "${GITHUB_ACTIONS:-}" = true ]; then echo "::warning::$*"; else echo "warning: $*" >&2; fi
}
# isNewer A B: whether version A is newer than version B (both MAJOR.MINOR.PATCH).
isNewer() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n 1)" = "$1" ]; }

app=${APP:?set APP to server, cli, desktop or extension}
releaseType=${RELEASE_TYPE:?set RELEASE_TYPE to patch, minor, major or custom}
customVersion=${CUSTOM_VERSION:-}
releaseRef=${GITHUB_REF:-$(git symbolic-ref -q HEAD || echo detached)}

case "$app" in
  server) title="Hisar Server" ;;
  cli) title="Hisar command-line tool" ;;
  desktop) title="Hisar" ;;
  extension) title="Hisar extension" ;;
  *) fail "unknown app $app (server, cli, desktop or extension)" ;;
esac
setReleasePathspec "$app"

previousTag=$(git tag --list "$app-*" | grep -E "^$app-(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$" | sort -V | tail -n 1 || true)
previousVersion=${previousTag#"$app"-}
IFS=. read -r major minor patch <<< "${previousVersion:-0.0.0}"

case "$releaseType" in
  patch | minor | major)
    [ -z "$customVersion" ] || fail "a version is only for the release type custom; $releaseType chooses it"
    if [ -z "$previousTag" ]; then
      case "$releaseType" in major) version=1.0.0 ;; *) version=0.1.0 ;; esac
    else
      case "$releaseType" in
        patch) version="$major.$minor.$((patch + 1))" ;;
        minor) version="$major.$((minor + 1)).0" ;;
        major) version="$((major + 1)).0.0" ;;
      esac
    fi
    ;;
  custom)
    [ -n "$customVersion" ] || fail "the release type custom needs a version, e.g. 0.2.1 or 0.3.0-rc1"
    version=$customVersion
    ;;
  *) fail "unknown release type $releaseType (patch, minor, major or custom)" ;;
esac

if ! versionError=$(bash "$scriptDirectory/get-build-number.sh" "$version" 2>&1 > /dev/null); then fail "$versionError"; fi
releaseVersion=${version%%-*}
isCustomBuild=false
[ "$version" = "$releaseVersion" ] || isCustomBuild=true
if [ -n "$previousTag" ] && ! isNewer "$releaseVersion" "$previousVersion"; then
  fail "$version is not newer than the previous release $previousTag"
fi
tag="$app-$version"
if git rev-parse -q --verify "refs/tags/$tag" > /dev/null; then fail "the tag $tag exists"; fi
if ! $isCustomBuild && [ "$releaseRef" != refs/heads/main ]; then
  fail "a release is made from main, not $releaseRef; a custom build (a version with a -label) can come from any branch"
fi
if $isCustomBuild; then title="$title $version (custom build)"; else title="$title $version"; fi

if [ -n "$previousTag" ]; then range=("$previousTag..HEAD"); else range=(HEAD); fi
commitCount=$(git rev-list --count --no-merges "${range[@]}" -- "${releasePathspec[@]}")
[ "$commitCount" -gt 0 ] || fail "nothing changed in $app since $previousTag"
breakingSubjects=$( {
  git log --no-merges --format=%s "${range[@]}" -- "${releasePathspec[@]}" | grep -E '^[a-z]+(\([^)]*\))?!:' || true
  git log --no-merges --format=%s --extended-regexp --grep='^BREAKING[ -]CHANGE:' "${range[@]}" -- "${releasePathspec[@]}"
} | sort -u)
if [ "$releaseType" = patch ] && [ -n "$breakingSubjects" ]; then
  warn "a patch release of $app with breaking changes; minor is the usual bump for them: $(paste -sd ';' - <<< "$breakingSubjects")"
fi
changes=$(bash "$scriptDirectory/release-notes.sh" "$app" "$previousTag" HEAD)

delimiter="EOF_$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
{
  echo "tag=$tag"
  echo "version=$version"
  echo "title=$title"
  echo "previousTag=$previousTag"
  echo "isCustomBuild=$isCustomBuild"
  echo "commitCount=$commitCount"
  echo "changes<<$delimiter"
  echo "$changes"
  echo "$delimiter"
} >> "${GITHUB_OUTPUT:-/dev/null}"

{
  echo "### $title"
  echo
  echo "| | |"
  echo "|---|---|"
  echo "| Tag | \`$tag\` |"
  echo "| Previous release | $(if [ -n "$previousTag" ]; then echo "\`$previousTag\`"; else echo none; fi) |"
  echo "| Release type | $releaseType |"
  echo "| Commit | \`$(git rev-parse --short HEAD)\` from \`${releaseRef#refs/heads/}\` |"
  echo "| Commits | $commitCount |"
  echo "| Custom build | $isCustomBuild |"
  if [ -n "$breakingSubjects" ]; then
    echo
    echo "Breaking changes:"
    echo
    while IFS= read -r subject; do echo "- $subject"; done <<< "$breakingSubjects"
  fi
  echo
  echo "$changes"
} | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"
