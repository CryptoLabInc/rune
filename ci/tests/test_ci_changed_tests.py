"""Tests for .github/scripts/ci_changed_tests.py"""

import re
import subprocess
import sys
import os
import pytest
from unittest.mock import MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', '.github', 'scripts'))

from ci_changed_tests import (
    find_test_node_ids,
    get_changed_line_numbers,
    main,
    write_github_output,
)

# Grep pattern used in pr-tests.yml to filter changed test files
_FILE_PATTERN = re.compile(r'(^|/)tests/test_[^/]+\.py$')


class TestFileDetectionPattern:
    def test_matches_tests_subdirectory(self):
        assert _FILE_PATTERN.search('agents/tests/test_foo.py')

    def test_matches_root_tests_directory(self):
        assert _FILE_PATTERN.search('tests/test_foo.py')

    def test_rejects_file_not_in_tests_dir(self):
        assert not _FILE_PATTERN.search('agents/test_foo.py')

    def test_rejects_filename_not_starting_with_test_(self):
        assert not _FILE_PATTERN.search('agents/tests/foo.py')

    def test_rejects_nested_file_inside_tests_subdir(self):
        assert not _FILE_PATTERN.search('agents/tests/sub/test_foo.py')


class TestFindTestNodeIds:
    def test_detects_method_in_changed_lines(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def test_bar(self):\n"
            "        assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [f"{f}::TestFoo::test_bar"]

    def test_returns_filepath_when_no_tests_match(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def test_bar(self):\n"
            "        assert True\n"
            "\n"
            "SOME_CONSTANT = 42\n"  # line 5 — real code, but not inside a test
        )
        ids = find_test_node_ids(str(f), changed_lines={5})
        assert ids == [str(f)]

    def test_ignores_class_not_starting_with_Test(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text("class Foo:\n    def test_bar(self):\n        assert True\n")
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [str(f)]

    def test_ignores_method_not_starting_with_test_(self, tmp_path):
        # helper is real code, but there are no test_ methods to return → fallback to filepath
        f = tmp_path / "test_sample.py"
        f.write_text("class TestFoo:\n    def helper(self):\n        pass\n")
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [str(f)]

    def test_skips_multiline_hash_comment_change(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    # comment line 1\n"   # line 2
            "    # comment line 2\n"   # line 3
            "    # comment line 3\n"   # line 4
            "    def test_bar(self):\n"
            "        assert True\n"
        )
        # lines 2-4 are # comments — no test should be detected
        ids = find_test_node_ids(str(f), changed_lines={2, 3, 4})
        assert ids == []

    def test_skips_multiline_docstring_change(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            '    """\n'                 # line 2
            "    Class docstring.\n"    # line 3
            '    """\n'                 # line 4
            "    def test_bar(self):\n"
            "        assert True\n"
        )
        # lines 2-4 are a docstring — no test should be detected
        ids = find_test_node_ids(str(f), changed_lines={2, 3, 4})
        assert ids == []

    def test_skips_comment_inside_method(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def test_bar(self):\n"
            "        # internal comment\n"  # line 3
            "        assert True\n"
        )
        # line 3 is a # comment inside test_bar — should not trigger the test
        ids = find_test_node_ids(str(f), changed_lines={3})
        assert ids == []

    def test_class_variable_change_returns_all_test_methods(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    shared = 'value'\n"   # line 2
            "    def test_bar(self):\n"
            "        assert True\n"
            "    def test_baz(self):\n"
            "        assert True\n"
        )
        # class variable changed → all test methods in the class should be returned
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [f"{f}::TestFoo::test_bar", f"{f}::TestFoo::test_baz"]

    def test_setup_method_change_returns_all_test_methods(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def setUp(self):\n"   # line 2
            "        self.x = 1\n"    # line 3
            "    def test_bar(self):\n"
            "        assert self.x\n"
            "    def test_baz(self):\n"
            "        assert self.x\n"
        )
        # setUp changed → all test methods in the class should be returned
        ids = find_test_node_ids(str(f), changed_lines={2, 3})
        assert ids == [f"{f}::TestFoo::test_bar", f"{f}::TestFoo::test_baz"]

    def test_detects_top_level_test_function(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text("def test_foo():\n    assert True\n")
        ids = find_test_node_ids(str(f), changed_lines={1})
        assert ids == [f"{f}::test_foo"]

    def test_ignores_top_level_function_not_starting_with_test_(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text("def helper():\n    pass\n")
        ids = find_test_node_ids(str(f), changed_lines={1})
        assert ids == [str(f)]

    def test_detects_method_when_decorator_line_changed(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "import pytest\n"
            "class TestFoo:\n"
            "    @pytest.mark.slow\n"
            "    def test_bar(self):\n"
            "        assert True\n"
        )
        # line 3 is the decorator — changing it alone should still detect test_bar
        ids = find_test_node_ids(str(f), changed_lines={3})
        assert ids == [f"{f}::TestFoo::test_bar"]

    def test_detects_top_level_function_when_decorator_line_changed(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "import pytest\n"
            "@pytest.mark.slow\n"   # line 2
            "def test_foo():\n"
            "    assert True\n"
        )
        # line 2 is the decorator — changing it alone should still detect test_foo
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [f"{f}::test_foo"]

    def test_detects_method_when_multiple_decorators_and_first_decorator_line_changed(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "import pytest\n"
            "class TestFoo:\n"
            "    @pytest.mark.slow\n"
            "    @pytest.mark.parametrize('x', [1])\n"
            "    def test_bar(self, x):\n"
            "        assert x\n"
        )
        # line 3 is the first decorator — range should start from there
        ids = find_test_node_ids(str(f), changed_lines={3})
        assert ids == [f"{f}::TestFoo::test_bar"]

    def test_private_helper_method_change_returns_all_test_methods(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def _helper(self):\n"    # line 2
            "        return 42\n"         # line 3
            "    def test_bar(self):\n"
            "        assert self._helper() == 42\n"
            "    def test_baz(self):\n"
            "        assert self._helper() == 42\n"
        )
        # _helper changed but no test_ method changed → all test methods returned
        ids = find_test_node_ids(str(f), changed_lines={2, 3})
        assert ids == [f"{f}::TestFoo::test_bar", f"{f}::TestFoo::test_baz"]

    def test_only_changed_class_is_returned_when_multiple_classes_exist(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def test_a(self):\n"
            "        assert True\n"
            "class TestBar:\n"
            "    def test_b(self):\n"    # line 5
            "        assert True\n"
        )
        # only TestBar::test_b changed
        ids = find_test_node_ids(str(f), changed_lines={5})
        assert ids == [f"{f}::TestBar::test_b"]

    def test_parse_failure_returns_filepath(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text("def test_foo(\n    broken syntax\n")
        ids = find_test_node_ids(str(f), changed_lines={1})
        assert ids == [str(f)]

    def test_detects_async_test_function(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "import asyncio\n"
            "async def test_foo():\n"   # line 2
            "    assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [f"{f}::test_foo"]

    def test_detects_async_method_in_class(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    async def test_bar(self):\n"   # line 2
            "        assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [f"{f}::TestFoo::test_bar"]

    def test_class_with_no_test_methods_falls_back_to_filepath(self, tmp_path):
        # TestFoo has only a helper — changing it produces no test_ methods → filepath fallback
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    shared = 'value'\n"   # line 2
        )
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [str(f)]

    def test_returns_empty_list_when_changed_lines_is_empty(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "class TestFoo:\n"
            "    def test_bar(self):\n"
            "        assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines=set())
        assert ids == []

    def test_ignores_out_of_range_changed_line(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "def test_foo():\n"
            "    assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={999})
        assert ids == []

    def test_skips_blank_line_change(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "def test_foo():\n"
            "\n"  # line 2
            "    assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == []

    def test_skips_module_level_docstring_change(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            '"""module docstring"""\n'  # line 1
            "def test_foo():\n"
            "    assert True\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={1})
        assert ids == []

    def test_detects_method_when_second_decorator_line_changed(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text(
            "import pytest\n"
            "class TestFoo:\n"
            "    @pytest.mark.slow\n"                  # line 3
            "    @pytest.mark.parametrize('x', [1])\n"  # line 4
            "    def test_bar(self, x):\n"
            "        assert x\n"
        )
        ids = find_test_node_ids(str(f), changed_lines={4})
        assert ids == [f"{f}::TestFoo::test_bar"]


class TestGetChangedLineNumbers:
    def _run_with_diff(self, diff_output: str) -> set:
        mock_result = MagicMock()
        mock_result.stdout = diff_output
        with patch("ci_changed_tests.subprocess.run", return_value=mock_result):
            return get_changed_line_numbers("base_sha", "head_sha", "test_sample.py")

    def test_deletion_only_hunk_adds_anchor(self):
        # @@ -2 +1,0 @@ means 1 line deleted; new_count=0 → deletion anchor at new line 1
        diff = "@@ -2 +1,0 @@\n-    # setup comment\n"
        changed = self._run_with_diff(diff)
        assert 1 in changed

    def test_deletion_only_hunk_with_multiple_lines_adds_anchor(self):
        # @@ -5,3 +4,0 @@ means 3 lines deleted; anchor at new line 4
        diff = "@@ -5,3 +4,0 @@\n-line5\n-line6\n-line7\n"
        changed = self._run_with_diff(diff)
        assert 4 in changed

    def test_normal_addition_hunk_adds_range(self):
        # @@ -1,0 +1,3 @@ means 3 lines added starting at new line 1
        diff = "@@ -1,0 +1,3 @@\n+line1\n+line2\n+line3\n"
        changed = self._run_with_diff(diff)
        assert changed == {1, 2, 3}

    def test_subprocess_failure_returns_empty_set(self):
        with patch(
            "ci_changed_tests.subprocess.run",
            side_effect=subprocess.CalledProcessError(128, "git"),
        ):
            changed = get_changed_line_numbers("base_sha", "head_sha", "test_sample.py")
        assert changed == set()

    def test_subprocess_failure_with_stderr_does_not_raise(self):
        err = subprocess.CalledProcessError(128, "git")
        err.stderr = "fatal: bad object base_sha"
        with patch("ci_changed_tests.subprocess.run", side_effect=err):
            changed = get_changed_line_numbers("base_sha", "head_sha", "test_sample.py")
        assert changed == set()

    def _run_with_diff(self, diff_output: str) -> set[int]:
        mock_result = MagicMock()
        mock_result.stdout = diff_output
        with patch("ci_changed_tests.subprocess.run", return_value=mock_result):
            return get_changed_line_numbers("base_sha", "head_sha", "test_sample.py")

    def test_multiple_hunks_are_merged(self):
        diff = (
            "@@ -1,0 +1,2 @@\n"
            "+line1\n"
            "+line2\n"
            "@@ -10,0 +12,2 @@\n"
            "+line12\n"
            "+line13\n"
        )
        changed = self._run_with_diff(diff)
        assert changed == {1, 2, 12, 13}

    def test_modification_hunk_adds_new_file_range(self):
        diff = (
            "@@ -5,2 +5,2 @@\n"
            "-old1\n"
            "-old2\n"
            "+new1\n"
            "+new2\n"
        )
        changed = self._run_with_diff(diff)
        assert changed == {5, 6}

    def test_non_hunk_lines_are_ignored(self):
        diff = (
            "diff --git a/test_sample.py b/test_sample.py\n"
            "index 123..456 100644\n"
            "--- a/test_sample.py\n"
            "+++ b/test_sample.py\n"
            "@@ -1,0 +1,1 @@\n"
            "+line1\n"
        )
        changed = self._run_with_diff(diff)
        assert changed == {1}


class TestWriteGithubOutput:
    def test_does_nothing_when_github_output_not_set(self, monkeypatch):
        monkeypatch.delenv("GITHUB_OUTPUT", raising=False)
        write_github_output(True, ["a::test_one"])

    def test_writes_multiline_output(self, tmp_path, monkeypatch):
        output_file = tmp_path / "github_output.txt"
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))

        write_github_output(True, ["a::test_one", "b::TestFoo::test_bar"])

        content = output_file.read_text(encoding="utf-8")
        assert "has_changed=true\n" in content
        assert "test_ids<<EOF\n" in content
        assert "a::test_one\n" in content
        assert "b::TestFoo::test_bar\n" in content
        assert content.endswith("EOF\n")

    def test_writes_false_with_empty_test_ids(self, tmp_path, monkeypatch):
        output_file = tmp_path / "github_output.txt"
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))

        write_github_output(False, [])

        content = output_file.read_text(encoding="utf-8")
        assert content == "has_changed=false\ntest_ids<<EOF\nEOF\n"


class TestMain:
    def test_exits_with_usage_when_not_enough_args(self, monkeypatch, capsys):
        monkeypatch.setattr(sys, "argv", ["ci_changed_tests.py"])

        with pytest.raises(SystemExit) as exc:
            main()

        assert exc.value.code == 1
        captured = capsys.readouterr()
        assert "Usage: ci_changed_tests.py" in captured.err

    def test_main_writes_false_when_no_changed_tests(self, monkeypatch, tmp_path, capsys):
        output_file = tmp_path / "github_output.txt"
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))
        monkeypatch.setattr(
            sys,
            "argv",
            ["ci_changed_tests.py", "base_sha", "head_sha", "tests/test_sample.py"],
        )

        with patch("ci_changed_tests.get_changed_line_numbers", return_value=set()):
            main()

        captured = capsys.readouterr()
        assert "No changed tests found." in captured.out

        content = output_file.read_text(encoding="utf-8")
        assert content == "has_changed=false\ntest_ids<<EOF\nEOF\n"

    def test_main_prints_and_writes_changed_ids(self, monkeypatch, tmp_path, capsys):
        output_file = tmp_path / "github_output.txt"
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))
        monkeypatch.setattr(
            sys,
            "argv",
            [
                "ci_changed_tests.py",
                "base_sha",
                "head_sha",
                "tests/test_one.py",
                "tests/test_two.py",
            ],
        )

        def fake_get_changed_line_numbers(base_sha, head_sha, filepath):
            if filepath == "tests/test_one.py":
                return {1}
            if filepath == "tests/test_two.py":
                return {2}
            return set()

        def fake_find_test_node_ids(filepath, changed_lines):
            if filepath == "tests/test_one.py":
                return ["tests/test_one.py::test_alpha"]
            if filepath == "tests/test_two.py":
                return [
                    "tests/test_two.py::TestFoo::test_beta",
                    "tests/test_two.py::TestFoo::test_beta",  # duplicate on purpose
                ]
            return []

        with patch("ci_changed_tests.get_changed_line_numbers", side_effect=fake_get_changed_line_numbers):
            with patch("ci_changed_tests.find_test_node_ids", side_effect=fake_find_test_node_ids):
                main()

        captured = capsys.readouterr()
        assert "Changed test node IDs:" in captured.out
        assert "tests/test_one.py::test_alpha" in captured.out
        assert "tests/test_two.py::TestFoo::test_beta" in captured.out

        content = output_file.read_text(encoding="utf-8")
        assert "has_changed=true\n" in content
        assert "tests/test_one.py::test_alpha\n" in content
        assert content.count("tests/test_two.py::TestFoo::test_beta\n") == 1
        assert content.endswith("EOF\n")