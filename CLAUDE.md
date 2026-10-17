# Hisar codebase map

Hisar is an end-to-end encrypted password, SSH-key and secrets manager: the server stores only ciphertext, and keys are
derived on devices from the master password and a Secret Key. Cryptography, protocols and product decisions are in
[docs/DESIGN.md](docs/DESIGN.md); this file says where code lives and where new code goes.

## Modules

Five Go modules joined by `go.work`, plus three non-Go apps.

| Module | Builds | May import |
|---|---|---|
| `core/` (`github.com/tryhisar/hisar/core`) | The shared library, including the app backend `core/app` | no other module |
| `server/` | `hisar-server` and test fixtures | `core` |
| `cli/` | `hisar` | `core` |
| `desktop/` | The Wails app, `hisar-browser-host`, `hisar-update-key` | `core`; `cli/mcpserver` in tests only |
| `mobile/` | `HisarKit.xcframework` (gomobile) for `mobile/ios` | `core`; `desktop` in one integration test only |
| `extension/` | The browser extension (TypeScript, MV3) | none |
| `admin/` | The admin dashboard (Vue, TypeScript) that `hisar-server` embeds and serves at `/admin` | none; its crypto is `server/cmd/hisar-admin-wasm` |
| `mobile/ios/` | The iOS app (Swift), on top of HisarKit | none |

`core` must never require `cli`, `desktop` or `server`: the server's Dockerfile copies only `core/`, `server/` and `admin/`.

## Package map

### core

| Package | What it holds |
|---|---|
| `crypt` | Every cryptographic operation and AAD label: sealing, Argon2id/HKDF, keyset, vault keys, items, approvals, share links, exports, Secret Key. `vectors_test.go` freezes the formats |
| `pake` | The OPAQUE configuration shared by apps and server |
| `apitypes` | JSON wire types of `/api/v1`, event names, roles, platform headers, share-link limits, `IsValidVaultRole`, `NormalizeEmail` |
| `apiclient` | Typed HTTP client of `/api/v1` and its event stream |
| `session` | A signed-in account: sign-up, login, unlock, item sync and encryption, rotation, sharing, share links, organization vaults (`CompleteVaultKeyRequests`, `RotatePendingVaults`), account changes, `Reauthenticate` (the OPAQUE proof that account changes carry), the account preferences record with the favorites, `GetVaultPermissions` |
| `item` | The plaintext item model and its categories (`schema.go`: types kept as named fields; `card.go`, `identity.go`, `apicredential.go`, `license.go`, `server.go`, `database.go`, `address.go`, whose fields identities and cards share), the chosen icon and its presets (`icon.go`), `LookupField`, the password generator |
| `itemicon` | Icon images: normalizing uploads and website icons into small PNGs, and fetching website icons from public https sites only |
| `cache` | The offline SQLite copy of the server's encrypted records, and the device's sealed website icons |
| `localstate` | `state.json` (session state plus embedded device `Settings`) and the Secret Key in the keychain |
| `app` | The backend of the Mac and iPhone apps, bound to the UI (see below) |
| `app/apptest` | An unlocked `app.Service` on an in-memory API, for tests outside `core/app` |
| `cliprotocol` | The CLI ↔ app protocol on `~/.hisar/cli.sock`: messages, results, framing, `Client` |
| `clisocket` | The app side of that socket: server, terminal-session approvals, coding-agent grants, peer credentials |
| `browserprotocol` | The extension ↔ app native-messaging protocol, `Handler`, `Answer`, URL matching |
| `localsocket` | Unix sockets only the user can reach, and the process on the other end |
| `sshagent` | The read-only SSH agent with approvals and session binding |
| `sshkey` | SSH key generation, import and parsing |
| `passkey` | The WebAuthn authenticator |
| `approval` | Phone approval requests and signed answers |
| `sharelink` | Building, sealing and opening share links |
| `secretreference` | `hisar://vault/item/field` references |
| `export`, `importer` | Hisar's export format; importers for other password managers |
| `invitelink`, `appversion`, `totp`, `emergencykit` | Invite links, app versions, one-time codes, the Emergency Kit PDF |

**`core/app`** is one package because every feature shares the unlocked session and its caches under one mutex. Files:
`service.go`, `auth.go`, `lock.go`, `vaults.go`, `items.go`, `clipboard.go`, `errors.go`, `account.go`, `sharing.go`,
`invitations.go`, `sharelink.go`, `browser.go` (extension pairing), `logins.go` (login matching), `passkey*.go`,
`platformpasskey.go` (iOS), `commandline*.go` and `codingagent.go` (the CLI socket handler), `approvals.go`,
`phoneapproval.go` (Mac side), `phoneapproval_answer.go` (iPhone side), `import.go`, `export.go`, `settings.go`, `favorites.go`, `websiteicons.go` (the sealed website icon cache and its setting), `addresses.go` (addresses for the extension),
`organizations.go` (organizations, teams, organization vaults, and delivering and rotating their keys on every sync).
Shared test fixtures are in `helpers_test.go`, the in-memory API in `fakeapi_test.go`.

### server

| Package | What it holds |
|---|---|
| `cmd/hisar-server` | `serve`, `migrate`, `invite`; `setup.go` reads `HISAR_*` and wires everything |
| `cmd/hisar-admin-wasm` | The admin dashboard's crypto (`js && wasm`, imports only `core`): `signIn` and `reauthenticate` on `globalThis.hisar` |
| `internal/api` | HTTP only: routes, sessions, per-IP limits, event stream, invite and share-link pages, the admin dashboard (`adminpage.go` serves the embedded `adminpage/dist`) and the `/api/v1/admin` routes, each behind `requireSession` and `requireAdmin`; handlers parse and call the service |
| `internal/service` | The rules: rights, validation, store calls, audit, events, email. One `Service` type, a file per domain (`vaults`, `auth`, `accounts`, `lifecycle`, `invitations`, `sharelinks`, `approvals`, `preferences`, `organizations`) |
| `internal/store` | Postgres with raw SQL (pgx) and embedded goose migrations |
| `internal/realtime` | The per-account event broker behind `GET /api/v1/events` |
| `internal/ratelimit` | Per-key token buckets |
| `internal/mail`, `internal/desktopupdate` | SMTP; the Sparkle appcast from GitHub Releases |
| `cmd/hisar-fake-github`, `cmd/hisar-smtp-sink`, `cmd/hisar-share-link-page-fixture`, `cmd/hisar-admin-page-fixture`, `internal/desktopupdate/fakegithub`, `internal/mail/mailtest` | Fakes and fixtures for the end-to-end and browser tests |

### cli

| Package | What it holds |
|---|---|
| `cmd/hisar` | `main.go` (command table, dispatch, `main.version`), `backend.go` (the `backend` interface), `backend_app.go`, `backend_standalone.go`, and one file per command group |
| `mcpserver` | `hisar mcp` (public: desktop tests drive it) |
| `internal/dotenv` | `.env` parsing and layout-preserving rewriting |
| `internal/masking` | Masking secrets in command output |

### desktop

| Package | What it holds |
|---|---|
| root (`main`) | Wails wiring: `main.go`, `adapters.go`, `windows.go`, `events.go`, `tray.go`, `touchid.go` |
| `agent` | SSH agent in the app and the approval queue (`approver*.go`, `presenter.go`, `purpose.go`, `sshconfig.go`) |
| `appinfo` | The app behind a process (`resolver.go`, `chain.go`, `findapp.go`, `bundle.go`) |
| `browser` | Native host, socket server and manifests for the extension |
| `commandline` | Installing `hisar` and Settings → Command-line tool |
| `loginitem`, `background` | Open at login; login item, Dock icon, appearance |
| `biometric`, `systemidle`, `windowplacement`, `updater` | Touch ID and keychain, idle time, window placement, Sparkle |
| `cmd/hisar-browser-host`, `cmd/hisar-update-key` | Helper binaries |

### mobile

`hisarkit` wraps `core/app` for gomobile: only strings, numbers, bools, `[]byte`, errors and interfaces cross into Swift,
so rich values are JSON strings.

### admin

| Path | What it holds |
|---|---|
| `src/sections.ts` | The navigation table: one `Section` (`id`, `title`, `path`, `component`) per screen |
| `src/api.ts` | The session token in `sessionStorage`, `fetch` to the same server's `/api/v1`, sign-in and sign-out |
| `src/bridge/` | `bridge.ts` posts typed messages to `worker.ts`, the Web Worker that runs `hisar-admin-wasm` |
| `src/views/`, `src/sections/<area>/` | The sign-in screen and Overview; one directory per later screen |
| `end-to-end/` | Playwright tests against `hisar-admin-page-fixture`; `perf.spec.ts` checks the size and sign-in time budget |

## Where new code goes

- **A new API endpoint:** the DTO in `core/apitypes`; the rule in a `server/internal/service` method that returns
  `*service.Error` for refusals; a thin handler and route in `server/internal/api`; the client method in `core/apiclient`;
  the session-level logic in `core/session` if apps need it.
- **A new app feature (Mac and iPhone):** methods on `app.Service` in `core/app` for the UI, a package function taking
  `*Service` for anything the UI must not call; then the Wails binding (regenerate) and the `hisarkit` wrapper.
- **A new CLI command:** a function `func(ctx, access backend, flags, args) error` in `cli/cmd/hisar`, a `withBackend`
  row in the command table, and the `backend` methods it needs in both implementations. Standalone-only commands use
  `run`. Update `testdata/usage.golden` with `go test ./cmd/hisar -run TestUsageGolden -update`.
- **A new item type:** the category and its named-field schema in `core/item` (a record struct and its `NamedField` list,
  as `card.go` and `identity.go`: fields with fixed names, so older apps keep them), `Item.OmitsLoginData` if its form has no login data, its rules in `core/app` `applyCategory`, a row in the CLI's `itemTypes` table, a `cliprotocol`
  feature in `GetCategoryFeature`, the importers, then the desktop and iPhone screens.
- **A new CLI ↔ app request:** the type string and result in `core/cliprotocol`, the answer in `core/app/commandline.go`.
- **A new desktop OS integration:** its own `desktop/<name>` package with platform files; wire it in `desktop/main.go`.
- **A new crypto primitive or AAD label:** `core/crypt` only, with a vector in `vectors_test.go`.
- **A new admin dashboard screen:** wire types in `core/apitypes/admin_<area>.go`, the rules in
  `server/internal/service/admin_<area>.go`, and routes in their own function in `server/internal/api/admin_<area>.go`
  that `CreateApp` calls with the `/api/v1/admin` group; each route lists `s.requireSession, requireAdmin` before its
  handler (`TestEveryAdminRouteNeedsAnAdmin` fails for one that does not). Then the screen under
  `admin/src/sections/<area>/` and one row in `admin/src/sections.ts`. The server checks the role; the dashboard only
  decides what to show.

## Conventions

- Idiomatic names: `vaults`, `itemsByVaultID`, `ListVaults`, constructors `NewX`. Each package has a doc comment.
- Platform files use the filename suffix (`_darwin.go`, `_linux.go`, `_windows.go`); cgo files also carry a `cgo` build
  tag with a fallback file for the inverse.
- Errors: low-level packages prefix `pkg:`; domain packages return sentences a user can read. Wrap with `%w` and test
  with `errors.Is`/`errors.As`.
- No mutable exported package variables for policy; use constants or functions.
- Shared test fixtures go in `helpers_test.go`.
- A new request field that an older app would silently ignore needs a `cliprotocol` feature announced in `hello`
  (`cliprotocol.ListFeatures`), checked by `hisar` before it sends the field. Bumping `ProtocolVersion` turns off app
  mode for every command, so keep it for changes that break the framing or existing requests.
- Commit messages: do not add a `Co-Authored-By: Claude …` trailer (or any other AI attribution line).

## Never break these

- **JSON tags** of `apitypes`, `cliprotocol`, `localstate.State` (state.json), export documents and cache rows. Old
  servers, old CLIs and existing files depend on them.
- **HTTP routes and status codes, cliprotocol type strings and error codes, event names, `HISAR_*` variables, CLI
  commands and flags, `hisar-server serve|migrate|invite`.**
- **`mobile/hisarkit`'s exported API**: Swift calls it by name (`listAllVault` and so on), so its names stay as they are.
- **`core/crypt` formats**: `vectors_test.go` must pass unchanged.
- **`main.version` in `cli/cmd/hisar`**: release scripts set it with `-X`.
- **Embedded assets** move with the files that embed them (`server/internal/api/{invitepage,sharelinkpage}/`, legal pages,
  `server/internal/store/migrations/`, and `server/internal/api/adminpage/dist/`, which `admin/vite.config.ts` writes).

## Wails bindings

Wails binds every exported method of each bound type (`app.Service`, `app.BrowserBridge`, `app.UpdateService`,
`app.InviteLinkService`, `agent.Service`, `commandline.Service`, `loginitem.Service` and the Touch ID services), so an
exported method is callable from the UI. After changing a bound type, run
`cd desktop && wails3 generate bindings -clean=true -ts -i` (install with `make install-wails`), commit
`desktop/frontend/bindings`, and update only `desktop/frontend/src/api.ts`, the one file that imports them.

## Commands

```sh
docker run -d --name hisar-pg -e POSTGRES_PASSWORD=hisar -e POSTGRES_DB=hisar -p 127.0.0.1:55432:5432 postgres:18
export HISAR_TEST_DATABASE_URL="postgres://postgres:hisar@127.0.0.1:55432/hisar?sslmode=disable"
make vet test            # every module; -p 1 because database tests share one schema
make build build-cli-all # hisar-server with the admin dashboard (needs Node 24), and hisar; the CLI for every OS
make test-end-to-end     # needs HISAR_DATABASE_URL to an empty database
make test-desktop-frontend
make test-admin          # the admin dashboard's typecheck and unit tests; make build-admin builds it alone
make test-admin-end-to-end # the dashboard in Chromium; needs HISAR_DATABASE_URL and Playwright's Chromium
make build-desktop       # or package-desktop for Hisar.app
make build-ios-framework test-ios
gofmt -l $(git ls-files '*.go')
```

## Tests

- Database tests skip without `HISAR_TEST_DATABASE_URL` and drop the `public` schema of that database.
- Integration tests are named `TestIntegration*` and need a running server; `scripts/test-end-to-end.sh` provides
  `HISAR_TEST_SERVER_URL` and invite links, and runs them by name, so keep those names.
- CI fails on any `--- SKIP` outside `TestIntegration*` and `TestRecordOpenSSHFixture`.
- The desktop root package needs cgo and the built frontend, so `make test` covers every other desktop package.
