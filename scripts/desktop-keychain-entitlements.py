#!/usr/bin/env python3
"""Validate a local macOS profile and emit the app's keychain entitlements."""

import datetime
import fnmatch
import hashlib
import plistlib
import re
import subprocess
import sys


def entitlements(profile, bundle_id, now):
    if "OSX" not in profile.get("Platform", []):
        raise ValueError("the provisioning profile must authorize macOS, not iOS")
    if profile.get("ExpirationDate", datetime.datetime.min) <= now:
        raise ValueError("the macOS provisioning profile has expired")
    team = "589NVZWDWZ"
    if profile.get("TeamIdentifier") != [team]:
        raise ValueError("the macOS provisioning profile must belong to team 589NVZWDWZ")
    if not profile.get("ProvisionsAllDevices"):
        raise ValueError("the macOS Developer ID profile must authorize distribution to all devices")
    allowed = profile.get("Entitlements", {})
    if allowed.get("get-task-allow", False):
        raise ValueError("a release profile must not enable debugger access")
    application_id = team + "." + bundle_id
    if not fnmatch.fnmatchcase(application_id, allowed.get("com.apple.application-identifier", "")):
        raise ValueError("the macOS provisioning profile does not authorize this app identifier")
    group = application_id
    if not any(fnmatch.fnmatchcase(group, entry) for entry in allowed.get("keychain-access-groups", [])):
        raise ValueError("the macOS provisioning profile does not authorize the app's keychain access group")
    return {
        "com.apple.application-identifier": application_id,
        "com.apple.developer.team-identifier": team,
        "keychain-access-groups": [group],
    }


def validate_identity(profile, identity, identities):
    certificates = {hashlib.sha1(bytes(cert)).hexdigest().upper() for cert in profile.get("DeveloperCertificates", [])}
    for fingerprint, name in re.findall(r'\d+\)\s+([0-9A-F]{40})\s+"([^"]+)"', identities):
        if identity in (fingerprint, name) and fingerprint in certificates and name.startswith("Developer ID Application: ") and name.endswith("(589NVZWDWZ)"):
            return
    raise ValueError("the signing identity must be a valid Developer ID identity authorized by this macOS profile")


def main():
    profile_path, info_path, output_path, identity = sys.argv[1:]
    decoded = subprocess.run(["security", "cms", "-D", "-i", profile_path], check=True, capture_output=True)
    profile = plistlib.loads(decoded.stdout)
    info = subprocess.run(["plutil", "-convert", "xml1", "-o", "-", info_path], check=True, capture_output=True)
    bundle_id = plistlib.loads(info.stdout)["CFBundleIdentifier"]
    value = entitlements(profile, bundle_id, datetime.datetime.now(datetime.timezone.utc).replace(tzinfo=None))
    identities = subprocess.run(["security", "find-identity", "-v", "-p", "codesigning"], check=True, capture_output=True, text=True)
    validate_identity(profile, identity, identities.stdout)
    with open(output_path, "wb") as stream:
        plistlib.dump(value, stream)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        sys.exit("Touch ID signing setup failed: " + str(error))
