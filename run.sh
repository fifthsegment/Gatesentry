#!/bin/bash

# DNS Server Configuration
# Set the listen address (default: 0.0.0.0 - all interfaces)
export GATESENTRY_DNS_ADDR="${GATESENTRY_DNS_ADDR:-0.0.0.0}"

# Set the DNS port (default: 53, use 5353 or other if 53 is in use)
export GATESENTRY_DNS_PORT="${GATESENTRY_DNS_PORT:-53}"

# Set the external resolver (default: 8.8.8.8:53)
export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-8.8.8.8:53}"

# Admin UI port — default 80 requires root; use 8080 for local dev
export GS_ADMIN_PORT="${GS_ADMIN_PORT:-8080}"

# Kill any existing gatesentry processes so the new binary can bind ports
pkill -f gatesentryb 2>/dev/null
sleep 1

rm -rf bin
mkdir bin
./build.sh && cd bin && ./gatesentrybin > ../log.txt 2>&1
