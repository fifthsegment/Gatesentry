#!/usr/bin/env bash
set -euo pipefail

EMBED_DIR="application/webserver/frontend/files"

# ── Step 1: Build the Svelte UI ──────────────────────────────────────────────
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

# ── Step 2: Build the Go binary ─────────────────────────────────────────────
if [ ! -d "bin" ]; then
    mkdir bin
else
    echo "Cleaning existing bin directory..."
    rm -rf bin/*
fi
echo "Building GateSentry..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ ./...
echo "Build successful. Executable is in the 'bin' directory."
