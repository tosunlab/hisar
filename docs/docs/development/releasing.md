---
sidebar_position: 1
sidebar_label: CI and releases
---

# CI and releases

How GitHub Actions tests pull requests, and how `server`, `cli`, `desktop`, `extension` and `docs` releases are cut, built and published. Deploying a released server image: [Self-hosting](../self-hosting.md).

GitHub Actions runs CI on pull requests, on manual runs (Actions → CI → Run workflow) and on a release that asks for it, never
on pushes to `main`: a merge was already tested as its pull request. A new push to a pull request cancels its running CI.

## Continuous integration

`.github/workflows/ci.yml` (Linux runners): gofmt, vet and all Go tests against Postgres (failing on any skipped test except the
`TestIntegration…` tests and `TestRecordOpenSSHFixture`) in one job, and the CLI for every release platform and the end-to-end test
of the built binaries in another (both unless the pull request changes only `extension/`, `docs/`, `raycast/`, `mobile/ios/`,
`mobile/android/` or Markdown; the tests create accounts with the minimum Argon2 parameters, through the test-only build tag
`hisar_fast_test_kdf`, which `scripts/check-fast-test-kdf.sh` keeps out of every release build),
the server Docker image for amd64 and arm64 (built into a throwaway registry and booted, never pushed; when the server or what it
builds from changes), the desktop app's Linux build (when the desktop app or what it builds from changes, as below), the extension
(tests, Chromium end-to-end tests and zip, when `extension/` changes) with the share link page in Chromium against the server (when
the extension or the server changes), the admin dashboard (typecheck, tests, the `admin/src/themes.css`
freshness check and Chromium end-to-end tests against the server; only when the pull request changes `admin/`, `server/`,
`core/`, `go.work` or the workflow), and the Android app (`HisarKit.aar`, the `ThemePalettes.kt` freshness check, the debug and
release APKs, lint and the JVM tests in one job, and the instrumented tests on x86_64 API 36 emulators against a local server in
three more, split by the test classes' measured run times in `scripts/ui-test-parts.sh`; only when the pull request changes
`mobile/android/`, `mobile/hisarkit/`, `mobile/go.mod`, the Android scripts, the themes,
`core/`, `go.work` or the workflow, and not while the pull request has the label `skip-android`, for changes whose Android build
and tests already passed locally with `make build-android-aar build-android test-android test-android-ui`; add the label before
pushing). One job runs the small checks, each when its app changes: the desktop frontend (typecheck, tests, bundle check), the docs
site (host typecheck, then its image built and served once), the extension's Firefox end-to-end tests on Firefox 140 and 153, the
omp plugin and the Raycast extension. A change to the Makefile or to the workflow runs every job.

`.github/workflows/caches.yml` runs on every push to `main`: it compiles every Go test and builds the Android app and both
`HisarKit` builds, so main's caches hold what pull requests restore; a pull request saves a cache only when main has none for its
key. It also deletes a closed pull request's caches, which otherwise count against the repository's 10 GB and evict main's.
Gradle's configuration cache is saved only with the repository secret `GRADLE_ENCRYPTION_KEY` (`openssl rand -base64 16`).

The extension job installs dependencies once. Its Chromium tests build the Chrome and Firefox outputs, then the job lints
the Firefox output and packages both without rebuilding. The Chrome ZIP contains the output used by those browser tests.
The standalone `make test-extension`, `make test-extension-end-to-end` and `make build-extension` commands still install
their own dependencies.

The docs job typechecks on the host and builds the site only inside `docs/Dockerfile`. It boots that image and checks the
home page and CLI page. Local `make test-docs` still typechecks and builds; `make build-docs` and the docs release workflow
also keep their builds.

`.github/workflows/macos.yml` builds the Mac app (Touch ID and Sparkle tests, bundle and signature checks) and the iPhone app
(HisarKit, Xcode project check, unit tests, then UI tests against a local server in three parts, split by the test classes'
measured run times in `scripts/ui-test-parts.sh`: the build job runs part 1 on the simulator it booted while building, and parts
2 and 3 start alongside it, boot their simulators while the app builds and then run its build) on
GitHub's `macos-26` runners with Xcode 26.6. macOS
minutes count ten times against the plan's included minutes, so each job runs only when the pull request, compared with its base
branch on every push, changes what it builds (`scripts/detect-app-changes.sh`):

- the Mac app when `desktop/`, `cli/` (the app bundles `hisar`), `core/`, `go.work`, its packaging scripts or the workflow
  changes (`scripts/app-paths.sh` lists them);
- the iPhone app when `mobile/ios/`, `mobile/hisarkit/`, `mobile/go.mod`, `mobile/build-framework.sh`, `core/`, `go.work` or the
  workflow changes; an Android-only change does not run it;
- both, whatever changed, while the pull request has the label `run-macos`;
- not the iPhone app while the pull request has the label `skip-ios`, for changes whose iPhone build, unit tests and UI tests
  already passed locally (`make build-ios-framework build-ios test-ios test-ios-ui`). Add the label before pushing: a run
  reads the labels the pull request has when it starts. The jobs then report as skipped;
- or by hand: Actions → macOS and iOS → Run workflow, on a branch (tick "Build and test the iPhone app" to add the iPhone job).

A run takes roughly 15–25 minutes for the Mac app, and for the iPhone app about 10 minutes of build then about 5 minutes of UI
tests in each of three parts, so about 150–250 included minutes for the Mac app alone and up to about 750 with the iPhone app. A desktop release build costs
about the same as the Mac app job.

## Releases

Releases are tags without a `v`: `server-X.Y.Z`, `cli-X.Y.Z`, `desktop-X.Y.Z`, `extension-X.Y.Z` and `docs-X.Y.Z`. Each release is one run of
Actions → Release → Run workflow (or `gh workflow run release.yml -f app=server -f type=minor`), on `main`:

- **App**: `server`, `cli`, `desktop`, `extension` or `docs`.
- **Release type**: `patch`, `minor` or `major` bumps the app's newest `<app>-X.Y.Z` tag (`0.1.14` → `0.1.15`, `0.2.0`,
  `1.0.0`; an app's first release is `0.1.0`), and `custom` takes **Version** as it is: `0.2.1`, or `0.3.0-rc1` for a custom
  build that is built the same way, marked "(custom build)" and can never be published. A custom build may run on any branch.
- **Dry run**: only the plan.
- **Skip the tests**: for emergencies only. The run skips CI and the macOS tests and goes straight to the tag and the build.
  The run name, its summary, `release-evidence.json` (`tests.ci: "skipped"`) and a warning on the publish step all say that
  the release was not tested; the approver of the publication sees it there. Everything else, the attestations included, is
  unchanged.

The run then goes on by itself (`scripts/plan-release.sh` plans it):

1. **Plan**: the next version, which must be newer than the previous release and not tagged yet, and the commits since the
   previous release that change what the app is built from (`scripts/app-paths.sh`, without tests, and without Markdown
   except for the docs site, which is made of it). It refuses a
   release without such commits, and warns about a patch release with a breaking change (`feat(core)!: …` or a
   `BREAKING CHANGE:` footer). The run summary shows the plan.
2. **CI** runs every `ci.yml` job on the exact event commit, plus the desktop jobs in `macos.yml` for desktop releases.
   Successful tests are mandatory, including custom builds, unless the run was started with **Skip the tests**; then both
   jobs are skipped and the evidence records it. A dry run stops after the plan.
3. **Tag**: the annotated tag on that commit; its message is the title and the changes, which the desktop app's Sparkle
   update shows as release notes. The deploy key in `RELEASE_DEPLOY_KEY` pushes it, past the tag ruleset.
4. **Build**: `release-<app>.yml`, which only the Release workflow calls. The desktop app builds the Mac DMG and Sparkle update
   and the Linux packages and Flatpak (amd64 and arm64); the server pushes `ghcr.io/tosunlab/hisar-server:X.Y.Z` and the
   docs site `ghcr.io/tosunlab/hisar-docs:X.Y.Z` (both amd64 and arm64). The extension builds the Chrome zip and the Firefox
   add-on, which addons.mozilla.org signs as unlisted (`web-ext sign`, with the extension's source attached for review) into
   `hisar-extension-firefox-X.Y.Z.xpi`. AMO signs each version once, even if its prerelease is never published, so the next
   release needs a new version; a custom build's Firefox package is the unsigned `hisar-extension-firefox-X.Y.Z-label-unsigned.zip`.
   Every builder checks out the immutable event SHA, not the release tag.
5. **Prerelease** contains every release file and `release-evidence.json`. The evidence binds the commit, test results (or that
   the tests were skipped), workflow run, file digests, and the server or docs image digest. Keyless GitHub artifact
   attestations cover every final file and the image index after packaging and signing. The run verifies them and rejects a moved release tag before creating the prerelease.
6. **Publish**: waits in the environment `publish-<app>` until a reviewer approves it (Review deployments on the run). A newer
   release of the app that reaches this step replaces an older one still waiting. It refuses a version older than the app's
   published release, verifies the evidence and attestations against the tag's full source SHA with the verifier from that
   commit, not from the default branch, then
   - server and docs: points `ghcr.io/tosunlab/hisar-server` or `hisar-docs` `:X.Y.Z`, `:X.Y` and `:latest` at the verified
     immutable image digest, so a version tag moved after the build is reset;
   - desktop: renames the release's `update-unpublished.json` to `update.json`, which offers the version to every Mac app as an
     automatic update;
   - extension: writes `updates.json`, naming this version's `.xpi` and its SHA-256, to the prerelease
     `extension-firefox-updates`; installed Firefox add-ons poll it (the manifest's `update_url`);
   - all: rewrites the notes to cover every change since the published release, turns the prerelease into a release and marks
     it latest.

A failed run continues with Re-run failed jobs; a new run refuses the tag it already pushed. Actions → Publish → Run workflow
publishes an existing prerelease by hand, through the same approval and attestation verification. Old prereleases without
evidence cannot use this path. Actions → Release server image (or Release docs image) → Run workflow on a branch first runs
mandatory exact-SHA CI, then pushes only `ghcr.io/tosunlab/hisar-server:sha-<commit>` (or `hisar-docs:sha-<commit>`), with
an image attestation, for testing.

## Verify release artifacts

Use a recent GitHub CLI with `gh attestation verify`. Choose the full commit SHA from an independently reviewed source
commit, not only a downloaded evidence document or a movable tag. Set `commit` to that SHA.
For a downloaded CLI archive, desktop file, extension zip, checksum file, or `release-evidence.json`, run:

```sh
gh attestation verify <file> --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

Verify every file you use. For the server, verify `release-evidence.json` with the command above, read its `image.digest`,
then verify and run that digest, not a mutable image tag:

```sh
digest=$(jq -r .image.digest release-evidence.json)
gh attestation verify "oci://ghcr.io/tosunlab/hisar-server@$digest" --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release-server.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

For the docs site, do the same with `ghcr.io/tosunlab/hisar-docs` and `release-docs.yml`.
The image signer is the reusable server or docs workflow. File signers are the release assembly workflow.
Verification constrains repository, workflow identity, source commit, and signer commit. A missing or invalid attestation
is a failure, not permission to trust a checksum instead. These checks identify the authorized workflow and its claims.
They do not prove that source, dependencies, or a compromised authorized builder are benign.

## Update pinned build images

`deploy/Dockerfile`, `docs/Dockerfile` and the build and CI workflow container images use multiarchitecture SHA-256 manifest digests.
The BuildKit builder image is pinned too. No image layers are pulled by these checks.

1. Run `python3 scripts/check-build-images.py --resolve-tags` to print current upstream tag digests.
2. Review upstream changes and replace the selected digest in every reference to that image.
   Keep the Go tag synchronized with `go.work` using `bash scripts/check-go-toolchain.sh`.
3. Run `python3 scripts/check-build-images.py --registry` to verify pinned manifest bytes and support for both
   `linux/amd64` and `linux/arm64`. Run `python3 scripts/check-build-images.py` for the offline pin check.
4. Run `bash scripts/test-app-paths.sh` and the affected image or packaging CI jobs before release.

The update process does not claim reproducible builds. Hosted runner images, package repositories, and other tools
remain separate build inputs. `scripts/app-paths.sh` includes the release workflows and evidence scripts, so changes to
these inputs select the affected releases.

## Repository secrets and environments

Repository secrets: `SPARKLE_PRIVATE_KEY`, the Sparkle EdDSA private key file's content (`gh secret set SPARKLE_PRIVATE_KEY --repo
tosunlab/hisar < <key file>`); `RELEASE_DEPLOY_KEY`, the private half of the deploy key "Release workflow tags" (write
access, a bypass actor of the ruleset "tags: only maintainers create"); and, for desktop releases, the team's Developer ID
Application identity and the App Store Connect API key that notarizes the app: `MACOS_SIGNING_CERTIFICATE` (the identity as a
base64 `.p12`: `base64 -i <identity.p12> | gh secret set MACOS_SIGNING_CERTIFICATE --repo tosunlab/hisar`),
`MACOS_SIGNING_CERTIFICATE_PASSWORD` (its export password), `MACOS_PROVISIONING_PROFILE` (the base64 macOS Developer ID
provisioning profile authorizing the app's keychain access group), `APPLE_NOTARY_KEY` (the base64 `.p8`), `APPLE_NOTARY_KEY_ID` and
`APPLE_NOTARY_ISSUER_ID`; for extension releases, an addons.mozilla.org API key that signs the Firefox add-on: `AMO_JWT_ISSUER`
and `AMO_JWT_SECRET` (the JWT issuer and secret from https://addons.mozilla.org/developers/addon/api/key/). Images go to the GitHub Container Registry with the
workflow's own `GITHUB_TOKEN`. The environments `publish-server`, `publish-cli`, `publish-desktop` and `publish-extension` have
the required reviewers and accept only `main`.

Once, before the first extension release, a maintainer creates the prerelease that holds the Firefox add-on's `updates.json`
(the tag ruleset lets only maintainers create its tag): `gh release create extension-firefox-updates --repo tosunlab/hisar
--prerelease --target main --title "Firefox add-on updates" --notes "The Hisar Firefox add-on updates from updates.json here;
publishing an extension release replaces it."`. Publishing an extension release fails until it exists.
