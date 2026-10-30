#!/usr/bin/env bash
# Tells the CI workflows which apps a pull request changes, so their jobs run only then. Every run compares the pull
# request's head with its base, not only the newest push: a run cancelled by a later push must not leave the app changes
# of the earlier push untested. Releases force every job instead of relying on a diff against a previous tag.
#
# scripts/app-paths.sh says what each app is built from: core/ and go.work count for each, and the desktop app also counts
# cli/ and omp/. The phone apps share mobile/hisarkit and mobile/go.mod; an Android-only change keeps off the macOS runner.
# The phone UI tests and the admin browser tests also run when what they run against changes (testBackendPaths:
# hisar-server, the hisar CLI and the Makefile), the desktop tests when the sign-in fixture site changes
# (desktopTestPaths), and every job runs when the Makefile changes. The Go tests read scripts
# and fixtures across the repository, so they run unless the change is only to the non-Go apps (nonGoPaths).
#
# Environment: BASE_SHA, HEAD_SHA, IS_FORCED ("true" runs everything) and WORKFLOW_FILE (a change to it, or to the
# ci-apt-install.sh its container jobs use, runs everything, so a workflow edit is tested). Writes isDesktopChanged,
# isMobileChanged (the iPhone app), isAndroidChanged, isAdminChanged, isGoChanged, isServerChanged (the server image),
# isExtensionChanged, isDocsChanged, isRaycastChanged and isOmpChanged to $GITHUB_OUTPUT.
set -euo pipefail

range="$BASE_SHA...$HEAD_SHA"
echo "Comparing $range"

ciPaths=("$WORKFLOW_FILE" scripts/ci-apt-install.sh scripts/detect-app-changes.sh scripts/app-paths.sh)

isChanged() {
  [ -n "$(git diff --name-only "$range" -- "$@" "${ciPaths[@]}")" ]
}

areas=(Desktop Mobile Android Admin Go Server Extension Docs Raycast Omp)
setChanged() { printf -v "is${1}Changed" '%s' "$2"; }
for area in "${areas[@]}"; do setChanged "$area" false; done
if [ "${IS_FORCED:-false}" = true ]; then
  for area in "${areas[@]}"; do setChanged "$area" true; done
else
  # shellcheck source=scripts/app-paths.sh
  source "$(dirname "$0")/app-paths.sh"
  if isChanged "${desktopPaths[@]}" "${desktopTestPaths[@]}" Makefile; then setChanged Desktop true; fi
  if isChanged "${mobilePaths[@]}" "${testBackendPaths[@]}"; then setChanged Mobile true; fi
  if isChanged "${androidPaths[@]}" "${testBackendPaths[@]}"; then setChanged Android true; fi
  if isChanged "${serverPaths[@]}" "${testBackendPaths[@]}"; then setChanged Admin true; fi
  if isChanged "${serverPaths[@]}" Makefile; then setChanged Server true; fi
  if isChanged "${extensionPaths[@]}" Makefile; then setChanged Extension true; fi
  if isChanged "${docsPaths[@]}" Makefile; then setChanged Docs true; fi
  if isChanged "${raycastPaths[@]}" Makefile; then setChanged Raycast true; fi
  if isChanged "${ompPaths[@]}" Makefile; then setChanged Omp true; fi
  nonGoPathspec=()
  for path in "${nonGoPaths[@]}"; do nonGoPathspec+=(":(exclude,glob)$path"); done
  if [ -n "$(git diff --name-only "$range" -- . "${nonGoPathspec[@]}")" ]; then setChanged Go true; fi
fi
for area in "${areas[@]}"; do
  flag="is${area}Changed"
  echo "$flag=${!flag}" | tee -a "${GITHUB_OUTPUT:-/dev/null}"
done
