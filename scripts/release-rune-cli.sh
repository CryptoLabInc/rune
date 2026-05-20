#!/usr/bin/env bash
# Cross-build the rune CLI
#
# Output layout: dist/rune-<os>-<arch>{,.exe} + dist/checksums.txt
#
# Usage:
#   scripts/release-rune-cli.sh [version]

set -euo pipefail

VERSION="${1:-v0.4.0-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/dist"

TARGETS=(
  "linux  amd64  rune-linux-amd64"
  "linux  arm64  rune-linux-arm64"
  "darwin amd64  rune-darwin-amd64"
  "darwin arm64  rune-darwin-arm64"
)

rm -rf "$OUT"
mkdir -p "$OUT"

# Strip debug and symbol tables, set runeVersion into cmd/rune/main.go
LDFLAGS="-s -w -X main.runeVersion=$VERSION"

for line in "${TARGETS[@]}"; do
  read -r GOOS GOARCH ASSET <<<"$line"
  echo "building $ASSET ($GOOS/$GOARCH)..."
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -trimpath -ldflags "$LDFLAGS" \
    -o "$OUT/$ASSET" "$ROOT/cmd/rune"
done

echo "computing checksums..."
(cd "$OUT" && sha256sum rune-* > checksums.txt)

echo
echo "release artifacts ready in $OUT/ for $VERSION:"
ls -lh "$OUT"
