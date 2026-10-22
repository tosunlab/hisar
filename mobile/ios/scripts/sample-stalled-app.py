#!/usr/bin/env python3
"""Samples the Hisar app in a booted simulator whenever its main thread leaves an XCTest request unanswered for 10 s.

usage: sample-stalled-app.py <simulator UDID> <output prefix>

XCTest posts every accessibility query to the app's main run loop and fails the test with "main thread busy for 30.0s"
when the main thread does not run it. That message names no frame, so this writes the app's call graph to
<prefix>-<n>.txt while the stall lasts; scripts/test-ios-ui.sh prints the main thread's part when the tests fail.

Every child runs in its own process group, recorded before SIGTERM can be handled: SIGTERM, the end of the log stream
and any error signal each group, wait up to 3 s for it to empty, then kill it. At most MAX_SAMPLES attempts are made,
each bounded in time, and `sample` writes into a pipe of which at most MAX_SAMPLE_BYTES reach the file.
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
children_lock = threading.Lock()
is_stopping = False


def start_child(command, **options):
    """Starts command as the leader of a new process group and records it, or returns None once stopping began.

    The SIGTERM handler takes children_lock, so it waits for a child being started to be recorded."""
    with children_lock:
        if is_stopping:
            return None
        child = subprocess.Popen(command, start_new_session=True, **options)
        children.append(child)
        return child


def is_group_alive(pgid):
    try:
        os.killpg(pgid, 0)
        return True
    except (ProcessLookupError, PermissionError):
        return False


def signal_group(pgid, number):
    try:
        os.killpg(pgid, number)
    except (ProcessLookupError, PermissionError):
        pass


def stop_children():
    """Signals each child's process group by its pgid, also after its leader exited, waits STOP_SECONDS, kills, reaps."""
    global is_stopping
    with children_lock:
        is_stopping = True
    for child in children:
        signal_group(child.pid, signal.SIGTERM)
    deadline = time.monotonic() + STOP_SECONDS
    while any(is_group_alive(child.pid) for child in children) and time.monotonic() < deadline:
        for child in children:
            child.poll()
        time.sleep(0.1)
    for child in children:
        signal_group(child.pid, signal.SIGKILL)
    for child in children:
        child.wait()


def exit_on_signal(*_):
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
    stop_children()
    os._exit(0)


def sample_app(pid, path):
    """Samples pid into path; `sample` writes to a pipe, so at most MAX_SAMPLE_BYTES ever reach the disk."""
    sampler = start_child(["sample", pid, str(SAMPLE_SECONDS), "10", "-mayDie", "-file", "/dev/stdout"],
                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    if sampler is None:
        return False
    timer = threading.Timer(SAMPLE_DEADLINE_SECONDS, signal_group, (sampler.pid, signal.SIGKILL))
    timer.start()
    report = sampler.stdout.read(MAX_SAMPLE_BYTES)
    with open(path, "wb") as output:
        output.write(report)
    timer.cancel()
    signal_group(sampler.pid, signal.SIGKILL)
    sampler.stdout.close()
    sampler.wait()
    return True


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
        if not sample_app(stalled_pids[0], f"{prefix}-{attempt}.txt"):
            return


udid, prefix = sys.argv[1], sys.argv[2]
posted_by_pid = {}
lock = threading.Lock()
# SIGTERM stays blocked until the stream is recorded; the sampling thread inherits the block, so only the main thread,
# which starts no child after that, runs the handler.
signal.pthread_sigmask(signal.SIG_BLOCK, {signal.SIGTERM})
signal.signal(signal.SIGTERM, exit_on_signal)
try:
    stream = start_child(
        ["xcrun", "simctl", "spawn", udid, "log", "stream", "--style", "compact", "--predicate",
         'process == "Hisar" AND eventMessage CONTAINS "XCTPerformOnMainRunLoop"'],
        stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    threading.Thread(target=sample_stalls, daemon=True).start()
    signal.pthread_sigmask(signal.SIG_UNBLOCK, {signal.SIGTERM})
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
    signal.pthread_sigmask(signal.SIG_BLOCK, {signal.SIGTERM})
    stop_children()
