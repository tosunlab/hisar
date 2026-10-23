#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
temporary=$(mktemp -d)
container=
cleanup() {
  if [[ -n "$container" ]]; then docker rm -f "$container" >/dev/null; fi
  rm -rf "$temporary"
}
trap cleanup EXIT

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=hisar.example.com' \
  -keyout "$temporary/privkey.pem" -out "$temporary/fullchain.pem" >/dev/null 2>&1
container=$(docker run --rm -d \
  -p 127.0.0.1::80 -p 127.0.0.1::443 \
  -v "$root/deploy/nginx.conf:/etc/nginx/conf.d/default.conf:ro" \
  -v "$temporary:/etc/letsencrypt/live/hisar.example.com:ro" \
  nginx:stable-alpine sh -c 'rm /var/log/nginx/access.log && nginx -t && exec nginx -g "daemon off;"')
httpAddress=$(docker port "$container" 80/tcp)
httpsAddress=$(docker port "$container" 443/tcp)
ready=false
for ((attempt = 0; attempt < 50; attempt++)); do
  if curl --silent --fail --max-time 2 "http://$httpAddress/healthz" >/dev/null; then
    ready=true
    break
  fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then
  docker logs "$container"
  echo 'nginx did not become ready' >&2
  exit 1
fi
docker exec "$container" nginx -t

for scheme in http https; do
  if [[ "$scheme" == http ]]; then
    address=$httpAddress
    expectedStatus=301
  else
    address=$httpsAddress
    expectedStatus=502
  fi
  index=0
  while IFS='|' read -r path expected; do
    index=$((index + 1))
    agent="redaction-$scheme-$index"
    status=$(curl --silent --show-error --insecure --http1.1 --max-time 5 \
      --user-agent "$agent" --output /dev/null --write-out '%{http_code}' "$scheme://$address$path")
    if [[ "$status" != "$expectedStatus" ]]; then
      echo "unexpected $scheme status $status for $path" >&2
      exit 1
    fi
    logs=$(docker exec "$container" cat /var/log/nginx/access.log)
    if ! printf '%s\n' "$logs" | grep -F "\"GET $expected HTTP/1.1\" $expectedStatus " | grep -Fq "\"$agent\""; then
      echo "missing redacted $scheme access log for $path" >&2
      printf '%s\n' "$logs" >&2
      exit 1
    fi
    if [[ "$logs" == *secret-capability* || "$logs" == *private-email* ]]; then
      echo 'access log exposed a capability or query string' >&2
      printf '%s\n' "$logs" >&2
      exit 1
    fi
  done <<'PATHS'
/invite/secret-capability|/invite/:token
/INVITE/secret-capability|/invite/:token
/InViTe/secret-capability|/invite/:token
/s/secret-capability|/s/:id
/S/secret-capability|/s/:id
/api/v1/public/share-links/secret-capability|/api/v1/public/share-links/:id
/API/V1/PUBLIC/SHARE-LINKS/secret-capability|/api/v1/public/share-links/:id
/ApI/V1/PuBlIc/ShArE-LiNkS/secret-capability|/api/v1/public/share-links/:id
/api/v1/auth/prelogin?email=private-email@example.com|/api/v1/auth/prelogin
PATHS
done
echo 'nginx redacts capability paths and query strings in HTTPS and redirect access logs'
