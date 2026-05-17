#!/bin/bash

export GATESENTRY_DNS_PORT="${GATESENTRY_DNS_PORT:-53}"
export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-8.8.8.8:53}"

pkill -f gatestentry 2>/dev/null || true
sleep 1

make run
