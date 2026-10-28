#!/usr/bin/env bash
# Runs the iOS XCUITests in the simulator against a freshly started hisar-server (scripts/ui-test-partner.sh).
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database and mobile/build/HisarKit.xcframework
# (make build-ios-framework).
#
# The test classes run one after another on the simulator by default. IOS_UI_TEST_WORKERS=2 (or more) runs them on that many
# clones of the simulator at once, each class signing up its own account; that is a loss on the GitHub macOS runners, where two
# clones starve each other and tests time out, but can be quicker on a Mac with cores to spare. IOS_TESTS_PREBUILT=1 runs the
# tests make build-ios built instead of building them again. IOS_UI_TEST_PART=1, 2 or 3 runs one of three parts of similar
# length, for CI (part 1 in the build job, 2 and 3 in jobs of their own), as scripts/ui-test-parts.sh splits the classes;
# part 3 also runs every class new to its table.
#
# The simulator boots after the server and the partner are ready, and the tests start once it has finished booting and its
# post-boot background work is over. A simulator booted earlier and then left alone goes down with CoreSimulatorService, which
# macOS reclaims when nothing is using it and memory runs short; xcodebuild then finds no simulator at all.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
source "$ROOT/scripts/ui-test-parts.sh"
case "${IOS_UI_TEST_PART:-}" in
  "") SELECTION=(-only-testing:"${IOS_UI_TESTS:-HisarUITests}") ;;
  1 | 2)
    ui_test_part "$IOS_UI_TEST_PART" 3 "${IOS_UI_TEST_SECONDS[@]}"
    SELECTION=("${UI_TEST_PART_CLASSES[@]/#/-only-testing:HisarUITests/}")
    ;;
  3)
    ui_test_part 3 3 "${IOS_UI_TEST_SECONDS[@]}"
    SELECTION=(-only-testing:HisarUITests "${UI_TEST_PART_CLASSES[@]/#/-skip-testing:HisarUITests/}")
    ;;
  *) echo "IOS_UI_TEST_PART is 1, 2 or 3" >&2; exit 1 ;;
esac
if [ -n "${IOS_UI_TEST_PART:-}" ] && [ -n "${IOS_UI_TESTS:-}" ]; then
  echo "set IOS_UI_TESTS or IOS_UI_TEST_PART, not both" >&2
  exit 1
fi

UI_TEST_PLATFORM_NAME=iOS
UI_TEST_PORT=${IOS_UI_TEST_PORT:-18082}
UI_TEST_OPEN_PORT=${IOS_UI_TEST_OPEN_PORT:-18086}
source "$ROOT/scripts/ui-test-partner.sh"

SIMULATOR_ID=${IOS_SIMULATOR_ID:-$("$ROOT/mobile/ios/scripts/find-simulator.sh")}
ACTION=test
if [ -n "${IOS_TESTS_PREBUILT:-}" ]; then ACTION=test-without-building; fi
WORKERS=${IOS_UI_TEST_WORKERS:-1}
PARALLEL=(-parallel-testing-enabled NO)
if [ "$WORKERS" -gt 1 ]; then PARALLEL=(-parallel-testing-enabled YES -parallel-testing-worker-count "$WORKERS"); fi

# XCTest fails a stalled app with "main thread busy for 30.0s" and no frame, so a sampler records what the main thread
# was doing (mobile/ios/scripts/sample-stalled-app.py) and a failure prints it. It watches one simulator, not clones.
# The diagnostic never decides the result: its output is bounded, its errors are ignored, and the exit status is the tests'.
UI_TEST_STALL_SAMPLER=
UI_TEST_LOAD_RECORDER=
stop_stall_sampler() {
  local pid=$UI_TEST_STALL_SAMPLER
  [ -n "$pid" ] || return 0
  UI_TEST_STALL_SAMPLER=
  # The sampler stops its children within 3 s of SIGTERM; a sampler still running after 5 s is killed.
  kill -TERM "$pid" 2>/dev/null || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.5
  done
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
stop_load_recorder() {
  local pid=$UI_TEST_LOAD_RECORDER
  [ -n "$pid" ] || return 0
  UI_TEST_LOAD_RECORDER=
  # The recorder stops within one sample of SIGTERM; one still running after 5 s is killed.
  kill -TERM "$pid" 2>/dev/null || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.5
  done
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
# xcodebuild boots its own clones of the simulator, and a booted simulator cannot be cloned. The tests start once the
# simulator has run its deferred post-boot work (mobile/ios/scripts/wait-for-post-boot-work.sh: the poster prewarm comes
# minutes after the boot) and its own processes are then quiet (mobile/ios/scripts/simulator-load.py), because the rest of
# the wave outlasts the prewarm. The recording goes on through the tests, and a failure prints it.
if [ "$WORKERS" -eq 1 ]; then
  trap 'status=$?; stop_stall_sampler; stop_load_recorder; ui_test_stop; exit $status' EXIT
  xcrun simctl bootstatus "$SIMULATOR_ID" -b
  /usr/bin/python3 -I "$ROOT/mobile/ios/scripts/simulator-load.py" record "$SIMULATOR_ID" "$T/simulator-load.jsonl" &
  UI_TEST_LOAD_RECORDER=$!
  "$ROOT/mobile/ios/scripts/wait-for-post-boot-work.sh" "$SIMULATOR_ID"
  /usr/bin/python3 -I "$ROOT/mobile/ios/scripts/simulator-load.py" wait "$T/simulator-load.jsonl"
  /usr/bin/python3 -I "$ROOT/mobile/ios/scripts/sample-stalled-app.py" "$SIMULATOR_ID" "$T/main-thread" >/dev/null 2>&1 &
  UI_TEST_STALL_SAMPLER=$!
fi
print_main_thread_samples() {
  local sample
  for sample in "$T"/main-thread-*.txt; do
    [ -e "$sample" ] || return 0
    echo "main thread while it left XCTest waiting ($(basename "$sample"), frames in at least a tenth of the samples):" >&2
    awk '/^    [0-9]+ Thread_.*(Main Thread|com\.apple\.main-thread)/ { total = $1; inside = 1; print substr($0, 1, 240); next }
      inside && /^    [0-9]+ Thread_/ { exit }
      inside && match($0, /[0-9]+ /) && substr($0, RSTART, RLENGTH) * 10 >= total { print substr($0, 1, 240); if (++printed == 60) exit }' "$sample" >&2
  done
}

if ! env "${UI_TEST_VALUES[@]/#/TEST_RUNNER_}" TEST_RUNNER_HISAR_UI_TEST_SCREENSHOT_DIRECTORY="${HISAR_UI_TEST_SCREENSHOT_DIRECTORY:-}" \
  xcodebuild "$ACTION" -project "$ROOT/mobile/ios/Hisar.xcodeproj" -scheme Hisar "${SELECTION[@]}" \
  -destination "id=$SIMULATOR_ID" "${PARALLEL[@]}" ${IOS_DERIVED_DATA:+-derivedDataPath "$IOS_DERIVED_DATA"} ${IOS_RESULT_BUNDLE:+-resultBundlePath "$IOS_RESULT_BUNDLE"}; then
  ui_test_print_logs
  print_main_thread_samples || true
  /usr/bin/python3 -I "$ROOT/mobile/ios/scripts/simulator-load.py" report "$T/simulator-load.jsonl" >&2 || true
  exit 1
fi
