#!/usr/bin/env python3
"""Prints each failed test of an xcresult bundle as <bundle>/<class>/<test>, a tab and its first failure message.

usage: failed-ui-tests.py <result bundle>

scripts/test-ios-ui.sh runs these tests a second time. It prints nothing when no test failed, as when the run failed before
or outside the tests.
"""
import json
import subprocess
import sys


def find_failed(node, bundle=None):
    if node.get("nodeType") == "UI test bundle":
        bundle = node.get("name")
    if node.get("nodeType") == "Test Case":
        if node.get("result") == "Failed":
            messages = [child.get("name", "") for child in node.get("children", []) if child.get("nodeType") == "Failure Message"]
            yield f"{bundle}/{node['nodeIdentifier'].removesuffix('()')}", " ".join(messages[0].split())[:300] if messages else ""
        return
    for child in node.get("children", []):
        yield from find_failed(child, bundle)


results = subprocess.run(["xcrun", "xcresulttool", "get", "test-results", "tests", "--path", sys.argv[1]],
                         check=True, capture_output=True, text=True)
for node in json.loads(results.stdout)["testNodes"]:
    for test, message in find_failed(node):
        print(f"{test}\t{message}")
