#!/usr/bin/env bash
# Fails when the hisar_fast_test_kdf build tag, or the HISAR_FAST_TEST_KDF switch that passes it to HisarKit, appears outside
# the files allowed to use it. The tag gives new accounts the minimum Argon2 parameters (core/crypt/newaccount_fast_test_kdf.go),
# so it belongs to test builds only: make test, CI's phone UI test jobs and the caches they restore. A release workflow or
# packaging script that mentions it fails here.
set -euo pipefail
cd "$(dirname "$0")/.."

allowed=(
  .github/workflows/caches.yml
  .github/workflows/ci.yml
  .github/workflows/macos.yml
  CLAUDE.md
  Makefile
  core/crypt/newaccount.go
  core/crypt/newaccount_fast_test_kdf.go
  docs/docs/development/releasing.md
  mobile/fast-test-kdf.sh
  scripts/check-fast-test-kdf.sh
)
unexpected=$(git grep -l -i -e hisar_fast_test_kdf -- . | grep -vxF -f <(printf '%s\n' "${allowed[@]}") || true)
if [ -n "$unexpected" ]; then
  printf 'hisar_fast_test_kdf is for test builds only; these files may not use it:\n%s\n' "$unexpected" >&2
  exit 1
fi
echo "hisar_fast_test_kdf appears only in test builds."
