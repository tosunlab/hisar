# shellcheck shell=bash
# Sourced by build-android-aar.sh and build-framework.sh: sets TAGS, the Go build tags of HisarKit.
# HISAR_FAST_TEST_KDF=1 builds a HisarKit whose new accounts get the minimum Argon2 parameters
# (core/crypt/newaccount_fast_test_kdf.go), for the phone UI tests on CI only. Outside GitHub Actions it refuses, so a
# HisarKit built on a Mac for an app release always has the real parameters.
TAGS=()
if [ "${HISAR_FAST_TEST_KDF:-}" = 1 ]; then
  if [ "${GITHUB_ACTIONS:-}" != true ]; then
    echo "HISAR_FAST_TEST_KDF=1 builds a HisarKit for CI's UI tests only; unset it" >&2
    exit 1
  fi
  echo "building HisarKit for tests: new accounts get the minimum Argon2 parameters"
  TAGS=(-tags hisar_fast_test_kdf)
fi
