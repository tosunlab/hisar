# Hisar — Design

Status: **Draft, Phase 0**. This document is the source of truth for the security model, data model and sync protocol. Change it before changing the code.

## 1. Goals and non-goals

**Goals**

- End-to-end encrypted password manager for one household and close friends (≈ 2–20 users).
- Self-hosted: one server binary + Postgres on our own server.
- Zero-knowledge server: the server never sees the master password, the Secret Key, any decryption key or any plaintext item data.
- Offline-first clients: every client keeps a full encrypted copy and works with no network.
- Shared vaults between family members, plus an admin-assisted recovery path.
- Clients: desktop (first), browser extension, mobile, CLI.
- One implementation of the crypto, written in Go, shared by all clients.

**Non-goals (for now)**

- Multi-tenant SaaS, billing, SSO/SCIM, enterprise policy.
- Protecting an unlocked device from local malware (out of scope for every password manager).
- Hiding metadata from the server completely (see §2.3).

## 2. Threat model

### 2.1 Assets

Item contents (passwords, notes, TOTP seeds, passkeys, attachments), item metadata (titles, URLs), vault names, and the users' master passwords and Secret Keys.

### 2.2 Adversaries and expected outcome

| Adversary | Capability | Expected outcome |
|---|---|---|
| Network attacker | Reads/modifies traffic | TLS + OPAQUE: learns nothing, cannot impersonate either side |
| Server breach (DB dump) | Full copy of DB and backups | Ciphertext only. Offline guessing impossible without the Secret Key (128-bit) |
| Malicious/compromised server (live) | Arbitrary responses | Cannot read data or forge items (AEAD + AAD). Can withhold or roll back data (detected, §7.5). Can substitute public keys during sharing (mitigated by fingerprint verification, §5.6) |
| Stolen locked device | Disk image | Needs master password; Secret Key on disk alone is not enough |
| Stolen unlocked device / local malware | Memory, keystrokes | Out of scope. Minimised by auto-lock and short key lifetime |
| Phishing site | Lures user to wrong origin | Extension autofills only on exact origin match (§9.2) |
| Other family member | Legit account | Sees only vaults shared with them |

### 2.3 Accepted metadata leakage

The server sees: account emails, which users can access which vaults, number of items per vault, ciphertext sizes, timestamps, device list, IPs. Item titles, URLs and vault names are encrypted.

## 3. Cryptographic primitives

All from the Go standard library or `golang.org/x/crypto`. No custom primitives.

| Purpose | Primitive | Go package |
|---|---|---|
| Password stretching | Argon2id | `golang.org/x/crypto/argon2` |
| Key derivation | HKDF-SHA256 | `crypto/hkdf` |
| Symmetric AEAD | XChaCha20-Poly1305 (24-byte random nonce) | `golang.org/x/crypto/chacha20poly1305` |
| Public-key wrapping | HPKE base mode, DHKEM(X25519, HKDF-SHA256), HKDF-SHA256, ChaCha20-Poly1305 (RFC 9180) | `crypto/hpke` |
| Signatures (key fingerprints, invites) | Ed25519 | `crypto/ed25519` |
| Authentication | OPAQUE (RFC 9807), ristretto255 suite | `github.com/bytemare/opaque` |
| Randomness | OS CSPRNG | `crypto/rand` |

Every ciphertext is versioned (`v1`) so primitives can be rotated later.

## 4. Key hierarchy

```
master password ──Argon2id──► k_pw ─┐
                                    ├─HKDF──► AUK  (Account Unlock Key) ──decrypts──► keyset
Secret Key ───────HKDF────────► k_sk ┘   └──HKDF──► auth secret ──► OPAQUE login

keyset = { sym key K_user, X25519 private key, Ed25519 signing key }
   └─ X25519 public key ◄── HPKE-wraps ── vault key K_vault (one per vault, per member)
                                              └─ XChaCha20-Poly1305 ── items, attachments keys
```

### 4.1 Secret Key

- 128 bits from `crypto/rand`, generated on the user's first device. Never sent to the server.
- Display format: `H1-AAAAAA-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX` — version, 6-char account ID, 26 Crockford base32 chars.
- Stored on each device in the OS keychain (macOS Keychain, Windows Credential Manager, libsecret, iOS Keychain, Android Keystore).
- Printed on the Emergency Kit (PDF generated client-side) together with the server URL and email.

### 4.2 Deriving AUK and the auth secret

```
pw      = NFKD(master password), UTF-8
salt    = random 16 bytes, per account, stored on server (public)
k_pw    = Argon2id(pw, salt, t=3, m=64 MiB, p=4, len=32)
k_sk    = HKDF-SHA256(ikm=SecretKey, salt=accountID, info="hisar/v1/secret-key", len=32)
ikm     = k_pw || k_sk
AUK     = HKDF-SHA256(ikm, salt=nil, info="hisar/v1/auk",  len=32)
authKey = HKDF-SHA256(ikm, salt=nil, info="hisar/v1/auth", len=32)
```

- Argon2id parameters are stored per account so they can be raised later. 64 MiB is chosen to fit inside the iOS AutoFill extension memory limit. Benchmark on the slowest target device before freezing.
- `authKey` is the OPAQUE password input. OPAQUE's internal key-stretching function is set to identity because stretching already happened. A server that steals the OPAQUE record still needs the Secret Key to guess the password.
- AUK and `authKey` are independent HKDF outputs: the server learns nothing about AUK from the login.

### 4.3 Keyset

Generated at signup on the first device:

- `K_user`: 32 random bytes, encrypts the user's private data that is not in a vault (settings, device list labels).
- X25519 keypair: receives vault keys.
- Ed25519 keypair: signs the X25519 public key and invites, so others can verify it (§5.6).

The private half is serialised and encrypted:

```
encKeyset = XChaCha20-Poly1305(key=AUK, nonce=random24, aad="hisar/v1/keyset"||accountID, plaintext=keyset)
```

The server stores `encKeyset`, the X25519 public key, the Ed25519 public key and the signature over the X25519 public key.

### 4.4 Vault keys

- Each vault has a random 32-byte `K_vault`.
- For every member, the server stores `HPKE.Seal(memberX25519Pub, info="hisar/v1/vault-key"||vaultID, aad=memberAccountID, K_vault)`.
- The recovery group (§6.6) gets the same wrapping for every vault that opts into recovery.
- Removing a member rotates `K_vault`: new key, all items re-encrypted by an admin client, new wrappings for the remaining members. Until rotation, the removed member could still decrypt copies they already downloaded. That is unavoidable.

### 4.5 Items

An item has two encrypted parts so lists and search don't decrypt every secret:

- **overview**: title, URLs, tags, icon hint, subtitle (username).
- **details**: fields, passwords, notes, TOTP seeds, passkeys, history of field values.

```
aad      = "hisar/v1/item" || vaultID || itemID || part      (part ∈ {"overview","details"})
ct_part  = XChaCha20-Poly1305(key=K_vault, nonce=random24, aad, plaintext=JSON(part))
```

Binding vault and item IDs in the AAD stops a malicious server from moving an item into another vault or swapping overview/details between items.

### 4.6 Attachments

Each file gets a random `K_file`. The file is chunked (64 KiB chunks) and each chunk is encrypted with `K_file`. The chunk index and an is-last flag go into the AAD. `K_file` is stored inside the item's details, so it inherits the vault key.

## 5. Accounts and devices

### 5.1 Invite (admin)

The admin creates an invite: email + one-time token, signed with the admin's Ed25519 key. The link is `https://<server>/invite/<token>`. Signup is invite-only: there is no open registration.

### 5.2 Signup (new user, first device)

1. Client generates the Secret Key, salt and keyset, then derives AUK and `authKey`.
2. OPAQUE registration with `authKey`.
3. Client uploads salt, Argon2 params, `encKeyset`, public keys, signature.
4. Client creates a **Personal** vault, and joins the shared **Family** vault once the admin wraps its key (§5.6).
5. Client generates the Emergency Kit PDF and requires the user to confirm it was saved.

### 5.3 Login on a new device

1. User enters server URL, email, Secret Key (or scans the Emergency Kit QR code) and master password.
2. Client fetches salt and params, derives `authKey`, runs OPAQUE login and gets a session.
3. Client downloads `encKeyset` and decrypts it with AUK.
4. The device registers (name, platform, public device key). Other devices see "new device signed in".

### 5.4 Unlock (known device)

The device already has the Secret Key in the keychain and a local copy of `encKeyset`. Unlock is offline: derive AUK, decrypt the keyset. Biometric unlock stores AUK in the keychain under a biometry-protected access control (Touch ID / Face ID / Android BiometricPrompt). The keychain entry is invalidated when biometrics change and expires after N days, forcing the password again.

### 5.5 Sessions

- After OPAQUE, the server issues an opaque random session token (256 bits). The server stores only its SHA-256 hash, together with the device ID and expiry (30 days, sliding).
- All API calls use `Authorization: Bearer <token>` over TLS.
- Devices can be listed and revoked by their owner or the admin.

### 5.6 Public key authenticity

Sharing a vault means wrapping `K_vault` to the recipient's X25519 public key, which the server delivers. A malicious server could substitute its own key. Mitigations:

- Each user's X25519 key is signed with their Ed25519 key.
- Clients pin (trust on first use) every other member's Ed25519 key and warn loudly if it changes.
- The UI shows a short fingerprint (e.g. 6 words) per member that family members can optionally compare in person once.

### 5.7 Changing the master password

The client derives a new AUK and `authKey` (new salt), re-encrypts the keyset and re-registers OPAQUE. Vault keys don't change. The Secret Key can be rotated the same way.

## 6. Recovery

Zero-knowledge means a forgotten master password or lost Secret Key loses data, unless recovery is set up ahead of time.

### 6.1 Recovery group

- Only the admin holds the **recovery keyset** (X25519 + Ed25519), encrypted with the admin's `K_user`. If the admin loses access, recoverable vaults can still be opened by their members, but nobody can perform further recoveries until a new recovery keyset is created.
- Every vault that opts into recovery also has its `K_vault` wrapped to the recovery public key. Shared vaults always opt in. Personal vaults opt in by default, and the user can opt out with a clear warning.

### 6.2 Recovery flow

1. Admin starts recovery for a user. The server marks the account "recovering" and emails a recovery link.
2. User opens the link on a device, chooses a new master password, and gets a new Secret Key and keyset. The new public keys are uploaded.
3. Admin approves on their own device: the client decrypts each of the user's vault keys with the recovery key and wraps them to the user's new public key.
4. The user's old sessions and devices are revoked.

The server never sees a vault key. The admin's client can technically decrypt recoverable vaults; this is the trust trade-off of recovery, and it is shown to users at signup.

## 7. Sync protocol

### 7.1 Model

- Each vault has a monotonic `seq` counter on the server.
- Each item has an `id` (UUIDv7, client generated), a `revision` (UUIDv7, client generated per edit), a `seq` (assigned by the server on write) and a `deleted` flag (tombstone).
- The server stores the current version and keeps item versions as history for 90 days.

### 7.2 Pull

```
GET /api/v1/vaults/{vaultID}/changes?since={seq}&limit=500
→ { items: [ {id, revision, seq, deleted, overview, details, updatedAt, updatedBy} ], nextSeq, more }
```

The client applies changes to its local SQLite cache and stores `nextSeq`. Polling starts with a simple interval (60 s plus on focus). A WebSocket/SSE `vault-changed` notification is added later.

### 7.3 Push

```
PUT /api/v1/vaults/{vaultID}/items/{itemID}
If-Match: <base revision>    (absent for new items)
{ revision, overview, details, deleted }
→ 200 { seq } | 409 { current item }
```

On `409` the client merges (§7.4) and retries with the current revision as the new base.

### 7.4 Conflict resolution (client side)

The server cannot read items, so merging happens on the client:

1. Three-way merge per field using the local base version (from local history).
2. If the same field changed on both sides: keep the remote value as current and the local value in the item's field history, and show a "conflict resolved" badge.
3. If the merge cannot be done (e.g. one side deleted): keep both — restore the item and add a "(conflicted copy)" item.

### 7.5 Rollback and withholding detection

- Clients remember the highest `seq` seen per vault. A response with a lower `seq` is rejected as a rollback.
- A server can still hide recent writes from other devices. This is accepted for a self-hosted server, because the server is ours.

## 8. Server

### 8.1 Data model (Postgres)

```
accounts       (id, email, name, role[admin|member], state, argon2_params, salt, opaque_record,
                enc_keyset, x25519_pub, ed25519_pub, x25519_sig, created_at)
devices        (id, account_id, name, platform, created_at, last_seen_at, revoked_at)
sessions       (token_hash, account_id, device_id, expires_at)
invites        (token_hash, email, created_by, signature, expires_at, used_at)
vaults         (id, type[personal|shared], enc_name, seq, recoverable, created_at)
vault_access   (vault_id, account_id, wrapped_key, role[owner|editor|reader], granted_by)
recovery_keys  (vault_id, wrapped_key)
recovery_group (id, x25519_pub, ed25519_pub, members...)
items          (vault_id, id, revision, seq, deleted, overview_ct, details_ct, updated_at, updated_by)
item_history   (vault_id, id, revision, overview_ct, details_ct, updated_at, updated_by)
attachments    (id, vault_id, item_id, size, chunk_count, storage_path)
audit_events   (id, account_id, device_id, type, ip, created_at)
```

`enc_name` is the vault name encrypted with `K_vault`.

### 8.2 API surface (v1)

| Area | Endpoints |
|---|---|
| Auth | `POST /auth/register/{start,finish}`, `POST /auth/login/{start,finish}`, `POST /auth/logout`, `GET /auth/prelogin?email=` (salt, params) |
| Account | `GET/PUT /account`, `GET /account/keyset`, `PUT /account/password` |
| Devices | `GET /devices`, `DELETE /devices/{id}` |
| Members | `GET /members` (public keys, signatures), `POST /invites`, `GET /invites/{token}` |
| Vaults | `GET /vaults`, `POST /vaults`, `PUT /vaults/{id}/access/{accountID}`, `DELETE /vaults/{id}/access/{accountID}`, `POST /vaults/{id}/rotate` |
| Items | `GET /vaults/{id}/changes`, `PUT /vaults/{id}/items/{itemID}`, `GET /vaults/{id}/items/{itemID}/history` |
| Attachments | `PUT/GET /vaults/{id}/attachments/{attID}/chunks/{n}` |
| Recovery | `POST /recovery/{accountID}/start`, `POST /recovery/{accountID}/complete` |
| Admin | `GET /admin/audit`, `GET /admin/accounts` |

All under `/api/v1`. JSON bodies, base64url for binary fields.

### 8.3 Hardening

- Rate limiting per IP and per email on `prelogin` and `login` endpoints. `prelogin` returns a deterministic fake salt for unknown emails so it doesn't reveal which accounts exist.
- Request size limits. Strict JSON decoding (unknown fields rejected).
- Security headers on the invite/recovery web pages (CSP, HSTS, no framing).
- Audit log for logins, device changes, invites, sharing changes and recovery.

### 8.4 Deployment

- Single static binary `hisar-server` in a Docker image, with Postgres 17 alongside it in Docker Compose.
- HTTP framework: Fiber v3 (fasthttp). It serves plain HTTP behind the reverse proxy; HTTP/2 and TLS are handled by Caddy.
- Served publicly at `https://tryhisar.com`, TLS terminated by Caddy (automatic certificates) in front.
- Attachments on local disk (later: S3-compatible storage).
- Nightly `pg_dump` + attachments encrypted with `age` and copied off-server. Restore tested quarterly.

## 9. Clients

All clients use `core/` for crypto, the local store and sync.

### 9.1 Desktop — Wails v3 (Go + Vue/TypeScript)

- The Go side owns the unlocked keyset and vault keys (in `memguard` locked buffers). The web UI calls Go methods and receives only the decrypted data it asks for.
- Local store: SQLite (`modernc.org/sqlite`, pure Go) holding the server's ciphertext as-is plus sync state. Nothing is stored decrypted on disk.
- Auto-lock after idle time and on screen lock/sleep. Clipboard cleared after 90 s.
- Ships the native-messaging host for the browser extension.

### 9.2 Browser extension — TypeScript, MV3

- No crypto and no keys in the extension. It talks to the desktop app through the browser's native messaging API.
- Pairing: on first connect, the desktop shows a code that the user confirms in the extension. The native host manifest allows only our extension IDs.
- Autofill only when the page origin matches an item URL (scheme + host, with a configurable eTLD+1 match). Never autofill automatically into cross-origin iframes. Fill on explicit user action by default.
- Detects new and changed logins and offers to save them.

### 9.3 Mobile — gomobile + native UI

- `gomobile bind` compiles `core/` into an iOS `.xcframework` and an Android `.aar`.
- Native UI: SwiftUI and Kotlin/Jetpack Compose.
- System autofill: iOS AutoFill Credential Provider extension, Android AutofillService. Both share the app's encrypted store through an app group / shared storage.
- Memory constraint: the iOS AutoFill extension must stay well below the system limit (Argon2 64 MiB).

### 9.4 CLI

`hisar` CLI for development and scripting: login, unlock, list, get, generate, and later `hisar run` to inject secrets into processes.

### 9.5 Go memory handling

Go's garbage collector can copy memory and strings are immutable, so secrets cannot be reliably wiped. Rules:

- Keys and secrets are `[]byte`, never `string`, inside `core/`.
- Long-lived keys (AUK, keyset, vault keys) live in `memguard` enclaves and are opened only for the duration of an operation.
- Plaintext item data is zeroed after use where possible, and the lock action drops all references.

## 10. Repository layout

```
core/       Go module: crypto, item model, local store, sync client   (github.com/tryhisar/hisar/core)
server/     Go module: hisar-server                                   (github.com/tryhisar/hisar/server)
cli/        Go module: hisar CLI                                      (github.com/tryhisar/hisar/cli)
desktop/    Wails app (Go + Vue)                                      (Phase 2)
extension/  Browser extension (TypeScript)                            (Phase 3)
mobile/     iOS + Android apps on top of gomobile-bound core          (Phase 4)
docs/       Design and decisions
go.work     Ties the Go modules together for local development
```

## 11. Roadmap

| Phase | Scope | Exit criteria |
|---|---|---|
| 0 | This document, repo skeleton, CI | Design reviewed and agreed |
| 1 | `core/crypto` with test vectors, OPAQUE auth, server (accounts, vaults, items, sync), CLI | Two CLI clients on two machines sync a shared vault through a self-hosted server |
| 2 | Desktop app: signup + Emergency Kit, unlock, items, generator, search, Personal + Family vaults, invites, recovery, auto-lock, Touch ID | Family uses it daily on macOS |
| 3 | Browser extension via native messaging: fill, save, generate | Daily logins without copy-paste |
| 4 | Mobile apps with system autofill | Logins on phones |
| 5 | TOTP, passkeys, attachments, Watchtower (reused/weak/breached via HIBP k-anonymity), import from 1Password/Bitwarden/CSV, SSE push sync | — |

## 12. Decisions

| # | Question | Decision |
|---|---|---|
| 1 | Argon2id parameters | Default t=3, m=64 MiB, p=4; confirmed by benchmark in Phase 1, re-checked on phones in Phase 4. Target ≈ 0.5–1 s |
| 2 | Wails version | v3 |
| 3 | Server domain | `tryhisar.com`, public |
| 4 | Recovery group members | Admin only |
| 5 | Item history retention | 90 days |
| 6 | Fingerprint check (§5.6) | Optional; key-change warnings are always on |
