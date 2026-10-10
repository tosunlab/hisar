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

**One-time passwords (TOTP).** `details.oneTimePassword` holds an `otpauth://totp` URI, in the encrypted details part.

- **Saving:** a pasted setup key or link is normalized into the URI (`core/totp`): base32 secret, SHA-1/256/512, 6–8 digits, a period of 1–300 s. Anything else is refused.
- **Codes:** `core/totp` generates them per RFC 6238 and is tested against its Appendix B vectors.
- **Desktop:** `GetOneTimeCode` returns the code with its expiry, and the item shows it with a countdown and asks for the next one when it expires. `CopyField("one-time code")` copies it. Item lists only carry `hasOneTimePassword`, never the secret.
- **CLI:** `hisar get <item> -f code`.
- **Browser extension:** `logins.code` returns only the code and its expiry, for a login that matches the page (§9.2). The extension fills two-factor fields and split code boxes from the inline menu, shows codes in the popup, and copies the code after filling a login that has one.
- **iOS:** the item shows the code with a countdown ring and copies it like other fields; the editor takes a setup key or link. The AutoFill extension offers one-time codes on iOS 18 and copies the code when it fills a password (§9.3).
- **Import:** one-time passwords from 1Password, Bitwarden and CSV exports become `details.oneTimePassword` (§9.7).

**Passkeys.** `details.passkeys` holds the item's WebAuthn credentials (§9.8): `[{credentialId, relyingPartyId, userHandle, userName, userDisplayName, privateKey, createdAt}]`. Binary values are base64 in the JSON; `privateKey` is the PKCS#8 DER of an ES256 (P-256) key.

- The overview carries only `passkeyRelyingPartyIds`, the sites of the item's passkeys, so lists can mark an item as having a passkey without its details.
- Editing an item never changes its passkeys: the backend copies them (and the overview marker) from the cached item, so editors that show passkeys read-only (desktop, iOS) or not at all keep them. Only "Delete passkey" removes one.

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

The client applies changes to its local SQLite cache and stores `nextSeq`. Changes arrive in two ways: server push, and polling (every 60 s and on focus) as a fallback.

**Push (server-sent events).** `GET /api/v1/events` (session required) is a `text/event-stream` that tells clients when to sync. It carries no vault data, only IDs and sequence numbers.

- `event: vault-changed` with `{"vaultID", "seq"}` goes to every member of a vault after an item is written (with the new `seq`) or the key is rotated (`seq` 0).
- `event: vaults-changed` with `{}` tells an account that its list of vaults changed: it was given or lost access, created a vault, was part of a vault before a rotation, or was recovered.
- The server sends a `: heartbeat` comment every 25 s and re-checks the stream's session each time, so logging out or revoking the device ends the stream.
- Each write pushes the connection's write deadline forward, so the server's 30 s response write timeout does not cut the stream.
- An account may hold 8 streams. A subscriber whose 32-event buffer fills is closed, so the client reconnects and resyncs.
- The broker lives in one server process, which is what a self-hosted deployment runs; several processes would need Postgres `LISTEN/NOTIFY`.
- The desktop backend keeps one stream open while unlocked. Its client has no overall request timeout, but closes a stream that sends nothing, not even a heartbeat, for 90 s. It reconnects with backoff from 1 s to 60 s, and stops on 401.
- On an event the backend syncs every vault incrementally (bursts collapse into one sync), so the SSH agent and the browser bridge see the change even when the window is hidden. It then emits `remote-change`, and the UI runs its background refresh.

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
- HTTP framework: Fiber v3 (fasthttp). It serves plain HTTP behind the reverse proxy; HTTP/2 and TLS are handled by the reverse proxy (nginx in the reference deployment).
- Served publicly over https behind nginx, which terminates TLS. The GitLab pipeline publishes the image for amd64 and arm64 to the container registry on every `v*` tag.
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
| `logins.match` | `url` | `logins`: `[{vaultID, itemID, title, username, vaultName, hasOneTimePassword}]`, sorted by title |
| `logins.fill` | `vaultID`, `itemID`, `url` | `username`, `password`, `hasOneTimePassword`; the desktop checks the URL match again |
| `logins.code` | `vaultID`, `itemID`, `url` | `code`, `expiresAt` (RFC 3339), `period`; the same URL match as `logins.fill`. The secret never leaves the desktop; `not-found` when the item has no one-time password |
| `logins.check` | `url`, `username`, `password` | `status` (`new`, `changed`, `unchanged`) and, unless `new`, `vaultID`, `itemID`, `title` |
| `logins.save` | `url`, `username`, `password`, optional `vaultID` + `itemID` | `vaultID`, `itemID`. Without an item, creates a login titled by the site's host in the Personal vault; with one, updates its username and password |
| `password.generate` | optional `length`, `digits`, `symbols` | `password` |

The desktop keeps paired browsers (`instanceID`, browser name, SHA-256 of the token, paired and last-used times) in its state file; Settings lists them with Remove. Signing out removes them.

**URL matching** (desktop, `desktop/browser`). Item URLs without a scheme are read as `https://`. A page matches an item URL when the ports match (default ports normalized) and the scheme is the same, or the item says `http` and the page is `https`. By default the hosts must share their registrable domain (eTLD+1 from the public suffix list, so `accounts.google.com` matches `google.com`, but two `github.io` sites do not); the Settings option "Only fill on the exact host" requires equal hosts. IP addresses and `localhost` always need equal hosts.

**Extension behaviour.**

- Popup: when not paired, the pairing flow; when locked, "Unlock in Hisar"; otherwise the logins matching the current tab with Fill, Copy username and Copy password, a search over them, and the password generator (Copy, Fill into the focused field). A login with a one-time password also shows its current code with a countdown ring and Copy; the popup asks `logins.code` again when the code expires. On a page with a one-time code field and no login form, Fill fills the code.
- Fill (popup, or the `fill-login` keyboard shortcut for the first match): the service worker asks the desktop with the frame's URL and fills only the top frame and frames with the top frame's origin; it never fills a frame that is cross-origin to the page, even when the item would match.
- Save: the content script records the username and password of a submitted login form and asks `logins.check`. For `new` or `changed` it shows an in-page bar (shadow DOM, top frame): Save / Update, Not now, Never for this site (stored per origin in `chrome.storage.local`).
- Permissions: `nativeMessaging`, `storage`, `scripting`, `activeTab`, `tabs`, `offscreen` and `clipboardWrite` (copying the one-time code after a fill, below), and host permissions for `<all_urls>` (the content script detects logins on every page). The extension stores only its `instanceID`, the token, the "never" list and the two inline menu settings.
- Clipboard: the popup copies with `navigator.clipboard.writeText`, which a focused extension page may do without a clipboard permission. A service worker has no clipboard, so the copy after a fill goes through an offscreen document (`offscreen.html`, reason `CLIPBOARD`) that copies with `execCommand("copy")`; that needs `clipboardWrite`, because no user gesture reaches the offscreen document. The offscreen page accepts copy messages only from the extension itself without a tab, so content scripts cannot use it. The extension does not clear the clipboard afterwards (the desktop app's own 90 s clearing of what it copies is unchanged).
- Frame rule in practice: a child frame counts as having the top frame's origin only when every frame between it and the top has that origin too (`location.ancestorOrigins`). The content script checks this again before filling.
- The service worker keeps one native port and reconnects on the next request after a disconnect, not on a timer, because each connection starts the host, and the host would relaunch Hisar after the user quit it.
- Inline menu: when a username or password field of a login form gets the focus, the content script puts a small Hisar icon inside the field's right edge, and opens a menu under the field on focus (setting "Open the menu when a login field is focused") or on a click on the icon or the down arrow.
  - Where it appears: only in frames that the frame rule could fill, never on hidden, tiny, read-only or disabled fields, and never on Hisar's own elements.
  - What it shows: up to 6 matching logins (title, username) and "More in Hisar…", which opens the popup. On the new-password fields of sign-up and change-password forms it shows "Use suggested password" (`password.generate`), which fills every new-password field. Otherwise it shows "Pair with Hisar", "Unlock Hisar" (`unlock`, then polls), "Open Hisar", or "No logins for this site".
  - The menu is an extension page (`inline-menu.html`) in an iframe inside a closed shadow root. Its URL comes from the service worker with the dynamic ID (`web_accessible_resources` with `use_dynamic_url`). Login titles and usernames therefore never enter the page's DOM.
  - The menu page talks to the service worker directly, under a per-field session that the first menu frame claims. The content script only positions the icon and the frame (fixed position, updated every animation frame, highest z-index) and fills the field the menu belongs to when the service worker asks. Fills take the same path as the popup's: the desktop checks the URL again, and the frame rule applies.
  - Clickjacking: before it asks the desktop for a password, the service worker asks the content script whether the menu has been visible and uncovered for 300 ms. The content script checks this on every frame: opacity, visibility, display and filters of the frame and its ancestors, its size and position, and `elementFromPoint` at the centre and corners. The menu page also ignores untrusted clicks and, where IntersectionObserver v2 exists, clicks before it has reported `isVisible` for 300 ms. That second check catches overlays with `pointer-events: none`.
  - Keyboard: the content script passes trusted Arrow and Enter key presses to the menu through the service worker; Enter is taken only after an arrow key selected an entry. Escape, typing in the field, a click outside, losing focus, the field leaving the viewport and `pagehide` close the menu.
  - One-time codes: a field gets the menu with "one-time code" purpose when it has `autocomplete="one-time-code"`, or its name, id, placeholder, aria-label, title or label says otp, totp, 2fa, mfa, one-time, two-factor, 2-step, authenticator, or an authentication, verification, security, SMS or login code (a bare "code", "token" or "pin" counts only with a maxlength of 4–10 or a numeric input mode). Coupon, promo, postal, phone, card and captcha fields never count. Four to eight visible one-character inputs (`maxlength=1`) close together with no other field among them are one split code when there are six of them, or they are numeric, or one of them is marked as a code; the icon then sits beside the last box and the menu spans the boxes. The menu lists only the matching logins with `hasOneTimePassword`; choosing one asks `logins.code` (after the same visibility check as passwords) and fills the field, or types one digit into each box, with input and change events. A code longer than the boxes is not filled.
  - After any fill of a login with `hasOneTimePassword` (popup, shortcut or inline menu), the service worker asks `logins.code` and copies the code to the clipboard, as 1Password does, so it can be pasted on the next page. A failed copy never fails the fill.
  - Settings in the popup: "Show Hisar in login fields" and "Open the menu when a login field is focused", both on by default and kept in `chrome.storage.local`.

### 9.3 Mobile — gomobile + native UI

- `gomobile bind` compiles `core/` into an iOS `.xcframework` and an Android `.aar`.
- Native UI: SwiftUI and Kotlin/Jetpack Compose.
- System autofill: iOS AutoFill Credential Provider extension, Android AutofillService. Both share the app's encrypted store through an app group / shared storage.
- Memory constraint: the iOS AutoFill extension must stay well below the system limit (Argon2 64 MiB).

**iOS (Phase 4, first).** `mobile/hisarkit` wraps the desktop backend (`desktop/vault`, which has no Wails import and compiles for `ios/arm64`), so the phone gets the same sign-in, offline cache, matching, sharing and recovery logic. `make build-ios-framework` runs `gomobile bind -target=ios,iossimulator -iosversion=17.0` and writes `mobile/build/HisarKit.xcframework` (Swift module `HisarKit`, classes prefixed `Hisarkit`).

- gomobile passes only strings, numbers, `[]byte` and interfaces, so lists and items cross as JSON strings with the desktop's types (`vault.Status`, `VaultSummary`, `ItemDetail`, `browser.Login`).
- `HisarkitNew(stateDirectory, deviceName, secretKeyStore, biometric, clipboard, listener)` keeps `hisar.json` and `hisar-cache.db` in `stateDirectory`, the app group container, so the app and the AutoFill extension share the state and the encrypted cache.
- **Methods:** `Status`, `SignIn`, `SignUp` (returns the status and the new Secret Key), `Unlock`, `UnlockWithBiometric`, `EnableBiometricUnlock`, `DisableBiometricUnlock`, `Lock`, `SignOut`, `ReportActivity`, `ListAllVault`, `ListAllItem` (no secrets), `GetItem`, `SaveItem` (item JSON in, item JSON out), `DeleteItem`, `CopyField`, `GetOneTimeCode`, `GeneratePassword`, `ListAllLoginForService`, `GetLoginForService`, `GetOneTimeCodeForService`, and for passkeys (§9.8) `ListAllPasskeyForRelyingParty`, `PreparePasskeyRegistration`, `CreatePasskey`, `AssertPasskey` and `DeletePasskey`. Item summaries and logins carry `hasOneTimePassword`, never the secret.
- **Interfaces implemented in Swift:**
  - `SecretKeyStore`: the Secret Key in the Keychain, in a keychain access group shared with the extension.
  - `Biometric`: Face ID or Touch ID. `StoreKey` writes the biometric unlock key to a Keychain item with `SecAccessControl` `.biometryCurrentSet` and `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`. `Authenticate` evaluates an `LAContext` that `LoadKey` passes as `kSecUseAuthenticationContext`, so the user sees one Face ID prompt.
  - `Clipboard`: `UIPasteboard`.
  - `EventListener`: auto-lock notification.
- **Biometrics after launch:** because iOS enforces the biometric check in the Keychain itself, `IsBiometricAllowedAfterLaunch` is on and Face ID can unlock after a launch without the master password first. The 14-day password interval still applies. Auto-lock is 5 minutes.
- **One-time codes:** `GetOneTimeCodeForService` checks the match as `GetLoginForService` does and returns only the code, expiry and period. When a password fill (list, QuickType or without user interaction) is for a login with `hasOneTimePassword`, the extension copies the current code to the pasteboard (`localOnly`, 90 s expiry) before completing the request.
- **AutoFill:** the extension receives `ASCredentialServiceIdentifier`s (a domain or a URL; bare domains are read as `https://`). `ListAllLoginForService` uses the same matching as the browser extension (§9.2), and `GetLoginForService` checks the match again before returning the password. Passkey requests carry a relying party ID instead, and only passkeys with exactly that RP ID are offered (§9.8).
- **Signing:** until the Apple Developer Program membership (§12 #7), the app runs on the developer's own iPhone with a free Apple ID (re-signed every 7 days). App groups and the AutoFill extension need the paid membership on a device, so the extension is developed and tested in the simulator until then.

**iOS app (`mobile/ios`).**

- **Project:** XcodeGen (`project.yml`, pinned 2.46.0) generates `Hisar.xcodeproj`, which is committed so Xcode opens it without XcodeGen; CI fails when the two differ. The team and the bundle identifier prefix live in `Configuration/Signing.xcconfig` (or an ignored `Local.xcconfig`). The extension (`.autofill`), the app group (`group.…`) and the keychain group (`$(AppIdentifierPrefix)….shared`) are derived from the prefix and reach the code through `Info.plist` keys.
- **Targets:** `Hisar` (app with the extension) and `HisarAutoFill` share `Shared/` as sources rather than a framework. HisarKit is a static framework, so each binary links its own copy of Go. `HisarPersonal` (scheme "Hisar Personal Team") is the same app for a free personal team: no extension, empty app group and keychain group, and its own bundle identifier (`….personal`) so the free team never registers the real one. With no app group the state goes to the app's Application Support folder, and Keychain items get no access group.
- **Threading:** Swift wraps `HisarkitHisar` behind a `VaultBackend` protocol. View models call it on a background queue, because Argon2 and network I/O block. Tests replace it with a fake.
- **Keychain:** the Secret Key is `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` in the shared access group. The Face ID key is `.biometryCurrentSet` + `WhenUnlockedThisDeviceOnly`; `Authenticate` keeps the evaluated `LAContext` and `LoadKey` passes it as `kSecUseAuthenticationContext`, so the user sees one prompt. The state directory is excluded from backups.
- **Clipboard:** copies are `localOnly` with a 90 s `expirationDate`. Go's own 90 s clearing asks `GetText`, which answers from `changeCount` instead of reading the pasteboard, so iOS never shows the paste banner for another app's text.
- **Lifecycle:** going to the background records the time; returning after 5 minutes (the auto-lock interval) locks. While the scene is inactive, a separate window at alert level covers everything, including sheets, so the app switcher snapshot shows only the logo. The lock screen asks for Face ID once per lock when it is ready.
- **AutoFill:** after every full sync and every save or delete, the app replaces the `ASCredentialIdentityStore` identities: host + username for each login with a URL, with record identifier `vaultID/itemID`, and on iOS 18 an `ASOneTimeCodeCredentialIdentity` (label: the username, or the title) for each such login with a one-time password, and an `ASPasskeyCredentialIdentity` (RP ID, user name, credential ID, user handle) for each passkey in a login. The extension declares `ProvidesPasswords`, `ProvidesPasskeys` and `ProvidesOneTimeCodes`; `prepareOneTimeCodeCredentialList` lists only matching logins with a code, and one-time code requests complete with `ASOneTimeCodeCredential` and never hand out the password. Every iOS 18 API is behind an availability check, so iOS 17 keeps password and passkey AutoFill. Without user interaction the extension answers only when its process is still unlocked; otherwise it asks for interaction and unlocks with Face ID or the master password. The list shows the logins matching each service identifier; search also covers every other login, which is filled only on an explicit tap. `hisarkit` calls `debug.FreeOSMemory` after every key derivation, so the extension drops back from about 126 MB at the Argon2 peak to about 68 MB once unlocked (simulator debug build).
- **Simulator limits:** a build without a team is registered as a provider but iOS does not offer it, so UI tests drive the extension's screens through a debug-only preview in the app. Face ID is tested in the simulator by posting the biometric daemon's enrollment and match notifications.

### 9.4 CLI

`hisar` CLI for development and scripting: login, unlock, list, get, generate, import (§9.7), and later `hisar run` to inject secrets into processes.

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
- Every request opens the **approval window**, like 1Password's: a small always-on-top window ("Hisar Access Requested", `?window=ssh-approval`, created hidden at launch and reused). It shows the app's icon linked to Hisar's, "Allow <app> to use SSH key", the key (expanding to the destination, program path and key fingerprint), "Approve for all applications", Deny and Authorize. `<app>` is the app bundle the program runs in, found by walking up the parent processes (`ssh` started in Terminal shows Terminal and its icon; `desktop/appinfo`), or the program name when there is none.
- On a Mac with Touch ID, the window starts Touch ID as soon as it shows a request: Apple's `LAAuthenticationView` is embedded in the window (`biometric.WindowPrompt`, placed over a slot the page measures), so touching the sensor approves without a click, as in 1Password. The reason is `allow "<app>" to use the SSH key "<title>" for <host>` (one Touch ID prompt at a time). If Touch ID fails, the window stays open with a message and Authorize starts it again. "Approve for all applications" can be changed while Touch ID waits; its value when Touch ID succeeds counts. Without Touch ID, Authorize approves. Deny, Escape and closing the window deny, and cancel a Touch ID check in progress.
- The backend emits `ssh-approval` (with a random request ID), `ssh-approval-failed` and `ssh-approval-closed`, and waits for `RespondSSHApproval(requestID, isAuthorized, isForAllApplication)`. After 60 seconds, or if the client disconnects, it denies; the window hides when no request is left. `ListAllPendingSSHApproval` lets the window show requests that arrived before it loaded.
- Approvals are remembered per (key fingerprint, program path), or per key for every program with "Approve for all applications", for one hour (`DefaultApprovalLifetime`) or until Hisar locks, whichever comes first; the vault backend calls the agent's `ClearAllApproval` from its lock hook. Requests through a forwarded agent always ask, and programs without a known path are only covered by an approval for all applications. "Ask every time" can be set per key (`SetAllAlwaysAskKeyID`; no UI yet).
- Agent forwarding (`ssh -A`) exposes the agent to the remote host. The per-use approval makes this visible ("through a forwarded agent"), and the docs recommend `ProxyJump` instead.

**Implementation**

- `core/sshagent` implements `agent.ExtendedAgent` over two interfaces: a key source and an approver. It has no UI dependency and is tested in CI with an in-memory approver and a real `x/crypto/ssh` client and server.
- `desktop/vault` stores SSH Key items (`GenerateSSHKey`, `ImportSSHKey`) and provides the key source, `vault.SSHKeySource`. It is a separate type from the bound `vault.Service`, so no method callable from the UI returns private keys.
- `desktop/agent` holds the approver (Touch ID or in-app, with the Wails event emitter injected), the `~/.ssh/config` editing and the bound status service. It does not import Wails and is unit-tested; `main.go` wires it to Touch ID, the window and the socket.
- The end-to-end test generates a key in a vault, serves it with the agent on a Unix socket and logs in to an in-process `x/crypto/ssh` server with a real SSH client; after Lock the agent lists no keys.
- Linux uses the same Unix socket; Windows (named pipe `\\.\pipe\openssh-ssh-agent`) comes later. The CLI gets `hisar ssh-key generate|import|list|public-key`.

### 9.7 Import from other password managers

`core/importer` turns an export into items. It is pure Go: it reads only the bytes it is given, never logs, and its errors carry positions (CSV line, JSON byte offset) but never values from the file.

| Format | Detected by | Imported | Not imported (reported with a reason) |
|---|---|---|---|
| 1Password `.1pux` (ZIP with `export.data`) | ZIP signature | Login and Password items with extra login fields and section fields (concealed fields stay concealed); Secure Notes; Server, Database, Wireless Router, Email Account, API Credential and Software License items as logins with their fields; SSH Keys with a private key | Credit cards, identities, documents and other personal records; archived items; attachments |
| 1Password CSV | header with `Title`, `OTPAuth` and `Archived` | title, URL, username, password, one-time password, tags, notes | archived rows |
| Bitwarden JSON (not encrypted) | JSON with `items` | logins with all URIs, notes, custom fields (hidden ones concealed, linked ones dropped), TOTP; secure notes; SSH keys; folder as tag | encrypted exports (the whole file is refused), cards, identities, passphrase-protected SSH keys |
| Bitwarden CSV | `login_username`/`login_password`/`login_uri` columns | as JSON, without hidden-field markers | rows of other types |
| CSV (Chrome, Edge, Firefox, Safari, LastPass and others) | a header row naming the columns | title (or the website's host), username, password, URL, notes, one-time password; other columns become fields. Comma, semicolon or tab separated; BOM, CRLF and quoted multiline values | rows with no content |

- Secure notes have no category of their own yet: they are logins with only a title and notes, and the preview counts them separately.
- An entry that repeats an earlier one exactly is reported, not imported twice. Items already in the target vault are matched by title, username and the first website (case and `www.`/scheme ignored; SSH keys by fingerprint) and skipped unless the user includes them.
- One-time passwords (1Password `totp` fields and `otpauth://` values, Bitwarden `login.totp`, a CSV `otpauth`/`totp`/`otp` column) go through `totp.Normalize` into `details.oneTimePassword` (§4.5). One that does not parse, such as a Steam code, or a second one on the same item, stays a concealed field.
- SSH private keys go through `sshagent.ImportPrivateKey` and are stored as OpenSSH keys, as in §9.6.
- Desktop (`desktop/vault/import.go`): the file comes from a native open dialog; the parsed items stay in memory only until the import finishes, is cancelled or Hisar locks. Items are saved in batches of 10 called by the UI, so it can show progress, and connection, permission and rate-limit errors pause the import instead of failing every remaining item. Afterwards the app offers to delete the export file.
- CLI: `hisar import <file> [-v vault] [-format f] [-y] [-include-duplicates]` parses the file before asking for the master password and asks before saving and before deleting the file; `-y` imports without questions and never deletes.

### 9.8 Passkeys (WebAuthn)

Hisar is a passkey provider for Chromium browsers: the extension takes over `navigator.credentials.create` and `get` for public-key credentials, and the desktop app holds the keys and signs. Private keys never leave the desktop's Go backend; neither the extension nor the UI sees them. On iOS 17+ the AutoFill extension is a passkey credential provider for Safari and apps, with the same authenticator and the passkeys already stored in login items; there the keys stay in the Go backend linked into the app and the extension (see **iOS** below).

**Authenticator**

| Property | Value |
|---|---|
| Algorithm | ES256 only (COSE `-7`, P-256). A `create` whose non-empty `pubKeyCredParams` lacks `-7` is left to the browser |
| AAGUID | `fb041679-042a-49ae-b616-8779e2ac14ce`, fixed for Hisar |
| Credential ID | 32 random bytes; every credential is discoverable (stored with its user handle) |
| Sign count | always 0, as for other synced passkeys; relying parties then skip the counter check |
| Flags | UP, UV, BE and BS; AT on create |
| Attestation | `none` (`fmt: "none"`, empty `attStmt`), whatever the relying party asks |
| Extensions | none processed; `credProps` reports `{rk: true}` when asked on create |
| Transports | `["hybrid", "internal"]`, attachment `platform` |

- **User presence and verification:** every create and get needs a click in Hisar's prompt in the page (presence), and Hisar must be unlocked, which took the master password or Touch ID (verification). So UV is set whatever `userVerification` asks. There is no extra Touch ID check per use: the SSH approval window (§9.6) would be the model, but it is built around per-process SSH approvals; "Ask for Touch ID before using a passkey" is a later option.
- `desktop/passkey` builds the authenticator data, the COSE key and the attestation object (CTAP2 canonical CBOR, hand-written for these few shapes) and signs with `crypto/ecdsa` (ASN.1 DER signature over `authenticatorData || SHA-256(clientDataJSON)`).

**Origin and relying party ID**

- The origin never comes from the page. The service worker takes it from the message sender: the calling frame's `sender.origin` (which must equal the origin of `sender.url`), and the top-level origin from `sender.tab.url`.
- Only `https:` origins, and `http://localhost` or `http://*.localhost`, as in browsers.
- The RP ID defaults to the origin's host. An explicit RP ID must equal the host or be a suffix of it at a label boundary, and must not be a public suffix (`golang.org/x/net/publicsuffix`, as for URL matching in §9.2); for IP addresses it must equal the host. The desktop checks this on every passkey request, so the extension never decides it alone.
- Frames: in the top frame and in frames with the top frame's origin (the frame rule of §9.2), `crossOrigin` is false. A cross-origin frame is served only when its permissions policy allows `publickey-credentials-get` (or `-create`), which the isolated content script checks with `document.featurePolicy.allowsFeature`; then `crossOrigin` is true and `topOrigin` is the tab's origin. Otherwise the browser decides.
- The desktop builds `clientDataJSON` as `{"type","challenge","origin","crossOrigin"[,"topOrigin"]}` in this order, with the challenge in base64url without padding.

**Creating a passkey**

- The prompt shows "Save passkey" for the user name and site, with the logins in writable vaults that match the origin (§9.2) and "New login". A login whose username equals the user name is preselected; with no matching login, "New login" is the only choice.
- A new login is created in the Personal vault, titled by the RP ID without `www.`, with the user name as its username and `https://<rpId>` as its URL.
- Saving to a login that already has a passkey with the same RP ID and user handle replaces it, as WebAuthn requires for discoverable credentials.
- `excludeCredentials`: when Hisar already has one of the listed credentials for the RP ID, the prompt says so; "OK" rejects with `InvalidStateError`, and "Use another device or browser" falls back. The desktop refuses such a create too.

**Signing in**

- The prompt lists Hisar's passkeys whose RP ID equals the request's (title, user name), only those in `allowCredentials` when it is not empty. Choosing one signs. With no matching passkey, Hisar steps aside and the browser handles the request without a Hisar prompt.
- **Conditional mediation** (`mediation: "conditional"`, passkey autofill): when Hisar is unlocked and has passkeys for the RP, the request stays pending, and focusing an input whose `autocomplete` contains `webauthn` opens the prompt, at the top right like the modal one so it does not cover the inline menu under the field. Otherwise the browser's own conditional request runs. Conditional requests have no timeout.

**Extension**

- `passkey-main.js`, a content script in the MAIN world at `document_start` in all frames, replaces `navigator.credentials.create` and `get`, `PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable` (true when the browser's is, or when this browser is paired with a signed-in Hisar) and `isConditionalMediationAvailable` (true). Requests without `publicKey` go to the browser untouched.
- It turns the options into JSON (BufferSources as base64url) and dispatches a `CustomEvent` with a JSON string on `document`. `passkey-content.js`, an isolated-world content script at `document_start`, forwards it to the service worker as `passkey.start` and answers with another event. Only strings cross between the worlds. A page that fakes these events gains nothing it could not get by calling `navigator.credentials` itself, and the origin still comes from the sender.
- The service worker validates the request (challenge 1–1024 bytes, user ID 1–64 bytes, at most 64 listed credentials of at most 1023 bytes, names at most 256 characters) and answers `fallback`, `prompt` (with a session and the prompt URL) or an error name.
- **Falls back to the browser without a prompt** when the native host or Hisar is unavailable, the browser is not paired, Hisar is signed out, the request is not for ES256 or asks for a `cross-platform` authenticator, Hisar has no matching passkey (get), or the desktop refuses the request (for example an RP ID that does not fit the origin, which the browser then rejects itself).
- **Locked:** the prompt shows "Unlock Hisar" (as the inline menu: `unlock`, then polling) and continues once Hisar is unlocked. Closing it while locked falls back to the browser.
- **Prompt:** an extension page (`passkey-prompt.html`, `web_accessible_resources` with `use_dynamic_url`) in an iframe in a closed shadow root at the top right of the calling frame. It uses the inline menu's style and its clickjacking defences (§9.2): before every action that reaches the desktop, the service worker asks the content script whether the prompt was visible and uncovered for 300 ms; the page ignores untrusted clicks, and uses IntersectionObserver v2 where available. It always offers "Use another device or browser", which calls the browser's original implementation with the same options. Closing it (×, Escape) rejects with `NotAllowedError`.
- **Timeouts and aborts:** the content script rejects with `NotAllowedError` after the request's `timeout` (clamped to 30 s–10 min, 5 min by default). An aborted `signal` closes the prompt and rejects with the signal's reason.
- **Result:** the MAIN-world script builds the credential with `PublicKeyCredential.prototype` (and `AuthenticatorAttestationResponse.prototype` or `AuthenticatorAssertionResponse.prototype`) as prototypes and own properties, so `instanceof` checks pass: `id`, `rawId`, `type`, `authenticatorAttachment`, `response` (`clientDataJSON`; `attestationObject`, `getAuthenticatorData()`, `getPublicKey()` as SPKI DER, `getPublicKeyAlgorithm()` and `getTransports()` for create; `authenticatorData`, `signature` and `userHandle` for get), `getClientExtensionResults()` and `toJSON()`.

**Messages** (extension ⇄ desktop, §9.2 framing, all need the token). Parameters are in a nested `passkey` object; binary values are base64url without padding.

| Type | `passkey` parameters | Result |
|---|---|---|
| `passkeys.list` | `origin`, `relyingPartyId`, `allowCredentials` | `passkeys`: `[{credentialId, vaultID, itemID, title, userName, userDisplayName}]` |
| `passkeys.prepare` | `origin`, `relyingPartyId`, `excludeCredentials` | `isExcluded`, `logins` (writable logins matching the origin) |
| `passkeys.create` | `origin`, `topOrigin`, `isCrossOrigin`, `relyingPartyId`, `userHandle`, `userName`, `userDisplayName`, `challenge`, `excludeCredentials`, optional `vaultID` + `itemID` | `credentialId`, `clientDataJSON`, `attestationObject`, `authenticatorData`, `publicKey`, `publicKeyAlgorithm`, `vaultID`, `itemID` |
| `passkeys.get` | `origin`, `topOrigin`, `isCrossOrigin`, `relyingPartyId`, `challenge`, `allowCredentials`, `credentialId` | `credentialId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle` |

The desktop checks the RP ID against the origin on each request, applies `allowCredentials` and `excludeCredentials` again, and signs only with a credential whose RP ID equals the request's.

**Desktop app and CLI**

- `desktop/vault/passkey.go` finds, creates and signs. These are unexported methods or plain functions, so nothing bound to the UI returns a private key. `ItemDetail.passkeys` carries `{credentialId, relyingPartyId, userName, userDisplayName, createdAt}`.
- The item view lists the passkeys (site, user name, created) with Delete (`DeletePasskey(vaultID, itemID, credentialId)`, after a confirmation). The editor shows them read-only and the backend keeps them.
- `hisar get` prints `passkey: <rpId> (<user name>)` per passkey, never the key.

**iOS (AutoFill credential provider, iOS 17+)**

- The extension declares `ProvidesPasskeys` next to `ProvidesPasswords` and `ProvidesOneTimeCodes`. iOS checks the site's or app's associated domain itself and hands the provider the RP ID and only `clientDataHash`, never `clientDataJSON` or the origin. Hisar signs over the hash it is given.
- **Same authenticator:** `desktop/passkey` has `CreateCredentialWithClientDataHash` and `SignAssertionWithClientDataHash` next to the browser functions, which build and hash their own `clientDataJSON` and then share the same code for the key, the authenticator data, the attestation object and the signature. AAGUID, flags, sign count 0, `none` attestation and ES256 are as in the table above. "none" attestation does not sign the hash, but a create still needs a 32-byte one.
- **RP ID check:** `passkey.ValidateRelyingPartyID`: a lower-case domain name of at most 253 characters, no IP address, port, path or wildcard, and not a public suffix (which also refuses single-label names such as `localhost`). There is no origin to compare it with; iOS has already bound it.
- **Go API** (`desktop/vault/platformpasskey.go`, plain functions, wrapped by `hisarkit`; binary values base64url): `ListAllPasskey` (every passkey in a login, with its user handle, for the identity store), `ListAllPasskeyForRelyingParty(rpId, allowedCredentialIDs)`, `PreparePasskeyRegistration(rpId, excludedCredentialIDs)` (`isExcluded` and the writable logins matching `https://<rpId>` by the §9.2 rules), `CreatePasskeyForRelyingParty({relyingPartyId, userName, userDisplayName, userHandle, clientDataHash, excludeCredentials, vaultID, itemID})` → `credentialId`, `attestationObject`, `authenticatorData`, `vaultID`, `itemID`, and `SignPasskeyForRelyingParty(rpId, credentialId, clientDataHash)` → `credentialId`, `authenticatorData`, `signature`, `userHandle`. The private key never crosses into Swift. Saving follows the desktop: the chosen login must be writable and match the RP, a passkey with the same RP ID and user handle in it is replaced, and with no login a new one is created in the Personal vault.
- **Identities:** with the password identities (§9.3) the app publishes an `ASPasskeyCredentialIdentity` (RP ID, user name, credential ID, user handle, record identifier `vaultID/itemID`) for each passkey after every sync, save, delete and passkey delete.
- **Signing in:** `prepareCredentialList(for:requestParameters:)` lists the passkeys for the request's RP ID (only those in `allowedCredentials` when it is not empty) above the matching logins. Choosing one signs and completes with `ASPasskeyAssertionCredential`. For a passkey chosen in the system sheet or QuickType bar, `provideCredentialWithoutUserInteraction` signs only while the extension's process is still unlocked; otherwise `prepareInterfaceToProvideCredential` unlocks with Face ID or the master password, then signs. As on the desktop, UV is set because Hisar is unlocked, and presence is the user's choice in the system UI.
- **Creating:** `prepareInterface(forPasskeyRegistration:)` unlocks if needed and shows "Save passkey" with the user name and the site, the writable logins matching the RP and "New login"; the login whose username equals the user name is preselected. A request whose non-empty `supportedAlgorithms` lacks ES256 is cancelled. On iOS 18 the request carries `excludedCredentials`: when Hisar has one of them, the screen says so and "OK" cancels with `ASExtensionError.matchedExcludedCredential`; Go refuses such a create too. iOS 17 has no exclude list. The result completes with `ASPasskeyRegistrationCredential`.
- **App:** the item view lists the passkeys (site, user name, created) with Delete after a confirmation, as on the desktop. The editor does not show them and the backend keeps them (§4.5).
- **Testing:** Go tests run both ceremonies from a `clientDataJSON` that go-webauthn builds and the test hashes, and verify them with go-webauthn. The simulator does not offer a build without a team as a provider, so the UI tests drive registration and sign-in through the debug-only AutoFill preview (§9.3) and check the attestation and the ES256 signature over `authenticatorData || clientDataHash` with CryptoKit. The real Safari flow needs the paid team (§12 #7) and is not verified yet.

**Not supported yet:** WebAuthn extensions (PRF, large blob, …), `getClientCapabilities`, the signal API, Firefox and Safari on the Mac, and on iOS the WebAuthn extension inputs of iOS 18 (`extensionInput`: PRF, large blob).

## 10. Repository layout

```
core/       Go module: crypto, item model, local store, sync client   (github.com/tryhisar/hisar/core)
server/     Go module: hisar-server                                   (github.com/tryhisar/hisar/server)
cli/        Go module: hisar CLI                                      (github.com/tryhisar/hisar/cli)
desktop/    Wails v3 app (Go + Vue): vault/ backend, agent/ SSH agent, passkey/ WebAuthn authenticator, frontend/ UI (Phase 2)
extension/  Browser extension (TypeScript)                            (Phase 3)
mobile/     iOS + Android apps on top of gomobile-bound core          (Phase 4)
docs/       Design and decisions
deploy/     Dockerfile, Docker Compose (Postgres + server), nginx site, deployment notes
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
