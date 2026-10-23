---
sidebar_position: 3
sidebar_label: Self-hosting
---

# Deploying hisar-server

Docker Compose runs Postgres and `hisar-server` from the container registry (linux/amd64 and linux/arm64). The server
listens on `127.0.0.1:8080` only; your nginx terminates TLS and proxies to it.

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
cp .env.example .env                # set HISAR_DOMAIN, pin HISAR_VERSION
openssl rand -hex 32 | tr -d '\n' > db_password.txt
sed "s/CHANGE_ME/$(cat db_password.txt)/" hisar.env.example > hisar.env
chmod 600 db_password.txt hisar.env
docker compose up -d
curl http://127.0.0.1:8080/healthz  # ok
```

On first start `hisar-server` creates `/data/hisar-secrets.json` in the `hisar-data` volume (OPAQUE server key and HMAC
key). **Back it up**: if it is lost, nobody can log in and every account must be re-created.

## nginx

`nginx.conf` is a site for `/etc/nginx/sites-available/`: replace `hisar.example.com` and the certificate paths
(e.g. from `certbot certonly --nginx -d <domain>`), and keep the port in line with `HISAR_PORT`. What the server needs from
the proxy:

- `client_max_body_size 32m`: vault key rotation uploads a whole vault.
- `/api/v1/events` (instant sync) unbuffered with a long `proxy_read_timeout`; the server sends a heartbeat every 25 s.
- `X-Forwarded-For` from `$proxy_add_x_forwarded_for`: with `HISAR_TRUST_PROXY=true` the server rate-limits by its last
  entry, the address nginx saw.

```sh
sudo nginx -t && sudo systemctl reload nginx
curl https://<domain>/healthz       # ok
```

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
   The SMTP password is then stored in the database in plain text, not encrypted: anyone who can read the database or
   its backups can read it. Changing the host, port or username later clears it, so enter it again in the same save.

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

## Backups

Back up nightly and copy off the server:

- `docker compose exec postgres pg_dump -U hisar hisar` (ciphertext only, but still keep it encrypted, e.g. with `age`)
- the `hisar-data` volume (`hisar-secrets.json`)

## Building from source

```sh
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```
