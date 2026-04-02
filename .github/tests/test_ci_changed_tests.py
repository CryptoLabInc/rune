"""Tests for .github/scripts/ci_changed_tests.py"""

import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'scripts'))

from ci_changed_tests import find_test_node_ids


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
