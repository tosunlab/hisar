#!/usr/bin/env bash
# Prints the UDID of an available iPhone simulator on the newest installed iOS runtime.
set -euo pipefail
xcrun simctl list devices available --json | /usr/bin/python3 -I -c '
import json, sys
allDevice = json.load(sys.stdin)["devices"]
def readVersion(runtime):
    return [int(part) for part in runtime.rsplit("iOS-", 1)[-1].split("-") if part.isdigit()]
for runtime in sorted((name for name in allDevice if ".iOS-" in name), key=readVersion, reverse=True):
    for device in allDevice[runtime]:
        if device["name"].startswith("iPhone"):
            print(device["udid"])
            sys.exit(0)
sys.exit("no available iPhone simulator")
'
