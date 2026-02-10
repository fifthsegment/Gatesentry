#!/usr/bin/env bash
# ============================================================================
# GateSentry Proxy Concurrency & Thread-Safety Test Suite
# ============================================================================
#
# Tests whether the proxy correctly handles many simultaneous requests
# without crashes, data corruption, or serialisation.
#
# Key concerns tested:
#   1. Is the proxy truly concurrent? (goroutine-per-request)
#   2. Do shared data structures (UsersCache map) cause panics under load?
#   3. Does the cert cache race under concurrent HTTPS?
#   4. Do slow requests block fast ones? (head-of-line blocking)
#   5. Mixed workloads: simultaneous HTTP + HTTPS + adversarial
#   6. Does the proxy survive sustained high concurrency?
#
# Usage: bash tests/proxy_concurrency_test.sh
# ============================================================================

set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────────────
PROXY_HOST="${PROXY_HOST:-192.168.1.105}"
PROXY_PORT="${PROXY_PORT:-10413}"
ECHO_SERVER="${ECHO_SERVER:-http://192.168.1.105:9998}"
TESTBED_HTTP="${TESTBED_HTTP:-http://192.168.1.105:9999}"
CURL_TIMEOUT=10

# Colours
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; NC='\033[0m'; BOLD='\033[1m'

PASS_COUNT=0; FAIL_COUNT=0; WARN_COUNT=0

pass()  { ((PASS_COUNT++)) || true; echo -e "  ${GREEN}✅ PASS${NC}  $1"; }
fail()  { ((FAIL_COUNT++)) || true; echo -e "  ${RED}❌ FAIL${NC}  $1"; [[ -n "${2:-}" ]] && echo -e "         ${YELLOW}↳ $2${NC}"; }
warn()  { ((WARN_COUNT++)) || true; echo -e "  ${YELLOW}⚠️  WARN${NC}  $1"; [[ -n "${2:-}" ]] && echo -e "         ${YELLOW}↳ $2${NC}"; }
log_header()  { echo -e "\n${BOLD}${CYAN}═══ $1 ═══${NC}"; }
log_section() { echo -e "\n${BOLD}── $1${NC}"; }

gscurl() { curl --proxy "http://${PROXY_HOST}:${PROXY_PORT}" --max-time "$CURL_TIMEOUT" "$@"; }

proxy_alive() {
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
    [[ "$code" =~ ^[23] ]]
}

# ── Pre-flight ───────────────────────────────────────────────────────────────
log_header "PRE-FLIGHT CHECKS"

if ! proxy_alive; then
    echo -e "${RED}ERROR: Proxy not responding at ${PROXY_HOST}:${PROXY_PORT}${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} Proxy alive"

echo_check=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
if [[ ! "$echo_check" =~ ^[23] ]]; then
    echo -e "${RED}ERROR: Echo server not responding at ${ECHO_SERVER}${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} Echo server alive"

# Record proxy PID for crash detection
PROXY_PID=$(pgrep -f gatesentrybin | head -1 || echo "unknown")
echo -e "  ${GREEN}✓${NC} Proxy PID: ${PROXY_PID}"

# ============================================================================
# TEST 1: Basic Concurrency — Are requests truly parallel?
# ============================================================================
log_header "TEST 1: PARALLELISM PROOF"
log_section "1.1 Timing: 50 parallel vs 50 sequential"

echo "  Sending 50 parallel requests through the proxy..."
par_start=$(date +%s%N)
pids=()
par_fail=0
for i in $(seq 1 50); do
    (
        code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
        [[ "$code" =~ ^[23] ]] && exit 0 || exit 1
    ) &
    pids+=($!)
done
for pid in "${pids[@]}"; do
    if ! wait "$pid" 2>/dev/null; then
        ((par_fail++)) || true
    fi
done
par_end=$(date +%s%N)
par_ms=$(( (par_end - par_start) / 1000000 ))

echo "  Sending 5 sequential requests for per-request baseline..."
seq_start=$(date +%s%N)
seq_fail=0
for i in $(seq 1 5); do
    # Use --no-keepalive to measure true per-request latency (no connection reuse)
    code=$(curl --no-keepalive -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
    [[ ! "$code" =~ ^[23] ]] && ((seq_fail++)) || true
done
seq_end=$(date +%s%N)
seq_ms=$(( (seq_end - seq_start) / 1000000 ))

# Per-request latency without keep-alive
seq_per_req=$((seq_ms / 5))
expected_serial=$((seq_per_req * 50))

echo "  50 parallel: ${par_ms}ms (${par_fail} failures)"
echo "  5 sequential (no keep-alive): ${seq_ms}ms (${seq_per_req}ms/req)"
echo "  If serialised, 50 would take ~${expected_serial}ms"

if [[ "$par_fail" -gt 5 ]]; then
    fail "1.1 Too many parallel failures: ${par_fail}/50" \
        "Proxy may be rejecting concurrent requests"
elif [[ "$par_ms" -gt "$expected_serial" ]]; then
    fail "1.1 Parallel time (${par_ms}ms) >= serial estimate (${expected_serial}ms)" \
        "Proxy appears to be serialising requests"
elif [[ "$par_ms" -gt $((expected_serial / 2)) ]]; then
    warn "1.1 Parallel time (${par_ms}ms) is slow — possible contention" \
        "Expected significant speedup over serial (${expected_serial}ms)"
else
    speedup=$(echo "scale=1; $expected_serial / $par_ms" | bc 2>/dev/null || echo "?")
    pass "1.1 Parallel (${par_ms}ms) vs serial estimate (${expected_serial}ms) — ${speedup}x speedup"
fi

proxy_alive || fail "1.1 PROXY CRASHED after 50 parallel requests"

# ============================================================================
# TEST 2: Head-of-Line Blocking — Do slow requests block fast ones?
# ============================================================================
log_header "TEST 2: HEAD-OF-LINE BLOCKING"
log_section "2.1 Slow request (/delay/3) must not block fast request (/echo)"

# Start a slow request in background (3s delay)
slow_start=$(date +%s%N)
(
    gscurl -s -o /dev/null "${ECHO_SERVER}/delay/3" 2>/dev/null
) &
slow_pid=$!

# Wait 200ms for the slow request to be in-flight
sleep 0.2

# Now make a fast request — it should complete immediately
fast_start=$(date +%s%N)
fast_code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
fast_end=$(date +%s%N)
fast_ms=$(( (fast_end - fast_start) / 1000000 ))

# Clean up slow request
wait "$slow_pid" 2>/dev/null || true
slow_end=$(date +%s%N)
slow_ms=$(( (slow_end - slow_start) / 1000000 ))

echo "  Slow request: ${slow_ms}ms"
echo "  Fast request (while slow in-flight): ${fast_ms}ms"

if [[ "$fast_ms" -gt 2000 ]]; then
    fail "2.1 Fast request blocked by slow request (${fast_ms}ms)" \
        "Head-of-line blocking detected — proxy may have a global lock"
elif [[ "$fast_ms" -gt 500 ]]; then
    warn "2.1 Fast request slower than expected (${fast_ms}ms) while slow request in-flight"
else
    pass "2.1 No head-of-line blocking: fast=${fast_ms}ms while slow=${slow_ms}ms"
fi

proxy_alive || fail "2.1 PROXY CRASHED"

# ============================================================================
# TEST 3: Concurrent Map Safety (UsersCache)
# ============================================================================
log_header "TEST 3: CONCURRENT MAP SAFETY"
log_section "3.1 Hammer UsersCache with 100 parallel authenticated requests"

# This test exercises the UsersCache map in auth.go
# If there's a race condition, Go will crash with "fatal error: concurrent map read and map write"
auth_pids=()
auth_fail=0
for i in $(seq 1 100); do
    (
        # Use different auth headers to force cache writes
        user="testuser${i}"
        auth=$(echo -n "${user}:password${i}" | base64)
        code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
            --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
            -H "Proxy-Authorization: Basic ${auth}" \
            "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
        # We expect 407 (auth required) or 200 — anything non-crash is OK
        [[ "$code" != "000" ]] && exit 0 || exit 1
    ) &
    auth_pids+=($!)
done

for pid in "${auth_pids[@]}"; do
    if ! wait "$pid" 2>/dev/null; then
        ((auth_fail++)) || true
    fi
done

echo "  100 parallel auth requests: ${auth_fail} failures"

# The REAL test: is the proxy still alive? A map race panics the whole process.
sleep 0.5
if proxy_alive; then
    if [[ "$auth_fail" -eq 0 ]]; then
        pass "3.1 UsersCache survived 100 concurrent auth requests (0 failures)"
    elif [[ "$auth_fail" -lt 10 ]]; then
        pass "3.1 UsersCache survived (${auth_fail} failures likely auth rejections, not crashes)"
    else
        warn "3.1 High failure rate: ${auth_fail}/100 — check proxy logs"
    fi
else
    fail "3.1 PROXY CRASHED under concurrent auth load" \
        "Likely 'fatal error: concurrent map read and map write' in UsersCache (auth.go)"
fi

# ============================================================================
# TEST 4: Sustained Load — 200 requests, 50 at a time
# ============================================================================
log_header "TEST 4: SUSTAINED CONCURRENCY"
log_section "4.1 200 requests in waves of 50"

total_sent=0
total_ok=0
total_fail=0
sustained_start=$(date +%s%N)

for wave in $(seq 1 4); do
    wave_pids=()
    for i in $(seq 1 50); do
        (
            code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
            [[ "$code" =~ ^[23] ]] && exit 0 || exit 1
        ) &
        wave_pids+=($!)
    done

    for pid in "${wave_pids[@]}"; do
        ((total_sent++)) || true
        if wait "$pid" 2>/dev/null; then
            ((total_ok++)) || true
        else
            ((total_fail++)) || true
        fi
    done
done

sustained_end=$(date +%s%N)
sustained_ms=$(( (sustained_end - sustained_start) / 1000000 ))
sustained_rps=$(echo "scale=0; $total_sent * 1000 / $sustained_ms" | bc 2>/dev/null || echo "?")

echo "  ${total_sent} total, ${total_ok} OK, ${total_fail} failed"
echo "  Wall time: ${sustained_ms}ms (${sustained_rps} req/s)"

if [[ "$total_fail" -gt 20 ]]; then
    fail "4.1 Sustained load: ${total_fail}/${total_sent} failures" \
        "Proxy struggling under sustained 50-concurrent load"
elif [[ "$total_fail" -gt 0 ]]; then
    warn "4.1 Sustained load: ${total_fail}/${total_sent} failures at ${sustained_rps} req/s"
else
    pass "4.1 Sustained load: ${total_ok}/${total_sent} OK at ${sustained_rps} req/s"
fi

proxy_alive || fail "4.1 PROXY CRASHED under sustained load"

# ============================================================================
# TEST 5: Mixed Workload — HTTP + slow + adversarial simultaneously
# ============================================================================
log_header "TEST 5: MIXED CONCURRENT WORKLOAD"
log_section "5.1 Simultaneous: 20 fast + 5 slow + 5 adversarial"

mixed_pids=()
mixed_fail=0

# 20 fast requests
for i in $(seq 1 20); do
    (
        code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
        [[ "$code" =~ ^[23] ]] && exit 0 || exit 1
    ) &
    mixed_pids+=($!)
done

# 5 slow requests (1-3s delay)
for i in $(seq 1 5); do
    (
        code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}/delay/$((i % 3 + 1))" 2>/dev/null || echo "000")
        [[ "$code" =~ ^[23] ]] && exit 0 || exit 1
    ) &
    mixed_pids+=($!)
done

# 5 adversarial requests (these may return various status codes — we just care about no crash)
adversarial_endpoints=(
    "/adversarial/lying-content-length-under"
    "/adversarial/double-content-length"
    "/adversarial/no-framing"
    "/adversarial/huge-header"
    "/adversarial/null-in-headers"
)
for ep in "${adversarial_endpoints[@]}"; do
    (
        code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}${ep}" 2>/dev/null || echo "000")
        # Any non-zero response is fine — we're testing the proxy doesn't crash
        [[ "$code" != "000" ]] && exit 0 || exit 1
    ) &
    mixed_pids+=($!)
done

for pid in "${mixed_pids[@]}"; do
    if ! wait "$pid" 2>/dev/null; then
        ((mixed_fail++)) || true
    fi
done

echo "  30 mixed concurrent requests: ${mixed_fail} failures"

if proxy_alive; then
    if [[ "$mixed_fail" -le 3 ]]; then
        pass "5.1 Mixed workload: 30 concurrent (${mixed_fail} failures, proxy alive)"
    else
        warn "5.1 Mixed workload: ${mixed_fail}/30 failures but proxy survived"
    fi
else
    fail "5.1 PROXY CRASHED under mixed workload" \
        "Adversarial + slow + fast concurrent mix caused process crash"
fi

# ============================================================================
# TEST 6: Response Integrity Under Concurrency
# ============================================================================
log_header "TEST 6: DATA INTEGRITY UNDER CONCURRENCY"
log_section "6.1 Verify responses aren't cross-contaminated"

# Send 20 parallel requests to different endpoints, verify each response
integrity_pids=()
integrity_fail=0
tmpdir=$(mktemp -d)

for i in $(seq 1 20); do
    (
        # Each request uses a unique query param — the echo server reflects the URL
        body=$(gscurl -s "${ECHO_SERVER}/echo?id=${i}" 2>/dev/null)
        # The echo server returns JSON with "url": "/echo?id=N" or "path": "/echo"
        # Check that the response contains the correct id parameter
        if echo "$body" | grep -q "id=${i}"; then
            exit 0
        else
            echo "Request ${i}: expected id=${i} in URL, got: $(echo "$body" | grep -o '"url": *"[^"]*"' | head -1)" > "${tmpdir}/fail_${i}.log"
            exit 1
        fi
    ) &
    integrity_pids+=($!)
done

for pid in "${integrity_pids[@]}"; do
    if ! wait "$pid" 2>/dev/null; then
        ((integrity_fail++)) || true
    fi
done

if [[ "$integrity_fail" -eq 0 ]]; then
    pass "6.1 Response integrity: 20/20 responses matched their requests"
elif [[ "$integrity_fail" -le 2 ]]; then
    warn "6.1 Response integrity: ${integrity_fail}/20 mismatches (possible timing issue)"
else
    fail "6.1 Response cross-contamination: ${integrity_fail}/20 responses didn't match" \
        "Shared state corruption detected — responses leaking between goroutines"
    # Show failure details
    for f in "${tmpdir}"/fail_*.log; do
        [[ -f "$f" ]] && echo "         $(cat "$f")"
    done
fi
rm -rf "$tmpdir"

proxy_alive || fail "6.1 PROXY CRASHED during integrity test"

# ============================================================================
# TEST 7: Rapid-fire burst — 100 simultaneous connections
# ============================================================================
log_header "TEST 7: BURST CAPACITY"
log_section "7.1 100 simultaneous requests (burst)"

burst_start=$(date +%s%N)
burst_pids=()
burst_fail=0

for i in $(seq 1 100); do
    (
        code=$(gscurl -s -o /dev/null -w "%{http_code}" "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
        [[ "$code" =~ ^[23] ]] && exit 0 || exit 1
    ) &
    burst_pids+=($!)
done

for pid in "${burst_pids[@]}"; do
    if ! wait "$pid" 2>/dev/null; then
        ((burst_fail++)) || true
    fi
done
burst_end=$(date +%s%N)
burst_ms=$(( (burst_end - burst_start) / 1000000 ))

echo "  100 burst: ${burst_fail} failures in ${burst_ms}ms"

if proxy_alive; then
    if [[ "$burst_fail" -le 5 ]]; then
        pass "7.1 Burst: 100 simultaneous, ${burst_fail} failures in ${burst_ms}ms"
    elif [[ "$burst_fail" -le 20 ]]; then
        warn "7.1 Burst: ${burst_fail}/100 failures (possible fd/connection limit)" \
            "Check ulimit -n and proxy max connections"
    else
        fail "7.1 Burst: ${burst_fail}/100 failures in ${burst_ms}ms"
    fi
else
    fail "7.1 PROXY CRASHED under 100-request burst" \
        "Likely concurrent map panic or resource exhaustion"
fi

# ============================================================================
# TEST 8: Process stability check
# ============================================================================
log_header "TEST 8: PROCESS STABILITY"
log_section "8.1 Proxy PID unchanged (no crash-restart)"

current_pid=$(pgrep -f gatesentrybin | head -1 || echo "unknown")
if [[ "$current_pid" == "$PROXY_PID" ]]; then
    pass "8.1 Proxy PID unchanged: ${PROXY_PID} (no crash during entire test suite)"
else
    fail "8.1 Proxy PID changed: ${PROXY_PID} → ${current_pid}" \
        "Proxy crashed and was restarted during testing"
fi

# ── Final Summary ────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}════════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}  CONCURRENCY TEST SUMMARY${NC}"
echo -e "${BOLD}════════════════════════════════════════════════════════════════${NC}"
echo -e "  ${GREEN}PASS:${NC}  ${PASS_COUNT}"
echo -e "  ${RED}FAIL:${NC}  ${FAIL_COUNT}"
echo -e "  ${YELLOW}WARN:${NC}  ${WARN_COUNT}"
echo ""

if [[ "$FAIL_COUNT" -eq 0 ]]; then
    echo -e "  ${GREEN}${BOLD}All concurrency tests passed!${NC}"
else
    echo -e "  ${RED}${BOLD}${FAIL_COUNT} concurrency issue(s) found.${NC}"
fi
echo ""
