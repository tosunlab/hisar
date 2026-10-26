#!/usr/bin/env bash
# Prints the UDID of an available iPhone simulator: a numbered iPhone (iPhone 17 Pro) on the newest installed iOS runtime
# that has one, so the tests run in the compact layout every iPhone shares, and the iPhone Duo only when no other iPhone
# exists.
set -euo pipefail
xcrun simctl list devices available --json | /usr/bin/python3 -I -c '
import json, re, sys
allDevice = json.load(sys.stdin)["devices"]
def readVersion(runtime):
    return [int(part) for part in runtime.rsplit("iOS-", 1)[-1].split("-") if part.isdigit()]
allRuntime = sorted((name for name in allDevice if ".iOS-" in name), key=readVersion, reverse=True)
allIPhone = [device for runtime in allRuntime for device in allDevice[runtime] if device["name"].startswith("iPhone")]
allNumbered = [device for device in allIPhone if re.match(r"iPhone \d", device["name"])]
for device in allNumbered + allIPhone:
    print(device["udid"])
    sys.exit(0)
sys.exit("no available iPhone simulator")
'
