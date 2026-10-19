#!/usr/bin/env bash
# Runs the iOS XCUITests in the simulator against a freshly started hisar-server (scripts/ui-test-partner.sh).
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database and mobile/build/HisarKit.xcframework
# (make build-ios-framework).
#
# The test classes run one after another on the simulator by default. IOS_UI_TEST_WORKERS=2 (or more) runs them on that many
# clones of the simulator at once, each class signing up its own account; that is a loss on the GitHub macOS runners, where two
# clones starve each other and tests time out, but can be quicker on a Mac with cores to spare. IOS_TESTS_PREBUILT=1 runs the
# tests make build-ios built instead of building them again. IOS_UI_TEST_PART=1 or 2 runs one of two parts of similar length,
# for two CI jobs: part 2 runs SECOND_PART, part 1 every other class, new ones included.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
UI_TEST_PLATFORM_NAME=iOS
UI_TEST_PORT=${IOS_UI_TEST_PORT:-18082}
source "$ROOT/scripts/ui-test-partner.sh"

SIMULATOR_ID=${IOS_SIMULATOR_ID:-$("$ROOT/mobile/ios/scripts/find-simulator.sh")}
ACTION=test
if [ -n "${IOS_TESTS_PREBUILT:-}" ]; then ACTION=test-without-building; fi
WORKERS=${IOS_UI_TEST_WORKERS:-1}
PARALLEL=(-parallel-testing-enabled NO)
if [ "$WORKERS" -gt 1 ]; then PARALLEL=(-parallel-testing-enabled YES -parallel-testing-worker-count "$WORKERS"); fi

# Balanced by the classes' run times on GitHub's runners (part 1 about 790 s, part 2 about 1,020 s).
SECOND_PART=(OrganizationUITests SharingUITests AutoFillUITests AccountLifecycleUITests FaceIDUITests)
case "${IOS_UI_TEST_PART:-}" in
  "") SELECTION=(-only-testing:"${IOS_UI_TESTS:-HisarUITests}") ;;
  1) SELECTION=(-only-testing:HisarUITests "${SECOND_PART[@]/#/-skip-testing:HisarUITests/}") ;;
  2) SELECTION=("${SECOND_PART[@]/#/-only-testing:HisarUITests/}") ;;
  *) echo "IOS_UI_TEST_PART is 1 or 2" >&2; exit 1 ;;
esac
if [ -n "${IOS_UI_TEST_PART:-}" ] && [ -n "${IOS_UI_TESTS:-}" ]; then
  echo "set IOS_UI_TESTS or IOS_UI_TEST_PART, not both" >&2
  exit 1
fi

if ! env "${UI_TEST_VALUES[@]/#/TEST_RUNNER_}" TEST_RUNNER_HISAR_UI_TEST_SCREENSHOT_DIRECTORY="${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" \
  xcodebuild "$ACTION" -project "$ROOT/mobile/ios/Hisar.xcodeproj" -scheme Hisar "${SELECTION[@]}" \
  -destination "id=$SIMULATOR_ID" "${PARALLEL[@]}" ${IOS_DERIVED_DATA:+-derivedDataPath "$IOS_DERIVED_DATA"} ${IOS_RESULT_BUNDLE:+-resultBundlePath "$IOS_RESULT_BUNDLE"}; then
  ui_test_print_logs
  exit 1
fi
