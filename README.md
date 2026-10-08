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

## Development

Requires Go 1.26+ and Docker (for Postgres).

```sh
docker run -d --name hisar-pg -e POSTGRES_PASSWORD=hisar -e POSTGRES_DB=hisar -p 127.0.0.1:55432:5432 postgres:18
export HISAR_TEST_DATABASE_URL="postgres://postgres:hisar@127.0.0.1:55432/hisar?sslmode=disable"

make vet test build      # database tests drop and recreate the public schema of that database
make build-cli-all       # cross-compile the CLI
make docker              # build the server image
```

End-to-end test of the built server and CLI binaries against an empty database:

```sh
docker exec hisar-pg createdb -U postgres hisar_end_to_end
HISAR_DATABASE_URL="postgres://postgres:hisar@127.0.0.1:55432/hisar_end_to_end?sslmode=disable" make test-end-to-end
```

CI (`.github/workflows/ci.yml`) runs gofmt, vet, all tests against Postgres (failing if any are skipped), CLI cross-compiles, the end-to-end test of the built binaries, and builds and boots the server Docker image.

Deployment: see [deploy/README.md](deploy/README.md).
