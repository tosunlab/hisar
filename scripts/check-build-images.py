#!/usr/bin/env python3
"""Check pinned build images and optionally inspect registry manifests without pulling layers."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import urllib.error
import urllib.parse
import urllib.request


ROOT = Path(__file__).resolve().parent.parent
ACCEPT = ", ".join(("application/vnd.oci.image.index.v1+json",
                    "application/vnd.docker.distribution.manifest.list.v2+json",
                    "application/vnd.oci.image.manifest.v1+json",
                    "application/vnd.docker.distribution.manifest.v2+json"))


def image_inputs():
    paths = [ROOT / "deploy/Dockerfile", ROOT / "docs/Dockerfile", *sorted((ROOT / ".github/workflows").glob("*.yml"))]
    images = set()
    for path in paths:
        for number, line in enumerate(path.read_text().splitlines(), 1):
            words = line.strip().split()
            if words and words[0] == "FROM":
                image = next(word for word in words[1:] if not word.startswith("--"))
            else:
                match = re.match(r"\s*(?:image:|container:|driver-opts: image=|image=)\s*(\S+)", line)
                if not match:
                    continue
                image = match[1]
            if not re.fullmatch(r"[^\s@]+:[^\s@]+@sha256:[0-9a-f]{64}", image):
                raise ValueError(f"{path.relative_to(ROOT)}:{number}: unpinned build image {image}")
            images.add(image)
    return sorted(images)


def manifest(image, resolve_tags):
    reference, pinned = image.split("@")
    parts = reference.split("/", 1)
    if len(parts) == 2 and ("." in parts[0] or ":" in parts[0]):
        registry, name = parts
    else:
        registry = "registry-1.docker.io"
        name = reference if "/" in reference else "library/" + reference
    name, tag = name.rsplit(":", 1)
    url = f"https://{registry}/v2/{name}/manifests/{tag if resolve_tags else pinned}"
    headers = {"Accept": ACCEPT}
    try:
        response = urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=30)
    except urllib.error.HTTPError as error:
        if error.code != 401:
            raise
        challenge = dict(re.findall(r'(\w+)="([^"]*)"', error.headers["WWW-Authenticate"]))
        realm = challenge.pop("realm")
        if urllib.parse.urlparse(realm).scheme != "https":
            raise ValueError("registry token realm must use HTTPS") from error
        with urllib.request.urlopen(realm + "?" + urllib.parse.urlencode(challenge), timeout=30) as token_response:
            token = json.load(token_response)
        headers["Authorization"] = "Bearer " + (token.get("token") or token["access_token"])
        response = urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=30)
    with response:
        raw = response.read()
    actual = "sha256:" + hashlib.sha256(raw).hexdigest()
    if not resolve_tags and actual != pinned:
        raise ValueError(f"registry manifest bytes do not match {image}")
    platforms = {f"{entry['platform'].get('os')}/{entry['platform'].get('architecture')}"
                 for entry in json.loads(raw).get("manifests", []) if "platform" in entry}
    if not {"linux/amd64", "linux/arm64"} <= platforms:
        raise ValueError(f"{image} must support linux/amd64 and linux/arm64, got {sorted(platforms)}")
    return reference + "@" + actual


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registry", action="store_true", help="verify pinned manifest bytes and both Linux architectures")
    parser.add_argument("--resolve-tags", action="store_true", help="print current tag digests for review; does not edit pins")
    args = parser.parse_args()
    for image in image_inputs():
        print(manifest(image, args.resolve_tags) if args.registry or args.resolve_tags else image)


if __name__ == "__main__":
    main()
