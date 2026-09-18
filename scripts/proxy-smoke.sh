#!/usr/bin/env bash
# proxy-smoke.sh - build a GateSentry binary, run it as an explicit proxy,
# and verify filtering, exceptions, and MITM/non-MITM forwarding with curl.
# No Docker required. Needs Go, curl, and network access.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${GS_SMOKE_BIN:-/tmp/gatesentry-proxy-smoke}"
# The binary derives its data dir from its own path: dirname + /gatesentry
bin_dir="$(dirname "$BIN")"
DATA_DIR="$bin_dir/gatesentry"
BOOTSTRAP_DIR="${GS_SMOKE_BOOTSTRAP:-/tmp/gatesentry-proxy-smoke-bootstrap}"
CA_FILE="${GS_SMOKE_CA:-/tmp/gatesentry-proxy-smoke-ca.pem}"
PID_FILE="${GS_SMOKE_PID:-/tmp/gatesentry-proxy-smoke.pid}"
LOG_FILE="${GS_SMOKE_LOG:-/tmp/gatesentry-proxy-smoke.log}"

# Ports are hardcoded in the binary: 10786 (web), 10413 (proxy).
# DNS port is configurable via environment variable.
WEB_PORT=10786
PROXY_PORT=10413
DNS_PORT="${GS_SMOKE_DNS_PORT:-10054}"

ADMIN_USER="smoke-admin"
ADMIN_PASS="smokepass1234"
PROXY_USER="smokeproxy"
PROXY_PASS="smokeproxy-pass"
TEST_DOMAIN="httpbin.org"
TEST_PATH="/get"
FORWARD_DOMAIN="example.com"

fail() { echo "error: $*" >&2; exit 1; }

response=$(mktemp); health=$(mktemp); token_json=$(mktemp)
group_json=$(mktemp); exc_json=$(mktemp)
block_out=$(mktemp); bypass_out=$(mktemp); forward_out=$(mktemp)
mitm_out=$(mktemp); noauth_out=$(mktemp)
pause_json=$(mktemp); sched_json=$(mktemp)
cleanup_files() { rm -f "$response" "$health" "$token_json" "$group_json" "$exc_json" "$pause_json" "$sched_json" "$block_out" "$bypass_out" "$forward_out" "$mitm_out" "$noauth_out"; }

stop_binary() {
  if [[ -f "$PID_FILE" ]]; then
    local pid; pid=$(cat "$PID_FILE" 2>/dev/null || true)
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true; sleep 2; kill -9 "$pid" 2>/dev/null || true
    fi; rm -f "$PID_FILE"
  fi
}
cleanup() { stop_binary; cleanup_files; }
trap cleanup EXIT

# ---- 1. Build the binary ---------------------------------------------
echo "== building binary =="
cd "$ROOT"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.24.10}"
export PATH="${GOROOT:-/usr/local/go}/bin:$PATH"
go build -buildvcs=false -o "$BIN" . || fail "go build failed"
echo "  built $BIN"

# ---- 2. Prepare clean state -------------------------------------------
stop_binary
rm -rf "$DATA_DIR" 2>/dev/null || true
mkdir -p "$BOOTSTRAP_DIR"
printf '{"username":"smoke-admin","password":"smokepass1234"}\n' > "$BOOTSTRAP_DIR/bootstrap.json"
chmod 600 "$BOOTSTRAP_DIR/bootstrap.json"

# ---- 3. Start the binary (config via env vars, ports hardcoded) ------
echo "== starting GateSentry (web=$WEB_PORT proxy=$PROXY_PORT dns=$DNS_PORT) =="
: > "$LOG_FILE"
GATESENTRY_DNS_PORT="$DNS_PORT" \
GS_BASE_PATH="/" \
GATESENTRY_BOOTSTRAP_FILE="$BOOTSTRAP_DIR/bootstrap.json" \
"$BIN" > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

base="http://127.0.0.1:$WEB_PORT/api"
proxy_url="http://$PROXY_USER:$PROXY_PASS@127.0.0.1:$PROXY_PORT"

# ---- 4. Wait for dashboard --------------------------------------------
echo "== waiting for dashboard =="
for attempt in $(seq 1 30); do
  if curl --fail --silent --show-error "http://127.0.0.1:$WEB_PORT/" > "$response" 2>/dev/null; then break; fi
  if [[ $attempt -eq 30 ]]; then cat "$LOG_FILE" >&2; fail "dashboard did not become ready"; fi
  sleep 1
done
grep -Eqi '<!doctype html|<html' "$response" || fail "dashboard response is not HTML"
curl --fail --silent --show-error "http://127.0.0.1:$WEB_PORT/health" > "$health"
grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' "$health" || fail "health endpoint did not report ok"
echo "  dashboard up, health ok"

# ---- 5. Auth + enable proxy users -------------------------------------
echo "== configuring proxy auth =="
curl --fail --silent --show-error -X POST "$base/auth/token" -H 'Content-Type: application/json' -d '{"username":"smoke-admin","pass":"smokepass1234"}' > "$token_json"
token=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['Jwtoken'])" "$token_json")
auth_header="Authorization: Bearer $token"
curl --fail --silent --show-error -X POST "$base/settings/EnableUsers" -H 'Content-Type: application/json' -H "$auth_header" -d '{"value":"true"}' > /dev/null
curl --fail --silent --show-error -X POST "$base/users" -H 'Content-Type: application/json' -H "$auth_header" -d '{"username":"smokeproxy","password":"smokeproxy-pass","allowaccess":true}' > /dev/null
echo "  admin token, proxy users enabled, proxy user created"

# ---- 6. Non-MITM: HTTP forward ----------------------------------------
echo "== non-MITM: HTTP forward =="
curl -s -o "$forward_out" -w '%{http_code}' --proxy "$proxy_url" "http://$FORWARD_DOMAIN/" || true
forward_size=$(wc -c < "$forward_out")
[[ "$forward_size" -gt 100 ]] || fail "HTTP forward returned only $forward_size bytes"
echo "  HTTP $FORWARD_DOMAIN forwarded ($forward_size bytes)"

# ---- 7. Non-MITM: HTTPS tunnel ----------------------------------------
echo "== non-MITM: HTTPS tunnel =="
curl -s -o "$forward_out" -w '%{http_code}' --proxy "$proxy_url" "https://$FORWARD_DOMAIN/" || true
forward_size=$(wc -c < "$forward_out")
[[ "$forward_size" -gt 100 ]] || fail "HTTPS tunnel returned only $forward_size bytes"
echo "  HTTPS $FORWARD_DOMAIN tunnelled ($forward_size bytes)"

# ---- 8. Unauthenticated proxy rejected --------------------------------
echo "== auth: unauthenticated proxy rejected =="
http_code=$(curl -s -o /dev/null -w '%{http_code}' --proxy "http://127.0.0.1:$PROXY_PORT" "http://$FORWARD_DOMAIN/" || true)
[[ "$http_code" == "407" ]] || fail "unauthenticated proxy returned $http_code, expected 407"
echo "  no-credentials -> 407"

# ---- 9. Block via policy group -----------------------------------------
echo "== block via policy group =="
curl --fail --silent --show-error -X POST "$base/policy/groups" -H 'Content-Type: application/json' -H "$auth_header" -d '{"name":"smoke-block","domains":["httpbin.org"],"action":"block","users":["smokeproxy"]}' > "$group_json"
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
[[ "$block_size" -eq 0 ]] || fail "blocked domain returned $block_size bytes, expected 0"
echo "  $TEST_DOMAIN blocked (size=0)"

# ---- 10. Exception bypass ---------------------------------------------
echo "== exception bypass =="
curl --fail --silent --show-error -X POST "$base/exceptions" -H 'Content-Type: application/json' -H "$auth_header" -d '{"domain":"httpbin.org","scope":"installation","duration":"1h"}' > "$exc_json"
exc_id=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['id'])" "$exc_json")
curl -s -o "$bypass_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
bypass_size=$(wc -c < "$bypass_out")
[[ "$bypass_size" -gt 10 ]] || fail "exception bypass returned only $bypass_size bytes"
echo "  $TEST_DOMAIN bypassed ($bypass_size bytes)"

# ---- 11. Revoke exception ---------------------------------------------
echo "== revoke exception =="
curl --fail --silent --show-error -X DELETE "$base/exceptions/$exc_id" -H "$auth_header" > /dev/null
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
[[ "$block_size" -eq 0 ]] || fail "after revoke, blocked domain returned $block_size bytes, expected 0"
echo "  block restored after revoke (size=0)"

# ---- 12. Pause suppresses block --------------------------------------
echo "== pause: temporary suppression =="
curl --fail --silent --show-error -X POST "$base/pauses" -H 'Content-Type: application/json' -H "$auth_header" -d '{"scope":"installation","duration":"5m","reason":"smoke test"}' > "$pause_json"
pause_id=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['id'])" "$pause_json")
curl -s -o "$bypass_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
bypass_size=$(wc -c < "$bypass_out")
[[ "$bypass_size" -gt 10 ]] || fail "pause did not suppress block: $bypass_size bytes, expected forwarded content"
echo "  $TEST_DOMAIN forwarded during pause ($bypass_size bytes)"

# ---- 13. Revoke pause restores block ---------------------------------
echo "== pause: revoke restores block =="
curl --fail --silent --show-error -X DELETE "$base/pauses/$pause_id" -H "$auth_header" > /dev/null
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
[[ "$block_size" -eq 0 ]] || fail "after pause revoke, blocked domain returned $block_size bytes, expected 0"
echo "  block restored after pause revoke (size=0)"

# ---- 14. Inactive schedule suppresses block --------------------------
echo "== schedule: inactive window suppresses block =="
group_id=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['group']['id'])" "$group_json")
# Build a 30-minute time window 12 hours from now so it is guaranteed
# inactive at the current wall-clock time.
cur_hh=$(date +%H)
sched_hh=$(printf "%02d" $(( (10#$cur_hh + 12) % 24 )))
sched_from="${sched_hh}:00"
sched_to="${sched_hh}:30"
curl --fail --silent --show-error -X PUT "$base/policy/groups/$group_id" -H 'Content-Type: application/json' -H "$auth_header" \
  -d "{\"name\":\"smoke-block\",\"domains\":[\"httpbin.org\"],\"action\":\"block\",\"users\":[\"smokeproxy\"],\"priority\":0,\"schedule\":{\"timezone\":\"UTC\",\"windows\":[{\"from\":\"$sched_from\",\"to\":\"$sched_to\"}]}}" > /dev/null
curl -s -o "$bypass_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
bypass_size=$(wc -c < "$bypass_out")
[[ "$bypass_size" -gt 10 ]] || fail "inactive schedule did not suppress block: $bypass_size bytes, expected forwarded content"
echo "  $TEST_DOMAIN forwarded outside schedule window ($bypass_size bytes)"

# ---- 15. Clear schedule restores block -------------------------------
echo "== schedule: clear schedule restores block =="
curl --fail --silent --show-error -X PUT "$base/policy/groups/$group_id" -H 'Content-Type: application/json' -H "$auth_header" \
  -d '{"name":"smoke-block","domains":["httpbin.org"],"action":"block","users":["smokeproxy"],"priority":0}' > /dev/null
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
[[ "$block_size" -eq 0 ]] || fail "after clearing schedule, blocked domain returned $block_size bytes, expected 0"
echo "  block restored after schedule cleared (size=0)"

# ---- 16. Policy preview: active decision ---------------------------
echo "== policy preview: active decision =="
curl --fail --silent --show-error -X POST "$base/policy/preview" -H 'Content-Type: application/json' -H "$auth_header" \
  -d '{"user":"smokeproxy","domain":"httpbin.org"}' > "$sched_json"
active_action=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['active']['action'])" "$sched_json")
[[ "$active_action" == "block" ]] || fail "preview active action was $active_action, expected block"
echo "  preview reports active=block for httpbin.org"

# ---- 17. Policy preview: proposed change ---------------------------
echo "== policy preview: proposed comparison =="
group_id=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['group']['id'])" "$group_json")
curl --fail --silent --show-error -X POST "$base/policy/preview" -H 'Content-Type: application/json' -H "$auth_header" \
  -d "{\"user\":\"smokeproxy\",\"domain\":\"httpbin.org\",\"proposed\":{\"groups\":[{\"id\":\"preview-allow\",\"name\":\"preview-allow\",\"action\":\"allow\",\"domains\":[\"httpbin.org\"],\"users\":[\"smokeproxy\"]}],\"assignments\":[]}}" > "$sched_json"
proposed_action=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['proposed']['action'])" "$sched_json")
changed=$(python3 -c "import json,sys; print(str(json.load(open(sys.argv[1]))['changed']).lower())" "$sched_json")
[[ "$proposed_action" == "allow" ]] || fail "preview proposed action was $proposed_action, expected allow"
[[ "$changed" == "true" ]] || fail "preview changed was $changed, expected true"
echo "  preview reports proposed=allow, changed=true"

# ---- 18. Policy preview: live policy untouched ---------------------
echo "== policy preview: live policy untouched =="
curl -s -o "$block_out" -w '%{http_code}' --proxy "$proxy_url" "http://$TEST_DOMAIN$TEST_PATH" || true
block_size=$(wc -c < "$block_out")
[[ "$block_size" -eq 0 ]] || fail "after preview, live block returned $block_size bytes, expected 0"
echo "  live block still enforced after preview (size=0)"


# ---- 19. MITM: enable, download CA, verify HTTPS interception ---------
echo "== MITM: HTTPS interception =="
curl --fail --silent --show-error -X POST "$base/settings/enable_https_filtering" -H 'Content-Type: application/json' -H "$auth_header" -d '{"value":"true"}' > /dev/null
curl --fail --silent --show-error -o "$CA_FILE" "$base/files/certificate" -H "$auth_header"
head -1 "$CA_FILE" | grep -q 'BEGIN CERTIFICATE' || fail "downloaded CA is not a PEM certificate"
echo "  MITM enabled, CA certificate downloaded"
curl -s -o "$mitm_out" -w '%{http_code}' --proxy "$proxy_url" --cacert "$CA_FILE" "https://$FORWARD_DOMAIN/" || true
mitm_size=$(wc -c < "$mitm_out")
[[ "$mitm_size" -gt 100 ]] || fail "MITM HTTPS returned only $mitm_size bytes, expected real content"
echo "  HTTPS $FORWARD_DOMAIN MITM with trusted CA ($mitm_size bytes)"
noauth_code=$(curl -s -o "$noauth_out" -w '%{http_code}' --proxy "$proxy_url" "https://$FORWARD_DOMAIN/" 2>/dev/null || true)
if [[ "$noauth_code" != "000" ]]; then
  noauth_size=$(wc -c < "$noauth_out")
  [[ "$noauth_size" -eq 0 ]] || fail "MITM HTTPS without trusted CA returned $noauth_code/$noauth_size bytes, expected 000"
fi
echo "  HTTPS without trusted CA -> connection fails (interception confirmed)"

# ---- 20. Disable MITM -> HTTPS tunnels again -------------------------
echo "== disable MITM =="
curl --fail --silent --show-error -X POST "$base/settings/enable_https_filtering" -H 'Content-Type: application/json' -H "$auth_header" -d '{"value":"false"}' > /dev/null
curl -s -o "$forward_out" -w '%{http_code}' --proxy "$proxy_url" "https://$FORWARD_DOMAIN/" || true
forward_size=$(wc -c < "$forward_out")
[[ "$forward_size" -gt 100 ]] || fail "after disabling MITM, HTTPS tunnel returned only $forward_size bytes"
echo "  HTTPS tunnels again after MITM disabled ($forward_size bytes)"

echo ""
echo "All proxy smoke checks passed: HTTP forward, HTTPS tunnel, 407 auth,"
echo "policy block, exception bypass, revoke, pause suppress, pause revoke,"
echo "schedule inactive, schedule clear, policy preview, MITM intercept, MITM disable."
exit 0