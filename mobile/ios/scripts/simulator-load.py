#!/usr/bin/env python3
"""Records how much CPU a booted simulator's own processes use, waits until they are quiet, and reports the timeline.

usage: simulator-load.py record <simulator UDID> <samples file>   (until SIGTERM)
       simulator-load.py wait <samples file>                       (returns once the simulator is quiet)
       simulator-load.py report <samples file>                     (per 30 s: cores used and the busiest processes)

After booting, a simulator runs minutes of background work (wallpaper posters, Spotlight, STExtractionService, Calendar's
first run, shortcuts, media analysis). In CI the first UI test of a job failed far more often than later ones while that
work ran, at whichever step it slowed. No single activity marks its end, so `wait` measures it: the tests start once the
simulator's processes used less than QUIET_CORES over the last QUIET_SECONDS.

Each line of the samples file is {"t": epoch seconds, "cpu": {"<name>[<pid>@<start>]": CPU seconds}}. A process is keyed by
its pid and start time, so a reused pid is a new process. CPU between two samples is the sum of every process's increase,
counting a process first seen in the later sample in full; work by a process born and gone between samples is not seen.
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
COMMAND_TIMEOUT_SECONDS = 10

is_stopping = False


def read_cpu_seconds(text):
    total = 0.0
    for part in text.replace("-", ":").split(":"):
        total = total * 60 + float(part)
    return total


def processes():
    """{pid: (ppid, start, name, CPU seconds)} for every process on the machine."""
    # lstart is five words; comm is the executable's path, with spaces inside a simulator runtime, so it comes last.
    rows = subprocess.run(["ps", "-A", "-o", "pid=,ppid=,time=,lstart=,comm="], capture_output=True, text=True,
                          check=True, timeout=COMMAND_TIMEOUT_SECONDS).stdout
    result = {}
    for line in rows.splitlines():
        fields = line.split(None, 8)
        if len(fields) < 9:
            continue
        pid, ppid, cpu_time, *start, command = fields
        result[int(pid)] = (int(ppid), " ".join(start), command.rsplit("/", 1)[-1], read_cpu_seconds(cpu_time))
    return result


def simulator_root(udid, table):
    """(pid, start) of the simulator's launchd_sim, or None when it is not running."""
    for pid, (_, start, name, _) in table.items():
        if name != "launchd_sim":
            continue
        args = subprocess.run(["ps", "-o", "args=", "-p", str(pid)], capture_output=True, text=True,
                              timeout=COMMAND_TIMEOUT_SECONDS).stdout
        if f"/Devices/{udid}/data/" in args:
            return pid, start
    return None


def simulator_cpu(root, table):
    """CPU seconds of every process under the simulator's launchd_sim, by "<name>[<pid>@<start>]"."""
    children = collections.defaultdict(list)
    for pid, (ppid, *_rest) in table.items():
        children[ppid].append(pid)
    cpu, pending = {}, [root]
    while pending:
        pid = pending.pop()
        _, start, name, seconds = table[pid]
        cpu[f"{name}[{pid}@{start}]"] = seconds
        pending.extend(children[pid])
    return cpu


def used(samples):
    """CPU seconds each process name used across consecutive samples."""
    total = collections.Counter()
    for earlier, later in zip(samples, samples[1:]):
        for key, seconds in later["cpu"].items():
            total[key.split("[", 1)[0]] += max(0.0, seconds - earlier["cpu"].get(key, 0.0))
    return total


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
    def stop(*_):
        global is_stopping
        is_stopping = True

    signal.signal(signal.SIGTERM, stop)
    root = None
    with open(path, "a") as file:
        while not is_stopping:
            table = processes()
            if root is None or table.get(root[0], (None, None))[1] != root[1]:
                root = simulator_root(udid, table)
            if root is not None:
                file.write(json.dumps({"t": time.time(), "cpu": simulator_cpu(root[0], table)}) + "\n")
                file.flush()
            time.sleep(SAMPLE_SECONDS)


def busiest(seconds_by_name, count=4):
    return ", ".join(f"{name} {seconds:.1f}s" for name, seconds in seconds_by_name.most_common(count))


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
            end = samples[-1]["t"]
            begin = max(index for index, sample in enumerate(samples) if end - sample["t"] >= QUIET_SECONDS)
            window = samples[begin:]
            by_name = used(window)
            cores = sum(by_name.values()) / (end - window[0]["t"])
            status = f"{cores:.2f} cores over the last {QUIET_SECONDS} s ({busiest(by_name)})"
            if cores < QUIET_CORES:
                print(f"The simulator is quiet: {status}, {end - samples[0]['t']:.0f} s after recording began")
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
    begin = 0
    for index in range(1, len(samples)):
        if samples[index]["t"] - samples[begin]["t"] >= REPORT_BIN_SECONDS or index == len(samples) - 1:
            by_name = used(samples[begin:index + 1])
            cores = sum(by_name.values()) / (samples[index]["t"] - samples[begin]["t"])
            print(f"  {samples[begin]['t'] - start:5.0f} s  {cores:4.2f}  {busiest(by_name, 3)}")
            begin = index


def main():
    command, *args = sys.argv[1:]
    {"record": record, "wait": wait, "report": report}[command](*args)


if __name__ == "__main__":
    main()
