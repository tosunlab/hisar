#!/usr/bin/env bash
# Builds and pushes the hisar-server image like deploy/Dockerfile, without a Docker daemon: the binary is cross-compiled
# and appended as layers to the distroless base with crane, so it works on unprivileged CI runners.
# Usage: build-server-image.sh <image> <tag> [<tag>...]   (one multi-arch index per tag; CRANE_INSECURE=1 for plain http)
set -euo pipefail

image=$1
shift
allTag=("$@")
baseImage=gcr.io/distroless/static-debian12:nonroot
allCraneFlag=()
if [ "${CRANE_INSECURE:-}" = 1 ]; then allCraneFlag=(--insecure); fi
allLabelFlag=()
if [ -n "${CI_PROJECT_URL:-}" ]; then allLabelFlag+=(--label "org.opencontainers.image.source=$CI_PROJECT_URL"); fi
if [ -n "${CI_COMMIT_SHA:-}" ]; then allLabelFlag+=(--label "org.opencontainers.image.revision=$CI_COMMIT_SHA"); fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

allArchitectureImage=()
for architecture in amd64 arm64; do
  mkdir -p "$work/$architecture/binary/usr/local/bin" "$work/$architecture/data/data"
  (cd server && CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" go build -trimpath -ldflags="-s -w" -o "$work/$architecture/binary/usr/local/bin/hisar-server" ./cmd/hisar-server)
  tar -C "$work/$architecture/binary" --owner=0 --group=0 --numeric-owner -cf "$work/$architecture/binary.tar" usr
  tar -C "$work/$architecture/data" --owner=65532 --group=65532 --numeric-owner -cf "$work/$architecture/data.tar" data
  architectureImage="$image:${allTag[0]}-$architecture"
  crane append "${allCraneFlag[@]}" --platform "linux/$architecture" --base "$baseImage" \
    -f "$work/$architecture/binary.tar" -f "$work/$architecture/data.tar" -t "$architectureImage" >/dev/null
  crane mutate "${allCraneFlag[@]}" "$architectureImage" -t "$architectureImage" \
    --entrypoint hisar-server --cmd serve \
    --env HISAR_ADDR=:8080 --env HISAR_SECRETS_FILE=/data/hisar-secrets.json "${allLabelFlag[@]}" >/dev/null
  allArchitectureImage+=(-m "$(crane digest "${allCraneFlag[@]}" --full-ref "$architectureImage")")
done
for tag in "${allTag[@]}"; do
  crane index append "${allCraneFlag[@]}" "${allArchitectureImage[@]}" -t "$image:$tag" >/dev/null
  echo "pushed $image:$tag"
done
