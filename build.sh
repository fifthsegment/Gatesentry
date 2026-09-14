#!/usr/bin/env bash
set -euo pipefail

BUILD_UI=true
OUTPUT="${OUTPUT:-bin/gatesentrybin}"

while [[ $# -gt 0 ]]; do
  case $1 in
    --no-ui) BUILD_UI=false; shift ;;
    -o) OUTPUT="$2"; shift 2 ;;
    *) shift ;;
  esac
done

OUTDIR=$(dirname "$OUTPUT")
mkdir -p "$OUTDIR"

./scripts/check-toolchains.sh go
export GOTOOLCHAIN=go1.24.10

if $BUILD_UI; then
  ./scripts/frontend.sh
fi

echo "Building GateSentry → $OUTPUT..."
go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$OUTPUT" .
echo "Build successful: $OUTPUT"
