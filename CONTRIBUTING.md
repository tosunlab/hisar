# Contributing to Hisar

Thanks for helping. This page says how to report a problem, how to get a change in, and what a pull request has to pass.

## Reporting a bug or asking for a feature

Open an issue with the bug report or feature request template. Say which app (Mac, Linux, iPhone, Android, extension, `hisar`, server) and which version (Settings → About, `hisar --version`, the image tag).

**Security.** A vulnerability goes to [GitHub's private vulnerability reporting](https://github.com/tosunlab/hisar/security/advisories/new) or to [support@tryhisar.com](mailto:support@tryhisar.com?subject=Security%20report), never to a public issue. We reply within two working days and credit reporters who want it.

## Before you write code

- **The design comes first.** [docs/DESIGN.md](docs/DESIGN.md) is the source of truth for the security model, the data model and the sync protocol. A change that touches any of them starts as a change to that document, in the same pull request or an earlier one.
- **Know where code goes.** [CLAUDE.md](CLAUDE.md) maps every package, says which module may import which, lists the conventions, and names the things that must never break (wire JSON tags, HTTP routes, CLI commands and flags, the `core/crypt` formats and their frozen test vectors, the mobile framework's exported API). Read it before a change outside one package.
- **Set up locally.** The [Development](README.md#development) section of the README has the Postgres container, the environment variable and the `make` targets; the desktop, mobile and extension READMEs cover their apps.
- **Ask first for big things.** For a new feature or a change in behaviour, open an issue before the pull request, so that the shape is agreed before the work.

## Pull requests

- Branch from `main` and open one pull request per change. Keep refactors out of a feature pull request.
- Add or change tests with the code. Go tests sit next to their package, the desktop app and the extension have frontend tests, the phones have unit and UI tests; [CLAUDE.md § Tests](CLAUDE.md#tests) says how each suite runs.
- Update the documentation the change makes wrong: the README, the app's README, `docs/DESIGN.md` and `CLAUDE.md`.
- `make vet test` must pass. The CI runs gofmt, vet, the Go tests against Postgres, the CLI build for every platform, the end-to-end test of the built binaries, the server image, the desktop frontend, the extension, the admin dashboard, and the Mac, iPhone and Android apps when a change touches what they are built from. It runs on every push to a pull request and never on `main`.
- The Mac and iPhone jobs are expensive and run only when `desktop/`, `cli/`, `mobile/`, `core/` or their scripts change. Labels steer them: `skip-ios` and `skip-android` when those builds and tests already passed locally, `run-macos` to force the Mac and iPhone jobs. Add a label before pushing: a run reads the labels the pull request has when it starts. Details: [docs/RELEASING.md](docs/RELEASING.md).
- A maintainer reviews the pull request, and it merges once CI is green. Nothing lands on `main` without a pull request: a merge was already tested as one.

## Commit messages

Conventional Commits with the module as scope: `feat(core): …`, `fix(desktop): …`, `ci(release): …`, `docs: …`. A breaking change carries `!` after the scope (`feat(core)!: …`) or a `BREAKING CHANGE:` footer; the release tooling warns when a patch release contains one. The subject lines become the release notes, so write what changed for a user, not what you did.

Do not add a `Co-Authored-By: Claude …` trailer or any other AI attribution line.

## Code conventions

The short version of [CLAUDE.md § Conventions](CLAUDE.md#conventions):

- Idiomatic Go names, a doc comment on every package, errors wrapped with `%w` and tested with `errors.Is` and `errors.As`.
- Low-level packages prefix errors with the package name; domain packages return sentences a user can read.
- No mutable exported package variables for policy; constants or functions.
- Platform files use the filename suffix (`_darwin.go`, `_linux.go`, `_windows.go`), with a fallback file for the inverse build tag.
- `core` imports no other module. The server's image copies only `core/`, `server/` and `admin/`.
- A new request field that an older app would ignore is announced as a `cliprotocol` feature, so an old `hisar` and a new app keep working together.

## Releases

Maintainers cut releases from `main` with the Release workflow; the steps, the tag names and the publish approval are in [docs/RELEASING.md](docs/RELEASING.md).
