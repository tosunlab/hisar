# Hisar codebase map

Hisar is an end-to-end encrypted password, SSH-key and secrets manager: the server stores only ciphertext, and keys are
derived on devices from the master password and a Secret Key. Cryptography, protocols and product decisions are in
[docs/DESIGN.md](docs/DESIGN.md); this file says where code lives and where new code goes.

## Modules

Five Go modules joined by `go.work`, plus six non-Go apps.

| Module | Builds | May import |
|---|---|---|
| `core/` (`github.com/tryhisar/hisar/core`) | The shared library, including the app backend `core/app` | no other module |
| `server/` | `hisar-server` and test fixtures | `core` |
| `cli/` | `hisar` | `core` |
| `desktop/` | The Wails app, `hisar-browser-host`, `hisar-update-key` | `core`; `cli/mcpserver` in tests only |
| `mobile/` | `HisarKit.xcframework` (gomobile) for `mobile/ios` and `HisarKit.aar` for `mobile/android` | `core`; `desktop` in one integration test only |
| `extension/` | The browser extension (TypeScript, MV3) | none |
| `raycast/` | The Raycast extension (TypeScript), installed in Raycast's development mode; it talks to the desktop app on `cli.sock` as a launcher | none |
| `omp/` | The Hisar plugin for omp (TypeScript, Bun), which `omp plugin link` loads: an extension, the `hisar` skill and the `hisar mcp --no-reveal` MCP server; it runs `hisar` and never imports Hisar code | none |
| `admin/` | The admin dashboard (Vue, TypeScript) that `hisar-server` embeds and serves at `/admin` | none; its crypto is `server/cmd/hisar-admin-wasm` |
| `mobile/ios/` | The iOS app (Swift), on top of HisarKit | none |
| `mobile/android/` | The Android app (Kotlin, Jetpack Compose, Gradle) on top of `HisarKit.aar`: `:model`, a JVM module with the app model and its tests, and `:app`, the Android app | none |

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
| `item` | The plaintext item model and its categories (`schema.go`: types kept as named fields; `card.go`, `identity.go`, `apicredential.go`, `license.go`, `server.go`, `database.go`, `address.go`, whose fields identities and cards share), the chosen icon and its presets (`icon.go`), the Android apps a login fills into, stored among its URLs as `androidapp://<package>?sha256=<certificate>` so older apps keep them (`androidapp.go`), `LookupField`, the password generator |
| `itemicon` | Icon images: normalizing uploads and website icons into small PNGs, and fetching website icons from public https sites only |
| `cache` | The offline SQLite copy of the server's encrypted records, and the device's sealed website icons |
| `snapshot` | A vault's published cache snapshot as `session` sees it (`Checkpoint`, the `Staging` that publishes one), without SQLite: `session` must never import `cache`, because the admin dashboard's wasm build imports `session` |
| `localstate` | `state.json` (session state plus embedded device `Settings`) and the Secret Key in the keychain (`ErrNoSecretService` when Linux has no keyring) |
| `app` | The backend of the Mac, iPhone and Android apps, bound to the UI (see below) |
| `app/apptest` | An unlocked `app.Service` on an in-memory API, for tests outside `core/app` |
| `cliprotocol` | The CLI ↔ app protocol on `~/.hisar/cli.sock`: messages, results, framing, `Client` |
| `clisocket` | The app side of that socket: server, approval scopes (terminal session, signed app, connection), coding-agent grants, launcher approvals, peer credentials |
| `agentharness` | The table of coding agent harnesses (omp, Claude Code, Codex) and how each is recognized, the process chain snapshot and `Find`, and `Sessions`, the live agent sessions both sockets share. A new harness is a row in `harnesses` (`harness.go`) with a test from its measured chain |
| `browserprotocol` | The extension ↔ app native-messaging protocol, `Handler`, `Answer`, URL matching |
| `localsocket` | Unix sockets only the user can reach, and the process on the other end |
| `sshagent` | The read-only SSH agent with approvals and session binding |
| `sshkey` | SSH key generation, import and parsing |
| `passkey` | The WebAuthn authenticator |
| `approval` | Phone approval requests and signed answers |
| `sharelink` | Building, sealing and opening share links |
| `secretreference` | `hisar://vault/item/field` references |
| `export`, `importer` | Hisar's export format; importers for other password managers, and the Credential Exchange (CXF) data the iPhone and Android apps receive from them (`credentialexchange.go`, the only source of imported passkeys) |
| `invitelink`, `appversion`, `totp`, `emergencykit` | Invite links, app versions, one-time codes, the Emergency Kit PDF |
| `project` | `GitHubRepository`, the one place that names the GitHub repository (releases, desktop update feed, docs quote it) |

**`core/app`** is one package because every feature shares the unlocked session and its caches under one mutex. Files:
`service.go`, `auth.go`, `lock.go`, `vaults.go`, `items.go`, `clipboard.go`, `errors.go`, `account.go`, `sharing.go`,
`invitations.go`, `sharelink.go`, `browser.go` (extension pairing), `logins.go` (login matching, for pages and Android apps), `androidapps.go` (linking logins to Android apps), `passkey*.go`,
`platformpasskey.go` (iOS), `commandline*.go`, `codingagent.go` and `launcher.go` (the CLI socket handler), `approvals.go`,
`phoneapproval.go` (Mac side), `phoneapproval_answer.go` (phone side), `import.go`, `export.go`, `settings.go`, `favorites.go`, `websiteicons.go` (the sealed website icon cache and its setting), `addresses.go` (addresses and identities for the extension), `cards.go` (cards for the extension),
`browseritems.go` (the extension popup's vault: items, vaults, new items), `browserunlock.go` (the popup's master password unlock),
`organizations.go` (organizations, teams, organization vaults, and delivering and rotating their keys on every sync).
Shared test fixtures are in `helpers_test.go`, the in-memory API in `fakeapi_test.go`. `Config` (`types.go`) has
`BiometricName`, the name Go's sentences give the biometric check: "Touch ID" on the Mac and in iOS hisarkit (Swift
replaces it with the device's Face ID or Touch ID), "biometric" on Android.

### server

| Package | What it holds |
|---|---|
| `cmd/hisar-server` | `serve`, `migrate`, `invite`; `setup.go` reads `HISAR_*` and wires everything |
| `cmd/hisar-admin-wasm` | The admin dashboard's crypto (`js && wasm`, imports only `core`): `signIn`, `reauthenticate` and the first admin's `signUp` on `globalThis.hisar` |
| `internal/api` | HTTP only: routes, sessions, per-IP limits, event stream, invite and share-link pages, the admin dashboard (`adminpage.go` serves the embedded `adminpage/dist`) and the `/api/v1/admin` routes, each behind `requireSession` and `requireAdmin`; handlers parse and call the service |
| `internal/service` | The rules: rights, validation, store calls, audit, events, email. One `Service` type, a file per domain (`vaults`, `auth`, `accounts`, `lifecycle`, `invitations`, `sharelinks`, `approvals`, `preferences`, `organizations`, `setup`, and `admin_server`, `admin_settings`, `admin_health`, `admin_accounts`, `admin_invites`, `admin_audit` for the dashboard; `audit.go` is the table of audit event types and their labels) |
| `internal/store` | Postgres with raw SQL (pgx) and embedded goose migrations |
| `internal/realtime` | The per-account event broker behind `GET /api/v1/events` |
| `internal/ratelimit` | Per-key token buckets |
| `internal/settings` | The instance settings admins edit: defaults, the stored document, `HISAR_*` overrides, `Validate` |
| `internal/mail`, `internal/desktopupdate`, `internal/serverupdate` | SMTP; the Sparkle appcast from GitHub Releases; the newest server release for Overview |
| `cmd/hisar-fake-github`, `cmd/hisar-smtp-sink`, `cmd/hisar-share-link-page-fixture`, `cmd/hisar-admin-page-fixture`, `internal/desktopupdate/fakegithub`, `internal/mail/mailtest` | Fakes and fixtures for the end-to-end and browser tests; `internal/fixturedatabase` keeps the two page fixtures' secrets, one file per database, so a restart on the same database works |

### cli

| Package | What it holds |
|---|---|
| `cmd/hisar` | `main.go` (command table, dispatch, `main.version`), `backend.go` (the `backend` interface), `backend_app.go`, `backend_standalone.go`, and one file per command group |
| `mcpserver` | `hisar mcp` (public: desktop tests drive it) |
| `internal/dotenv` | `.env` parsing, the environment `hisar run --env-file` and `run_with_secrets` build from env files, and layout-preserving rewriting |
| `internal/masking` | Masking secrets in command output |

### desktop

| Package | What it holds |
|---|---|
| root (`main`) | Wails wiring: `main.go`, `adapters.go`, `windows.go`, `events.go`, `tray.go`, `touchid.go`, `helpers*.go` (where the browser host and `hisar` are: in Hisar.app, or at the Linux package's fixed paths) |
| `agent` | SSH agent in the app and the approval queue (`approver*.go`, `presenter.go`, `purpose.go`, `sshconfig.go`) |
| `appinfo` | The app behind a process (`resolver.go`, `chain.go`, `findapp.go`, `bundle.go`) |
| `browser` | Native host, socket server and manifests for the extension |
| `commandline` | Installing `hisar`, Settings → Command-line tool and its approved launchers, and the signed app behind a caller without a terminal (`identity.go`) |
| `harness` | The agents Settings → Coding agents connects: one row per agent in `service.go` (where its tool is installed, its connector), bound as `ListAgents`, `Connect(id)`, `Disconnect(id)`. omp (`omp.go`) links or unlinks the omp plugin in `Hisar.app/Contents/Resources/omp-plugin` (the repository's `omp/` in a development build); Claude Code (`claudecode.go`) and Codex (`codex.go`) get `Contents/Helpers/hisar mcp --no-reveal`, through `claude mcp add --scope user` and through Hisar's own table in Codex's `config.toml` (`codextoml.go`, a table-level editor that keeps every other byte). A `hisar` server Hisar did not add is a conflict and never changed |
| `portable` | Running from an AppImage or a Flatpak: copies the browser host and `hisar` out, with the `hisar-app` launcher |
| `loginitem`, `background` | Open at login; login item, Dock icon, appearance |
| `biometric`, `systemidle`, `windowplacement`, `updater`, `pasteboard` | Touch ID and keychain, idle time (D-Bus on Linux), window placement, Sparkle (`Notifier` announces versions on Linux), the clipboard (copies concealed from clipboard managers) |
| `cmd/hisar-browser-host`, `cmd/hisar-update-key` | Helper binaries |

### mobile

`hisarkit` wraps `core/app` for gomobile: only strings, numbers, bools, `[]byte`, errors and interfaces cross into Swift
and Kotlin, so rich values are JSON strings. `platform_android.go` and `platform_other.go` (`//go:build !android`) hold
what differs between the phones: the platform the device registers with (`android/mobile`, `ios/mobile`), the
`BiometricName` it passes to `core/app`, and two error sentences. `timezone_android.go` links `time/tzdata`. Entry points
only Android calls:

- `SetTimeZone(name)`, a package function, before `New`: Go on Android starts in UTC.
- `SetTrustedCertificates(pem)`, right after `New`: every `AndroidCAStore` certificate, system and user-installed. Go
  then trusts exactly those for the server and website icons.
- `SetBiometricKeyPreparer(preparer)`, right after `New`: `PrepareStoreKey` and `PrepareLoadKey` build the cipher of the
  per-use Keystore key, and the next `Biometric.Authenticate` puts it in the prompt.
- `androidapps.go`: logins linked to Android apps (`ListAllLoginForAndroidApp`, `GetLoginForAndroidApp`,
  `GetOneTimeCodeForAndroidApp`, `AddAndroidApp`, `RemoveAndroidApp`) and `ListAllLoginWebsite` for Digital Asset Links.
- `BuildAndroidAppOrigin`, `BuildPasskeyClientData` and `ValidatePasskeyRelyingPartyID`, package functions for passkeys
  that a native app asks for.

`cmd/hisar-ui-test-mac` plays the user's Mac in the Android phone approval UI test, which `scripts/test-android-ui.sh`
starts it for: it signs up, sends the phone real approval requests, and saves each answer it verified in the Personal vault.

### mobile/android

| Package | What it holds |
|---|---|
| `:model` `com.tryhisar.hisar.model` | Plain JVM, no Android: `Models.kt` (HisarKit's JSON types, decoded with `backendJson`), `VaultBackend` (one method per HisarKit call, with the iPhone app's names and argument order), the logic the screens share (`VaultLogic`, `OrganizationLogic`, …), `theme/ThemePalettes.kt` (written by `make generate-themes`) |
| `:model` `com.tryhisar.hisar.app` | The app model, the port of the iPhone app's `AppModel`. `AppModel` holds the status, the screen, `UnlockedData` (what an unlock loads), the lifecycle, events and notices; one area model per feature holds the rest (`SignInModel`, `LockModel`, `VaultsModel`, `ItemModel`, `ItemEditorModel`, `SharingModel`, `OrganizationsModel`, `SettingsModel`, `ApprovalsModel`, …). An area model takes the `ModelContext`, which `AppModel` implements, and `AppModel` creates it in one line (`val vaults = VaultsModel(this)`) |
| `:model` `com.tryhisar.hisar.autofill` | AutoFill's logic: `FillRequestParser`, `LoginMatcher` (an app's linked logins first, then the sites that vouch for it), `AssetLinks`, `AssetLinksReader` (the password and native-passkey reader, with shared workers, at most 64 queued reads and 128 cached answers), `AutoFillModel`, `SavedLogin` |
| `:model` `src/testFixtures` | `FakeBackend` and `AppModelHarness`, used by the JVM tests of both modules and by the instrumented tests |
| `:app` `com.tryhisar.hisar` | `HisarApplication` (the process's one HisarKit backend, shared by the app and AutoFill), `MainActivity`, `AppViewModel` (owns `AppModel`, reports background and foreground), `ScreenshotSetting` |
| `:app` `backend` | `HisarKitBackend`: `VaultBackend` over the AAR, never on the main thread, Go errors as `BackendError` |
| `:app` `platform` | What Go calls back: `KeystoreSecretKeyStore`, `BiometricKeystore`, `ClipboardAdapter` with its clear alarm, `EventRelay`; and `StateDirectory`, `TrustedCertificates`, `ForegroundActivity` |
| `:app` `ui/<area>` | Compose screens per area (`signin`, `lock`, `root`, `vaults`, `search`, `item`, `sharing`, `organizations`, `approvals`, `settings`), with `components` and `theme` (`HisarTheme` maps `ThemePalettes` onto Material 3, never dynamic color) |
| `:app` `autofill` | `HisarAutofillService`, `AutofillActivity` and its screens, `Datasets`, `HttpAssetLinksVerifier`, `InstalledApps`, `SavedLoginInbox`, and the AutoFill sections of Settings |
| `:app` `src/debug`, `src/androidTest` | `PreviewActivity` (debug-only AutoFill screens for UI tests); the instrumented tests, with `HisarUITestCase` (the base of the tests against a real `hisar-server`) and `HisarTestApplication`, which installs a `FakeBackend` for tests without a server |

### admin

| Path | What it holds |
|---|---|
| `src/sections.ts` | The navigation table: one `Section` (`id`, `title`, `path`, `component`) per screen |
| `src/navigation.ts` | The current path and `navigate`, for pages below a section such as `/admin/accounts/<id>` |
| `src/api.ts` | The session token in `sessionStorage`, `fetch` to the same server's `/api/v1`, sign-in and sign-out |
| `src/bridge/` | `bridge.ts` posts typed messages to `worker.ts`, the Web Worker that runs `hisar-admin-wasm` |
| `src/views/`, `src/sections/<area>/` | The sign-in screen and `SetupView.vue`, the first-run setup at `/admin/setup` with its steps in `views/setup/`; one directory per section (`overview`, `accounts`, `invites`, `settings`, `audit`) |
| `end-to-end/` | Playwright tests against `hisar-admin-page-fixture`; `perf.spec.ts` checks the size and sign-in time budget |

### omp

| Path | What it holds |
|---|---|
| `package.json` | The omp manifest (`omp.extensions`) and the `@oh-my-pi/pi-coding-agent` version the plugin typechecks against, the one Tern ships |
| `mcp.json` | The `hisar` MCP server omp starts for the plugin: `bin/run-hisar mcp --no-reveal`, with no client timeout so an approval can take its time. Its instructions reach the system prompt, so the extension adds no guidance of its own |
| `bin/run-hisar` | Runs the `hisar` beside the plugin in `Hisar.app` (`Contents/Resources/omp-plugin`), else the first on `PATH`, else the one in `/Applications` or `~/Applications`; omp roots a `./` command at the plugin |
| `src/index.ts` | The extension: `session_start` finds `hisar` and probes its state, `before_agent_start` names `hisar`'s full path when the shell's `PATH` lacks it, `tool_call` runs the bash guard, `tool_result` refreshes the state after a `hisar` command, and `/hisar` |
| `src/bash.ts` | `classifyBashCommand`, the pure guard that blocks `hisar read`, `get`, `export` and `run --no-masking` in a `bash` call; it catches accidents, and the app's approvals are the enforcement |
| `src/status.ts` | The probe, `HISAR_NO_LAUNCH=1 hisar status --json`, and the status line texts |
| `src/locate.ts` | Finding `hisar` for the shell (`PATH`, then `Hisar.app` in `/Applications` and `~/Applications`) and the path note |
| `skills/hisar/SKILL.md` | The skill omp lists for the agent: `search_items`, `run_with_secrets` (`command` or `shell`, `env`, `envFiles`, `stdin`), `hisar run` in `bash` for live output, input or the shell's environment, approval timeouts |

## Where new code goes

- **A new API endpoint:** the DTO in `core/apitypes`; the rule in a `server/internal/service` method that returns
  `*service.Error` for refusals; a thin handler and route in `server/internal/api`; the client method in `core/apiclient`;
  the session-level logic in `core/session` if apps need it.
- **A new app feature (Mac, iPhone and Android):** methods on `app.Service` in `core/app` for the UI, a package function
  taking `*Service` for anything the UI must not call; then the Wails binding (regenerate) and the `hisarkit` wrapper.
  On Android, a new HisarKit call goes into `VaultBackend`, `HisarKitBackend` and `FakeBackend`.
- **A new Android screen or area:** the area model in `:model` `com.tryhisar.hisar.app` with its JVM tests on
  `FakeBackend` (`AppModelHarness`), one line in `AppModel` that creates it, and the screen in `:app` `ui/<area>`, whose
  test tags are the Swift `accessibilityIdentifier`s. Port the iPhone test under the same name (or add a row to
  `IosTestParityTest`'s `renamed` table) and delete its line from the expected-missing list in the same commit.
- **A new CLI command:** a function `func(ctx, access backend, flags, args) error` in `cli/cmd/hisar`, a `withBackend`
  row in the command table, and the `backend` methods it needs in both implementations. Standalone-only commands use
  `run`. Update `testdata/usage.golden` with `go test ./cmd/hisar -run TestUsageGolden -update`.
- **A new item type:** the category and its named-field schema in `core/item` (a record struct and its `NamedField` list,
  as `card.go` and `identity.go`: fields with fixed names, so older apps keep them), `Item.OmitsLoginData` if its form has no login data, its rules in `core/app` `applyCategory`, a row in the CLI's `itemTypes` table, a `cliprotocol`
  feature in `GetCategoryFeature`, the importers, then the desktop, iPhone and Android screens.
- **A new CLI ↔ app request:** the type string and result in `core/cliprotocol`, the answer in `core/app/commandline.go`.
- **A new coding agent to connect from Settings:** a row in `desktop/harness/service.go` with a connector (`getState`,
  `connect`, `disconnect`, each converging), and its card copy in `CodingAgentConnections.vue`.
- **A new desktop OS integration:** its own `desktop/<name>` package with platform files; wire it in `desktop/main.go`.
- **A new crypto primitive or AAD label:** `core/crypt` only, with a vector in `vectors_test.go`.
- **A new admin dashboard screen:** wire types in `core/apitypes/admin_<area>.go`, the rules in
  `server/internal/service/admin_<area>.go`, and routes in their own function in `server/internal/api/admin_<area>.go`
  that `CreateApp` calls with the `/api/v1/admin` group; each route lists `s.requireSession, requireAdmin` before its
  handler (`TestEveryAdminRouteNeedsAnAdmin` fails for one that does not). Then the screen under
  `admin/src/sections/<area>/` and one row in `admin/src/sections.ts`. The server checks the role; the dashboard only
  decides what to show.
- **A new audit event:** a constant and its row (stored name, group, dashboard label) in `auditTypes` in
  `server/internal/service/audit.go`; `recordAudit` takes only those constants.

## Conventions

- Idiomatic names: `vaults`, `itemsByVaultID`, `ListVaults`, constructors `NewX`. Each package has a doc comment.
- Platform files use the filename suffix (`_darwin.go`, `_linux.go`, `_windows.go`); cgo files also carry a `cgo` build
  tag with a fallback file for the inverse. `mobile/hisarkit` pairs `_android.go` with a `//go:build !android` file.
- Android screens follow the iPhone app's layout, components and copy, with Hisar's palettes from
  `assets/themes/themes.json`; Material 3 is only a restyled component layer, never dynamic color.
- Errors: low-level packages prefix `pkg:`; domain packages return sentences a user can read. Wrap with `%w` and test
  with `errors.Is`/`errors.As`.
- No mutable exported package variables for policy; use constants or functions.
- Shared test fixtures go in `helpers_test.go`.
- A new request field that an older app would silently ignore needs a `cliprotocol` feature announced in `hello`
  (`cliprotocol.ListFeatures`), checked by `hisar` before it sends the field. Bumping `ProtocolVersion` turns off app
  mode for every command, so keep it for changes that break the framing or existing requests.
- Commit messages: do not add a `Co-Authored-By: Claude …` trailer (or any other AI attribution line).

## Never break these

- **JSON tags** of `apitypes`, `cliprotocol`, `localstate.State` (state.json), export documents, cache rows and
  `hisar status --json`. Old servers, old CLIs, the omp plugin and existing files depend on them.
- **HTTP routes and status codes, cliprotocol type strings and error codes, event names, `HISAR_*` variables, CLI
  commands and flags, `hisar-server serve|migrate|invite`.**
- **`mobile/hisarkit`'s exported API**: Swift and Kotlin call it by name (`listAllVault`, `Hisarkit.new_` and so on),
  so its names stay as they are.
- **The Android app link format** `androidapp://<package>?sha256=<64 lower-case hex>` among a login's URLs
  (`core/item/androidapp.go`): it is stored in synced items, and released apps keep it because it is a URL.
- **`core/crypt` formats**: `vectors_test.go` must pass unchanged.
- **`main.version` in `cli/cmd/hisar`**: release scripts set it with `-X`.
- **Embedded assets** move with the files that embed them (`server/internal/api/{invitepage,sharelinkpage}/`, legal pages,
  `server/internal/store/migrations/`, and `server/internal/api/adminpage/dist/`, which `admin/vite.config.ts` writes).

## Wails bindings

Wails binds every exported method of each bound type (`app.Service`, `app.BrowserBridge`, `app.UpdateService`,
`app.InviteLinkService`, `agent.Service`, `commandline.Service`, `harness.Service`, `loginitem.Service` and the Touch ID services), so an
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
make test-raycast        # the Raycast extension's typecheck, tests, lint and build; it never installs into Raycast
make test-omp            # the omp plugin's typecheck, tests and format check (needs Bun)
make test-admin          # the admin dashboard's typecheck and unit tests; make build-admin builds it alone
make test-admin-end-to-end # the dashboard in Chromium; needs HISAR_DATABASE_URL and Playwright's Chromium
make build-desktop       # or package-desktop for Hisar.app; package-desktop-linux VERSION=… for the .deb, .rpm and AppImage (on Linux); package-desktop-flatpak VERSION=… for the Flatpak; scripts/test-desktop-linux-package.sh installs one and checks the app starts
make build-ios-framework test-ios
make install-android-sdk # the pinned SDK packages, NDK 30 and the emulators hisar-api29, hisar-api34, hisar-api37
make build-android-aar build-android test-android # HisarKit.aar; the APKs, the test APK and lint; the JVM tests
make test-android-ui     # instrumented tests on the one emulator or ANDROID_SERIAL; needs HISAR_DATABASE_URL to an empty database
gofmt -l $(git ls-files '*.go')
```

Make selects the exact `go.work` toolchain even if `GOTOOLCHAIN` is set in your environment or Make arguments.
The first use needs network access unless that release is already cached.

The Android targets need `ANDROID_HOME` (the SDK, with the command-line tools in `cmdline-tools/latest`),
`ANDROID_NDK_HOME` (`$ANDROID_HOME/ndk/30.0.16248370`) and JDK 21 for the Gradle daemon
(`gradle/gradle-daemon-jvm.properties`). `make generate-themes` writes `ThemePalettes.kt`, with the Swift palettes and
the desktop and admin `themes.css`, from `assets/themes/themes.json`.
## Releases

One run of Actions → Release per release (`gh workflow run release.yml -f app=<server|cli|desktop|extension> -f type=<patch|minor|major|custom> [-f version=X.Y.Z]`):
it plans the version (`scripts/plan-release.sh`), requires full CI on the exact release SHA and macOS tests for desktop,
pushes the `<app>-X.Y.Z` tag, and builds through `release-<app>.yml`. It attests every delivered file and the server image
digest, creates the GitHub prerelease, then verifies that evidence before approval-gated publication in `publish-<app>`.
Dry runs only plan. Custom builds require the same tests and cannot publish. `runTests=false` is not supported.
What an app is built from lives in `scripts/app-paths.sh`, shared with
`scripts/detect-app-changes.sh`; a new build input of an app goes there. A release's install text is
`.github/release-notes/<app>.md`, and `scripts/check-release-files.sh` lists the files each release must have.

Local Make builds, packaging scripts, mobile bindings and direct Wails tasks select the exact `toolchain` release in
`go.work` through `scripts/go-toolchain.sh`, even when the installed compiler is newer or mobile sets `GOWORK=off`.
Docker and Flatpak pin the same release independently and disable automatic toolchain switching.
CI runs `bash scripts/check-go-toolchain.sh` to reject drift from `go.work`.
The CI changes job also runs `bash scripts/test-app-paths.sh` to check that a Docker-context-only change selects server
releases and admin/image CI.
Release verification and the build-image digest update procedure are in docs/RELEASING.md.
`python3 scripts/check-build-images.py` rejects unpinned build images. `--registry` verifies pinned registry manifest bytes
and both Linux architectures without pulling layers. `python3 scripts/test-release-evidence.py` exercises fail-closed
test evidence, tag movement, artifact substitution, and repository/workflow-bound attestation verification.

Sparkle consumers run `scripts/download-sparkle.sh` before linking, copying the framework or signing an update.
The downloader checks the archive SHA-256 on every use and compares extracted cache bytes, permissions and symlinks
with a fresh extraction. Matching caches stay in place for concurrent universal builds. A failed Python comparison
stops the build without replacing the cache. The macOS desktop job runs `bash scripts/test-download-sparkle.sh` to
check cache repair, concurrent reuse and Python failures.

## Tests

- Database tests skip without `HISAR_TEST_DATABASE_URL` and drop the `public` schema of that database.
- Integration tests are named `TestIntegration*` and need a running server; `scripts/test-end-to-end.sh` provides
  `HISAR_TEST_SERVER_URL` and invite links, and runs them by name, so keep those names.
- CI fails on any `--- SKIP` outside `TestIntegration*` and `TestRecordOpenSSHFixture`.
- The desktop root package needs cgo and the built frontend, so `make test` covers every other desktop package.
- `IosTestParityTest` (`make test-android`) reads every `func test…` in `mobile/ios/Tests/Unit` and `Tests/UI` and every
  `fun test…` in `mobile/android/*/src/test` and `src/androidTest`. Each iPhone test needs an Android test of the same
  name, a row in its `renamed` table, or a reason in `notApplicable`. `model/src/test/resources/ios-test-parity-expected-missing.txt`
  lists the iPhone tests not ported yet and only shrinks: a ported test fails the check until its line is deleted.
- `make test-android-ui` (`scripts/test-android-ui.sh`) starts `hisar-server`, the SMTP sink and the sharing partner
  through `scripts/ui-test-partner.sh`, which `make test-ios-ui` shares, plus `hisar-ui-test-mac` and
  `answer_as_other_device`, the `hisar` CLI as the user's other device in `CrossDeviceSyncUITests`. Each test reads its
  account as `HISAR_UI_TEST_<NAME>_INVITE_LINK` and `_EMAIL`, the iPhone's names; invites are single-use, so a new
  server test adds a `ui_test_add_account` line. The test orchestrator runs each test in a new process with the app's
  data cleared. `ANDROID_UI_TESTS=<class or test names>` or `ANDROID_UI_TESTS=smoke` runs a subset.
- CI runs the iPhone UI tests in three jobs and the Android instrumented tests in two, split by the test classes' measured
  run times in `scripts/ui-test-parts.sh` (`IOS_UI_TEST_PART`, `ANDROID_UI_TEST_PART`); a class missing from its table runs
  in the last part.
