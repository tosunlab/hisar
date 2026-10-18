#!/usr/bin/env bash
# Tells the CI workflows whether a pull request changes the desktop app, the iPhone app, the Android app or the admin
# dashboard, so their jobs run only then. Every run compares the pull request's head with its base, not only the newest
# push: a run cancelled by a later push must not leave the app changes of the earlier push untested.
#
# The apps compile against core/ (the desktop build also regenerates and checks its TypeScript bindings), so a change to
# core/ or go.work counts for each. The desktop app also counts cli/: its module requires it and the Mac app bundles the
# hisar CLI. The phone apps share mobile/hisarkit and mobile/go.mod; an Android-only change keeps off the macOS runner.
#
# Environment: BASE_SHA, HEAD_SHA, IS_FORCED ("true" runs everything) and WORKFLOW_FILE (a change to it runs everything, so
# a workflow edit is tested). Writes isDesktopChanged, isMobileChanged (the iPhone app), isAndroidChanged and isAdminChanged
# to $GITHUB_OUTPUT.
set -euo pipefail

range="$BASE_SHA...$HEAD_SHA"
echo "Comparing $range"

isChanged() { [ -n "$(git diff --name-only "$range" -- "$@" "$WORKFLOW_FILE" scripts/detect-app-changes.sh)" ]; }

isDesktopChanged=false
isMobileChanged=false
isAndroidChanged=false
isAdminChanged=false
if [ "${IS_FORCED:-false}" = true ]; then
  isDesktopChanged=true
  isMobileChanged=true
  isAndroidChanged=true
  isAdminChanged=true
else
  sharedPaths=(core/ go.work go.work.sum)
  phonePaths=(mobile/hisarkit/ mobile/go.mod mobile/go.sum "${sharedPaths[@]}")
  if isChanged desktop/ cli/ scripts/package-desktop-linux.sh "${sharedPaths[@]}"; then isDesktopChanged=true; fi
  if isChanged mobile/ios/ mobile/build-framework.sh scripts/test-ios-ui.sh "${phonePaths[@]}"; then isMobileChanged=true; fi
  if isChanged mobile/android/ mobile/build-android-aar.sh scripts/install-android-sdk.sh scripts/test-android-ui.sh assets/themes/ scripts/themes/ "${phonePaths[@]}"; then isAndroidChanged=true; fi
  if isChanged admin/ server/ "${sharedPaths[@]}"; then isAdminChanged=true; fi
fi
echo "isDesktopChanged=$isDesktopChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isMobileChanged=$isMobileChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isAndroidChanged=$isAndroidChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isAdminChanged=$isAdminChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
