#!/usr/bin/env bash
# Tells the CI workflows whether a pull request event changes the desktop or iPhone app, so their jobs run only then.
# A push to the pull request is compared with the commit before it; a newly opened or reopened pull request, or a push
# whose earlier commit is gone (a force push), is compared with the base branch.
#
# Environment: EVENT_ACTION, BASE_SHA, BEFORE_SHA, HEAD_SHA, IS_FORCED ("true" runs everything) and WORKFLOW_FILE (a change
# to it runs everything, so a workflow edit is tested). Writes isDesktopChanged and isMobileChanged to $GITHUB_OUTPUT.
set -euo pipefail

range="$BASE_SHA...$HEAD_SHA"
if [ "${EVENT_ACTION:-}" = synchronize ] && [ -n "${BEFORE_SHA:-}" ] && git cat-file -e "$BEFORE_SHA^{commit}" 2>/dev/null; then
  range="$BEFORE_SHA..$HEAD_SHA"
fi
echo "Comparing $range"

isChanged() { [ -n "$(git diff --name-only "$range" -- "$@" "$WORKFLOW_FILE" scripts/detect-app-changes.sh)" ]; }

isDesktopChanged=false
isMobileChanged=false
if [ "${IS_FORCED:-false}" = true ]; then
  isDesktopChanged=true
  isMobileChanged=true
else
  if isChanged desktop/; then isDesktopChanged=true; fi
  if isChanged mobile/; then isMobileChanged=true; fi
fi
echo "isDesktopChanged=$isDesktopChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
echo "isMobileChanged=$isMobileChanged" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
