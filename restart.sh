#!/bin/bash

export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-192.168.1.1:53}"

# Admin UI port — default 80 requires root; use 8080 for local dev
export GS_ADMIN_PORT="${GS_ADMIN_PORT:-8080}"
export GS_MAX_SCAN_SIZE_MB="${GS_MAX_SCAN_SIZE_MB:-2}"

# Unset proxy env vars — the GateSentry proxy server must not route its own
# outbound requests through itself (or any other proxy).
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY no_proxy NO_PROXY

P=$(pgrep gatesentry)

if [ "$P" == "" ]; then
  echo "No existing server process found. Starting new server..."

  cd bin && ./gatesentrybin > ../log.txt 2>&1 &

  echo "Done."
  exit 0
fi

echo "Stopping PID:$P..."

kill $P

while [ "$P" != "" ]; do
  P=$(pgrep gatesentry)
done

echo -n "Starting new server..."

cd bin && ./gatesentrybin > ../log.txt 2>&1 &

while [ "$P" == "" ]; do
  P=$(pgrep gatesentry)
done

echo "PID:$P"...
echo "Done."
