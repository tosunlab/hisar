# CI and releases

How GitHub Actions tests pull requests, and how `server`, `cli`, `desktop` and `extension` releases are cut, built and published. Deploying a released server image: [deploy/README.md](../deploy/README.md).

GitHub Actions runs CI on pull requests, on manual runs (Actions → CI → Run workflow) and on a release that asks for it, never
on pushes to `main`: a merge was already tested as its pull request. A new push to a pull request cancels its running CI.

## Continuous integration

`.github/workflows/ci.yml` (Linux runners): gofmt, vet, all Go tests against Postgres (failing on any skipped test except the
`TestIntegration…` tests and `TestRecordOpenSSHFixture`), the CLI for every release platform, the end-to-end test of the built
binaries, the server Docker image for amd64 and arm64 (built into a throwaway registry and booted, never pushed), the desktop frontend
(typecheck, tests, bundle check) and the desktop app's Linux build (both only when the pull request changes the desktop app or what it builds from, as below), the extension (tests, Chromium end-to-end tests, zip),
the share link page in Chromium against the server, the admin dashboard (typecheck, tests, the `admin/src/themes.css`
freshness check and Chromium end-to-end tests against the server; only when the pull request changes `admin/`, `server/`,
`core/`, `go.work` or the workflow), and the Android app (`HisarKit.aar`, the `ThemePalettes.kt` freshness check, the debug and
release APKs, lint, the JVM tests and the instrumented tests on x86_64 API 36 emulators against a local server, split over four
jobs on Blacksmith's `blacksmith-8vcpu-ubuntu-2404` runners, whose nested virtualization runs the emulator; only when the pull
request changes `mobile/android/`, `mobile/hisarkit/`, `mobile/go.mod`, the Android scripts, the themes,
`core/`, `go.work` or the workflow, and not while the pull request has the label `skip-android`, for changes whose Android build
and tests already passed locally with `make build-android-aar build-android test-android test-android-ui`; add the label before
pushing).

`.github/workflows/macos.yml` builds the Mac app (Touch ID and Sparkle tests, bundle and signature checks) and the iPhone app
(HisarKit, Xcode project check, unit tests, then UI tests against a local server in four jobs that run the one build) on
Blacksmith's `blacksmith-6vcpu-macos-26` runners with Xcode 26.6. The Blacksmith GitHub App must be installed on the repository
for these and the Android jobs to find a runner. A 6-vCPU macOS minute counts as 20 Linux 2-vCPU minutes
([Blacksmith's billing](https://docs.blacksmith.sh/blacksmith-runners/overview)), so each job runs only when the pull request,
compared with its base branch on every push, changes what it builds (`scripts/detect-app-changes.sh`):

- the Mac app when `desktop/`, `cli/` (the app bundles `hisar`), `core/`, `go.work`, its packaging scripts or the workflow
  changes (`scripts/app-paths.sh` lists them);
- the iPhone app when `mobile/ios/`, `mobile/hisarkit/`, `mobile/go.mod`, `mobile/build-framework.sh`, `core/`, `go.work` or the
  workflow changes; an Android-only change does not run it. A draft pull request gets the build and unit tests but not the UI
  tests; marking it ready for review starts a run with them;
- both, whatever changed, while the pull request has the label `run-macos`;
- not the iPhone app while the pull request has the label `skip-ios`, for changes whose iPhone build, unit tests and UI tests
  already passed locally (`make build-ios-framework build-ios test-ios test-ios-ui`). Add the label before pushing: a run
  reads the labels the pull request has when it starts. The jobs then report as skipped;
- or by hand: Actions → macOS and iOS → Run workflow, on a branch (tick "Build and test the iPhone app" to add the iPhone job).

On GitHub's `macos-26` runners a run took roughly 15–25 minutes for the Mac app, and for the iPhone app about 10 minutes of
build, then UI tests of about 1,950 seconds plus about 10 minutes of setup per job. Four UI test jobs bring each to about 8
minutes of tests. A desktop release build costs about the same as the Mac app job.

## Releases

Releases are tags without a `v`: `server-X.Y.Z`, `cli-X.Y.Z`, `desktop-X.Y.Z` and `extension-X.Y.Z`. Each release is one run of
Actions → Release → Run workflow (or `gh workflow run release.yml -f app=server -f type=minor`), on `main`:

- **App**: `server`, `cli`, `desktop` or `extension`.
- **Release type**: `patch`, `minor` or `major` bumps the app's newest `<app>-X.Y.Z` tag (`0.1.14` → `0.1.15`, `0.2.0`,
  `1.0.0`; an app's first release is `0.1.0`), and `custom` takes **Version** as it is: `0.2.1`, or `0.3.0-rc1` for a custom
  build that is built the same way, marked "(custom build)" and can never be published. A custom build may run on any branch.
- **Dry run**: only the plan.
- **Run tests**: also runs `ci.yml` on the commit, plus `macos.yml` for the desktop app, before tagging it. Off by default:
  pull requests ran them already, and a release should be quick when a fix has to go out.

The run then goes on by itself (`scripts/plan-release.sh` plans it):

1. **Plan**: the next version, which must be newer than the previous release and not tagged yet, and the commits since the
   previous release that change what the app is built from (`scripts/app-paths.sh`, without tests and docs). It refuses a
   release without such commits, and warns about a patch release with a breaking change (`feat(core)!: …` or a
   `BREAKING CHANGE:` footer). The run summary shows the plan.
2. **CI**, only with **Run tests**: `ci.yml` for the commit, plus `macos.yml` for the desktop app.
3. **Tag**: the annotated tag on that commit; its message is the title and the changes, which the desktop app's Sparkle
   update shows as release notes. The deploy key in `RELEASE_DEPLOY_KEY` pushes it, past the tag ruleset.
4. **Build**: `release-<app>.yml`, which only the Release workflow calls. The desktop app builds the Mac DMG and Sparkle update
   and the Linux packages and Flatpak (amd64 and arm64); the server pushes `ghcr.io/tosunlab/hisar-server:X.Y.Z`.
5. **Prerelease**: the GitHub prerelease with every file at once (`scripts/check-release-files.sh` checks them first), its
   changes and the app's install text from `.github/release-notes/<app>.md`. Nobody gets it yet.
6. **Publish**: waits in the environment `publish-<app>` until a reviewer approves it (Review deployments on the run). A newer
   release of the app that reaches this step replaces an older one still waiting. It refuses a version older than the app's
   published release, then
   - server: points `ghcr.io/tosunlab/hisar-server:latest` and `:X.Y` at `:X.Y.Z`;
   - desktop: renames the release's `update-unpublished.json` to `update.json`, which offers the version to every Mac app as an
     automatic update;
   - all: rewrites the notes to cover every change since the published release, turns the prerelease into a release and marks
     it latest.

A failed run continues with Re-run failed jobs; a new run refuses the tag it already pushed. Actions → Publish → Run workflow
publishes an existing prerelease by hand, through the same approval. Actions → Release server image → Run workflow on a branch
pushes only `ghcr.io/tosunlab/hisar-server:sha-<commit>`, for testing.

## Repository secrets and environments

Repository secrets: `SPARKLE_PRIVATE_KEY`, the Sparkle EdDSA private key file's content (`gh secret set SPARKLE_PRIVATE_KEY --repo
tosunlab/hisar < <key file>`); `RELEASE_DEPLOY_KEY`, the private half of the deploy key "Release workflow tags" (write
access, a bypass actor of the ruleset "tags: only maintainers create"); and, for desktop releases, the team's Developer ID
Application identity and the App Store Connect API key that notarizes the app: `MACOS_SIGNING_CERTIFICATE` (the identity as a
base64 `.p12`: `base64 -i <identity.p12> | gh secret set MACOS_SIGNING_CERTIFICATE --repo tosunlab/hisar`),
`MACOS_SIGNING_CERTIFICATE_PASSWORD` (its export password), `APPLE_NOTARY_KEY` (the base64 `.p8`), `APPLE_NOTARY_KEY_ID` and
`APPLE_NOTARY_ISSUER_ID`. Images go to the GitHub Container Registry with the
workflow's own `GITHUB_TOKEN`. The environments `publish-server`, `publish-cli`, `publish-desktop` and `publish-extension` have
the required reviewers and accept only `main`.
