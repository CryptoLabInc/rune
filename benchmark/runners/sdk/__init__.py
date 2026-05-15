"""SDK-version-dispatched adapters for the unified latency benchmark.

`get_sdk_adapter()` detects the installed pyenvector version and returns the
matching adapter. The runner stays SDK-agnostic and talks only to the
`SdkAdapter` interface.
"""

from __future__ import annotations

from .base import SdkAdapter, SearchableCtx

__all__ = ["SdkAdapter", "SearchableCtx", "get_sdk_adapter"]


def get_sdk_adapter() -> SdkAdapter:
    """Return the adapter matching the installed pyenvector version.

    Raises RuntimeError on an unsupported version — we deliberately never
    fall back to a "closest" adapter, because measuring on the wrong SDK
    configuration silently produces misleading numbers.
    """
    import pyenvector

    version = getattr(pyenvector, "__version__", "")

    if version.startswith("1.2.2"):
        from .v122 import V122Adapter

        return V122Adapter()

    if version.startswith("1.4.3"):
        from .v143 import V143Adapter

        return V143Adapter()

    raise RuntimeError(
        f"Unsupported pyenvector version {version!r}. "
        f"Supported: 1.2.2, 1.4.3. To add another, write a new adapter in "
        f"benchmark/runners/sdk/ and register it in get_sdk_adapter()."
    )
