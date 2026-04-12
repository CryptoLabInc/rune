#!/usr/bin/env python3
"""
Find pytest node IDs for tests whose definitions changed between two commits.

This script is designed for CI optimization:
- It detects changed lines via `git diff --unified=0`
- Parses Python test files with `ast`
- Returns pytest node IDs for changed test functions/methods
- Falls back conservatively to class-wide or file-wide selection when needed

Usage:
    python ci_changed_tests.py <base_sha> <head_sha> <test_file1> [...]

Outputs to $GITHUB_OUTPUT:
    has_changed=true|false
    test_ids<<EOF
    <nodeid1>
    <nodeid2>
    EOF
"""

from __future__ import annotations

import ast
import os
import re
import subprocess
import sys
from typing import Iterable


HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")


def warn(message: str) -> None:
    print(f"Warning: {message}", file=sys.stderr)


def unique_preserve_order(items: Iterable[str]) -> list[str]:
    return list(dict.fromkeys(items))


def get_changed_line_numbers(base_sha: str, head_sha: str, filepath: str) -> set[int]:
    """
    Return line numbers in the new version of `filepath` that changed
    between `base_sha` and `head_sha`.

    For pure deletions (`count == 0`), add the start line as a deletion anchor.
    """
    try:
        result = subprocess.run(
            ["git", "diff", "--unified=0", f"{base_sha}...{head_sha}", "--", filepath],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        warn(
            f"git diff failed for {filepath!r} "
            f"(base={base_sha}, head={head_sha}, returncode={e.returncode})"
        )
        if e.stderr:
            warn(e.stderr.strip())
        return set()

    changed: set[int] = set()

    for line in result.stdout.splitlines():
        m = HUNK_RE.match(line)
        if not m:
            continue

        start = int(m.group(1))
        count = int(m.group(2)) if m.group(2) is not None else 1

        if count == 0:
            # Deletion-only hunk: mark the anchor line in the new file.
            changed.add(start)
        else:
            changed.update(range(start, start + count))

    return changed


def get_docstring_lines(tree: ast.AST) -> set[int]:
    """
    Return line numbers occupied by docstring expressions.
    """
    docstring_lines: set[int] = set()

    for node in ast.walk(tree):
        if (
            isinstance(node, ast.Expr)
            and isinstance(node.value, ast.Constant)
            and isinstance(node.value.value, str)
            and hasattr(node, "lineno")
            and hasattr(node, "end_lineno")
            and node.end_lineno is not None
        ):
            docstring_lines.update(range(node.lineno, node.end_lineno + 1))

    return docstring_lines


def filter_meaningful_changed_lines(
    source_lines: list[str],
    changed_lines: set[int],
    docstring_lines: set[int],
) -> set[int]:
    """
    Keep only changed lines that are:
    - within file bounds
    - non-empty
    - not comments
    - not docstrings
    """
    meaningful = {
        ln
        for ln in changed_lines
        if 1 <= ln <= len(source_lines)
        and source_lines[ln - 1].strip()
        and not source_lines[ln - 1].strip().startswith("#")
    }

    return meaningful - docstring_lines


def line_range_for_node(node: ast.AST) -> set[int]:
    """
    Return the line range for a node, including decorators if present.
    """
    if not hasattr(node, "lineno") or not hasattr(node, "end_lineno") or node.end_lineno is None:
        return set()

    start = node.lineno

    decorator_list = getattr(node, "decorator_list", None)
    if decorator_list:
        first_decorator = decorator_list[0]
        if hasattr(first_decorator, "lineno"):
            start = first_decorator.lineno

    return set(range(start, node.end_lineno + 1))


def is_test_function(node: ast.AST) -> bool:
    return isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name.startswith("test_")


def is_test_class(node: ast.AST) -> bool:
    return isinstance(node, ast.ClassDef) and node.name.startswith("Test")


def find_test_node_ids(filepath: str, changed_lines: set[int]) -> list[str]:
    """
    Return pytest node IDs affected by changed lines in `filepath`.

    Behavior:
    - Directly changed test functions/methods -> return those node IDs
    - Class-level meaningful changes affecting a test class -> return all test methods in that class
    - Parse failure -> return [filepath] as conservative fallback
    - If nothing meaningful remains after filtering -> return []
    """
    try:
        with open(filepath, encoding="utf-8") as f:
            source = f.read()
        tree = ast.parse(source, filename=filepath)
    except Exception as e:
        warn(f"failed to parse {filepath!r}: {e}")
        return [filepath]

    source_lines = source.splitlines()
    docstring_lines = get_docstring_lines(tree)
    changed_lines = filter_meaningful_changed_lines(source_lines, changed_lines, docstring_lines)

    if not changed_lines:
        return []

    node_ids: list[str] = []

    for node in tree.body:
        if is_test_class(node):
            class_lines = set(range(node.lineno, node.end_lineno + 1))
            class_changed_lines = class_lines & changed_lines

            if not class_changed_lines:
                continue

            changed_methods = [
                child
                for child in node.body
                if is_test_function(child) and (line_range_for_node(child) & changed_lines)
            ]

            if changed_methods:
                node_ids.extend(
                    f"{filepath}::{node.name}::{method.name}"
                    for method in changed_methods
                )
                continue

            # No test method itself changed, but something meaningful inside the class did.
            # Be conservative: if any child node overlaps changed lines, run all test methods in class.
            child_code_lines: set[int] = set()
            all_test_methods: list[ast.AST] = []

            for child in node.body:
                if hasattr(child, "lineno") and hasattr(child, "end_lineno") and child.end_lineno is not None:
                    child_code_lines.update(range(child.lineno, child.end_lineno + 1))
                if is_test_function(child):
                    all_test_methods.append(child)

            if child_code_lines & class_changed_lines:
                node_ids.extend(
                    f"{filepath}::{node.name}::{method.name}"
                    for method in all_test_methods
                )

        elif is_test_function(node):
            if line_range_for_node(node) & changed_lines:
                node_ids.append(f"{filepath}::{node.name}")

    return unique_preserve_order(node_ids) or [filepath]


def write_github_output(has_changed: bool, test_ids: list[str]) -> None:
    github_output = os.environ.get("GITHUB_OUTPUT")
    if not github_output:
        return

    with open(github_output, "a", encoding="utf-8") as f:
        f.write(f"has_changed={'true' if has_changed else 'false'}\n")
        f.write("test_ids<<EOF\n")
        if test_ids:
            f.write("\n".join(test_ids))
            f.write("\n")
        f.write("EOF\n")


def main() -> None:
    if len(sys.argv) < 4:
        print("Usage: ci_changed_tests.py <base_sha> <head_sha> <file1> [...]", file=sys.stderr)
        sys.exit(1)

    base_sha, head_sha = sys.argv[1], sys.argv[2]
    test_files = sys.argv[3:]

    all_ids: list[str] = []

    for filepath in test_files:
        changed_lines = get_changed_line_numbers(base_sha, head_sha, filepath)
        if not changed_lines:
            continue

        all_ids.extend(find_test_node_ids(filepath, changed_lines))

    all_ids = unique_preserve_order(all_ids)

    if not all_ids:
        print("No changed tests found.")
        write_github_output(False, [])
        return

    print("Changed test node IDs:")
    for node_id in all_ids:
        print(f"  {node_id}")

    write_github_output(True, all_ids)


if __name__ == "__main__":
    main()
