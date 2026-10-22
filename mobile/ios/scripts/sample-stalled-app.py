#!/usr/bin/env python3
"""Samples the Hisar app in a booted simulator whenever its main thread leaves an XCTest request unanswered for 10 s.

usage: sample-stalled-app.py <simulator UDID> <output prefix>

XCTest posts every accessibility query to the app's main run loop and fails the test with "main thread busy for 30.0s"
when the main thread does not run it. That message names no frame, so this writes the app's call graph to
<prefix>-<n>.txt while the stall lasts; scripts/test-ios-ui.sh prints the main thread's part when the tests fail.

Every child runs in its own process group and is owned here: SIGTERM, the end of the log stream and any error stop
them all, wait up to 3 s, then kill what is left. At most MAX_SAMPLES attempts are made, each bounded in time, and each
file is cut to MAX_SAMPLE_BYTES.
"""
import os
import re
import signal
import subprocess
import sys
import threading
import time

STALL_SECONDS = 10
SAMPLE_SECONDS = 3
SAMPLE_DEADLINE_SECONDS = 30
MAX_SAMPLES = 3
MAX_SAMPLE_BYTES = 2 << 20
STOP_SECONDS = 3


children = []
# Reentrant: the SIGTERM handler runs on the main thread, possibly while it is starting the log stream.
children_lock = threading.RLock()
is_stopping = False


def start_child(command, **options):
    """Starts command in its own process group, so stopping it also stops what it started (xcrun runs simctl)."""
    with children_lock:
        if is_stopping:
            return None
        child = subprocess.Popen(command, start_new_session=True, **options)
        children.append(child)
        return child


def signal_children(number):
    for child in children:
        if child.poll() is None:
            try:
                os.killpg(child.pid, number)
            except ProcessLookupError:
                pass


def stop_children():
    """Terminates every child, then kills the ones still running after STOP_SECONDS, and reaps them."""
    global is_stopping
    with children_lock:
        is_stopping = True
    signal_children(signal.SIGTERM)
    deadline = time.monotonic() + STOP_SECONDS
    while any(child.poll() is None for child in children) and time.monotonic() < deadline:
        time.sleep(0.1)
    signal_children(signal.SIGKILL)
    for child in children:
        child.wait()


def exit_on_signal(*_):
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
    stop_children()
    os._exit(0)


signal.signal(signal.SIGTERM, exit_on_signal)
udid, prefix = sys.argv[1], sys.argv[2]
posted_by_pid = {}
lock = threading.Lock()


def sample_stalls():
    for attempt in range(1, MAX_SAMPLES + 1):
        while True:
            time.sleep(1)
            with lock:
                stalled_pids = [pid for pid, posted in posted_by_pid.items() if posted and time.time() - posted[0] > STALL_SECONDS]
                for pid in stalled_pids:
                    posted_by_pid[pid] = []
            if stalled_pids:
                break
        path = f"{prefix}-{attempt}.txt"
        sampler = start_child(["sample", stalled_pids[0], str(SAMPLE_SECONDS), "10", "-mayDie", "-file", path],
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if sampler is None:
            return
        try:
            sampler.wait(timeout=SAMPLE_DEADLINE_SECONDS)
        except subprocess.TimeoutExpired:
            os.killpg(sampler.pid, signal.SIGKILL)
            sampler.wait()
        if os.path.exists(path) and os.path.getsize(path) > MAX_SAMPLE_BYTES:
            os.truncate(path, MAX_SAMPLE_BYTES)


try:
    stream = start_child(
        ["xcrun", "simctl", "spawn", udid, "log", "stream", "--style", "compact", "--predicate",
         'process == "Hisar" AND eventMessage CONTAINS "XCTPerformOnMainRunLoop"'],
        stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    threading.Thread(target=sample_stalls, daemon=True).start()
    for line in stream.stdout:
        match = re.search(r"Hisar\[(\d+):", line)
        if not match:
            continue
        with lock:
            posted = posted_by_pid.setdefault(match.group(1), [])
            if "about to CFRunLoopPerform" in line:
                posted.append(time.time())
            elif posted and ("inside CFRunLoopPerform" in line or "did not respond" in line):
                posted.pop(0)
finally:
    stop_children()
