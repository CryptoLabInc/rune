"""Tests for .github/scripts/ci_changed_tests.py"""

import re
import sys
import os
from unittest.mock import MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', '.github', 'scripts'))

from ci_changed_tests import find_test_node_ids, get_changed_line_numbers

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
        f.write_text("class TestFoo:\n    def test_bar(self):\n        assert True\n")
        ids = find_test_node_ids(str(f), changed_lines={99})
        assert ids == [str(f)]

    def test_ignores_class_not_starting_with_Test(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text("class Foo:\n    def test_bar(self):\n        assert True\n")
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [str(f)]

    def test_ignores_method_not_starting_with_test_(self, tmp_path):
        f = tmp_path / "test_sample.py"
        f.write_text("class TestFoo:\n    def helper(self):\n        pass\n")
        ids = find_test_node_ids(str(f), changed_lines={2})
        assert ids == [f"{f}::TestFoo"]

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
