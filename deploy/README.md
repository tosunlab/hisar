# Deploying hisar-server

Docker Compose runs Postgres and `hisar-server` from the container registry (linux/amd64 and linux/arm64). The server
listens on `127.0.0.1:8080` only; your nginx terminates TLS and proxies to it.

## Images

Pushing a tag `server-<version>` (e.g. `server-0.1.7`) runs the `server-image` job of the GitLab pipeline, which publishes
`reg.tryhisar.com/hisar/core/hisar-server` as `:<version>` and `:sha-<commit>`, for linux/amd64 and linux/arm64. Running the
manual `server-publish` job in the tag's pipeline then points `:<major>.<minor>` and `:latest` at it; a custom build
(`server-1.0.0-alpha-test`) can't be published. Running the pipeline by hand from a branch offers the job as a manual action that publishes
`:sha-<commit>` only.

The runners are unprivileged, so the job does not use Docker: `scripts/build-server-image.sh` cross-compiles the server
and appends it to the distroless base with [crane](https://github.com/google/go-containerregistry), giving the same image
as `deploy/Dockerfile`. Merge requests run the same script against a throwaway registry and boot the result.

reg.tryhisar.com is behind basic auth. The job logs in with two CI/CD variables (Settings → CI/CD → Variables, on the
project or the `hisar` group):

| Variable | Value | Flags |
|---|---|---|
| `REGISTRY_USERNAME` | a basic-auth user that may push | protected if only protected tags may publish |
| `REGISTRY_PASSWORD` | its password | masked, protected as above |

On the server, log in once with a basic-auth user that only needs to pull, and keep `HISAR_IMAGE` in `.env` as in the
example:

```sh
docker login reg.tryhisar.com
```

## First start

Copy `compose.yaml`, `.env.example` and `hisar.env.example` to the server (for example `/opt/hisar`), then:

```sh
cp .env.example .env                # set HISAR_DOMAIN and HISAR_IMAGE, pin HISAR_VERSION
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
`hisar-desktop` generic package that the `desktop-<version>` release pipeline publishes in GitLab, so it needs a read-only token:

1. In GitLab, open the project (`hisar/core`) → **Settings → Access tokens** → **Add new token**.
2. Name it e.g. `hisar-server updates`, set an expiry date, choose the role **Reporter** and only the scope **read_api**,
   then create it and copy the token.
3. In `.env` on the server, uncomment and fill in:

   ```sh
   HISAR_UPDATE_GITLAB_URL=https://git.tryhisar.com
   HISAR_UPDATE_GITLAB_PROJECT_ID=2          # Settings → General, "Project ID"
   HISAR_UPDATE_GITLAB_TOKEN=glpat-…
   ```

4. `chmod 600 .env && docker compose up -d`. The log says `desktop updates from https://git.tryhisar.com project 2`.

Check it with a session token from a signed-in device (`desktop.json` in the app's state folder):
`curl -H "Authorization: Bearer <token>" https://<domain>/api/v1/desktop/updates/appcast.xml`. Without the variables the
endpoints answer 404 "updates are not configured on this server". Renew the token before it expires; until then, update
checks fail with "the update source is unreachable".

## Plans and Lemon Squeezy (optional)

Pro is sold through Lemon Squeezy, the merchant of record (`docs/DESIGN.md` §8.7). Without the variables below nothing is
sold: everyone is on Free unless the operator made them a founding member, and the apps show no Upgrade button.

1. In Lemon Squeezy (test mode first), create the product **Hisar Pro** with two subscription variants: monthly at
   €2.49 and yearly at €19.99. Note the store ID (**Settings → Stores**) and both variant IDs (the product's variants,
   or `GET https://api.lemonsqueezy.com/v1/variants`).
2. **Settings → API**: create an API key. A test-mode key creates test checkouts; switch to a live key when the store is
   approved.
3. **Settings → Webhooks**: add `https://<domain>/api/v1/billing/lemon-squeezy/webhook` with a signing secret of your
   choice (at least 32 random characters) and these events: `subscription_created`, `subscription_updated`,
   `subscription_cancelled`, `subscription_resumed`, `subscription_expired`, `subscription_paused`,
   `subscription_unpaused`, `subscription_payment_success`, `subscription_payment_failed`,
   `subscription_payment_recovered` and `subscription_plan_changed`.
4. In `.env` on the server, uncomment and fill in:

   ```sh
   HISAR_LEMON_SQUEEZY_API_KEY=…
   HISAR_LEMON_SQUEEZY_STORE_ID=12345
   HISAR_LEMON_SQUEEZY_WEBHOOK_SECRET=…            # the webhook's signing secret
   HISAR_LEMON_SQUEEZY_MONTHLY_VARIANT_ID=111111
   HISAR_LEMON_SQUEEZY_YEARLY_VARIANT_ID=222222
   HISAR_LEMON_SQUEEZY_REDIRECT_URL=https://tryhisar.com/pricing   # optional
   ```

5. `chmod 600 .env && docker compose up -d`. The log says `plans sold through Lemon Squeezy store 12345`.

Buy Pro in test mode with Lemon Squeezy's test card from the Mac (Settings → Account → Upgrade) or with
`hisar plan upgrade`; the webhook's delivery log in Lemon Squeezy should show `204` answers, and `hisar plan` then says Pro.

**Founding members** have Pro for life without paying. Only the operator grants it, on the server:

```sh
docker compose exec hisar hisar-server operator grant-founding --email ada@example.com
docker compose exec hisar hisar-server operator show-plan --email ada@example.com
docker compose exec hisar hisar-server operator revoke-founding --email ada@example.com
```

Each command asks for the email when `--email` is missing and confirms before changing anything (`--yes` skips the
question, for scripts). There is no command that lists accounts.

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
