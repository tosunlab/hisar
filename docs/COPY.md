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
| Change master password…, Replace Secret Key… | Reset password, Rotate key, Regenerate Secret Key |
| "Your other devices will be signed out. Sign in on them with the new master password." / "…with your master password and the new Secret Key from the new Emergency Kit." | Logged out, session invalidated |
| "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one." | Session expired, unauthorized |
| Delete account…, Delete account (the confirm button, after typing the email) | Close account, Remove account, Deactivate |
| "Export your items first in Settings → Export." (Mac), "… in Settings → Account → Export data." (iPhone), "… with `hisar export`" (CLI) | Back up your data |

## People and sharing

| Use | Meaning |
|---|---|
| Invite to Hisar | Creates a Hisar account: the server emails a sign-up link. Settings → People. Never "+ Invite" alone. |
| Share, Invite by email | Gives someone a vault. The vault's Share panel (Mac) or sheet (iPhone). |
| invitation | A pending vault share ("Invitations for you", "Invitations you sent"). |
| share link | An end-to-end encrypted link for anyone, with expiry and view limits. |
| Owner, Editor, Reader | Vault roles. Never "admin" for a vault role. |
| security code | The 16-digit code to compare a person's keys. Not fingerprint. |

## Organizations

| Use | Meaning |
|---|---|
| organization, team | A group of accounts, and a group inside it. Not company, workspace, group. |
| Admin, Member | Organization roles. Admins manage members, teams and which teams have which vaults; being an admin gives no vault. |
| organization vault | A vault that belongs to an organization and reaches people through its teams. |
| Create organization…, New team…, Invite by email, Send invitation, Give a vault…, Give to a team…, Take back, Add member, Remove, Make admin, Make member | Actions (Settings → Organizations, the organization screen, an organization vault's sharing panel). |
| "Ada invited you to Acme as a member" · Accept · Decline | A received organization invitation (Settings → Organizations and the banner above the vaults). |
| Remove from Acme…, Leave Acme…, Rename organization…, Delete organization…, Rename…, Delete team… | Confirmed actions; each confirmation names what goes: "Bob loses the vaults they have through Acme's teams." · "Its members lose the vaults they have only through Platform." · "Leave Acme?" / "You lose the vaults you have through Acme's teams." · "Delete Acme?" / "Its teams and pending invitations are deleted with it." · "Everyone in Acme sees the new name." |
| "You joined Acme. Its vaults appear once an admin adds you to a team." | After accepting an organization invitation. |
| "Platform's members get this vault. Manage it in Settings → Organizations." | The new-vault Where footer for a team. |
| Admin · 3 members · Organization · Editor | An organization's row; an organization vault's row (iPhone). |
| "Hisar couldn't give every vault key: …" · "Hisar couldn't re-encrypt every vault that needs a new key: …" | Warnings above the vaults when a key delivery or a rotation failed. |
| Shared through teams in Acme · Ask an admin of Acme to change who has this vault. | An organization vault's sharing panel. |
| A vault you can't open | An organization's vault this device has no key for. |
| Waiting for an owner of a vault in Acme to give you its key · Waiting for the owners of N vaults in Acme to give you their keys | One line per organization above the vaults: a vault a team gives you that no owner's Hisar has delivered yet. |
| Acme is deleted with your account · Ada becomes an admin of Acme · You leave Acme | Account deletion. |

## Items

| Use | Not |
|---|---|
| vault, Personal vault, shared vault | folder, collection |
| item, login, SSH key | entry, record |
| one-time code | TOTP, 2FA code, verification code |
| Other fields (custom fields below username and password) | Fields, Custom fields, Extra fields |
| Secret (a field hidden until revealed) | Concealed, Hidden |
| secret reference | secret URI, reference link |
| Favorites (the list at the top of the sidebar and of Vaults), favorite, star (the icon) | Starred, Pinned, Bookmarks |
| Add to Favorites, Remove from Favorites (the star's label, the context menu, swipe actions; ⌘D on the Mac) | Favorite, Unfavorite, Star (as a button label) |
| Added to Favorites · Removed from Favorites (notices; the iPhone ends them with a period) · `Added "GitHub" (Personal) to Favorites.` (`hisar favorite`) | Starred!, Pinned |
| "No favorites yet" · "Star an item with ★ or ⌘D to keep it here, across all your vaults and devices. Only you see your favorites." (Mac) · "Star an item to keep it here, on all your devices. Only you see your favorites." (iPhone) · "No favorites yet. Add one with: hisar favorite <item> [-v vault]" (CLI) | Shared favorites, favorites for the vault |

## Vaults and your data

| Use | Not |
|---|---|
| Rename…, Delete vault…, Leave vault… (the vault's menu; ⋯ is labelled "Vault actions") | Remove vault, Archive, Exit vault |
| Delete “Work”? · These people lose access: · Move its N items to [vault] · Delete the vault and its N items · Type Work to confirm · Delete vault | Are you sure?, Destroy, Wipe |
| Leave “Work”? · You lose access to its items on all your devices. An owner can invite you again. · Leave vault | Unsubscribe, Quit vault |
| Your Personal vault can't be deleted. It is the vault Hisar keeps for you alone, and logins saved from the browser and the iPhone go there. | Cannot delete default vault |
| “Work” is no longer available: it was deleted, or you no longer have access to it. | Vault removed by admin |
| Moved N items to Personal and deleted Work. · Deleted Work. · You left Work. | — |
| Export (Settings section on the Mac), Export data… (iPhone, Settings → Account), `hisar export` | Backup, Download my data, Takeout |
| encrypted export, export passphrase ("at least 10 characters"; Hisar can't recover it) | export password, backup key, master password (for the export's own passphrase) |
| Plain JSON, with the warning "Plain JSON is not encrypted. Anyone who gets the file can read every password, one-time code secret and SSH private key in it. Delete it when you are done." | unencrypted backup, raw export |
| Export with Touch ID… / Export with Face ID · Exported N items from M vaults to <path>. | — |
| that passphrase doesn't open this export | wrong password, decryption failed |
| `.env` import: "Store N values in Personal › shop", "Kept as they are: PORT", "Run a command with them: hisar run --env-file .env -- <command>" | migrate, vault-ify |

## Developer tools

| Use | Not |
|---|---|
| the hisar command-line tool (`hisar`) | the CLI (in app copy), the binary |
| SSH agent, Use Hisar for SSH | key agent |
| Approve requests on my iPhone (Mac) / Approve requests from my Mac (iPhone) | Allow approvals from my iPhone, Allow approving Mac requests |
| Approve, Deny | Allow, Reject |
| Coding agents, Available to agents | AI, MCP (except in the `hisar mcp` command) |

## Invites and updates

| Use | Not | Where |
|---|---|---|
| "Ada invited you to Hisar"; Download for Mac; Open in Hisar; invite code | sign-up token, registration link | Invite page and invite email |
| This invite isn't valid | expired, already used, not found (never say which) | Invite page for any code that does not work |
| Update required (screen title); This server needs Hisar 0.2.0 or later. This Mac has 0.1.0. | Your app is outdated, unsupported version | Mac and iPhone blocking screen |
| Update now (Mac, Sparkle); Install the latest Hisar (iPhone, until TestFlight or the App Store); Check again | Upgrade | Blocking screen buttons |
| Hisar needs an update | update required error, 426 | What the SSH agent, the command-line tool, coding agents and the extension are told |
| Update the Hisar extension | | Extension popup when the extension itself is too old |

## Admin dashboard

The web page at `/admin` where admins of the Hisar server manage it. The apps never show it or link to it.

| Use | Not | Where |
|---|---|---|
| Admin dashboard; "Hisar · Admin dashboard"; Hisar admin dashboard (the page title) | admin panel, console, back office | Page title, sign-in screen, sidebar |
| Sign in to the admin dashboard · "Use the email, Secret Key and master password of an admin account on the Hisar server." · Sign in, Signing in… · Show, Hide (the Secret Key) | Log in, admin login | Sign-in screen |
| "This account is not an admin on the Hisar server." | Access denied, Forbidden, Unauthorized | Sign-in screen, for an account whose role is member |
| incorrect email, master password or Secret Key | wrong password, login failed | Sign-in screen; the same message as the Mac |
| Overview · Signed in as · Server | Home, Dashboard, Status | The first section |
| Sign out, Signing out… | Log out | Sidebar |
| an admin on the Hisar server, admin, member (account roles) | administrator, superuser, root | Copy about server roles; say "on the Hisar server" where an organization admin could be meant |

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
