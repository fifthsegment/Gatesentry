#!/usr/bin/env bash
set -euo pipefail

EMBED_DIR="application/webserver/frontend/files"
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

if $BUILD_UI; then
  if [ -d "ui/node_modules" ]; then
    echo "Building Svelte UI..."
    (cd ui && npm run build)
    echo "Copying UI dist into Go embed directory..."
    find "${EMBED_DIR}" -mindepth 1 ! -name '.gitkeep' -delete
    cp -r ui/dist/* "$EMBED_DIR"/
  else
    echo "Skipping UI build (ui/node_modules not found — run 'cd ui && npm install' first)"
    echo "Using existing frontend files in $EMBED_DIR"
  fi
fi

echo "Building GateSentry → $OUTPUT..."
go build -ldflags="-s -w" -o "$OUTPUT" .
echo "Build successful: $OUTPUT"
