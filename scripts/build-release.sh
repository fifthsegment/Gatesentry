#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUTPUT_DIR=${1:-$ROOT/dist}
REVISION=$(git -C "$ROOT" rev-parse HEAD)

"$ROOT/scripts/frontend.sh" validate
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

build() {
  local goos=$1 goarch=$2 output=$3
  echo "Building $output from $REVISION"
  (cd "$ROOT" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" -o "$OUTPUT_DIR/$output" .)
}

build linux amd64 gatesentry-linux-amd64
build linux arm64 gatesentry-linux-arm64
build darwin amd64 gatesentry-darwin-amd64
build darwin arm64 gatesentry-darwin-arm64
build windows amd64 gatesentry-windows-amd64.exe
printf '%s\n' "$REVISION" > "$OUTPUT_DIR/REVISION"
(cd "$OUTPUT_DIR" && sha256sum gatesentry-* > SHA256SUMS)

