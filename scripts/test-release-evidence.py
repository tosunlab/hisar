#!/usr/bin/env python3
"""Exercise release evidence with disposable Git history and a local attestation service double."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest
import zipfile


ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts/release-evidence.py"
PUBLISH = ROOT / ".github/workflows/publish.yml"


def workflow_script(workflow, containing):
    """Return the one `run: |` block of the workflow that contains the given text."""
    lines = workflow.read_text().splitlines()
    scripts = []
    for index, line in enumerate(lines):
        key = line.lstrip().removeprefix("- ")
        if key != "run: |":
            continue
        indent = len(line) - len(key)
        body = []
        for following in lines[index + 1:]:
            if following.strip() and len(following) - len(following.lstrip()) <= indent:
                break
            body.append(following)
        scripts.append(textwrap.dedent("\n".join(body)))
    matches = [script for script in scripts if containing in script]
    if len(matches) != 1:
        raise AssertionError(f"{workflow} has {len(matches)} run steps containing {containing!r}")
    return matches[0]


class ReleaseEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repository = self.root / "repository"
        self.repository.mkdir()
        self.directory = self.root / "release-files"
        self.directory.mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "attestations.jsonl"
        self.env = dict(os.environ, GH_REPO="tosunlab/hisar", GITHUB_REPOSITORY="tosunlab/hisar",
                        GITHUB_RUN_ID="1234", CI_RESULT="success", MACOS_RESULT="success",
                        IMAGE_DIGEST="sha256:" + "a" * 64, ATTESTATION_LOG=str(self.log),
                        PATH=str(self.bin) + os.pathsep + os.environ["PATH"])
        gh = self.bin / "gh"
        gh.write_text("#!" + sys.executable + "\n" +
                      "import json, os, shutil, sys\n"
                      "if sys.argv[1:3] == ['release', 'download']:\n"
                      "    shutil.copytree(os.environ['PUBLISHED_RELEASE'], sys.argv[sys.argv.index('--dir') + 1],"
                      " dirs_exist_ok=True)\n"
                      "    sys.exit(0)\n"
                      "with open(os.environ['ATTESTATION_LOG'], 'a') as f: f.write(json.dumps(sys.argv[1:]) + '\\n')\n"
                      "sys.exit(1 if os.environ.get('REJECT_ATTESTATION') else 0)\n")
        gh.chmod(0o755)
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.invalid")
        self.git("config", "commit.gpgsign", "false")
        self.git("config", "tag.gpgsign", "false")
        self.git("commit", "--allow-empty", "-qm", "tested commit")
        self.sha = self.git("rev-parse", "HEAD")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.repository, text=True).strip()

    def run_script(self, *args, success=True):
        result = subprocess.run([sys.executable, str(SCRIPT), *args], cwd=self.repository,
                                env=self.env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.assertEqual(result.returncode == 0, success, result.stdout)
        return result

    def fixture(self, app, version="1.2.3"):
        self.app = app
        self.tag = app + "-" + version
        self.git("tag", "-a", self.tag, "-m", "Release")
        names = []
        if app == "extension":
            names = [f"hisar-extension-{version}.zip"]
        elif app == "cli":
            names = [f"hisar-{version}-{system}-{arch}.{'zip' if system == 'windows' else 'tar.gz'}"
                     for system in ("linux", "darwin", "windows") for arch in ("amd64", "arm64")]
        elif app == "desktop":
            names = [f"Hisar-{version}.dmg"]
            for arch in ("amd64", "arm64"):
                names.extend(f"Hisar-{version}-linux-{arch}.{kind}" for kind in ("deb", "rpm", "AppImage", "flatpak"))
            if "-" not in version:
                names.append("update-unpublished.json")
                with zipfile.ZipFile(self.directory / f"Hisar-{version}.zip", "w") as archive:
                    for name in ("package.json", "mcp.json", "bin/run-hisar"):
                        info = zipfile.ZipInfo("Hisar.app/Contents/Resources/omp-plugin/" + name)
                        info.external_attr = 0o100755 << 16
                        archive.writestr(info, b"fixture")
        for name in names:
            (self.directory / name).write_bytes(b"release fixture\n")
        if app == "cli":
            self.checksums("SHA256SUMS", names)
        elif app == "desktop":
            for arch in ("amd64", "arm64"):
                self.checksums("SHA256SUMS-linux-" + arch,
                               [f"Hisar-{version}-linux-{arch}.{kind}" for kind in ("deb", "rpm", "AppImage")])
                self.checksums("SHA256SUMS-flatpak-" + arch, [f"Hisar-{version}-linux-{arch}.flatpak"])

    def checksums(self, name, files):
        (self.directory / name).write_text("".join(hashlib.sha256((self.directory / file).read_bytes()).hexdigest()
                                                  + "  " + file + "\n" for file in files))

    def create(self, success=True):
        return self.run_script("create", self.app, self.tag, self.sha, str(self.directory), success=success)

    def verify(self, success=True):
        return self.run_script("verify", self.app, self.tag, self.sha, str(self.directory), success=success)

    def publish_step(self, containing, runner_temp):
        """Run the publish.yml step containing the given text as GitHub runs a bash step, in the default-branch checkout."""
        env = dict(self.env, APP=self.app, TAG=self.tag, VERSION=self.tag.removeprefix(self.app + "-"),
                   SOURCE_SHA=self.sha, RUNNER_TEMP=str(runner_temp))
        return subprocess.run(["bash", "--noprofile", "--norc", "-eo", "pipefail", "-c",
                               workflow_script(PUBLISH, containing)], cwd=self.repository, env=env, text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

    def test_each_app_attests_every_delivered_file(self):
        for app in ("server", "cli", "desktop", "extension"):
            with self.subTest(app=app):
                for path in self.directory.iterdir():
                    path.unlink()
                self.log.unlink(missing_ok=True)
                self.fixture(app)
                self.create()
                self.verify()
                calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                expected = {str(path) for path in self.directory.iterdir()}
                if app == "server":
                    expected.add("oci://ghcr.io/tosunlab/hisar-server@" + self.env["IMAGE_DIGEST"])
                self.assertEqual({call[2] for call in calls}, expected)
                for call in calls:
                    flags = dict(zip(call[3:-1:2], call[4:-1:2]))
                    self.assertEqual(flags["--repo"], "tosunlab/hisar")
                    workflow = "release-server.yml" if call[2].startswith("oci://") else "release.yml"
                    self.assertEqual(flags["--signer-workflow"], "tosunlab/hisar/.github/workflows/" + workflow)
                    self.assertEqual(flags["--source-digest"], self.sha)
                    self.assertEqual(flags["--signer-digest"], self.sha)
                    self.assertEqual(call[-1], "--deny-self-hosted-runners")

    def test_failed_skipped_cancelled_or_missing_tests_cannot_create_evidence(self):
        self.fixture("desktop")
        for key in ("CI_RESULT", "MACOS_RESULT"):
            for result in ("failure", "skipped", "cancelled", ""):
                with self.subTest(key=key, result=result):
                    self.env[key] = result
                    self.create(success=False)
                    self.assertFalse((self.directory / "release-evidence.json").exists())
            self.env[key] = "success"

    def test_custom_version_still_requires_tested_source_and_files(self):
        self.fixture("desktop", "1.2.3-local")
        self.create()
        self.verify()

    def test_moving_annotated_or_lightweight_tag_is_rejected(self):
        self.fixture("extension")
        self.run_script("check-source", self.sha, self.tag)
        self.git("commit", "--allow-empty", "-qm", "different source")
        other = self.git("rev-parse", "HEAD")
        self.git("checkout", "--detach", self.sha)
        for annotated in (False, True):
            self.git("tag", "-d", self.tag)
            options = ("-a", "-m", "moved") if annotated else ()
            self.git("tag", *options, self.tag, other)
            self.run_script("check-source", self.sha, self.tag, success=False)
            self.create(success=False)

    def test_checkout_must_equal_tested_sha_even_when_tag_is_correct(self):
        self.fixture("extension")
        self.git("commit", "--allow-empty", "-qm", "different checkout")
        self.create(success=False)

    def test_modified_missing_extra_and_symlink_artifacts_are_rejected(self):
        self.fixture("extension")
        self.create()
        asset = next(path for path in self.directory.iterdir() if path.suffix == ".zip")
        original = asset.read_bytes()
        asset.write_bytes(b"substituted")
        self.verify(success=False)
        asset.unlink()
        self.verify(success=False)
        asset.symlink_to(self.root / "outside")
        self.verify(success=False)
        asset.unlink()
        asset.write_bytes(original)
        extra = self.directory / "unexpected"
        extra.write_bytes(b"extra")
        self.verify(success=False)
        self.assertFalse(self.log.exists())

    def test_old_unattested_release_and_wrong_commit_cannot_publish(self):
        self.fixture("extension")
        self.verify(success=False)
        self.create()
        self.run_script("verify", self.app, self.tag, "b" * 40, str(self.directory), success=False)
        self.env["GH_REPO"] = "attacker/hisar"
        self.verify(success=False)

    def test_attestation_failure_stops_publication(self):
        self.fixture("extension")
        self.create()
        self.env["REJECT_ATTESTATION"] = "1"
        self.verify(success=False)

    def test_published_update_rename_keeps_identical_verified_bytes(self):
        self.fixture("desktop")
        self.create()
        (self.directory / "update-unpublished.json").rename(self.directory / "update.json")
        self.verify()

    def test_server_requires_full_image_digest(self):
        self.fixture("server")
        for digest in ("", "latest", "sha256:1234"):
            self.env["IMAGE_DIGEST"] = digest
            self.create(success=False)

    def test_cli_cannot_omit_supported_platform_from_archive_manifest(self):
        self.fixture("cli")
        (self.directory / "hisar-1.2.3-windows-arm64.zip").unlink()
        remaining = sorted(path.name for path in self.directory.iterdir() if path.name != "SHA256SUMS")
        self.checksums("SHA256SUMS", remaining)
        self.create()
        self.verify(success=False)
        self.assertFalse(self.log.exists())

    def test_checksum_files_list_every_file_they_cover(self):
        for app, sum_file, dropped in (("cli", "SHA256SUMS", "hisar-1.2.3-windows-arm64.zip"),
                                       ("desktop", "SHA256SUMS-linux-arm64", "Hisar-1.2.3-linux-arm64.rpm"),
                                       ("desktop", "SHA256SUMS-flatpak-amd64", "Hisar-1.2.3-linux-amd64.flatpak")):
            with self.subTest(sum_file=sum_file):
                for path in self.directory.iterdir():
                    path.unlink()
                subprocess.run(["git", "tag", "-d", app + "-1.2.3"], cwd=self.repository, capture_output=True)
                self.fixture(app)
                checksums = self.directory / sum_file
                listed = [line for line in checksums.read_text().splitlines() if not line.endswith("  " + dropped)]
                checksums.write_text("".join(line + "\n" for line in listed))
                self.create()
                self.verify(success=False)
                self.assertFalse(self.log.exists())

    def test_publication_verifies_with_the_release_commit_policy_not_the_default_branch(self):
        scripts = self.repository / "scripts"
        scripts.mkdir()
        for name in ("release-evidence.py", "check-release-files.sh"):
            shutil.copy(ROOT / "scripts" / name, scripts / name)
        self.git("add", "scripts")
        self.git("commit", "-qm", "release source")
        self.sha = self.git("rev-parse", "HEAD")
        self.fixture("extension")
        self.create()
        self.env["PUBLISHED_RELEASE"] = str(self.directory)
        (scripts / "release-evidence.py").write_text("import sys\nsys.exit(0)\n")
        self.git("commit", "-qam", "default branch accepts any release")
        intact = self.publish_step("gh release download", self.root / "intact")
        self.assertEqual(intact.returncode, 0, intact.stdout)
        next(self.directory.glob("*.zip")).write_bytes(b"substituted")
        substituted = self.publish_step("gh release download", self.root / "substituted")
        self.assertNotEqual(substituted.returncode, 0, substituted.stdout)

    def test_image_verifier_rejects_mutable_docker_and_workflow_images(self):
        scripts = self.root / "scripts"
        scripts.mkdir()
        checker = scripts / "check-build-images.py"
        checker.write_bytes((ROOT / "scripts/check-build-images.py").read_bytes())
        deploy = self.root / "deploy"
        deploy.mkdir()
        workflows = self.root / ".github/workflows"
        workflows.mkdir(parents=True)
        dockerfile = deploy / "Dockerfile"
        workflow = workflows / "release.yml"
        pinned = "node:24@sha256:" + "a" * 64
        for docker_image, workflow_image, success in (
                ("node:24", pinned, False), (pinned, "node:24", False), (pinned, pinned, True)):
            dockerfile.write_text("FROM " + docker_image + " AS build\n")
            workflow.write_text("jobs:\n  build:\n    container: " + workflow_image + "\n")
            result = subprocess.run([sys.executable, str(checker)], text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode == 0, success, result.stdout)


if __name__ == "__main__":
    unittest.main()
