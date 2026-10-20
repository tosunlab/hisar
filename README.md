# Hisar

End-to-end encrypted password, SSH-key and secrets manager for developers, hosted at api.tryhisar.com.

The server stores only ciphertext. Keys are derived on your devices from your master password and a Secret Key that never leaves them. See [docs/DESIGN.md](docs/DESIGN.md).

## Layout

| Path | What |
|---|---|
| `core/` | Shared Go library: crypto, item model, sessions and sync, and `core/app`, the app backend of the Mac, the iPhone and Android |
| `server/` | `hisar-server` sync server (Go + Postgres) |
| `cli/` | `hisar` command-line client |
| `desktop/` | Desktop app (Wails v3 + Vue), see [desktop/README.md](desktop/README.md) |
| `extension/` | Browser extension |
| `mobile/` | iOS and Android apps, see [mobile/README.md](mobile/README.md) |
| `docs/` | Design docs |

Where each package is and where new code goes: [CLAUDE.md](CLAUDE.md).

## Desktop app on Linux

Each `desktop-<version>` GitHub Release has `Hisar-<version>-linux-<arch>.deb` and `.rpm` for amd64 and arm64, built for
GTK 4 and WebKitGTK 6.0 (Ubuntu 24.04+, Debian 13+, Fedora 40+). Check them against `SHA256SUMS-linux-<arch>`, then:

```sh
sudo apt install ./Hisar-0.3.0-linux-amd64.deb     # Fedora: sudo dnf install ./Hisar-0.3.0-linux-amd64.rpm
```

The package installs the app (`/usr/bin/Hisar`, in the app menu), `hisar` at `/usr/bin/hisar`, and the browser
extension's host. On other distributions, run `Hisar-<version>-linux-<arch>.AppImage` (it needs the system's GTK 4 and
WebKitGTK 6.0, for example `sudo pacman -S gtk4 webkitgtk-6.0` on Arch; without them it says which packages to install), or install the Flatpak, which brings them with the GNOME runtime: `flatpak install --user
Hisar-<version>-linux-<arch>.flatpak`. Both copy `hisar` to `~/.local/share/hisar/bin`; Settings → Command-line tool
links it onto your `PATH`. The Secret Key goes into the desktop's keyring, so a Secret Service has to run (GNOME Keyring or
KWallet); without one, set `HISAR_SECRET_KEY`. What works as on the Mac and what differs (no Touch ID, update notices
instead of installing updates, no app names in approvals): [docs/DESIGN.md §9.1.3](docs/DESIGN.md).

## Command-line tool

`hisar` lists, reads and edits your items from a terminal or a script (`hisar --help`).

**On a Mac with the Hisar app**, the app includes it: Settings → Developer tools → **Install command-line tool**. `hisar` then works
through the running app, which asks you to approve each terminal with Touch ID; `hisar status` shows whether it uses the app.
Details: [desktop/README.md](desktop/README.md#command-line-tool).

**Keep the app running.** SSH clients, `git` and commit signing reach Hisar through `~/.hisar/agent.sock`, and they cannot start the
app themselves: while Hisar is not running they fail at once with "Connection refused" or "Permission denied (publickey)". So the app
opens at login (Settings → General → **Open Hisar at login**, on by default) and keeps running in the menu bar when you close its window;
⌘Q or **Quit Hisar** in the menu bar quits it. `hisar` itself starts the app when it is installed but not running, waits up to 10 seconds
for it, and otherwise says "Hisar isn't running. Open the Hisar app and try again."; `HISAR_NO_LAUNCH=1` turns that off for scripts and CI.

**Anywhere else** (Linux, Windows, servers, or without the app), download the standalone `hisar` from the published GitHub Release of a `cli-<version>` tag
(github.com/tosunlab/hisar → Releases): `hisar-<version>-<os>-<arch>.tar.gz` for Linux and macOS,
`.zip` for Windows, and `SHA256SUMS`. Check the download before you run it, then put `hisar` on your `PATH`:

```sh
sha256sum -c --ignore-missing SHA256SUMS          # macOS: shasum -a 256 -c --ignore-missing SHA256SUMS
tar -xzf hisar-0.2.1-linux-amd64.tar.gz && sudo install hisar /usr/local/bin/
hisar --version
hisar login                                       # its own sign-in: server, email, Secret Key, master password
```

On Windows, `Get-FileHash hisar-0.2.1-windows-amd64.zip -Algorithm SHA256` must match the line in `SHA256SUMS`. The standalone
`hisar` keeps its session in `state.json` and asks for the master password for each command; `hisar --standalone` (or
`HISAR_STANDALONE=1`) uses this mode even when the Mac app is running.

### Secrets for commands: `hisar run` and `hisar read`

Name a secret with a reference instead of copying it: `hisar://<vault>/<item>/<field>`. Vault and item are names (case does not
matter) or IDs; the field is `password`, `username`, `notes`, `url`, `otp` (the current one-time code), `public key`,
`fingerprint` or a custom field's label. SSH private keys never leave Hisar (use its SSH agent), so `private key` references are refused. Characters such as `/`, `%` and spaces are percent-encoded (`AWS%20prod`). In the
Hisar app, the terminal button next to a field's copy button copies its reference.

Keep references, not values, in your `.env`:

```sh
# .env — safe to commit; it holds no secrets
DATABASE_URL=postgres://localhost:5432/app
DATABASE_PASSWORD=hisar://Work/Postgres%20staging/password
GITHUB_TOKEN=hisar://Personal/GitHub/API%20token
AWS_SECRET_ACCESS_KEY="hisar://Work/AWS prod/secret access key"
```

```sh
hisar run --env-file .env -- npm test
hisar run --env-file .env -- docker compose up
GITHUB_TOKEN=hisar://Personal/GitHub/API%20token hisar run -- gh release list
hisar read hisar://Personal/GitHub/otp            # one value on stdout, for scripts
```

`hisar run` resolves every reference in one request, so the app asks once (with Touch ID, or on your smart device) and shows the command
and the secrets it reads. The command gets the real values in its environment; nothing is written to disk. Every value it prints is
replaced with `<concealed by hisar>`. That needs pipes, so use `--no-masking` for commands that need the terminal itself (colours,
prompts, full-screen tools). Several `--env-file` options are applied in order; variables already in your environment win unless they
are references themselves. Signals are passed on, and `hisar run` exits with the command's exit code. Approving covers the terminal
session: other `hisar` commands in it don't ask again until 10 minutes after the last one, or until Hisar locks.

**With coding agents.** Give the agent the `.env` with references and let it run `hisar run --env-file .env -- <command>`. It can
run your tests, migrations and deploys, but what it sees (and what ends up in its transcript and logs) has the secrets masked, and the
`.env` it reads holds only references. Masking is a guard against showing secrets, not a sandbox: the command itself has the values,
and an agent that can run `hisar read` in an approved terminal can read them. An approval covers every vault, so approve an agent's
terminal only when it may use them. Details: [docs/DESIGN.md](docs/DESIGN.md) §9.4.1. For agents that speak MCP, `hisar mcp`
(below) is narrower: it sees only the vaults you choose and asks per command.

### Use Hisar with coding agents

`hisar mcp` is an [MCP](https://modelcontextprotocol.io) server on stdio that lets coding agents use your secrets without seeing
them. It always works through the Hisar app, which must be running with Settings → Developer tools → "Let the command-line tool
use Hisar" on.

1. In Hisar, open Settings → Developer tools → Coding agents and turn on "Available to agents" for the vaults an agent may use. Every vault starts
   off, and the setting is per device. Agents cannot see the other vaults at all.
2. Connect your agent. Claude Code, Codex and omp connect with one click in the same section (below). Cursor, in
   `~/.cursor/mcp.json` or a project's `.cursor/mcp.json`:

   ```json
   { "mcpServers": { "hisar": { "command": "hisar", "args": ["mcp", "--no-reveal"] } } }
   ```

   Any other MCP client: a stdio server with the command `hisar` and the arguments `mcp --no-reveal`. `--no-reveal` leaves out
   `get_secret` and `get_one_time_code`, so no secret value can reach the agent through MCP; drop it only for an agent you let read
   values.

The agent gets these tools:

| Tool | What it does | Asks you |
|---|---|---|
| `list_vaults` | the vaults available to agents | once per agent connection |
| `search_items` | items by title, username, URL or vault: metadata, field labels and `hisar://` references, never values or notes | once per agent connection |
| `run_with_secrets` | runs a command, as an argument list or a `/bin/sh -c` command line, with secrets from `env` (variable → `hisar://` reference), `envFiles` (as `hisar run --env-file`) and `stdin` (text or one reference); through `direnv exec` when the directory has an allowed `.envrc`; output masked, at most 64 KiB per stream and streamed as progress to clients that ask, timeout up to 1 hour | every call, showing the command with the env files' plain values (`env NAME=value …`) and its stdin, the directory and secrets; "Allow these secrets for this agent for 1 hour" |
| `get_one_time_code` | the current one-time code of an item | every call; can be allowed for 1 hour |
| `get_secret` | reveals one value to the agent | every call, for that item; "Allow every field of “&lt;item&gt;” for this agent for 1 hour". `private key` is always refused |

The first dialog names the agent (from its MCP client name) and the app it runs in. Denying fails the tool with "The user denied
access in Hisar.", which the agent can relay to you. Approvals and allowed secrets end when the agent disconnects or Hisar locks.

Limits: a command run with secrets has the real values and can write them to a file, send them elsewhere or print them encoded,
which masking does not catch, so approve only commands you would run yourself; allowing secrets for an hour lets the agent run any
command with them for that hour. A value from `get_secret` is in the agent's transcript and logs. Details:
[docs/DESIGN.md](docs/DESIGN.md) §9.4.2.

### Claude Code

In Hisar, open Settings → Developer tools → Coding agents and click **Connect** next to Claude Code. Hisar finds `claude` on your
PATH, in `~/.local/bin`, `/opt/homebrew/bin`, `/usr/local/bin` or `~/.claude/local`, and adds the `hisar` inside Hisar.app to
your user settings (`~/.claude.json`, or `$CLAUDE_CONFIG_DIR/.claude.json`):

```sh
claude mcp add --scope user --transport stdio hisar -- /Applications/Hisar.app/Contents/Helpers/hisar mcp --no-reveal
```

That command is also the way to connect by hand. New Claude Code sessions load Hisar; restart the ones already running.
**Disconnect** runs `claude mcp remove hisar --scope user`. If your settings already have a server named `hisar` that Hisar did not
add, or Hisar's with a setting you added or changed, the card says "Claude Code already has a server named hisar. Remove it to
connect Hisar." and Hisar never changes it.
Commands Claude Code runs in its shell get the coding-agent rules, as omp's do (below).

### Codex

Click **Connect** next to Codex in the same section. Hisar finds `codex` on your PATH, in `/opt/homebrew/bin`, `/usr/local/bin`,
`~/.npm-global/bin` or `~/.local/bin`, and adds this table to `~/.codex/config.toml` (or `$CODEX_HOME/config.toml`), leaving the
rest of the file as it was:

```toml
[mcp_servers.hisar]
command = "/Applications/Hisar.app/Contents/Helpers/hisar"
args = ["mcp", "--no-reveal"]
startup_timeout_sec = 15
tool_timeout_sec = 4200
```

To connect by hand, add the same table: `codex mcp add` cannot set the timeouts, and Codex's default of 60 seconds per tool call
would end a `run_with_secrets` call while its approval is still open. New Codex sessions load Hisar; restart the ones already
running. **Disconnect** runs `codex mcp remove hisar`. A `hisar` server that Hisar did not add, or this table with a key you added
or changed, is left alone, as for Claude Code.
Codex's sandbox blocks Unix sockets for the commands it runs, so `hisar run` in Codex's shell cannot reach the app; Codex uses
your secrets through `run_with_secrets`, which runs outside the sandbox.

### omp and Tern

Hisar comes with a plugin for omp, the coding agent that also runs in Tern's panes. Connecting it gives omp both the plugin and
Hisar's MCP server as `hisar mcp --no-reveal`, so omp runs commands with your secrets without seeing them, and its status line
shows whether Hisar is locked.

1. In Hisar, open Settings → Developer tools → Coding agents and turn on "Available to agents" for the vaults omp may use, as for
   `hisar mcp` above.
2. Next to omp in the same section, click **Connect**. Hisar finds omp on your PATH, in `~/bin`, in Tern
   (`~/Library/Application Support/Tern/omp/bin`) or in `~/.bun/bin`, and runs `omp plugin link` with the plugin inside Hisar.app.
   The plugin starts the MCP server with the `hisar` inside Hisar.app. New omp sessions load Hisar; restart the ones already
   running. **Disconnect** runs `omp plugin uninstall @tryhisar/omp-plugin`, which removes the MCP server too.
   On Linux, link the plugin from a checkout of this repository: `omp plugin link <checkout>/omp`.
3. Start omp. Its status line says "Hisar locked" or "Hisar unlocked", and `/hisar` shows Hisar's state. The agent gets
   `list_vaults`, `search_items` and `run_with_secrets`, never `get_secret` or `get_one_time_code`. omp shows nothing of a
   `run_with_secrets` command's output until it ends, so for a long command, one that asks for input or one that needs the shell's
   environment, the agent runs `hisar run --env-file .env -- <command>` in its shell instead.

Every `hisar` command omp runs, in its shell or as `hisar mcp`, gets the coding-agent rules
([docs/DESIGN.md](docs/DESIGN.md) §9.4.4): it sees only the vaults available to agents, `hisar run` asks once per secret for each omp
session, `hisar read` and `hisar get` ask per item, and changes to items are refused. The approval names omp and the app it runs in
("omp in Tern wants to use 1 secret"). Approvals end when that omp exits or Hisar locks. ssh and git inside omp get only the SSH
keys in vaults available to agents. The app omp runs in, such as Tern, becomes an agent host: its other processes without a
terminal then ask on every connection, and it can no longer be a launcher.

## Development

Requires Go 1.26+, Docker (for Postgres) and Node 24 (`make build` runs `npm ci` and Vite to build the admin dashboard into
`hisar-server`).

```sh
docker run -d --name hisar-pg -e POSTGRES_PASSWORD=hisar -e POSTGRES_DB=hisar -p 127.0.0.1:55432:5432 postgres:18
export HISAR_TEST_DATABASE_URL="postgres://postgres:hisar@127.0.0.1:55432/hisar?sslmode=disable"

make vet test build      # database tests drop and recreate the public schema of that database
make build-cli-all       # cross-compile the CLI
make package-cli-release VERSION=0.2.1   # the release archives and SHA256SUMS in bin/cli-release
make docker              # build the server image
```

End-to-end test of the built server and CLI binaries against an empty database:

```sh
docker exec hisar-pg createdb -U postgres hisar_end_to_end
HISAR_DATABASE_URL="postgres://postgres:hisar@127.0.0.1:55432/hisar_end_to_end?sslmode=disable" make test-end-to-end
```

The phone apps build from `mobile/`: the iPhone app with Xcode on a Mac, and the Android app with the Android SDK and JDK 21
on macOS or Linux (`make build-android-aar build-android test-android`). See [mobile/README.md](mobile/README.md).

### CI and releases

GitHub Actions runs CI on pull requests, on manual runs (Actions → CI → Run workflow) and on a release that asks for it, never
on pushes to `main`: a merge was already tested as its pull request. A new push to a pull request cancels its running CI.

`.github/workflows/ci.yml` (Linux runners): gofmt, vet, all Go tests against Postgres (failing on any skipped test except the
`TestIntegration…` tests and `TestRecordOpenSSHFixture`), the CLI for every release platform, the end-to-end test of the built
binaries, the server Docker image for amd64 and arm64 (built into a throwaway registry and booted, never pushed), the desktop frontend
(typecheck, tests, bundle check) and the desktop app's Linux build (both only when the pull request changes the desktop app or what it builds from, as below), the extension (tests, Chromium end-to-end tests, zip),
the share link page in Chromium against the server, the admin dashboard (typecheck, tests, the `admin/src/themes.css`
freshness check and Chromium end-to-end tests against the server; only when the pull request changes `admin/`, `server/`,
`core/`, `go.work` or the workflow), and the Android app (`HisarKit.aar`, the `ThemePalettes.kt` freshness check, the debug and
release APKs, lint, the JVM tests and the instrumented tests on x86_64 API 36 emulators against a local server, split over two
jobs; only when the pull request changes `mobile/android/`, `mobile/hisarkit/`, `mobile/go.mod`, the Android scripts, the themes,
`core/`, `go.work` or the workflow, and not while the pull request has the label `skip-android`, for changes whose Android build
and tests already passed locally with `make build-android-aar build-android test-android test-android-ui`; add the label before
pushing).

`.github/workflows/macos.yml` builds the Mac app (Touch ID and Sparkle tests, bundle and signature checks) and the iPhone app
(HisarKit, Xcode project check, unit tests, then UI tests against a local server in two jobs that run the one build) on
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

A run takes roughly 15–25 minutes for the Mac app, and for the iPhone app about 10 minutes of build then 20 minutes of UI tests in
two jobs, so about 150–250 included minutes for the Mac app alone and up to 850 with the iPhone app. A desktop release build costs
about the same as the Mac app job.

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

Repository secrets: `SPARKLE_PRIVATE_KEY`, the Sparkle EdDSA private key file's content (`gh secret set SPARKLE_PRIVATE_KEY --repo
tosunlab/hisar < <key file>`); `RELEASE_DEPLOY_KEY`, the private half of the deploy key "Release workflow tags" (write
access, a bypass actor of the ruleset "tags: only maintainers create"); and, for desktop releases, the team's Developer ID
Application identity and the App Store Connect API key that notarizes the app: `MACOS_SIGNING_CERTIFICATE` (the identity as a
base64 `.p12`: `base64 -i <identity.p12> | gh secret set MACOS_SIGNING_CERTIFICATE --repo tosunlab/hisar`),
`MACOS_SIGNING_CERTIFICATE_PASSWORD` (its export password), `APPLE_NOTARY_KEY` (the base64 `.p8`), `APPLE_NOTARY_KEY_ID` and
`APPLE_NOTARY_ISSUER_ID`. Images go to the GitHub Container Registry with the
workflow's own `GITHUB_TOKEN`. The environments `publish-server`, `publish-cli`, `publish-desktop` and `publish-extension` have
the required reviewers and accept only `main`.

Deployment: see [deploy/README.md](deploy/README.md).
