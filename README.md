# Hisar

Self-hosted, end-to-end encrypted password manager for family and friends.

The server stores only ciphertext. Keys are derived on your devices from your master password and a Secret Key that never leaves them. See [docs/DESIGN.md](docs/DESIGN.md).

## Layout

| Path | What |
|---|---|
| `core/` | Shared Go library: crypto, item model, local store, sync |
| `server/` | `hisar-server` sync server (Go + Postgres) |
| `cli/` | `hisar` command-line client |
| `desktop/` | Desktop app (Wails) |
| `extension/` | Browser extension |
| `mobile/` | iOS and Android apps |
| `docs/` | Design docs |

## Development

Requires Go 1.26+.

```sh
make vet test build
./bin/hisar-server -addr :8080
```
