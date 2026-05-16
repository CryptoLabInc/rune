#!/usr/bin/env bash
# Select and install the pyenvector SDK version for the latency benchmark.
#
# The benchmark measures one SDK version per machine — 1.2.x and 1.4.x cannot
# coexist in one venv. This selector installs exactly the requested version
# into the Rune plugin venv (the environment the benchmark runs in) so the
# runner's get_sdk_adapter() detects it.
#
#   1.2.2    rmp / flat      — Phase 3
#   1.4.3    mm32 / ivf_vct  — Phase 4
#   latest   newest release  — installs only; the runner has no adapter for
#                              versions other than 1.2.2 / 1.4.3 yet
#
# Usage: benchmark/scripts/install_sdk.sh <1.2.2|1.4.3|latest>
set -euo pipefail

choice="${1:-}"

# The benchmark runs in the Rune plugin venv. Its cache path is version-coded
# (~/.claude/plugins/cache/cryptolab/rune/<version>/.venv) — pick the most
# recently modified so a plugin upgrade is followed automatically.
venv="$(ls -dt "$HOME"/.claude/plugins/cache/cryptolab/rune/*/.venv 2>/dev/null | head -1)"

usage() {
    echo "usage: $(basename "$0") <1.2.2|1.4.3|latest>" >&2
    echo "  1.2.2    rmp / flat      — Phase 3" >&2
    echo "  1.4.3    mm32 / ivf_vct  — Phase 4" >&2
    echo "  latest   newest release  — install only, no runner adapter yet" >&2
}

if [[ -z "$venv" || ! -x "$venv/bin/pip" ]]; then
    echo "error: Rune plugin venv not found under" >&2
    echo "       ~/.claude/plugins/cache/cryptolab/rune/*/.venv" >&2
    exit 1
fi

case "$choice" in
    1.2.2|1.4.3) spec="pyenvector==$choice" ;;
    latest)      spec="pyenvector" ;;
    *)           usage; exit 2 ;;
esac

echo "Installing $spec into $venv ..."
"$venv/bin/pip" install --upgrade "$spec"

installed="$("$venv/bin/python" -c 'import pyenvector; print(pyenvector.__version__)')"
echo
echo "pyenvector $installed installed in $venv"

case "$installed" in
    1.2.2*|1.4.3*)
        echo "runner: get_sdk_adapter() recognises this version."
        ;;
    *)
        echo "WARNING: the benchmark runner supports only 1.2.2 and 1.4.3." >&2
        echo "         get_sdk_adapter() will raise on $installed — add an" >&2
        echo "         adapter under benchmark/runners/sdk/ before benchmarking." >&2
        ;;
esac
