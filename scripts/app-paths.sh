# shellcheck shell=bash disable=SC2034
# What each app is built from, as git pathspecs. Sourced by detect-app-changes.sh (which CI jobs a pull request needs) and
# plan-release.sh and release-notes.sh (which commits a release has).
#
# The Go apps compile against core/ and go.work. The desktop app also counts cli/ and omp/: its module requires cli/, and
# the app bundles the hisar CLI and the omp plugin. The server image embeds the admin dashboard. The iPhone and Android
# apps share mobile/hisarkit and mobile/go.mod; each also counts its own folder, so an Android-only change does not run
# the iPhone job. The docs site is docs/ without the repository's design docs, which are not pages of it.

sharedGoPaths=(core/ go.work go.work.sum)
serverPaths=(server/ admin/ deploy/Dockerfile "${sharedGoPaths[@]}")
cliPaths=(cli/ scripts/package-cli-release.sh "${sharedGoPaths[@]}")
desktopPaths=(desktop/ cli/ omp/ scripts/download-sparkle.sh scripts/get-build-number.sh scripts/package-desktop-dmg.sh
  scripts/package-desktop-flatpak.sh scripts/package-desktop-linux.sh scripts/package-desktop-update.sh
  scripts/sign-desktop-app.sh scripts/test-desktop-linux-package.sh "${sharedGoPaths[@]}")
phonePaths=(mobile/hisarkit/ mobile/go.mod mobile/go.sum scripts/ui-test-partner.sh "${sharedGoPaths[@]}")
mobilePaths=(mobile/ios/ mobile/build-framework.sh scripts/test-ios-ui.sh "${phonePaths[@]}")
# The Android CI job runs its build and tests through Makefile recipes, so a Makefile change runs it too.
androidPaths=(mobile/android/ mobile/build-android-aar.sh mobile/cmd/ scripts/install-android-sdk.sh scripts/test-android-ui.sh
  assets/themes/ scripts/themes/ Makefile "${phonePaths[@]}")
extensionPaths=(extension/)
docsPaths=(docs/ ':(exclude)docs/DESIGN.md' ':(exclude)docs/COPY.md' ':(exclude)docs/RELEASING.md')

# A commit that only changes tests or documentation is not a reason to release; the docs site is made of Markdown, so
# only the tests are excluded from it.
releaseExcludedTestPaths=(':(exclude,glob)**/*_test.go' ':(exclude,glob)**/testdata/**' ':(exclude,glob)**/*.spec.ts'
  ':(exclude,glob)**/*.test.ts' ':(exclude,glob)**/end-to-end/**')
releaseExcludedMarkdownPaths=(':(exclude,glob)**/*.md')

# Sets releasePathspec to the pathspec of the commits a release of app ($1: server, cli, desktop, extension or docs) counts.
setReleasePathspec() {
  case "$1" in
    server) releasePathspec=("${serverPaths[@]}") ;;
    cli) releasePathspec=("${cliPaths[@]}") ;;
    desktop) releasePathspec=("${desktopPaths[@]}") ;;
    extension) releasePathspec=("${extensionPaths[@]}") ;;
    docs) releasePathspec=("${docsPaths[@]}") ;;
    *) echo "unknown app $1 (server, cli, desktop, extension or docs)" >&2; return 1 ;;
  esac
  releasePathspec+=("${releaseExcludedTestPaths[@]}")
  [ "$1" = docs ] || releasePathspec+=("${releaseExcludedMarkdownPaths[@]}")
}
