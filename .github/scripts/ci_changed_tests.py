#!/usr/bin/env python3
"""
Find pytest node IDs for test functions/classes added or modified between two commits.

Usage: python ci_changed_tests.py <base_sha> <head_sha> <test_file1> [...]
Writes 'has_changed' and 'test_ids' to $GITHUB_OUTPUT.
"""

import ast
import os
import re
import subprocess
import sys


def get_changed_line_numbers(base_sha: str, head_sha: str, filepath: str) -> set[int]:
    result = subprocess.run(
        ["git", "diff", "--unified=0", f"{base_sha}...{head_sha}", "--", filepath],
        capture_output=True,
        text=True,
    )
    changed: set[int] = set()
    for line in result.stdout.splitlines():
        m = re.match(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@", line)
        if m:
            start = int(m.group(1))
            count = int(m.group(2)) if m.group(2) is not None else 1
            if count == 0:
                changed.add(start)  # deletion anchor: marks where lines were removed
            else:
                changed.update(range(start, start + count))
    return changed


def find_test_node_ids(filepath: str, changed_lines: set[int]) -> list[str]:
    try:
        with open(filepath) as f:
            source = f.read()
        tree = ast.parse(source)
    except Exception:
        return [filepath]

    node_ids: list[str] = []

    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name.startswith("Test"):
            class_lines = set(range(node.lineno, node.end_lineno + 1))
            if not (class_lines & changed_lines):
                continue
            methods = [
                child
                for child in node.body
                if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef))
                and child.name.startswith("test_")
                and set(range(
                    child.decorator_list[0].lineno if child.decorator_list else child.lineno,
                    child.end_lineno + 1,
                )) & changed_lines
            ]
            if methods:
                node_ids.extend(
                    f"{filepath}::{node.name}::{m.name}" for m in methods
                )
            else:
                # Non-method lines changed (e.g. new class, fixture, docstring)
                node_ids.append(f"{filepath}::{node.name}")

        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            if node.name.startswith("test_"):
                if set(range(node.lineno, node.end_lineno + 1)) & changed_lines:
                    node_ids.append(f"{filepath}::{node.name}")

    return node_ids or [filepath]


def main() -> None:
    if len(sys.argv) < 4:
        print("Usage: ci_changed_tests.py <base_sha> <head_sha> <file1> [...]")
        sys.exit(1)

    base_sha, head_sha = sys.argv[1], sys.argv[2]
    test_files = sys.argv[3:]

    all_ids: list[str] = []
    for filepath in test_files:
        changed_lines = get_changed_line_numbers(base_sha, head_sha, filepath)
        if changed_lines:
            all_ids.extend(find_test_node_ids(filepath, changed_lines))

    github_output = os.environ.get("GITHUB_OUTPUT", "")

    if not all_ids:
        print("No changed tests found.")
        if github_output:
            with open(github_output, "a") as f:
                f.write("has_changed=false\ntest_ids=\n")
        return

    print("Changed test node IDs:")
    for nid in all_ids:
        print(f"  {nid}")

    if github_output:
        with open(github_output, "a") as f:
            f.write("has_changed=true\n")
            f.write(f"test_ids={' '.join(all_ids)}\n")


if __name__ == "__main__":
    main()
