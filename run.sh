#!/bin/bash

# DNS Server Configuration
# Set the listen address (default: 0.0.0.0 - all interfaces)
export GATESENTRY_DNS_ADDR="${GATESENTRY_DNS_ADDR:-0.0.0.0}"

# Set the DNS port (default: 10053 for local dev, avoids conflict with system DNS)
export GATESENTRY_DNS_PORT="${GATESENTRY_DNS_PORT:-10053}"

# Set the external resolver (default: local network DNS)
# 192.168.1.1 is the authoritative DNS for the local network,
# including custom records (e.g. httpbin.org → 192.168.1.105)
export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-192.168.1.1:53}"

# Admin UI port — default 80 requires root; use 8080 for local dev
export GS_ADMIN_PORT="${GS_ADMIN_PORT:-8080}"
export GS_MAX_SCAN_SIZE_MB="${GS_MAX_SCAN_SIZE_MB:-2}"

# Kill any existing gatesentry processes so the new binary can bind ports
pkill -f gatesentryb 2>/dev/null
sleep 1

if [ "$1" == "--build" ]; then
    echo "Building GateSentry binary..."
    bash build.sh
    if [ $? -ne 0 ]; then
        echo "Build failed. Exiting."
        exit 1
    fi
fi

cd bin && ./gatesentrybin > ../log.txt 2>&1
