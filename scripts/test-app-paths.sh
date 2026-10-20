#!/usr/bin/env bash
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
workDirectory=$(mktemp -d)
trap 'rm -rf "$workDirectory"' EXIT
cd "$workDirectory"
git init -q
git -c commit.gpgsign=false -c user.name=Test -c user.email=test@example.invalid commit --allow-empty -qm base
base=$(git rev-parse HEAD)
printf '*\n' > .dockerignore
git add .dockerignore
git -c commit.gpgsign=false -c user.name=Test -c user.email=test@example.invalid commit -qm 'Change only Docker context policy'
head=$(git rev-parse HEAD)

# shellcheck source=scripts/app-paths.sh
source "$scriptDirectory/app-paths.sh"
releasePathspec=()
setReleasePathspec server
selected=$(git diff --name-only "$base" "$head" -- "${releasePathspec[@]}")
if [ "$selected" != .dockerignore ]; then
  printf 'Expected server release inputs to include only .dockerignore, got: %s\n' "$selected" >&2
  exit 1
fi
output=$(BASE_SHA=$base HEAD_SHA=$head WORKFLOW_FILE=.github/workflows/ci.yml GITHUB_OUTPUT=/dev/null \
  bash "$scriptDirectory/detect-app-changes.sh")
for expected in isAdminChanged=true isDesktopChanged=false isMobileChanged=false isAndroidChanged=false; do
  if [[ "$output" != *"$expected"* ]]; then
    printf 'Expected change detector output to contain %s, got:\n%s\n' "$expected" "$output" >&2
    exit 1
  fi
done
printf '%s\n' "$selected" "$output" 'Docker-context-only change selects server releases and admin/image CI.'
