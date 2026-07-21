#!/usr/bin/env bash
set -euo pipefail

OUTDIR=${1:-dist}
VERSION=${VERSION:-v5.7.2}

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST="$ROOT_DIR/$OUTDIR"
mkdir -p "$DIST"

cd "$ROOT_DIR"
export CGO_ENABLED=0

build() {
  local GOOS=$1
  local GOARCH=$2
  local OUT=$3
  echo "Building $GOOS/$GOARCH -> $DIST/$OUT"
  GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags "-s -w" -o "$DIST/$OUT" ./cmd/pingmonitor
}

build windows amd64 "pingmonitor_${VERSION}_windows_amd64.exe"
build linux amd64   "pingmonitor_${VERSION}_linux_amd64"
build linux arm64   "pingmonitor_${VERSION}_linux_arm64"
build darwin amd64  "pingmonitor_${VERSION}_darwin_amd64"
build darwin arm64  "pingmonitor_${VERSION}_darwin_arm64"

MANIFEST="$DIST/SHA256SUMS_${VERSION}.txt"
: > "$MANIFEST"
for artifact in "$DIST"/pingmonitor_${VERSION}_*; do
  if command -v sha256sum >/dev/null 2>&1; then
    hash=$(sha256sum "$artifact" | awk '{print $1}')
  else
    hash=$(shasum -a 256 "$artifact" | awk '{print $1}')
  fi
  printf '%s  %s\n' "$hash" "$(basename "$artifact")" >> "$MANIFEST"
done
echo "Checksums -> $MANIFEST"
