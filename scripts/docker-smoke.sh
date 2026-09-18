#!/usr/bin/env bash
set -euo pipefail

IMAGE=${IMAGE:-gatesentry:local}
CONTAINER=gatesentry-smoke-$$
response=$(mktemp)
health=$(mktemp)
bootstrap=$(mktemp)
token_json=$(mktemp)
group_json=$(mktemp)
exc_json=$(mktemp)
block_out=$(mktemp)
bypass_out=$(mktemp)
mitm_out=$(mktemp)

# Admin and proxy-user credentials for the smoke test.
SMOKE_ADMIN_USER=smoke-admin
SMOKE_ADMIN_PASS=smokepass1234
SMOKE_PROXY_USER=smokeproxy
SMOKE_PROXY_PASS=smokeproxy-pass

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -f "$response" "$health" "$bootstrap" "$token_json" "$group_json" "$exc_json" "$block_out" "$bypass_out" "$mitm_out"
}
trap cleanup EXIT

# A domain we control nothing about but can reach through the proxy.
TEST_DOMAIN=httpbin.org
TEST_PATH=/get

# GateSentry auto-completes first-run setup when a bootstrap file with a
# username/password is mounted.  The file must be mode 0600.
cat > "$bootstrap" <<JSON
{"username":"$SMOKE_ADMIN_USER","password":"$SMOKE_ADMIN_PASS"}
JSON
chmod 600 "$bootstrap"

docker run -d --name "$CONTAINER" \
  -p 127.0.0.1::10786 -p 127.0.0.1::10413 \
  -e GATESENTRY_DNS_PORT=10053 \
  -e GS_BASE_PATH=/ \
  -v "$bootstrap:/bootstrap.json:ro" \
  -e GATESENTRY_BOOTSTRAP_FILE=/bootstrap.json \
  "$IMAGE" >/dev/null

web_port=$(docker port "$CONTAINER" 10786/tcp | awk -F: 'NR == 1 {print $NF}')
proxy_port=$(docker port "$CONTAINER" 10413/tcp | awk -F: 'NR == 1 {print $NF}')
base="http://127.0.0.1:$web_port/api"

# --- Wait for dashboard ---
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
curl --fail --silent --show-error "http://127.0.0.1:$web_port/health" > "$health"
grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' "$health" || { echo "error: health endpoint did not report ok" >&2; exit 1; }

# --- Auth ---
curl --fail --silent --show-error -X POST "$base/auth/token" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$SMOKE_ADMIN_USER\",\"pass\":\"$SMOKE_ADMIN_PASS\"}" > "$token_json"
token=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['Jwtoken'])" "$token_json")
auth_header="Authorization: Bearer $token"

# --- Enable proxy users ---
curl --fail --silent --show-error -X POST "$base/settings/EnableUsers" \
  -H 'Content-Type: application/json' -H "$auth_header" \
  -d '{"value":"true"}' > /dev/null

# --- Create proxy user ---
curl --fail --silent --show-error -X POST "$base/users" \
  -H 'Content-Type: application/json' -H "$auth_header" \
  -d "{\"username\":\"$SMOKE_PROXY_USER\",\"password\":\"$SMOKE_PROXY_PASS\",\"allowaccess\":true}" > /dev/null

# --- Create block policy group ---
curl --fail --silent --show-error -X POST "$base/policy/groups" \
  -H 'Content-Type: application/json' -H "$auth_header" \
  -d "{\"name\":\"smoke-block\",\"domains\":[\"$TEST_DOMAIN\"],\"action\":\"block\",\"users\":[\"$SMOKE_PROXY_USER\"]}" > "$group_json"

proxy_url="http://$SMOKE_PROXY_USER:$SMOKE_PROXY_PASS@127.0.0.1:$proxy_port"

# --- Test 1: Blocked domain returns empty body ---
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
if [[ "$block_size" -ne 0 ]]; then
  echo "error: blocked domain returned $block_size bytes, expected 0 (block path writes no body)" >&2
  exit 1
fi
echo "  block: $TEST_DOMAIN returns empty body (size=0)"

# --- Test 2: Installation-scoped exception bypasses the block ---
curl --fail --silent --show-error -X POST "$base/exceptions" \
  -H 'Content-Type: application/json' -H "$auth_header" \
  -d "{\"domain\":\"$TEST_DOMAIN\",\"scope\":\"installation\",\"duration\":\"1h\"}" > "$exc_json"
exc_id=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['id'])" "$exc_json")

curl -s -o "$bypass_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
bypass_size=$(wc -c < "$bypass_out")
if [[ "$bypass_size" -le 10 ]]; then
  echo "error: exception bypass returned only $bypass_size bytes, expected real content" >&2
  exit 1
fi
echo "  exception: $TEST_DOMAIN bypasses block (size=$bypass_size)"

# --- Test 3: Revoking the exception restores the block ---
curl --fail --silent --show-error -X DELETE "$base/exceptions/$exc_id" -H "$auth_header" > /dev/null
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
if [[ "$block_size" -ne 0 ]]; then
  echo "error: after revoke, blocked domain returned $block_size bytes, expected 0" >&2
  exit 1
fi
echo "  revoke: block restored after exception revoked (size=0)"

# --- Test 4: Non-MITM proxy forwards normal traffic ---
curl -s -o "$mitm_out" -w '%{http_code}' --proxy "$proxy_url" "http://example.com/" || true
mitm_size=$(wc -c < "$mitm_out")
if [[ "$mitm_size" -le 100 ]]; then
  echo "error: normal HTTP proxy returned only $mitm_size bytes for example.com" >&2
  exit 1
fi
echo "  proxy: HTTP example.com forwarded (size=$mitm_size)"

# --- Test 5: Unauthenticated proxy is rejected ---
http_code=$(curl -s -o /dev/null -w '%{http_code}' --proxy "http://127.0.0.1:$proxy_port" "http://example.com/" || true)
if [[ "$http_code" != "407" ]]; then
  echo "error: unauthenticated proxy returned $http_code, expected 407" >&2
  exit 1
fi
echo "  auth: unauthenticated proxy rejected (407)"

echo "Docker dashboard, health, proxy block/exception/revoke, and auth smoke checks passed"

