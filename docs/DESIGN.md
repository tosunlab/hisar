# Hisar — Design

Status: **Draft, Phase 0**. This document is the source of truth for the security model, data model and sync protocol. Change it before changing the code.

## 1. Goals and non-goals

**Goals**

- End-to-end encrypted password manager for developers, run as a hosted service (`api.tryhisar.com`): one server binary + Postgres.
- Person-to-person sharing by email: anyone can share a vault they own with anyone else, and nobody can list the accounts on the server.
- Zero-knowledge server: the server never sees the master password, the Secret Key, any decryption key or any plaintext item data.
- Offline-first clients: every client keeps a full encrypted copy and works with no network.
- Shared vaults between people. Account recovery only through the Emergency Kit (§6): a forgotten master password cannot be recovered by anyone.
- Clients: desktop (first), browser extension, mobile, CLI.
- One implementation of the crypto, written in Go, shared by all clients.

**Non-goals (for now)**

- Teams and organizations, billing, SSO/SCIM, enterprise policy.
- Open registration: sign-up stays invite-only (§5.1).
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
| Another user | Legit account | Sees only vaults shared with them, and only the people they share a vault with or who invited them (§5.8). Cannot learn whether an email has an account |

### 2.3 Accepted metadata leakage

The server sees: account emails, the emails people invite or share with, which users can access which vaults, number of items per vault, ciphertext sizes, timestamps, device list, IPs. Item titles, URLs and vault names are encrypted. For share links (§5.9) it sees who created a link, its size, expiry, view limit and views, whether it needs a passphrase, and the IP addresses that open it; never what was shared or the key.

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
| Share links only (§5.9), which a browser decrypts with WebCrypto | AES-256-GCM (12-byte random nonce), PBKDF2-HMAC-SHA256 for the optional passphrase | `crypto/aes`, `crypto/cipher`, `crypto/pbkdf2` |

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
- Removing a member rotates `K_vault` (`session.RemoveVaultMember`, `POST /vaults/{id}/rotate`, owner only):
  - The owner's client generates a new key, re-encrypts the vault name and every item, including tombstones, under a new revision, and wraps the new key for each remaining member. Each member's signed KEM key is verified, and the client's `MemberVerifier` hook (key pinning in the CLI and the desktop app) must accept it.
  - The server applies it in one transaction. It returns 409 unless the submitted items match the stored items and revisions exactly, so concurrent edits are never lost; the client then retries, up to 3 times. It replaces all access grants, gives every item a new seq so all devices resync, and drops the vault's item history, because that history is encrypted with the old key.
  - The rotating owner must keep owner access. Rotation requests may be up to 32 MiB; every other endpoint is limited to 2 MiB.
  - Copies the removed member downloaded before the rotation stay readable to them; that is unavoidable.

### 4.5 Items

An item has two encrypted parts so lists and search don't decrypt every secret:

- **overview**: title, category, URLs, tags, icon hint, subtitle (username).
- **details**: fields, passwords, notes, TOTP seeds, passkeys, history of field values.

**Categories.** `overview.category` is the item's type, set when the item is created and kept by every edit:

| Category | Holds | Shown as |
|---|---|---|
| `login`, or empty (items saved before categories) | username, password, websites, one-time password, passkeys, custom fields, notes | a login; the only category the browser extension and iOS AutoFill offer |
| `secure-note` | notes and custom fields; saving moves a username, password, website or one-time password into custom fields (Username, Password, Website, One-time password; secrets concealed), so a note never hides a secret its form does not show and nothing is lost | a notes-first view with Copy and a secret reference (`hisar://<vault>/<note>/notes`) |
| `credit-card` | the custom fields Cardholder, Type, Card number, Expiry (`MM/YY`), Security code, PIN and Billing address, first and in that order, then other custom fields and notes; the number, security code and PIN are concealed; saving, and importing, move login data into custom fields as for a secure note | the brand and last four digits ("Visa •••• 4242"), the number and codes masked until revealed; lists show only the brand and last four digits |
| `ssh-key` | §9.6 | §9.6 |

- **Creating:** desktop **+** → Secure note or Credit card, iOS **+** → New secure note or New credit card (or a vault's Add item), `hisar add --type secure-note|credit-card`. The backend refuses a category it does not know.
- **Credit cards** keep their values as named custom fields rather than a details section of their own, so an app that does not know cards shows them as fields and keeps them when it edits the item, and `hisar get -f "card number"` and `hisar://<vault>/<card>/security code` work as for any field. `core/item/card.go` holds the schema: `ReadCard`/`WriteCard` map a `Card` to the fields (names matched ignoring case), and `NormalizeCard` writes a known type the same way whatever its spelling ("amex", "mc"), fills an empty type in from the number, groups the number as printed on the card and writes the expiry as `MM/YY`; the desktop and iPhone editors apply the same formatting as the user types. `DescribeCard` names the brand from the number's first digits (Visa, Mastercard, American Express, Discover, Diners Club, JCB, UnionPay, Maestro, Troy) and warns about a failed Luhn check, an unreadable expiry or an expired card; the editors show the warnings, and a card saves anyway. The editors' type picker follows the number until the user picks a different type, and then hints when the two disagree. Bitwarden's `brand` and 1Password's card type are imported as the type. Card autofill in the extension is not part of this.
- **Converting:** a login with nothing but notes and custom fields (`item.HasOnlyNotes`), such as one imported before secure notes existed, shows "Convert to secure note" (`app.Service.ConvertToSecureNote`, driven by `ItemDetail.canConvertToSecureNote`). Nothing is converted without the user asking.
- **Older apps** show a secure note as a login with only notes and keep its category when they edit it.

```
aad      = "hisar/v1/item" || vaultID || itemID || revision || part      (part ∈ {"overview","details"})
ct_part  = XChaCha20-Poly1305(key=K_vault, nonce=random24, aad, plaintext=JSON(part))
```

**One-time passwords (TOTP).** `details.oneTimePassword` holds an `otpauth://totp` URI, in the encrypted details part.

- **Saving:** a pasted setup key or link is normalized into the URI (`core/totp`): base32 secret, SHA-1/256/512, 6–8 digits, a period of 1–300 s. Anything else is refused.
- **Codes:** `core/totp` generates them per RFC 6238 and is tested against its Appendix B vectors.
- **Desktop:** `GetOneTimeCode` returns the code with its expiry, and the item shows it with a countdown and asks for the next one when it expires. `CopyField("one-time code")` copies it. Item lists only carry `hasOneTimePassword`, never the secret.
- **CLI:** `hisar get <item> -f code`, or the secret reference `hisar://<vault>/<item>/otp` with `hisar read` and `hisar run` (§9.4.1).
- **Browser extension:** `logins.code` returns only the code and its expiry, for a login that matches the page (§9.2). The extension fills two-factor fields and split code boxes from the inline menu, shows codes in the popup, and copies the code after filling a login that has one.
- **iOS:** the item shows the code with a countdown ring and copies it like other fields; the editor takes a setup key or link. The AutoFill extension offers one-time codes on iOS 18 and copies the code when it fills a password (§9.3).
- **Import:** one-time passwords from 1Password, Bitwarden and CSV exports become `details.oneTimePassword` (§9.7).

**Passkeys.** `details.passkeys` holds the item's WebAuthn credentials (§9.8): `[{credentialId, relyingPartyId, userHandle, userName, userDisplayName, privateKey, createdAt}]`. Binary values are base64 in the JSON; `privateKey` is the PKCS#8 DER of an ES256 (P-256) key.

- The overview carries only `passkeyRelyingPartyIds`, the sites of the item's passkeys, so lists can mark an item as having a passkey without its details.
- The PRF secret of a passkey is not stored; it is derived from `privateKey` (§9.8), so the stored fields are the same with or without PRF.
- Editing an item never changes its passkeys: the backend copies them (and the overview marker) from the cached item, so editors that show passkeys read-only (desktop, iOS) or not at all keep them. Only "Delete passkey" and the WebAuthn signals (§9.8) remove one; the signals also update user names.

Binding vault ID, item ID and revision in the AAD stops a malicious server from moving an item into another vault, swapping overview/details between items, or mixing parts of different revisions.

All `||` concatenations of variable-length fields are length-prefixed (4-byte big-endian length before each field), so field boundaries are unambiguous.

### 4.6 Attachments

Each file gets a random `K_file`. The file is chunked (64 KiB chunks) and each chunk is encrypted with `K_file`. The chunk index and an is-last flag go into the AAD. `K_file` is stored inside the item's details, so it inherits the vault key.

## 5. Accounts and devices

### 5.1 Invites

Sign-up is invite-only: there is no open registration. An invite is an email address plus a one-time token (the server stores its SHA-256 hash in `invites`), valid for 7 days. The sign-up link is `https://<HISAR_PUBLIC_URL host>/invite/<token>` (`http` only for a loopback host, in development); the token alone is the invite code the apps accept.

- **Any signed-in user** invites a person by email with `POST /invites {email}`. The server emails the invite itself (§8.5); the response is always `202 {"message": "Invitation sent"}` and never contains the link or the code. When the email already has an account, nothing is created or sent and the answer is the same, so invites do not reveal who has an account. Invites from users always create `member` accounts.
- Without email configured on the server (§8.4), `POST /invites` answers `501 email is not configured on this server`.
- Rate limits: 10 invites per account and 20 per IP address per day.
- **The operator** creates invites with `hisar-server invite --email … [--role admin|member]`, which prints the link and needs no email; the first account is usually created this way, as an admin.

**Invite page.** `GET /invite/{code}` (the link in the email) is a page rendered by `hisar-server` from an embedded template, in the share link page's style (§5.9), light and dark with the system, and usable at phone width.

- **Valid invite:** who invited the person (the inviter's name and email, which the email already told them; operator invites name nobody), the address the invite is for, and when it expires. Then **Download for Mac** (`https://tryhisar.com/download`), **Open in Hisar** (`hisar://invite/<code>?server=<public origin>`), the invite code with **Copy**, and for the terminal how to get `hisar` (on a Mac it comes with the app: Settings → Developer tools; on Linux or a server, the download page) and `hisar signup <invite link>` with **Copy**. The copy buttons appear only with JavaScript; without it the code is still selectable text.
- **Used, expired or unknown code, or a malformed one:** one page, "This invite isn't valid", with the same status (404) and bytes, so the page tells nobody whether a code or an email exists. The lookup is one query whatever the state. Nothing on the page lists or looks up accounts.
- **Headers:** the share link page's set. The style and script are inlined and allowed by their SHA-256 hashes (`default-src 'none'; script-src 'sha256-…'; style-src 'sha256-…'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`); there are no inline style attributes or event handlers. `Referrer-Policy: no-referrer` keeps the code out of the download site's logs; `Cache-Control: no-store`, `X-Robots-Tag: noindex, nofollow`. Rendering escapes every value from the database, and the style and script are put in only around the rendered data, so a name can never become either.
- **Rate limit:** 30 page loads per IP address, refilled at one every 2 seconds (429 with a plain-text page), like the public share link reads. Codes are 256 random bits, so the limit protects the server, not the codes.
- **Opening the apps.** The Mac (`CFBundleURLTypes` in `Info.plist`, Wails' `ApplicationLaunchedWithUrl` event) and the iPhone (the `hisar` scheme it already registers for the Emergency Kit's `hisar://sign-in`) handle `hisar://invite/<code>?server=<origin>`: signed out, they open Create account with the invite filled in, as the bare code for `api.tryhisar.com` or as `<server>/invite/<code>` for another server, which the form then shows ("Server: hisar.example.com"). Signed in or locked, they say to sign out first instead. Universal links (`https://…/invite/…` opening the app directly) wait for the Apple Developer membership (§12 #7).

**Roles.** `accounts.role` (`admin` or `member`) stays in the data model, but no app feature depends on it: any user invites, shares and manages their own devices, and the apps neither show the role nor offer admin screens. The operator's `hisar-server` command (`invite`, `migrate`) on the server is the only admin surface.

### 5.2 Signup (new user, first device)

1. Client generates the Secret Key, salt and keyset, then derives AUK and `authKey`.
2. OPAQUE registration with `authKey`.
3. Client uploads salt, Argon2 params, `encKeyset`, public keys, signature.
4. Client creates a **Personal** vault. Share invitations sent to the email before the account existed (§5.8) show up in the app to accept.
5. Client generates the Emergency Kit PDF and requires the user to confirm it was saved.
6. On a Mac with Touch ID (an iPhone with Face ID), the next screen asks "Unlock with Touch ID?" with **Turn on Touch ID** and **Not now**. One click turns it on (§5.4) without asking for the master password again: sign-up keeps a copy of AUK in memory (`session.SignUpKeepingAccountUnlockKey`) until the offer is answered, the app locks or signs out, or onboarding ends (`ForgetNewAccountSecretKey`), and only the Touch ID check is asked.

The account form rates the master password with the same strength meter as the item editor (Weak, Fair, Good, Strong). It refuses fewer than 10 characters and passwords estimated below 28 bits (common words and patterns such as `password1234`), with "This password is too easy to guess. Add a few more words or characters."; a Weak password above that is allowed with a hint, because the Secret Key also protects the account. The server only checks the length.

### 5.3 Login on a new device

1. User enters email, Secret Key (or scans the Emergency Kit QR code) and master password. The server is `api.tryhisar.com` unless the user opens **Use a different server…**, which shows the Server field; it starts open when the saved or scanned server is another one.
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
- The upgrade keeps the UI and the `Biometric` interface (`core/app`); only the storage in `desktop/biometric` changes.

Common to both:

- The master password is still required after every app launch and at least every 14 days (`LastPasswordUnlockAt` in the state file).
- **In-window prompt**: like 1Password, the lock screen embeds Apple's `LAAuthenticationView` (`LocalAuthenticationEmbeddedUI`, macOS 12+) in Hisar's own window instead of the system dialog. The UI measures a slot on the lock screen and the app places the native view over it; "Use master password" cancels the evaluation (`LAContext.invalidate`).
- Touch ID also approves SSH agent requests (§9.6). Face ID and Android BiometricPrompt follow the same pattern in Phase 4.

### 5.5 Sessions

- After OPAQUE, the server issues an opaque random session token (256 bits). The server stores only its SHA-256 hash, together with the device ID and expiry (30 days, sliding).
- All API calls use `Authorization: Bearer <token>` over TLS.
- Devices can be listed and revoked by their owner.

### 5.6 Public key authenticity

Sharing a vault means wrapping `K_vault` to the recipient's KEM public key, which the server delivers. A malicious server could substitute its own key. Mitigations:

- Each user's KEM public key is signed with their Ed25519 key, bound to their account ID.
- Clients pin (trust on first use) every other member's Ed25519 key in their local state when they first share with them, which is when the owner's app completes an accepted share invitation (§5.8; `localstate.State.CheckMemberPin`, used by the CLI and the desktop app). Completing a share, changing a member's role and key rotation stop if a pinned key changes; the desktop app also marks that person in its lists.
- The UI shows a short security code per person (Settings → People and the vault's sharing panel) that the two people can optionally compare once, in person or over another channel: the first 8 bytes of SHA-256 of their Ed25519 public key as 4 groups of 4 uppercase hex digits, e.g. `3F2A 91C0 7B44 E1D8` (`crypt.FormatKeyFingerprint`).

### 5.7 Changing the master password and replacing the Secret Key

Settings → Security on the Mac and the iPhone has **Change master password…** and **Replace Secret Key…**; the CLI has `hisar account change-password` and `hisar account replace-secret-key [-o file]`. Both need the current master password and a connection; the server never sees either secret.

1. The client fetches the account (`GET /account`), derives the current AUK and `authKey` from the current master password and Secret Key, and decrypts the keyset; a wrong password fails here, before anything is sent. The keyset's public keys must match the account's and the open session's.
2. It picks a new random salt, the Argon2 parameters (the defaults, never below what the account already uses) and, for a replacement, a new Secret Key with the same account ID. It derives the new AUK and `authKey` and re-encrypts the **same** keyset with the new AUK (§4.3). `K_user`, the KEM and signing keys, every vault key and every item stay as they are, so shared vaults keep working and other people's pinned keys (§5.6) do not change.
3. **Re-authentication.** `POST /account/reauthenticate {ke1, registrationRequest}` (session; per-IP auth limit and 5 per account, refilled every 30 s) starts an OPAQUE login against the current record with the current `authKey` and, in the same call, answers the OPAQUE registration request for the new `authKey`. The server keeps the login state for one minute, bound to the session that started it and to a SHA-256 of the record it was checked against; it can never be finished on `/auth/login/finish`, so it never creates a session.
4. `PUT /account/credentials {reauthenticationId, ke3, change, record, salt, argon2, encKeyset}` (`change` is `master-password` or `secret-key`). A wrong proof answers 403 (never 401, which clients read as "signed out") and is audited as `reauthentication_failed`. In one transaction the server locks the account row, checks that the record is still the one the proof was made against (409 otherwise: changed on another device meanwhile) and that the session is still live, replaces the record, salt, parameters and `encKeyset`, revokes every other device and deletes their sessions. It closes their event streams at once, records `credentials_changed` and emails the account: "Your Hisar master password was changed" or "Your Hisar Secret Key was replaced", with the number of devices signed out and no secrets.
5. The client updates its cached account record; after a replacement it stores the new Secret Key in the keychain and keeps it in memory, like at sign-up (§5.2), until the new Emergency Kit is saved or Hisar locks. Touch ID / Face ID unlock is sealed again with the new AUK (§5.4), since the user just typed the master password.

**Other devices.** Every other device of the account is signed out by the change, so no device keeps working with the old secrets. The apps say so before the user confirms: "Your other devices will be signed out. Sign in on them with the new master password" (or "with your master password and the new Secret Key from the new Emergency Kit; the old Emergency Kit stops working"). A device learns it was signed out from a 401: its event stream ends at once, and an unlock or any request then gets 401. The desktop and iPhone backend then forgets the account on that device (`forgetLocalAccount`: the Secret Key and Touch ID key in the keychain, the encrypted cache, the session and pins in the state file) and shows the sign-in screen with "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one." A device that stays offline can still be unlocked from its cache with the old password until it next reaches the server; that is the limit of an offline-first client.

Replacing the Secret Key does not replace the keyset: someone who already holds the old Secret Key, the master password and a copy of the old `encKeyset` (a device cache) can still open the keyset. Rotating the keyset itself would mean re-wrapping every vault key and re-pinning with every person; it is not part of this.

### 5.7.1 Deleting an account

Mac Settings → Account → **Delete account…**, iPhone Settings → Account → **Delete account** (App Store guideline 5.1.1(v)) and `hisar account delete` (interactive; `--email` for scripts).

- **Before confirming** the app shows `GET /account/deletion-preview`: the vaults the user owns with no other owner, which are deleted with their items, and for each how many people lose access; the vaults with another owner (or owned by someone else), where only the user's access is removed and the vault stays; the live share links that stop working; the invitations sent and received that are withdrawn; and the devices that are signed out. Vault names are encrypted, so the server lists IDs and types and the client names them. The dialog recommends exporting first (§9.11): "Export your items first in Settings → Export." on the Mac, "… in Settings → Account → Export data." on the iPhone, and `hisar export` in the CLI.
- **Confirming** needs the master password, proved to the server with the same OPAQUE re-authentication as §5.7 (`POST /account/reauthenticate {ke1}`), and the account's email typed in. `POST /account/delete {reauthenticationId, ke3, email}`: a different email is 400, a wrong proof 403.
- **Server.** One transaction, after the same record and session checks as §5.7: delete the vaults the user owns alone (their items, history, access and share invitations cascade), the share invitations the user sent, accepted or that were addressed to their email, the sign-up invites addressed to the email or sent by the user and not yet used, the share links, sessions, devices, the remaining vault access (vaults with another owner) and the account. Migration `00006_account_deletion` drops the foreign keys from `vaults.created_by`, `vault_access.granted_by` and `items.updated_by` to `accounts`, so vaults that stay with another owner may name the deleted account there, as `item_history.updated_by` already could. Phone approvals of the account are dropped from memory and its event streams closed.
- **Afterwards** the people who shared a vault with the user get `vaults-changed`, and the owners and recipients of the withdrawn invitations `share-invitations-changed`, so their apps drop the vault or the member at once (there is no other notice). The audit log records `account_deleted` with the account ID only, and the account's email gets "Your Hisar account was deleted", which also says the encrypted backups are kept 30 days and the audit log up to a year (§8.6).
- No key rotation happens for the vaults that stay: the deleted account can no longer fetch anything, and its devices are wiped.
- The app forgets the account like a sign-out (§5.7) and returns to sign-in; the iPhone also removes its AutoFill identities.

### 5.8 Sharing a vault by email

Sharing is person to person: the owner of a shared vault types an email address and a role. Nobody can list the accounts on the server, and the server answers every share the same way, so sharing cannot be used to find out who has an account.

1. **Invite (owner).** `POST /vaults/{id}/share-invitations {email, role}` (an owner of a shared vault; personal vaults cannot be shared) stores a share invitation (`share_invitations`: vault, inviter, email, role, state, expiry after 7 days; one per vault and email, and a repeat replaces the role and expiry and sets it back to pending) and answers `202 {"message": "Invitation sent"}`. After answering, in the background:
   - when the email has an account, the server sends that account `share-invitations-changed` (§7.2) and an email saying who shared a vault with them (vault names are encrypted, so it says "a vault");
   - otherwise it creates a sign-up invite for the email (§5.1) and emails it, saying a vault is waiting. Without email configured nothing is sent; the invitation still waits for an account with that email.
   - Only errors that depend on the owner's own data are reported: the email is the owner's own (`400`), or belongs to someone who already has access to the vault (`409`; the owner sees the vault's members anyway). Rate limits: 50 share invitations per account and 100 per IP address per day.
2. **Accept or decline (recipient).** `GET /share-invitations` returns `received`: the pending, unexpired invitations to the caller's email, with the inviter's name and email, the role and the vault ID. `POST /share-invitations/{id}/accept` marks the invitation accepted by the caller's account; nothing secret is published. `POST /share-invitations/{id}/decline` deletes it. Both notify the vault's owners with `share-invitations-changed`. Until the share is completed the recipient has no access. `GET /share-invitations` also returns `waiting`: the invitations the caller accepted that no owner's app has completed yet, with the inviter's name and email, the role, the vault ID and `acceptedAt`, so the recipient's app can say who it is waiting for and since when.
   - **Telling the owner.** An owner whose app is unlocked and online completes the share within seconds (step 3). Otherwise, with email configured, the server emails the inviter 2 minutes after the accept (`Config.ShareAcceptedEmailDelay`) if the invitation is still accepted with the same `accepted_at` and the inviter is still an owner of the vault: "<Name> accepted your invitation in Hisar", saying to open and unlock Hisar to finish sharing. The server cannot read vault names, so the email says "a vault". It goes only to the inviter, who already knows the recipient's address, and at most 50 such emails per inviter per day; the timer lives in memory, so a restart drops it (the apps still show the banner below).
3. **Complete (owner's app).** `GET /share-invitations` also returns `sent`: the invitations of every vault the caller owns, with `state` (`pending` or `accepted`), and for accepted ones the recipient's account ID, name, email and signed public keys (only there: never for pending invitations or for vaults the caller does not own). On every sync, and at once on `share-invitations-changed`, the owner's app (`session.CompleteAllAcceptedShare`) verifies each accepted recipient's signed KEM key, pins their signing key (§5.6), wraps the vault key exactly as for any other member (§4.4) and calls `PUT /vaults/{id}/access/{accountID}`. The server accepts a new member there only with an accepted invitation for that vault and account, deletes the invitation in the same transaction and sends the recipient `vaults-changed`. The app then lists the person with their security code to compare.
   - `sent` entries carry `acceptedAt` once accepted. While the owner's app is **locked** it cannot complete anything (the vault key needs the keyset), but the session token is in the local state, so the lock screens on the Mac and the iPhone read `GET /share-invitations` (`ListAllShareAwaitingUnlock`; quietly nothing when offline or signed out) and show "Ada accepted your invitation to a vault — unlock to finish sharing" for each accepted invitation. The vault name is encrypted and unknown while locked, so it is not shown there. Unlocking completes the shares at once; if one still fails (a changed key, §5.6), the vault list's banner says "Ada accepted your invitation to Work. Hisar couldn't finish sharing" with the reason.
   - The recipient's vault list shows one banner per `waiting` entry: "Waiting for Ada's Hisar to finish sharing · accepted 2 h ago"; it disappears when `vaults-changed` brings the vault.
4. **Cancel (owner).** `DELETE /share-invitations/{id}` (an owner of the vault) withdraws a pending or accepted invitation.

`PUT /vaults/{id}/access/{accountID}` otherwise only changes the role of an existing member, and a key rotation (§4.4) can only keep existing members, so no vault is ever shared without the recipient accepting.

**People.** `GET /members` returns the caller and every account that shares at least one vault with them, with signed public keys; it is the member source for sharing, role changes, removal and rotation. There is no list of every account anywhere (API, desktop, iOS, CLI, extension); only the operator's `hisar-server` reads the database directly. The desktop's Settings → People shows those people with their security codes, the invitations the user sent (Cancel) and received (Accept, Decline), and the invite form; a banner above the vault list shows received invitations.

### 5.9 Share links

A share link hands one item, or a piece of text, to anyone, including people without a Hisar account: `https://<server>/s/<id>#<key>`. Browsers never send the fragment, so the server stores only ciphertext it cannot read. Links expire and can be limited to a number of views.

**What can be shared.** The item's title and the fields the user picks: username, password, websites, notes and custom fields; by default the username and the password (those the item has). Never an SSH private key, a passkey or a one-time password secret: `core/sharelink` has no name for them and never copies them into a payload. A text share has a title and one text field. Attachments are not supported.

**Crypto** (`core/crypt/sharelink.go`, `core/sharelink`). The recipient's browser decrypts with WebCrypto, which has neither XChaCha20-Poly1305 nor Argon2id, so share links use AES-256-GCM and PBKDF2 instead of the primitives of §3. Each content key encrypts exactly one message, so a random 96-bit nonce is safe.

```
linkKey       = 32 random bytes                       (the URL fragment, base64url without padding)
contentKey    = HKDF-SHA256(ikm = linkKey, salt = "", info = "hisar/v1/share-link/content", len = 32)
with a passphrase:
passphraseKey = PBKDF2-HMAC-SHA256(NFKD(passphrase), salt = random 16 bytes, iterations = 600 000, len = 32)
contentKey    = HKDF-SHA256(ikm = linkKey || passphraseKey, salt = "", info = "hisar/v1/share-link/content-with-passphrase", len = 32)
ciphertext    = 0x01 || nonce(12) || AES-256-GCM(contentKey, nonce, aad = "hisar/v1/share-link", JSON(payload))
payload       = {version: 1, title, fields: [{kind, label, value, isConcealed}]}
                kind ∈ {username, password, url, notes, custom, text}
```

- Without a passphrase the link alone opens it. With one, both are needed: a leaked link alone is not enough, and the server, which stores the salt but never sees the link key, cannot guess the passphrase offline. Someone holding the link and a copy of the server's row can guess the passphrase at the PBKDF2 cost, so the apps ask for at least 8 characters and say to send the passphrase another way.
- PBKDF2 rather than Argon2id: WebCrypto has PBKDF2 built in, while Argon2id would mean shipping a WebAssembly build inside the page. 600 000 iterations is OWASP's current advice for PBKDF2-HMAC-SHA256. The server hands out the salt and the iteration count, so the page and `core/sharelink` refuse fewer than 600 000 or more than 10 000 000 iterations, and a salt that is not 16 bytes.
- The JSON payload is at most 48 KiB, so the ciphertext stays below the server's 64 KiB.
- The creator's lists show what each link shares without the server learning it: the creating client also uploads a label, `XChaCha20-Poly1305(K_user, aad = "hisar/v1/share-link-label" || accountID, JSON{title, vaultID, itemID, fieldLabels, isText, linkKey})` (§4.3), which only the creator's devices open. It holds the link key, so **Copy link** works later on any of the creator's devices; the passphrase is never stored.

**Server.**

| Endpoint | Body → result |
|---|---|
| `POST /share-links` (session) | `{ciphertext, encryptedLabel, expiresInSeconds, maxViews, passphrase: {salt, iterations} or null}` → 201 `{id, createdAt, expiresAt, maxViews, viewCount, hasPassphrase}`. `expiresInSeconds` is 3600, 86400, 604800 or 2592000 (1 hour, 1 day, 7 days, 30 days); `maxViews` is 1–100, or 0 for unlimited; the ciphertext is 1 byte to 64 KiB and the label at most 4 KiB; 400 otherwise |
| `GET /share-links` (session) | the caller's live links, newest first: `[{id, encryptedLabel, createdAt, expiresAt, maxViews, viewCount, hasPassphrase}]` |
| `DELETE /share-links/{id}` (session) | revokes: 204 for the caller's own link, 404 for anything else |
| `GET /public/share-links/{id}` (no session) | counts one view → `{ciphertext, passphrase, expiresAt, maxViews, viewCount}`; the last allowed view also deletes the link. 404 `{"error": "not found"}` for an ID that never existed or is malformed, and for an expired, used-up or revoked link: the same status and body, so they cannot be told apart |

- IDs are 128 random bits chosen by the server, as 22 base64url characters.
- A view is counted by one `UPDATE share_links SET view_count = view_count + 1 WHERE id = $1 AND expires_at > now() AND (max_views = 0 OR view_count < max_views) RETURNING …`. Postgres re-checks the condition after taking the row lock, so concurrent opens never get more views than allowed. When that view was the last one, the same transaction deletes the row.
- An expired link is gone for readers at once; `hisar-server` deletes expired rows every 10 minutes. Revoking deletes the row.
- Table `share_links` (migration `00005_share_links`); deleting an account deletes its links.
- Rate limits: 100 new links per account per day and at most 500 live links per account (429); public reads 30 per IP address, refilled at one every 2 seconds (429). IDs cannot be guessed, so the read limit protects the server, not the links.
- The creator's event streams get `share-links-changed` (`{}`) when one of their links is created, viewed, used up or revoked, so lists refresh (§7.2).
- The audit log records creating and revoking a link, with its ID only.

**Recipient page.** `GET /s/{id}` serves one static page embedded in `hisar-server`, the same bytes for every ID: serving it looks nothing up, counts nothing and reveals nothing.

- No third-party code. Its script and style are inlined and allowed by their SHA-256 hashes: `Content-Security-Policy: default-src 'none'; script-src 'sha256-…'; style-src 'sha256-…'; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`, with `Referrer-Policy: no-referrer`, `Cache-Control: no-store`, `X-Robots-Tag: noindex, nofollow` and `X-Frame-Options: DENY`.
- **Reveal first.** The page reads the key from the fragment but fetches nothing until the recipient clicks **Reveal**, so link previews in chat apps, mail scanners and crawlers, which load the URL without clicking, never use up a view. Reveal fetches the ciphertext (one view), asks for the passphrase when the link has one, and decrypts in the browser with WebCrypto.
- A wrong passphrase keeps the ciphertext in the page, so another try costs no view ("That passphrase doesn't open this link."). A missing or malformed key says the link is incomplete; a 404 says "This link has expired or was already viewed", without saying which.
- It shows the title and each field with **Copy** (concealed fields hidden until **Show**), websites as text, notes as preformatted text, and when the link expires and how many views are left, or "That was the last view: this link no longer works". Light and dark follow the system; it works on phones, with the keyboard and with screen readers (labelled controls, live status messages).
- **Trust.** The page comes from the server that stores the ciphertext, so a compromised server could serve a page that sends the key elsewhere. That is the limit of every web-based share link; recipients with `hisar` can open links without the page (`hisar share-link open`, below), and opening them in the desktop and iPhone apps is a follow-up.
- The footer says "Shared with Hisar, the end-to-end encrypted password, SSH key and secrets manager for developers. Nothing you see here is stored by this page." without a link: there is no public product page to link to yet.

**Desktop app.**

- The item view has **Share link…** next to Edit (not for SSH keys without notes or fields). Its dialog offers the fields to share as checkboxes (username and password checked by default), **Expires** (1 hour, 1 day, 7 days, 30 days; 7 days by default), **Views** (1–100, 1 by default, or Unlimited) and an optional passphrase. **Create link** shows the link with **Copy** (through the backend, cleared after 90 s) and what it shares.
- The item view lists that item's live links. **Shared links** in the sidebar, under the vaults, lists all of them; the new item menu has **Share text…**. Each link shows its title, what it shares, views used, expiry and whether it needs a passphrase, with **Copy link** and **Revoke**.
- Lists refresh on `share-links-changed`. Offline, creating and revoking are refused like other changes.

**CLI.** `hisar share-link <item> [-v vault] [--field name]... [--expires 1h|1d|7d|30d] [--views 1-100|unlimited] [--passphrase]` prints the link on stdout (by default username and password, 7 days, 1 view); `--field` is `username`, `password`, `url`, `notes` or a custom field's label, and `--passphrase` asks for it twice. `hisar share-link --text [--title t]` shares text read from stdin. `hisar share-links` lists the live links and `hisar share-link revoke <id>` revokes one. `hisar share-link open <url>` opens any share link in the terminal, without an account and without the web page: it counts a view, asks for the passphrase when the link has one, decrypts with `core/sharelink` and prints the title and fields. Through the app, creating a link always opens the approval dialog, even in an approved terminal session, because it hands secrets to people outside Hisar: "hisar in Terminal wants to create a share link for “GitHub”", with the expiry, the views and the fields; approving it also approves the terminal session. Standalone, the commands ask for the master password like the others.

### 5.10 Renaming, deleting and leaving vaults

Owners rename and delete vaults; anyone who was given a vault can leave it. The server checks every rule again and never sees a vault name or an item.

**Rename (owner).** Vault names are encrypted with `K_vault` (§4.4), so the owner's client seals the new name (`crypt.SealVaultName`, `session.RenameVault`) and sends only the ciphertext: `PUT /vaults/{id}/name {encName}`, 204, 403 for anyone but an owner, 404 for a non-member. Every member gets `vaults-changed` and shows the new name on its next sync. A Personal vault can be renamed too. Secret references (§9.4.1) that name the old name stop resolving, which the CLI says after a rename.

**Delete (owner).** `POST /vaults/{id}/delete {moveToVaultId, movedItems, deleteItems}`; one transaction locks the vault (and the target, in ID order) and checks, in this order:

- the caller is an owner (403), the vault is not a Personal vault (400 "personal vaults can't be deleted": it is the vault Hisar keeps for the user alone, and logins saved from the browser and the iPhone go there), and it is not the caller's only vault (400);
- with `moveToVaultId`: the target is another vault the caller can write to (owner or editor; 403 otherwise, 404 for a non-member), and `movedItems` are exactly the vault's live items at their current revisions (409 otherwise, so a concurrent edit is never lost; the client re-reads and retries up to 3 times). Each moved item keeps its item ID, gets a new revision and the target's next `seq`, and a history entry in the target. The client (`session.DeleteVault`) decrypts each item with the source vault's key and seals it again with the target's key, vault ID and the new revision in the AAD (§4.5), so the server only copies ciphertext;
- without a move, a vault with live items needs `deleteItems: true` (409 otherwise). The apps only send it after the user typed the vault's name.

Then it deletes the vault row; access grants, wrapped keys, items, tombstones, item history and share invitations go with it (`ON DELETE CASCADE`). It answers 204, records `vault_deleted`, sends `vaults-changed` to every former member and `vault-changed` to the target's members. Requests may be as large as a key rotation (32 MiB, at most 50 000 items).

**Leave (member).** `DELETE /vaults/{id}/access/{own account ID}` removes the caller's access and wrapped key (`store.LeaveVault`, one transaction). An owner may leave only while another owner remains (400 otherwise: "delete it, or make someone else an owner first"); a Personal vault has one owner and cannot be left. Leaving does not rotate the vault key: only owners can rotate, and the items the leaver already downloaded stay readable to them anyway, as after a removal (§4.4). The remaining members get `vaults-changed`; the audit log records `vault_left`.

**Who loses access.** Before deleting a shared vault the apps list its other members with their roles (`GetVaultActions`; never anyone else, §5.8). Their apps drop the vault on their next sync: listing vaults replaces the cached list (§9.1), and `session.Session` compares it with the cached one first. A vault that disappeared is reported once, with its name decrypted from the cache while the cached wrapped key still opens it (`TakeAllRemovedVault`), except the vaults this session deleted or left itself. The Mac and the iPhone show "“Work” is no longer available: it was deleted, or you no longer have access to it." above the vault list; the server does not say which, so neither does the notice. A device without a cache of that vault (signed in later) shows nothing.

**Apps.** `core/app` has `GetVaultActions` (whether the user may rename, delete or leave, the reason when not, the members who lose access and the vaults its items can move to), `RenameVault`, `DeleteVault` (move, or delete with items after the typed name; the backend checks the name again), `LeaveVault` and `ListRemovedVaults`, used by the Mac, the iPhone (through `mobile/hisarkit`) and `hisar` (§9.4). Deleting or leaving also turns the vault's "Available to agents" switch off (§9.4.2).

**CLI.** `hisar vault rename <vault> [name]`, `hisar vault delete <vault> [--move-to <vault> | --delete-items --confirm <vault name>] [-y]`, `hisar vault leave <vault> [-y]` and `hisar vault create <name>` (`vault-create` stays). In a terminal they ask what is missing: the new name; for a delete, the item count, "These people lose access:" with each member's name, email and role, then "What happens to its items?" with one "Move them to <vault>" line per writable vault (1, the default) and "Delete them with the vault", which asks "Type Work to delete it and its N items"; a move or an empty vault asks "(y/N)". Without a terminal nothing is asked: a vault with items needs `--move-to` or `--delete-items --confirm <name>`, and a move, an empty vault or a leave needs `-y`. A name typed or passed that is not the vault's exact name deletes nothing. Through the app the commands use `vaults.actions`, `vaults.rename`, `vaults.delete` and `vaults.leave` (§9.4); standalone they run the same session code.

## 6. Account recovery

Recovery is only through the Emergency Kit (§4.1). With the email, server and Secret Key printed on it, plus their master password, a user signs in on any device (§5.3). There is no other path: no admin recovery, no recovery links, no trusted contacts.

- **Lost Secret Key:** it is on the Emergency Kit, and on every device still signed in, where the desktop app saves a new Emergency Kit after the master password (Settings → Security → Emergency Kit) and the CLI has `hisar emergency-kit`. A Secret Key that may have leaked is replaced from a signed-in device (§5.7).
- **Forgotten master password:** it cannot be recovered by anyone, including Hisar. The server never sees it, and nothing the server or a device stores opens the keyset without it, so the data only that account can open is lost; shared vaults stay with their other members. The apps say so plainly when the account is created and on the Secret Key / Emergency Kit screen.
- Losing the Emergency Kit and every signed-in device loses the account as well.

**Admin recovery was removed.** Earlier versions let one admin hold a recovery keyset that every recoverable vault key was wrapped to, and recover other users' accounts with one-time links. Migration `00004_remove_recovery` deletes all of it: the tables `recovery_keyset`, `vault_recovery`, `recovery_tokens` and `account_recovery` and the column `vaults.recovery_opt_out`. Its down migration recreates only the empty structure; the deleted keyset, wrappings and pending recoveries cannot come back. The recovery endpoints are gone and answer 404. Older clients keep working: a 404 for the recovery keyset means "recovery is not set up" to them, so they stop wrapping vault keys to it.

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
- `event: vaults-changed` with `{}` tells an account that its list of vaults changed: it was given or lost access, created a vault, or was part of a vault before a rotation, a rename or a deletion, or that someone left it (§5.10).
- `event: share-invitations-changed` with `{}` tells an account that its received or sent share invitations changed (§5.8): one was sent to it or withdrawn, or one it can complete was accepted or declined. The desktop backend then completes accepted shares and refreshes the UI.
- `event: share-links-changed` with `{}` tells an account that one of its share links was created, viewed, used up or revoked (§5.9).
- `event: approval-requested`, `approval-answered` and `approval-resolved` with `{"requestId"}` go to every stream of the account for phone approvals (§9.9). They carry no request or answer, only the ID.
- The server sends a `: heartbeat` comment every 25 s and re-checks the stream's session each time, so logging out or revoking the device ends the stream.
- Each write pushes the connection's write deadline forward, so the server's 30 s response write timeout does not cut the stream.
- An account may hold 8 streams. A subscriber whose 32-event buffer fills is closed, so the client reconnects and resyncs.
- The broker lives in one server process, which is what the deployment runs; several processes would need Postgres `LISTEN/NOTIFY`.
- The desktop backend keeps one stream open while unlocked. Its client has no overall request timeout, but closes a stream that sends nothing, not even a heartbeat, for 90 s. It reconnects with backoff from 1 s to 60 s, and stops on 401.
- The iOS app runs the same backend. iOS suspends it in the background, after which the old stream can be dead without the client noticing for up to 90 s, and the backoff may have grown to 60 s. So on returning to the foreground the app calls `ReconnectEvents`, which drops the stream and opens a new one at once with the backoff reset.
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
- A server can still hide recent writes from other devices. This is accepted because we run the server.

## 8. Server

### 8.1 Data model (Postgres)

```
accounts       (id, email, name, role[admin|member], state, argon2_params, salt, opaque_record,
                enc_keyset, x25519_pub, ed25519_pub, x25519_sig, created_at)
devices        (id, account_id, name, platform, created_at, last_seen_at, revoked_at)
sessions       (token_hash, account_id, device_id, expires_at)
invites        (token_hash, email, role, created_by, created_at, expires_at, used_at)
share_links    (id, account_id, ciphertext, encrypted_label, passphrase_salt, passphrase_iterations,
                max_views, view_count, created_at, expires_at)
share_invitations (id, vault_id, email, role, invited_by, state[pending|accepted], accepted_by,
                created_at, expires_at, accepted_at)   -- unique (vault_id, email)
vaults         (id, type[personal|shared], enc_name, seq, created_at)
vault_access   (vault_id, account_id, wrapped_key, role[owner|editor|reader], granted_by)
items          (vault_id, id, revision, seq, deleted, overview_ct, details_ct, updated_at, updated_by)
item_history   (vault_id, id, revision, overview_ct, details_ct, updated_at, updated_by)
attachments    (id, vault_id, item_id, size, chunk_count, storage_path)
audit_events   (id, account_id, device_id, type, detail, ip, created_at)   -- kept 1 year (§8.6)
```

`enc_name` is the vault name encrypted with `K_vault`.

Phone approvals (§9.9) are not in Postgres: they live in the server process's memory for at most 4 minutes, like the event broker.

### 8.2 API surface (v1)

| Area | Endpoints |
|---|---|
| Auth | `POST /auth/register/{start,finish}`, `POST /auth/login/{start,finish}`, `POST /auth/logout`, `GET /auth/prelogin?email=` (salt, params) |
| Account | `GET /account`, `POST /account/reauthenticate`, `PUT /account/credentials` (§5.7), `GET /account/deletion-preview`, `POST /account/delete` (§5.7.1) |
| Devices | `GET /devices`, `DELETE /devices/{id}` |
| People | `GET /members` (the caller and the people they share a vault with; public keys, signatures), `POST /invites` (any user; emailed) |
| Share invitations | `POST /vaults/{id}/share-invitations` (owner), `GET /share-invitations` (`received`, `sent`, `waiting`), `POST /share-invitations/{id}/accept`, `POST /share-invitations/{id}/decline` (recipient), `DELETE /share-invitations/{id}` (owner) |
| Vaults | `GET /vaults`, `POST /vaults`, `GET /vaults/{id}/members`, `PUT /vaults/{id}/access/{accountID}`, `DELETE /vaults/{id}/access/{accountID}` (an owner removes someone, or a member leaves with their own ID), `POST /vaults/{id}/rotate` (Phase 2), `PUT /vaults/{id}/name` and `POST /vaults/{id}/delete` (owner, §5.10) |
| Items | `GET /vaults/{id}/changes`, `PUT /vaults/{id}/items/{itemID}`, `GET /vaults/{id}/items/{itemID}/history` |
| Attachments | `PUT/GET /vaults/{id}/attachments/{attID}/chunks/{n}` |
| Share links (§5.9) | `POST /share-links`, `GET /share-links`, `DELETE /share-links/{id}`; without a session `GET /public/share-links/{id}` (counts a view). The recipient page is `GET /s/{id}`, outside `/api/v1` |
| Approvals (§9.9) | `POST /approvals`, `GET /approvals` (pending), `GET /approvals/{id}`, `POST /approvals/{id}/answer`, `DELETE /approvals/{id}` (resolve; the requesting device only) |
| Desktop updates | `GET /desktop/updates/appcast.xml` (Sparkle appcast), `GET /desktop/updates/{version}/{file}` (streams a release file); any signed-in session, 404 when desktop updates are off (§9.1.1) |

| App versions (§8.7) | `GET /version-check` (no session): 204 when the server accepts the caller's version, 426 otherwise |

All under `/api/v1`. JSON bodies, base64url for binary fields. There is no recovery API (§6) and no admin API. Outside `/api/v1` the server serves the pages `GET /s/{id}` (§5.9), `GET /invite/{code}` (§5.1), `GET /privacy` and `GET /terms` (§8.6).

### 8.3 Hardening

- Rate limiting per IP and per email on `prelogin` and `login` endpoints. `prelogin` returns a deterministic fake salt for unknown emails so it doesn't reveal which accounts exist.
- No account enumeration: `POST /invites` and `POST /vaults/{id}/share-invitations` answer the same whether or not the email has an account, and do the account-dependent work (events, invites, email) after answering. Both are rate-limited per account and per IP (§5.1, §5.8).
- Request size limits. Strict JSON decoding (unknown fields rejected).
- Security headers on the invite page and the share link page (CSP, HSTS, no framing, §5.1, §5.9).
- Audit log for logins, device changes, invites, share invitations, sharing changes, credential changes, failed re-authentications, account deletions, and renaming, deleting and leaving vaults (`vault_renamed`, `vault_deleted` with the member count and the target vault and item count of a move, `vault_left`; IDs only), kept 1 year (§8.6).

### 8.4 Deployment

- Single static binary `hisar-server` in a Docker image, with Postgres 17 alongside it in Docker Compose.
- Schema migrations: goose (`pressly/goose`), SQL files embedded in the binary, up and down sections, applied on start under a Postgres advisory lock or explicitly with `hisar-server migrate up|down|status`.
- HTTP framework: Fiber v3 (fasthttp). It serves plain HTTP behind the reverse proxy; HTTP/2 and TLS are handled by the reverse proxy (nginx in the reference deployment).
- Served publicly over https behind nginx, which terminates TLS. The GitHub Actions workflow "Release server image" builds `deploy/Dockerfile` for amd64 and arm64 and pushes it to the GitHub Container Registry as `ghcr.io/tosunlab/hisar-server:<version>` and `:sha-<commit>` on every `server-<version>` tag; the manual Publish workflow then points `:latest` and `:<major>.<minor>` at it. A custom build (`server-1.2.3-label`) can't be published.
- Attachments on local disk (later: S3-compatible storage).
- Optional email (§8.5): `HISAR_SMTP_HOST`, `HISAR_SMTP_PORT`, `HISAR_SMTP_USERNAME`, `HISAR_SMTP_PASSWORD` and `HISAR_SMTP_FROM`, all or none; the server refuses to start with only some of them.
- Desktop update source (§9.1.1): `HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY` (default `tosunlab/hisarapp-core`; `off` turns updates off and the update endpoints answer 404), the optional `HISAR_DESKTOP_UPDATE_GITHUB_TOKEN` (a fine-grained token with read-only Contents access to that repository, needed while it is private) and `HISAR_DESKTOP_UPDATE_GITHUB_API_URL` (default `https://api.github.com`, for GitHub Enterprise and tests).
- Optional minimum app versions (§8.7): `HISAR_MINIMUM_VERSION_DESKTOP_MACOS`, `HISAR_MINIMUM_VERSION_IOS`, `HISAR_MINIMUM_VERSION_CLI` and `HISAR_MINIMUM_VERSION_EXTENSION`, each a semantic version such as `0.1.6`; empty means no minimum. The server refuses to start with a value that is not a version.
- Nightly `pg_dump` + attachments encrypted with `age` and copied off-server. Restore tested quarterly.

### 8.5 Email

`server/internal/mail` has a small `Mailer` interface with an SMTP implementation and a recording fake for tests.

- **Transport.** Port 465 uses implicit TLS; any other port must offer STARTTLS, which is used before authenticating. Plaintext is refused, so credentials never cross the network unencrypted; the only exception is a loopback host, which the end-to-end test's SMTP sink uses (`server/cmd/hisar-smtp-sink`). Authentication is PLAIN over TLS. Each message has a 30 s deadline.
- **Messages.** Plain text in UTF-8, no HTML, no images and no tracking; short and written for developers. Names from users are stripped of control characters before they go into a header.
  - Invite: who invited them, the sign-up link (the invite page, §5.1: "Get Hisar and create your account") and the invite code ("Or paste this invite code under Create account in Hisar"), the expiry, and that the email can be ignored.
  - Share invitation to an account: "<Name> shared a vault with you in Hisar", the role, and where to accept (Settings → People, `hisar invitations`).
  - Share invitation without an account: the same, plus the sign-up link and code.
  - Share accepted, to the inviter (§5.8, sent only if their app has not completed it after 2 minutes): "<Name> accepted your invitation in Hisar", that the server cannot say which vault, and to open and unlock Hisar on a device to finish sharing.
  - Master password changed or Secret Key replaced (§5.7), to the account: when, how many other devices were signed out and how to sign in on them, and to contact support if it was not them.
  - Account deleted (§5.7.1), to the deleted address: what was deleted, what stays with other owners, and the backup and audit log retention.
- Sending happens in the background after the API answered; failures are logged without the recipient's address.

### 8.6 Privacy policy and Terms

`GET /privacy` and `GET /terms` serve two static pages embedded in `hisar-server` (`privacy.html`, `terms.html`), linked from each other, from the store listings and from the desktop app's create-account screen. They describe the hosted service at `api.tryhisar.com` exactly as this document does: what the server stores (§2.3, §8.1), what it cannot read, the sub-processors (Hetzner Online GmbH in Helsinki, Finland, for hosting; Google for email) and the retention below. The operator is Alper Berber, the contact is support@tryhisar.com, and the terms are under Dutch law. Both pages get the share link page's headers minus scripts: `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'`, cacheable for an hour.

**Retention**, as the pages state it:

- Account data, devices, vaults, items and invitations: while the account exists. Users delete their account in the apps or the CLI (§5.7.1), or by emailing support. Item history: 90 days. Share links: deleted at expiry, after the last view or when revoked (§5.9). A share invitation: until the share is completed, declined or cancelled. Sign-up invites and pending share invitations that were not used: `hisar-server` deletes them once a day when they expired more than 30 days ago (accepted share invitations do not expire; they wait for the owner's app).
- Audit log (`audit_events`: account, device, event type, IP address and details: the device's name and platform for a login, the email for a sign-up, an invite or a share invitation, otherwise IDs only): 1 year; `hisar-server` deletes older rows once a day.
- Rate-limit counters, the event broker and phone approvals: server memory only, never written to disk, gone on restart (approvals after at most 4 minutes).
- The reverse proxy's access logs: 14 days. The encrypted backups (§8.4): 30 days.

### 8.7 Minimum app versions

The operator can require a minimum version per app, mostly for API compatibility: an app below it must update before it can be used. The minimum is independent of the latest version the update feed offers (§9.1.1); raising it is an explicit, rare decision.

- **Headers.** Every app sends `Hisar-Platform` and `Hisar-Version` with every request, the event stream included. The platform is the one the device registers with: `darwin/desktop` (Mac app), `ios/mobile` (iPhone app), `<os>/cli` (`hisar`, any OS); the desktop app checks the browser extension and an app-mode `hisar` on their behalf with `browser/extension` and `<os>/cli` (below). Development builds (`development`, or no bundle version) send neither header.
- **Middleware.** `requireMinimumVersion` runs first on every `/api/v1` request, before rate limits, sessions and the handlers, so sign-in, sign-up and `GET /version-check` answer it too and a signed-out app knows. When the platform has a minimum and the version is below it, the answer is `426 Upgrade Required` with `{"error": "update required", "minimumVersion", "updateURL"}`; `updateURL` is `https://tryhisar.com/download` for the Mac, iPhone and `hisar`, and the Chrome Web Store page for the extension.
- **What passes:** requests without either header, from platforms without a minimum (Linux desktop, other callers), and versions that do not parse. So very old apps from before the headers, scripts and other callers keep working; only an app that says it is too old is refused. `/api/v1/desktop/updates/…` always passes, so a refused Mac app can still update itself through Sparkle.
- **Versions** are semantic versions compared by `core/appversion` (also the updater's parser, `desktopupdate.GetBuildNumber`): a pre-release comes before its release (`1.0.0-alpha-test` < `1.0.0`; a custom build `1.2.3-label` is below a minimum of `1.2.3`), build metadata is ignored.
- **Clients.** `core/apiclient` sends the headers when `Client.Version` is set and turns a 426 into `*apiclient.UpgradeRequiredError` (not "unreachable", so the offline cache never hides it), calling `Client.OnUpgradeRequired`.
  - **Mac app:** the backend records the requirement from any answer (`Status.updateRequired`: current version, minimum, update URL), stops the event stream and emits `update-required`, which shows the window. The app also asks `GET /version-check` at launch. The UI then shows only a blocking "Update required" screen: "This server needs Hisar 0.2.0 or later. This Mac has 0.1.0.", **Update now** (Sparkle when it is available and signed in, otherwise the download page) and **Check again** (any answer but 426 clears the requirement, e.g. after the operator lowered the minimum). Meanwhile the SSH agent (signing and listing keys, which also shows the screen), the command-line socket (every request but `hello`, coding agents included) and the browser bridge (every paired request) answer at once with "Hisar needs an update: the server requires version 0.2.0 or later…" instead of waiting.
  - **iPhone app:** the same requirement through `hisarkit` (`SetAppVersion` right after `New`, `CheckMinimumVersion`, `EventListener.OnUpdateRequired`), checked at launch and on returning to the foreground; a blocking "Update required" screen with **Install the latest Hisar** (the download page until TestFlight or the App Store exist) and **Check again**. The AutoFill extension says Hisar needs an update instead of offering logins.
  - **hisar:** standalone, a 426 prints `hisar: update required: the Hisar server needs hisar 1.0.0 or later, and this is 0.9.0.` with where to download it, and exits with 1. In app mode it sends `clientPlatform` and `clientVersion` with every request; the app refuses with `update-required` when the app itself needs an update or when the server's `GET /version-check` refuses that `hisar` (asked by the app, cached for an hour).
  - **Browser extension:** `hello` carries `extensionVersion`; the app answers `updateRequired: {target: "app" | "extension", currentVersion, minimumVersion, updateURL}` (the extension's version is checked like `hisar`'s). The popup shows "Hisar needs an update" with **Open Hisar**, or "Update the Hisar extension" with **Update extension**; while the extension is too old it refuses fills itself, and the inline menu points to the popup.
- **Offline:** without an answer from the server nothing is refused and the cache keeps working; the check applies on the next contact. A requirement already known stays until the server accepts the version again or the app restarts (a restart checks again at launch).

## 9. Clients

All clients use `core/` for crypto, the local store and sync.

### 9.1 Desktop — Wails v3 (Go + Vue/TypeScript)

- The Go side owns the unlocked keyset and vault keys (in `memguard` locked buffers). The web UI calls Go methods and receives only the decrypted data it asks for.
- Local store: SQLite (`modernc.org/sqlite`, pure Go) holding the server's ciphertext as-is plus sync state. Nothing is stored decrypted on disk.
  - `core/cache` stores the account record (salt, Argon2 params, `encKeyset`, public keys), the vault list as the server returns it (wrapped keys, encrypted names), every live item's encrypted parts, and each vault's `seq`. The file is `desktop-cache.db` next to `desktop.json`, mode 0600, and is deleted on sign-in, sign-up and sign-out.
  - `session.Session.Cache`: listing vaults replaces the cached list and forgets vaults that disappeared (removed access). Listing items asks for changes since the cached `seq`, applies them page by page in one transaction each (tombstones delete the row), and decrypts from the cache. A `nextSeq` below the cached `seq` is a rollback (§7.5). A cached item that no longer decrypts makes the vault download from `seq` 0 once.
  - The server counts as unreachable on a connection error or a 502/503/504 from the reverse proxy (`client.IsUnreachable`). Then reads, unlock with the master password, Touch ID unlock and the master password check come from the cache, and the session reports `IsOffline`. Any other error, such as 401 for a revoked device, is never hidden by the cache.
  - Offline, the app shows an "Offline" notice and refuses changes with a clear message. Edits are not queued; that would need the merge of §7.4. Background sync runs every 60 s and when the window gets focus, and stays quiet while an item is being edited.
- Auto-lock after idle time (chosen in Settings: 30 seconds, 1 minute, 5 minutes, 10 minutes by default, or 1 hour; a device preference kept across sign-out) and on screen lock/sleep. Clipboard cleared after 90 s.
  - **Idle means the Mac is idle.** On macOS the idle time is the time since the last keyboard, mouse or trackpad input anywhere on the Mac (`CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateHIDSystemState, kCGAnyInputEventType)`, `desktop/systemidle`), not only in Hisar's window. So a developer typing in a terminal or editor keeps Hisar unlocked, and the approvals they gave ("Don't ask again for 1 hour", remembered commit signing, command-line sessions) last as long as promised; a Mac nobody touches locks after the chosen time even while a script, coding agent or remembered approval keeps using Hisar. Requests (SSH agent signatures, the command-line tool, coding agents, browser fills) therefore never postpone auto-lock themselves: counting them would let unattended automation keep Hisar unlocked forever. The timer fires at the chosen time after the last reset and checks the system idle time; when the Mac saw input since, it waits for the rest of the interval and checks again.
  - Where the system idle time is not available (Linux, or a failed call), idle is measured as before: input in Hisar's window (`ReportActivity`) and calls into the vault backend from the UI or the command-line tool.
  - Screen lock and sleep still lock at once (`background`), whatever the idle time.
- Ships the native-messaging host for the browser extension and the `hisar` CLI, which can use the app's session (§9.4).
- **Settings** (⌘, or the gear) is a window-like dialog, about 880 × 620 points: a source list on the left with a **Search settings** field, the selected section on the right with its rows in rounded groups. Sections, in this order:
  - **General**: Appearance and Color (§9.10); Open Hisar at login (§9.1.2); the version with **Check for updates** and **Automatically check for updates** (§9.1.1); Import from another password manager (§9.7).
  - **Security**: Unlock with Touch ID, Auto-lock, the Emergency Kit with the Secret Key, **Change master password…** and **Replace Secret Key…** (§5.7).
  - **SSH & Git**: the SSH agent's status with **Use Hisar for SSH** and the `~/.ssh/config` section folded under a disclosure, Compact SSH approvals, Approve requests on my iPhone, and **Remember commit signing** (Always ask by default, or up to 10 minutes or 1 hour; §9.6).
  - **Developer tools**: the `hisar` command-line tool (install, PATH, Let the command-line tool use Hisar, §9.4) and Coding agents (`hisar mcp` and the per-vault "Available to agents" switch, §9.4.2).
  - **Browser**: browsers the extension is ready in, paired browsers with Remove, Only fill on the exact host (§9.2).
  - **People**: invitations for you (the source list shows their number as a badge), people with their security codes, invitations you sent, and **Invite to Hisar** in the header (§5.1), which explains that it creates a Hisar account and that vaults are shared from the vault's Share panel. Sharing a vault is **Invite by email** with **Share** in that panel (§5.8).
  - **Devices**: signed-in devices with last activity, Revoke.
  - **Export**: "Export your data" (§9.11): **Encrypted export (recommended)** with the export passphrase entered twice, or **Plain JSON** behind a warning box; **Export with Touch ID…** (or the master password field and **Export…** without Touch ID) opens a save dialog and says "Exported N items from M vaults to <path>."
  - **Account**: name, email, server, Sign out and **Delete account…** (§5.7.1).
  - **Help**: **Contact support** with the address `support@tryhisar.com` as selectable text and **Copy** (one constant, `desktop/frontend/src/support.ts`), and **Diagnostics**: the Hisar version and build, the macOS version, the server URL and this Mac's device ID, with **Copy diagnostics**. Diagnostics never include secrets, names or other people's emails. The server does not report its version, and there is no public documentation URL yet, so neither is shown.
  - Search matches row titles, descriptions and keywords: the source list keeps only sections with a match, the selected section shows only its matching rows, and no match says so. ↑/↓ move between sections, ⌘F focuses the search, Esc closes. The last selected section is per-device UI state (the web view's local storage) and opens again next time.
- **Vault menu** (§5.10): right-clicking a vault in the sidebar, or **⋯** (Vault actions) in the item list's header, offers **Rename…** (owners), **Delete vault…** and **Leave vault…** (shared vaults). An action the user may not take opens a short dialog with the reason (for the Personal vault: why it can't be deleted) instead of a disabled item. **Delete “Work”?** lists "These people lose access:" for a shared vault and, when it has items, offers "Move its N items to [vault]" (the default) or "Delete the vault and its N items", which needs "Type Work to confirm" before **Delete vault** enables. **Leave “Work”?** says the user loses access to its items on all their devices and that an owner can invite them again. Afterwards the app selects the target vault (or Personal) and shows "Moved N items to Personal and deleted Work.", "Deleted Work." or "You left Work."
- Runs the SSH agent (§9.6) and handles Touch ID (§5.4).
- **Sign in and create account**: one tagline on both tabs ("Passwords, SSH keys and secrets for developers, end-to-end encrypted.") with factual bullets: sign-in shows the SSH agent (private keys never leave Hisar), the CLI (reads passwords and one-time codes and uses SSH keys through the agent) and end-to-end encryption; create account shows breach safety, the two secrets and the Emergency Kit. The Server field is behind **Use a different server…** (§5.3), and the account form has the strength meter (§5.2).

#### 9.1.1 Automatic updates (macOS)

The app updates itself with [Sparkle 2](https://sparkle-project.org) through the user's own Hisar server, so the app never talks to a third party and a server's users only get builds its operator allows.

- **Framework.** Sparkle (pinned version and SHA-256 in `scripts/download-sparkle.sh`) is downloaded from its GitHub release at build time, never committed, and embedded in `Hisar.app/Contents/Frameworks`; the app links it with the rpath `@executable_path/../Frameworks` (Go build tag `sparkle`, set for every packaged macOS build; `wails3 dev` and other platforms build without it and report updates as unavailable). `scripts/sign-desktop-app.sh` signs Sparkle's helpers, the framework and then the app, inside out; ad-hoc today, a Developer ID identity (with the hardened runtime) later through `HISAR_SIGNING_IDENTITY`.
- **Trust.** Every update archive is signed with an EdDSA (Ed25519) key; `SUPublicEDKey` in `Info.plist` holds the public key, and with `SUVerifyUpdateBeforeExtraction` Sparkle checks the signature before it even unpacks the archive and refuses it if it does not verify; the unpacked app must also be code signed (Sparkle allows moving from ad-hoc to a Developer ID, not dropping the signature). The private key never enters the repository: the release workflow reads it from the repository secret `SPARKLE_PRIVATE_KEY` into a temporary file that it deletes when the job ends. The server only relays, so a compromised server can withhold updates or offer an older signed build (Sparkle never installs a lower `CFBundleVersion`) but cannot ship code.
- **Feed.** `Info.plist` has no `SUFeedURL`; the feed is `<server>/api/v1/desktop/updates/appcast.xml` of the signed-in server, and Sparkle sends the session's `Authorization: Bearer <token>` header with the appcast request and the download. While signed out, update checks are skipped (a manual check says to sign in). `SUEnableAutomaticChecks` is on with a daily `SUScheduledCheckInterval`; Settings → General → Updates shows the version, has **Check for updates** and an **Automatically check for updates** switch, stored in `desktop.json` (it survives signing out).
- **Versions.** Releases are `MAJOR.MINOR.PATCH`, each part at most 999. `CFBundleShortVersionString` is the version and `CFBundleVersion`, which Sparkle compares, is the build number `MAJOR × 1 000 000 + MINOR × 1 000 + PATCH` (0.2.1 → 2001, 1.0.0 → 1000000), so it grows with every release.
- **Pipeline.** A `desktop-<version>` tag builds the DMG and also `Hisar-<version>.zip` (`ditto -c -k --sequesterRsrc --keepParent`), signs the zip with Sparkle's `sign_update`, and writes `update.json`: `{version, buildNumber, archive, edSignature, length, minimumSystemVersion, releaseNotes, releaseNotesFormat ("markdown" or "plain-text"), publishedAt}`. The release notes are the tag's annotation message. The workflow "Release desktop" (GitHub-hosted macOS runner) puts the files on the GitHub Release of the tag, created as a prerelease. The contract an update source reads:
  - Release: tag and release `desktop-<version>` in `tosunlab/hisarapp-core`, titled "Hisar for macOS <version>" ("… (custom build)" for a label).
  - Assets: `Hisar-<version>.dmg`, `Hisar-<version>.zip` (the signed Sparkle archive, the `archive` in `update.json`), and `update-unpublished.json` until the version is published.
  - Published: running the Publish workflow (app `desktop`) renames `update-unpublished.json` to `update.json`, turns the prerelease into a release and marks it latest. A version is offered to apps only when its release has an asset named exactly `update.json`; a source should also skip draft and prerelease releases and tags with a label, and must not rely on GitHub's single "latest" release, which every app's publish moves.
  - A published release is never rebuilt: the workflow refuses a tag whose release is no longer a prerelease, so `update.json` always matches its zip.
  A custom build (`desktop-1.2.3-label`) is only the DMG, gets the build number just below `1.2.3` so that release updates it, and can't be published (servers also ignore versions with a label).
- **Server.** The server reads the contract above from the GitHub Releases of `HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY` (default `tosunlab/hisarapp-core`) through the REST API, with `HISAR_DESKTOP_UPDATE_GITHUB_TOKEN` as a bearer token when set (§8.4). It lists every page of releases (up to 1000), keeps the `desktop-MAJOR.MINOR.PATCH` releases that are neither draft nor prerelease and have an `update.json` asset, reads `update.json` of the newest ones, and builds an appcast of the newest 5 valid releases (newest first, `sparkle:format` for the notes), cached for 10 minutes. Each page is asked with its `ETag` (`If-None-Match`), so an unchanged list costs no rate limit, and an `update.json` asset is read once (a new upload gets a new asset ID). When GitHub fails the server keeps serving the cached appcast and retries after a minute; when GitHub answers 403 or 429 for its rate limit, it waits for `Retry-After` or `X-RateLimit-Reset` (at most an hour) before asking again. Enclosure URLs point back to the server (`HISAR_PUBLIC_URL`), so the download also needs a session and apps never talk to GitHub. `GET /desktop/updates/{version}/{file}` accepts only a `MAJOR.MINOR.PATCH` version and an asset of that version's release in the appcast, fetches it through the API (`Accept: application/octet-stream`, following the redirect to GitHub's storage) and streams it with its `Content-Length` without buffering it; the token is never forwarded to another host on redirects. Both endpoints have their own per-IP rate limit.

#### 9.1.2 Staying available: open at login and the menu bar

`ssh`, `git` (push, fetch, commit signing) and `hisar` reach Hisar through `~/.hisar/agent.sock` and `~/.hisar/cli.sock`, which exist only while the app runs. An SSH client cannot start an app: with Hisar not running it fails at once (`connect to agent … Connection refused`, then `Permission denied (publickey)` or a signing error). So the app starts at login and keeps running when its window closes.

- **Open at login.** Settings → General → "Open Hisar at login", on by default. The first launch of a build with this switch registers the login item once and records that in `desktop.json` (`openAtLoginConfigured`). After that Hisar registers only when the user turns the switch on; turning it off unregisters it and records `openAtLoginDisabled`, so Hisar never registers itself again. Both are device preferences that sign-out keeps.
  - macOS 13 and later: `SMAppService.mainAppService` (ServiceManagement through cgo). Settings shows what the system reports, not only the switch: on; "needs your approval" (Hisar was turned off in System Settings → General → Login Items) with **Open Login Items** (`+[SMAppService openSystemSettingsLoginItems]`); or not registered (removed there; turning the switch off and on registers it again). A development build that is not inside `Hisar.app`, and macOS 12, show it as unavailable and never register.
  - Linux: an XDG autostart entry, `$XDG_CONFIG_HOME/autostart/com.tryhisar.hisar.desktop` (`~/.config/autostart` by default), with `Exec=<the app's executable> --hidden`. It counts as on while that file exists without `Hidden=true` or `X-GNOME-Autostart-enabled=false`; off removes the file.
- **Hidden launch.** When the app is opened at login (macOS: the launch Apple event carries `keyAELaunchedAsLogInItem`; Linux: `--hidden`) or by `hisar` (`--hidden`, §9.4), it opens no window. The SSH agent, the CLI socket and the browser bridge run, Hisar is locked as after every start, and the first request opens the approval window, which unlocks it (§9.6).
- **Closing the window keeps Hisar running.** Closing the main window hides it; the sockets stay up and auto-lock works as before. The menu bar item (Wails system tray; the tower as a template image) has **Open Hisar**, **Lock** (**Unlock…** while locked and **Sign In…** while signed out, both open the window) and **Quit Hisar**. Clicking the Dock icon, opening Hisar.app again or, on Linux, starting the binary again shows the main window, never the hidden approval window; a second instance hands over to the running one (Wails single instance) instead of taking over the sockets.
- **Dock icon.** On macOS the app is an accessory app (no Dock icon, no app menu) while its main window is hidden and a regular app while it is shown; the activation policy follows every show and hide of the main window. The approval window shows without a Dock icon.
- **Quitting** is explicit: Hisar → Quit Hisar (⌘Q) or **Quit Hisar** in the menu bar item. Both stop the sockets. Sparkle's install-and-relaunch goes through the same terminate path (§9.1.1), and the relaunched app opens its window as after any normal launch.
- Linux desktops without a tray (GNOME without the AppIndicator extension) show no menu bar item; Hisar still keeps running, and starting it again shows the window.

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

**Messages.** Requests are `{"id": "<uuid>", "type": "…", "token": "…", …parameters}`; responses are `{"id", "ok": true, "result": {…}}` or `{"id", "ok": false, "error": {"code", "message"}}`. Error codes: `invalid-request`, `not-paired`, `pairing-failed`, `signed-out`, `locked`, `offline`, `not-found`, `no-match`, `desktop-unavailable`, `update-required` (§8.7), `internal`. Every type except `hello`, `pair.start` and `pair.finish` requires the pairing `token`.

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

**URL matching** (desktop, `core/browserprotocol`). Item URLs without a scheme are read as `https://`. A page matches an item URL when the ports match (default ports normalized) and the scheme is the same, or the item says `http` and the page is `https`. By default the hosts must share their registrable domain (eTLD+1 from the public suffix list, so `accounts.google.com` matches `google.com`, but two `github.io` sites do not); the Settings option "Only fill on the exact host" requires equal hosts. IP addresses and `localhost` always need equal hosts.

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

**iOS (Phase 4, first).** `mobile/hisarkit` wraps the app backend (`core/app`, shared with the Mac app, which has no Wails import and compiles for `ios/arm64`), so the phone gets the same sign-in, offline cache, matching and sharing logic. `make build-ios-framework` runs `gomobile bind -target=ios,iossimulator -iosversion=26.0` and writes `mobile/build/HisarKit.xcframework` (Swift module `HisarKit`, classes prefixed `Hisarkit`).

- gomobile passes only strings, numbers, `[]byte` and interfaces, so lists and items cross as JSON strings with the desktop's types (`vault.Status`, `VaultSummary`, `ItemDetail`, `browser.Login`).
- `HisarkitNew(stateDirectory, deviceName, secretKeyStore, biometric, clipboard, listener)` keeps `hisar.json` and `hisar-cache.db` in `stateDirectory`, the app group container, so the app and the AutoFill extension share the state and the encrypted cache.
- **Methods:** `Status`, `SignIn`, `SignUp` (returns the status and the new Secret Key), `Unlock`, `UnlockWithBiometric`, `EnableBiometricUnlock`, `DisableBiometricUnlock`, `Lock`, `SignOut`, `ReportActivity`, `ListAllVault`, `ListAllItem` (no secrets), `GetItem`, `SaveItem` (item JSON in, item JSON out), `DeleteItem`, `CopyField`, `GetOneTimeCode`, `GeneratePassword`, `ListAllLoginForService`, `GetLoginForService`, `GetOneTimeCodeForService`, for sharing (§5.8) `CreateVault` (always a shared vault), `ListAllPerson` (the people the user shares a vault with), `InvitePerson` (the server emails a sign-up invite), `ListAllVaultMember`, `ShareVault` (sends a share invitation), `ListAllShareInvitation` (completes accepted shares first, as the desktop's sync does), `AcceptShareInvitation`, `DeclineShareInvitation`, `CancelShareInvitation`, `ChangeVaultMemberRole`, `RemoveVaultMember` (rotates the vault key, as on the desktop), `GetVaultSharing` (one shared vault's sharing screen: the vault, whether the user owns it, its members with role and security code, its sent invitations, and the completion errors for its accepted invitations; completes accepted shares first and refuses a personal vault), for passkeys (§9.8) `ListAllPasskeyForRelyingParty`, `PreparePasskeyRegistration`, `CreatePasskey`, `AssertPasskey` and `DeletePasskey`, and for Settings `SetAutoLock` (30, 60, 300, 600 or 3600 seconds, the desktop's choices, stored as `autoLockSeconds` in `hisar.json`; `Status` reports it), `ListAllDevice` and `RevokeDevice` (another device; this one signs out with `SignOut`), `ExportEmergencyKit(password, directory)` (checks the master password like an unlock, writes `Hisar Emergency Kit.pdf` into `directory` and returns the Secret Key and the file's path), `CopySecretReference` (`hisar://<vault>/<item>/<field>`, §9.4.1), `IsMacApprovalAllowed` / `SetMacApprovalAllowed`, for vaults (§5.10) `GetVaultActions`, `RenameVault`, `DeleteVault` (a `VaultDeletionRequest` as JSON), `LeaveVault` and `ListRemovedVaults`, and for export (§9.11) `GetExportOptions` and `ExportAllData(format, passphrase, password, directory)` (Face ID first, or the master password when one is given; writes the file into `directory` and returns its path). Item summaries and logins carry `hasOneTimePassword`, never the secret.
- **Phone approvals (§9.9):** `ListAllPendingApproval` (the open, unexpired requests of other devices, opened and checked, as JSON), `ApproveApproval(requestID, isRemembered)` (asks `Biometric.Authenticate` first), `ApproveApprovalWithPassword(requestID, password, isRemembered)` (checks the master password instead) and `DenyApproval`; `isRemembered` is the sheet's switch.
- **Interfaces implemented in Swift:**
  - `SecretKeyStore`: the Secret Key in the Keychain, in a keychain access group shared with the extension.
  - `Biometric`: Face ID or Touch ID. `StoreKey` writes the biometric unlock key to a Keychain item with `SecAccessControl` `.biometryCurrentSet` and `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`. `Authenticate` evaluates an `LAContext` that `LoadKey` passes as `kSecUseAuthenticationContext`, so the user sees one Face ID prompt.
  - `Clipboard`: `UIPasteboard`.
  - `EventListener`: auto-lock notification, `OnApprovalEvent(name, requestID)` for the approval events of the event stream (§9.9), and `OnRemoteChange()` after the backend synced a `vault-changed`, `vaults-changed` or `share-invitations-changed` event (the desktop's `OnRemoteChange`), so the app reloads its vaults, the received invitations and an open sharing sheet.
- **Biometrics after launch:** because iOS enforces the biometric check in the Keychain itself, `IsBiometricAllowedAfterLaunch` is on and Face ID can unlock after a launch without the master password first. The 14-day password interval still applies. Auto-lock defaults to 5 minutes; Settings → Security changes it.
- **One-time codes:** `GetOneTimeCodeForService` checks the match as `GetLoginForService` does and returns only the code, expiry and period. When a password fill (list, QuickType or without user interaction) is for a login with `hasOneTimePassword`, the extension copies the current code to the pasteboard (`localOnly`, 90 s expiry) before completing the request.
- **AutoFill:** the extension receives `ASCredentialServiceIdentifier`s (a domain or a URL; bare domains are read as `https://`). `ListAllLoginForService` uses the same matching as the browser extension (§9.2), and `GetLoginForService` checks the match again before returning the password. Passkey requests carry a relying party ID instead, and only passkeys with exactly that RP ID are offered (§9.8).
- **Signing:** until the Apple Developer Program membership (§12 #7), the app runs on the developer's own iPhone with a free Apple ID (re-signed every 7 days). App groups and the AutoFill extension need the paid membership on a device, so the extension is developed and tested in the simulator until then.

**iOS app (`mobile/ios`).**

- **Project:** XcodeGen (`project.yml`, pinned 2.46.0) generates `Hisar.xcodeproj`, which is committed so Xcode opens it without XcodeGen; CI fails when the two differ. The team and the bundle identifier prefix live in `Configuration/Signing.xcconfig` (or an ignored `Local.xcconfig`). The extension (`.autofill`), the app group (`group.…`) and the keychain group (`$(AppIdentifierPrefix)….shared`) are derived from the prefix and reach the code through `Info.plist` keys.
- **Targets:** `Hisar` (app with the extension) and `HisarAutoFill` share `Shared/` as sources rather than a framework. HisarKit is a static framework, so each binary links its own copy of Go. `HisarPersonal` (scheme "Hisar Personal Team") is the same app for a free personal team: no extension, empty app group and keychain group, and its own bundle identifier (`….personal`) so the free team never registers the real one. With no app group the state goes to the app's Application Support folder, and Keychain items get no access group.
- **Threading:** Swift wraps `HisarkitHisar` behind a `VaultBackend` protocol. View models call it on a background queue, because Argon2 and network I/O block. Tests replace it with a fake.
- **Keychain:** the Secret Key is `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` in the shared access group. The Face ID key is `.biometryCurrentSet` + `WhenUnlockedThisDeviceOnly`; `Authenticate` keeps the evaluated `LAContext` and `LoadKey` passes it as `kSecUseAuthenticationContext`, so the user sees one prompt. The state directory is excluded from backups.
- **Clipboard:** copies are `localOnly` with a 90 s `expirationDate`. Go's own 90 s clearing asks `GetText`, which answers from `changeCount` instead of reading the pasteboard, so iOS never shows the paste banner for another app's text.
- **Lifecycle:** going to the background records the time; returning after the auto-lock interval (Settings → Security, 5 minutes by default) locks. While the scene is inactive, a separate window at alert level covers everything, including sheets, so the app switcher snapshot shows only the logo. The lock screen asks for Face ID once per lock when it is ready.
- **AutoFill:** after every full sync and every save or delete, the app replaces the `ASCredentialIdentityStore` identities: host + username for each login with a URL, with record identifier `vaultID/itemID`, an `ASOneTimeCodeCredentialIdentity` (label: the username, or the title) for each such login with a one-time password, and an `ASPasskeyCredentialIdentity` (RP ID, user name, credential ID, user handle) for each passkey in a login. The extension declares `ProvidesPasswords`, `ProvidesPasskeys` and `ProvidesOneTimeCodes`; `prepareOneTimeCodeCredentialList` lists only matching logins with a code, and one-time code requests complete with `ASOneTimeCodeCredential` and never hand out the password. Without user interaction the extension answers only when its process is still unlocked; otherwise it asks for interaction and unlocks with Face ID or the master password. The list shows the logins matching each service identifier; search also covers every other login, which is filled only on an explicit tap. `hisarkit` calls `debug.FreeOSMemory` after every key derivation, so the extension drops back from about 126 MB at the Argon2 peak to about 68 MB once unlocked (simulator debug build).
- **Deployment target:** iOS 26.0 (Xcode 27, iOS 27 SDK), so the app uses the iOS 26 design directly and has no fallback UI for older systems.
- **Interface:** system components only; the app draws no chrome of its own. Light and dark follow the system, text uses Dynamic Type styles, every icon is an SF Symbol with an accessibility label, and the only custom colour is Hisar's accent (`AccentColor`: graphite in light mode, the desktop's `#eceef1` in dark), set as the asset catalog's global accent colour.
  - A `TabView` with the Liquid Glass tab bar: **Vaults**, **Approvals** (badge: the number of waiting Mac requests) and **Settings**, plus a search tab (`Tab(role: .search)`) that searches every item. Each tab is a `NavigationStack` with a large title and the Liquid Glass navigation bar; actions are `.toolbar` items and `Menu`s.
  - **Vaults:** an `.insetGrouped` `List`. Waiting Mac requests come first as a card (the Mac's name and a live countdown, the request's sentence, the key, **Deny** and **Approve**, and "N more requests" opening the Approvals tab); then, as on the Mac, a banner for each share invitation the user received ("Ada shared a vault with you as editor", the inviter's email, **Decline** and **Accept**; after accepting, the vault appears once the owner's Hisar completes the share); then the vaults with their item counts, and **By type** (Logins, SSH keys) across all vaults. Toolbar: **Lock** and a **+** menu (New login, New vault). Pull to refresh syncs, which also completes the shares people accepted and reloads the received invitations.
  - **Approvals:** the waiting requests with a live countdown; tapping one opens the approval sheet, swiping offers Deny. Empty, a `ContentUnavailableView` says that requests from Hisar on the Mac appear here while Hisar is open and unlocked on this iPhone.
  - **Approval sheet:** a system sheet (medium and large detents) with the request's grouped `LabeledContent` rows, the forwarded-request warning, the encryption note, and **Approve with Face ID** and **Deny** in the bottom toolbar; the master password field appears in the sheet when Face ID is unavailable.
  - **Vault:** the items, searchable, with **+** (writable vaults) in the toolbar. A shared vault has **Share** (owners) or **Members** (editors and readers) in the toolbar; a personal vault's **Share** explains in an alert that personal vaults are private and a shared vault is needed, the Mac's rule. A **⋯** menu has **Rename…**, **Delete vault…** and **Leave vault…** (§5.10); an action the user may not take shows its reason in an alert. Rename is an alert with a text field. **Delete** is a sheet: "These people lose access" for a shared vault, then "Move N items to another vault" (the first writable vault by default, with a **Move to** picker) or "Delete the vault and its N items" with "Type Work to confirm", and the destructive **Delete vault**; an empty vault needs one confirmation. **Leave** is a `confirmationDialog`. When the open vault disappears, the screen goes back to the list.
  - **Removed vaults:** after every refresh the Vaults list shows, at the top, a dismissible row for each vault that disappeared ("“Work” is no longer available: it was deleted, or you no longer have access to it.").
  - **Sharing sheet** (`GetVaultSharing`, medium and large detents): for owners, **Invite by email** (email field, a role `Picker` with the Mac's roles Owner, Editor and Reader, and **Send invitation**; every address gets the same "Invitation sent", whether or not it has an account); a warning row when an accepted invitation could not be completed (e.g. "the signing key of … changed since you last shared with them; stopping"); **Members**, owners first, each with role and security code, and a key-changed warning; **Invited**, this vault's sent invitations ("Waiting for them to accept" or "Accepted, finishing on your next sync"), each with **Cancel** behind a `confirmationDialog`. Tapping a member opens their detail: the security code to compare (with `ShareLink` for the user's own code), and for owners a role `Picker` and **Remove from vault** behind a `confirmationDialog` that says the vault is re-encrypted with a new key. Editors and readers see the members and codes only. The sheet reloads on `OnRemoteChange`, so a person who accepts appears without a refresh.
  - **Completing shares:** the iPhone runs the same Go code as the Mac (`session.CompleteAcceptedShares` through `core/app`): on every sync, every `share-invitations-changed` event, pull to refresh, the People screen and the sharing sheet, it verifies each accepted recipient's signed keys, pins their signing key, refuses on a changed key and wraps the vault key. Only an unlocked app does this; a locked iPhone leaves the share to the owner's next unlocked device.
  - **Item detail:** grouped rows for username, password (reveal, copy), one-time code (with a progress ring), custom fields, websites, passkeys and notes. Copying plays a success haptic and shows the 90-second note; each copyable row's context menu also has **Copy secret reference**, and a quiet last row copies the password's reference.
  - **Settings** (what of the Mac's settings makes sense on a phone): **Account** (name, email, server; **Export data…**, §9.11; **Delete account** with the preview, master password and typed email, §5.7.1; **Secret Key and Emergency Kit**, which asks for the master password, then shows the Secret Key and offers the Emergency Kit PDF through `ShareLink`; the file is written to the app's temporary folder and deleted when the screen closes), **Security** (Face ID, Auto-lock picker 30 s · 1 min · 5 min · 10 min · 1 h, Lock now, the note that copied values clear after 90 seconds, **Change master password** and **Replace Secret Key**, which shows the new Secret Key and shares the new Emergency Kit PDF like the Account screen, §5.7), **Mac approvals** (the switch "Approve requests from my Mac", on by default, and the note that approving answers one request and never unlocks the Mac), **People** (invitations for you with a badge, invitations you sent, **Invite to Hisar**, which creates an account, and each person with their security code and a "key changed" warning), **Devices** (each signed-in device with platform and last activity, "This iPhone" first; another device can be signed out after a confirmation), **AutoFill**, **Password generator**, **Help** (Contact support with the address and Copy, Copy diagnostics with the app and iOS versions, the server URL and this iPhone's device ID), **About**, and **Sign out**. The SSH agent, command-line tool, browser extension, agent vault access, open at login, updates and import stay on the Mac.
  - **Sign in, create account and unlock:** `Form`s with the same copy as before (the Emergency Kit wording included), the Server field behind **Use a different server…** (§5.3), and after the new Secret Key the "Unlock with Face ID?" offer (§5.2); the lock screen shows the mark, the account's email, **Unlock with Face ID** and the master password field, and above them the accepted shares waiting for this unlock (§5.8).
- **Simulator limits:** a build without a team is registered as a provider but iOS does not offer it, so UI tests drive the extension's screens through a debug-only preview in the app. Face ID is tested in the simulator by posting the biometric daemon's enrollment and match notifications.

### 9.4 CLI

`hisar` is the command-line client for the terminal and for scripts: vaults (`vaults`, `vault create|rename|delete|leave`, §5.10), items (`ls`, `get`, `add`, `edit`, `rm`), the generator, import of other password managers, Hisar exports and `.env` files (§9.7), export (`export`, §9.11), sharing (`share <vault> <email> [-r role]`, `unshare`, `people`, `invitations [accept|decline|cancel <id>]`, §5.8), invites (`invite <email>`), the account (`account change-password`, `account replace-secret-key [-o file]`, `account delete`, standalone only because they need the master password and the Secret Key, §5.7, §5.7.1), share links (`share-link`, `share-links`, §5.9), devices, `read` and `run`, which hand secrets to scripts and commands through secret references (§9.4.1), and `mcp`, the server that lets coding agents use Hisar (§9.4.2). It works in one of two modes:

- **Through the desktop app**, like 1Password's CLI integration: `hisar` asks the running Hisar app for what each command needs over `~/.hisar/cli.sock`. It has no session, state file or keys of its own, and nothing is unlocked in the `hisar` process.
- **Standalone**: its own sign-in, with the session in `state.json` (`HISAR_STATE`) and the Secret Key in the OS keychain (`HISAR_SECRET_KEY` overrides it); every command that decrypts asks for the master password. This is the mode on servers, where no app is installed, and whenever the app does not answer (see "Starting the app").

**Choosing the mode.** `--standalone` (before the command) or `HISAR_STANDALONE=1` forces standalone. Otherwise `hisar` connects to the socket (`HISAR_CLI_SOCKET` overrides the path, for the app and the CLI) and sends `hello`. It uses the app when the app answers. When the socket is missing or refuses the connection, `hisar` starts the app and waits for it (below). It falls back to standalone when no app is installed, when the app answers `integration-disabled`, or when the started app did not answer but a standalone sign-in exists. An error after the app answered (denied, signed out, offline) is reported; `hisar` never retries it standalone. `hisar status` prints the mode, why that mode was chosen, and the account (`email` on `server`, and whether the app is locked). Missing arguments are asked for interactively; flags are for scripts. Standalone, `hisar invitations` completes accepted shares after asking for the master password, as the app does on every sync. `signup`, `login` and `logout` manage the standalone sign-in, and `emergency-kit` needs the Secret Key, which the app never hands out; in app mode it stops with a message that points to the app or to `--standalone`. The removed `recover` and `recovery-*` commands only print `account recovery was removed; sign in with your Emergency Kit` and exit with an error. `hisar --version` prints the version stamped at build time (`-X main.version`), which it also sends to the server (§8.7).

**Starting the app.** SSH clients cannot start Hisar, but `hisar` can. When the socket is missing or refuses the connection, a command that can run through the app (every command except `signup`, `login`, `logout` and `emergency-kit`, which then run standalone as before) starts the app hidden (§9.1.2) and prints "Starting Hisar…" on stderr. It does not when the SSH agent socket (`HISAR_SSH_AGENT_SOCKET` or `~/.hisar/agent.sock`) accepts a connection: then the app is running with the command-line switch off, and starting it again would only show its window.

- macOS: `open -g -a <Hisar.app> --args --hidden` when `hisar` is the binary bundled in a `Hisar.app` (also through the installed symlink), otherwise `open -g -b com.tryhisar.hisar --args --hidden`. `open` failing means no app is installed.
- Linux: the first executable `Hisar` next to `hisar`, on the `PATH`, in `/usr/local/bin` or in `/usr/bin`, started with `--hidden` in its own session with its output discarded. None found means no app is installed.

It then dials the socket and sends `hello` every 200 ms for up to 10 s. Once the app answers, the command runs through it. If no app is installed, `hisar` runs standalone as before. If the app was started but did not answer in time, `hisar` runs standalone when a standalone sign-in exists; otherwise the command fails with "Hisar isn't running. Open the Hisar app and try again." instead of a connection error. `HISAR_NO_LAUNCH=1` (for scripts and CI) never starts the app and keeps the earlier behaviour: standalone whenever the app does not answer.

**Bundled in the macOS app.** `Hisar.app/Contents/Helpers/hisar` is a universal (arm64 + x86_64) build of the CLI. It cannot be `Contents/MacOS/hisar`: the default macOS file system ignores case, so that is the app's own executable `Hisar`. It is built with `CGO_ENABLED=0`, stamped with the app version and signed by `scripts/sign-desktop-app.sh` before the app, and it updates with the app through Sparkle (§9.1.1). Settings → Developer tools → Command-line tool:

- Shows whether `hisar` is installed (a symlink to the bundled binary) and where, and whether the first `hisar` on the user's `PATH` is this app's. Apps started from Finder get a minimal `PATH`, so the app reads the user's from their login shell (`$SHELL -l -i -c`, 3 s timeout, cached for the session) and falls back to `/etc/paths`, `/etc/paths.d` and its own `PATH`.
- **Install command-line tool** creates the symlink `hisar` → the bundled binary in `/opt/homebrew/bin` when that directory exists and is writable, else `/usr/local/bin` when it exists and is writable, else `~/.local/bin` (created, 0755), and then shows a one-line hint when that directory is not on the `PATH`. It never asks for an administrator password. An existing `hisar` there that is a symlink into a `Hisar.app` (an older or moved copy) is replaced; anything else, a file or a symlink to somewhere else, is never touched, and Settings shows what is there.
- **Uninstall** removes only `hisar` symlinks in those three directories that point into a `Hisar.app`.
- **Let the command-line tool use Hisar** (on by default; stored in `desktop.json` and kept on sign-out). Off closes and removes the socket, so `hisar` falls back to standalone (it sees the app running on the SSH agent socket and does not start it again); a request that still arrives is answered `integration-disabled`.
- Development builds have no bundled binary; the section says so and offers no install.

**Socket.** `~/.hisar/cli.sock`, socket 0600 in the same 0700 directory as the SSH agent and browser sockets, served while the app runs and the switch is on. The app accepts only peers with its own user ID (`LOCAL_PEERCRED` on macOS, `SO_PEERCRED` on Linux) and closes any other connection without an answer. Frames are a 32-bit big-endian length followed by UTF-8 JSON, at most 16 MiB. Requests are `{"id", "type", …parameters}`; responses `{"id", "ok": true, "result"}` or `{"id", "ok": false, "error": {"code", "message"}}`. Error codes: `invalid-request`, `integration-disabled`, `signed-out`, `locked`, `denied`, `offline`, `not-found`, `update-required` (§8.7), `internal`. Shared types live in `core/cliprotocol`.

| Type | Parameters | Result |
|---|---|---|
| `hello` | — | `protocolVersion` (1), `state` (`signed-out`, `locked`, `unlocked`), `email`, `server`, `isApproved` (for this terminal session), `features` (what the app understands beyond the protocol version: `secure-notes`, `credit-cards`; an older app sends none); never needs an approval |
| `account` | — | `accountID`, `name`, `email`, `role`, `server` |
| `vaults.list` | — | `vaults`: `[{id, name, type, role, itemCount}]` |
| `vaults.create` | `name` | `vault` |
| `items.list` | `vaultID` | `items`: summaries `{id, revision, category, title, username, urls, hasOneTimePassword}` |
| `items.get` | `vaultID`, `itemID` | `item`: the summary plus `password`, `notes`, `fields`, `passkeys` (`relyingPartyID`, `userName`), `publicKey`, `fingerprint` |
| `items.code` | `vaultID`, `itemID` | `code`, `expiresAt` (RFC 3339), `period` |
| `items.save` | `vaultID`, `item` (`id` and `revision` for an update; `title`, `username`, `urls`, `password`, `notes`, optional `fields`, optional `category` of a new item: `login` or `secure-note`, sent only to an app whose `hello` lists `secure-notes`, since an older app ignores it and saves a login) | `item`. An update keeps everything else of the item: category, one-time password, passkeys, SSH key, and the fields unless `fields` is sent (`.env` import) |
| `vaults.actions` | `vaultID` | `vault`, `canRename`, `canDelete`, `deleteBlockedReason`, `canLeave`, `leaveBlockedReason`, `membersLosingAccess` (`name`, `email`, `role`), `targetVaults` (§5.10) |
| `vaults.rename` | `vaultID`, `name` | `vault` |
| `vaults.delete` | `vaultID`, `vaultDeletion` (`moveToVaultID`, or `isDeletingItems` with `confirmedName`, the vault's name as typed) | `movedCount` |
| `vaults.leave` | `vaultID` | `{}` |
| `export` | `export` (`format`: `encrypted` or `plain`, `passphrase`, `path`: absolute, must not exist) | `path`, `vaultCount`, `itemCount`. The app writes the file itself (mode 0600, never replacing a file), so private keys and one-time password secrets never cross the socket; always asks first (§9.11) |
| `items.delete` | `vaultID`, `itemID` | `{}` |
| `items.import` | `vaultID`, `importedItems` (at most 10 parsed items) | `savedCount`, `failures` `[{title, message}]` |
| `people.list` | — | `people`: `[{accountID, name, email, role, securityCode, isCurrentUser, isKeyChanged}]`, the people the user shares a vault with (§5.8) |
| `vaults.share` | `vaultID`, `email`, `role` | `{}`; sends a share invitation (§5.8) |
| `vaults.unshare` | `vaultID`, `accountID` | `{}` (rotates the vault key, §4.4) |
| `invite` | `email` | `{}`; the server emails the invite (§5.1) |
| `invitations.list` | — | `received`, `sent` as in the app; listing first completes accepted shares |
| `invitations.accept`, `invitations.decline`, `invitations.cancel` | `invitationID` | `{}` |
| `shareLinks.create` | `vault` (name or ID), `query` (item title or ID, matched as by `hisar get`), `shareLink` (`fields`, `expiresInSeconds`, `maxViews`, `passphrase`; or `text` and `title`) | `shareLink` `{id, url, title, fieldLabels, expiresAt, maxViews}`; always asks first (§5.9) |
| `shareLinks.list` | — | `shareLinks`: `[{id, title, fieldLabels, isText, createdAt, expiresAt, maxViews, viewCount, hasPassphrase}]` |
| `shareLinks.revoke` | `shareLinkID` | `{}` |
| `devices.list` | — | `devices` |
| `devices.revoke` | `deviceID` | `{}` |
| `secrets.resolve` | `references` (at most 500 `hisar://` references), `commandLine` (the command `hisar run` starts, as its arguments; empty for `hisar read`) | `values`, in the order of `references`; fails as a whole with `not-found` or `invalid-request` and a message that names every reference that failed and why (§9.4.1) |
| `agent.hello`, `agent.*` | — | requests of `hisar mcp`, only on an agent connection (§9.4.2) |

The app answers with the bound `app.Service` methods, so the CLI gets the same checks, offline behaviour and errors as the UI. It never sends the master password, the Secret Key, account or vault keys, SSH or passkey private keys, or one-time password secrets: items carry what `hisar get` prints, and `items.code` and `otp` references return only the current code. A `private key` reference is refused (§9.4.1); SSH keys are used through the SSH agent (§9.6). `hisar` parses import files itself and sends the parsed items in batches of 10.

**Approval.**

- Approvals are per terminal session. From the peer PID (`LOCAL_PEERPID`, `SO_PEERCRED`) the app reads the session ID (`getsid`), the controlling terminal device and the start time of the session leader, so a session ID that is reused after the terminal closed does not inherit an approval. The terminal app and its icon come from `desktop/appinfo`, starting from the parent of `hisar`, because the bundled `hisar` itself lives inside `Hisar.app`.
- Every request except `hello` needs an approval for its session. The first one opens the approval dialog of the SSH agent (§9.6) in its full layout with "hisar in Terminal wants to use Hisar", Terminal's icon with the Hisar badge, and the program path and terminal device in the detail box; there is no "Don't ask again" checkbox and never the compact layout. On a Mac with Touch ID, Touch ID starts at once in the dialog; without it, Approve approves.
- If Hisar is locked, the same window unlocks it: with Touch ID when Touch ID unlock is ready (§5.4; the one Touch ID check approves and unlocks), otherwise with the master password typed into the window. A wrong password keeps the request open with a message. Signed out, requests fail with `signed-out` and no window.
- Deny, Escape or closing the window fail the request with `denied`, and `hisar` prints "Hisar denied the request (it was declined)". No answer before the dialog's deadline, 60 s or 2 minutes when the iPhone can answer (§9.9), fails it the same way with "Hisar denied the request (it was not answered within a minute)" or "… within 2 minutes)". Requests from one session that arrive while its approval is open wait for that one window.
- An approval lasts until 10 minutes after the session's last request, or until Hisar locks, signs out or quits.
- `export` always opens the dialog, even in an approved session, like a share link (§5.9): "hisar in Terminal wants to export all your vaults", with "Encrypted with an export passphrase · <path>" or "Plain JSON, not encrypted · <path>" and, for plain JSON, the warning "The file will hold every password, one-time code secret and SSH private key in plain text." Touch ID, the master password or the iPhone (§9.9) answer it.
- Same-user code that runs inside an approved terminal session can use Hisar through `hisar` until the approval ends, as with 1Password; code outside that session needs its own approval.
- When the request that opens the dialog is `secrets.resolve`, the dialog says what it is for: "hisar in Terminal wants to run a command with 3 secrets" (`hisar run`) or "wants to read 1 secret" (`hisar read`). The detail box shows the command `hisar run` starts (shell-quoted and shortened like the SSH dialog's command lines, §9.6), the secrets as "vault › item › field" (names when Hisar is unlocked, the reference as written while it is locked; the first 4, then "and N more"), and the terminal device. The approval still covers the whole terminal session: references that are read later in an approved session, by `hisar run`, `hisar read` or any other `hisar` command, do not ask again.

**Standalone releases.** A `cli-<version>` tag runs the GitHub Actions workflow "Release CLI" on a Linux runner. It builds `hisar` for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 and windows/arm64 (`CGO_ENABLED=0`, version from the tag), packs `hisar-<version>-<os>-<arch>.tar.gz` (`.zip` with `hisar.exe` for Windows) and `SHA256SUMS`, and uploads them to the GitHub prerelease `cli-<version>` (`scripts/package-cli-release.sh`), `SHA256SUMS` last, so a complete release is one that has it. The manual Publish workflow then checks that every file in `SHA256SUMS` is there and turns the prerelease into the latest release (not for a custom build `cli-1.2.3-label`).

#### 9.4.1 Secret references, `hisar read` and `hisar run`

Developers, scripts and coding agents name secrets instead of copying them: `.env` files, CI-like scripts and agent instructions hold references, and `hisar run` hands the values to one command without writing them to disk or showing them. `core/secretreference` parses, encodes and resolves references for the app and for standalone `hisar`.

**Syntax.** `hisar://<vault>/<item>/<field>`, for example `hisar://Personal/GitHub/password` or `hisar://Work/AWS%20prod/secret%20access%20key`.

- Exactly three segments, none empty. The scheme is case-insensitive; nothing follows the field (`?` and `#` are part of a name unless encoded).
- Segments are percent-decoded (`%2F` for `/`, `%25` for `%`); a malformed escape or a result that is not UTF-8 is an error. Spaces may also be written as they are, so `hisar://Work/AWS prod/secret key` works in quotes.
- Encoding (the desktop app's "Copy secret reference") keeps ASCII letters, digits, `-`, `.`, `_`, `~` and every non-ASCII character as they are, and percent-encodes every other byte, so a reference is one shell word and one unquoted dotenv value.

**Resolution.**

- Vault: the vault whose ID equals the segment, else the vault whose name equals it ignoring case (Unicode case folding). Item: the same with the item's ID or title, within that vault. No substring matches, unlike `hisar get`.
- Two vaults with that name, or two items with that title in the vault, are an error that says so and lists the IDs to use instead.
- Field, ignoring case: `password`, `username`, `notes`, `url` (the first website), `otp` (also `one-time code` and `code`: the current one-time code, generated when the request is answered), `public key`, `fingerprint`; anything else is the custom field with that label. A built-in label always means the built-in field; two custom fields with the same label are an error.
- `private key` is refused for every item: SSH private keys never leave Hisar, so the reference fails with "SSH private keys never leave Hisar; use the Hisar SSH agent instead". The label stays reserved, so a custom field labelled `private key` cannot be read through a reference either.
- A missing vault, item or field, and an empty value, are errors. Every error names the reference and says why (`hisar://Personal/GitHub/passwrd: item "GitHub" has no field "passwrd"`); a value is never part of a message. All references of a request are checked, and the error lists every one that failed.

**`hisar read <reference> [--no-newline]`** prints one value to stdout, for scripts. It adds a newline only when stdout is a terminal, and never with `--no-newline`. Nothing is masked: the caller asked for the value.

**`hisar run [--env-file path]... [--no-masking] -- <command> [arguments...]`**

- *Collecting.* Every variable of `hisar`'s environment whose whole value is a reference (it starts with `hisar://`), and every variable of the `--env-file` files, given in order. A later file overrides an earlier one; the real environment overrides the files, except a real variable whose value is itself a reference, which a file overrides. Values that are not references pass through as they are.
- *Env files* use dotenv syntax: `KEY=value` lines, `#` comments on their own line or after whitespace in an unquoted value, blank lines, an optional `export ` prefix, keys `[A-Za-z_][A-Za-z0-9_]*`. Values are unquoted (trimmed), single-quoted (literal) or double-quoted (`\n`, `\r`, `\t`, `\"`, `\\` and `\$` escapes); quoted values may span lines. There is no variable expansion. A syntax error names the file and line, never the value.
- *Resolving.* All references go to the app in one `secrets.resolve` request, which asks for at most one approval, with the command line, and resolve all or nothing. With no references, `hisar run` prints `hisar: no secret references found` on stderr, contacts neither the app nor the server, and runs the command.
- *Starting.* The command gets `hisar`'s environment with the env files applied and every reference replaced by its value. It is looked up on `hisar`'s `PATH` before anything is resolved: a missing command exits 127 and one that is not executable 126, without asking for an approval. stdin is passed through as it is, so the command can read the terminal or a pipe.
- *Masking* (the default). stdout and stderr go through pipes, and every occurrence of a resolved value is replaced with `<concealed by hisar>`. Matching works on bytes, so binary output is fine. Overlapping and adjacent occurrences of different values are replaced as one run when they overlap. A value that spans two writes is still found: `hisar` writes everything at once except a tail that could still be the start of a value, and writes that tail as it is when nothing completes it within 100 ms or the stream ends. Values shorter than 4 bytes are not masked, because they would garble ordinary output; `hisar run` names those variables on stderr. Because the command writes into a pipe, it sees no terminal: colours, progress bars and interactive prompts may change, and `isatty` checks fail.
- *`--no-masking`* gives the command `hisar`'s own stdout and stderr, so it keeps the terminal and its behaviour, and its output is not changed.
- *Signals.* SIGINT, SIGTERM, SIGHUP and SIGQUIT are forwarded to the command. The command shares `hisar`'s process group, so when `hisar` is the terminal's foreground job, the terminal already sends SIGINT and SIGQUIT (Ctrl-C, Ctrl-\) to both, and `hisar` does not forward those a second time; tools such as `docker compose` treat a second Ctrl-C as "stop now". SIGTERM and SIGHUP are always forwarded.
- *Exit code.* The command's exit code, or 128 + the signal number when a signal ended it. Errors of `hisar` itself (a bad reference, a denied approval, a syntax error in an env file) exit 1 and run nothing.
- *After the command exits*, `hisar` writes what is left of its output and exits. A background process that the command started and that keeps the pipes open gets 1 s more; output after that is lost.

**Limits.** The command gets the real values and can do anything with them: write them to a file, send them over the network, or print them encoded, reversed or in pieces, none of which masking catches. Output written straight to `/dev/tty` is not masked. Masking is a guard against accidental display in terminals, logs and coding agents' transcripts, not a sandbox. Values sit in the command's environment, which other processes of the same user may be able to read (`/proc/<pid>/environ` on Linux). An approved terminal session can read any secret through `hisar` until the approval ends (§9.4).

**Standalone**, `hisar read` and `hisar run` ask for the master password like other commands and resolve the references in the `hisar` process with the same rules; the password is read from stdin when it is not a terminal, before the command starts.

**Desktop.** Every field of an item that a reference can name (username, password, one-time code, custom fields; for SSH keys the fingerprint and public key) has a secondary "Copy secret reference" button (a terminal icon, tooltip "Copy secret reference — use it in a terminal with hisar run or hisar read") next to its copy button. It copies `hisar://<vault>/<item>/<field>`, encoded as above, with the vault's name and the item's title, or their IDs when the name is shared by another vault, or the title by another item in the vault. A custom field whose label is a built-in label or is used twice in the item cannot be referenced, and the button says why. A reference is not a secret, so the clipboard is not cleared after 90 s. The SSH key view shows a "Private key" row that says it never leaves Hisar, without the key and without a reference button.

#### 9.4.2 Coding agents: `hisar mcp`

`hisar mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server on stdio, so coding agents (Claude Code, Cursor, Codex, VS Code and other MCP clients) can use Hisar. The principle: **agents use secrets without seeing them**. They find items by their metadata and run commands with secrets that are masked in the output, as `hisar run` does (§9.4.1); a value reaches the agent only through `get_secret`, which the user approves for that one item in Hisar's approval dialog.

**Server.** `hisar mcp` uses the official Go SDK (`github.com/modelcontextprotocol/go-sdk`, `cli/mcpserver`) and always works through the desktop app: there is no standalone mode, because the app owns the approvals. It connects to `~/.hisar/cli.sock` on its first tool call (not at start, so an agent that never uses Hisar never opens a dialog), keeps that one connection, and sends its requests one at a time; after a lost connection the next call connects again, which needs a new approval. Server name `hisar`, title "Hisar", the CLI's version, and instructions for the model: prefer `run_with_secrets`; never ask the user to paste a secret; `get_secret` is for when the value itself must be shown. If the app is not running, does not let the command-line tool use it, or is signed out, a tool fails with a message the model can relay.

**Agent visibility.** Each vault has an "Available to agents" switch, per device, kept in `desktop.json` (`agentVaults`: the vault IDs; Reset at sign-out clears it). It is off for every vault until the user turns it on, in Settings → Developer tools → Coding agents, which lists the vaults with the switch and the line "Agents can find items in these vaults and use their secrets after you approve; other vaults stay invisible to them." Agent requests only ever see vaults with the switch on; every other vault is absent, never "forbidden": `list_vaults` leaves it out, `search_items` never finds its items, and a reference into it fails exactly like a vault that does not exist. With no vault available, the tools say how to turn one on.

**Tools.** Inputs and outputs are JSON with schemas; results are also returned as text.

| Tool | Input | Output |
|---|---|---|
| `list_vaults` | — | `vaults`: `[{id, name, itemCount}]` of the agent-visible vaults; `message` when there are none |
| `search_items` | `query` (matches title, username, URLs and vault name, ignoring case; empty lists everything), optional `vault` (name or ID) | `items`: `[{id, vaultId, vaultName, title, type (login, ssh-key, …), username, urls, hasNotes, hasOneTimeCode, fields: [{label, reference}]}]`, at most 50 with `totalCount`. Never a value and never the notes, which may hold secrets; `fields` lists the referenceable fields (username, password, one-time code, custom fields, public key, fingerprint) with their `hisar://` reference, built as "Copy secret reference" does (§9.4.1) |
| `run_with_secrets` | `command` (argv array; no shell), `env` (map of variable name → `hisar://` reference), optional `workingDirectory`, optional `timeoutSeconds` (default 120, at most 600) | `exitCode`, `stdout`, `stderr`, `isStdoutTruncated`, `isStderrTruncated`, `durationSeconds`, `isTimedOut` |
| `get_one_time_code` | `item`: a `hisar://` reference to any field of the item, or the item's ID | `code`, `secondsRemaining`, `period` |
| `get_secret` | `reference` | `value` |

- `run_with_secrets` runs like `hisar run` with masking always on, in the `hisar mcp` process: the command is looked up on `PATH` (a missing command fails before anything is asked), gets `hisar mcp`'s environment plus the resolved variables, no stdin, and its own process group. Every value is replaced with `<concealed by hisar>` before the output is kept; stdout and stderr are each kept up to 64 KiB (the first and last 32 KiB with "[… N bytes not shown …]" between them). At the timeout the group gets SIGTERM, then SIGKILL after 5 s, and `isTimedOut` is set. Variable names are `[A-Za-z_][A-Za-z0-9_]*`, at most 100; every value must be a reference.
- `get_secret` refuses `private key` like every reference (§9.4.1). A reference outside the agent-visible vaults fails as not found.

**App protocol.** The agent requests are on the CLI socket (§9.4). A connection becomes an agent connection with `agent.hello` (`agentName`, `agentTitle`, `agentVersion`: the MCP client's `clientInfo` from `initialize`); after that it may send only `hello` and `agent.*` requests, and other connections may not send `agent.*` requests (`invalid-request`).

| Type | Parameters | Result |
|---|---|---|
| `agent.hello` | `agentName`, `agentTitle`, `agentVersion` | `{}`; never needs an approval |
| `agent.vaults.list` | — | `vaults`: `[{id, name, itemCount}]` |
| `agent.items.search` | `query`, optional `vault` | `items`, `totalCount` |
| `agent.secrets.resolve` | `references` (at most 100), `commandLine`, `workingDirectory` | `values` |
| `agent.items.code` | `references` (one) or `itemID` | `code`, `expiresAt`, `period` |
| `agent.secrets.reveal` | `references` (one) | `values` |

**Approval model.** The app checks every request; `hisar mcp` only relays.

- *The agent connection* needs a first approval, like a terminal session (§9.4), but the dialog names the agent: "Claude Code wants to use Hisar", subtitle "via hisar mcp in Terminal". The agent's name is its `clientInfo` title, else a known display name for its `clientInfo` name (`claude-code` → Claude Code, `cursor-vscode` → Cursor, `codex-mcp-client` → Codex, …), else the name itself (printable characters, at most 60), else "A coding agent". The app around it (`in Terminal`) comes from `desktop/appinfo`, starting from the parent of `hisar mcp`, and is left out when it has the agent's name. The detail box shows the `hisar` path and terminal device. The agent's name is what it says about itself; the app and path come from the system. The approval lasts until 10 minutes after the connection's last request, until the connection closes, or until Hisar locks, signs out or quits. It is keyed by the connection, so it never covers a terminal session, and a terminal approval never covers an agent.
- *After that*, `list_vaults` and `search_items` need nothing more.
- *`run_with_secrets` and `get_one_time_code`* ask per call: "Claude Code wants to run a command with 2 secrets" with the command (shell-quoted and shortened like §9.4.1's), the working directory and the secrets as "vault › item › field", or "wants the one-time code of “GitHub”". The checkbox "Allow these secrets for this agent for 1 hour" (off by default) lets later calls of this connection that use only those secrets (vault, item and field) run without asking for an hour.
- *`get_secret`* always asks, per item: "Claude Code wants to read “Stripe › api key”" with the warning "The value will be visible to Claude Code and can end up in its transcript and logs." The checkbox "Allow this item for 1 hour" (off by default) lets this connection read any field of that item for an hour without asking. Reading is never covered by a "use" approval, and the reverse.
- Remembered secrets and items skip the dialog only while the connection's own approval is current.
- When the first request of a connection is one of these, its dialog also approves the connection ("Approving also lets Claude Code list and search the vaults available to agents"), so the user sees one dialog, not two.
- References are checked before anything is asked: a reference that does not resolve among the agent-visible vaults, or `private key`, fails without a dialog. While Hisar is locked the dialog unlocks it first, as for the CLI (§9.4), with the references as written; they are checked after the unlock.
- Touch ID, the master password and the iPhone (§9.9) answer these dialogs like the other approvals; the phone request is a `cli` request whose app is "Claude Code (via hisar mcp) in Terminal". Remembered secrets and items end when Hisar locks, signs out or quits, or when the connection closes.
- A denial, Deny, Escape or the timeout fails the tool with "The user denied access in Hisar." (or "… did not answer Hisar's approval dialog in time."), which the model can relay.

**Activity log.** Hisar has no local activity or audit view yet, and the server's audit log (§8.3) must not learn which items an agent used, so agent requests are not recorded; an on-device activity log is a follow-up.

**Limits.** "Use without seeing" holds only as far as the command allows: a command run with secrets has the real values and can write them to a file, send them over the network, or print them encoded, none of which masking catches. That is why every `run_with_secrets` dialog shows the command and its working directory: approve only commands you would run yourself. An agent with a shell can also run `hisar` itself in an approved terminal session (§9.4.1). Remembering secrets for an hour lets the agent run any command with them during that hour.

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
- **Add to GitHub / Add to GitLab** copy the public key and open `https://github.com/settings/ssh/new` or `https://gitlab.com/-/user_settings/ssh_keys`, with the hint to paste it and choose the key type there (GitHub: "Authentication Key" for SSH, again as "Signing Key" for commit signing; GitLab: "Authentication & Signing").
- **Set up commit signing** unfolds a panel with the exact commands, each with a copy button, and **Copy all**:

  ```
  git config --global gpg.format ssh
  git config --global user.signingkey "key::<key type> <base64 key>"
  git config --global commit.gpgsign true
  mkdir -p ~/.ssh && echo "$(git config --get user.email) namespaces=\"git\" <key type> <base64 key>" >> ~/.ssh/allowed_signers
  git config --global gpg.ssh.allowedSignersFile ~/.ssh/allowed_signers
  ```

  The key is the public key without its comment, so the commands need no further quoting. Hisar never runs them: the user pastes them into a terminal, and the `allowed_signers` line lets `git log --show-signature` verify the user's own signatures. Signing then goes through the agent and asks as described under Approval.

**Agent**

- Implements the OpenSSH agent protocol with `golang.org/x/crypto/ssh/agent`, including the `session-bind@openssh.com` extension (OpenSSH 8.9 and later, `PROTOCOL.agent`): `string hostkey, string session_id, string signature, bool is_forwarding`. As `ssh-agent` does, the agent verifies the signature of the session ID with the host key (any algorithm the key allows), keeps at most 16 bindings per connection, accepts the same binding again but refuses one for the same session with a different host key or flag, and refuses any binding after one for user authentication (`is_forwarding` false). A user authentication request (`publickey`, or `publickey-hostbound-v00@openssh.com` with its host key) on a connection whose last binding is for authentication must carry that binding's session ID and host key. Clients that never send it (older OpenSSH, other SSH libraries, `ssh-keygen`) get the behaviour without it: no verified destination, and the request counts as local.
- Listens on `~/.hisar/agent.sock` (directory 0700, socket 0600; `HISAR_SSH_AGENT_SOCKET` overrides the path). It starts with the app and stops when the app quits; the app opens at login and keeps running without a window (§9.1.2), because an SSH client cannot start it. If the socket cannot be created (for example another agent is listening there), Settings shows the error.
- Offers the SSH keys of every vault the user can access while Hisar is unlocked; personal vault keys come first. Locked, a request for keys or a signature opens the approval dialog, which unlocks Hisar (see Approval); signed out, it offers no keys. It is read-only: requests to add or remove keys, and the agent's lock/unlock requests, are refused.
- Settings → SSH & Git shows the status and, only when the user clicks "Use Hisar for SSH", adds this section to the top of `~/.ssh/config` after copying the file to `~/.ssh/config.hisar-backup-<timestamp>` (the file is created with mode 0600 and `~/.ssh` with 0700 if missing):

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

Every use of a key asks, unless an approval for the same key and app is remembered. The dialog says who asks, for what and where, like "Terminal wants to push to git.tryhisar.com".

- **The app.** Every approval (SSH sign-in and push, commit and tag signing, the command-line tool, `hisar run`, `hisar mcp`, share-link creation from the command line) names the GUI app responsible for the request, so "iTerm2 wants to sign a commit" rather than "ssh-keygen wants to sign a commit". The agent and the command-line socket read the peer PID (`LOCAL_PEERPID` on macOS, `SO_PEERCRED` on Linux), and `desktop/appinfo` resolves it (`appinfo.Resolver`):
  1. It reads up to 16 processes from the peer to its oldest ancestor before `launchd` (path from the kernel, arguments and working directory).
  2. The first process, nearest first, whose outermost `.app` bundle is a running app with a Dock presence (`NSRunningApplication`, activation policy regular) is the app; so is the app macOS holds responsible for a process (`responsibility_get_pid_responsible_for_pid`, which maps Electron and VS Code helpers and what they start to the app). `Code Helper (Plugin).app` inside `Visual Studio Code.app` resolves to Visual Studio Code; Cursor, Zed, JetBrains IDEs, Terminal, Ghostty, Warp, WezTerm, Alacritty and kitty come out as themselves. Developer tools and toolchains are walked past: anything under `/Library/Developer/` or `*.app/Contents/Developer/` (`/usr/bin/git` is an `xcrun` shim that runs Xcode's `git`), programs outside bundles and Hisar's own helpers (`Hisar.app/Contents/Helpers/`, `Hisar.app/Contents/MacOS/hisar-*`). Without a Dock app in the chain, the nearest bundle that is not a developer tool counts (a menu bar app).
  3. A chain whose oldest process left its parent behind (its parent is `launchd`) is followed further: iTerm2 runs its shells under `iTermServer-<version>` in `~/Library/Application Support/iTerm2/`, a daemon that outlives iTerm2's restarts and whose parent is `launchd`, so the parent walk alone never reached iTerm2 and the dialog showed `ssh-keygen`. A daemon outside any bundle whose code signature is valid and carries a Team ID belongs to the one running Dock app signed with the same Team ID (iTermServer and iTerm2 are both signed by H7V7XYVQ7D); with none or several, it does not count. A `tmux` or `screen` server leads to its clients: when all running `tmux` (or `screen`) clients resolve to one app, that app, with `tmux` last in the chain; otherwise "tmux". An `sshd` (or `sshd-session`) in the chain makes it "an SSH session from <host>" (the remote address of the session's TCP socket, read with `proc_pidfdinfo`), or "an SSH session". Anything else with `launchd`, `cron` or `launchd`-started jobs above it is "a background process"; without a parent chain it is "a process without an app".
  - The headline name and icon come from the bundle on disk, never from `argv[0]`, the environment or the request: the name Finder shows, or the bundle's `CFBundleDisplayName`/`CFBundleName` when that extends it (iTerm.app is "iTerm2"; Visual Studio Code.app's "Code" does not count), and `NSWorkspace iconForFile:` rendered as a PNG, both cached per bundle path. The executable path comes from the kernel (`proc_pidpath`, symlinks resolved), not from the path the process was started with. The code signature of the bundle (`SecStaticCodeCheckValidity` without resources, cached per bundle path and modification time) gives "Signed by Apple", "Signed by <developer> (<Team ID>)", "Not signed" or "Signature not valid"; the last two show a warning, so a fake "iTerm2" with a copied icon but no valid signature stands out.
  - The chain under the app is listed by program name (from the kernel's executable path) nearest first, for example `ssh-keygen ← git ← zsh`, without the app's own processes, `login` and the daemon used in step 3; each program's arguments (passwords redacted as for the command) are in its tooltip.
  - The bundle path is also the app approvals are remembered for (below).
- **The purpose.** From the same process chain (path, arguments and working directory of each process; never the environment) and the signed data, the approver derives:
  - `ssh … git-receive-pack '<repository>'` (or `git-lfs-authenticate … upload`): "push to <host>"; `git-upload-pack`, `git-upload-archive` or LFS `download`: "pull from" when a `git pull` is above it, "clone from" for `git clone`, else "fetch from";
  - any other `ssh` to a host: "sign in to <host>";
  - signed data in the SSHSIG format (`ssh-keygen -Y sign`) with the namespace `git`: "sign a commit", or "sign the tag <name>". `git` passes the commit or tag to `ssh-keygen -Y sign -n git -f <key> <file>` in a temporary file; the approver reads it (a regular file of at most 64 KiB, the last argument of an `ssh-keygen -Y sign` process in the chain) for the subject line, and the branch from the repository's `HEAD`. Without it, the dialog says "a commit";
  - anything else: "use your SSH key", with the destination when a session binding names one.
  - The host comes from the `ssh` arguments (the destination, `-p`, `-o Port=`, `ssh://host:port`, port 22 by default), else from the session binding (the host key looked up in `~/.ssh/known_hosts` and `/etc/ssh/ssh_known_hosts`, `[host]:port` entries included), else the host key fingerprint. The command is the outermost `git` process's arguments (or `ssh`'s when there is no `git`), quoted like a shell would, at most 160 characters, with passwords in URLs and the values of `-c` options whose names mention a token, password, secret, header or auth replaced by `•••`. The repository is the working directory of the `git` process next to `ssh` or `ssh-keygen` (git runs in the top directory of the work tree), with `$HOME` shown as `~`.
- **The dialog** (`?window=ssh-approval`, a window created hidden at launch, reused, always on top, 480 points wide, its height fitted to the content, about 400): the title "<App> wants to <purpose> <host>", the subtitle `with your SSH key "<key title>"`, the app's icon with a small Hisar badge, under the title a line with the process chain (`ssh-keygen ← git ← zsh`) and the app's signature ("Signed by Apple", "Signed by George Nachman (H7V7XYVQ7D)"; a red warning line for an unsigned app or one whose signature does not verify), a two-line monospace detail box (the command, e.g. `git push origin main`; then `<repository> · <host>:<port>`; for commit signing the subject line, then `<repository> · <branch>`), the checkbox "Don't ask again for <App> for 1 hour" (checked by default), and a footer bar with the Touch ID glyph (Apple's `LAAuthenticationView`, embedded through `biometric.WindowPrompt` over a slot the page measures), "Touch ID to approve" / "or use your master password", and Deny. Touch ID starts as soon as the dialog shows a request, so touching the sensor approves without a click, as in 1Password; the reason is `allow "<app>" to <purpose> <host>` (one Touch ID prompt at a time). If it fails, the dialog stays open with a message and Try again restarts it. "or use your master password" shows a password field instead (Return submits it; the password is checked against the account, offline from the cache). Without Touch ID, an Approve button approves. Escape, Deny and closing the window deny, and cancel a Touch ID check in progress. The checkbox can be changed while Touch ID waits; its value when the request is approved counts.
- **Compact approvals** (opt-in: Settings → SSH & Git → "Compact SSH approvals", off by default, stored in `desktop.json` and kept on sign-out): a card about 380 points wide at the top right of the display under the pointer, like a notification (16 points inside the visible area: below the menu bar and notch, clear of a Dock on the right; when its content changes height the top-right corner stays put): the app's icon, "<App> · <action>", `<host> with "<key title>"`, the warning line of an unsigned app, and inline "Touch ID to allow for 1 hour" (for a commit, "Touch ID to sign") and Deny. Clicking the card expands it into the full dialog in the middle of the same display; the full dialog otherwise opens in the middle of the display under the pointer. Only requests that need nothing else use it: Hisar unlocked, Touch ID available, not forwarded; the others always open the full dialog.
- **Commit signing** asks once per signature by default, and is never covered by a remembered SSH approval: the dialog shows a "Sign a commit" (or "Sign a tag") badge and no checkbox.
- **Remembered commit signing** (opt-in: Settings → SSH & Git → "Remember commit signing": **Always ask** by default, **Up to 10 minutes** or **Up to 1 hour**; `signingRememberSeconds` in `desktop.json`, a device preference kept on sign-out). Only with the setting on does the commit signing dialog show the checkbox "Don't ask again for <App> in <repository> for 10 minutes" (or 1 hour), unchecked every time: the setting is the deliberate opt-in, and the checkbox scopes each use of it. Checked when the signature is approved, later commit signatures are signed without asking while all of these match:
  - the app: the bundle path that app attribution resolved (above). A request without a resolved app ("a background process", an SSH session, tmux with clients in several apps, a program outside any bundle) never offers it, so it never spans apps or reaches a process the user cannot see;
  - the repository: the working directory of the `git` process next to `ssh-keygen` (the purpose's repository); a signature without one never offers it;
  - the key fingerprint.

  It lasts the chosen time from the approval, not from the last use, and ends earlier when Hisar locks (the lock hook calls the approver's `ClearAllRememberedSigning` next to the agent's `ClearAllApproval`) or when the setting changes. Tag signatures, forwarded requests and keys set to "ask every time" always ask. A `git rebase` (likewise `cherry-pick` or `am`) with `commit.gpgsign` runs `ssh-keygen -Y sign` once per commit, each a child of the same `git` in the same work tree and app, so one approval with the checkbox covers the whole run; without the checkbox it asks per commit as before. The compact card never offers it (its inline Touch ID signs once); expanding the card shows the checkbox. When Hisar is locked, the unlock dialog for a commit signature offers it too, since the signature names its key.
- **Locked.** When Hisar is locked and the user is signed in, a request for keys or a signature opens the dialog with "Unlock Hisar to use your SSH keys" instead of failing, and the dialog unlocks Hisar the way it does for the command-line tool (§9.4): with Touch ID when Touch ID unlock is ready (§5.4; one Touch ID check unlocks and approves), otherwise with the master password typed into the dialog (Return submits; a wrong password keeps the request open with a message). The unlock approves that agent connection, so the signature that follows on it does not ask again, and the checkbox is remembered for the key that then signs. Deny or the timeout leave that connection without keys, as before. Signed out, the agent offers no keys and shows nothing.
- **Forwarded requests.** A connection with a binding whose `is_forwarding` is true is a request from a remote host through agent forwarding (`ssh -A`): the local `ssh` binds the forwarded connection to the host it connected to. The dialog then shows a red banner "Forwarded from another computer — this request comes from <host> through agent forwarding, not from this Mac" (the host of the first forwarding binding from `known_hosts`, else the local `ssh`'s destination, else the host key fingerprint), offers only "Allow once" (no checkbox, never remembered), and Deny is the default button: Return denies, and Touch ID starts only after "Allow once". It never uses the compact layout. The docs recommend `ProxyJump` over agent forwarding.
- **Remembering.** "Don't ask again" remembers the approval per (key fingerprint, app): the app bundle's path, or the program's path when there is no app; a program without a known path is never remembered. It lasts one hour (`DefaultApprovalLifetime`) or until Hisar locks, whichever comes first; the vault backend calls the agent's `ClearAllApproval` from its lock hook. Auto-lock counts input anywhere on the Mac (§9.1), so the hour holds while the user works in other apps. Forwarded requests, commit and tag signatures (see Remembered commit signing), and keys set to "ask every time" (`SetAllAlwaysAskKeyID`; no UI yet) are never remembered here.
- **Approving on the iPhone.** While Hisar is unlocked, the dialog is also sent to the account's iPhones and its footer adds "or approve on your iPhone" (§9.9). A phone's approval counts as a Touch ID approval, a phone's Deny as Deny. Whether a phone's approval is remembered is the choice made on the phone, where the request shows it as a switch ("Also allows iTerm2 for 1 hour"), never the Mac's checkbox (§9.9).
- **Events.** The backend emits `ssh-approval` (with a random request ID and the request as shown), `ssh-approval-failed` and `ssh-approval-closed`, and waits for `RespondSSHApproval(requestID, isAuthorized, isRemembered)` or `RespondApprovalWithPassword(requestID, password)`; `SetSSHApprovalRemembered` follows the checkbox, `SetSigningRemember(seconds)` stores the Settings choice (0, 600 or 3600; the status reports `signingRememberSeconds`) and `FitSSHApprovalWindow(isCompact, height)` sizes and places the window (centered, or at the top right). Each request has its own hard deadline, set when it arrives: 60 seconds, or 2 minutes when the iPhone can answer (§9.9). The deadline runs in the backend whatever the window does: shown, hidden, still waiting behind earlier requests, or not drawn at all because the screen is locked or asleep. At the deadline, or as soon as the client disconnects (the agent and the command-line socket watch for the peer closing while a request waits), the request is denied, a Touch ID check in progress for it is cancelled, it leaves the window and its phone request is withdrawn. The window shows requests one at a time in the order they arrived; a stuck request holds the ones behind it at most until its own deadline, and each of them is already on the phone, which can answer them in any order. The window hides when no request is left. Showing the window and sending its events never hold up a request: they run in order on their own goroutine. `ListAllPendingSSHApproval` lets the window show requests that arrived before it loaded. The same dialog handles the command-line tool's requests (§9.4).

**Implementation**

- `core/sshagent` implements `agent.ExtendedAgent` over two interfaces: a key source and an approver. It has no UI dependency and is tested in CI with an in-memory approver and a real `x/crypto/ssh` client and server.
- `core/app` stores SSH Key items (`GenerateSSHKey`, `ImportSSHKey`) and provides the key source, `app.SSHKeySource`. It is a separate type from the bound `app.Service`, so no method callable from the UI returns private keys.
- `desktop/agent` holds the approver (Touch ID or in-app, with the Wails event emitter injected), the purpose detection, the `~/.ssh/config` editing and the bound status service. It does not import Wails and is unit-tested with fake process trees; `main.go` wires it to Touch ID, the window and the socket. `desktop/appinfo` reads the process chain (`proc_pidpath` for the executable, `kern.procargs2` for the arguments, `proc_pidinfo` for the working directory on macOS; `/proc` on Linux) and resolves the responsible app (`appinfo.Resolver` over an injected `appinfo.System`, so the attribution rules are unit-tested with fake process tables; the AppKit and Security code builds only for `darwin && !ios && cgo`, because `desktop/agent` is also linked into the iPhone framework). Linux has no Dock apps or signatures: it shows SSH sessions, `tmux`, background processes and bundles found in the chain.
- `session-bind@openssh.com` is tested with fixtures recorded from a real OpenSSH client and `sshd` (`core/sshagent/testdata/openssh`: a direct login, a forwarded agent with a second hop, and `ssh-keygen -Y sign -n git`), which `HISAR_RECORD_OPENSSH_FIXTURE=1 go test -run TestRecordOpenSSHFixture ./sshagent/` records again where `ssh`, `ssh-keygen` and `sshd` are installed, and with hand-built messages for the error cases.
- The end-to-end test generates a key in a vault, serves it with the agent on a Unix socket and logs in to an in-process `x/crypto/ssh` server with a real SSH client; after Lock the agent lists no keys.
- Linux uses the same Unix socket; Windows (named pipe `\\.\pipe\openssh-ssh-agent`) comes later. The CLI gets `hisar ssh-key generate|import|list|public-key`.

### 9.7 Import from other password managers

`core/importer` turns an export into items. It is pure Go: it reads only the bytes it is given, never logs, and its errors carry positions (CSV line, JSON byte offset) but never values from the file.

| Format | Detected by | Imported | Not imported (reported with a reason) |
|---|---|---|---|
| 1Password `.1pux` (ZIP with `export.data`) | ZIP signature | Login and Password items with extra login fields and section fields (concealed fields stay concealed); Secure Notes; Server, Database, Wireless Router, Email Account, API Credential and Software License items as logins with their fields; Credit Cards (cardholder, number, expiry, verification number, PIN and billing address as the card's fields, the rest as custom fields); SSH Keys with a private key | identities, documents and other personal records; archived items; attachments |
| 1Password CSV | header with `Title`, `OTPAuth` and `Archived` | title, URL, username, password, one-time password, tags, notes | archived rows |
| Bitwarden JSON (not encrypted) | JSON with `items` | logins with all URIs, notes, custom fields (hidden ones concealed, linked ones dropped), TOTP; secure notes; cards (cardholder, number, expiry and security code; the brand comes from the number); SSH keys; folder as tag | encrypted exports (the whole file is refused), identities, passphrase-protected SSH keys |
| Bitwarden CSV | `login_username`/`login_password`/`login_uri` columns | as JSON, without hidden-field markers | cards (the CSV leaves out their details; export as JSON), rows of other types |
| CSV (Chrome, Edge, Firefox, Safari, LastPass and others) | a header row naming the columns | title (or the website's host), username, password, URL, notes, one-time password; other columns become fields. Comma, semicolon or tab separated; BOM, CRLF and quoted multiline values | rows with no content |
| Hisar export, encrypted or plain JSON (§9.11; CLI) | JSON with `format` `hisar-encrypted-export` or `hisar-export` | every item of every vault, vault by vault (below) | — |
| `.env` (CLI) | the file name, or `-format dotenv` | the chosen variables as fields of one item; the file is rewritten to secret references (below) | empty values and existing references |

- Secure notes are saved as `secure-note` items (§4.5): 1Password Secure Notes, Bitwarden secure notes (JSON type 2 and CSV `type` `note`) and LastPass `http://sn` rows, even with only custom fields or nothing but a title, and any other entry with nothing but notes (and custom fields). A source note that comes with a username, password or website stays a login, so they are shown.
- An entry that repeats an earlier one exactly is reported, not imported twice. Items already in the target vault are matched by title, username and the first website (case and `www.`/scheme ignored; SSH keys by fingerprint) and skipped unless the user includes them.
- One-time passwords (1Password `totp` fields and `otpauth://` values, Bitwarden `login.totp`, a CSV `otpauth`/`totp`/`otp` column) go through `totp.Normalize` into `details.oneTimePassword` (§4.5). One that does not parse, such as a Steam code, or a second one on the same item, stays a concealed field.
- SSH private keys go through `sshagent.ImportPrivateKey` and are stored as OpenSSH keys, as in §9.6.
- Desktop (`core/app/import.go`): the file comes from a native open dialog; the parsed items stay in memory only until the import finishes, is cancelled or Hisar locks. Items are saved in batches of 10 called by the UI, so it can show progress, and connection, permission and rate-limit errors pause the import instead of failing every remaining item. Afterwards the app offers to delete the export file.
- CLI: `hisar import <file> [-v vault] [-format f] [-y] [-include-duplicates]` parses the file before asking for the master password and asks before saving and before deleting the file; `-y` imports without questions and never deletes.

**Hisar's own export** (§9.11, CLI only). `hisar import` recognises both formats by their `format` field, whatever the file is called (`-format` set to another format skips this). An encrypted export asks for the export passphrase (3 tries in a terminal, one line from stdin otherwise) before anything else; a wrong one fails with "that passphrase doesn't open this export" and contacts nothing. Each exported vault goes to the user's vault of the same name when they can write to it, the exported Personal vault to the Personal vault, and any other vault to a new vault with its name ("<name> (imported)" when a vault of that name is read-only); `-v` puts every item into that one vault. The preview lists "Personal → Personal: 12 items, 2 already there" per vault; items already there are matched as above and skipped unless included. Items keep everything the export holds (fields, one-time passwords, SSH private keys, passkeys) but get new IDs. A plain export is offered for deletion afterwards like other exports; an encrypted one is kept.

**`.env` files** (CLI). `hisar import .env [--vault v --item i] [--keys K1,K2] [--dry-run] [-y]` moves secrets out of a dotenv file into one item and leaves references behind, for `hisar run --env-file` (§9.4.1). A file named `.env`, `.env.*` or `*.env`, or `-format dotenv`, is read as dotenv with the syntax of §9.4.1.

- *Choosing.* Every variable with a value is a candidate; empty ones and values that are already `hisar://` references are kept as they are. Values that look like configuration start unchosen: `PORT`, `HOST`, `NODE_ENV` and similar names, names ending in `_PORT`, `_HOST`, `_ENV`, `_MODE`, `_LEVEL`, `_REGION`, `_TIMEOUT`, `_VERSION`, `_DEBUG`, `_ENABLED`, `_NAME`, `_URL`, `_URI`, `_ENDPOINT`, `_DOMAIN`, `_ORIGIN`, `_ADDR` or `_ADDRESS`, numbers, `true`/`false`-like switches, and URLs without credentials. A name with a word such as SECRET, TOKEN, PASSWORD, PASS, KEY, PRIVATE, CREDENTIAL, AUTH, SALT, DSN or WEBHOOK, a URL with a password or a credential-like query parameter, and everything else start chosen. In a terminal the list shows `[x]`/`[ ]` per variable with the reason and takes numbers to switch them; `--keys` chooses exactly those names.
- *Where.* In a terminal it asks for the vault (the writable ones, Personal first) and the item name (the directory's name, plus the environment for `.env.production`); otherwise `--vault` and `--item` are required. An existing item with that title is updated (a field with the same label is replaced, others are kept); otherwise a login with only these fields is created. Each value becomes a concealed field labelled with the variable's name; `PASSWORD`, `USERNAME`, `NOTES` and `URL` fill the built-in field a reference with that name reads, and `OTP`, `CODE` and `FINGERPRINT` are refused because a reference with that name means something else. Before saving, every chosen reference is resolved against the item as it will be saved.
- *Rewriting.* Each chosen line becomes `KEY=hisar://<vault>/<item>/<KEY>`, encoded like "Copy secret reference" (the vault's ID when its name is shared by another vault). Everything else stays byte for byte: comments, blank lines, order, `export` prefixes, indentation, the original quotes around the new value, a comment after the value, unchosen lines, CRLF line endings and a byte order mark. A multiline quoted value becomes one line. A name set more than once is rewritten everywhere and stores the last value, the one `hisar run` reads, and the plan says so when the values differed. The file is written to a temporary file in the same directory with the original mode and renamed over it (through a symlink, to its target), so it is never half-written and no copy of the old content is kept. The item is saved before the file is rewritten; if the rewrite fails, the message says the values are already stored.
- *Plan.* The command prints what it will store and every rewritten line before it asks "Continue?" (`-y` skips it; without a terminal `-y` is required). `--dry-run` prints the plan and writes nothing; with `--vault` and `--item` it contacts neither the app nor the server. Afterwards it prints "Run a command with them: hisar run --env-file .env -- <command>".

### 9.8 Passkeys (WebAuthn)

Hisar is a passkey provider for Chromium browsers: the extension takes over `navigator.credentials.create` and `get` for public-key credentials, and the desktop app holds the keys and signs. Private keys never leave the desktop's Go backend; neither the extension nor the UI sees them. On iOS the AutoFill extension is a passkey credential provider for Safari and apps, with the same authenticator and the passkeys already stored in login items; there the keys stay in the Go backend linked into the app and the extension (see **iOS** below).

**Authenticator**

| Property | Value |
|---|---|
| Algorithm | ES256 only (COSE `-7`, P-256). A `create` whose non-empty `pubKeyCredParams` lacks `-7` is left to the browser |
| AAGUID | `fb041679-042a-49ae-b616-8779e2ac14ce`, fixed for Hisar |
| Credential ID | 32 random bytes; every credential is discoverable (stored with its user handle) |
| Sign count | always 0, as for other synced passkeys; relying parties then skip the counter check |
| Flags | UP, UV, BE and BS; AT on create |
| Attestation | `none` (`fmt: "none"`, empty `attStmt`), whatever the relying party asks |
| Extensions | `prf` (below); `credProps` reports `{rk: true}` when asked on create; others are ignored |
| Transports | `["hybrid", "internal"]`, attachment `platform` |

- **User presence and verification:** every create and get needs a click in Hisar's prompt in the page (presence), and Hisar must be unlocked, which took the master password or Touch ID (verification). So UV is set whatever `userVerification` asks. There is no extra Touch ID check per use: the SSH approval window (§9.6) would be the model, but it is built around per-process SSH approvals; "Ask for Touch ID before using a passkey" is a later option.
- `core/passkey` builds the authenticator data, the COSE key and the attestation object (CTAP2 canonical CBOR, hand-written for these few shapes) and signs with `crypto/ecdsa` (ASN.1 DER signature over `authenticatorData || SHA-256(clientDataJSON)`).

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
- **Signals and capabilities:** the same script defines or wraps `PublicKeyCredential.signalUnknownCredential`, `signalAllAcceptedCredentials`, `signalCurrentUserDetails` and `getClientCapabilities` (below).
- **Result:** the MAIN-world script builds the credential with `PublicKeyCredential.prototype` (and `AuthenticatorAttestationResponse.prototype` or `AuthenticatorAssertionResponse.prototype`) as prototypes and own properties, so `instanceof` checks pass: `id`, `rawId`, `type`, `authenticatorAttachment`, `response` (`clientDataJSON`; `attestationObject`, `getAuthenticatorData()`, `getPublicKey()` as SPKI DER, `getPublicKeyAlgorithm()` and `getTransports()` for create; `authenticatorData`, `signature` and `userHandle` for get), `getClientExtensionResults()` and `toJSON()`. PRF results are ArrayBuffers in `getClientExtensionResults()` and base64url in `toJSON()`.

**PRF extension** ([WebAuthn PRF extension](https://w3c.github.io/webauthn/#prf-extension))

Each passkey has a PRF secret that is derived from its private key, not stored, so passkeys created before PRF support have it too and every Hisar client (iOS later) derives the same value:

```
d          = the P-256 private scalar of item.Passkey.privateKey, 32 bytes big-endian
prfSecret  = HKDF-SHA256(IKM = d, salt = "" (empty), info = "hisar passkey prf v1", L = 32)
prf(input) = HMAC-SHA256(key = prfSecret, SHA-256("WebAuthn PRF" || 0x00 || input))
```

`prf(input)` applies the salt transformation the spec defines for hmac-secret to the page's input; these concatenations are the spec's plain byte concatenations, not length-prefixed (§4.5). `core/passkey` holds the derivation; the secret and the key never leave the desktop's Go backend.

- **Create:** when the request has `extensions.prf`, the result reports `prf: {enabled: true}`; when it has `prf.eval`, also `results` (`first`, and `second` when given) for the new credential. `prf.evalByCredential` on create rejects with `NotSupportedError`.
- **Get:** `prf.eval` and `prf.evalByCredential` (a record of base64url credential IDs). The MAIN-world script checks them as the spec's client does: a non-empty `evalByCredential` with an empty `allowCredentials` rejects with `NotSupportedError`; a key that is empty, not base64url, or not the ID of an `allowCredentials` entry rejects with `SyntaxError`. The desktop checks this again and evaluates only for the credential that signs: `evalByCredential[credentialId]` when present, else `eval`. The result has `prf: {results: {first[, second]}}`, or `prf: {}` when no input applies.
- Inputs are 0–1024 bytes; a larger one leaves the request to the browser.
- When the desktop is older and answers without `prf`, the page gets `prf: {enabled: false}` on create and `prf: {}` on get.

**Signals** ([WebAuthn signal methods](https://w3c.github.io/webauthn/#sctn-signal-methods))

- `signalUnknownCredential({rpId, credentialId})`, `signalAllAcceptedCredentials({rpId, userId, allAcceptedCredentialIds})` and `signalCurrentUserDetails({rpId, userId, name, displayName})` reject with `TypeError` when a required member is missing or an ID is not base64url. The service worker then checks `rpId` against the calling frame's origin from the message sender (as for create and get): it must equal the host or be a suffix of it at a label boundary with at least two labels (IP addresses: equal). Otherwise the promise rejects with `SecurityError`. The browser's own method, which the script also calls when it exists, applies the public suffix list, and the desktop checks again with `ResolveRelyingPartyID`. Valid signals resolve with `undefined` without waiting for Hisar.
- The service worker forwards a valid signal only when it already has a connection to Hisar and is paired: a signal never starts the native host or Hisar. Signals are best effort, so nothing reports back to the page.
- The desktop acts only when unlocked (without counting as activity for auto-lock); locked or signed out, it ignores the signal. It touches only passkeys whose RP ID equals the checked one, in login items of vaults the user can write:
  - unknown credential: removes that passkey from its item;
  - all accepted credentials: removes the passkeys with that RP ID and user handle whose credential ID is not in the list (an empty list removes all of them, as the spec says);
  - current user details: sets `userName` and `userDisplayName` (trimmed, at most 256 characters) of the passkeys with that RP ID and user handle.
- Changes go through the normal item save (new revision, synced, kept in the item history like any edit), without a prompt, and only when something changed, so sites that signal on every sign-in cause no writes. Nothing else in the item changes: an item whose only content was the passkey stays, without it.

**Client capabilities**

`PublicKeyCredential.getClientCapabilities()` returns the browser's capabilities (when the browser has the method) merged with Hisar's: `signalUnknownCredential`, `signalAllAcceptedCredentials` and `signalCurrentUserDetails` are always true (the script implements them); `passkeyPlatformAuthenticator`, `userVerifyingPlatformAuthenticator`, `conditionalGet` and `extension:prf` are true when the browser's are, or when Hisar is available (the same check as `isUserVerifyingPlatformAuthenticatorAvailable`: paired, connected, signed in, never starting the native host). `conditionalCreate` and every other capability stay as the browser reports them, since Hisar leaves conditional create to the browser.

**Messages** (extension ⇄ desktop, §9.2 framing, all need the token). Parameters are in a nested `passkey` object; binary values are base64url without padding.

| Type | `passkey` parameters | Result |
|---|---|---|
| `passkeys.list` | `origin`, `relyingPartyId`, `allowCredentials` | `passkeys`: `[{credentialId, vaultID, itemID, title, userName, userDisplayName}]` |
| `passkeys.prepare` | `origin`, `relyingPartyId`, `excludeCredentials` | `isExcluded`, `logins` (writable logins matching the origin) |
| `passkeys.create` | `origin`, `topOrigin`, `isCrossOrigin`, `relyingPartyId`, `userHandle`, `userName`, `userDisplayName`, `challenge`, `excludeCredentials`, optional `vaultID` + `itemID`, optional `prf` (`{eval?: {first, second?}}`) | `credentialId`, `clientDataJSON`, `attestationObject`, `authenticatorData`, `publicKey`, `publicKeyAlgorithm`, `vaultID`, `itemID`, and `prf` (`{enabled: true, results?: {first, second?}}`) when asked |
| `passkeys.get` | `origin`, `topOrigin`, `isCrossOrigin`, `relyingPartyId`, `challenge`, `allowCredentials`, `credentialId`, optional `prf` (`{eval?, evalByCredential?}`) | `credentialId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`, and `prf` (`{results?: {first, second?}}`) when asked |
| `passkeys.signalUnknownCredential` | `origin`, `relyingPartyId`, `credentialId` | `changedPasskeyCount` |
| `passkeys.signalAllAcceptedCredentials` | `origin`, `relyingPartyId`, `userHandle`, `allAcceptedCredentialIds` | `changedPasskeyCount` |
| `passkeys.signalCurrentUserDetails` | `origin`, `relyingPartyId`, `userHandle`, `userName`, `userDisplayName` | `changedPasskeyCount` |

The desktop checks the RP ID against the origin on each request, applies `allowCredentials` and `excludeCredentials` again, and signs only with a credential whose RP ID equals the request's. The signal requests answer `changedPasskeyCount: 0` when Hisar is locked or signed out.

**Desktop app and CLI**

- `core/app/passkey.go` finds, creates and signs. These are unexported methods or plain functions, so nothing bound to the UI returns a private key. `ItemDetail.passkeys` carries `{credentialId, relyingPartyId, userName, userDisplayName, createdAt}`.
- The item view lists the passkeys (site, user name, created) with Delete (`DeletePasskey(vaultID, itemID, credentialId)`, after a confirmation). The editor shows them read-only and the backend keeps them.
- `hisar get` prints `passkey: <rpId> (<user name>)` per passkey, never the key.

**iOS (AutoFill credential provider)**

- The extension declares `ProvidesPasskeys` next to `ProvidesPasswords` and `ProvidesOneTimeCodes`. iOS checks the site's or app's associated domain itself and hands the provider the RP ID and only `clientDataHash`, never `clientDataJSON` or the origin. Hisar signs over the hash it is given.
- **Same authenticator:** `core/passkey` has `CreateCredentialWithClientDataHash` and `SignAssertionWithClientDataHash` next to the browser functions, which build and hash their own `clientDataJSON` and then share the same code for the key, the authenticator data, the attestation object and the signature. AAGUID, flags, sign count 0, `none` attestation and ES256 are as in the table above. "none" attestation does not sign the hash, but a create still needs a 32-byte one.
- **RP ID check:** `passkey.ValidateRelyingPartyID`: a lower-case domain name of at most 253 characters, no IP address, port, path or wildcard, and not a public suffix (which also refuses single-label names such as `localhost`). There is no origin to compare it with; iOS has already bound it.
- **Go API** (`core/app/platformpasskey.go`, plain functions, wrapped by `hisarkit`; binary values base64url): `ListPasskeys` (every passkey in a login, with its user handle, for the identity store), `ListPasskeysForRelyingParty(rpId, allowedCredentialIDs)`, `PreparePasskeyRegistration(rpId, excludedCredentialIDs)` (`isExcluded` and the writable logins matching `https://<rpId>` by the §9.2 rules), `CreatePasskeyForRelyingParty({relyingPartyId, userName, userDisplayName, userHandle, clientDataHash, excludeCredentials, vaultID, itemID})` → `credentialId`, `attestationObject`, `authenticatorData`, `vaultID`, `itemID`, and `SignPasskeyForRelyingParty(rpId, credentialId, clientDataHash)` → `credentialId`, `authenticatorData`, `signature`, `userHandle`. The private key never crosses into Swift. Saving follows the desktop: the chosen login must be writable and match the RP, a passkey with the same RP ID and user handle in it is replaced, and with no login a new one is created in the Personal vault.
- **Identities:** with the password identities (§9.3) the app publishes an `ASPasskeyCredentialIdentity` (RP ID, user name, credential ID, user handle, record identifier `vaultID/itemID`) for each passkey after every sync, save, delete and passkey delete.
- **Signing in:** `prepareCredentialList(for:requestParameters:)` lists the passkeys for the request's RP ID (only those in `allowedCredentials` when it is not empty) above the matching logins. Choosing one signs and completes with `ASPasskeyAssertionCredential`. For a passkey chosen in the system sheet or QuickType bar, `provideCredentialWithoutUserInteraction` signs only while the extension's process is still unlocked; otherwise `prepareInterfaceToProvideCredential` unlocks with Face ID or the master password, then signs. As on the desktop, UV is set because Hisar is unlocked, and presence is the user's choice in the system UI.
- **Creating:** `prepareInterface(forPasskeyRegistration:)` unlocks if needed and shows "Save passkey" with the user name and the site, the writable logins matching the RP and "New login"; the login whose username equals the user name is preselected. A request whose non-empty `supportedAlgorithms` lacks ES256 is cancelled. The request carries `excludedCredentials`: when Hisar has one of them, the screen says so and "OK" cancels with `ASExtensionError.matchedExcludedCredential`; Go refuses such a create too. The result completes with `ASPasskeyRegistrationCredential`.
- **App:** the item view lists the passkeys (site, user name, created) with Delete after a confirmation, as on the desktop. The editor does not show them and the backend keeps them (§4.5).
- **Testing:** Go tests run both ceremonies from a `clientDataJSON` that go-webauthn builds and the test hashes, and verify them with go-webauthn. The simulator does not offer a build without a team as a provider, so the UI tests drive registration and sign-in through the debug-only AutoFill preview (§9.3) and check the attestation and the ES256 signature over `authenticatorData || clientDataHash` with CryptoKit. The real Safari flow needs the paid team (§12 #7) and is not verified yet.

**Not supported yet:** WebAuthn extensions other than `prf` and `credProps` (large blob, …), Firefox and Safari on the Mac, and on iOS the WebAuthn extension inputs of iOS 18 (`extensionInput`: PRF, large blob).

### 9.9 Approving on the iPhone

The Mac's approval dialog (SSH agent, §9.6, and command-line tool, §9.4) can also be answered on the user's iPhone, for example while coding agents run on the Mac and the user is away from its keyboard.

**Rules**

- The phone answers approve or deny for one specific request, nothing else. It never unlocks the Mac's Hisar, and no unlock key, password or vault key leaves a device: the answer is a signed decision.
- Only while Hisar on the Mac is unlocked. A locked Mac still needs Touch ID or the master password at the Mac, and its dialog is not sent to the phone. When the Mac locks, it withdraws its open phone requests and ignores their answers.
- Only while Hisar is open and unlocked on the phone, in the foreground. There are no push notifications yet: they need the Apple Developer Program (§12 #7) and come later.
- The Mac sends a request as soon as it receives it, before the dialog is shown and in parallel with it, if Settings → SSH & Git → **Approve requests on my iPhone** is on (default on; stored in `desktop.json`, a device preference kept on sign-out) and the account has at least one iOS device (`GET /devices`, platform `ios/…`, read when Hisar unlocks; the same list gives the Mac its own device ID). A list older than 5 minutes is read again in the background while the last one is used; when no list is known yet, or the last one had no iPhone and is stale, the request waits for a fresh list for at most 3 seconds, so the first request after unlock or after the Mac wakes is not kept from the phone by an old answer. A refresh that fails or hangs keeps the last list.
- Whichever answers first wins: Touch ID or the master password on the Mac, Deny on the Mac, the phone, or the dialog's timeout. A dialog that the phone can answer waits 2 minutes instead of 60 seconds (`PhoneApprovalTimeout`), to leave time to reach for the phone and open Hisar. The Mac then withdraws the request (`DELETE /approvals/{id}`), which sends `approval-resolved`, and the phone closes its sheet.
- A phone approval counts as a Touch ID approval, except for remembering: the phone shows what approving would also allow ("Also allows iTerm2 for 1 hour") as a switch that starts as the Mac's checkbox starts (on for SSH, off for remembered commit signing and for agent secrets), and only the choice signed into the phone's answer counts; the Mac's checkbox does not. An answer without that choice, from a phone built before it, is never remembered. Commit and tag signing ask once per signature unless remembered commit signing is on and chosen (§9.6), and a forwarded request (§9.6) is allowed once. A phone's Deny is Deny.

**Request** (`core/approval`, sealed by the Mac)

```
request       = JSON {requestId, requesterDeviceId, requesterDeviceName, createdAt, expiresAt,
                      kind ("ssh" | "cli"), appName, purpose, host, commandLine, repositoryPath,
                      keyName, isForwarded, forwardedFromHost, terminalName, secrets, appIcon, appWarning,
                      rememberLabel, isRememberedByDefault}
sealedRequest = 0x01 || HPKE.Seal(accountKEMPub, info = "hisar/v1/approval-request" || accountID || requestId, request)
```

- `requestId` is 128 random bits as 32 lower-case hex digits. `expiresAt` is the dialog's own deadline, so the request stays answerable exactly as long as the Mac waits; `expiresAt - createdAt` is at most 3 minutes. Times are RFC 3339 UTC with milliseconds.
- `purpose` is the dialog's sentence ("push to git.tryhisar.com", "sign a commit", "use Hisar", "run a command with 3 secrets"); the other fields are the dialog's detail lines, empty when the dialog has none. For `secrets.resolve` (§9.4.1), `commandLine` is the command `hisar run` starts and `secrets` lists the secrets as the dialog does, at most 4 and then "and N more"; it is left out of other requests. `appName` is the app as the dialog names it (from its bundle); `appIcon` is its icon as a 32-pixel PNG (base64 in the JSON) when that is at most 6 KiB, so the sealed request stays well under its 16 KiB limit, and is left out otherwise or when the sealed request would still exceed it; `appWarning` is the dialog's signature warning, if any. The phone shows the icon in place of its generic symbol and the warning as a detail row. `rememberLabel` is set only when approving can be remembered: the sentence for the phone's switch ("Also allows iTerm2 for 1 hour", "Also signs commits for iTerm2 in ~/src/hisar for 10 minutes", "Also allows these secrets for Claude Code for 1 hour"), and `isRememberedByDefault` is the Mac checkbox's starting value.
- Sealing to the account's own KEM public key (§4.3; the same HPKE as vault keys, §4.4) lets any unlocked device of the account open it, and the server cannot. The info binds the account and the request ID, so the server cannot move a request to another ID or account.
- Requests and answers are decoded leniently: unknown fields are ignored, so a Mac or phone that is one version behind still opens them. New fields must therefore be optional, and an older device must stay safe without them; the fields that are present are still checked (JSON types, one message with nothing after it, IDs, kind, lifetime, expiry, size). Ignoring them is safe because the answer's signature binds the SHA-256 of the exact sealed request and each side acts only on fields it understands. Phones built before this rule refuse requests with fields they do not know, such as `appIcon`.

**Answer** (signed and sealed by the phone)

```
requestHash  = SHA-256(sealedRequest)                      the exact bytes the server relayed
signed       = "hisar/v1/approval-answer" || accountID || requestId || requestHash || decision ("approve" | "deny")
               || answeringDeviceId || answeringDeviceName || answeredAt
               [|| remember ("remember" | "once")]         only when the answer carries isRemembered
signature    = Ed25519.Sign(account signing key, signed)
answer       = JSON {requestId, decision, answeringDeviceId, answeringDeviceName, answeredAt, requestHash, signature,
                     isRemembered (optional)}
sealedAnswer = 0x01 || HPKE.Seal(accountKEMPub, info = "hisar/v1/approval-answer-envelope" || accountID || requestId, answer)
```

The concatenations are length-prefixed (§4.5). Before it shows a request, the phone opens it and checks that its ID is the one the server named, that it lasts at most 3 minutes, that it has not expired by the phone's clock, and that it did not come from the phone itself. It signs only what it showed, because the hash covers the sealed request byte for byte. Sealing the answer as well keeps the decision and the device names from the server.

`isRemembered` is the phone's switch. A phone includes it, and the extra signed part, only for a request with a `rememberLabel`; an older Mac never sends one, so it still verifies the answers of newer phones. The Mac verifies the signature with the extra part when the answer has the field and without it otherwise, so the field cannot be added, removed or flipped without breaking the signature; an answer without it (an older phone) counts as "once". The Mac also ignores `isRemembered` for a request it did not offer to remember.

**The Mac accepts an answer only if**

1. it opens with the account's KEM key, and its `requestId` is a request the Mac is waiting for;
2. the signature verifies with the account's own signing public key, taken from the unlocked keyset, which was checked against the account record at unlock (§5.4); a key the server hands out later is never used;
3. `requestHash` equals the hash of the sealed request the Mac itself posted;
4. it arrives before `expiresAt` by the Mac's clock;
5. it is the first valid answer for that request: single use, so a replayed answer, or one for a finished request, is refused;
6. `answeringDeviceId` is not the Mac's own device ID.

`core/approval` implements these checks (`SealRequest`, `OpenRequest`, `CreateAnswer`, `Tracker`) on top of `core/crypt` (`SealApprovalRequest`, `SignApprovalAnswer`, `VerifyApprovalAnswer`, …), which has test vectors for the sealed request and the signature.

**What the server can and cannot do.** It relays sealed blobs. It sees the account, the requesting session's device, the request ID, sizes and times; never the description, the decision or the answering device. It cannot forge or change an approval, because it has no account signing key. It cannot reuse an answer for another request (the request ID and hash are signed) or later (expiry, single use). It can withhold, delay or drop requests and answers, and it can fill a request's single answer slot with garbage; all of that only means the Mac's own dialog decides, like any denial of service.

**Server relay** (`server/internal/service/approvals.go`, in memory)

| Endpoint | Body → result |
|---|---|
| `POST /approvals` | `{requestId, sealedRequest, expiresAt}` → 201. 400 for a bad ID, an empty sealed request or one over 16 KiB, or an expiry that is not within the next 3 minutes; 409 for a duplicate ID; 429 when the account has 16 open requests. Sends `approval-requested` |
| `GET /approvals` | `[{requestId, sealedRequest, requesterDeviceId, createdAt, expiresAt}]`: the account's requests that are not answered, resolved or expired |
| `GET /approvals/{id}` | `{requestId, sealedRequest, sealedAnswer, isResolved, expiresAt}`; 404 for an unknown request or another account's |
| `POST /approvals/{id}/answer` | `{sealedAnswer}` (at most 16 KiB) → 204. 404 unknown, 410 expired or resolved, 409 already answered, 403 from a session of the requesting device. Sends `approval-answered` |
| `DELETE /approvals/{id}` | → 204, only from a session of the requesting device (403 otherwise); marks it resolved and sends `approval-resolved` |

- Every endpoint needs a session and sees only the session's account. Each account has a token bucket of 2 requests per second with a burst of 60 across these endpoints (429).
- A request is kept until 60 s after its expiry, so at most 4 minutes after it was posted, and is never served after that; every write drops the expired ones.

**Mac** (`core/app/phoneapproval.go`, `desktop/agent`)

- When a request that needs no unlock arrives, the approver asks `PhoneApprover.IsPhoneApprovalAvailable` (unlocked, the switch on, and an iPhone on the account; refreshed as above, and not gated on the vault's last sync having been offline, since posting is the test); when it is, the vault backend posts the sealed request in the background right away, before the dialog is shown (a failed post is tried twice more, 2 s apart, while the request waits; a 409 means an earlier try arrived), the dialog's event has `isPhoneApproval`, and the footer adds "or approve on your iPhone". Every queued request is posted on its own, so the phone can answer a request that waits behind another one in the Mac's window.
- On `approval-answered` for one of its requests, the backend fetches the answer, checks it (above) while holding the session, and hands the decision to the waiting dialog. An approval or a denial also cancels a Touch ID check in progress. An invalid answer is ignored and the dialog stays open.

**iPhone** (`mobile/hisarkit`, `mobile/ios`)

- The phone does not need to be open when the Mac asks: the Mac posts the request whenever the account has an iOS device, and the request waits on the server until the dialog's deadline.
- While unlocked and in the foreground, the app lists the open requests right at unlock (before the vault sync), on returning to the foreground, on `approval-requested`, and every 15 s as a fallback for a missed event. The first appears as a card at the top of the Vaults tab: "<Mac name> · Expires in …", the request's sentence, the key, and **Deny** and **Approve** (Approve asks for Face ID as in the sheet); the Approvals tab lists them all with their badge count on the tab.
- Settings → Mac approvals → **Approve requests from my Mac** (on by default; `phoneApprovalDisabled` in the phone's `hisar.json`, kept on sign-out) turns this off for this iPhone: `ListAllPendingApproval` then returns nothing, `ApproveApproval` refuses, and the Mac's dialog decides on its own.
- A request that arrives while the app is in the foreground also opens the sheet at once; requests found at unlock or on returning to the foreground are only listed. Closing the sheet keeps the request in the list.
- The sheet shows an icon for the kind, "Approve on <Mac name>?", the description rows (app, action, command, repository, server, key, secrets, forwarded from, terminal), the note "End-to-end encrypted. Only your devices can read this request. Approving lets this one request continue; it does not unlock Hisar on your Mac.", **Approve with Face ID** and **Deny**. A request with a `rememberLabel` adds a switch with that sentence under the header, before the details, starting at `isRememberedByDefault`; its value when the user approves is signed into the answer. The Vaults card has no switch, so its Approve approves once; remembering needs the sheet.
- Approve needs Face ID (or Touch ID) first; when biometrics are unavailable or the user picks the fallback, the sheet asks for the master password, which is checked like an unlock. Deny needs neither.
- A request leaves the list, and its sheet closes, on `approval-answered` or `approval-resolved`, at `expiresAt`, and when the app locks. Expired, answered or withdrawn requests are never shown.
- UI tests drive the sheet and the list through debug-only previews (`-HisarApprovalPreview`, `-HisarApprovalBannerPreview`), like the AutoFill preview (§9.3). The cross-device flow is tested end to end in Go: a desktop backend and a HisarKit backend signed in to the same account, where the phone approves an SSH signature, a denied one fails, a forged answer is refused, and a phone app that starts only after the Mac asked still finds and approves the request.

### 9.10 Themes

Each device picks its own look from two settings; nothing is synced.

- **Appearance:** System (the default, follows macOS or iOS light and dark), Light or Dark.
- **Color:** Neutral (the default; Dark Neutral is the original Mac look), Blue (`#2356c4`), Purple (`#6a3dcc`) or Green (`#1b7348`). In light, a colour tints the chrome by 3–6 % (sidebar, item list, raised surfaces, borders) and the selection by 12–15 %, and becomes the accent of buttons, switches, focus rings and the selected icons; content stays on white. In dark, surfaces stay near-black with the same small tint, and the accent is a lighter shade of the hue (Blue `#7ea6f8`, Purple `#ad8ff5`, Green `#5cc48f`) with near-black text on it. Green keeps the separate success green, so a "running" dot never reads as the accent.
- **Single source:** `assets/themes/themes.json` holds the tokens: a full set per appearance (`base.light`, `base.dark`, which are Neutral) and per colour only the tokens it changes. `make generate-themes` (`node scripts/themes/generate.ts`, no dependencies) writes `desktop/frontend/src/themes.css` and `mobile/ios/Shared/Views/ThemePalettes.swift`; both are committed, and the frontend tests fail when they differ from what the JSON generates.
- **Contrast:** the same frontend tests compute the WCAG 2 contrast ratio for every colour × appearance and fail below the minimum: 4.5:1 for primary and secondary text on the background, sidebar, item list, surfaces, hover and selection, for muted text on the background, sidebar and surfaces (a selected row shows its subtitle in secondary text), for accent text, status text (danger, warning, success), text on the accent, on solid danger and in error and warning boxes, initials on every tile colour, and, on the iPhone, for the tint, the secondary label and the warning amber on the grouped background and the cells and for text on the tint; 3:1 for focus rings, the accent as a control colour and accent icons on the selection.
- **Mac:** `themes.css` defines the variables for `[data-color=…][data-appearance=light|dark]`, and for `data-appearance=system` inside `prefers-color-scheme` media queries. Every window (main, Settings, lock and sign-in screens, the SSH, CLI and agent approval dialogs, full and compact) sets both attributes on `<html>` from `GetTheme`, and follows the `theme-changed` event that `SetTheme` emits after saving, so a choice applies everywhere at once. Components use only these variables; the logo keeps its own colours. The choice is saved in `desktop.json` (`appearance`, `color`, kept on sign-out); a copy in the web view's local storage only spares a new window a flash of the default theme. The app also sets `NSApp.appearance` (Aqua, Dark Aqua or none for System), so the title bar, Touch ID sheets and file dialogs match. Settings → General → Appearance has a System · Light · Dark segmented control and four colour swatches drawn with each colour's own tokens; a click previews at once and a failed save goes back.
- **iPhone:** Settings → Appearance is a native list: Appearance with checkmarks (System, Light, Dark; `preferredColorScheme`) and Color with a swatch and a checkmark per colour. The colour sets the tint and, for Blue, Purple and Green, a lightly tinted grouped background (dark: near-black with a trace of the hue); cells, bars and Liquid Glass stay the system's. Tints are the darker, accessible shades above, not `systemBlue` and friends, and secondary labels are a solid `#6c6c70` (dark `#98989f`) instead of the translucent system grey, which falls below 4.5:1 on the tinted backgrounds. Warnings (a request's countdown, key changes) use an amber (`#8f5b00`, dark `#d9a33c`) instead of the system orange, which is about 2:1 on white. `GetTheme` and `SetTheme` in `mobile/hisarkit` store the choice in `hisar.json` in the app group, so the AutoFill extension reads the same theme when it opens.

### 9.11 Export

Users can take all their data out at any time, in a format Hisar reads back (§9.7). Export depends on nothing but an unlocked account: no plan, subscription or server-side switch is consulted, and offline it exports this device's encrypted cache (§9.1). It includes every vault the user can open, shared vaults too, with every item type: logins, notes, one-time password secrets, SSH private keys, passkeys and fields, and the vault names. Attachments are not supported yet.

**Plain JSON** (`core/export`, `format: "hisar-export"`):

```
{ "format": "hisar-export", "version": 1, "exportedAt": RFC 3339,
  "account": { "email", "server" },
  "vaults": [ { "id", "name", "type": "personal" | "shared",
                "items": [ { "id", "overview": item overview (§4.5), "details": item details (§4.5) } ] } ] }
```

Binary values (passkey IDs and keys) are base64 as in the item JSON. It is not encrypted: every app shows "Plain JSON is not encrypted. Anyone who gets the file can read every password, one-time code secret and SSH private key in it. Delete it when you are done." and asks before writing one.

**Encrypted export** (`format: "hisar-encrypted-export"`), the default, sealed with an export passphrase the user picks (at least 10 characters, entered twice in the apps; nobody, Hisar included, can open the file without it):

```
{ "format": "hisar-encrypted-export", "version": 1,
  "kdf": { "algorithm": "argon2id", "salt": base64 (16 bytes), "iterations": 24, "memoryKiB": 65536, "parallelism": 4 },
  "cipher": "xchacha20-poly1305",
  "ciphertext": base64 of 0x01 || nonce (24) || XChaCha20-Poly1305(exportKey, nonce, aad, JSON of the plain export) }

stretched = Argon2id(NFKD(passphrase), salt, iterations, memoryKiB, parallelism, len = 32)
exportKey = HKDF-SHA256(ikm = stretched, salt = nil, info = "hisar/v1/export-key", len = 32)
aad       = "hisar/v1/export"
```

- The file supplies the parameters, so readers refuse anything outside `MinArgon2Params`…`MaxArgon2Params` (§4.2) and a salt that is not 16 bytes before deriving anything: a crafted file cannot exhaust memory.
- A wrong passphrase, a changed byte, salt or ciphertext all fail the AEAD and read as "that passphrase doesn't open this export"; an unknown `kdf.algorithm` or `cipher`, or a `version` other than 1, is refused with its own message. A future version adds a new `version` and keeps reading 1.

**Mac.** Settings → Export (§9.1). Touch ID approves the export (`Biometric.Authenticate("export your Hisar data")`), or the master password when Touch ID is unavailable; then a save dialog proposes "Hisar export <date>.hisar" or "Hisar export <date> (not encrypted).json", and the file is written to a temporary file with mode 0600 and renamed into place.

**iPhone.** Settings → Account → **Export data…**: a sheet with the format picker, the passphrase twice or the plain JSON warning, and **Export with Face ID** (the master password field when Face ID is unavailable). The file is written into a new folder in the app's temporary directory and offered with **Save or Share Export** (`ShareLink`); the folder is deleted when the sheet closes.

**CLI.** `hisar export [-o file] [--format encrypted|plain] [-y]` writes `hisar-export-<date>.hisar` (or `.json`) in the current directory unless `-o` names another file, and never replaces one. In a terminal it asks for the format, then the passphrase twice; a script pipes the passphrase on one line, and a plain export needs `-y`. Through the app the app writes the file after its approval dialog (§9.4), which asks every time; standalone the command asks for the master password like the others.

## 10. Repository layout

The package map, the rules for where new code goes and the build commands are in [CLAUDE.md](../CLAUDE.md).

```
core/       Go module: crypto, wire types, the API client, sessions, the app backend shared by macOS and iOS (core/app)
server/     Go module: hisar-server (internal/api speaks HTTP, internal/service holds the rules, internal/store is Postgres)
cli/        Go module: hisar, standalone or through the desktop app; mcpserver/ hisar mcp
desktop/    Wails v3 app: the Wails main package and OS integration (SSH agent approvals, browser host, login item, Touch ID, updates), frontend/ UI
mobile/     hisarkit, the gomobile framework of the iOS app, on top of core/app; ios/ the Swift app
extension/  Browser extension (TypeScript)
docs/       Design and decisions
assets/     Logo sources and themes/themes.json, the colour tokens of every theme (§9.10)
deploy/     Dockerfile, Docker Compose (Postgres + server), nginx site, deployment notes
scripts/    Packaging, signing and the end-to-end test of the built server and CLI
go.work     Ties the Go modules together for local development
```

## 11. Roadmap

| Phase | Scope | Exit criteria |
|---|---|---|
| 0 | This document, repo skeleton, CI | Design reviewed and agreed |
| 1 | `core/crypt` with test vectors, OPAQUE auth, server (accounts, vaults, items, sync), CLI, Docker deployment, CI (unit and database tests, end-to-end test of the built binaries, image build) | Two CLI clients on two machines sync a shared vault through a self-hosted server |
| 2 (MVP) | Desktop app (macOS first): signup + Emergency Kit, unlock, **Touch ID unlock** (§5.4), items, generator, search, personal and shared vaults, invites, vault key rotation, auto-lock, **SSH agent with SSH Key items and Touch ID approval** (§9.6), local encrypted cache. CI builds the desktop app | Family uses it daily on macOS; `ssh` and `git` use keys from Hisar |
| 3 | Browser extension via native messaging: fill, save, generate | Daily logins without copy-paste |
| 4 | Mobile apps with system autofill | Logins on phones |
| 5 | TOTP, passkeys, attachments, Watchtower (reused/weak/breached via HIBP k-anonymity), import from 1Password/Bitwarden/CSV, SSE push sync | — |
| 6 | Person-to-person sharing by email (§5.8), emailed invites (§5.1, §8.5), Settings → People | No list of all accounts anywhere |
| 7 | Secret references, `hisar read` and `hisar run` (§9.4.1), "Copy secret reference" in the desktop app, `hisar mcp` for coding agents with per-vault agent visibility (§9.4.2), end-to-end encrypted share links for people without a Hisar account (§5.9; iPhone later) | Developers and coding agents run commands with secrets they never see |
| 9 | Rename, delete and leave vaults on the Mac, the iPhone and the CLI (§5.10); encrypted and plain JSON export and their import (§9.11, §9.7); `.env` import that leaves secret references behind (§9.7) | Users can take all their data out at any time and bring it back; a `.env` file holds only references |

## 12. Decisions

| # | Question | Decision |
|---|---|---|
| 1 | Argon2id parameters | t=24, m=64 MiB, p=4 (≈ 270 ms on M2 Pro); re-check on phones in Phase 4. Target ≈ 0.5–1 s on the slowest device |
| 2 | Wails version | v3, pinned beta (`v3.0.0-beta.23`; no stable v3 exists yet). Upgrade deliberately, one beta at a time, with the macOS and Linux CI builds as the gate |
| 3 | Server domain | `api.tryhisar.com`, public: the API, invite and share links, and the privacy policy. `tryhisar.com` serves no page yet, so the apps and pages do not link to it |
| 4 | Account recovery | Emergency Kit only (§6). Admin recovery was removed; a forgotten master password is unrecoverable |
| 5 | Item history retention | 90 days |
| 6 | Fingerprint check (§5.6) | Optional: a 16-hex-digit security code per person, shown in the desktop app's sharing panel and Settings → People. Key pinning and key-change refusals are always on |
| 7 | Apple Developer Program membership (code signing and notarization; required for biometric Keychain items and Gatekeeper) | **Open**: needed before Touch ID can ship. During development, local signing with a free Apple ID team works on your own Mac |
| 8 | Product model | A hosted service, not one family per server: person-to-person sharing by email that the recipient accepts, no account enumeration, invites emailed by the server over SMTP, sign-up stays invite-only, no teams or organizations yet. Account recovery is only through the Emergency Kit (§6) |
