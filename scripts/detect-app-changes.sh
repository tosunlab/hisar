#!/usr/bin/env bash
# Tells the CI workflows whether a pull request changes the desktop app, the iPhone app or the admin dashboard, so their
# jobs run only then. Every run compares the pull request's head with its base, not only the newest push: a run cancelled
# by a later push must not leave the app changes of the earlier push untested. For a release, the base is the previous
# release's tag.
#
# scripts/app-paths.sh says what each app is built from: core/ and go.work count for each, and the desktop app also counts
# cli/.
#
# Environment: BASE_SHA, HEAD_SHA, IS_FORCED ("true" runs everything) and WORKFLOW_FILE (a change to it runs everything, so
# a workflow edit is tested). Writes isDesktopChanged, isMobileChanged and isAdminChanged to $GITHUB_OUTPUT.
set -euo pipefail

range="$BASE_SHA...$HEAD_SHA"
echo "Comparing $range"

isChanged() { [ -n "$(git diff --name-only "$range" -- "$@" "$WORKFLOW_FILE" scripts/detect-app-changes.sh scripts/app-paths.sh)" ]; }

isDesktopChanged=false
isMobileChanged=false
isAdminChanged=false
if [ "${IS_FORCED:-false}" = true ]; then
  isDesktopChanged=true
  isMobileChanged=true
  isAdminChanged=true
else
  # shellcheck source=scripts/app-paths.sh
  source "$(dirname "$0")/app-paths.sh"
  if isChanged "${desktopPaths[@]}"; then isDesktopChanged=true; fi
  if isChanged "${mobilePaths[@]}"; then isMobileChanged=true; fi
  if isChanged "${serverPaths[@]}"; then isAdminChanged=true; fi
fi
echo "isDesktopChanged=$isDesktopChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isMobileChanged=$isMobileChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isAdminChanged=$isAdminChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
