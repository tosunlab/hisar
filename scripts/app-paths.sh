# shellcheck shell=bash disable=SC2034
# What each app is built from, as git pathspecs. Sourced by detect-app-changes.sh (which CI jobs a pull request needs) and
# plan-release.sh and release-notes.sh (which commits a release has).
#
# The Go apps compile against core/ and go.work. The desktop app also counts cli/: its module requires it and the app
# bundles the hisar CLI. The server image embeds the admin dashboard.

sharedGoPaths=(core/ go.work go.work.sum)
serverPaths=(server/ admin/ deploy/Dockerfile "${sharedGoPaths[@]}")
cliPaths=(cli/ scripts/package-cli-release.sh "${sharedGoPaths[@]}")
desktopPaths=(desktop/ cli/ scripts/download-sparkle.sh scripts/get-build-number.sh scripts/package-desktop-dmg.sh
  scripts/package-desktop-flatpak.sh scripts/package-desktop-linux.sh scripts/package-desktop-update.sh
  scripts/sign-desktop-app.sh scripts/test-desktop-linux-package.sh "${sharedGoPaths[@]}")
mobilePaths=(mobile/ "${sharedGoPaths[@]}")
extensionPaths=(extension/)

# A commit that only changes tests or documentation is not a reason to release.
releaseExcludedPaths=(':(exclude,glob)**/*_test.go' ':(exclude,glob)**/testdata/**' ':(exclude,glob)**/*.spec.ts'
  ':(exclude,glob)**/*.test.ts' ':(exclude,glob)**/end-to-end/**' ':(exclude,glob)**/*.md')

# Sets releasePathspec to the pathspec of the commits a release of app ($1: server, cli, desktop or extension) counts.
setReleasePathspec() {
  case "$1" in
    server) releasePathspec=("${serverPaths[@]}") ;;
    cli) releasePathspec=("${cliPaths[@]}") ;;
    desktop) releasePathspec=("${desktopPaths[@]}") ;;
    extension) releasePathspec=("${extensionPaths[@]}") ;;
    *) echo "unknown app $1 (server, cli, desktop or extension)" >&2; return 1 ;;
  esac
  releasePathspec+=("${releaseExcludedPaths[@]}")
}
