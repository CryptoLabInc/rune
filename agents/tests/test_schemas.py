"""Tests for DecisionRecord schema."""
import pytest


def test_reusable_insight_field():
    """DecisionRecord should have reusable_insight field with schema 2.1."""
    from agents.common.schemas import DecisionRecord, DecisionDetail, Domain

    record = DecisionRecord(
        id="dec_2026-03-28_architecture_test",
        title="Test",
        decision=DecisionDetail(what="Test decision"),
        reusable_insight="We chose PostgreSQL over MongoDB because ACID compliance is critical.",
    )
    assert record.reusable_insight == "We chose PostgreSQL over MongoDB because ACID compliance is critical."
    assert record.schema_version == "2.1"


def test_reusable_insight_defaults_empty():
    """reusable_insight should default to empty string for backward compat."""
    from agents.common.schemas import DecisionRecord, DecisionDetail

    record = DecisionRecord(
        id="dec_2026-03-28_architecture_test",
        title="Test",
        decision=DecisionDetail(what="Test decision"),
    )
    assert record.reusable_insight == ""
