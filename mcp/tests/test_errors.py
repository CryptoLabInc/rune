# tests/test_errors.py
import pytest
from server.errors import (
    RuneError,
    VaultConnectionError,
    VaultDecryptionError,
    EnvectorConnectionError,
    EnvectorInsertError,
    PipelineNotReadyError,
    InvalidInputError,
    make_error,
)

class TestRuneErrorHierarchy:
    def test_base_defaults(self):
        e = RuneError("boom")
        assert str(e) == "boom"
        assert e.code == "INTERNAL_ERROR"
        assert e.retryable is False

    def test_vault_connection_error(self):
        e = VaultConnectionError("unreachable")
        assert isinstance(e, RuneError)
        assert e.code == "VAULT_CONNECTION_ERROR"
        assert e.retryable is True

    def test_vault_decryption_error(self):
        e = VaultDecryptionError("bad token")
        assert isinstance(e, RuneError)
        assert e.code == "VAULT_DECRYPTION_ERROR"
        assert e.retryable is False

    def test_envector_connection_error(self):
        e = EnvectorConnectionError("timeout")
        assert isinstance(e, RuneError)
        assert e.code == "ENVECTOR_CONNECTION_ERROR"
        assert e.retryable is True

    def test_envector_insert_error(self):
        e = EnvectorInsertError("index not found")
        assert isinstance(e, RuneError)
        assert e.code == "ENVECTOR_INSERT_ERROR"
        assert e.retryable is True

    def test_pipeline_not_ready_error(self):
        e = PipelineNotReadyError("scribe not initialized")
        assert isinstance(e, RuneError)
        assert e.code == "PIPELINE_NOT_READY"
        assert e.retryable is False

    def test_invalid_input_error(self):
        e = InvalidInputError("topk too large")
        assert isinstance(e, RuneError)
        assert e.code == "INVALID_INPUT"
        assert e.retryable is False

    def test_override_code_and_retryable(self):
        e = RuneError("custom", code="CUSTOM_CODE", retryable=True)
        assert e.code == "CUSTOM_CODE"
        assert e.retryable is True


class TestMakeError:
    def test_rune_error_produces_structured_dict(self):
        result = make_error(VaultConnectionError("cannot reach vault"))
        assert result["ok"] is False
        assert result["error"]["code"] == "VAULT_CONNECTION_ERROR"
        assert result["error"]["message"] == "cannot reach vault"
        assert result["error"]["retryable"] is True

    def test_generic_exception_falls_back(self):
        result = make_error(RuntimeError("unexpected"))
        assert result["ok"] is False
        assert result["error"]["code"] == "INTERNAL_ERROR"
        assert result["error"]["message"] == "unexpected"
        assert result["error"]["retryable"] is False

    def test_invalid_input_error(self):
        result = make_error(InvalidInputError("topk must be 10 or less"))
        assert result["ok"] is False
        assert result["error"]["code"] == "INVALID_INPUT"
        assert result["error"]["retryable"] is False

    def test_pipeline_not_ready_error(self):
        result = make_error(PipelineNotReadyError("Retriever not initialized"))
        assert result["ok"] is False
        assert result["error"]["code"] == "PIPELINE_NOT_READY"
        assert result["error"]["message"] == "Retriever not initialized"
        assert result["error"]["retryable"] is False
