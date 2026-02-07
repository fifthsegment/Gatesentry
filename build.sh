#!/usr/bin/env bash
set -euo pipefail

if [ ! -d "bin" ]; then
    mkdir bin
else
    echo "Cleaning existing bin directory..."
    rm -rf bin/*
fi
echo "Building GateSentry..."
go build -o bin/ ./...
echo "Build successful. Executable is in the 'bin' directory."
