#!/usr/bin/env python3
"""Bind release tests, source, files, and image digests before publication."""

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys


EVIDENCE = "release-evidence.json"
SERVER_IMAGE = "ghcr.io/tosunlab/hisar-server"
APPS = {"cli", "server", "desktop", "extension"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def check_source(sha, tag=None):
    require(re.fullmatch(r"[0-9a-f]{40}", sha), "source must be a full commit SHA")
    require(git("rev-parse", "HEAD") == sha, "checkout differs from tested source SHA")
    if tag:
        require(git("rev-parse", f"refs/tags/{tag}^{{commit}}") == sha,
                "release tag differs from tested source SHA")


def files(directory):
    result = {}
    for path in sorted(directory.iterdir()):
        require(path.is_file() and not path.is_symlink(), f"not a regular release file: {path}")
        if path.name != EVIDENCE:
            # Publishing renames this asset without changing its bytes.
            name = "update-unpublished.json" if path.name == "update.json" else path.name
            require(name not in result, f"duplicate release file: {name}")
            digest = hashlib.sha256()
            with path.open("rb") as stream:
                for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                    digest.update(chunk)
            result[name] = digest.hexdigest()
    return result


def validate(record, app, tag, sha, repository, directory):
    require(record.get("schema") == 1, "unsupported release evidence schema")
    for key, expected in (("app", app), ("tag", tag), ("commit", sha), ("repository", repository)):
        require(record.get(key) == expected, f"release evidence {key} differs")
    require(record.get("tests") == {"ci": "success", "macos": "success" if app == "desktop" else "not-required"},
            "required exact-commit release tests did not succeed")
    require(record.get("files") == files(directory), "release file set or bytes differ from test evidence")
    require(re.fullmatch(r"[1-9][0-9]*", record.get("runID", "")), "missing release workflow run")
    image = record.get("image")
    if app == "server":
        require(isinstance(image, dict) and image.get("name") == SERVER_IMAGE
                and re.fullmatch(r"sha256:[0-9a-f]{64}", image.get("digest", "")), "missing immutable server image digest")
    else:
        require(image is None, "unexpected image evidence")


def attest(subject, repository, workflow, sha):
    subprocess.run(["gh", "attestation", "verify", subject, "--repo", repository,
                    "--signer-workflow", f"{repository}/.github/workflows/{workflow}",
                    "--signer-digest", sha, "--source-digest", sha,
                    "--deny-self-hosted-runners"], check=True)


def main():
    command, *args = sys.argv[1:]
    if command == "check-source":
        check_source(*args)
        return
    app, tag, sha, location = args
    require(app in APPS and re.fullmatch(rf"{app}-[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9][A-Za-z0-9.-]*)?", tag),
            "invalid release app or tag")
    require(re.fullmatch(r"[0-9a-f]{40}", sha), "source must be a full commit SHA")
    repository = os.environ.get("GH_REPO") or os.environ["GITHUB_REPOSITORY"]
    directory = Path(location)
    evidence = directory / EVIDENCE
    if command == "create":
        check_source(sha, tag)
        require(not evidence.exists(), "refusing to replace release evidence")
        record = {"schema": 1, "app": app, "tag": tag, "commit": sha, "repository": repository,
                  "runID": os.environ["GITHUB_RUN_ID"],
                  "tests": {"ci": os.environ["CI_RESULT"],
                            "macos": os.environ.get("MACOS_RESULT") if app == "desktop" else "not-required"},
                  "files": files(directory),
                  "image": {"name": SERVER_IMAGE, "digest": os.environ.get("IMAGE_DIGEST", "")} if app == "server" else None}
        validate(record, app, tag, sha, repository, directory)
        evidence.write_text(json.dumps(record, indent=2, sort_keys=True) + "\n")
    elif command == "verify":
        require(evidence.is_file() and not evidence.is_symlink(), "missing release evidence")
        record = json.loads(evidence.read_text())
        validate(record, app, tag, sha, repository, directory)
        subprocess.run(["bash", str(Path(__file__).with_name("check-release-files.sh")),
                        app, tag.removeprefix(app + "-"), str(directory)], check=True)
        # Verify the evidence itself as well as every byte it identifies.
        for path in sorted(directory.iterdir()):
            attest(str(path), repository, "release.yml", sha)
        if record["image"]:
            attest(f"oci://{SERVER_IMAGE}@{record['image']['digest']}", repository, "release-server.yml", sha)
    else:
        raise ValueError(f"unknown command: {command}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(f"release evidence: {error}")
