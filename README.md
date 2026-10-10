# Hisar

Self-hosted, end-to-end encrypted password manager for family and friends.

The server stores only ciphertext. Keys are derived on your devices from your master password and a Secret Key that never leaves them. See [docs/DESIGN.md](docs/DESIGN.md).

## Layout

| Path | What |
|---|---|
| `core/` | Shared Go library: crypto, item model, local store, sync |
| `server/` | `hisar-server` sync server (Go + Postgres) |
| `cli/` | `hisar` command-line client |
| `desktop/` | Desktop app (Wails v3 + Vue), see [desktop/README.md](desktop/README.md) |
| `extension/` | Browser extension |
| `mobile/` | iOS and Android apps |
| `docs/` | Design docs |

## Command-line tool

`hisar` lists, reads and edits your items from a terminal or a script (`hisar --help`).

**On a Mac with the Hisar app**, the app includes it: Settings → Command-line tool → **Install command-line tool**. `hisar` then works
through the running app, which asks you to approve each terminal with Touch ID; `hisar status` shows whether it uses the app.
Details: [desktop/README.md](desktop/README.md#command-line-tool).

**Anywhere else** (Linux, Windows, servers, or without the app), download the standalone `hisar` from the release of a `v*` tag on
GitLab (Deploy → Releases, or the generic package `hisar-cli/<version>`): `hisar-<version>-<os>-<arch>.tar.gz` for Linux and macOS,
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

## Development

Requires Go 1.26+ and Docker (for Postgres).

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

CI (`.github/workflows/ci.yml`) runs gofmt, vet, all tests against Postgres (failing if any are skipped), CLI cross-compiles, the end-to-end test of the built binaries, and builds and boots the server Docker image.

Deployment: see [deploy/README.md](deploy/README.md).
