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
2. Add the server to your agent:

   ```sh
   claude mcp add hisar -- hisar mcp        # Claude Code
   ```

   Cursor, in `~/.cursor/mcp.json` or a project's `.cursor/mcp.json`:

   ```json
   { "mcpServers": { "hisar": { "command": "hisar", "args": ["mcp"] } } }
   ```

   Any other MCP client: a stdio server with the command `hisar` and the argument `mcp`.

The agent gets these tools:

| Tool | What it does | Asks you |
|---|---|---|
| `list_vaults` | the vaults available to agents | once per agent connection |
| `search_items` | items by title, username, URL or vault: metadata, field labels and `hisar://` references, never values or notes | once per agent connection |
| `run_with_secrets` | runs a command (an argument list, no shell) with references in its environment; output masked, at most 64 KiB per stream, timeout up to 600 s | every call, showing the command, directory and secrets; "Allow these secrets for this agent for 1 hour" |
| `get_one_time_code` | the current one-time code of an item | every call; can be allowed for 1 hour |
| `get_secret` | reveals one value to the agent | every call, for that item; "Allow this item for 1 hour". `private key` is always refused |

The first dialog names the agent (from its MCP client name) and the app it runs in. Denying fails the tool with "The user denied
access in Hisar.", which the agent can relay to you. Approvals and allowed secrets end when the agent disconnects or Hisar locks.

Limits: a command run with secrets has the real values and can write them to a file, send them elsewhere or print them encoded,
which masking does not catch, so approve only commands you would run yourself; allowing secrets for an hour lets the agent run any
command with them for that hour. A value from `get_secret` is in the agent's transcript and logs. Details:
[docs/DESIGN.md](docs/DESIGN.md) §9.4.2.

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

GitHub Actions runs CI on pull requests and on manual runs (Actions → CI → Run workflow), never on pushes to `main`: a merge was already
tested as its pull request. A new push to a pull request cancels its running CI.

`.github/workflows/ci.yml` (Linux runners): gofmt, vet, all Go tests against Postgres (failing on any skipped test except the
`TestIntegration…` tests and `TestRecordOpenSSHFixture`), the CLI for every release platform, the end-to-end test of the built
binaries, the server Docker image for amd64 and arm64 (built into a throwaway registry and booted, never pushed), the desktop frontend
(typecheck, tests, bundle check) and the desktop app's Linux build (both only when the pull request changes the desktop app or what it builds from, as below), the extension (tests, Chromium end-to-end tests, zip),
the share link page in Chromium against the server, the admin dashboard (typecheck, tests, the `admin/src/themes.css`
freshness check and Chromium end-to-end tests against the server; only when the pull request changes `admin/`, `server/`,
`core/`, `go.work` or the workflow), and the Android app (`HisarKit.aar`, the `ThemePalettes.kt` freshness check, the debug and
release APKs, lint, the JVM tests and the instrumented tests on an x86_64 API 37 emulator against a local server; only when the
pull request changes `mobile/android/`, `mobile/hisarkit/`, `mobile/go.mod`, the Android scripts, the themes, `core/`, `go.work`
or the workflow).

`.github/workflows/macos.yml` builds the Mac app (Touch ID and Sparkle tests, bundle and signature checks) and the iPhone app
(HisarKit, Xcode project check, unit tests, UI tests against a local server) on GitHub's `macos-26` runners with Xcode 26.6. macOS
minutes count ten times against the plan's included minutes, so each job runs only when the pull request, compared with its base
branch on every push, changes what it builds (`scripts/detect-app-changes.sh`):

- the Mac app when `desktop/`, `cli/` (the app bundles `hisar`), `core/`, `go.work` or the workflow changes;
- the iPhone app when `mobile/ios/`, `mobile/hisarkit/`, `mobile/go.mod`, `mobile/build-framework.sh`, `core/`, `go.work` or the
  workflow changes; an Android-only change does not run it;
- both, whatever changed, while the pull request has the label `run-macos`;
- not the iPhone app while the pull request has the label `skip-ios`, for changes whose iPhone build, unit tests and UI tests
  already passed locally (`make build-ios-framework build-ios test-ios test-ios-ui`). Add the label before pushing: a run
  reads the labels the pull request has when it starts. The job then reports as skipped;
- or by hand: Actions → macOS and iOS → Run workflow, on a branch (tick "Build and test the iPhone app" to add the iPhone job).

A run takes roughly 15–25 minutes for the Mac app and 40–60 minutes for the iPhone app, so about 150–250 included minutes for the Mac
app alone and up to 850 with the iPhone app. A desktop release build costs about the same as the Mac app job.

Releases are tags without a `v`: `server-X.Y.Z`, `cli-X.Y.Z`, `desktop-X.Y.Z` and `extension-X.Y.Z`. Pushing one runs its release
workflow, which builds it into a GitHub prerelease (or, for the server, pushes the image) that nobody gets yet. A custom build such as
`desktop-1.0.0-alpha-test` is built the same way and marked "(custom build)", and can never be published. Publishing is a separate,
deliberate step: Actions → Publish → Run workflow with the app and the tag. It refuses anything but a plain `<app>-X.Y.Z` tag, then

- server: points `ghcr.io/tosunlab/hisar-server:latest` and `:X.Y` at `:X.Y.Z`;
- desktop: renames the release's `update-unpublished.json` to `update.json`, which offers the version to every app as an automatic update;
- cli, extension (and desktop): turns the prerelease into a release and marks it latest.

A tag pushed without the workflows in its tree (one from before the move to GitHub) is built by running its release workflow by hand
with the tag as input: the workflow comes from `main`, the code from the tag.

Repository secrets: `SPARKLE_PRIVATE_KEY`, the Sparkle EdDSA private key file's content (`gh secret set SPARKLE_PRIVATE_KEY --repo
tosunlab/hisar < <key file>`). Images go to the GitHub Container Registry with the workflow's own `GITHUB_TOKEN`.

Deployment: see [deploy/README.md](deploy/README.md).
