# Deploying hisar-server

Docker Compose runs Postgres and `hisar-server` from the container registry (linux/amd64 and linux/arm64). The server
listens on `127.0.0.1:8080` only; your nginx terminates TLS and proxies to it.

## Images

Pushing a tag `server-<version>` (e.g. `server-0.1.7`) runs the GitHub Actions workflow "Release server image", which publishes
`ghcr.io/hisarapp/server` as `:<version>` and `:sha-<commit>`, for linux/amd64 and linux/arm64, built from `deploy/Dockerfile`.
Running the Publish workflow (app `server`, the tag) then points `:<major>.<minor>` and `:latest` at it; a custom build
(`server-1.0.0-alpha-test`) can't be published. Running "Release server image" by hand on a branch without a tag pushes
`:sha-<commit>` only. Pull requests build the same image into a throwaway registry and boot it, without pushing.

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

## Invite yourself (admin)

```sh
docker compose exec hisar hisar-server invite
```

It asks for the email address; the first invite defaults to the admin role. Open the printed link in the desktop app,
the iPhone app, or the CLI (`hisar signup https://<domain>/invite/<token>`). The operator command needs no email setup.

## Email (invites and sharing)

Signed-in users invite people and share vaults by email, and the server sends those emails itself
(`docs/DESIGN.md` §5.1, §5.8). Without email, `POST /invites` answers "email is not configured on this server"; sharing
with someone who already has an account still works, and they see the invitation in the app.

1. Get SMTP credentials from your mail provider for a sending address such as `noreply@<domain>`, and set up SPF and
   DKIM for that domain so the emails are delivered.
2. In `.env` on the server, uncomment and fill in all five (the server refuses to start with only some of them):

   ```sh
   HISAR_SMTP_HOST=smtp.example.com
   HISAR_SMTP_PORT=587                       # 465 for implicit TLS; any other port must offer STARTTLS
   HISAR_SMTP_USERNAME=…
   HISAR_SMTP_PASSWORD=…
   HISAR_SMTP_FROM=Hisar <noreply@example.com>
   ```

3. `chmod 600 .env && docker compose up -d`. The log says `email through smtp.example.com port 587`.

The server never sends credentials without TLS. The emails are plain text without tracking; failures are logged as
`email not sent: …`, without the recipient's address.

## Desktop app updates (optional)

The macOS app checks the server it is signed in to for updates (`docs/DESIGN.md` §9.1.1). The server reads the
`hisar-desktop` generic package of a GitLab project that holds the signed desktop releases, so it needs a read-only token:

1. In GitLab, open that project → **Settings → Access tokens** → **Add new token**.
2. Name it e.g. `hisar-server updates`, set an expiry date, choose the role **Reporter** and only the scope **read_api**,
   then create it and copy the token.
3. In `.env` on the server, uncomment and fill in:

   ```sh
   HISAR_UPDATE_GITLAB_URL=https://gitlab.example.com
   HISAR_UPDATE_GITLAB_PROJECT_ID=123        # Settings → General, "Project ID"
   HISAR_UPDATE_GITLAB_TOKEN=glpat-…
   ```

4. `chmod 600 .env && docker compose up -d`. The log says `desktop updates from https://gitlab.example.com project 123`.

Check it with a session token from a signed-in device (`desktop.json` in the app's state folder):
`curl -H "Authorization: Bearer <token>" https://<domain>/api/v1/desktop/updates/appcast.xml`. Without the variables the
endpoints answer 404 "updates are not configured on this server". Renew the token before it expires; until then, update
checks fail with "the update source is unreachable".

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
