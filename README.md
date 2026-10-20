<p align="center">
  <a href="https://tryhisar.com">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/tosunlab/hisar/main/assets/logo/png/hisar-wordmark-on-dark-504.png">
      <img alt="Hisar" width="252" src="https://raw.githubusercontent.com/tosunlab/hisar/main/assets/logo/png/hisar-wordmark-on-light-504.png">
    </picture>
  </a>
</p>

<p align="center"><strong>End-to-end encrypted password, SSH-key and secrets manager for developers.</strong></p>

<p align="center">
  <a href="https://tryhisar.com">Website</a> ·
  <a href="https://github.com/tosunlab/hisar/releases">Releases</a> ·
  <a href="docs/DESIGN.md">Design</a> ·
  <a href="deploy/README.md">Self-hosting</a> ·
  <a href="https://tryhisar.com/security">Security</a>
</p>

<p align="center">
  <a href="https://github.com/tosunlab/hisar/releases?q=desktop&expanded=true"><img src="https://img.shields.io/github/v/release/tosunlab/hisar?filter=desktop-*&logo=apple&logoColor=white" alt="Latest desktop app release"></a>
  <a href="https://github.com/tosunlab/hisar/releases?q=server&expanded=true"><img src="https://img.shields.io/github/v/release/tosunlab/hisar?filter=server-*&logo=docker&logoColor=white" alt="Latest server release"></a>
  <a href="https://github.com/tosunlab/hisar/releases?q=extension&expanded=true"><img src="https://img.shields.io/github/v/release/tosunlab/hisar?filter=extension-*&logo=googlechrome&logoColor=white" alt="Latest browser extension release"></a>
  <a href="core/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/tosunlab/hisar?filename=core/go.mod&logo=go&logoColor=white" alt="Go version"></a>
</p>

---

Hisar keeps your passwords, passkeys, SSH keys, one-time codes and API secrets in vaults that only your devices can read. The server, hosted at `api.tryhisar.com` or [run by you](deploy/README.md), stores encrypted vault payloads alongside plaintext account, sharing and operational metadata: every key is derived on your devices from your master password and a **Secret Key**, so neither a copy of the database nor a network attacker can read your items. A compromised live server has different limits, especially through the admin and share-link pages it serves ([threat model and metadata](docs/DESIGN.md#2-threat-model)). The cryptography, the sync protocol and every product decision are written down in [docs/DESIGN.md](docs/DESIGN.md).

Hisar is one Go implementation of the crypto shared by a Mac and Linux app, a browser extension, an iPhone app, an Android app and a command-line tool. It is built for people who live in a terminal: the app is an SSH agent, `hisar run` hands secrets to commands and masks them in their output, and `hisar mcp` lets coding agents pass secrets to commands you approve.

## Features

- **Encrypted vault payloads.** Hisar's clients never send the master password to the server (OPAQUE), and the 128-bit Secret Key, which a stolen database does not contain, adds 128 bits of resistance to offline guessing against it. Titles, URLs and vault names are encrypted too; account, sharing and audit metadata is not ([threat model](docs/DESIGN.md#2-threat-model)).
- **Offline first.** The native apps keep an encrypted copy of the records they have synced and work without a network; offline reads and exports use that copy, which can be stale or incomplete. Sync rejects a lower sequence number from the server, but does not detect a server that withholds writes or replays old records.
- **Every kind of secret.** Logins with one-time codes and passkeys, SSH keys, secure notes, credit cards, identities, addresses, API credentials, software licenses, servers and databases.
- **SSH agent with Touch ID.** Private keys never sit in `~/.ssh`: `ssh`, `git` and commit signing ask Hisar, which approves each use with Touch ID, the master password or your phone. For SSH commit signing, the app shows the `git config` commands ready to copy; it never changes your Git config itself.
- **Browser extension.** Fills and saves logins, passkeys, cards, identities and addresses in Chromium browsers. A login fills only on pages whose host shares its registrable domain (`accounts.google.com` for a login saved for `google.com`), or only on the exact host if you turn that on. The extension holds no keys: it talks to the desktop app over native messaging.
- **Phones.** The iPhone app with an AutoFill extension and the Android app with autofill into apps, built on the same Go backend. The phone can also approve requests that arrive at your Mac.
- **Sharing.** Share a vault with anyone by email, send a login to someone without an account through an end-to-end encrypted share link, and group people into organizations and teams with organization vaults.
- **Secrets for commands.** Keep `hisar://vault/item/field` references in your `.env` instead of values. `hisar run --env-file .env -- <command>` resolves them in one approval and masks the secret values it recognizes in what the command prints.
- **Coding agents.** `hisar mcp` is an MCP server that shows an agent only the vaults you allow and asks you before every command it runs with secrets. Claude Code, Codex and omp connect with one click in Settings.
- **In and out.** Import from 1Password, Bitwarden, LastPass and CSV, and passkeys through Credential Exchange on the phone. Export everything as encrypted or plain JSON and import it back.
- **Recovery you control.** A forgotten master password cannot be recovered by anyone, Hisar included. The Emergency Kit PDF holds your email, server and Secret Key, so that with your master password you can sign in on any new device.
- **Self-hosted in one binary.** `hisar-server` and Postgres, with an admin dashboard at `/admin` whose crypto runs in the browser as WebAssembly, so its API offers no way to read a vault. The server serves that code, so an admin who signs in there trusts the server with their factors.

## Install

Every download is on [GitHub Releases](https://github.com/tosunlab/hisar/releases), one release per app. The CLI archives, the Linux packages and the Flatpak come with `SHA256SUMS` files to check them against; the Mac DMG is signed and notarized instead.

| Platform | How to get it |
|---|---|
| **macOS** | Open `Hisar-<version>.dmg` from the latest `desktop-<version>` release and drag Hisar into Applications. The app updates itself (Sparkle) and bundles `hisar`: Settings → Developer tools → **Install command-line tool**. |
| **Linux** (amd64, arm64) | `sudo apt install ./Hisar-<version>-linux-<arch>.deb` on Ubuntu 24.04+ or Debian 13+, `sudo dnf install ./Hisar-<version>-linux-<arch>.rpm` on Fedora 40+. Elsewhere, the `.AppImage` needs the system's GTK 4 and WebKitGTK 6.0, or the `.flatpak` brings them along. The packages include `hisar`. Details [below](#desktop-app-on-linux). |
| **Browser extension** | Unzip `hisar-extension-<version>.zip` from the latest `extension-<version>` release, open `chrome://extensions`, turn on Developer mode and choose **Load unpacked**. It needs the desktop app. |
| **iPhone and Android** | No store builds yet: build the apps from [`mobile/`](mobile/README.md) and install them on your own phone. |
| **Command-line tool** (Windows, servers, without the app) | The standalone `hisar-<version>-<os>-<arch>` archive from a `cli-<version>` release; see [cli/README.md](cli/README.md). |
| **Server** | `ghcr.io/tosunlab/hisar-server:<version>` for linux/amd64 and linux/arm64, run with Docker Compose behind your own TLS: [deploy/README.md](deploy/README.md). |

Sign-up is invite-only: an existing user invites you by email, or you run your own server and invite yourself from its admin dashboard.

### Desktop app on Linux

The `.deb` and `.rpm` are built for GTK 4 and WebKitGTK 6.0 (Ubuntu 24.04+, Debian 13+, Fedora 40+) and install the app at `/usr/bin/Hisar`, `hisar` at `/usr/bin/hisar` and the browser extension's host. On other distributions, run `Hisar-<version>-linux-<arch>.AppImage` with the system's GTK 4 and WebKitGTK 6.0 (for example `sudo pacman -S gtk4 webkitgtk-6.0` on Arch; without them it says which packages to install), or install the Flatpak, which brings them with the GNOME runtime: `flatpak install --user Hisar-<version>-linux-<arch>.flatpak`. Both copy `hisar` to `~/.local/share/hisar/bin`; Settings → Command-line tool links it onto your `PATH`.

The Secret Key goes into the desktop's keyring, so a Secret Service has to run (GNOME Keyring or KWallet); without one, set `HISAR_SECRET_KEY`. What works as on the Mac and what differs (no Touch ID, update notices instead of installing updates, no app names in approvals): [docs/DESIGN.md §9.1.3](docs/DESIGN.md).

## Quick start

With the Hisar app running on your Mac or Linux desktop, `hisar` works through it and asks you to approve each terminal (with Touch ID on the Mac, with the master password or your phone on Linux). On Windows, servers and machines without the app, `hisar login` signs in on its own.

```sh
hisar status                                      # through the app, or standalone?
hisar vaults                                      # your vaults
hisar get "GitHub"                                # an item, by title
hisar read hisar://Personal/GitHub/otp            # one field on stdout, here the current one-time code
```

Keep secret references, not values, in your `.env`, and let `hisar run` resolve them:

```sh
# .env — safe to commit; it holds no secrets
DATABASE_PASSWORD=hisar://Work/Postgres%20staging/password
GITHUB_TOKEN=hisar://Personal/GitHub/API%20token
```

```sh
hisar run --env-file .env -- npm test             # one approval; printed secrets become <concealed by hisar>
```

Let a coding agent run commands with your secrets: in Settings → Developer tools → Coding agents, turn on **Available to agents** for a vault and click **Connect** next to Claude Code, Codex or omp. Any other MCP client gets a stdio server:

```sh
hisar mcp --no-reveal                             # --no-reveal: no tool returns a secret's value directly
```

Everything `hisar` can do, the reference format, masking and the MCP tools: [cli/README.md](cli/README.md).

## How it works

- **Keys.** Argon2id derives the account unlock key from the master password and the Secret Key; HKDF and AEAD seal a keyset, per-vault keys and each item, with authenticated labels so a record cannot be moved or swapped. [§3](docs/DESIGN.md#3-cryptographic-primitives), [§4](docs/DESIGN.md#4-key-hierarchy).
- **Sign-in.** OPAQUE proves the master password without sending it. A new device needs the Secret Key and the master password; a known device unlocks locally, with Touch ID on the Mac and Face ID on the iPhone. [§5](docs/DESIGN.md#5-accounts-and-devices).
- **Sharing.** A vault key is wrapped to each member's public key, verified by fingerprint. Removing someone rotates the vault. [§5.8](docs/DESIGN.md#58-sharing-a-vault-by-email), [§5.11](docs/DESIGN.md#511-organizations-teams-and-organization-vaults).
- **Sync.** Clients pull and push encrypted records; conflicts resolve on the client, and a lower sequence number from the server is rejected as a rollback; withholding and replay are not detected. [§7](docs/DESIGN.md#7-sync-protocol).
- **What the server sees.** Emails and account names, who can reach which vault, organization and team names and rosters, devices, counts, sizes, timestamps and IPs. Never item contents, titles, URLs or vault names, unless a compromised server changes the pages it serves. [§2.3](docs/DESIGN.md#23-accepted-metadata-leakage).

## Repository layout

| Path | What |
|---|---|
| [`core/`](core) | Shared Go library: crypto, item model, API client, sessions and sync, and `core/app`, the backend of the Mac, iPhone and Android apps |
| [`server/`](server) | `hisar-server`, the sync server (Go + Postgres) |
| [`admin/`](admin) | The admin dashboard (Vue) that `hisar-server` embeds at `/admin` |
| [`cli/`](cli) | `hisar`, the command-line tool and MCP server, see [cli/README.md](cli/README.md) |
| [`desktop/`](desktop) | The Mac and Linux app (Wails v3 + Vue), the SSH agent and the extension's native host, see [desktop/README.md](desktop/README.md) |
| [`extension/`](extension) | The browser extension (TypeScript, MV3), see [extension/README.md](extension/README.md) |
| [`mobile/`](mobile) | The iPhone (Swift) and Android (Kotlin) apps on the gomobile-bound core, see [mobile/README.md](mobile/README.md) |
| [`raycast/`](raycast) | The Raycast extension |
| [`omp/`](omp) | The Hisar plugin for omp, the coding agent that also runs in Tern |
| [`deploy/`](deploy) | Dockerfile and Docker Compose for the server, see [deploy/README.md](deploy/README.md) |
| [`docs/`](docs) | [DESIGN.md](docs/DESIGN.md), the source of truth for the security model, and [RELEASING.md](docs/RELEASING.md) |

Where each package is and where new code goes: [CLAUDE.md](CLAUDE.md).

## Development

Requires Go 1.26+, Docker (for Postgres) and Node 24 (`make build` runs `npm ci` and Vite to build the admin dashboard into `hisar-server`).

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

The desktop app builds with `make build-desktop` ([desktop/README.md](desktop/README.md)), the extension with `make build-extension`, the iPhone app with Xcode on a Mac and the Android app with the Android SDK and JDK 21 (`make build-android-aar build-android test-android`, see [mobile/README.md](mobile/README.md)). The other targets are in the [Makefile](Makefile).

## Contributing

Bug reports and feature requests are welcome as issues, and changes as pull requests on `main`. [CONTRIBUTING.md](CONTRIBUTING.md) has what a pull request needs to pass, the commit message format and the code conventions. A change to the security model starts in [docs/DESIGN.md](docs/DESIGN.md): change the document before the code. How CI and releases work: [docs/RELEASING.md](docs/RELEASING.md).

## Maintainers

<table>
  <tr>
    <td align="center"><a href="https://github.com/alperb"><img src="https://github.com/alperb.png?size=64" width="48" height="48" alt="Alper Berber"><br>Alper Berber</a></td>
    <td align="center"><a href="https://github.com/emrebdr"><img src="https://github.com/emrebdr.png?size=64" width="48" height="48" alt="Edip Emre Bodur"><br>Edip Emre Bodur</a></td>
    <td align="center"><a href="https://github.com/bilgehanertan"><img src="https://github.com/bilgehanertan.png?size=64" width="48" height="48" alt="M. Bilgehan Ertan"><br>M. Bilgehan Ertan</a></td>
    <td align="center"><a href="https://github.com/emirhanboge"><img src="https://github.com/emirhanboge.png?size=64" width="48" height="48" alt="Emirhan Böge"><br>Emirhan Böge</a></td>
  </tr>
</table>

## Security

Found a vulnerability? Use [GitHub's private vulnerability reporting](https://github.com/tosunlab/hisar/security/advisories/new) or email [support@tryhisar.com](mailto:support@tryhisar.com?subject=Security%20report), not a public issue. We reply within two working days and credit reporters who want it. The threat model and what the server can and cannot learn are in [docs/DESIGN.md §2](docs/DESIGN.md#2-threat-model); the primitives and their frozen test vectors are in `core/crypt`.
