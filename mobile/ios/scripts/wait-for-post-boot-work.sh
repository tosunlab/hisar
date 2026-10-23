#!/usr/bin/env bash
# Returns once a booted simulator has run its post-boot background work, so the first UI test does not run inside it.
# Usage: wait-for-post-boot-work.sh <udid>
#
# simctl bootstatus returns once SpringBoard is up, but the simulator then runs a wave of background tasks (post-install and
# first-unlock work, dyld closures, and SpringBoard's poster prewarm, which renders every wallpaper through its extension). On
# a 3-core runner that wave can start minutes after bootstatus and keep the cores busy for two more, and a first test in it
# loses taps. The prewarm is the wave's longest task, and dasd logs when it submits, starts and completes it:
# - completed or canceled: the wave is over;
# - not submitted 8 minutes after the boot: this boot has none (on the runners it came 3.6 to 4.6 minutes after the boot);
# - submitted but not started 15 minutes after the boot: dasd is not running it, so it cannot load the tests either;
# - started and still running 15 minutes after the boot: the script fails with what dasd logged.
# A query that fails stops the script with its error, so a broken query cannot pass for a prewarm that has not finished.
# The simulator's log keeps earlier boots (log show --last boot does not tell them apart), so the query starts at the moment
# this boot's launchd_sim started.
set -euo pipefail

udid=$1
activity=com.apple.springboard.posterboardupdate.prewarm
# By process name: pgrep -f on the path also matches any process whose arguments mention it, such as a shell running this one.
launchd_pid=
for pid in $(pgrep -x launchd_sim || true); do
  if [[ "$(ps -o args= -p "$pid")" == *"/Devices/$udid/data/"* ]]; then launchd_pid=$pid; fi
done
[ -n "$launchd_pid" ] || { echo "simulator $udid is not booted" >&2; exit 1; }
boot_epoch=$(LC_ALL=C date -j -f "%a %b %d %T %Y" "$(LC_ALL=C ps -o lstart= -p "$launchd_pid" | xargs)" +%s)
# In UTC with an explicit offset: log show reads a time without one in its own zone, which need not be the caller's.
boot_time=$(date -u -r "$boot_epoch" "+%Y-%m-%d %H:%M:%S+0000")
predicate="process == \"dasd\" AND eventMessage CONTAINS \"$activity:\" AND (eventMessage BEGINSWITH \"SUBMITTING: \""
predicate+=" OR eventMessage BEGINSWITH \"STARTING: \" OR eventMessage BEGINSWITH \"COMPLETED <\" OR eventMessage BEGINSWITH \"CANCELED: \")"
error_file=$(mktemp)
trap 'rm -f "$error_file"' EXIT

echo "Waiting for the simulator's post-boot background work (SpringBoard's poster prewarm), booted at $boot_time"
reported=
while true; do
  if ! lines=$(xcrun simctl spawn "$udid" log show --start "$boot_time" --style compact --predicate "$predicate" 2>"$error_file"); then
    echo "reading the simulator's log for $activity failed:" >&2
    cat "$error_file" >&2
    exit 1
  fi
  seconds=$(($(date +%s) - boot_epoch))
  state=waiting
  if grep -q -e '] COMPLETED <' -e '] CANCELED: ' <<<"$lines"; then
    state=finished
  elif ! grep -q '] SUBMITTING: ' <<<"$lines"; then
    if [ "$seconds" -ge 480 ]; then state=never-submitted; fi
  elif ! grep -q '] STARTING: ' <<<"$lines"; then
    state=submitted
    if [ "$seconds" -ge 900 ]; then state=never-started; fi
  else
    state=running
    if [ "$seconds" -ge 900 ]; then state=stuck; fi
  fi
  if [ "$state" != "$reported" ] && { [ "$state" = submitted ] || [ "$state" = running ]; }; then
    echo "The poster prewarm is $state ($seconds s after the boot)"
  fi
  reported=$state
  case "$state" in
    finished) echo "The simulator's post-boot background work finished ($seconds s after the boot)"; exit 0 ;;
    never-submitted) echo "No poster prewarm was submitted in the 8 minutes after the boot; the tests start"; exit 0 ;;
    never-started) echo "dasd submitted the poster prewarm but did not start it in the 15 minutes after the boot; the tests start"; exit 0 ;;
    stuck)
      echo "the simulator's poster prewarm was still running 15 minutes after the boot; dasd logged:" >&2
      printf '%s\n' "$lines" >&2
      exit 1
      ;;
  esac
  sleep 2
done
