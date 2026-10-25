---
sidebar_position: 3
sidebar_label: Self-hosting
---

# Deploying hisar-server

Docker Compose runs Postgres and `hisar-server` from the container registry (linux/amd64 and linux/arm64). The server
listens on `127.0.0.1:8080` only; your nginx terminates TLS and proxies to it. Keep it that way: the server speaks plain
HTTP and, with `HISAR_TRUST_PROXY=true`, believes the `X-Forwarded-For` it is sent, so nothing but the proxy may reach
it. Run one `hisar-server` per database: rate limits, phone approvals, pending sign-ins and the event streams live in
that one process, so a second replica would neither see nor notify the first one's clients.

## Images

A server release is a run of Actions → Release with the app `server` and a release type (patch, minor, major, or custom with
a version; [CI and releases](./development/releasing.md) has the steps). It tags `server-<version>`, publishes
`ghcr.io/tosunlab/hisar-server` as `:<version>` and `:sha-<commit>`, for linux/amd64 and linux/arm64, built from
`deploy/Dockerfile`, and creates the GitHub prerelease `server-<version>`. Approving the run's Publish job then points
`:<major>.<minor>` and `:latest` at it and turns the prerelease into a release; only published releases make the admin
dashboard say "Version \<version> is available". A custom build (`server-1.0.0-alpha-test`) can't be published. Running
"Release server image" by hand on a branch pushes `:sha-<commit>` only. Pull requests build the same image into a throwaway
registry and boot it, without pushing.

The workflows push with their own `GITHUB_TOKEN`, so the registry needs no secrets. While the package is private, the server
must log in once with a GitHub personal access token (classic) that has only the `read:packages` scope:

```sh
docker login ghcr.io -u <GitHub user>     # paste the token as the password
```

## First start

Copy `compose.yaml`, `.env.example` and `hisar.env.example` to the server (for example `/opt/hisar`), then:

```sh
(
  umask 077                         # only you may read the three files: they hold secrets
  cp .env.example .env              # set HISAR_DOMAIN, pin HISAR_VERSION
  openssl rand -hex 32 | tr -d '\n' > db_password.txt
  sed "s/CHANGE_ME/$(cat db_password.txt)/" hisar.env.example > hisar.env
)
chmod 600 .env db_password.txt hisar.env
docker compose up -d
curl http://127.0.0.1:8080/healthz  # ok
```

On an empty, uninitialized database, `hisar-server` creates `/data/hisar-secrets.json` in the `hisar-data` volume.
It contains the OPAQUE server keys and HMAC key. Back up this file with the database. Startup refuses a missing file
when accounts or an enrolled secrets fingerprint exist. Restore the matching file instead of generating new keys.
After fingerprint enrollment, a different valid file also fails startup. The first upgrade of an older database cannot
detect a wrong file supplied before enrollment, so confirm the original secrets volume before upgrading.

## nginx

`nginx.conf` is a site for `/etc/nginx/sites-available/`: replace `hisar.example.com` and the certificate paths
(e.g. from `certbot certonly --nginx -d <domain>`), and keep the port in line with `HISAR_PORT`. What the server needs from
the proxy:

- `client_max_body_size 32m`: vault key rotation uploads a whole vault.
- `/api/v1/events` (instant sync) unbuffered with a long `proxy_read_timeout`; the server sends a heartbeat every 25 s.
- `X-Forwarded-For` from `$proxy_add_x_forwarded_for`: with `HISAR_TRUST_PROXY=true` the server rate-limits by its last
  entry, the address nginx saw.
- The `hisar` log format, which logs paths without query strings and only the route of `/invite/…`, `/s/…` and
  `/api/v1/public/share-links/…`, regardless of letter case, so invite codes, share link IDs and sign-in email addresses
  stay out of the access log. nginx's error log still names the whole request when it cannot reach the server, so keep
  it as private as the database.

```sh
sudo nginx -t && sudo systemctl reload nginx
curl https://<domain>/healthz       # ok
```

Run `bash scripts/test-nginx.sh` from the repository root to check this configuration in a Docker container. It checks
`nginx -t` and the HTTPS and redirect access logs with lowercase, uppercase, and mixed-case capability paths.

## Open the setup URL from the log

On an empty database, `hisar-server serve` logs one line with a one-time setup URL:

```sh
docker compose logs hisar | grep 'to create the first admin'
# Open https://<domain>/admin/setup#<token> to create the first admin.
```

Open it in a browser. The dashboard creates the first admin in the browser (name, email and master password), shows the
Secret Key and the **Download Emergency Kit** button, runs the server's checks (it continues past warnings, never past a
failed check), and asks who can invite people. It ends on **Overview**, signed in as the new admin. The token sits in the
URL fragment, so it never reaches nginx's access log or a `Referer` header. Each restart replaces it with a new one until
the first account exists; a link from an earlier start answers "This setup link is no longer valid. Use the link in the
latest server log."

To start from the command line instead, run `docker compose exec hisar hisar-server invite`. It asks for the email
address; the first invite defaults to the admin role. Open the printed link in the desktop app, the iPhone app, or the
CLI (`hisar signup https://<domain>/invite/<token>`). Once that account exists the setup URL stops working, and opening
`/admin/setup` while signed in as that admin finishes the remaining steps. Neither way needs email setup.

## Email (invites and sharing)

Signed-in users invite people and share vaults by email, and the server sends those emails itself
([Design](./design.md) §5.1, §5.8). Without email, `POST /invites` answers "email is not configured on this server"; sharing
with someone who already has an account still works, and they see the invitation in the app.

1. Get SMTP credentials from your mail provider for a sending address such as `noreply@<domain>`, and set up SPF and
   DKIM for that domain so the emails are delivered.
2. Sign in to the admin dashboard at `https://<domain>/admin`, open **Settings → Email**, fill in the host, port,
   username, password and sender, click **Save**, then **Send test email…** to check them. The change applies at once.
   The server encrypts the SMTP password with a domain-separated key derived from `hisar-secrets.json`.
   A database-only dump does not contain that key. A live server, or someone with both the dump and secrets file, can
   decrypt it. Changing the host, port or username later clears it, so enter it again in the same save.

Or set them in `.env` on the server, which keeps the SMTP password out of the database. Any `HISAR_SMTP_*` variable
there sets all of Email: the values saved in the dashboard are ignored, a variable left empty gives its default (port
587), and the dashboard shows every Email field read-only with "Email is set by HISAR_SMTP_* variables on the server."
With a host, the server needs a port, a sender and, with a username, a password, and refuses to start without them:

   ```sh
   HISAR_SMTP_HOST=smtp.example.com
   HISAR_SMTP_PORT=587                       # 465 for implicit TLS; any other port must offer STARTTLS
   HISAR_SMTP_USERNAME=…
   HISAR_SMTP_PASSWORD=…
   HISAR_SMTP_FROM=Hisar <noreply@example.com>
   ```

Then `chmod 600 .env && docker compose up -d`. The log says `email through smtp.example.com port 587`.

The server never sends credentials without TLS. The emails are plain text without tracking; failures are logged as
`email not sent: …`, without the recipient's address.

## Push notifications

The iPhone and Android apps get a notification when your Mac asks the phone to approve something, when a device signs
in to the account, and when someone shares with you ([Design](./design.md) §9.12). A notification names only its kind
("Approval requested"), never what or who it is about.

- **iPhones:** your server sends through Hisar's push relay at `https://push.tryhisar.com`, which holds Apple's key for
  the official app, so you need no Apple account. The relay never learns which server or account sent a push.
- **Android phones:** your server sends to the UnifiedPush app each user installed, such as ntfy, by Web Push, encrypted
  to the phone, with no relay and no Google service. The server needs outbound HTTPS to those apps' servers, and it
  refuses endpoints on private or loopback addresses, so a ntfy on your own network must have a public address.

To turn notifications off, open **Settings → Push notifications** in the dashboard and empty the field, or set
`HISAR_PUSH_RELAY_URL=off` in `.env`; this turns off Android notifications too. Your own relay (`hisar-server push-relay`,
with its own APNs key) only works with an iPhone app you build and sign yourself.

## Instance settings

Admins change the server's settings in the admin dashboard under **Settings**, without a restart: **Email** (above),
**Sign-up** (who can invite people: anyone with an account, or admins only), **App versions** (the minimum app versions
of [Design](./design.md) §8.7) and **Security** (session lifetime, auth burst, and how long item history, the audit log and
expired invitations are kept; the audit log at least 30 days). They are stored in the `instance_settings` table. These
variables in `.env` override a field and lock it in the dashboard (any of the Email variables locks all of Email);
`compose.yaml` passes each of them to the server, and an empty value leaves the field to the dashboard:

| Setting | Variable |
|---|---|
| Email | `HISAR_SMTP_HOST`, `HISAR_SMTP_PORT`, `HISAR_SMTP_USERNAME`, `HISAR_SMTP_PASSWORD`, `HISAR_SMTP_FROM` |
| App versions | `HISAR_MINIMUM_VERSION_DESKTOP_MACOS`, `HISAR_MINIMUM_VERSION_IOS`, `HISAR_MINIMUM_VERSION_ANDROID`, `HISAR_MINIMUM_VERSION_CLI`, `HISAR_MINIMUM_VERSION_EXTENSION` |
| Auth burst | `HISAR_AUTH_BURST` |

Run one server process per database: each process reads the settings at start and after its own saves.

An HTTP 507 answer from the server means a user's change hit a growth budget, such as the vaults one account can
create or a vault's item history ([Design](./design.md) §8.2.2). It is not a server fault: the user sees what to remove, and
nothing needs fixing on the server.

## Desktop app updates

The macOS app checks the server it is signed in to for updates ([Design](./design.md) §9.1.1). The server reads the published
`desktop-<version>` releases of `tosunlab/hisar` on GitHub and relays the appcast and the download, so apps never talk to
GitHub. This needs no setup once the repository is public; while it is private, the server needs a read-only token:

1. On GitHub, open **Settings → Developer settings → Personal access tokens → Fine-grained tokens** →
   **Generate new token**.
2. Name it e.g. `hisar-server updates`, set an expiry date, choose the resource owner `tosunlab`, **Only select
   repositories** → `tosunlab/hisar`, and under **Repository permissions** only **Contents: Read-only**. Create it and copy
   the token.
3. In `.env` on the server, uncomment and fill in:

   ```sh
   HISAR_DESKTOP_UPDATE_GITHUB_TOKEN=github_pat_…
   ```

4. `chmod 600 .env && docker compose up -d`. The log says
   `desktop updates from the GitHub Releases of tosunlab/hisar, with a token`.

Check it with a session token from a signed-in device (`desktop.json` in the app's state folder):
`curl -H "Authorization: Bearer <token>" https://<domain>/api/v1/desktop/updates/appcast.xml`. The appcast is cached for
10 minutes; if GitHub is unreachable or rate limits the server, the last appcast is served. Without a usable token for a
private repository (missing or expired), update checks fail with "the update source is unreachable" and the log says
`no repository tosunlab/hisar (a private repository needs a token)`. `HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY` reads another
repository (e.g. a fork with its own signed releases), and `HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY=off` turns updates off:
the endpoints then answer 404 "updates are not configured on this server".

## Upgrades

```sh
# set HISAR_VERSION in .env to the new release
docker compose pull && docker compose up -d
```

`hisar-server serve` applies pending migrations on start, guarded by a Postgres advisory lock. To run them explicitly
first:

```sh
docker compose run --rm hisar migrate status
docker compose run --rm hisar migrate up
docker compose run --rm hisar migrate down    # roll back the latest migration
```

Take a backup before applying a migration that changes existing tables.

Migration `00013_invitation_sources` invalidates every sign-up invite that has not been used yet, including links from
`hisar-server invite`, the admin dashboard, and share or organization invitations sent to people without an account.
Older invites cannot be linked to the invitation they came from, so canceling that invitation could not stop them.
Accounts and invites that were already used are not changed. After the upgrade, resend any invite that is still
needed. Stop every server process running an older version before the migration runs.

Migration `00016_operational_secrets` enrolls a fingerprint of both authentication keys, and migration
`00018_smtp_password_encryption` encrypts any existing dashboard SMTP password, on the next server startup. The
transaction finishes before settings are read or requests are served, including when SMTP environment overrides are
set. Existing plaintext in older backups remains sensitive. Once secrets are enrolled, `migrate down` refuses both
migrations. To run a pre-upgrade binary, restore its matching pre-upgrade database and secrets backup into an isolated
instance. Do not remove the fingerprint or decrypt the row by hand.

## Backups

Back up nightly, after configuration changes, and before upgrades. Keep the database dump and matching secrets archive
as one recovery set. Stop the server while taking the pair. The Postgres container stays running.

```sh
umask 077
mkdir backup
docker compose stop hisar
docker compose exec -T postgres pg_dump -U hisar -Fc hisar > backup/database.dump
docker cp "$(docker compose ps -aq hisar):/data/hisar-secrets.json" - > backup/secrets.tar
docker compose start hisar
```

The tar archive preserves the secrets file's owner and private permissions. Encrypt the recovery set before copying it
off-server, for example with `age`, and keep its decryption key elsewhere. Preserve the deployment's protected `.env`,
`hisar.env`, and `db_password.txt` configuration too. A backup containing both the database and server secrets
exposes the SMTP password if its own encryption fails.
Retain several dated sets, including a known-good pre-upgrade pair, under your organization's retention policy.

The dump contains encrypted vault payloads, plaintext account and sharing metadata, audit events and instance settings.
It is sensitive, not ciphertext-only. The [database schema](https://github.com/tosunlab/hisar/blob/main/server/internal/store/migrations/00001_init.sql) records emails, devices, access grants and audit IPs outside the encrypted item payloads. [Organization names and rosters](https://github.com/tosunlab/hisar/blob/main/server/internal/store/migrations/00008_organizations.sql) are also plaintext. Operational credentials have different protection from vault keys, as described under **Email** above. Treat both backup components as sensitive. Possession of server secrets does not itself supply vault decryption keys.

A native client's offline cache is not a substitute for a server backup. It contains only records that device has downloaded and may omit whole unsynced vaults or newer writes. See [the cache and export limits](./design.md#91-desktop--wails-v3-go--vuetypescript).

### Test a paired restore

Use a fresh directory and an isolated Compose project, never the running project's volumes. Copy the deployment files
and protected configuration there. Pin the original server version first. Decrypt a recovery set into `backup/`.
Set `HISAR_PORT=18080` in the copied `.env`, or choose another unused port and adjust the check below. Keep `HISAR_PUBLIC_URL` unchanged and route its original hostname to
the isolated host through a private TLS proxy for the sign-in check. Do not expose the restore instance publicly.

```sh
docker compose -p hisar-restore up -d postgres
docker compose -p hisar-restore exec -T postgres pg_restore --clean --if-exists -U hisar -d hisar < backup/database.dump
docker compose -p hisar-restore create hisar
docker cp -a - "$(docker compose -p hisar-restore ps -aq hisar):/data" < backup/secrets.tar
docker compose -p hisar-restore start hisar
curl http://127.0.0.1:18080/healthz
```

Expect `ok`. Sign in with an existing test account using its master password and Secret Key. Open a known encrypted
item and compare its value. Send a test email through a disposable SMTP sink, not a production recipient. Repeat after
upgrading the isolated instance. Test this recovery procedure quarterly and record the results.

If startup reports missing or mismatched secrets, stop and restore the matching archive. Do not delete account data,
remove the fingerprint, or replace keys to silence the error. Without the original authentication secrets, new keys
cannot recover existing accounts. The missing-file check also applies to `hisar-server invite`. A healthy `/healthz`
alone does not prove that accounts or encrypted SMTP settings were recovered.

## Building from source

```sh
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```
