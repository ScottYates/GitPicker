#!/usr/bin/env python3
"""Parity check: the Go picker must draw the same list as the Python one.

Builds a fixture repository with a deliberately awkward set of branches, then
compares the Go binary and git_picker.py on:

  * the full list, in order, as drawn (name, [remote] tag and age columns)
  * the fuzzy filtered order for a set of queries

The clock is pinned on both sides so the age column cannot drift between the
two runs.

Run it from the project folder:  python parity_check.py
"""
from __future__ import annotations

import asyncio
import os
import subprocess
import sys
import tempfile
from datetime import datetime, timezone

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import git_picker as gp  # noqa: E402
from pfzy import fuzzy_match  # noqa: E402
from pfzy.score import fzy_scorer  # noqa: E402

GO_EXE = os.path.join(HERE, "git-picker.exe")

# A fixed clock, so the age column is the same on both sides.
FIXED_NOW = 1795000000

PASS = []
FAIL = []

# Branches that exist only on the remote, so they must show the [remote] tag.
REMOTE_ONLY = ["feature/remote-only", "hotfix/patch-99", "release/9.9"]

# Local branches, with the age each one should show.
LOCAL_BRANCHES = {
    "master": 400,
    "main": 900,
    "develop": 1200,
    "dev": 3600,
    "uat": 40000,
    "test": 200000,
    "prod": 900000,
    "production": 5000000,
    "PROD": 120,
    "prods": 300,
    "uat-2": 400,
    "preprod": 500,
    "feature/TICKET-123-add-stuff": 700,
    "feature/other_thing": 800,
    "hotfix/urgent-fix": 60,
    "release/1.2.3": 20000,
    "fix/nested/deep/name": 1500,
}

# Queries that exercise the fuzzy matcher: exact, partial, spaced, and ones
# that match nothing.
QUERIES = [
    "",
    "ma",
    "master",
    "feature",
    "TICKET",
    "feature ticket",
    "hot fix",
    "uat",
    "uat2",
    "remote",
    "xyzzy",
    "d e v",
    "1.2.3",
]


def check(name: str, cond: bool, detail: str = "") -> None:
    (PASS if cond else FAIL).append(name)
    print(("[PASS] " if cond else "[FAIL] ") + name + ((" | " + detail) if detail else ""))


def run(*args: str, cwd: str | None = None, env: dict | None = None) -> subprocess.CompletedProcess:
    merged = dict(os.environ)
    merged.update(env or {})
    merged["GIT_TERMINAL_PROMPT"] = "0"
    return subprocess.run(args, cwd=cwd, capture_output=True, text=True, env=merged)


def make_fixture() -> str:
    """Build a repo with awkward branch names and a remote-only set."""
    root = tempfile.mkdtemp(prefix="gp-parity-")
    bare = os.path.join(root, "remote.git")
    work = os.path.join(root, "work")

    run("git", "init", "-q", "--bare", bare)
    run("git", "init", "-q", work)
    run("git", "config", "user.email", "p@p.p", cwd=work)
    run("git", "config", "user.name", "p", cwd=work)
    run("git", "remote", "add", "origin", bare, cwd=work)

    # A dated commit on the default branch, then one branch per name with its
    # own age.
    def commit_at(age_seconds: int, message: str) -> None:
        stamp = datetime.fromtimestamp(FIXED_NOW - age_seconds, tz=timezone.utc)
        iso = stamp.isoformat()
        run("git", "commit", "-q", "--allow-empty", "-m", message, cwd=work,
            env={"GIT_AUTHOR_DATE": iso, "GIT_COMMITTER_DATE": iso})

    commit_at(400, "root")
    run("git", "branch", "-M", "master", cwd=work)

    for name, age in LOCAL_BRANCHES.items():
        if name == "master":
            continue
        run("git", "checkout", "-q", "-b", name, cwd=work)
        commit_at(age, name)
        run("git", "checkout", "-q", "master", cwd=work)

    # The remote-only branches: made locally, pushed, then removed locally.
    for name in REMOTE_ONLY:
        run("git", "checkout", "-q", "-b", name, cwd=work)
        commit_at(200, name)
        run("git", "push", "-q", "origin", name, cwd=work)
        run("git", "checkout", "-q", "master", cwd=work)
        run("git", "branch", "-q", "-D", name, cwd=work)

    run("git", "push", "-q", "--all", "origin", cwd=work)
    # Give origin a HEAD ref, which the picker must skip.
    run("git", "remote", "set-head", "origin", "-a", cwd=work)
    return work


def python_labels(work: str) -> list[str]:
    branches = gp.sort_branches(gp.list_branches(work))
    width = gp.label_name_width(branches)
    current = gp.current_branch(work)
    return [gp.display_label(b, current, FIXED_NOW, width) for b in branches]


def python_filtered(work: str, query: str) -> list[str]:
    branches = gp.sort_branches(gp.list_branches(work))
    width = gp.label_name_width(branches)
    current = gp.current_branch(work)
    labels = [gp.display_label(b, current, FIXED_NOW, width) for b in branches]
    if not query:
        return labels
    choices = [{"name": label} for label in labels]
    matched = asyncio.run(fuzzy_match(query, choices, key="name", scorer=fzy_scorer))
    return [item["name"] for item in matched]


def go_dump(work: str, query: str) -> list[str]:
    args = [GO_EXE, "--dump-labels", f"--now={FIXED_NOW}"]
    if query:
        args.append(f"--query={query}")
    proc = run(*args, cwd=work)
    if proc.returncode != 0:
        raise RuntimeError(f"go dump failed: {proc.stderr.strip()}")
    return proc.stdout.splitlines()


def main() -> int:
    if not os.path.exists(GO_EXE):
        print(f"Error: {GO_EXE} not found. Build it first: go build -o git-picker.exe .")
        return 1

    work = make_fixture()
    print(f"fixture repo: {work}")
    print(f"branches: {len(LOCAL_BRANCHES)} local, {len(REMOTE_ONLY)} remote only")
    print()

    expected = python_labels(work)
    actual = go_dump(work, "")
    check(
        "the full list matches the Python picker row for row",
        expected == actual,
        first_difference(expected, actual),
    )

    for query in QUERIES:
        expected = python_filtered(work, query)
        actual = go_dump(work, query)
        check(
            f"fuzzy order matches for query {query!r}",
            expected == actual,
            first_difference(expected, actual),
        )

    print()
    print(f"{len(PASS)} passed, {len(FAIL)} failed")
    return 1 if FAIL else 0


def first_difference(expected: list[str], actual: list[str]) -> str:
    """Describe the first line that differs, for a useful failure message."""
    for i in range(max(len(expected), len(actual))):
        want = expected[i] if i < len(expected) else "<missing>"
        got = actual[i] if i < len(actual) else "<missing>"
        if want != got:
            return f"line {i + 1}: python={want!r} go={got!r}"
    return ""


if __name__ == "__main__":
    sys.exit(main())
