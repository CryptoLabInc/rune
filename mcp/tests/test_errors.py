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
        assert e.recovery_hint == ""

    def test_vault_connection_error(self):
        e = VaultConnectionError("unreachable")
        assert isinstance(e, RuneError)
        assert e.code == "VAULT_CONNECTION_ERROR"
        assert e.retryable is True
        assert "Vault is unreachable" in e.recovery_hint
        assert "/rune:status" in e.recovery_hint

    def test_vault_decryption_error(self):
        e = VaultDecryptionError("bad token")
        assert isinstance(e, RuneError)
        assert e.code == "VAULT_DECRYPTION_ERROR"
        assert e.retryable is False
        assert "token" in e.recovery_hint.lower()
        assert "/rune:configure" in e.recovery_hint

    def test_envector_connection_error(self):
        e = EnvectorConnectionError("timeout")
        assert isinstance(e, RuneError)
        assert e.code == "ENVECTOR_CONNECTION_ERROR"
        assert e.retryable is True
        assert "enVector" in e.recovery_hint

    def test_envector_insert_error(self):
        e = EnvectorInsertError("index not found")
        assert isinstance(e, RuneError)
        assert e.code == "ENVECTOR_INSERT_ERROR"
        assert e.retryable is True
        assert "retry" in e.recovery_hint.lower()

    def test_pipeline_not_ready_error(self):
        e = PipelineNotReadyError("scribe not initialized")
        assert isinstance(e, RuneError)
        assert e.code == "PIPELINE_NOT_READY"
        assert e.retryable is False
        assert "/rune:activate" in e.recovery_hint

    def test_invalid_input_error(self):
        e = InvalidInputError("topk too large")
        assert isinstance(e, RuneError)
        assert e.code == "INVALID_INPUT"
        assert e.retryable is False

    def test_override_code_and_retryable(self):
        e = RuneError("custom", code="CUSTOM_CODE", retryable=True)
        assert e.code == "CUSTOM_CODE"
        assert e.retryable is True

    def test_override_recovery_hint(self):
        e = VaultConnectionError("unreachable", recovery_hint="Custom hint for this case.")
        assert e.recovery_hint == "Custom hint for this case."

    def test_base_error_no_recovery_hint(self):
        e = RuneError("generic failure")
        assert e.recovery_hint == ""


class TestMakeError:
    def test_rune_error_produces_structured_dict(self):
        result = make_error(VaultConnectionError("cannot reach vault"))
        assert result["ok"] is False
        assert result["error"]["code"] == "VAULT_CONNECTION_ERROR"
        assert result["error"]["message"] == "cannot reach vault"
        assert result["error"]["retryable"] is True
        assert "recovery_hint" in result["error"]
        assert "/rune:status" in result["error"]["recovery_hint"]

    def test_generic_exception_falls_back(self):
        result = make_error(RuntimeError("unexpected"))
        assert result["ok"] is False
        assert result["error"]["code"] == "INTERNAL_ERROR"
        assert result["error"]["message"] == "unexpected"
        assert result["error"]["retryable"] is False
        assert "recovery_hint" not in result["error"]

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
        assert "/rune:activate" in result["error"]["recovery_hint"]

    def test_custom_recovery_hint_in_make_error(self):
        e = EnvectorConnectionError("timeout", recovery_hint="Custom: check your firewall settings.")
        result = make_error(e)
        assert result["error"]["recovery_hint"] == "Custom: check your firewall settings."

    def test_empty_recovery_hint_omitted(self):
        e = RuneError("generic")
        result = make_error(e)
        assert "recovery_hint" not in result["error"]
