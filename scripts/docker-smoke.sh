#!/usr/bin/env bash
set -euo pipefail

IMAGE=${IMAGE:-gatesentry:local}
CONTAINER=gatesentry-smoke-$$
response=$(mktemp)
cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -f "$response"
}
trap cleanup EXIT

docker run -d --name "$CONTAINER" -p 127.0.0.1::10786 -p 127.0.0.1::10413 -e GATESENTRY_DNS_PORT=10053 "$IMAGE" >/dev/null
web_port=$(docker port "$CONTAINER" 10786/tcp | awk -F: 'NR == 1 {print $NF}')
proxy_port=$(docker port "$CONTAINER" 10413/tcp | awk -F: 'NR == 1 {print $NF}')

for attempt in $(seq 1 30); do
  if curl --fail --silent --show-error "http://127.0.0.1:$web_port/" > "$response"; then
    break
  fi
  if [[ $attempt -eq 30 ]]; then
    docker logs "$CONTAINER"
    echo "error: dashboard did not become ready" >&2
    exit 1
  fi
  sleep 1
done

grep -Eqi '<!doctype html|<html' "$response" || { echo "error: dashboard smoke response is not HTML" >&2; exit 1; }
curl --fail --silent --show-error --proxy "http://127.0.0.1:$proxy_port" http://example.com/ >/dev/null
echo "Docker dashboard and explicit proxy smoke checks passed"

