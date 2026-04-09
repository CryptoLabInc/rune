"""Tests for .github/scripts/ci_changed_tests.py"""

import re
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', '.github', 'scripts'))

from ci_changed_tests import find_test_node_ids

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
