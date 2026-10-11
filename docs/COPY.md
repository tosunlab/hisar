# Hisar copy glossary

One name for each thing, the same on the Mac, the iPhone, the browser extension, the CLI and the server's pages and emails. Check here before adding or renaming user-facing text.

## Product

| Use | Not | Where |
|---|---|---|
| End-to-end encrypted password, SSH key and secrets manager for developers. | self-hosted, family password manager | App descriptions, Info.plist, Linux `.desktop`, store listing, share link page |
| Passwords, SSH keys and secrets for developers, end-to-end encrypted. | Two taglines, exclamation marks | The one tagline: Mac sign-in, iPhone About |
| `api.tryhisar.com` | `tryhisar.com` (serves no page yet), `hisar.example.com` as a suggestion | Default server, links, privacy policy URL (`https://api.tryhisar.com/privacy`) |
| Use a different server… | A required Server field | Sign in, Mac and iPhone |
| the Hisar server | your Hisar server, your family's server | Explanations |

## Accounts and secrets

| Use | Not |
|---|---|
| master password | password (when the master password is meant), passphrase |
| Secret Key | secret key, account key |
| Emergency Kit: "The Emergency Kit holds your Secret Key and has a space to write your master password." | "has both places to keep them" |
| Unlock with Touch ID / Unlock with Face ID; Turn on Touch ID, Not now | Biometrics, fingerprint unlock |
| Sign in, Sign out, Create account | Log in, Register |

## People and sharing

| Use | Meaning |
|---|---|
| Invite to Hisar | Creates a Hisar account: the server emails a sign-up link. Settings → People. Never "+ Invite" alone. |
| Share, Invite by email | Gives someone a vault. The vault's Share panel (Mac) or sheet (iPhone). |
| invitation | A pending vault share ("Invitations for you", "Invitations you sent"). |
| share link | An end-to-end encrypted link for anyone, with expiry and view limits. |
| Owner, Editor, Reader | Vault roles. Never "admin" in the apps. |
| security code | The 16-digit code to compare a person's keys. Not fingerprint. |

## Items

| Use | Not |
|---|---|
| vault, Personal vault, shared vault | folder, collection |
| item, login, SSH key | entry, record |
| one-time code | TOTP, 2FA code, verification code |
| Other fields (custom fields below username and password) | Fields, Custom fields, Extra fields |
| Secret (a field hidden until revealed) | Concealed, Hidden |
| secret reference | secret URI, reference link |

## Developer tools

| Use | Not |
|---|---|
| the hisar command-line tool (`hisar`) | the CLI (in app copy), the binary |
| SSH agent, Use Hisar for SSH | key agent |
| Approve requests on my iPhone (Mac) / Approve requests from my Mac (iPhone) | Allow approvals from my iPhone, Allow approving Mac requests |
| Approve, Deny | Allow, Reject |
| Coding agents, Available to agents | AI, MCP (except in the `hisar mcp` command) |

## Help

| Use | Not |
|---|---|
| Help (Settings section on the Mac, row on the iPhone) | About & support, Feedback |
| Contact support, `support@tryhisar.com` | — |
| Copy diagnostics: app version and build, OS version, server URL, this device's ID | Anything secret, item names, other people's emails |

## Style

- Plain sentences: no exclamation marks, no comma splices, and only claims the code keeps ("private keys never leave Hisar"; the command-line tool reads passwords and one-time codes but never private keys).
- Developer features lead; sharing with other people is described as person-to-person, never as family sharing.
- Sentence case for buttons, labels and menu items ("Copy one-time code"), title case only for product names (Secret Key, Emergency Kit, Touch ID).
- An ellipsis (…) on an action that opens more input; "…" while busy ("Signing in…").
- Settings paths in prose: Settings, Security (in a sentence) or Settings → Security (in docs).
