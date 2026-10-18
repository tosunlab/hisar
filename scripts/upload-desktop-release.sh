#!/usr/bin/env bash
# Uploads files to the GitHub prerelease desktop-<version>, creating it when this job is the first to finish.
# Usage: upload-desktop-release.sh <tag> <file>…     (needs gh with GH_TOKEN and GH_REPO; the macOS job sets the title)
set -euo pipefail

tag=${1:?usage: upload-desktop-release.sh <tag> <file>…}
shift
notesPath="$(cd "$(dirname "$0")/.." && pwd)/.github/release-desktop-notes.md"
if [ "$(gh release view "$tag" --json isPrerelease --jq .isPrerelease 2>/dev/null)" = false ]; then
  echo "the release $tag is already published; refusing to replace its files"
  exit 1
fi
if ! gh release view "$tag" >/dev/null 2>&1; then
  # Another job may create it at the same moment; only a release that is still missing is an error.
  gh release create "$tag" --verify-tag --prerelease --latest=false --title "Hisar ${tag#desktop-}" \
    --notes "$(sed "s/<version>/${tag#desktop-}/g" "$notesPath")" ||
    gh release view "$tag" >/dev/null
fi
gh release upload "$tag" --clobber "$@"
