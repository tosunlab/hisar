#!/usr/bin/env bash
# Runs the iOS XCUITests in the simulator against a freshly started hisar-server (scripts/ui-test-partner.sh).
# Requires HISAR_DATABASE_URL pointing at an empty, disposable database and mobile/build/HisarKit.xcframework
# (make build-ios-framework).
#
# The test classes run one after another on the simulator by default. IOS_UI_TEST_WORKERS=2 (or more) runs them on that many
# clones of the simulator at once, each class signing up its own account; that is a loss on the GitHub macOS runners, where two
# clones starve each other and tests time out, but can be quicker on a Mac with cores to spare. IOS_TESTS_PREBUILT=1 runs the
# tests make build-ios built instead of building them again. IOS_UI_TEST_PART=1, 2 or 3 runs one of three parts of similar
# length, for three CI jobs, as scripts/ui-test-parts.sh splits the classes; part 3 also runs every class new to its table.
#
# The simulator boots after the server and the partner are ready, and the tests start once it has finished booting and run its
# post-boot background work. A simulator booted earlier and then left alone goes down with CoreSimulatorService, which macOS
# reclaims when nothing is using it and memory runs short; xcodebuild then finds no simulator at all.
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
source "$ROOT/scripts/ui-test-partner.sh"

SIMULATOR_ID=${IOS_SIMULATOR_ID:-$("$ROOT/mobile/ios/scripts/find-simulator.sh")}
ACTION=test
if [ -n "${IOS_TESTS_PREBUILT:-}" ]; then ACTION=test-without-building; fi
WORKERS=${IOS_UI_TEST_WORKERS:-1}
PARALLEL=(-parallel-testing-enabled NO)
if [ "$WORKERS" -gt 1 ]; then PARALLEL=(-parallel-testing-enabled YES -parallel-testing-worker-count "$WORKERS"); fi

# xcodebuild boots its own clones of the simulator, and a booted simulator cannot be cloned.
#
# bootstatus returns once SpringBoard is up, but the simulator then runs a wave of background tasks (post-install and
# first-unlock work, dyld closures, and SpringBoard's poster prewarm, which renders every wallpaper through its extension). On
# a 3-core runner that wave can start minutes after bootstatus and keep the cores busy for two more, and a first test in it
# loses taps. The prewarm runs once per boot and is the wave's longest task, so its completion is when the tests can start.
# The simulator's log keeps earlier boots (log show --last boot does not tell them apart), so the query starts at the
# moment this boot's launchd_sim started.
wait_for_post_boot_work() {
  local predicate='process == "dasd" AND eventMessage BEGINSWITH "COMPLETED <_DASActivity: \"com.apple.springboard.posterboardupdate.prewarm:"'
  local deadline=$((SECONDS + 600)) launchd_pid boot_time
  launchd_pid=$(pgrep -f "launchd_sim .*/Devices/$SIMULATOR_ID/data/")
  # In UTC with an explicit offset: log show reads a time without one in its own zone, which need not be the caller's TZ.
  boot_time=$(LC_ALL=C date -j -u -f "%a %b %d %T %Y" "$(TZ=UTC LC_ALL=C ps -o lstart= -p "$launchd_pid" | xargs)" "+%Y-%m-%d %H:%M:%S+0000")
  echo "Waiting for the simulator's post-boot background work (SpringBoard's poster prewarm), booted at $boot_time"
  until xcrun simctl spawn "$SIMULATOR_ID" log show --start "$boot_time" --style compact --predicate "$predicate" 2>/dev/null | grep -q COMPLETED; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "the simulator's poster prewarm did not complete within 10 minutes of booting" >&2
      return 1
    fi
    sleep 2
  done
  echo "The simulator's post-boot background work finished"
}
if [ "$WORKERS" -eq 1 ]; then
  xcrun simctl bootstatus "$SIMULATOR_ID" -b
  wait_for_post_boot_work
fi

# XCTest fails a stalled app with "main thread busy for 30.0s" and no frame, so a sampler records what the main thread
# was doing (mobile/ios/scripts/sample-stalled-app.py) and a failure prints it. It watches one simulator, not clones.
# The diagnostic never decides the result: its output is bounded, its errors are ignored, and the exit status is the tests'.
UI_TEST_STALL_SAMPLER=
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
if [ "$WORKERS" -eq 1 ]; then
  /usr/bin/python3 -I "$ROOT/mobile/ios/scripts/sample-stalled-app.py" "$SIMULATOR_ID" "$T/main-thread" >/dev/null 2>&1 &
  UI_TEST_STALL_SAMPLER=$!
  trap 'status=$?; stop_stall_sampler; ui_test_stop; exit $status' EXIT
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
  exit 1
fi
