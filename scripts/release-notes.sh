#!/usr/bin/env bash
# Prints the changes of a release as markdown: the subjects of the commits from <since-tag> to <to-ref> that change what
# <app> is built from (scripts/app-paths.sh), newest first. Without a <since-tag> (the app's first release) it lists every
# such commit up to <to-ref>, at most 100. With a <version>, it prints the body of the GitHub Release instead: a custom
# build's notice, the changes, and the app's install text from .github/release-notes/<app>.md.
# Usage: release-notes.sh <app> <since-tag or ""> <to-ref> [<version>]
set -euo pipefail

app=${1:?usage: release-notes.sh <app> <since-tag or ""> <to-ref>}
sinceTag=${2-}
toRef=${3:?usage: release-notes.sh <app> <since-tag or ""> <to-ref> [<version>]}
version=${4-}
maximumCount=100

# shellcheck source=scripts/app-paths.sh
source "$(dirname "$0")/app-paths.sh"
setReleasePathspec "$app"

if [ -n "$sinceTag" ]; then
  range=("$sinceTag..$toRef")
  heading="Changes since $sinceTag:"
else
  range=("$toRef")
  heading="Changes:"
fi

printChanges() {
  local count
  count=$(git rev-list --count --no-merges "${range[@]}" -- "${releasePathspec[@]}")
  if [ "$count" -eq 0 ]; then
    echo "No changes since $sinceTag."
    return
  fi
  echo "$heading"
  echo
  git log --no-merges --format='- %s' --max-count="$maximumCount" "${range[@]}" -- "${releasePathspec[@]}"
  if [ "$count" -gt "$maximumCount" ]; then
    echo "- … and $((count - maximumCount)) earlier changes"
  fi
}

if [ -z "$version" ]; then
  printChanges
  exit 0
fi
if [ "$version" != "${version%%-*}" ]; then
  echo "Custom build $version for testing. It can't be published, and the next release replaces it."
  echo
fi
printChanges
echo
sed "s/<version>/$version/g" "$(dirname "$0")/../.github/release-notes/$app.md"
