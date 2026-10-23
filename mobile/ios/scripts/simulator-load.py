#!/usr/bin/env python3
"""Records how much CPU a booted simulator's own processes use, waits until they are quiet, and reports the timeline.

usage: simulator-load.py record <simulator UDID> <samples file>   (until SIGTERM)
       simulator-load.py wait <samples file>                       (returns once the simulator is quiet)
       simulator-load.py report <samples file>                     (per 30 s: cores used and the busiest processes)

After booting, a simulator runs minutes of background work: wallpaper posters, Spotlight, STExtractionService, Calendar's
first run, shortcuts, media analysis. On a 3-core runner that work starves the app and the keyboard, and the first UI test
of a job failed about 27 times as often as later ones, at whatever step the starvation hit. No single activity marks the
end of that work, so `wait` measures it: the tests start once the simulator's processes used less than QUIET_CORES over
the last QUIET_SECONDS. Each line of the samples file is {"t": epoch seconds, "cpu": {"<name>[<pid>]": CPU seconds}}.
"""
import collections
import json
import signal
import subprocess
import sys
import time

SAMPLE_SECONDS = 2
QUIET_CORES = 0.25
QUIET_SECONDS = 30
WAIT_LIMIT_SECONDS = 900
REPORT_BIN_SECONDS = 30


def read_cpu_seconds(text):
    total = 0.0
    for part in text.replace("-", ":").split(":"):
        total = total * 60 + float(part)
    return total


def simulator_root(udid):
    """The pid of the simulator's launchd_sim, or None when it is not running."""
    for pid in subprocess.run(["pgrep", "-x", "launchd_sim"], capture_output=True, text=True).stdout.split():
        args = subprocess.run(["ps", "-o", "args=", "-p", pid], capture_output=True, text=True).stdout
        if f"/Devices/{udid}/data/" in args:
            return int(pid)
    return None


def simulator_cpu(root):
    """CPU seconds of every process under the simulator's launchd_sim, by "<name>[<pid>]"."""
    # comm is the executable's path, which has spaces in a simulator runtime (iOS 26.5.simruntime), so it is the last column.
    rows = subprocess.run(["ps", "-A", "-o", "pid=,ppid=,time=,comm="], capture_output=True, text=True, check=True)
    children, processes = collections.defaultdict(list), {}
    for line in rows.stdout.splitlines():
        fields = line.split(None, 3)
        if len(fields) < 4:
            continue
        pid, ppid, cpu_time, command = fields
        children[int(ppid)].append(int(pid))
        processes[int(pid)] = (command.rsplit("/", 1)[-1], read_cpu_seconds(cpu_time))
    cpu, pending = {}, [root]
    while pending:
        pid = pending.pop()
        if pid in processes:
            name, seconds = processes[pid]
            cpu[f"{name}[{pid}]"] = seconds
            pending.extend(children[pid])
    return cpu


def used_between(earlier, later):
    """CPU seconds each process name used between two samples; a process new in `later` counts in full."""
    used = collections.Counter()
    for key, seconds in later.items():
        used[key.split("[", 1)[0]] += max(0.0, seconds - earlier.get(key, 0.0))
    return used


def read_samples(path):
    samples = []
    try:
        with open(path) as file:
            for line in file:
                try:
                    samples.append(json.loads(line))
                except json.JSONDecodeError:
                    break
    except FileNotFoundError:
        pass
    return samples


def record(udid, path):
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    root = None
    with open(path, "a") as file:
        while True:
            root = root or simulator_root(udid)
            cpu = simulator_cpu(root) if root else {}
            if cpu:
                file.write(json.dumps({"t": time.time(), "cpu": cpu}) + "\n")
                file.flush()
            else:
                root = None
            time.sleep(SAMPLE_SECONDS)


def busiest(used, count=4):
    return ", ".join(f"{name} {seconds:.1f}s" for name, seconds in used.most_common(count))


def wait(path):
    started = time.time()
    reported = started
    status = "no samples yet"
    while True:
        if time.time() - started > WAIT_LIMIT_SECONDS:
            sys.exit(f"the simulator was not quiet after {WAIT_LIMIT_SECONDS} s: {status}")
        samples = read_samples(path)
        if samples and time.time() - samples[-1]["t"] > 5 * SAMPLE_SECONDS:
            sys.exit("the simulator's load recording stopped")
        if samples and samples[-1]["t"] - samples[0]["t"] >= QUIET_SECONDS:
            last = samples[-1]
            first = next(sample for sample in reversed(samples) if last["t"] - sample["t"] >= QUIET_SECONDS)
            used = used_between(first["cpu"], last["cpu"])
            cores = sum(used.values()) / (last["t"] - first["t"])
            status = f"{cores:.2f} cores over the last {QUIET_SECONDS} s ({busiest(used)})"
            if cores < QUIET_CORES:
                print(f"The simulator is quiet: {status}, {last['t'] - samples[0]['t']:.0f} s after recording began")
                return
            if time.time() - reported >= REPORT_BIN_SECONDS:
                reported = time.time()
                print(f"Simulator busy: {status}", flush=True)
        time.sleep(SAMPLE_SECONDS)


def report(path):
    samples = read_samples(path)
    if len(samples) < 2:
        return
    start = samples[0]["t"]
    print(f"Simulator CPU per {REPORT_BIN_SECONDS} s since recording began (cores, busiest processes):")
    bin_start = samples[0]
    for sample in samples[1:]:
        if sample["t"] - bin_start["t"] >= REPORT_BIN_SECONDS or sample is samples[-1]:
            used = used_between(bin_start["cpu"], sample["cpu"])
            cores = sum(used.values()) / (sample["t"] - bin_start["t"])
            print(f"  {bin_start['t'] - start:5.0f} s  {cores:4.2f}  {busiest(used, 3)}")
            bin_start = sample


def main():
    command, *args = sys.argv[1:]
    {"record": record, "wait": wait, "report": report}[command](*args)


if __name__ == "__main__":
    main()
