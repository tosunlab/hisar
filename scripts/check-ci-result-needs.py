#!/usr/bin/env python3
"""Check that each pull request workflow's result job needs every other job in it.

The ruleset requires only the result jobs, so a job missing from their needs would stop gating merges without anyone
noticing.
"""

from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS = (".github/workflows/ci.yml", ".github/workflows/macos.yml")
RESULT_JOB = "result"


def check(relative_path):
    lines = (ROOT / relative_path).read_text().splitlines()
    start = lines.index("jobs:")
    jobs = {}
    current = None
    for line in lines[start + 1:]:
        match = re.fullmatch(r"  ([A-Za-z0-9_-]+):\s*", line)
        if match:
            current = match[1]
            jobs[current] = None
            continue
        match = re.fullmatch(r"    needs: \[(.*)\]\s*", line)
        if match and current == RESULT_JOB:
            jobs[current] = {need.strip() for need in match[1].split(",") if need.strip()}
    if RESULT_JOB not in jobs:
        return [f"{relative_path}: has no {RESULT_JOB} job"]
    needs = jobs.pop(RESULT_JOB)
    if needs is None:
        return [f"{relative_path}: the {RESULT_JOB} job must list its needs on one line as needs: [a, b, ...]"]
    problems = [f"{relative_path}: the {RESULT_JOB} job does not need {job}" for job in sorted(jobs.keys() - needs)]
    problems += [f"{relative_path}: the {RESULT_JOB} job needs {job}, which is not a job" for job in sorted(needs - jobs.keys())]
    return problems


def main():
    problems = [problem for path in WORKFLOWS for problem in check(path)]
    for problem in problems:
        print(problem, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
