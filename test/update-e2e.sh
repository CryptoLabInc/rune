#!/usr/bin/env bash
#
# End to end offline test for `rune update`
#
# Requirements: go, python3, curl, tar
# Test env: RUNE_UPDATE_TEST_WORK (default /tmp/rune-update-test)
#
# Options (default "all"):
#   mcp       - rune-mcp raw binary update test
#   runed     - runed update test as real tar.gz with live supervisor
#   plugin    - version outdated guard test
#   autocheck - automatic update test
#   noauto    - RUNE_NO_AUTO_UPDATE disable auto check
#   lock      - concurrent install test
#   all       - run all tests

set -euo pipefail

# Helper
PASS=0
green() { printf '\033[32m%s\033[0m' "$1"; }
red() { printf '\033[31m%s\033[0m' "$1"; }
pass() {
  PASS=$((PASS + 1))
  printf '  %s %s\n' "$(green PASS)" "$1"
}
fail() {
  printf '  %s %s\n' "$(red FAIL)" "$1"
  [ -f "$RUNED_HOME/logs/daemon.log" ] && {
    echo "  --- daemon.log ---"
    sed 's/^/  /' "$RUNED_HOME/logs/daemon.log"
  }
  exit 1
}
need() { command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 2; }; }

OUT="" # output
RC=0   # exit code
run() {
  set +e
  OUT="$("$@" 2>&1)"
  RC=$?
  set -e
}

assert_contains() { case "$1" in *"$2"*) pass "$3" ;; *) fail "$3 (got: $1)" ;; esac; }
assert_missing() { case "$1" in *"$2"*) fail "$3 (unexpectedly present: $2)" ;; *) pass "$3" ;; esac; }
assert_eq() { [ "$1" = "$2" ] && pass "$3" || fail "$3 (want $2, got $1)"; }

# Setup temporary isolated env for test
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="${RUNE_UPDATE_TEST_WORK:-/tmp/rune-update-test}"
TUPLE="$(go env GOOS)-$(go env GOARCH)"
SCENARIO="${1:-all}"

export RUNE_HOME="$WORK/home/.rune"
export RUNED_HOME="$WORK/home/.runed"

RUNE="$WORK/rune"
PORT=""
BASE=""
SRV_PID=""
MCP_PID=""
declare -a SUP_PIDS=()

STAMP="$RUNE_HOME/last-update-check" # RFC3339 stamp
UPDATE_LOG="$RUNE_HOME/update.log"   # detached `rune update` log
INSTALL_LOCK="$RUNED_HOME/install.lock"

# fake binary: raw_binary <dest-in-release> <marker text>
raw_binary() { printf '#!/bin/sh\n# %s\nexec tail -f /dev/null\n' "$2" >"$WORK/release/$1"; chmod +x "$WORK/release/$1"; }

# tarball with fake binary: runed_tgz <marker text> - build release/runed.tgz containing an executable `runed`
runed_tgz() {
  local d="$WORK/stage-runed"
  rm -rf "$d"; mkdir -p "$d"
  printf '#!/bin/sh\n# %s\nexec tail -f /dev/null\n' "$1" >"$d/runed"
  chmod +x "$d/runed"
  tar -C "$d" -czf "$WORK/release/runed.tgz" runed
}

# Read MCP_*/RUNED_*/PLUGIN_* from env, compute sha and size
write_manifest() { OUT_MANIFEST="$WORK/release/manifest.json" TUPLE="$TUPLE" python3 "$WORK/gen_manifest.py"; }

sha_of() { python3 - "$1" <<'PY'
import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],'rb').read()).hexdigest())
PY
}
audit_field() { python3 - "$RUNE_HOME/installed.json" "$@" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
for k in sys.argv[2:]:
    d=d[k]
print(d)
PY
}

# Server
start_server() {
  PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"
  BASE="http://127.0.0.1:${PORT}"
  export RUNE_MANIFEST="${BASE}/manifest.json"
  python3 -m http.server -d "$WORK/release" -b 127.0.0.1 "$PORT" >/dev/null 2>&1 &
  SRV_PID=$!
}

wait_socket() {
  python3 - "$1" <<'PY'
import os,sys,time
p=sys.argv[1]
for _ in range(150):
    if os.path.exists(p): sys.exit(0)
    time.sleep(0.1)
sys.exit(1)
PY
}

# Simulate `rune mcp-server`
spawn_mcp() {
  env "$@" timeout 60 "$RUNE" mcp-server </dev/null >>"$WORK/mcp-server.log" 2>&1 &
  MCP_PID=$!
}
stop_mcp() { { [ -n "${MCP_PID:-}" ] && kill "$MCP_PID" 2>/dev/null && wait "$MCP_PID" 2>/dev/null; } || true; MCP_PID=""; }

# poll until file <f> matches regex <re>
poll_contains() {
  local f="$1" re="$2" t="${3:-30}" i=0
  while :; do
    [ -f "$f" ] && grep -qE "$re" "$f" 2>/dev/null && return 0
    i=$((i + 1)); [ "$i" -ge "$t" ] && return 1
    sleep 1
  done
}

cleanup() {
  # Kill whole group
  for p in "${SUP_PIDS[@]:-}"; do
    [ -n "${p:-}" ] || continue
    kill -TERM "-$p" 2>/dev/null || true # negative pid = process group
    kill -TERM "$p" 2>/dev/null || true  # leader
  done

  [ -n "${MCP_PID:-}" ] && kill "$MCP_PID" 2>/dev/null || true
  if command -v pkill >/dev/null 2>&1; then
    pkill -TERM -f "$WORK/rune runed" 2>/dev/null || true
    pkill -TERM -f "$WORK/rune mcp-server" 2>/dev/null || true
    pkill -TERM -f "$WORK/rune update" 2>/dev/null || true
  fi
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

reset_home() { rm -rf "$WORK/home"; }

### Tests
scenario_mcp() {
  echo "=== scenario: rune-mcp raw update + integrity check ==="
  reset_home
  raw_binary rune-mcp "rune-mcp v0.1.0"
  raw_binary runed "runed v0.1.0"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" install
  assert_eq "$RC" 0 "install seeds v0.1.0"
  assert_eq "$(audit_field rune_mcp_version)" v0.1.0 "audit rune_mcp = v0.1.0"

  # Publish new version of rune-mcp
  raw_binary rune-mcp "rune-mcp v0.2.0 UPDATED"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.2.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" update --check
  assert_contains "$OUT" "rune_mcp: v0.1.0 -> v0.2.0" "--check reports rune_mcp outdated"

  run "$RUNE" update --only rune_mcp
  assert_eq "$RC" 0 "update --only rune_mcp exits 0"
  assert_contains "$OUT" "updated rune_mcp: v0.1.0 -> v0.2.0" "reports the apply"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.2.0 UPDATED" "on-disk rune-mcp swapped"
  assert_eq "$(audit_field rune_mcp_version)" v0.2.0 "audit bumped to v0.2.0"

  # Integrity test
  raw_binary rune-mcp "rune-mcp v0.3.0 CORRUPTED" # new version with wrong sha
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.3.0 MCP_EXTRACT="" MCP_SHA_OVERRIDE="$(printf '0%.0s' {1..64})" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" update --only rune_mcp
  assert_eq "$RC" 1 "checksum mismatch fails the update"
  assert_contains "$OUT" "checksum mismatch" "reports the mismatch"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.2.0 UPDATED" "bad download did NOT swap the binary"
  assert_eq "$(audit_field rune_mcp_version)" v0.2.0 "audit unchanged after failed update"
}

scenario_runed() {
  echo "=== scenario: runed tar.gz + live supervisor reload ==="
  reset_home
  raw_binary rune-mcp "rune-mcp v0.1.0"
  runed_tgz "runed v0.1.0"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed.tgz" RUNED_FILE="$WORK/release/runed.tgz" RUNED_VER=v0.1.0 RUNED_EXTRACT="tar.gz" \
    write_manifest

  run "$RUNE" install
  assert_eq "$RC" 0 "install extracts runed tarball"
  assert_contains "$(cat "$RUNED_HOME/bin/runed")" "runed v0.1.0" "extracted runed present"
  assert_eq "$(audit_field artifacts runed dest_sha256)" "$(sha_of "$RUNED_HOME/bin/runed")" \
    "audit dest_sha256 == extracted runed hash (not archive)"

  # Start runed supervisor and runed daemon
  run "$RUNE" runed --detach
  assert_eq "$RC" 0 "rune runed --detach launch the supervisor"
  wait_socket "$RUNED_HOME/supervisor.sock" || fail "supervisor socket never appeared"
  run "$RUNE" runed --status
  assert_contains "$OUT" "running (pid" "supervisor is running"
  local sup_before
  sup_before="$(printf '%s' "$OUT" | sed -n 's/.*pid \([0-9][0-9]*\).*/\1/p')"
  SUP_PIDS+=("$sup_before")

  # Publish new runed
  runed_tgz "runed v0.2.0 RELOADED"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed.tgz" RUNED_FILE="$WORK/release/runed.tgz" RUNED_VER=v0.2.0 RUNED_EXTRACT="tar.gz" \
    write_manifest

  run "$RUNE" update --only runed
  assert_eq "$RC" 0 "update --only runed exits 0"
  assert_contains "$OUT" "daemon reloaded" "live daemon was reloaded"
  assert_contains "$(cat "$RUNED_HOME/bin/runed")" "v0.2.0 RELOADED" "on-disk runed swapped"
  assert_eq "$(audit_field runed_version)" v0.2.0 "audit runed bumped to v0.2.0"
  assert_eq "$(audit_field artifacts runed dest_sha256)" "$(sha_of "$RUNED_HOME/bin/runed")" \
    "post-update audit dest_sha256 == extracted v0.2.0 runed hash (not archive)"

  run "$RUNE" runed --status
  assert_contains "$OUT" "running (pid" "supervisor still running after reload"
  local sup_after
  sup_after="$(printf '%s' "$OUT" | sed -n 's/.*pid \([0-9][0-9]*\).*/\1/p')"
  assert_eq "$sup_after" "$sup_before" "same supervisor running after reload (child restarted, not the supervisor)"
}

scenario_plugin() {
  echo "=== scenario: plugin/binary outdated version guard ==="
  reset_home
  raw_binary rune-mcp "rune-mcp v0.1.0"
  raw_binary runed "runed v0.1.0"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" install
  assert_eq "$RC" 0 "seed install"

  # Fake plugin package 0.4.1
  local proot="$WORK/plugin"
  mkdir -p "$proot/.claude-plugin"
  printf '{"name":"rune","version":"0.4.1"}\n' >"$proot/.claude-plugin/plugin.json"

  # Refuse: min_plugin_version (0.5.0) > installed (0.4.1)
  raw_binary rune-mcp "rune-mcp v0.2.0 FLOOR"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.2.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    PLUGIN_VER=0.5.0 MIN_PLUGIN_VER=0.5.0 write_manifest

  run "$RUNE" update --only rune_mcp --plugin-root "$proot"
  assert_eq "$RC" 1 "below minimum version is refused"
  assert_contains "$OUT" "refusing to apply" "refusal is surfaced"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.1.0" "refused update did NOT swap"
  assert_eq "$(audit_field rune_mcp_version)" v0.1.0 "audit unchanged on refusal"

  # Skip check outdated with '--allow-plugin-oudtaed'
  run "$RUNE" update --only rune_mcp --plugin-root "$proot" --allow-plugin-outdated
  assert_eq "$RC" 0 "bypass applies"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.2.0 FLOOR" "bypass swapped the binary"
  assert_contains "$OUT" "allow-plugin-outdated" "bypass warns about skew"

  # Installed 0.4.1 is >= min 0.4.0 but < expected 0.6.0
  raw_binary rune-mcp "rune-mcp v0.3.0 ADVISORY"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.3.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    PLUGIN_VER=0.6.0 MIN_PLUGIN_VER=0.4.0 write_manifest

  run "$RUNE" update --only rune_mcp --plugin-root "$proot"
  assert_eq "$RC" 0 "advisory-only update applies"
  assert_contains "$OUT" "plugin package is 0.4.1" "advisory note surfaced"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.3.0 ADVISORY" "advisory update swapped"
}

scenario_autocheck() {
  echo "=== scenario: auto-check ==="
  need timeout
  reset_home
  raw_binary rune-mcp "rune-mcp v0.1.0"
  raw_binary runed "runed v0.1.0"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" install
  assert_eq "$RC" 0 "seed install v0.1.0"

  # Publish new rune-mcp to trigger auto-check
  raw_binary rune-mcp "rune-mcp v0.2.0 AUTO"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.2.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  # Run auto check
  rm -f "$STAMP" "$UPDATE_LOG"
  spawn_mcp
  if poll_contains "$STAMP" 'T' 15; then # RFC3339 stamp always contains 'T'
    pass "stale stamp -> spawn ran the auto-check (stamp recorded)"
  else
    fail "spawn did not run the auto-check (no stamp)"
  fi
  if poll_contains "$RUNE_HOME/bin/rune-mcp" "v0.2.0 AUTO" 30 || poll_contains "$UPDATE_LOG" "rune_mcp" 5; then
    pass "spawn fired a detached rune update (rune-mcp swapped to v0.2.0)"
  else
    fail "spawn did not fire the detached rune update (no swap and no update.log within timeout)"
  fi
  stop_mcp

  # Initial setup -> no auto check
  date -u +%Y-%m-%dT%H:%M:%SZ >"$STAMP"
  rm -f "$UPDATE_LOG"
  spawn_mcp
  sleep 8
  [ ! -s "$UPDATE_LOG" ] && pass "fresh stamp -> auto-check suppressed" || fail "fresh stamp did not suppress the update"
  stop_mcp

  # Terminate detached `rune update`
  if command -v pkill >/dev/null 2>&1; then pkill -f "$WORK/rune update" 2>/dev/null || true; fi
  for _ in $(seq 1 50); do pgrep -f "$WORK/rune update" >/dev/null 2>&1 || break; sleep 0.1; done
}

scenario_noauto() {
  echo "=== scenario: RUNE_NO_AUTO_UPDATE disable auto-check ==="
  need timeout
  reset_home
  raw_binary rune-mcp "rune-mcp v0.1.0"
  raw_binary runed "runed v0.1.0"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" install
  assert_eq "$RC" 0 "seed install v0.1.0"

  raw_binary rune-mcp "rune-mcp v0.2.0 AUTO"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.2.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest
  rm -f "$STAMP" "$UPDATE_LOG"
  spawn_mcp RUNE_NO_AUTO_UPDATE=1
  sleep 6
  stop_mcp
  assert_eq "$([ -s "$UPDATE_LOG" ] && echo fired || echo none)" none "RUNE_NO_AUTO_UPDATE=1 -> no detached update"
  assert_eq "$([ -f "$STAMP" ] && echo stamped || echo none)" none "no stamp written"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.1.0" "installed rune-mcp intact"
}

scenario_lock() {
  echo "=== scenario: concurrent install lock ==="
  need flock
  reset_home
  raw_binary rune-mcp "rune-mcp v0.1.0"
  raw_binary runed "runed v0.1.0"
  MCP_URL="$BASE/rune-mcp" MCP_FILE="$WORK/release/rune-mcp" MCP_VER=v0.1.0 MCP_EXTRACT="" \
    RUNED_URL="$BASE/runed" RUNED_FILE="$WORK/release/runed" RUNED_VER=v0.1.0 RUNED_EXTRACT="" \
    write_manifest

  run "$RUNE" install
  assert_eq "$RC" 0 "seed install v0.1.0"

  # Hold install lock to trigger ErrInstallInProgress
  mkdir -p "$RUNED_HOME"
  exec 9>"$INSTALL_LOCK"
  flock -x 9

  run "$RUNE" install
  flock -u 9 2>/dev/null || true
  exec 9>&-
  assert_eq "$RC" 1 "concurrent install refused"
  assert_contains "$OUT" "another install in progress" "reports ErrInstallInProgress"
  assert_contains "$(cat "$RUNE_HOME/bin/rune-mcp")" "v0.1.0" "installed rune-mcp intact after refusal"
}

# Test main
need go; need python3; need curl; need tar

rm -rf "$WORK"; mkdir -p "$WORK/release"
echo "==+ build CLI ==="
( cd "$REPO" && go build -o "$RUNE" ./cmd/rune )

cat >"$WORK/gen_manifest.py" <<'PY'
import json, os, hashlib
def art(url, path, extract):
    b = open(path, "rb").read()
    d = {"url": url, "sha256": hashlib.sha256(b).hexdigest(), "size": len(b)}
    if extract:
        d["extract"] = extract
    return d
tuple_ = os.environ["TUPLE"]
m = {
    "version": 1,
    "rune_mcp_version": os.environ["MCP_VER"],
    "runed_version": os.environ["RUNED_VER"],
    "platforms": {tuple_: {
        "runed": art(os.environ["RUNED_URL"], os.environ["RUNED_FILE"], os.environ.get("RUNED_EXTRACT", "")),
        "rune_mcp": art(os.environ["MCP_URL"], os.environ["MCP_FILE"], os.environ.get("MCP_EXTRACT", "")),
    }},
}
if os.environ.get("PLUGIN_VER"):
    m["plugin_version"] = os.environ["PLUGIN_VER"]
if os.environ.get("MIN_PLUGIN_VER"):
    m["min_plugin_version"] = os.environ["MIN_PLUGIN_VER"]
# optional integrity-test override: advertise a wrong rune-mcp hash
if os.environ.get("MCP_SHA_OVERRIDE"):
    m["platforms"][tuple_]["rune_mcp"]["sha256"] = os.environ["MCP_SHA_OVERRIDE"]
json.dump(m, open(os.environ["OUT_MANIFEST"], "w"), indent=2)
PY

start_server

if ! curl --retry 50 --retry-connrefused --connect-timeout 2 --retry-max-time 15 -sf "${BASE}/" >/dev/null; then
  if kill -0 "$SRV_PID" 2>/dev/null; then
    echo "local server did not become ready on $BASE" >&2
  else
    echo "local http server failed to start (maybe check port $PORT is available) - re-run" >&2
  fi
  exit 1
fi

case "$SCENARIO" in
  mcp) scenario_mcp ;;
  runed) scenario_runed ;;
  plugin) scenario_plugin ;;
  autocheck) scenario_autocheck ;;
  noauto) scenario_noauto ;;
  lock) scenario_lock ;;
  all) scenario_mcp; echo; scenario_runed; echo; scenario_plugin; echo; scenario_autocheck; echo; scenario_noauto; echo; scenario_lock ;;
  *) echo "unknown scenario: $SCENARIO (want: mcp|runed|plugin|autocheck|noauto|lock|all)" >&2; exit 2 ;;
esac

echo
echo "$(green "ALL PASS") - $PASS checks"
