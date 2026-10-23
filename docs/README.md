# Hisar docs

The documentation site at [docs.tryhisar.com](https://docs.tryhisar.com), built with [Docusaurus](https://docusaurus.io/docs). Pages are Markdown files in `docs/`; the sidebar follows the folder structure and each page's `sidebar_position`. `DESIGN.md` and `COPY.md` next to this file are the repository's design docs, not pages of the site.

## Develop

Needs Node 24.

```sh
npm ci
npm start          # http://localhost:3000, reloads on save
npm run typecheck
npm run build      # the static site in build/
npm run serve      # serves build/ the way the image does
```

From the repository root, `make test-docs` typechecks and builds, and `make build-docs` builds; CI runs both and builds the image.

## Layout

| Path | What |
|---|---|
| `docs/` | The pages. `intro.md` is the front page (`slug: /`). A folder becomes a sidebar category; `_category_.json` names and orders it |
| `docusaurus.config.ts` | Site title, URL, navbar, footer and the `classic` preset: docs at `/`, no blog |
| `sidebars.ts` | One sidebar, generated from `docs/` |
| `src/css/custom.css` | Hisar's neutral palette on top of Infima |
| `static/img/` | The logo and favicon from `assets/logo`, and the social card |
| `Dockerfile`, `nginx.conf` | The image: Node builds the site, unprivileged nginx serves it on port 8080 with `/healthz` |

## Run the image

```sh
docker build -t hisar-docs docs
docker run --rm -p 8082:8080 hisar-docs
```

## Release

`docs` is an app of the Release workflow like `server`: `gh workflow run release.yml -f app=docs -f type=patch` tags `docs-<version>`, pushes `ghcr.io/tosunlab/hisar-docs:<version>` (linux/amd64 and linux/arm64) and, once the publish is approved, points `:latest` and `:<major>.<minor>` at it. See [RELEASING.md](RELEASING.md). The compose service and nginx server block for docs.tryhisar.com live in the deployment, next to the website's.
