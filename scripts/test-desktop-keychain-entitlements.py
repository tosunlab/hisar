#!/usr/bin/env python3
"""Exercise the signing profile boundary without accessing user keychain items."""

import copy
import datetime
import importlib.util
import pathlib
import unittest
import plistlib
import subprocess
import tempfile
from unittest import mock

spec = importlib.util.spec_from_file_location("desktop_signing", pathlib.Path(__file__).with_name("desktop-keychain-entitlements.py"))
signing = importlib.util.module_from_spec(spec)
spec.loader.exec_module(signing)


class ProfileTests(unittest.TestCase):
    def setUp(self):
        self.now = datetime.datetime(2026, 10, 20)
        self.profile = {
            "Platform": ["OSX"],
            "ProvisionsAllDevices": True,
            "ExpirationDate": datetime.datetime(2027, 10, 20),
            "TeamIdentifier": ["589NVZWDWZ"],
            "Entitlements": {
                "com.apple.application-identifier": "589NVZWDWZ.com.tryhisar.hisar",
                "keychain-access-groups": ["589NVZWDWZ.*"],
            },
        }

    def test_matching_profile_emits_only_authorized_entitlements(self):
        value = signing.entitlements(self.profile, "com.tryhisar.hisar", self.now)
        self.assertEqual(value, {
            "com.apple.application-identifier": "589NVZWDWZ.com.tryhisar.hisar",
            "com.apple.developer.team-identifier": "589NVZWDWZ",
            "keychain-access-groups": ["589NVZWDWZ.com.tryhisar.hisar"],
        })

    def test_wrong_platform_team_app_expiry_or_group_is_refused(self):
        for field, value in [("Platform", ["iOS"]), ("TeamIdentifier", ["OTHER"]), ("ExpirationDate", self.now), ("ProvisionsAllDevices", False)]:
            with self.subTest(field=field):
                profile = copy.deepcopy(self.profile)
                profile[field] = value
                with self.assertRaises(ValueError):
                    signing.entitlements(profile, "com.tryhisar.hisar", self.now)
        for field, value in [("com.apple.application-identifier", "589NVZWDWZ.other.app"), ("keychain-access-groups", ["589NVZWDWZ.other.app"])]:
            with self.subTest(field=field):
                profile = copy.deepcopy(self.profile)
                profile["Entitlements"][field] = value
                with self.assertRaises(ValueError):
                    signing.entitlements(profile, "com.tryhisar.hisar", self.now)

    def test_identity_must_match_a_profile_certificate(self):
        certificate = b"test certificate"
        self.profile["DeveloperCertificates"] = [certificate]
        fingerprint = signing.hashlib.sha1(certificate).hexdigest().upper()
        name = "Developer ID Application: Hisar (589NVZWDWZ)"
        identities = f'  1) {fingerprint} "{name}"'
        signing.validate_identity(self.profile, fingerprint, identities)
        signing.validate_identity(self.profile, name, identities)
        with self.assertRaises(ValueError):
            signing.validate_identity(self.profile, "0" * 40, identities)
        self.profile["DeveloperCertificates"] = [b"another certificate"]
        with self.assertRaises(ValueError):
            signing.validate_identity(self.profile, fingerprint, identities)

    def test_signing_accepts_the_real_bundle_info_format(self):
        self.profile["ExpirationDate"] = datetime.datetime.now() + datetime.timedelta(days=365)
        certificate = b"test certificate"
        self.profile["DeveloperCertificates"] = [certificate]
        fingerprint = signing.hashlib.sha1(certificate).hexdigest().upper()
        name = "Developer ID Application: Hisar (589NVZWDWZ)"
        real_run = subprocess.run

        def run(arguments, **options):
            if arguments[:3] == ["security", "cms", "-D"]:
                return subprocess.CompletedProcess(arguments, 0, plistlib.dumps(self.profile))
            if arguments[:2] == ["security", "find-identity"]:
                return subprocess.CompletedProcess(arguments, 0, f'1) {fingerprint} "{name}"')
            return real_run(arguments, **options)

        info = pathlib.Path(__file__).parent.parent / "desktop/build/darwin/Info.plist"
        with tempfile.TemporaryDirectory() as directory:
            output = pathlib.Path(directory) / "entitlements.plist"
            with mock.patch.object(signing.subprocess, "run", side_effect=run), mock.patch.object(signing.sys, "argv", ["signing", "fixture.profile", str(info), str(output), fingerprint]):
                signing.main()
            with output.open("rb") as stream:
                self.assertEqual(plistlib.load(stream)["keychain-access-groups"], ["589NVZWDWZ.com.tryhisar.hisar"])


if __name__ == "__main__":
    unittest.main()
