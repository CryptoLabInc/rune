"""
Custom exception for the Rune MCP server.

Agents programmatically decide whether to retry, reconfigure, or report the failure

Error response format:
    {
        "ok": false,
        "error": {
            "code": "ERROR_CODE",
            "message": "...",
            "retryable": true
        }
    }
"""


class RuneError(Exception):
    code: str = "INTERNAL_ERROR"
    retryable: bool = False

    def __init__(self, message: str = "", *, code: str = None, retryable: bool = None):
        super().__init__(message)
        if code is not None:
            self.code = code
        if retryable is not None:
            self.retryable = retryable


# Vault errors
class VaultConnectionError(RuneError):
    code = "VAULT_CONNECTION_ERROR"
    retryable = True


class VaultDecryptionError(RuneError):
    code = "VAULT_DECRYPTION_ERROR"
    retryable = False


# envector errors
class EnvectorConnectionError(RuneError):
    code = "ENVECTOR_CONNECTION_ERROR"
    retryable = True


class EnvectorInsertError(RuneError):
    code = "ENVECTOR_INSERT_ERROR"
    retryable = True


# Pipeline errros
class PipelineNotReadyError(RuneError):
    code = "PIPELINE_NOT_READY"
    retryable = False


# Input errors
class InvalidInputError(RuneError):
    code = "INVALID_INPUT"
    retryable = False


# Helper functions
def make_error(exc: Exception) -> dict:
    """
    Convert an exception into a structured MCP error
    """
    if isinstance(exc, RuneError):
        return {
            "ok": False,
            "error": {
                "code": exc.code,
                "message": str(exc),
                "retryable": exc.retryable,
            },
        }
    # Fallback for unexpected exceptions (out of Rune)
    return {
        "ok": False,
        "error": {
            "code": "INTERNAL_ERROR",
            "message": str(exc),
            "retryable": False,
        },
    }
