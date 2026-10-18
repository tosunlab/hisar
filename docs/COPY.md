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
| incorrect email, master password or Secret Key | wrong password, login failed | Sign-in screen; the Mac's sign-in message without its "for <server>; check that this is your account's server" suffix, since the dashboard is on its server |
| Overview · Signed in as · Server · Version · Database · Usage · Checks | Home, Dashboard, Status, Health | The first section |
| Version 10.0.0 is available · 48.2 MB · All 8 migrations applied, 7 of 8 migrations applied · Accounts, Suspended accounts, Vaults, Items, Share links, Pending invites (Account, Suspended account, Vault, Item, Share link, Pending invite for 1) · "The item count may be up to a minute old." | Update now, Upgrade, DB | Overview |
| Public URL, HTTPS, Reverse proxy, Database migrations, Secrets backup, Email (check titles) · OK, Warning, Failed (status marks) · "Back up hisar-secrets.json. Nobody can sign in without it." · "Email is not set up. Invites from the apps will fail." · "The server could not reach its public URL from inside: <the error>. This is common in Docker or behind a router; open <URL>/healthz from another device and check that it shows ok." | Error, Critical, Healthy | Overview checks; each message is one or two plain sentences that name the setting to change |
| I have a backup, Saving… · "An admin confirmed a backup of hisar-secrets.json on 2026-10-17." | Acknowledge, Dismiss, Done | Overview, the Secrets backup check |
| Sign out, Signing out… | Log out | Sidebar |
| Settings · Email, Sign-up, App versions, Security (the groups) · Save, Saving…, Saved · Unsaved changes | Configuration, Preferences, Apply | The Settings section |
| SMTP host, Port, Username, Password, Sender · "Saved; type to replace" (a saved password) · "Enter the password again: changing the host, port or username clears the saved one." (under Password) · Send test email…, Send to, Send, Sending…, Close · "Hisar sends one email with the saved settings." · "Test email sent to you@example.com." · "The test email was not sent: <the SMTP error>" · "Email is not set up. Save the SMTP settings first." · "Too many test emails. Wait a minute and try again." | Mail server, From address, Test connection | Settings → Email |
| "Test email from Hisar" (subject) · "This is a test email from the Hisar server at <origin>. Email works: the server can send invites and sharing notifications." | — | The test email |
| Who can invite people: Anyone with an account, Admins only · "Only admins can invite people to the Hisar server." (the apps show it when a member invites under Admins only) | Invite permission, Restrict invites | Settings → Sign-up; the apps' invite error |
| Mac app, iPhone app, Command-line tool, Browser extension · No minimum | macOS client, CLI | Settings → App versions |
| Audit log · Account ("Email or account ID"), Type, From, To (the filters) · All events, All <group> (All admin actions) · Load more · Export CSV, Exporting… · Time, Event, Account, Details, IP address (the columns) · Deleted account, No account · "No events match these filters." · Loading… · 120 events, 1 event (how many are shown) · "Filter by one type or one group, not both." · "The start of the date range must be before its end." · "This page of the audit log is no longer valid. Load the audit log again." | Activity, History, Logs, Download | The Audit log section |
| Signed in, Account suspended, Settings changed … (one label per audit event type, from `auditTypes` in `server/internal/service/audit.go`) · Accounts and sign-in, Vaults and items, Sharing, Organizations and teams, Admin actions (the groups) | login, admin_account_suspended (the stored names) | Audit log, the Event column and the Type filter |
| Session lifetime, Auth burst, Keep item history, Keep the audit log, Keep expired invitations · Days, "Days, at least 30" (the audit log) | Session TTL, Rate limit, Retention policy | Settings → Security |
| Set by HISAR_SMTP_HOST (a field the server's environment sets; read-only) · "Email is set by HISAR_SMTP_* variables on the server. Change them there and restart the server." (all of Email) | Locked, Managed by environment | Under the field; under the Email description |
| Accounts · Search ("Email or name") · Account, Role, State, Signed up, Last activity, Devices, Vaults (columns) · Admin, Member · Active, Suspended · "5 minutes ago", "yesterday", Just now, Never (last activity) · "2 suspended" (devices of a suspended account) · Show more · "No account matches the search." | Users, Members list, Status, Enabled/Disabled, Banned | The Accounts section |
| ← Accounts · Suspend…, Reactivate, Change role…, Sign out everywhere…, Delete account… · Devices, Sign out (one device), "Suspended · last activity 5 minutes ago" (a device of a suspended account) · "Use Settings → Account in the Hisar app for your own account." | Ban, Disable, Lock account, Kick, Remove user | An account's page |
| Suspend <name>? · "<email> cannot sign in, and their devices stay signed in but cannot reach the server until you reactivate the account. Their share links stop opening until then. Their vaults, items and organizations stay as they are. To remove the devices, use Sign out everywhere." · Suspend, Suspending… | Ban, Block | The Suspend… dialog |
| Sign out <name> everywhere? · "Every device of <email> is signed out. They can sign in again with their master password and Secret Key." · Sign out everywhere, Signing out… | Log out all sessions, Kill sessions | The Sign out everywhere… dialog |
| Change the role of <name> · Role on the Hisar server · "Admin: manages the Hisar server from this dashboard", "Member: uses the Hisar apps only" · Change role, Saving… | Promote, Demote, Grant admin | The Change role… dialog |
| Delete <name>? · "The account is deleted with its vaults that have no other owner and their items, its access to other vaults, its organization memberships, devices, share links and invitations. Nobody can undo this. Hisar emails <email> that an admin deleted it." · Type <email> to confirm · Delete account, Deleting… | Remove user, Purge | The Delete account… dialog |
| "Enter your own Secret Key and master password to confirm it's you." · Your Secret Key, Your master password · Checking master password… | Re-enter credentials, Verify identity, Sudo | Change role… and Delete account… |
| "The Hisar server needs at least one admin." · "This account is suspended. Contact the admin of the Hisar server." (sign-in on the Mac, the iPhone, `hisar login` and the dashboard, only after the right master password; unlocking a device of a suspended account and every `hisar` command on it) | Last admin, Account disabled, Account locked | A refused change; signing in to or using a suspended account |
| "Your Hisar account <email> was deleted by an admin of the Hisar server on <date>." · "If you have questions, contact the admin of the Hisar server." | Your account was removed | The deletion email after an admin deleted the account |
| Invites · Invite to Hisar…, Email, Role: Member, Admin · "An admin can also sign in to this dashboard." · Create invite, Creating…, Cancel · Email, Role, Sent by, Sent, Expires (columns) · hisar-server invite (the sender of an operator's invite) · Resend, Revoke · No pending invites. | Invite user, Pending invitations, Delete, Cancel invite, Reissue | The Invites section |
| "Hisar emailed the invite to carol@example.com." · "Email is not set up on the server, so nobody was emailed. Copy the link and send it to carol@example.com yourself." · "The email was not sent: <the SMTP error>. Copy the link and send it to carol@example.com yourself." · Copy link, Copied, "Select the link and copy it." · "The link is shown only now. Resend the invite for a new one." · Done | Token, Invite code, Share | Invites, after Create invite or Resend |
| "An account with this email already exists." · "This invite no longer exists. It was used, revoked or has expired." · "Choose admin or member." | Duplicate, Not found, Invalid role | Invites errors |
| "Hisar · Set up the server" · Step 1 of 4 · Create the first admin · "This account manages the Hisar server from this dashboard. It is also a normal Hisar account for the apps." · Name, Email, Master password, Repeat master password · Create account, Creating account… | Register admin, Bootstrap, Onboarding wizard | `/admin/setup`, the first step |
| "This setup link is no longer valid. Use the link in the latest server log." | Invalid token, Invite expired | `/admin/setup`, for a used or replaced setup invite or a URL without one |
| "Open <url>/admin/setup#<token> to create the first admin." | Setup token, Bootstrap URL | The server log on a server without accounts |
| Save your Secret Key · Copy · Download Emergency Kit · I saved my Emergency Kit · Continue · "The Emergency Kit could not be created (…). Write down the Secret Key above." · I wrote down my Secret Key | Download PDF, Recovery kit | `/admin/setup`, the second step |
| "Your account was created, but the Personal vault could not be created (…). You can create a vault later from the Hisar apps. Save your Secret Key now." | Sign-up failed, Vault error | `/admin/setup`, the second step, when the Personal vault failed after the account exists |
| Check the server · "Fix every failed check before you continue. A warning can wait, and Overview keeps showing it." · Check again · "Fix <check> first, then check again." | Health check, Preflight | `/admin/setup`, the third step |
| Choose who can invite people · Finish setup, Finishing… | Invite settings | `/admin/setup`, the last step |
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
