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
| Public-key wrapping | HPKE base mode, hybrid post-quantum KEM ML-KEM-768 + X25519, HKDF-SHA256, ChaCha20-Poly1305 (RFC 9180) | `crypto/hpke` |
| Signatures (key fingerprints, invites) | Ed25519 | `crypto/ed25519` |
| Authentication | OPAQUE (RFC 9807), ristretto255 suite | `github.com/bytemare/opaque` |
| Randomness | OS CSPRNG | `crypto/rand` |

Every ciphertext is versioned (`v1`) so primitives can be rotated later.

## 4. Key hierarchy

```
master password ──Argon2id──► k_pw ─┐
                                    ├─HKDF──► AUK  (Account Unlock Key) ──decrypts──► keyset
Secret Key ───────HKDF────────► k_sk ┘   └──HKDF──► auth secret ──► OPAQUE login

keyset = { sym key K_user, KEM seed (ML-KEM-768+X25519), Ed25519 signing key }
   └─ KEM public key ◄── HPKE-wraps ── vault key K_vault (one per vault, per member)
                                              └─ XChaCha20-Poly1305 ── items, attachments keys
```

### 4.1 Secret Key

- 128 bits from `crypto/rand`, generated on the user's first device. Never sent to the server.
- Display format: `H1-AAAAAA-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX` — version, 6-char account ID, 26 Crockford base32 chars.
- Stored on each device in the OS keychain (macOS Keychain, Windows Credential Manager, libsecret, iOS Keychain, Android Keystore).
- Printed on the Emergency Kit together with the server URL and email. The A4 PDF is generated client-side (`core/emergencykit`, gopdf with the embedded Go fonts) and has a blank line for the master password, which is never printed. Its QR code holds `hisar://sign-in?server=…&email=…&secretKey=…`.
- Desktop saves the kit through a native save dialog: right after sign up (the Secret Key stays in memory until the user confirms or the app locks), and later from Settings after re-entering the master password. The CLI has `hisar emergency-kit [-o file]`. Files are written with mode 0600, and the CLI never overwrites an existing file.

### 4.2 Deriving AUK and the auth secret

```
pw      = NFKD(master password), UTF-8
salt    = random 16 bytes, per account, stored on server (public)
k_pw    = Argon2id(pw, salt, t=24, m=64 MiB, p=4, len=32)
k_sk    = HKDF-SHA256(ikm=SecretKey, salt=accountID, info="hisar/v1/secret-key", len=32)
ikm     = k_pw || k_sk
AUK     = HKDF-SHA256(ikm, salt=nil, info="hisar/v1/auk",  len=32)
authKey = HKDF-SHA256(ikm, salt=nil, info="hisar/v1/auth", len=32)
```

- Argon2id parameters are stored per account so they can be raised later. 64 MiB is chosen to fit inside the iOS AutoFill extension memory limit, so cost is tuned with `t`. t=24 takes ≈ 270 ms on an M2 Pro; re-check on phones in Phase 4.
- Clients enforce minimum and maximum parameters (`MinArgon2Params`/`MaxArgon2Params` in `core/crypt`) because the server supplies them: a malicious server must not be able to weaken stretching or exhaust memory.
- `authKey` is the OPAQUE password input. OPAQUE's internal key-stretching function is set to identity because stretching already happened. A server that steals the OPAQUE record still needs the Secret Key to guess the password.
- AUK and `authKey` are independent HKDF outputs: the server learns nothing about AUK from the login.

### 4.3 Keyset

Generated at signup on the first device:

- `K_user`: 32 random bytes, encrypts the user's private data that is not in a vault (settings, device list labels).
- Hybrid KEM keypair (ML-KEM-768 + X25519, derived from a 32-byte seed): receives vault keys. The hybrid stays secure as long as either half holds, so recorded traffic and DB dumps remain safe against a future quantum computer.
- Ed25519 keypair: signs the KEM public key bound to the account ID, and invites, so others can verify it (§5.6).

The keyset is stored as three 32-byte seeds (`K_user`, KEM seed, Ed25519 seed); the key pairs are derived from them.

The private half is serialised and encrypted:

```
encKeyset = XChaCha20-Poly1305(key=AUK, nonce=random24, aad="hisar/v1/keyset"||accountID, plaintext=keyset)
```

The server stores `encKeyset`, the KEM public key, the Ed25519 public key and the signature `Ed25519.Sign("hisar/v1/kem-key" || accountID || kemPub)`.

### 4.4 Vault keys

- Each vault has a random 32-byte `K_vault`.
- For every member, the server stores `0x01 || HPKE.Seal(memberKEMPub, info="hisar/v1/vault-key" || vaultID || memberAccountID, K_vault)`.
- The recovery keyset (§6.1) gets the same wrapping, with recipient ID `"recovery"`, for every vault that opts into recovery.
- Removing a member rotates `K_vault` (`session.RemoveVaultMember`, `POST /vaults/{id}/rotate`, owner only):
  - The owner's client generates a new key, re-encrypts the vault name and every item, including tombstones, under a new revision, and wraps the new key for each remaining member. Each member's signed KEM key is verified, and the client's `MemberVerifier` hook (key pinning in the CLI and the desktop app) must accept it.
  - The server applies it in one transaction. It returns 409 unless the submitted items match the stored items and revisions exactly, so concurrent edits are never lost; the client then retries, up to 3 times. It replaces all access grants, gives every item a new seq so all devices resync, and drops the vault's item history, because that history is encrypted with the old key.
  - The rotating owner must keep owner access. Rotation requests may be up to 32 MiB; every other endpoint is limited to 2 MiB.
  - Copies the removed member downloaded before the rotation stay readable to them; that is unavoidable.

### 4.5 Items

An item has two encrypted parts so lists and search don't decrypt every secret:

- **overview**: title, URLs, tags, icon hint, subtitle (username).
- **details**: fields, passwords, notes, TOTP seeds, passkeys, history of field values.

```
aad      = "hisar/v1/item" || vaultID || itemID || revision || part      (part ∈ {"overview","details"})
ct_part  = XChaCha20-Poly1305(key=K_vault, nonce=random24, aad, plaintext=JSON(part))
```

Binding vault ID, item ID and revision in the AAD stops a malicious server from moving an item into another vault, swapping overview/details between items, or mixing parts of different revisions.

All `||` concatenations of variable-length fields are length-prefixed (4-byte big-endian length before each field), so field boundaries are unambiguous.

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
2. Client fetches salt and params, derives `authKey`, runs OPAQUE login and gets a session. OPAQUE identities: client = account ID, server = the constant `"hisar"` (`core/pake`; `bytemare/opaque` v0.18.0 cannot decode key material with an empty identity). The server's OPAQUE key material and an HMAC key live in `hisar-secrets.json`, created on first start.
3. Client downloads `encKeyset` and decrypts it with AUK.
4. The device registers (name, platform, public device key). Other devices see "new device signed in".

### 5.4 Unlock (known device)

The device already has the Secret Key in the keychain and a local copy of `encKeyset` (the desktop cache, §9.1). Unlock works offline: derive AUK, decrypt the keyset. Online, the client takes the account record from the server and refreshes the cached copy.

**Touch ID (macOS, MVP).** Target design, once the app is signed with a team ID (§12 #7): the desktop app stores AUK in a Keychain item protected by `SecAccessControl` with `kSecAccessControlBiometryCurrentSet` and `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` (never synced to iCloud), so macOS itself releases it only after Touch ID, and re-enrolling fingerprints invalidates it.

**Interim implementation (ad-hoc signed builds).** Keychain items with biometric access control need a team-signed app, so until then:

- Enabling Touch ID requires the master password and a Touch ID check. The app derives AUK, generates a random 32-byte wrapping key, stores `Seal(wrappingKey, AUK, "hisar/v1/biometric-unlock:" || accountID)` in the local state file, and stores the wrapping key as a generic password in the login Keychain via `Security.framework` (so the item's ACL trusts the Hisar binary, not a shared CLI tool).
- Unlocking with Touch ID: `LAContext.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics)` succeeds, then the app loads the wrapping key, opens AUK and decrypts the keyset. The fingerprint check is enforced by the app, not by the Keychain; this is the weaker part that the target design removes.
- If the sealed key no longer opens the keyset (master password changed elsewhere) or the Keychain item is missing, Touch ID unlock is turned off and the master password is required.
- The upgrade keeps the UI and the `Biometric` interface (`desktop/vault`); only the storage in `desktop/biometric` changes.

Common to both:

- The master password is still required after every app launch and at least every 14 days (`LastPasswordUnlockAt` in the state file).
- **In-window prompt**: like 1Password, the lock screen embeds Apple's `LAAuthenticationView` (`LocalAuthenticationEmbeddedUI`, macOS 12+) in Hisar's own window instead of the system dialog. The UI measures a slot on the lock screen and the app places the native view over it; "Use master password" cancels the evaluation (`LAContext.invalidate`).
- Touch ID also approves SSH agent requests (§9.6). Face ID and Android BiometricPrompt follow the same pattern in Phase 4.

### 5.5 Sessions

- After OPAQUE, the server issues an opaque random session token (256 bits). The server stores only its SHA-256 hash, together with the device ID and expiry (30 days, sliding).
- All API calls use `Authorization: Bearer <token>` over TLS.
- Devices can be listed and revoked by their owner or the admin.

### 5.6 Public key authenticity

Sharing a vault means wrapping `K_vault` to the recipient's KEM public key, which the server delivers. A malicious server could substitute its own key. Mitigations:

- Each user's KEM public key is signed with their Ed25519 key, bound to their account ID.
- Clients pin (trust on first use) every other member's Ed25519 key in their local state when they first share with them (`localstate.State.CheckMemberPin`, used by the CLI and the desktop app). Sharing, changing a member's role and key rotation stop if a pinned key changes; the desktop app also marks that member in its member lists.
- The UI shows a short security code per member that family members can optionally compare in person once: the first 8 bytes of SHA-256 of their Ed25519 public key as 4 groups of 4 uppercase hex digits, e.g. `3F2A 91C0 7B44 E1D8` (`crypt.FormatKeyFingerprint`).

### 5.7 Changing the master password

The client derives a new AUK and `authKey` (new salt), re-encrypts the keyset and re-registers OPAQUE. Vault keys don't change. The Secret Key can be rotated the same way.

## 6. Recovery

Zero-knowledge means a forgotten master password or lost Secret Key loses data, unless recovery is set up ahead of time.

### 6.1 Recovery keyset

- Only the admin holds the **recovery keyset**, a regular `crypt` keyset created once with `session.CreateRecoveryKeyset` (`hisar recovery-setup`):
  - Its private part is encrypted with the admin's `K_user`: `XChaCha20-Poly1305(key=K_user, aad="hisar/v1/recovery-keyset"||adminAccountID)`.
  - Its KEM public key is signed with the admin's Ed25519 key, bound to the literal recipient ID `"recovery"`: `Ed25519.Sign("hisar/v1/kem-key" || "recovery" || kemPub)`, the same format as account keys (§4.3), so `PublicKeys.Verify("recovery")` checks it.
  - The server stores the KEM public key, the admin's signing public key, the signature, the encrypted keyset and `created_by`. It accepts the keyset only from an admin, only once, and only signed by that admin's current signing key. `GET /recovery/keyset` returns the encrypted keyset to its creator only.
- Before wrapping anything to the recovery key, clients verify it (`session.GetRecoveryPublicKey`): `created_by` must be an admin in `/members`, whose own signed keys verify and whose signing key is the one that signed the recovery key. The CLI then pins the recovery KEM key trust-on-first-use (`AllPin["recovery"]`), like member keys (§5.6).
- Vault keys are wrapped to the recovery key with recipient ID `"recovery"` (§4.4) and stored in `vault_recovery`:
  - `session.CreateVault` wraps every new vault when a recovery keyset exists. Shared vaults must be wrapped: the server rejects a shared vault without the wrapping. Personal vaults are wrapped by default.
  - `PUT /vaults/{id}/recovery` (owner) sets the wrapping. `DELETE /vaults/{id}/recovery` (owner, personal vaults only) removes it and records the opt-out (`vaults.recovery_opt_out`), so later bulk enabling skips the vault. The UI must warn that an opted-out vault is lost if the user loses access.
  - `GET /vaults` returns `recoverable` and `recoveryOptOut` per vault. `session.ListAllVaultWithoutRecovery` lists owned vaults without a wrapping, and `session.EnableRecoveryForAllVault` (`hisar recovery-enable`, and `hisar recovery-setup` for the admin) wraps the ones that are not opted out. Vaults created before the keyset existed need this.
  - Key rotation (§4.4) carries the new key wrapped to the recovery key when the vault had a wrapping. The server replaces it in the rotation transaction, and returns 409 if the vault has a wrapping and the rotation does not carry one.
- If the admin loses access, recoverable vaults can still be opened by their members, but nobody can perform further recoveries until a new recovery keyset is created. When another admin recovers the creating admin, the recovery keyset and all recovery wrappings are deleted in the same transaction (they were sealed with the old `K_user`), and recovery must be set up again.

### 6.2 Recovery flow

1. **Start (admin).** `POST /admin/recovery/{accountID}` (`hisar recovery-start <email>`) creates a one-time token (the server stores its SHA-256 hash; it expires after 7 days) and returns `<PublicURL>/recover/<token>`. Starting again replaces the token and any pending submission. The account keeps working until completion. Admins cannot recover themselves; another admin must do it.
2. **Submit (user).** `session.StartRecovery` (`hisar recover <link>`) asks for the email and a new master password:
   - `POST /auth/recovery/account {token, email}` returns the account ID, because the Secret Key and the derived keys (§4.2) depend on it.
   - The client then does what signup does for the existing account ID: new Secret Key, salt, Argon2 params and keyset, and OPAQUE registration with credential identifier = account ID (`POST /auth/recovery/start {token, email, request}`).
   - `POST /auth/recovery/finish {token, email, record, salt, argon2, encKeyset, publicKeys, device}` is validated like signup (signature, params, sizes), consumes the token and stores the submission as pending (`account_recovery`, one row per account).
   - The user gets the new Secret Key but cannot sign in with it until the admin completes the recovery; until then the old credentials still work. An expired, used or unknown token, or a different email, returns 403 `invalid recovery link`. These endpoints are rate-limited like the other auth endpoints.
3. **Review (admin).** `GET /admin/recovery` (`hisar recovery-list`) lists pending recoveries: account ID, email, name, the new public keys and signature, submission time, and every vault the account can access with its recovery wrapped key (`null` when it has none).
4. **Complete (admin).** `session.CompleteRecovery` (`hisar recovery-complete <email>`), run by the admin who created the recovery keyset:
   - The client verifies the new public keys (`PublicKeys.Verify(accountID)`), decrypts the recovery keyset with its `K_user`, unwraps each vault key (recipient `"recovery"`) and wraps it to the user's new KEM key (recipient = account ID).
   - `POST /admin/recovery/{accountID}/complete {vaultKeys: [{vaultId, wrappedKey}]}` applies it in one transaction: it replaces the account's OPAQUE record, salt, Argon2 params, encrypted keyset and public keys with the pending ones, updates the account's wrapped key for each provided vault, removes its access to every other vault, deletes all of its sessions and devices, and deletes the pending row and the token.
   - The response lists the vaults that could not be recovered (opted out, or without a wrapping), and the UI tells the admin. The admin's CLI re-pins the user's new signing key; other members see a key-change warning (§5.6).
5. **Sign in (user).** The user signs in normally with the email, the new Secret Key and the new master password.

Every step is audit-logged (`recovery_keyset_created`, `vault_recovery_set`, `vault_recovery_opt_out`, `recovery_started`, `recovery_submitted`, `recovery_completed`).

The desktop app (`desktop/vault/recovery.go`) offers the same steps: Settings → Account recovery lets an admin set up recovery, lists pending recoveries and completes them (showing the vaults that could not be recovered), and a family member's row starts a recovery and shows the link. Members see whether their vaults are recoverable, and the owner of a personal vault can turn recovery off there (with the warning above) and back on. The sign-in screen's **Recover account** submits a recovery link while signed out, shows the new Secret Key and can save it as an Emergency Kit. Like the CLI, the desktop app pins the recovery KEM key on first use and re-pins a member's signing key after completing their recovery.

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
vaults         (id, type[personal|shared], enc_name, seq, recovery_opt_out, created_at)
vault_access   (vault_id, account_id, wrapped_key, role[owner|editor|reader], granted_by)
vault_recovery (vault_id, wrapped_key, updated_at)
recovery_keyset  (kem_pub, sign_pub, kem_sig, enc_keyset, created_by, created_at)   -- at most one row
recovery_tokens  (account_id, token_hash, created_by, created_at, expires_at, used_at)
account_recovery (account_id, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig, submitted_at)
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
| Vaults | `GET /vaults`, `POST /vaults` (optional `recoveryWrappedKey`), `GET /vaults/{id}/members`, `PUT /vaults/{id}/access/{accountID}`, `DELETE /vaults/{id}/access/{accountID}`, `POST /vaults/{id}/rotate` (Phase 2, optional `recoveryWrappedKey`), `PUT /vaults/{id}/recovery`, `DELETE /vaults/{id}/recovery` (owner; delete is personal vaults only) |
| Items | `GET /vaults/{id}/changes`, `PUT /vaults/{id}/items/{itemID}`, `GET /vaults/{id}/items/{itemID}/history` |
| Attachments | `PUT/GET /vaults/{id}/attachments/{attID}/chunks/{n}` |
| Recovery (Phase 2) | `GET /recovery/keyset` (any member; the encrypted keyset only for its creator), `POST /recovery/keyset` (admin, once); `POST /auth/recovery/account`, `POST /auth/recovery/start`, `POST /auth/recovery/finish` (recovery token, rate-limited); `GET /admin/recovery`, `POST /admin/recovery/{accountID}`, `POST /admin/recovery/{accountID}/complete` (admin) |
| Admin | `GET /admin/audit`, `GET /admin/accounts` |

All under `/api/v1`. JSON bodies, base64url for binary fields.

### 8.3 Hardening

- Rate limiting per IP and per email on `prelogin` and `login` endpoints. `prelogin` returns a deterministic fake salt for unknown emails so it doesn't reveal which accounts exist.
- Request size limits. Strict JSON decoding (unknown fields rejected).
- Security headers on the invite/recovery web pages (CSP, HSTS, no framing).
- Audit log for logins, device changes, invites, sharing changes and recovery.

### 8.4 Deployment

- Single static binary `hisar-server` in a Docker image, with Postgres 17 alongside it in Docker Compose.
- Schema migrations: goose (`pressly/goose`), SQL files embedded in the binary, up and down sections, applied on start under a Postgres advisory lock or explicitly with `hisar-server migrate up|down|status`.
- HTTP framework: Fiber v3 (fasthttp). It serves plain HTTP behind the reverse proxy; HTTP/2 and TLS are handled by Caddy.
- Served publicly at `https://tryhisar.com`, TLS terminated by Caddy (automatic certificates) in front.
- Attachments on local disk (later: S3-compatible storage).
- Nightly `pg_dump` + attachments encrypted with `age` and copied off-server. Restore tested quarterly.

## 9. Clients

All clients use `core/` for crypto, the local store and sync.

### 9.1 Desktop — Wails v3 (Go + Vue/TypeScript)

- The Go side owns the unlocked keyset and vault keys (in `memguard` locked buffers). The web UI calls Go methods and receives only the decrypted data it asks for.
- Local store: SQLite (`modernc.org/sqlite`, pure Go) holding the server's ciphertext as-is plus sync state. Nothing is stored decrypted on disk.
  - `core/cache` stores the account record (salt, Argon2 params, `encKeyset`, public keys), the vault list as the server returns it (wrapped keys, encrypted names), every live item's encrypted parts, and each vault's `seq`. The file is `desktop-cache.db` next to `desktop.json`, mode 0600, and is deleted on sign-in, sign-up and sign-out.
  - `session.Session.Cache`: listing vaults replaces the cached list and forgets vaults that disappeared (removed access). Listing items asks for changes since the cached `seq`, applies them page by page in one transaction each (tombstones delete the row), and decrypts from the cache. A `nextSeq` below the cached `seq` is a rollback (§7.5). A cached item that no longer decrypts makes the vault download from `seq` 0 once.
  - The server counts as unreachable on a connection error or a 502/503/504 from the reverse proxy (`client.IsUnreachable`). Then reads, unlock with the master password, Touch ID unlock and the master password check come from the cache, and the session reports `IsOffline`. Any other error, such as 401 for a revoked device, is never hidden by the cache.
  - Offline, the app shows an "Offline" notice and refuses changes with a clear message. Edits are not queued; that would need the merge of §7.4. Background sync runs every 60 s and when the window gets focus, and stays quiet while an item is being edited.
- Auto-lock after idle time and on screen lock/sleep. Clipboard cleared after 90 s.
- Ships the native-messaging host for the browser extension.
- Runs the SSH agent (§9.6) and handles Touch ID (§5.4).

### 9.2 Browser extension — TypeScript, MV3

- No crypto and no keys in the extension. It talks to the desktop app through the browser's native messaging API; the desktop app must be running (the host starts it when it is not).
- Pairing: on first connect, the desktop shows a code that the user confirms in the extension. The native host manifest allows only our extension IDs.
- Autofill only when the page origin matches an item URL (below). Never autofill automatically into cross-origin iframes. Fill on explicit user action by default.
- Detects new and changed logins and offers to save them.
- Chromium browsers first (Chrome, Brave, Edge, Arc, Vivaldi, Chromium), from one MV3 build. Firefox later from the same code; Safari needs a team-signed app (§12 #7). Distributed as an unlisted Chrome Web Store item.

**Extension ID.** The manifest's `key` holds the public key, so the ID is `mmipiffoogblblocokimpemdopmdjghf` both unpacked and from the store (the private key is uploaded once with the first store submission and is kept outside the repository). The native host manifest allows only `chrome-extension://mmipiffoogblblocokimpemdopmdjghf/`.

**Chain.** `extension (service worker) ⇄ hisar-browser-host ⇄ Hisar desktop app`:

- The browser starts `hisar-browser-host` (shipped in `Hisar.app/Contents/MacOS/`) with the caller's origin as its first argument and speaks native messaging over stdio: each message is a 32-bit length in native byte order followed by UTF-8 JSON. Hisar caps messages at 1 MiB in both directions.
- The host connects to the desktop app on `~/.hisar/browser.sock` (directory 0700, socket 0600) and relays frames unchanged, in the same framing. Its first frame is `{"type":"connect","extensionOrigin":"chrome-extension://…/"}`; the desktop refuses other origins. When the socket is missing, the host launches the app (`open -g -b com.tryhisar.hisar`) and retries for 10 s, then answers each request with `desktop-unavailable`.
- When the packaged host exists next to the app binary, the desktop accepts only connections whose peer process is that executable (peer PID → executable path, as the SSH agent does).
- The desktop installs the host manifest `com.tryhisar.hisar.json` (`type: stdio`, `path`, `allowed_origins`) into `NativeMessagingHosts` of every installed Chromium browser at launch, and shows the result in Settings.

**Messages.** Requests are `{"id": "<uuid>", "type": "…", "token": "…", …parameters}`; responses are `{"id", "ok": true, "result": {…}}` or `{"id", "ok": false, "error": {"code", "message"}}`. Error codes: `invalid-request`, `not-paired`, `pairing-failed`, `signed-out`, `locked`, `offline`, `not-found`, `no-match`, `desktop-unavailable`, `internal`. Every type except `hello`, `pair.start` and `pair.finish` requires the pairing `token`.

| Type | Parameters | Result |
|---|---|---|
| `hello` | `instanceID`, `browser`, `extensionVersion` | `protocolVersion` (1), `isPaired`, `state` (`signed-out`, `locked`, `unlocked`) |
| `pair.start` | `instanceID`, `browser` | `{}`; the desktop window shows "Pair <browser>?" with a 6-digit code |
| `pair.finish` | `instanceID`, `code` | `token`. The code is valid for 2 minutes and 5 attempts |
| `status` | — | `state`, `email`, `isOffline` |
| `unlock` | — | `{}`; brings the desktop window forward (on the Touch ID screen when it is ready) |
| `logins.match` | `url` | `logins`: `[{vaultID, itemID, title, username, vaultName}]`, sorted by title |
| `logins.fill` | `vaultID`, `itemID`, `url` | `username`, `password`; the desktop checks the URL match again |
| `logins.check` | `url`, `username`, `password` | `status` (`new`, `changed`, `unchanged`) and, unless `new`, `vaultID`, `itemID`, `title` |
| `logins.save` | `url`, `username`, `password`, optional `vaultID` + `itemID` | `vaultID`, `itemID`. Without an item, creates a login titled by the site's host in the Personal vault; with one, updates its username and password |
| `password.generate` | optional `length`, `digits`, `symbols` | `password` |

The desktop keeps paired browsers (`instanceID`, browser name, SHA-256 of the token, paired and last-used times) in its state file; Settings lists them with Remove. Signing out removes them.

**URL matching** (desktop, `desktop/browser`). Item URLs without a scheme are read as `https://`. A page matches an item URL when the ports match (default ports normalized) and the scheme is the same, or the item says `http` and the page is `https`. By default the hosts must share their registrable domain (eTLD+1 from the public suffix list, so `accounts.google.com` matches `google.com`, but two `github.io` sites do not); the Settings option "Only fill on the exact host" requires equal hosts. IP addresses and `localhost` always need equal hosts.

**Extension behaviour.**

- Popup: when not paired, the pairing flow; when locked, "Unlock in Hisar"; otherwise the logins matching the current tab with Fill, Copy username and Copy password, a search over them, and the password generator (Copy, Fill into the focused field).
- Fill (popup, or the `fill-login` keyboard shortcut for the first match): the service worker asks the desktop with the frame's URL and fills only the top frame and frames with the top frame's origin; it never fills a frame that is cross-origin to the page, even when the item would match.
- Save: the content script records the username and password of a submitted login form and asks `logins.check`. For `new` or `changed` it shows an in-page bar (shadow DOM, top frame): Save / Update, Not now, Never for this site (stored per origin in `chrome.storage.local`).
- Permissions: `nativeMessaging`, `storage`, `scripting`, `activeTab`, `tabs`, and host permissions for `<all_urls>` (the content script detects logins on every page). The extension stores only its `instanceID`, the token and the "never" list.
- Clipboard: MV3 service workers cannot reach the clipboard, so copying runs in an offscreen document, which clears the clipboard after 90 s if it still holds the copied value. This adds the `offscreen`, `clipboardWrite` and `clipboardRead` permissions.
- Frame rule in practice: a child frame counts as having the top frame's origin only when every frame between it and the top has that origin too (`location.ancestorOrigins`). The content script checks this again before filling.
- The service worker keeps one native port and reconnects on the next request after a disconnect, not on a timer, because each connection starts the host, and the host would relaunch Hisar after the user quit it.

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

### 9.6 SSH agent (MVP)

Like 1Password, the desktop app is an SSH agent for keys stored in vaults, so private keys never sit in `~/.ssh`.

**SSH Key items**

- Item category `ssh-key` (`item.Overview.Category`). The overview holds the title, the public key as an `authorized_keys` line (`publicKey`), the SHA-256 fingerprint and the key type (`ssh-ed25519`, `ssh-rsa`, `ecdsa-sha2-*`); the details hold the private key as an unencrypted OpenSSH PEM (`privateKey`), protected by the vault encryption like every other secret.
- Generate inside the app (Ed25519 by default, RSA 4096 if requested), or import an existing OpenSSH, PKCS#1, SEC 1 or PKCS#8 key. Passphrase-protected keys are decrypted on import and stored without the passphrase.
- The UI never receives a private key: `ItemDetail` carries only the category, public key, fingerprint and key type. Editing an SSH key changes only its title and notes; the backend keeps the key material from the cached item.
- The detail view shows the key type, fingerprint and public key with "Copy public key" (copied through the backend and cleared after 90 seconds) for `authorized_keys`, GitHub and so on.

**Agent**

- Implements the OpenSSH agent protocol with `golang.org/x/crypto/ssh/agent`, including `session-bind@openssh.com`.
- Listens on `~/.hisar/agent.sock` (directory 0700, socket 0600; `HISAR_SSH_AGENT_SOCKET` overrides the path). It starts with the app and stops when the app quits. If the socket cannot be created (for example another agent is listening there), Settings shows the error.
- Offers the SSH keys of every vault the user can access while Hisar is unlocked; personal vault keys come first. Locked or signed out, it offers no keys. It is read-only: requests to add or remove keys, and the agent's lock/unlock requests, are refused.
- Settings → SSH agent shows the status and, only when the user clicks "Use Hisar for SSH", adds this section to the top of `~/.ssh/config` after copying the file to `~/.ssh/config.hisar-backup-<timestamp>` (the file is created with mode 0600 and `~/.ssh` with 0700 if missing):

  ```
  # Added by Hisar: use the Hisar SSH agent
  Host *
    IdentityAgent "~/.hisar/agent.sock"
  # End of Hisar section
  ```

  Nothing changes if the start line is already present. "Remove from SSH config" backs the file up again and deletes the lines between the markers; backups never overwrite each other, and a symlinked config is written through to its target.
- Servers allow about 6 key attempts per connection (`MaxAuthTries`). With more keys, the user points `IdentityFile` in `~/.ssh/config` at the exported public key to pick one per host; the agent still does the signing.
- Works for `git` over SSH and for SSH commit signing (`gpg.format ssh`), since both go through the agent.

**Approval**

- If Hisar is locked, the agent lists no keys and signing fails; the user unlocks Hisar and retries.
- The first use of a key by a client process asks for approval. The prompt names the requesting application, found from the socket peer PID (`LOCAL_PEERPID` on macOS, `SO_PEERCRED` on Linux) → executable path, and the destination host: OpenSSH sends the server host key with `session-bind@openssh.com`, and the agent looks it up in `~/.ssh/known_hosts` and `/etc/ssh/ssh_known_hosts` (the host key fingerprint is shown if it is not there).
- With Touch ID available, approval is a Touch ID prompt with the reason `allow "<program>" to use the SSH key "<title>" for <host>`. It uses the system dialog because the Hisar window may be in the background; one prompt is shown at a time, and cancelling denies.
- Without Touch ID, the app brings its window to the front and shows an in-app prompt ("<program> wants to use <key>", destination, program path, key fingerprint) with Deny and Allow. The backend emits the Wails event `ssh-approval` with a random request ID and waits for `RespondSSHApproval(requestID, isAllowed)`; after 60 seconds, or if the client disconnects, it denies and emits `ssh-approval-closed`. Unknown or finished request IDs are ignored.
- Approvals are remembered per (key fingerprint, program path) until Hisar locks; the vault backend calls the agent's `ClearAllApproval` from its lock hook. Requests through a forwarded agent and from programs without a known path always ask. "Ask every time" can be set per key (`SetAllAlwaysAskKeyID`; no UI yet).
- Agent forwarding (`ssh -A`) exposes the agent to the remote host. The per-use approval makes this visible ("through a forwarded agent"), and the docs recommend `ProxyJump` instead.

**Implementation**

- `core/sshagent` implements `agent.ExtendedAgent` over two interfaces: a key source and an approver. It has no UI dependency and is tested in CI with an in-memory approver and a real `x/crypto/ssh` client and server.
- `desktop/vault` stores SSH Key items (`GenerateSSHKey`, `ImportSSHKey`) and provides the key source, `vault.SSHKeySource`. It is a separate type from the bound `vault.Service`, so no method callable from the UI returns private keys.
- `desktop/agent` holds the approver (Touch ID or in-app, with the Wails event emitter injected), the `~/.ssh/config` editing and the bound status service. It does not import Wails and is unit-tested; `main.go` wires it to Touch ID, the window and the socket.
- The end-to-end test generates a key in a vault, serves it with the agent on a Unix socket and logs in to an in-process `x/crypto/ssh` server with a real SSH client; after Lock the agent lists no keys.
- Linux uses the same Unix socket; Windows (named pipe `\\.\pipe\openssh-ssh-agent`) comes later. The CLI gets `hisar ssh-key generate|import|list|public-key`.

## 10. Repository layout

```
core/       Go module: crypto, item model, local store, sync client   (github.com/tryhisar/hisar/core)
server/     Go module: hisar-server                                   (github.com/tryhisar/hisar/server)
cli/        Go module: hisar CLI                                      (github.com/tryhisar/hisar/cli)
desktop/    Wails v3 app (Go + Vue): vault/ backend, agent/ SSH agent, frontend/ UI (Phase 2)
extension/  Browser extension (TypeScript)                            (Phase 3)
mobile/     iOS + Android apps on top of gomobile-bound core          (Phase 4)
docs/       Design and decisions
deploy/     Dockerfile, Docker Compose (Postgres + server + Caddy), deployment notes
scripts/    test-end-to-end.sh: end-to-end test of the built server and CLI (run in CI)
go.work     Ties the Go modules together for local development
```

## 11. Roadmap

| Phase | Scope | Exit criteria |
|---|---|---|
| 0 | This document, repo skeleton, CI | Design reviewed and agreed |
| 1 | `core/crypt` with test vectors, OPAQUE auth, server (accounts, vaults, items, sync), CLI, Docker deployment, CI (unit and database tests, end-to-end test of the built binaries, image build) | Two CLI clients on two machines sync a shared vault through a self-hosted server |
| 2 (MVP) | Desktop app (macOS first): signup + Emergency Kit, unlock, **Touch ID unlock** (§5.4), items, generator, search, Personal + Family vaults, invites, recovery, vault key rotation, auto-lock, **SSH agent with SSH Key items and Touch ID approval** (§9.6), local encrypted cache. CI builds the desktop app | Family uses it daily on macOS; `ssh` and `git` use keys from Hisar |
| 3 | Browser extension via native messaging: fill, save, generate | Daily logins without copy-paste |
| 4 | Mobile apps with system autofill | Logins on phones |
| 5 | TOTP, passkeys, attachments, Watchtower (reused/weak/breached via HIBP k-anonymity), import from 1Password/Bitwarden/CSV, SSE push sync | — |

## 12. Decisions

| # | Question | Decision |
|---|---|---|
| 1 | Argon2id parameters | t=24, m=64 MiB, p=4 (≈ 270 ms on M2 Pro); re-check on phones in Phase 4. Target ≈ 0.5–1 s on the slowest device |
| 2 | Wails version | v3, pinned beta (`v3.0.0-beta.23`; no stable v3 exists yet). Upgrade deliberately, one beta at a time, with the macOS and Linux CI builds as the gate |
| 3 | Server domain | `tryhisar.com`, public |
| 4 | Recovery group members | Admin only |
| 5 | Item history retention | 90 days |
| 6 | Fingerprint check (§5.6) | Optional: a 16-hex-digit security code per member, shown in the desktop app's sharing panel and Settings → Family. Key pinning and key-change refusals are always on |
| 7 | Apple Developer Program membership (code signing and notarization; required for biometric Keychain items and Gatekeeper) | **Open**: needed before Touch ID can ship. During development, local signing with a free Apple ID team works on your own Mac |
