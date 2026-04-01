"""
Custom exception for the Rune MCP server.

Agents programmatically decide whether to retry, reconfigure, or report the failure

Error response format:
    {
        "ok": false,
        "error": {
            "code": "ERROR_CODE",
            "message": "...",
            "retryable": true,
            "recovery_hint": "..."
        }
    }
"""


class RuneError(Exception):
    code: str = "INTERNAL_ERROR"
    retryable: bool = False
    recovery_hint: str = ""

    def __init__(self, message: str = "", *, code: str = None, retryable: bool = None, recovery_hint: str = None):
        super().__init__(message)
        if code is not None:
            self.code = code
        if retryable is not None:
            self.retryable = retryable
        if recovery_hint is not None:
            self.recovery_hint = recovery_hint


# Vault errors
class VaultConnectionError(RuneError):
    code = "VAULT_CONNECTION_ERROR"
    retryable = True
    recovery_hint = (
        "Vault is unreachable. Check: (1) Is the Vault server running? "
        "(2) Is the endpoint correct in ~/.rune/config.json? "
        "Run /rune:status for diagnostics."
    )


class VaultDecryptionError(RuneError):
    code = "VAULT_DECRYPTION_ERROR"
    retryable = False
    recovery_hint = (
        "Vault rejected the decryption request. Check: (1) Is your Vault token valid and not expired? "
        "(2) Does the token have permission for this team index? "
        "Run /rune:configure to update credentials."
    )


# envector errors
class EnvectorConnectionError(RuneError):
    code = "ENVECTOR_CONNECTION_ERROR"
    retryable = True
    recovery_hint = (
        "Cannot reach enVector Cloud. Check: (1) Network connectivity, "
        "(2) enVector endpoint in ~/.rune/config.json. "
        "Run /rune:status for diagnostics."
    )


class EnvectorInsertError(RuneError):
    code = "ENVECTOR_INSERT_ERROR"
    retryable = True
    recovery_hint = (
        "Failed to store data in enVector. This may be transient — retry in a moment. "
        "If persistent, check your API key and index permissions via /rune:status."
    )


# Pipeline errors
class PipelineNotReadyError(RuneError):
    code = "PIPELINE_NOT_READY"
    retryable = False
    recovery_hint = (
        "Pipelines are not initialized. Run /rune:activate to reinitialize, "
        "or restart Claude Code if the problem persists."
    )


# Input errors
class InvalidInputError(RuneError):
    code = "INVALID_INPUT"
    retryable = False
    recovery_hint = "Check input parameters and try again"


# Helper functions
def make_error(exc: Exception) -> dict:
    """
    Convert an exception into a structured MCP error
    """
    if isinstance(exc, RuneError):
        result = {
            "ok": False,
            "error": {
                "code": exc.code,
                "message": str(exc),
                "retryable": exc.retryable,
            },
        }
        hint = exc.recovery_hint
        if hint:
            result["error"]["recovery_hint"] = hint
        return result
    # Fallback for unexpected exceptions (out of Rune)
    return {
        "ok": False,
        "error": {
            "code": "INTERNAL_ERROR",
            "message": str(exc),
            "retryable": False,
        },
    }
