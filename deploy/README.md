# Deploying hisar-server

Docker Compose runs Postgres, `hisar-server` and Caddy (automatic HTTPS for `tryhisar.com`).

## First start

```sh
cd deploy
openssl rand -base64 32 | tr -d '\n' > db_password.txt
cp hisar.env.example hisar.env      # put the same password into HISAR_DATABASE_URL
chmod 600 db_password.txt hisar.env
docker compose up -d --build
```

Point the `tryhisar.com` DNS A/AAAA records at the server before starting, so Caddy can get a certificate.

On first start `hisar-server` creates `/data/hisar-secrets.json` in the `hisar-data` volume (OPAQUE server key and HMAC key). **Back it up**: if it is lost, nobody can log in and every account must be re-created.

## Invite yourself (admin)

```sh
docker compose exec hisar hisar-server invite
```

It asks for the email address; the first invite defaults to the admin role. Open the printed link with the CLI:

```sh
hisar signup https://tryhisar.com/invite/<token>
```

## Database migrations

Migrations live in `server/internal/store/migrations` and run with [goose](https://github.com/pressly/goose). `hisar-server serve` applies pending migrations on start, guarded by a Postgres advisory lock. For production upgrades you can run them explicitly first:

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

## Building for another architecture

```sh
docker buildx build --platform linux/amd64 -f deploy/Dockerfile -t hisar-server .
```
