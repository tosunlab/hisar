#!/usr/bin/env bash
# Tells the CI workflows whether a pull request changes the desktop or iPhone app, so their jobs run only then. Every run
# compares the pull request's head with its base, not only the newest push: a run cancelled by a later push must not leave
# the app changes of the earlier push untested.
#
# Both apps compile against the shared backend in core/ (the desktop build also regenerates and checks its TypeScript
# bindings), so a change to core/ or go.work counts for both. The desktop app also counts cli/: its module requires it and
# the Mac app bundles the hisar CLI.
#
# Environment: BASE_SHA, HEAD_SHA, IS_FORCED ("true" runs everything) and WORKFLOW_FILE (a change to it runs everything, so
# a workflow edit is tested). Writes isDesktopChanged and isMobileChanged to $GITHUB_OUTPUT.
set -euo pipefail

range="$BASE_SHA...$HEAD_SHA"
echo "Comparing $range"

isChanged() { [ -n "$(git diff --name-only "$range" -- "$@" "$WORKFLOW_FILE" scripts/detect-app-changes.sh)" ]; }

isDesktopChanged=false
isMobileChanged=false
if [ "${IS_FORCED:-false}" = true ]; then
  isDesktopChanged=true
  isMobileChanged=true
else
  sharedPaths=(core/ go.work go.work.sum)
  if isChanged desktop/ cli/ "${sharedPaths[@]}"; then isDesktopChanged=true; fi
  if isChanged mobile/ "${sharedPaths[@]}"; then isMobileChanged=true; fi
fi
echo "isDesktopChanged=$isDesktopChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isMobileChanged=$isMobileChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
