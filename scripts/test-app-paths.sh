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

# commitChange <file> commits one change to <file> on top of $base and prints the commit.
commitChange() {
  git checkout -q "$base"
  mkdir -p "$(dirname "$1")"
  printf 'change\n' > "$1"
  git add "$1"
  git -c commit.gpgsign=false -c user.name=Test -c user.email=test@example.invalid commit -qm "Change $1"
  git rev-parse HEAD
}

# The phone UI tests and the admin browser tests run against hisar-server and the hisar CLI, and every job builds through
# the Makefile, so those changes select the tests that exercise them; a desktop build uses neither server nor phone code.
while read -r file desktop mobile android admin; do
  head=$(commitChange "$file")
  output=$(BASE_SHA=$base HEAD_SHA=$head WORKFLOW_FILE=.github/workflows/ci.yml GITHUB_OUTPUT=/dev/null \
    bash "$scriptDirectory/detect-app-changes.sh")
  for expected in "isDesktopChanged=$desktop" "isMobileChanged=$mobile" "isAndroidChanged=$android" "isAdminChanged=$admin"; do
    if [[ "$output" != *"$expected"* ]]; then
      printf 'A change to %s: expected %s, got:\n%s\n' "$file" "$expected" "$output" >&2
      exit 1
    fi
  done
done <<'EOF'
server/internal/api/server.go false true true true
cli/cmd/hisar/main.go true true true true
Makefile true true true true
mobile/ios/App/App.swift false true false false
mobile/android/app/build.gradle.kts false false true false
extension/src/background/controller.ts false false false false
EOF

# Release sets stay what each app is built from: a CLI change is not a reason to release the server.
head=$(commitChange cli/cmd/hisar/main.go)
setReleasePathspec server
if [ -n "$(git diff --name-only "$base" "$head" -- "${releasePathspec[@]}")" ]; then
  printf 'A CLI-only change selected a server release\n' >&2
  exit 1
fi
echo 'Server, CLI and Makefile changes select the tests that run against them; release sets are unchanged.'
