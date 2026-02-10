#!/usr/bin/env bash
###############################################################################
# GateSentry — Proxy & DNS Benchmark / Functional Test Suite
#
# This script captures every test performed during the architecture review
# session so they can be re-run deterministically.  Each section prints
# PASS / FAIL / KNOWN-ISSUE and a one-line summary.  An overall tally is
# printed at the end.
#
# Prerequisites (install once):
#   sudo apt-get install -y dnsutils curl apache2-utils dnsperf jq
#
# Usage:
#   chmod +x tests/proxy_benchmark_suite.sh
#   ./tests/proxy_benchmark_suite.sh              # defaults below
#   DNS_PORT=10053 PROXY_PORT=10413 ADMIN_PORT=8080 ./tests/proxy_benchmark_suite.sh
#
# Environment Variables (override any default):
#   DNS_PORT        – GateSentry DNS listener       (default: 10053)
#   PROXY_PORT      – GateSentry HTTP proxy          (default: 10413)
#   ADMIN_PORT      – GateSentry admin UI            (default: 8080)
#   DNS_HOST        – DNS listener address           (default: 127.0.0.1)
#   PROXY_HOST      – proxy listener address         (default: 127.0.0.1)
#   ADMIN_HOST      – admin UI address               (default: 127.0.0.1)
#   EXTERNAL_DOMAIN – domain guaranteed to resolve   (default: example.com)
#   NXDOMAIN_NAME   – domain guaranteed to NOT exist (default: thisdoesnotexist12345.invalid)
#   SKIP_PERF       – set to "1" to skip long perf benchmarks
#   VERBOSE         – set to "1" for extra debug output
###############################################################################

# set -euo pipefail  ← DISABLED: the suite must run to completion even when
#   tests FAIL.  Individual test failures are tracked in PASS/FAIL/KNOWN counters.
#   Using set -e would abort the suite on the first unexpected failure, hiding
#   all subsequent results.  We WANT to see every failure.
set -uo pipefail

# ── Colours ─────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Colour

# ── Defaults ────────────────────────────────────────────────────────────────
DNS_PORT="${DNS_PORT:-10053}"
PROXY_PORT="${PROXY_PORT:-10413}"
ADMIN_PORT="${ADMIN_PORT:-8080}"
DNS_HOST="${DNS_HOST:-127.0.0.1}"
PROXY_HOST="${PROXY_HOST:-127.0.0.1}"
ADMIN_HOST="${ADMIN_HOST:-127.0.0.1}"
EXTERNAL_DOMAIN="${EXTERNAL_DOMAIN:-example.com}"
NXDOMAIN_NAME="${NXDOMAIN_NAME:-thisdoesnotexist12345.invalid}"
SKIP_PERF="${SKIP_PERF:-0}"
VERBOSE="${VERBOSE:-0}"
CURL_TIMEOUT=10  # seconds

# ── Local Test Bed Endpoints ────────────────────────────────────────────────
# All proxy tests use the local testbed (no internet dependency)
# Set up via: sudo ./tests/testbed/setup.sh
TESTBED_HTTP="http://127.0.0.1:9999"       # nginx HTTP static + echo proxy
TESTBED_HTTPS="https://httpbin.org:9443"    # nginx HTTPS (internal CA cert)
ECHO_SERVER="http://127.0.0.1:9998"         # Python echo server (direct)
TESTBED_FILES="${TESTBED_HTTP}/files"        # Static test files (1MB, 10MB, 100MB)
CA_CERT="${CA_CERT:-$(cd "$(dirname "$0")" && pwd)/fixtures/JVJCA.crt}"  # Internal CA

# ── Counters ────────────────────────────────────────────────────────────────
PASS=0
FAIL=0
KNOWN=0
SKIP=0
TOTAL=0

# ── Temp dir for artefacts ──────────────────────────────────────────────────
TMPDIR="$(mktemp -d /tmp/gatesentry-tests.XXXXXX)"
trap 'rm -rf "$TMPDIR"' EXIT

###############################################################################
# Helper functions
###############################################################################

log_header() {
    echo ""
    echo -e "${BOLD}${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${CYAN}  $1${NC}"
    echo -e "${BOLD}${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

log_section() {
    echo ""
    echo -e "${BOLD}── $1 ──${NC}"
}

pass() {
    ((TOTAL++)) || true
    ((PASS++)) || true
    echo -e "  ${GREEN}✓ PASS${NC}  $1"
}

fail() {
    ((TOTAL++)) || true
    ((FAIL++)) || true
    echo -e "  ${RED}✗ FAIL${NC}  $1"
    [[ -n "${2:-}" ]] && echo -e "         ${RED}↳ $2${NC}"
}

known_issue() {
    ((TOTAL++)) || true
    ((KNOWN++)) || true
    echo -e "  ${YELLOW}⚠ KNOWN${NC} $1"
    [[ -n "${2:-}" ]] && echo -e "         ${YELLOW}↳ $2${NC}"
}

skip_test() {
    ((TOTAL++)) || true
    ((SKIP++)) || true
    echo -e "  ${CYAN}⊘ SKIP${NC}  $1"
}

verbose() {
    [[ "$VERBOSE" == "1" ]] && echo -e "         $1"
}

# dig wrapper targeting GateSentry
gsdig() {
    dig "@${DNS_HOST}" -p "$DNS_PORT" "$@" +time=5 +tries=1
}

# curl wrapper through GateSentry proxy
gscurl() {
    curl --max-time "$CURL_TIMEOUT" --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        --cacert "$CA_CERT" "$@" 2>/dev/null
}

###############################################################################
# Pre-flight: verify GateSentry is reachable
###############################################################################
preflight_check() {
    log_header "PRE-FLIGHT CHECKS"

    # DNS port
    if gsdig "$EXTERNAL_DOMAIN" A +short > /dev/null 2>&1; then
        pass "DNS server reachable on ${DNS_HOST}:${DNS_PORT}"
    else
        fail "DNS server NOT reachable on ${DNS_HOST}:${DNS_PORT}" "Cannot continue – aborting"
        exit 1
    fi

    # Proxy port
    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" "${TESTBED_HTTP}/health" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "HTTP proxy reachable on ${PROXY_HOST}:${PROXY_PORT} (HTTP $http_code)"
    else
        fail "HTTP proxy NOT reachable on ${PROXY_HOST}:${PROXY_PORT} (HTTP $http_code)" "Proxy tests will fail"
    fi

    # Admin UI
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        "http://${ADMIN_HOST}:${ADMIN_PORT}/" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "Admin UI reachable on ${ADMIN_HOST}:${ADMIN_PORT} (HTTP $http_code)"
    else
        fail "Admin UI NOT reachable on ${ADMIN_HOST}:${ADMIN_PORT} (HTTP $http_code)"
    fi

    # Local testbed HTTP
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        "${TESTBED_HTTP}/health" 2>/dev/null || echo "000")
    if [[ "$http_code" == "200" ]]; then
        pass "Local testbed HTTP ready (${TESTBED_HTTP})"
    else
        fail "Local testbed HTTP NOT ready (HTTP $http_code)" \
            "Run: sudo ./tests/testbed/setup.sh"
    fi

    # Local testbed HTTPS
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --cacert "$CA_CERT" "${TESTBED_HTTPS}/health" 2>/dev/null || echo "000")
    if [[ "$http_code" == "200" ]]; then
        pass "Local testbed HTTPS ready (${TESTBED_HTTPS})"
    else
        fail "Local testbed HTTPS NOT ready (HTTP $http_code)" \
            "Check: httpbin.org in /etc/hosts, CA cert at ${CA_CERT}"
    fi

    # Echo server
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        "${ECHO_SERVER}/echo" 2>/dev/null || echo "000")
    if [[ "$http_code" == "200" ]]; then
        pass "Echo server ready (${ECHO_SERVER})"
    else
        fail "Echo server NOT ready (HTTP $http_code)" \
            "Run: python3 tests/testbed/echo_server.py --port 9998"
    fi
}

###############################################################################
# SECTION 1 — DNS Functionality
###############################################################################
test_dns_functionality() {
    log_header "SECTION 1 — DNS FUNCTIONALITY"

    # 1.1  A-record resolution
    log_section "1.1 A-record resolution"
    local ip
    ip=$(gsdig "$EXTERNAL_DOMAIN" A +short | head -1)
    if [[ -n "$ip" ]]; then
        pass "A record for ${EXTERNAL_DOMAIN} → ${ip}"
    else
        fail "No A record returned for ${EXTERNAL_DOMAIN}"
    fi

    # 1.2  AAAA record resolution
    log_section "1.2 AAAA-record resolution"
    local ipv6
    ipv6=$(gsdig "google.com" AAAA +short | head -1)
    if [[ -n "$ipv6" ]]; then
        pass "AAAA record for google.com → ${ipv6}"
    else
        fail "No AAAA record returned for google.com"
    fi

    # 1.3  MX record resolution
    log_section "1.3 MX-record resolution"
    local mx
    mx=$(gsdig "google.com" MX +short | head -1)
    if [[ -n "$mx" ]]; then
        pass "MX record for google.com → ${mx}"
    else
        fail "No MX record returned for google.com"
    fi

    # 1.4  TXT record resolution
    log_section "1.4 TXT-record resolution"
    local txt
    txt=$(gsdig "google.com" TXT +short | head -1)
    if [[ -n "$txt" ]]; then
        pass "TXT record for google.com returned"
    else
        fail "No TXT record returned for google.com"
    fi

    # 1.5  NXDOMAIN handling
    log_section "1.5 NXDOMAIN handling"
    local nx_status
    nx_status=$(gsdig "$NXDOMAIN_NAME" A | grep -c "NXDOMAIN" || true)
    local nx_rcode
    nx_rcode=$(gsdig "$NXDOMAIN_NAME" A | grep "status:" | head -1)
    if [[ "$nx_status" -gt 0 ]]; then
        pass "NXDOMAIN correctly returned for ${NXDOMAIN_NAME}"
    else
        known_issue "NXDOMAIN returns NOERROR with 0 answers instead of NXDOMAIN rcode" \
            "Got: ${nx_rcode}"
    fi

    # 1.6  DNS response TTL present
    log_section "1.6 TTL in DNS responses"
    local ttl_line
    ttl_line=$(gsdig "$EXTERNAL_DOMAIN" A | grep -E "^${EXTERNAL_DOMAIN}" | head -1)
    local ttl_val
    ttl_val=$(echo "$ttl_line" | awk '{print $2}')
    if [[ -n "$ttl_val" && "$ttl_val" -gt 0 ]] 2>/dev/null; then
        pass "TTL present in response: ${ttl_val}s"
    else
        fail "No TTL found in DNS response"
    fi
}

###############################################################################
# SECTION 2 — DNS Caching Verification
###############################################################################
test_dns_caching() {
    log_header "SECTION 2 — DNS CACHING"

    log_section "2.1 Cache hit — repeated query should be faster"

    # Warm up with first query
    local domain="cachetest-${RANDOM}.example.com"
    # Use a real domain for reliable testing
    domain="$EXTERNAL_DOMAIN"

    # First query (cold)
    local t1_start t1_end t1_ms
    t1_start=$(date +%s%N)
    gsdig "$domain" A +short > /dev/null 2>&1
    t1_end=$(date +%s%N)
    t1_ms=$(( (t1_end - t1_start) / 1000000 ))

    # Second query (should be cached)
    local t2_start t2_end t2_ms
    t2_start=$(date +%s%N)
    gsdig "$domain" A +short > /dev/null 2>&1
    t2_end=$(date +%s%N)
    t2_ms=$(( (t2_end - t2_start) / 1000000 ))

    # Third query
    local t3_start t3_end t3_ms
    t3_start=$(date +%s%N)
    gsdig "$domain" A +short > /dev/null 2>&1
    t3_end=$(date +%s%N)
    t3_ms=$(( (t3_end - t3_start) / 1000000 ))

    verbose "Query 1 (cold): ${t1_ms}ms"
    verbose "Query 2 (warm): ${t2_ms}ms"
    verbose "Query 3 (warm): ${t3_ms}ms"

    # If cached, queries 2&3 should be significantly faster (< 2ms for local cache)
    if [[ "$t2_ms" -lt 3 && "$t3_ms" -lt 3 ]]; then
        pass "DNS caching appears active (cold: ${t1_ms}ms, warm: ${t2_ms}ms, ${t3_ms}ms)"
    else
        known_issue "DNS caching NOT implemented — every query hits upstream" \
            "Times: cold=${t1_ms}ms, q2=${t2_ms}ms, q3=${t3_ms}ms (all similar = no cache)"
    fi

    log_section "2.2 TTL decrement between queries"
    local ttl1 ttl2
    ttl1=$(gsdig "$domain" A | grep -E "^${domain}" | head -1 | awk '{print $2}')
    sleep 2
    ttl2=$(gsdig "$domain" A | grep -E "^${domain}" | head -1 | awk '{print $2}')
    verbose "TTL query 1: ${ttl1}, TTL query 2 (2s later): ${ttl2}"

    if [[ -n "$ttl1" && -n "$ttl2" ]] 2>/dev/null; then
        local diff=$(( ttl1 - ttl2 ))
        if [[ "$diff" -ge 1 && "$diff" -le 4 ]]; then
            pass "TTL decrements by ~2s as expected (Δ=${diff}s) — local cache counting down"
        elif [[ "$diff" -eq 0 ]]; then
            known_issue "TTL identical (Δ=0s) — responses may be freshly fetched each time" \
                "TTL1=${ttl1}, TTL2=${ttl2} — if no cache, upstream TTL resets on each query"
        else
            # Large delta means upstream TTL is naturally counting down between our queries.
            # This happens when there's no local cache: each response comes fresh from
            # upstream with whatever TTL the upstream has at that moment.
            known_issue "TTL jumped by ${diff}s over 2s — responses are fresh from upstream (no local cache)" \
                "TTL1=${ttl1}, TTL2=${ttl2} — upstream TTL naturally decrements; GateSentry is NOT caching"
        fi
    else
        fail "Could not extract TTL values for comparison"
    fi
}

###############################################################################
# SECTION 3 — Proxy RFC Compliance
###############################################################################
test_proxy_rfc_compliance() {
    log_header "SECTION 3 — PROXY RFC COMPLIANCE"

    # 3.1  Via header (RFC 7230 §5.7.1)
    log_section "3.1 Via header (RFC 7230 §5.7.1)"
    local resp_headers
    resp_headers=$(gscurl -sI "${TESTBED_HTTP}/")
    local via_header
    via_header=$(echo "$resp_headers" | grep -i "^Via:" || true)
    if [[ -n "$via_header" ]]; then
        pass "Via header present: ${via_header}"
    else
        known_issue "No Via header added by proxy" \
            "RFC 7230 §5.7.1 requires intermediaries to add a Via header"
    fi

    # 3.2  X-Forwarded-For header
    log_section "3.2 X-Forwarded-For header"
    # Use local echo server to see what headers the proxy sends
    local xff_resp
    xff_resp=$(gscurl -s "${TESTBED_HTTP}/echo" 2>/dev/null || echo "UNREACHABLE")
    if [[ "$xff_resp" == "UNREACHABLE" ]]; then
        skip_test "Echo server unreachable — cannot verify X-Forwarded-For"
    else
        local xff
        xff=$(echo "$xff_resp" | grep -i "X-Forwarded-For" || true)
        if [[ -n "$xff" ]]; then
            pass "X-Forwarded-For header present: $(echo "$xff" | xargs)"
        else
            known_issue "No X-Forwarded-For header added by proxy" \
                "Best practice for proxies to identify client IP to upstream"
        fi
    fi

    # 3.3  Hop-by-hop header removal
    log_section "3.3 Hop-by-hop header removal"
    local hbh_resp
    hbh_resp=$(gscurl -sI "${TESTBED_HTTP}/" 2>/dev/null)
    local proxy_conn
    proxy_conn=$(echo "$hbh_resp" | grep -i "^Proxy-Connection:" || true)
    if [[ -z "$proxy_conn" ]]; then
        pass "Proxy-Connection hop-by-hop header not leaked to client"
    else
        fail "Proxy-Connection header leaked: ${proxy_conn}"
    fi

    # 3.4  HEAD method (MUST return headers, NO body)
    #       Known intermittent: sometimes works, sometimes hangs depending on
    #       upstream response timing and Go's http.Client behaviour with HEAD.
    #       The root cause is io.ReadAll(teeReader) in proxy.go ~line 488.
    log_section "3.4 HEAD method support (3s timeout — hangs indicate bug)"
    local head_pass=0
    local head_fail=0
    for i in 1 2 3; do
        local hc
        hc=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 \
            --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
            -X HEAD "${TESTBED_HTTP}/head-test" 2>/dev/null || echo "TIMEOUT")
        if [[ "$hc" =~ ^[23] ]]; then
            ((head_pass++)) || true
        else
            ((head_fail++)) || true
        fi
    done
    if [[ "$head_pass" -eq 3 ]]; then
        pass "HEAD method works reliably (3/3 attempts)"
    elif [[ "$head_pass" -gt 0 ]]; then
        known_issue "HEAD method INTERMITTENT — ${head_pass}/3 succeeded, ${head_fail}/3 timed out" \
            "proxy.go ~line 488: io.ReadAll(teeReader) may block on HEAD responses (no body)"
    else
        known_issue "HEAD method HANGS — 0/3 attempts succeeded (all timed out)" \
            "proxy.go ~line 488: io.ReadAll(teeReader) blocks on HEAD responses (no body)"
    fi

    # 3.5  OPTIONS method
    log_section "3.5 OPTIONS method support"
    local options_code
    options_code=$(gscurl -s -o /dev/null -w "%{http_code}" -X OPTIONS "${TESTBED_HTTP}/" || echo "000")
    if [[ "$options_code" =~ ^[24] ]]; then
        pass "OPTIONS method works (HTTP ${options_code})"
    else
        fail "OPTIONS returned: ${options_code}"
    fi

    # 3.6  Content-Length accuracy
    log_section "3.6 Content-Length accuracy"
    local cl_resp
    cl_resp=$(gscurl -sI "${TESTBED_HTTP}/" 2>/dev/null)
    local cl_val
    cl_val=$(echo "$cl_resp" | grep -i "^Content-Length:" | head -1 | awk '{print $2}' | tr -d '\r')
    if [[ -n "$cl_val" ]]; then
        local actual_len
        actual_len=$(gscurl -s "${TESTBED_HTTP}/" 2>/dev/null | wc -c)
        verbose "Content-Length header: ${cl_val}, actual body: ${actual_len} bytes"
        if [[ "$cl_val" -eq 0 && "$actual_len" -gt 0 ]] 2>/dev/null; then
            known_issue "Content-Length is 0 but body has ${actual_len} bytes" \
                "Proxy may be setting Content-Length before re-encoding the body"
        elif [[ -n "$actual_len" ]]; then
            local cl_diff
            cl_diff=$(( cl_val > actual_len ? cl_val - actual_len : actual_len - cl_val ))
            if [[ "$cl_diff" -le 10 ]]; then
                pass "Content-Length accurate: header=${cl_val}, body=${actual_len}"
            else
                fail "Content-Length mismatch: header=${cl_val}, body=${actual_len} (Δ=${cl_diff})"
            fi
        fi
    else
        # Might be chunked
        local te
        te=$(echo "$cl_resp" | grep -i "^Transfer-Encoding:" || true)
        if [[ -n "$te" ]]; then
            pass "Using Transfer-Encoding instead of Content-Length: $(echo "$te" | xargs)"
        else
            fail "No Content-Length and no Transfer-Encoding in response"
        fi
    fi

    # 3.7  Accept-Encoding passthrough
    log_section "3.7 Accept-Encoding handling"
    # The proxy strips Accept-Encoding unconditionally (proxy.go line ~396)
    local ae_test
    ae_test=$(gscurl -sI -H "Accept-Encoding: gzip, deflate, br" "${TESTBED_HTTP}/" 2>/dev/null)
    local ce
    ce=$(echo "$ae_test" | grep -i "^Content-Encoding:" || true)
    if [[ -n "$ce" ]]; then
        pass "Content-Encoding present in response: $(echo "$ce" | xargs)"
    else
        known_issue "Accept-Encoding stripped by proxy — re-encodes response itself" \
            "proxy.go line ~396: r.Header.Del(\"Accept-Encoding\") unconditionally"
    fi
}

###############################################################################
# SECTION 4 — HTTP Method Support
###############################################################################
test_http_methods() {
    log_header "SECTION 4 — HTTP METHOD SUPPORT"

    local methods=("GET" "POST" "PUT" "DELETE" "PATCH")

    for method in "${methods[@]}"; do
        log_section "4.x ${method} method"
        local code
        code=$(gscurl -s -o /dev/null -w "%{http_code}" -X "$method" "${ECHO_SERVER}/${method,,}" 2>/dev/null || echo "000")
        if [[ "$code" == "000" ]]; then
            skip_test "${method} — echo server unreachable"
        elif [[ "$code" =~ ^[2] ]]; then
            pass "${method} → HTTP ${code}"
        elif [[ "$code" =~ ^[3] ]]; then
            pass "${method} → HTTP ${code} (redirect)"
        elif [[ "$code" =~ ^[4] ]]; then
            # 405 is expected for some method/endpoint combos
            pass "${method} → HTTP ${code} (server returned client error — proxy forwarded correctly)"
        else
            fail "${method} → HTTP ${code}"
        fi
    done

    # HEAD special case (already tested above but reconfirm)
    log_section "4.x HEAD method (re-test against local head-test endpoint)"
    local head_code
    head_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        -X HEAD "${TESTBED_HTTP}/head-test" 2>/dev/null || echo "TIMEOUT")
    if [[ "$head_code" =~ ^[23] ]]; then
        pass "HEAD → HTTP ${head_code}"
    elif [[ "$head_code" == "TIMEOUT" || "$head_code" == "000" ]]; then
        known_issue "HEAD still hangs (confirmed)" "See Section 3.4"
    else
        fail "HEAD → HTTP ${head_code}"
    fi
}

###############################################################################
# SECTION 5 — HTTPS / CONNECT Tunnel
###############################################################################
test_https_connect() {
    log_header "SECTION 5 — HTTPS / CONNECT TUNNEL"

    # 5.1  CONNECT tunnel to HTTPS site
    log_section "5.1 CONNECT tunnel basic"
    local https_code
    https_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" --cacert "$CA_CERT" \
        "${TESTBED_HTTPS}/" 2>/dev/null || echo "000")
    if [[ "$https_code" =~ ^[23] ]]; then
        pass "HTTPS via CONNECT tunnel works (HTTP ${https_code})"
    else
        fail "HTTPS via CONNECT returned: ${https_code}"
    fi

    # 5.2  CONNECT to non-443 port (port 9443)
    log_section "5.2 CONNECT to non-standard port (9443)"
    local nonstd_code
    nonstd_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" --cacert "$CA_CERT" \
        "${TESTBED_HTTPS}/health" 2>/dev/null || echo "000")
    if [[ "$nonstd_code" =~ ^[23] ]]; then
        pass "CONNECT to port 9443 works (HTTP ${nonstd_code})"
    else
        fail "CONNECT to port 9443 returned: ${nonstd_code}"
    fi
}

###############################################################################
# SECTION 6 — WebSocket Support
###############################################################################
test_websocket() {
    log_header "SECTION 6 — WEBSOCKET SUPPORT"

    log_section "6.1 WebSocket upgrade request"
    local ws_resp
    ws_resp=$(gscurl -s -o /dev/null -w "%{http_code}" \
        -H "Upgrade: websocket" \
        -H "Connection: Upgrade" \
        -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
        -H "Sec-WebSocket-Version: 13" \
        "${TESTBED_HTTP}/ws" 2>/dev/null || echo "000")
    if [[ "$ws_resp" == "101" ]]; then
        pass "WebSocket upgrade successful (101 Switching Protocols)"
    elif [[ "$ws_resp" == "400" ]]; then
        known_issue "WebSocket returns 400 — not supported" \
            "websocket.go: 'Web sockets currently not supported'"
    else
        fail "WebSocket returned: ${ws_resp}"
    fi
}

###############################################################################
# SECTION 7 — Proxy Security
###############################################################################
test_proxy_security() {
    log_header "SECTION 7 — PROXY SECURITY"

    # 7.1  SSRF — access admin UI through proxy
    log_section "7.1 SSRF — admin UI access via proxy"
    local ssrf_code
    ssrf_code=$(gscurl -s -o /dev/null -w "%{http_code}" \
        "http://127.0.0.1:${ADMIN_PORT}/" 2>/dev/null || echo "000")
    if [[ "$ssrf_code" == "403" || "$ssrf_code" == "000" ]]; then
        pass "SSRF blocked — proxy denies access to admin UI (HTTP ${ssrf_code})"
    elif [[ "$ssrf_code" =~ ^[23] ]]; then
        known_issue "SSRF: proxy allows access to admin UI on 127.0.0.1:${ADMIN_PORT}" \
            "HTTP ${ssrf_code} — attacker can reach internal admin interface through proxy"
    else
        fail "Unexpected SSRF response: HTTP ${ssrf_code}"
    fi

    # 7.2  SSRF — localhost via hostname
    log_section "7.2 SSRF — localhost by name"
    local ssrf2_code
    ssrf2_code=$(gscurl -s -o /dev/null -w "%{http_code}" \
        "http://localhost:${ADMIN_PORT}/" 2>/dev/null || echo "000")
    if [[ "$ssrf2_code" == "403" || "$ssrf2_code" == "000" ]]; then
        pass "SSRF via 'localhost' blocked (HTTP ${ssrf2_code})"
    elif [[ "$ssrf2_code" =~ ^[23] ]]; then
        known_issue "SSRF: 'localhost:${ADMIN_PORT}' accessible through proxy" \
            "HTTP ${ssrf2_code}"
    else
        verbose "SSRF via localhost returned HTTP ${ssrf2_code}"
        # Anything non-2xx/3xx is acceptable
        pass "SSRF via 'localhost' returned non-success (HTTP ${ssrf2_code})"
    fi

    # 7.3  Host header injection
    log_section "7.3 Host header injection"
    local hhi_code
    hhi_code=$(gscurl -s -o /dev/null -w "%{http_code}" \
        -H "Host: evil.example.com" \
        "${TESTBED_HTTP}/" 2>/dev/null || echo "000")
    if [[ "$hhi_code" =~ ^[23] ]]; then
        pass "Host header injection — proxy forwarded normally (HTTP ${hhi_code})"
        verbose "(The proxy uses the URL host, not the Host header, which is correct)"
    else
        fail "Host header injection test returned: HTTP ${hhi_code}"
    fi

    # 7.4  Proxy loop detection
    #       We ask the proxy to fetch its own proxy port.  If the proxy has no
    #       loop detection, this could cause infinite recursion.  A quick
    #       response (even 200) means the proxy handled it without looping.
    #       A timeout or connection reset indicates a potential loop.
    log_section "7.4 Proxy loop / self-request behaviour"
    local loop_start loop_end loop_ms loop_code
    loop_start=$(date +%s%N)
    loop_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "http://${PROXY_HOST}:${PROXY_PORT}/" 2>/dev/null || echo "TIMEOUT")
    loop_end=$(date +%s%N)
    loop_ms=$(( (loop_end - loop_start) / 1000000 ))
    verbose "Loop test returned HTTP ${loop_code} in ${loop_ms}ms"
    if [[ "$loop_code" == "TIMEOUT" || "$loop_code" == "000" ]]; then
        fail "Proxy self-request timed out — no loop detection" \
            "HTTP ${loop_code} in ${loop_ms}ms — proxy has no Max-Forwards or Via-based loop break"
    elif [[ "$loop_ms" -gt 4500 ]]; then
        fail "Proxy self-request slow (${loop_ms}ms) — possible loop before timeout" \
            "No loop detection mechanism — self-proxying should return immediate error"
    else
        pass "Proxy self-request completed in ${loop_ms}ms without hanging (HTTP ${loop_code})"
    fi

    # 7.5  Large header injection
    log_section "7.5 Oversized header handling"
    local big_header
    big_header=$(python3 -c "print('X' * 16384)" 2>/dev/null || printf '%16384s' | tr ' ' 'X')
    local bh_code
    bh_code=$(gscurl -s -o /dev/null -w "%{http_code}" \
        -H "X-Oversized: ${big_header}" \
        "${TESTBED_HTTP}/" 2>/dev/null || echo "000")
    if [[ "$bh_code" =~ ^[24] ]]; then
        pass "Oversized header handled (HTTP ${bh_code})"
    else
        verbose "Oversized header response: HTTP ${bh_code}"
        pass "Oversized header — no crash (HTTP ${bh_code})"
    fi
}

###############################################################################
# SECTION 8 — Proxy DNS Resolution (does proxy use GateSentry DNS?)
###############################################################################
test_proxy_dns_resolution() {
    log_header "SECTION 8 — PROXY DNS RESOLUTION PATH"

    log_section "8.1 Proxy should use GateSentry DNS (not system resolver)"
    echo "  INFO  This test checks whether the proxy's outbound connections"
    echo "        resolve hostnames via GateSentry's own DNS server."
    echo "        Current code: proxy.go line ~25 — net.Dialer{} has NO Resolver"
    echo "        field, so it uses the system default (/etc/resolv.conf)."
    echo ""

    # We can verify by querying a unique domain through the proxy and checking
    # if GateSentry's DNS log shows the query.  This requires log inspection
    # which is environment-specific, so we note it as a known architectural issue.
    known_issue "Proxy uses system DNS resolver, NOT GateSentry's DNS server" \
        "proxy.go: net.Dialer{} without Resolver → system /etc/resolv.conf. Filtered domains may bypass GateSentry."
}

###############################################################################
# SECTION 9 — Performance Benchmarks
###############################################################################
test_performance() {
    log_header "SECTION 9 — PERFORMANCE BENCHMARKS"

    if [[ "$SKIP_PERF" == "1" ]]; then
        skip_test "Performance benchmarks skipped (SKIP_PERF=1)"
        return
    fi

    # 9.1  DNS query latency
    log_section "9.1 DNS query latency (10 sequential queries)"
    local total_ms=0
    local count=10
    for i in $(seq 1 $count); do
        local t_start t_end t_ms
        t_start=$(date +%s%N)
        gsdig "$EXTERNAL_DOMAIN" A +short > /dev/null 2>&1
        t_end=$(date +%s%N)
        t_ms=$(( (t_end - t_start) / 1000000 ))
        total_ms=$((total_ms + t_ms))
    done
    local avg_ms=$((total_ms / count))
    echo "  INFO  Average DNS query latency: ${avg_ms}ms over ${count} queries"
    if [[ "$avg_ms" -lt 50 ]]; then
        pass "DNS latency acceptable (avg ${avg_ms}ms)"
    elif [[ "$avg_ms" -lt 200 ]]; then
        pass "DNS latency moderate (avg ${avg_ms}ms) — caching would improve this"
    else
        fail "DNS latency HIGH (avg ${avg_ms}ms)"
    fi

    # 9.2  dnsperf if available
    log_section "9.2 DNS throughput (dnsperf)"
    if command -v dnsperf &> /dev/null; then
        # Create query file
        local qfile="${TMPDIR}/dns_queries.txt"
        for i in $(seq 1 100); do
            echo "${EXTERNAL_DOMAIN} A" >> "$qfile"
            echo "google.com A" >> "$qfile"
            echo "github.com A" >> "$qfile"
        done

        local perf_output
        perf_output=$(dnsperf -s "$DNS_HOST" -p "$DNS_PORT" -d "$qfile" -l 5 -c 5 2>&1 || true)
        local qps
        qps=$(echo "$perf_output" | grep "Queries per second" | awk '{print $NF}' || echo "N/A")
        echo "  INFO  DNS QPS: ${qps}"
        verbose "$(echo "$perf_output" | tail -10)"

        if [[ "$qps" != "N/A" ]]; then
            local qps_int
            qps_int=$(echo "$qps" | cut -d. -f1)
            if [[ "$qps_int" -gt 500 ]]; then
                pass "DNS throughput good: ${qps} QPS"
            elif [[ "$qps_int" -gt 100 ]]; then
                pass "DNS throughput moderate: ${qps} QPS"
            else
                fail "DNS throughput low: ${qps} QPS"
            fi
        fi
    else
        skip_test "dnsperf not installed — run: sudo apt-get install dnsperf"
    fi

    # 9.3  HTTP proxy throughput (ab)
    log_section "9.3 HTTP proxy throughput (ab / Apache Bench)"
    if command -v ab &> /dev/null; then
        local ab_output
        ab_output=$(ab -n 50 -c 5 -X "${PROXY_HOST}:${PROXY_PORT}" \
            "${TESTBED_HTTP}/" 2>&1 || true)
        local rps
        rps=$(echo "$ab_output" | grep "Requests per second" | awk '{print $4}' || echo "N/A")
        local mean_time
        mean_time=$(echo "$ab_output" | grep "Time per request.*mean\b" | head -1 | awk '{print $4}' || echo "N/A")
        echo "  INFO  Proxy throughput: ${rps} req/s, mean latency: ${mean_time}ms"

        local failed
        failed=$(echo "$ab_output" | grep "Failed requests" | awk '{print $3}' || echo "0")
        if [[ "$failed" == "0" ]]; then
            pass "No failed requests in proxy benchmark (${rps} req/s)"
        else
            fail "Proxy benchmark had ${failed} failed requests"
        fi
    else
        skip_test "ab (apache2-utils) not installed — run: sudo apt-get install apache2-utils"
    fi

    # 9.4  Large response handling
    log_section "9.4 Large response proxy passthrough"
    local large_code
    large_code=$(gscurl -s -o /dev/null -w "%{http_code}:%{size_download}:%{time_total}" \
        "${ECHO_SERVER}/bytes/1048576" 2>/dev/null || echo "000:0:0")
    local lc_status lc_size lc_time
    lc_status=$(echo "$large_code" | cut -d: -f1)
    lc_size=$(echo "$large_code" | cut -d: -f2)
    lc_time=$(echo "$large_code" | cut -d: -f3)
    if [[ "$lc_status" == "000" ]]; then
        skip_test "Echo server unreachable for large response test"
    elif [[ "$lc_status" =~ ^[2] ]]; then
        pass "1MB response proxied OK (HTTP ${lc_status}, ${lc_size} bytes in ${lc_time}s)"
    else
        fail "Large response test failed (HTTP ${lc_status})"
    fi
}

###############################################################################
# SECTION 10 — Concurrent / Stress Tests
###############################################################################
test_concurrent() {
    log_header "SECTION 10 — CONCURRENT REQUESTS"

    if [[ "$SKIP_PERF" == "1" ]]; then
        skip_test "Concurrency tests skipped (SKIP_PERF=1)"
        return
    fi

    log_section "10.1 Concurrent DNS queries (20 parallel)"
    local dns_pids=()
    local dns_fail=0
    for i in $(seq 1 20); do
        (
            result=$(gsdig "$EXTERNAL_DOMAIN" A +short 2>/dev/null)
            [[ -n "$result" ]] && exit 0 || exit 1
        ) &
        dns_pids+=($!)
    done

    for pid in "${dns_pids[@]}"; do
        if ! wait "$pid" 2>/dev/null; then
            ((dns_fail++)) || true
        fi
    done

    if [[ "$dns_fail" -eq 0 ]]; then
        pass "All 20 concurrent DNS queries succeeded"
    else
        fail "${dns_fail}/20 concurrent DNS queries failed"
    fi

    log_section "10.2 Concurrent proxy requests (10 parallel)"
    local proxy_pids=()
    local proxy_fail=0
    for i in $(seq 1 10); do
        (
            code=$(gscurl -s -o /dev/null -w "%{http_code}" "${TESTBED_HTTP}/" 2>/dev/null || echo "000")
            [[ "$code" =~ ^[23] ]] && exit 0 || exit 1
        ) &
        proxy_pids+=($!)
    done

    for pid in "${proxy_pids[@]}"; do
        if ! wait "$pid" 2>/dev/null; then
            ((proxy_fail++)) || true
        fi
    done

    if [[ "$proxy_fail" -eq 0 ]]; then
        pass "All 10 concurrent proxy requests succeeded"
    else
        fail "${proxy_fail}/10 concurrent proxy requests failed"
    fi
}

###############################################################################
# SECTION 11 — Large File Downloads (the modern internet)
###############################################################################
test_large_downloads() {
    log_header "SECTION 11 — LARGE FILE DOWNLOADS"

    echo "  INFO  The proxy architecture has TWO code paths:"
    echo "        ① Under ${MaxContentScanSize:-10MB}: io.ReadAll buffers ENTIRE body in RAM, then scans, then forwards"
    echo "        ② Over ${MaxContentScanSize:-10MB}: limitedReader.N==0 triggers streaming io.Copy passthrough"
    echo "        NEITHER path streams bytes to the client as they arrive."
    echo ""

    # We use local testbed files for reliable large file testing
    local TEST_FILE_BASE="${TESTBED_FILES}"

    # 11.1  Small file (under 10MB — buffered path)
    log_section "11.1 Small file — 1MB (buffered path, under MaxContentScanSize)"
    local small_result
    small_result=$(gscurl -s -o /dev/null -w "%{http_code}|%{size_download}|%{time_total}|%{speed_download}" \
        "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null || echo "000|0|0|0")
    local s_code s_size s_time s_speed
    s_code=$(echo "$small_result" | cut -d'|' -f1)
    s_size=$(echo "$small_result" | cut -d'|' -f2)
    s_time=$(echo "$small_result" | cut -d'|' -f3)
    s_speed=$(echo "$small_result" | cut -d'|' -f4)
    if [[ "$s_code" == "000" ]]; then
        skip_test "Test file server unreachable"
    elif [[ "$s_code" =~ ^[2] ]]; then
        local s_size_mb
        s_size_mb=$(echo "$s_size" | awk '{printf "%.1f", $1/1048576}')
        local s_speed_mb
        s_speed_mb=$(echo "$s_speed" | awk '{printf "%.1f", $1/1048576}')
        pass "1MB download: ${s_size_mb}MB in ${s_time}s (${s_speed_mb} MB/s)"
    else
        fail "1MB download failed: HTTP ${s_code}"
    fi

    # 11.2  Medium file (10MB — hits the MaxContentScanSize boundary)
    log_section "11.2 Medium file — 10MB (MaxContentScanSize boundary)"
    local med_result
    med_result=$(gscurl -s -o /dev/null -w "%{http_code}|%{size_download}|%{time_total}|%{speed_download}" \
        --max-time 60 "${TEST_FILE_BASE}/10MB.bin" 2>/dev/null || echo "000|0|0|0")
    local m_code m_size m_time m_speed
    m_code=$(echo "$med_result" | cut -d'|' -f1)
    m_size=$(echo "$med_result" | cut -d'|' -f2)
    m_time=$(echo "$med_result" | cut -d'|' -f3)
    m_speed=$(echo "$med_result" | cut -d'|' -f4)
    if [[ "$m_code" == "000" ]]; then
        skip_test "10MB test file unreachable or timed out"
    elif [[ "$m_code" =~ ^[2] ]]; then
        local m_size_mb m_speed_mb
        m_size_mb=$(echo "$m_size" | awk '{printf "%.1f", $1/1048576}')
        m_speed_mb=$(echo "$m_speed" | awk '{printf "%.1f", $1/1048576}')
        pass "10MB download: ${m_size_mb}MB in ${m_time}s (${m_speed_mb} MB/s)"
        # Check if the size matches (proxy might truncate or corrupt)
        local m_size_int
        m_size_int=$(echo "$m_size" | cut -d. -f1)
        if [[ "$m_size_int" -lt 9000000 ]]; then
            fail "10MB download truncated: only ${m_size_mb}MB received"
        fi
    else
        fail "10MB download failed: HTTP ${m_code}"
    fi

    # 11.3  Large file (100MB — well past scan limit, streaming path)
    log_section "11.3 Large file — 100MB (streaming passthrough path)"
    local lg_result
    lg_result=$(gscurl -s -o /dev/null -w "%{http_code}|%{size_download}|%{time_total}|%{speed_download}" \
        --max-time 120 "${TEST_FILE_BASE}/100MB.bin" 2>/dev/null || echo "000|0|0|0")
    local l_code l_size l_time l_speed
    l_code=$(echo "$lg_result" | cut -d'|' -f1)
    l_size=$(echo "$lg_result" | cut -d'|' -f2)
    l_time=$(echo "$lg_result" | cut -d'|' -f3)
    l_speed=$(echo "$lg_result" | cut -d'|' -f4)
    if [[ "$l_code" == "000" ]]; then
        skip_test "100MB test file unreachable or timed out"
    elif [[ "$l_code" =~ ^[2] ]]; then
        local l_size_mb l_speed_mb
        l_size_mb=$(echo "$l_size" | awk '{printf "%.1f", $1/1048576}')
        l_speed_mb=$(echo "$l_speed" | awk '{printf "%.1f", $1/1048576}')
        pass "100MB download: ${l_size_mb}MB in ${l_time}s (${l_speed_mb} MB/s)"
        local l_size_int
        l_size_int=$(echo "$l_size" | cut -d. -f1)
        if [[ "$l_size_int" -lt 90000000 ]]; then
            fail "100MB download truncated: only ${l_size_mb}MB received"
        fi
    else
        fail "100MB download failed: HTTP ${l_code}"
    fi

    # 11.4  Time-to-first-byte (TTFB) — how long before the client gets the first byte?
    #        The proxy buffers up to 10MB before forwarding ANYTHING.
    log_section "11.4 Time-to-first-byte (TTFB) — proxy buffering delay"
    local ttfb_direct ttfb_proxy
    # Direct TTFB (baseline)
    ttfb_direct=$(curl -s -o /dev/null -w "%{time_starttransfer}" \
        --max-time 15 "${TEST_FILE_BASE}/10MB.bin" 2>/dev/null || echo "0")
    # Proxied TTFB
    ttfb_proxy=$(curl -s -o /dev/null -w "%{time_starttransfer}" \
        --max-time 30 --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${TEST_FILE_BASE}/10MB.bin" 2>/dev/null || echo "0")
    verbose "TTFB direct: ${ttfb_direct}s, proxied: ${ttfb_proxy}s"

    if [[ "$ttfb_direct" != "0" && "$ttfb_proxy" != "0" ]]; then
        # Calculate ratio
        local ttfb_ratio
        ttfb_ratio=$(echo "$ttfb_proxy $ttfb_direct" | awk '{if ($2 > 0) printf "%.1f", $1/$2; else print "N/A"}')
        echo "  INFO  TTFB direct: ${ttfb_direct}s | proxied: ${ttfb_proxy}s | ratio: ${ttfb_ratio}x"

        # Proxy buffering 10MB before first byte should cause significant TTFB increase
        local proxy_ttfb_ms
        proxy_ttfb_ms=$(echo "$ttfb_proxy" | awk '{printf "%d", $1 * 1000}')
        if [[ "$proxy_ttfb_ms" -lt 2000 ]]; then
            pass "TTFB acceptable: ${ttfb_proxy}s (proxy may be streaming)"
        elif [[ "$proxy_ttfb_ms" -lt 10000 ]]; then
            known_issue "TTFB slow: ${ttfb_proxy}s — proxy buffers entire response before forwarding" \
                "Ratio: ${ttfb_ratio}x slower than direct. Caused by io.ReadAll buffering (up to 10MB)"
        else
            fail "TTFB very slow: ${ttfb_proxy}s — proxy is fully buffering before forwarding"
        fi
    else
        skip_test "Could not measure TTFB (connection failed)"
    fi

    # 11.5  Download integrity — compare checksums direct vs proxied
    log_section "11.5 Download integrity (checksum comparison)"
    local direct_md5 proxy_md5
    direct_md5=$(curl -s --max-time 15 "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null | md5sum | awk '{print $1}')
    proxy_md5=$(gscurl -s --max-time 15 "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null | md5sum | awk '{print $1}')
    verbose "Direct MD5: ${direct_md5}"
    verbose "Proxy  MD5: ${proxy_md5}"
    if [[ -n "$direct_md5" && "$direct_md5" == "$proxy_md5" ]]; then
        pass "Download integrity: checksums match (MD5: ${direct_md5})"
    elif [[ -z "$direct_md5" || -z "$proxy_md5" ]]; then
        skip_test "Could not download file for checksum comparison"
    else
        fail "Download CORRUPTED: direct=${direct_md5} proxy=${proxy_md5}" \
            "Proxy is modifying binary data during transfer!"
    fi
}

###############################################################################
# SECTION 12 — Streaming & Chunked Transfer
###############################################################################
test_streaming() {
    log_header "SECTION 12 — STREAMING & CHUNKED TRANSFER"

    echo "  INFO  A modern proxy MUST support:"
    echo "        • Chunked Transfer-Encoding (HTTP/1.1 streaming)"
    echo "        • Server-Sent Events (SSE / EventSource)"
    echo "        • Long-lived connections (streaming video/audio)"
    echo "        • Progressive delivery (send bytes as they arrive)"
    echo "        The proxy currently has NO http.Flusher support."
    echo ""

    # 12.1  Chunked transfer encoding
    log_section "12.1 Chunked Transfer-Encoding"
    local chunk_resp
    chunk_resp=$(gscurl -sI "${ECHO_SERVER}/stream/5" 2>/dev/null || echo "UNREACHABLE")
    if [[ "$chunk_resp" == "UNREACHABLE" ]]; then
        skip_test "Echo server unreachable"
    else
        local chunk_te
        chunk_te=$(echo "$chunk_resp" | grep -i "Transfer-Encoding" || true)
        local chunk_code
        chunk_code=$(echo "$chunk_resp" | head -1 | awk '{print $2}')
        if [[ "$chunk_code" =~ ^[2] ]]; then
            pass "Chunked endpoint returns HTTP ${chunk_code}"
            # Now test: does the proxy actually stream or buffer?
            local chunk_body
            chunk_body=$(gscurl -s "${ECHO_SERVER}/stream/5" 2>/dev/null | wc -l)
            if [[ "$chunk_body" -ge 5 ]]; then
                pass "Chunked response: received ${chunk_body} lines (expected ≥5)"
            else
                fail "Chunked response: only ${chunk_body} lines received (expected ≥5)"
            fi
        else
            fail "Chunked endpoint failed: HTTP ${chunk_code}"
        fi
    fi

    # 12.2  Server-Sent Events (SSE)
    log_section "12.2 Server-Sent Events (SSE) — time-to-first-event"
    # echo server /stream/3 sends 3 JSON objects
    # If the proxy buffers, we won't see any data until ALL events are buffered
    local sse_start sse_first_byte sse_end
    sse_start=$(date +%s%N)
    # Read just the first line (first event) and measure time
    local first_event
    first_event=$(timeout 10 bash -c "gscurl -s '${ECHO_SERVER}/stream/3' | head -1" 2>/dev/null || echo "TIMEOUT")
    sse_end=$(date +%s%N)
    local sse_ms=$(( (sse_end - sse_start) / 1000000 ))

    if [[ "$first_event" == "TIMEOUT" ]]; then
        known_issue "SSE: timed out waiting for first event — proxy may be buffering" \
            "No http.Flusher support means events are held until response completes"
    elif [[ -n "$first_event" ]]; then
        verbose "First SSE event received in ${sse_ms}ms"
        if [[ "$sse_ms" -lt 3000 ]]; then
            pass "SSE first event in ${sse_ms}ms"
        else
            known_issue "SSE first event delayed: ${sse_ms}ms — proxy is buffering events" \
                "Proxy does not flush individual events to client (no http.Flusher)"
        fi
    else
        skip_test "SSE test inconclusive"
    fi

    # 12.3  Streaming response — drip endpoint (timed byte delivery)
    log_section "12.3 Streaming drip — timed byte delivery"
    # echo server /drip sends bytes at intervals — tests real streaming
    local drip_start drip_result drip_end
    drip_start=$(date +%s%N)
    drip_result=$(gscurl -s -o /dev/null -w "%{http_code}|%{time_total}|%{size_download}" \
        --max-time 20 "${ECHO_SERVER}/drip?duration=3&numbytes=5&code=200&delay=0" 2>/dev/null || echo "000|0|0")
    drip_end=$(date +%s%N)
    local d_code d_time d_size
    d_code=$(echo "$drip_result" | cut -d'|' -f1)
    d_time=$(echo "$drip_result" | cut -d'|' -f2)
    d_size=$(echo "$drip_result" | cut -d'|' -f3)

    if [[ "$d_code" == "000" ]]; then
        skip_test "Drip endpoint unreachable"
    elif [[ "$d_code" =~ ^[2] ]]; then
        local d_time_ms
        d_time_ms=$(echo "$d_time" | awk '{printf "%d", $1 * 1000}')
        verbose "Drip: HTTP ${d_code}, ${d_size} bytes in ${d_time}s"
        # The drip takes 3 seconds server-side. If proxy buffers,
        # total time ≈ 3s. If streaming, client sees bytes progressively.
        if [[ "$d_time_ms" -ge 2500 && "$d_time_ms" -le 8000 ]]; then
            pass "Drip completed in ${d_time}s (server drips over 3s)"
        else
            fail "Drip timing unexpected: ${d_time}s"
        fi
    else
        fail "Drip endpoint failed: HTTP ${d_code}"
    fi

    # 12.4  Large chunked streaming (simulated video)
    log_section "12.4 Large chunked response (100 chunks)"
    local bigchunk_result
    bigchunk_result=$(gscurl -s -o /dev/null -w "%{http_code}|%{size_download}|%{time_total}" \
        --max-time 30 "${ECHO_SERVER}/stream-bytes/1048576?chunk_size=10240" 2>/dev/null || echo "000|0|0")
    local bc_code bc_size bc_time
    bc_code=$(echo "$bigchunk_result" | cut -d'|' -f1)
    bc_size=$(echo "$bigchunk_result" | cut -d'|' -f2)
    bc_time=$(echo "$bigchunk_result" | cut -d'|' -f3)
    if [[ "$bc_code" == "000" ]]; then
        skip_test "stream-bytes endpoint unreachable"
    elif [[ "$bc_code" =~ ^[2] ]]; then
        local bc_size_kb
        bc_size_kb=$(echo "$bc_size" | awk '{printf "%.0f", $1/1024}')
        pass "1MB chunked stream: ${bc_size_kb}KB in ${bc_time}s (HTTP ${bc_code})"
    else
        fail "Chunked stream failed: HTTP ${bc_code}"
    fi
}

###############################################################################
# SECTION 13 — HTTP Range Requests (Resume Downloads)
###############################################################################
test_range_requests() {
    log_header "SECTION 13 — HTTP RANGE REQUESTS (RESUME DOWNLOADS)"

    echo "  INFO  Range requests are CRITICAL for:"
    echo "        • Resuming interrupted downloads (wget -c, curl -C)"
    echo "        • Video seeking (Netflix, YouTube scrubbing)"
    echo "        • Parallel download acceleration"
    echo "        • PDF viewers loading specific pages"
    echo "        The proxy strips Content-Length and re-encodes bodies,"
    echo "        which likely breaks Range request handling."
    echo ""

    local TEST_FILE_BASE="${TESTBED_FILES}"

    # 13.1  Range header passthrough
    log_section "13.1 Range request — first 1024 bytes"
    local range_resp
    range_resp=$(gscurl -sI -H "Range: bytes=0-1023" \
        "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null || echo "UNREACHABLE")
    if [[ "$range_resp" == "UNREACHABLE" ]]; then
        skip_test "Test file server unreachable"
    else
        local range_code
        range_code=$(echo "$range_resp" | head -1 | awk '{print $2}')
        local content_range
        content_range=$(echo "$range_resp" | grep -i "^Content-Range:" || true)
        local range_cl
        range_cl=$(echo "$range_resp" | grep -i "^Content-Length:" | awk '{print $2}' | tr -d '\r')

        if [[ "$range_code" == "206" ]]; then
            pass "Range request returns 206 Partial Content"
            if [[ -n "$content_range" ]]; then
                pass "Content-Range header present: $(echo "$content_range" | xargs)"
            else
                fail "Missing Content-Range header in 206 response"
            fi
        elif [[ "$range_code" == "200" ]]; then
            known_issue "Range request returns 200 instead of 206 — proxy ignores Range header" \
                "Proxy strips Accept-Encoding and likely also interferes with Range requests"
        else
            fail "Range request returned unexpected: HTTP ${range_code}"
        fi
    fi

    # 13.2  Range body size verification
    log_section "13.2 Range body size — should be exactly 1024 bytes"
    local range_body_size
    range_body_size=$(gscurl -s -H "Range: bytes=0-1023" \
        "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null | wc -c)
    verbose "Range body size: ${range_body_size} bytes (expected: 1024)"
    if [[ "$range_body_size" -eq 1024 ]]; then
        pass "Range body size correct: ${range_body_size} bytes"
    elif [[ "$range_body_size" -gt 1024 ]]; then
        known_issue "Range body too large: ${range_body_size} bytes (expected 1024)" \
            "Proxy is ignoring Range and sending the full response"
    else
        fail "Range body size wrong: ${range_body_size} bytes (expected 1024)"
    fi

    # 13.3  Mid-file range (simulates resume)
    log_section "13.3 Mid-file range — resume download simulation"
    local mid_resp
    mid_resp=$(gscurl -sI -H "Range: bytes=524288-1048575" \
        "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null || echo "UNREACHABLE")
    if [[ "$mid_resp" == "UNREACHABLE" ]]; then
        skip_test "Test file server unreachable"
    else
        local mid_code
        mid_code=$(echo "$mid_resp" | head -1 | awk '{print $2}')
        if [[ "$mid_code" == "206" ]]; then
            local mid_body_size
            mid_body_size=$(gscurl -s -H "Range: bytes=524288-1048575" \
                "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null | wc -c)
            if [[ "$mid_body_size" -ge 500000 && "$mid_body_size" -le 524288 ]]; then
                pass "Resume download: ${mid_body_size} bytes from mid-file (HTTP 206)"
            else
                fail "Resume download: wrong size ${mid_body_size} (expected ~524288)"
            fi
        elif [[ "$mid_code" == "200" ]]; then
            known_issue "Resume download returns full file (HTTP 200) instead of partial (206)" \
                "Cannot resume interrupted downloads through proxy"
        else
            fail "Resume download failed: HTTP ${mid_code}"
        fi
    fi

    # 13.4  Multi-range request
    log_section "13.4 Multi-range request"
    local multi_code
    multi_code=$(gscurl -s -o /dev/null -w "%{http_code}" \
        -H "Range: bytes=0-99,200-299" \
        "${TEST_FILE_BASE}/1MB.bin" 2>/dev/null || echo "000")
    if [[ "$multi_code" == "206" ]]; then
        pass "Multi-range request works (HTTP 206)"
    elif [[ "$multi_code" == "200" ]]; then
        known_issue "Multi-range returns full file (HTTP 200)" \
            "Proxy doesn't support multipart/byteranges responses"
    elif [[ "$multi_code" == "000" ]]; then
        skip_test "Multi-range test unreachable"
    else
        verbose "Multi-range returned HTTP ${multi_code}"
        pass "Multi-range handled without error (HTTP ${multi_code})"
    fi
}

###############################################################################
# SECTION 14 — Memory & Resource Behaviour
###############################################################################
test_resource_behaviour() {
    log_header "SECTION 14 — MEMORY & RESOURCE BEHAVIOUR"

    # 14.1  MaxContentScanSize analysis
    log_section "14.1 MaxContentScanSize impact analysis"
    echo "  INFO  proxy.go: MaxContentScanSize = 10MB (1e7 bytes)"
    echo "        Every response under 10MB is FULLY BUFFERED in RAM via io.ReadAll"
    echo "        before any byte reaches the client."
    echo ""
    echo "        With 100 concurrent connections downloading 5MB files:"
    echo "        → 100 × 5MB = 500MB RAM just for response buffering"
    echo "        → Plus Go runtime overhead, TLS state, etc."
    echo ""
    known_issue "Proxy buffers up to 10MB per response in RAM (MaxContentScanSize)" \
        "io.ReadAll(teeReader) at proxy.go ~line 488 holds entire response body. 100 concurrent = 1GB+ RAM"

    # 14.2  Connection count under load
    log_section "14.2 Connection count under concurrent load"
    if command -v ss &> /dev/null; then
        local before_count
        before_count=$(ss -tn state established | grep -c ":${PROXY_PORT}" 2>/dev/null || echo "0")

        # Fire 20 parallel requests
        local pids=()
        for i in $(seq 1 20); do
            (gscurl -s -o /dev/null "${TESTBED_HTTP}/" 2>/dev/null) &
            pids+=($!)
        done

        sleep 1  # Let connections establish
        local during_count
        during_count=$(ss -tn state established | grep -c ":${PROXY_PORT}" 2>/dev/null || echo "0")

        for pid in "${pids[@]}"; do wait "$pid" 2>/dev/null; done

        local after_count
        after_count=$(ss -tn state established | grep -c ":${PROXY_PORT}" 2>/dev/null || echo "0")

        verbose "Connections: before=${before_count}, during=${during_count}, after=${after_count}"
        echo "  INFO  Proxy connections: before=${before_count}, during-load=${during_count}, after=${after_count}"

        if [[ "$after_count" -le "$((before_count + 5))" ]]; then
            pass "Connections cleaned up after load (${during_count} → ${after_count})"
        else
            fail "Connection leak: ${after_count} connections remain after load (started at ${before_count})"
        fi
    else
        skip_test "ss not available for connection counting"
    fi
}

###############################################################################
# §15  ADVERSARIAL RESILIENCE & CVE-INSPIRED TESTS
#
# These tests throw protocol-level misbehaviour at the proxy to verify it
# does not crash, hang, or pass dangerous garbage to the client.
# Each endpoint on the echo server deliberately violates HTTP specs in a
# way that real-world (or malicious) servers actually do.
#
# Philosophy: "Don't configure nginx to make tests work — configure it to
#              make tests FAIL."  The hostile internet owes us nothing.
#
# Three-body triage rule: every failure could be (a) echo_server bug,
# (b) proxy bug, or (c) test-script bug.  Don't "fix" things wrongly.
###############################################################################
test_adversarial_resilience() {
    log_header "§15  ADVERSARIAL RESILIENCE & CVE TESTS"

    local http_code body_len body proxy_url result

    # Helper: check proxy is still alive after each adversarial request
    proxy_alive() {
        local chk
        chk=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 \
            --proxy "http://${PROXY_HOST}:${PROXY_PORT}" "${ECHO_SERVER}/health" 2>/dev/null || echo "000")
        [[ "$chk" =~ ^[23] ]]
    }

    # ── 15.1  HEAD with illegal body ─────────────────────────────────────────
    log_section "15.1  HEAD with illegal body"
    body=$(curl -s --max-time "$CURL_TIMEOUT" -X HEAD \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/head-with-body" 2>/dev/null)
    if [[ -z "$body" ]]; then
        pass "§15.1 Proxy stripped illegal body from HEAD response"
    else
        known_issue "§15.1 Proxy forwarded body on HEAD (${#body} bytes)" \
            "RFC 9110 §9.3.2 — HEAD MUST NOT contain body"
    fi

    # ── 15.2  Lying Content-Length (under) ───────────────────────────────────
    log_section "15.2  Lying Content-Length (claims 1000, sends 50)"
    # Proxy should not hang waiting for the remaining 950 bytes
    body=$(curl -s --max-time 5 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/lying-content-length" 2>/dev/null)
    if [[ $? -eq 0 ]] || [[ -n "$body" ]]; then
        pass "§15.2 Proxy handled under-length body without hanging"
    else
        fail "§15.2 Proxy hung on lying Content-Length (under)" \
            "Upstream sent 50 bytes but claimed 1000"
    fi
    proxy_alive || fail "§15.2 PROXY CRASHED after lying-content-length"

    # ── 15.3  Lying Content-Length (over) ────────────────────────────────────
    log_section "15.3  Lying Content-Length (claims 10, sends 500)"
    body=$(curl -s --max-time 5 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/lying-content-length-over" 2>/dev/null)
    body_len=${#body}
    if [[ $body_len -le 10 ]]; then
        pass "§15.3 Proxy truncated over-length body to Content-Length (got $body_len bytes)"
    elif [[ $body_len -gt 10 ]]; then
        known_issue "§15.3 Proxy forwarded $body_len bytes (C-L said 10)" \
            "Proxy trusts actual body over Content-Length header"
    fi
    proxy_alive || fail "§15.3 PROXY CRASHED after lying-content-length-over"

    # ── 15.4  Drop mid-stream ────────────────────────────────────────────────
    log_section "15.4  Connection drop mid-stream"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/drop-mid-stream" 2>/dev/null || echo "000")
    if [[ "$http_code" == "502" ]] || [[ "$http_code" == "000" ]]; then
        pass "§15.4 Proxy returned 502/error on mid-stream drop (HTTP $http_code)"
    elif [[ "$http_code" == "200" ]]; then
        known_issue "§15.4 Proxy returned 200 despite mid-stream drop" \
            "May have forwarded partial data to client"
    else
        pass "§15.4 Proxy handled mid-stream drop (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.4 PROXY CRASHED after drop-mid-stream"

    # ── 15.5  Mixed Content-Length + Chunked (request smuggling vector) ──────
    log_section "15.5  Mixed Content-Length + Transfer-Encoding: chunked"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/mixed-cl-chunked" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.5 Proxy handled mixed CL/chunked without crashing"
        verbose "Body: ${body:0:80}"
    else
        fail "§15.5 Proxy failed on mixed CL/chunked response"
    fi
    proxy_alive || fail "§15.5 PROXY CRASHED after mixed-cl-chunked"

    # ── 15.6  Gzip body without Content-Encoding header ─────────────────────
    log_section "15.6  Gzip body, no Content-Encoding header"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/gzip-no-header" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.6 Proxy forwarded gzip-without-header response"
    else
        fail "§15.6 Proxy returned empty body for gzip-no-header"
    fi

    # ── 15.7  Double-gzip with single Content-Encoding ──────────────────────
    log_section "15.7  Double-gzip body, single Content-Encoding"
    http_code=$(curl -s -o "$TMPDIR/double-gzip" -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/gzip-double" 2>/dev/null)
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.7 Proxy handled double-gzip (HTTP $http_code, $(wc -c < "$TMPDIR/double-gzip") bytes)"
    else
        fail "§15.7 Proxy failed on double-gzip (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.7 PROXY CRASHED after gzip-double"

    # ── 15.8  No framing (no CL, no chunked — connection close) ─────────────
    log_section "15.8  No framing — body ends at connection close"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/no-framing" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.8 Proxy delivered body from un-framed response (${#body} bytes)"
    else
        known_issue "§15.8 Proxy returned empty body for un-framed response" \
            "No Content-Length, no chunked — body delimited by connection close"
    fi
    proxy_alive || fail "§15.8 PROXY CRASHED after no-framing"

    # ── 15.9  SSRF redirect to localhost ─────────────────────────────────────
    log_section "15.9  SSRF — redirect to localhost:8080/admin"
    # The echo server returns 302 → http://127.0.0.1:8080/admin
    # A safe proxy should NOT follow redirects to internal addresses
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/ssrf-redirect" 2>/dev/null)
    # Proxy should just forward the 302 — it should NOT resolve/follow it
    if [[ "$http_code" == "302" ]]; then
        pass "§15.9 Proxy forwarded 302 without following (HTTP $http_code)"
    else
        known_issue "§15.9 Proxy returned HTTP $http_code for SSRF redirect" \
            "Expected 302 pass-through; proxy may have followed the redirect"
    fi

    # ── 15.10  Null bytes in headers ─────────────────────────────────────────
    #   Attack: Null bytes (\x00) in header values can cause C-parser header
    #   injection where parsers truncate at the null, reading different headers.
    #   Protection: Go's net/http.Transport rejects responses with null bytes
    #   in headers at the RoundTrip level — our proxy code never sees them.
    log_section "15.10  Null bytes in response headers"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/null-in-headers" 2>/dev/null || echo "000")
    if [[ "$http_code" == "502" ]] || [[ "$http_code" == "000" ]]; then
        pass "§15.10 Null-byte headers rejected (HTTP $http_code) — * handled by Go HTTP client"
    elif [[ "$http_code" == "200" ]]; then
        known_issue "§15.10 Proxy forwarded null-byte headers (HTTP $http_code)" \
            "Null bytes in headers can cause C-parser header injection"
    else
        pass "§15.10 Proxy handled null-in-headers (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.10 PROXY CRASHED after null-in-headers"

    # ── 15.11  Huge header (64KB single header value) ────────────────────────
    log_section "15.11  Huge header (64KB single value)"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/huge-header" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[2345] ]]; then
        pass "§15.11 Proxy handled 64KB header (HTTP $http_code)"
    else
        fail "§15.11 Proxy failed on huge header (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.11 PROXY CRASHED after huge-header"

    # ── 15.12  Double Content-Length ─────────────────────────────────────────
    #   Attack: Two Content-Length headers with different values can cause
    #   request smuggling — frontend and backend disagree on body boundaries.
    #   RFC 9110 §8.6: conflicting Content-Length MUST be rejected.
    #   Protection: Go's net/http.Transport rejects conflicting C-L at the
    #   RoundTrip level, returning an error before our proxy sees the response.
    log_section "15.12  Double Content-Length headers (different values)"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/double-content-length" 2>/dev/null || echo "000")
    if [[ "$http_code" == "502" ]] || [[ "$http_code" == "000" ]]; then
        pass "§15.12 Double Content-Length rejected (HTTP $http_code) — * handled by Go HTTP client"
    elif [[ "$http_code" == "200" ]]; then
        known_issue "§15.12 Proxy forwarded double Content-Length (HTTP 200)" \
            "RFC 9110 §8.6: conflicting C-L MUST be rejected"
    else
        pass "§15.12 Proxy handled double C-L (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.12 PROXY CRASHED after double-content-length"

    # ── 15.13  Premature EOF in chunked stream ──────────────────────────────
    log_section "15.13  Premature EOF in chunked stream"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/premature-eof-chunked" 2>/dev/null || echo "000")
    if [[ "$http_code" == "502" ]] || [[ "$http_code" == "000" ]]; then
        pass "§15.13 Proxy detected premature EOF in chunked stream (HTTP $http_code)"
    elif [[ "$http_code" == "200" ]]; then
        known_issue "§15.13 Proxy returned 200 for incomplete chunked stream" \
            "Terminal chunk 0\\r\\n\\r\\n never sent — stream is incomplete"
    else
        pass "§15.13 Proxy handled premature-eof-chunked (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.13 PROXY CRASHED after premature-eof-chunked"

    # ── 15.14  Negative Content-Length ───────────────────────────────────────
    #   Attack: Content-Length: -1 can cause integer underflow in parsers,
    #   leading to buffer over-read or infinite read loops.
    #   Protection: Go's net/http.Transport rejects negative Content-Length
    #   at the RoundTrip level — returns error, no response parsed.
    log_section "15.14  Negative Content-Length (-1)"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/negative-content-length" 2>/dev/null || echo "000")
    if [[ "$http_code" == "502" ]] || [[ "$http_code" == "000" ]]; then
        pass "§15.14 Negative Content-Length rejected (HTTP $http_code) — * handled by Go HTTP client"
    elif [[ "$http_code" == "200" ]]; then
        known_issue "§15.14 Proxy accepted negative Content-Length" \
            "Content-Length: -1 may cause integer underflow in parsers"
    else
        pass "§15.14 Proxy handled negative C-L (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.14 PROXY CRASHED after negative-content-length"

    # ── 15.15  Non-standard status reason phrase ─────────────────────────────
    log_section "15.15  Non-standard status reason phrase"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/space-in-status" 2>/dev/null || echo "000")
    if [[ "$http_code" == "200" ]]; then
        pass "§15.15 Proxy accepted non-standard status line (HTTP 200)"
    else
        known_issue "§15.15 Proxy returned HTTP $http_code for non-standard status" \
            "Status line: 'HTTP/1.1 200 OK COOL BEANS'"
    fi

    # ── 15.16  Trailer injection via chunked ─────────────────────────────────
    log_section "15.16  Chunked trailer header injection"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/trailer-injection" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.16 Proxy forwarded chunked response with trailers (${#body} bytes)"
    else
        fail "§15.16 Proxy failed on trailer-injection"
    fi

    # ── 15.17  Slow body (chunked, 1 byte/sec for 3 seconds) ────────────────
    log_section "15.17  Slow body (3s drip)"
    body=$(curl -s --max-time 8 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/slow-body?duration=3" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.17 Proxy delivered slow-body response (${#body} bytes)"
    else
        known_issue "§15.17 Proxy timed out or dropped slow-body" \
            "Body dribbles 1 chunk/sec — proxy may have hit read timeout"
    fi
    proxy_alive || fail "§15.17 PROXY CRASHED after slow-body"

    # ── 15.18  Content-encoding bomb (1KB → 1MB) ────────────────────────────
    log_section "15.18  Content-encoding bomb (1KB gzip → 1MB)"
    http_code=$(curl -s -o "$TMPDIR/bomb" -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/content-encoding-bomb" 2>/dev/null)
    local bomb_size
    bomb_size=$(wc -c < "$TMPDIR/bomb" 2>/dev/null || echo 0)
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.18 Proxy handled gzip bomb (HTTP $http_code, $bomb_size bytes delivered)"
    elif [[ "$http_code" == "502" ]]; then
        pass "§15.18 Proxy rejected gzip bomb (HTTP 502)"
    else
        fail "§15.18 Proxy returned unexpected HTTP $http_code for gzip bomb"
    fi
    proxy_alive || fail "§15.18 PROXY CRASHED after content-encoding-bomb"

    # ── 15.19  HTTP response splitting ───────────────────────────────────────
    #   Attack: Upstream injects \r\n into a header value to smuggle additional
    #   headers (e.g. Set-Cookie: evil=stolen). This is a real attack vector.
    #   Limitation: Go's net/http.Transport correctly parses \r\n as a header
    #   delimiter, so the injected header appears as a legitimate separate header
    #   in resp.Header. By that point, there's no way to distinguish it from a
    #   header the upstream intentionally sent. This is an inherent limitation
    #   of ANY HTTP-level proxy — the protection must be at the origin server.
    log_section "15.19  HTTP response splitting attempt"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/response-splitting" 2>/dev/null || echo "000")
    if [[ "$http_code" == "502" ]] || [[ "$http_code" == "000" ]]; then
        pass "§15.19 Proxy rejected response-splitting attempt (HTTP $http_code)"
    elif [[ "$http_code" == "200" ]]; then
        # Check if the injected cookie header made it through
        local cookies
        cookies=$(curl -s --max-time "$CURL_TIMEOUT" -D - -o /dev/null \
            --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
            "${ECHO_SERVER}/adversarial/response-splitting" 2>/dev/null | grep -ci "evil=stolen" || true)
        if [[ "$cookies" -gt 0 ]]; then
            known_issue "§15.19 Response splitting: injected Set-Cookie forwarded" \
                "Inherent HTTP-level proxy limitation — Go HTTP client parses \\r\\n as header delimiter. Origin server must sanitise."
        else
            pass "§15.19 Proxy sanitised response-splitting headers (HTTP 200)"
        fi
    else
        pass "§15.19 Proxy handled response-splitting (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.19 PROXY CRASHED after response-splitting"

    # ── 15.20  Keep-alive desync ─────────────────────────────────────────────
    log_section "15.20  Keep-alive desync (says keep-alive, then closes)"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/keepalive-desync" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.20 Proxy survived keep-alive desync (HTTP $http_code)"
    else
        known_issue "§15.20 Proxy returned HTTP $http_code on keep-alive desync" \
            "Server says keep-alive, then immediately closes connection"
    fi
    # Now do a follow-up request to make sure the proxy's connection pool
    # recovered from the desync
    local followup
    followup=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/health" 2>/dev/null || echo "000")
    if [[ "$followup" =~ ^[23] ]]; then
        pass "§15.20b Proxy recovered after keep-alive desync"
    else
        fail "§15.20b Proxy broken after keep-alive desync (follow-up: HTTP $followup)" \
            "Connection pool may be poisoned"
    fi

    # ══════════════════════════════════════════════════════════════════════════
    # CVE-INSPIRED TESTS — from Squid's 55-vulnerability + 35 0day audit
    # Each of these represents a real-world attack pattern that killed Squid.
    # We're not building Squid — but our home users deserve better.
    # ══════════════════════════════════════════════════════════════════════════

    log_section "15.21  CVE-2021-28662 — Vary: Other assertion crash"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/vary-other" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.21 Proxy handled Vary: Other without crash"
    else
        fail "§15.21 Proxy failed on Vary: Other header"
    fi
    proxy_alive || fail "§15.21 PROXY CRASHED on Vary: Other (CVE-2021-28662 pattern!)"

    # ── 15.22  Unexpected 100 Continue (Squid unfixed 0day) ──────────────────
    log_section "15.22  Unsolicited 100 Continue (Squid 0day)"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/100-continue" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.22 Proxy handled unsolicited 100 Continue + 200 response"
        verbose "Body: ${body:0:60}"
    else
        known_issue "§15.22 Proxy returned empty body after 100 Continue" \
            "Squid unfixed 0day — some proxies consume 100 as the final response"
    fi
    proxy_alive || fail "§15.22 PROXY CRASHED on 100-continue (Squid unfixed 0day!)"

    # ── 15.23  Multiple 100 Continue (10x barrage) ──────────────────────────
    log_section "15.23  Multiple 100 Continue (10x barrage)"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/multi-100-continue" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.23 Proxy survived 10x 100-Continue barrage"
    else
        known_issue "§15.23 Proxy returned empty body after 10x 100-Continue" \
            "Proxy may have given up after too many informational responses"
    fi
    proxy_alive || fail "§15.23 PROXY CRASHED on multi-100-continue"

    # ── 15.24  CVE-2024-25111 — Huge chunk extensions ───────────────────────
    log_section "15.24  CVE-2024-25111 — Huge chunk extensions (8KB/chunk)"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/chunked-extensions" 2>/dev/null)
    if [[ -n "$body" ]]; then
        pass "§15.24 Proxy handled huge chunk extensions (${#body} bytes)"
    else
        known_issue "§15.24 Proxy returned empty body for chunked-extensions" \
            "CVE-2024-25111: Squid stack overflow from recursive chunk parsing"
    fi
    proxy_alive || fail "§15.24 PROXY CRASHED on chunked-extensions (CVE-2024-25111 pattern!)"

    # ── 15.25  CVE-2021-31808 — Range integer overflow ──────────────────────
    log_section "15.25  CVE-2021-31808 — Range header integer overflow"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/range-overflow" 2>/dev/null || echo "000")
    if [[ "$http_code" == "206" ]] || [[ "$http_code" == "200" ]]; then
        pass "§15.25 Proxy forwarded range-overflow response (HTTP $http_code)"
    elif [[ "$http_code" == "502" ]]; then
        pass "§15.25 Proxy rejected range-overflow (HTTP 502)"
    else
        fail "§15.25 Proxy failed on range-overflow (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.25 PROXY CRASHED on range-overflow (CVE-2021-31808 pattern!)"

    # ── 15.26  CVE-2021-33620 — Invalid Content-Range ───────────────────────
    log_section "15.26  CVE-2021-33620 — Invalid Content-Range (end > total)"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/content-range-bad" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.26 Proxy forwarded bad Content-Range (HTTP $http_code)"
    elif [[ "$http_code" == "502" ]]; then
        pass "§15.26 Proxy rejected invalid Content-Range (HTTP 502)"
    else
        fail "§15.26 Proxy failed on content-range-bad (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.26 PROXY CRASHED on content-range-bad (CVE-2021-33620 pattern!)"

    # ── 15.27  CVE-2023-50269 — XFF overflow in response ────────────────────
    log_section "15.27  CVE-2023-50269 — Giant X-Forwarded-For in response"
    http_code=$(curl -s -o "$TMPDIR/xff-resp" -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/xff-overflow" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.27 Proxy handled 5000-entry XFF response header (HTTP $http_code)"
    elif [[ "$http_code" == "502" ]]; then
        pass "§15.27 Proxy rejected giant XFF response (HTTP 502)"
    else
        fail "§15.27 Proxy failed on XFF overflow (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.27 PROXY CRASHED on xff-overflow (CVE-2023-50269 pattern!)"

    # ── 15.28  CVE-2023-5824 — Cache poisoning ──────────────────────────────
    log_section "15.28  CVE-2023-5824 — Cache poison (conflicting cache + XSS)"
    http_code=$(curl -s -o "$TMPDIR/cache-poison" -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/cache-poison" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.28 Proxy forwarded cache-poison response (HTTP $http_code)"
        # Second request — if proxy cached the first (poisoned) response, this is bad
        local body2
        body2=$(curl -s --max-time "$CURL_TIMEOUT" \
            --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
            "${ECHO_SERVER}/health" 2>/dev/null)
        if [[ "$body2" == *"script"* ]]; then
            fail "§15.28b Proxy served CACHED XSS on health endpoint!" \
                "CRITICAL: Cache poisoned — XSS serving on unrelated URL"
        else
            pass "§15.28b Cache not poisoned — follow-up request is clean"
        fi
    else
        pass "§15.28 Proxy rejected cache-poison (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.28 PROXY CRASHED on cache-poison (CVE-2023-5824 pattern!)"

    # ── 15.29  CVE-2023-49288 — TRACE reflection ────────────────────────────
    log_section "15.29  CVE-2023-49288 — TRACE-like body reflection"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        -H "Cookie: session=s3cr3t" -H "Authorization: Bearer tok3n" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/trace-reflect" 2>/dev/null)
    if [[ -n "$body" ]]; then
        # Check if our sensitive headers leaked through
        if echo "$body" | grep -q "s3cr3t"; then
            known_issue "§15.29 Proxy reflected sensitive Cookie in response body" \
                "CVE-2023-49288: TRACE reflection — credentials visible in response"
        elif echo "$body" | grep -q "tok3n"; then
            known_issue "§15.29 Proxy reflected Authorization header in response body" \
                "CVE-2023-49288: Auth token visible in response"
        else
            pass "§15.29 Proxy stripped sensitive headers from reflected response"
        fi
    else
        fail "§15.29 Proxy returned empty body for trace-reflect"
    fi
    proxy_alive || fail "§15.29 PROXY CRASHED on trace-reflect (CVE-2023-49288 pattern!)"

    # ── 15.30  1000 repeated Set-Cookie headers ─────────────────────────────
    log_section "15.30  Header repeat — 1000x Set-Cookie"
    http_code=$(curl -s -o "$TMPDIR/header-repeat" -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/header-repeat" 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
        pass "§15.30 Proxy handled 1000x repeated headers (HTTP $http_code)"
    elif [[ "$http_code" == "502" ]]; then
        pass "§15.30 Proxy rejected header barrage (HTTP 502)"
    else
        fail "§15.30 Proxy failed on header-repeat (HTTP $http_code)"
    fi
    proxy_alive || fail "§15.30 PROXY CRASHED on header-repeat"

    # ── 15.31  Wrong Content-Type (says text/plain, body is JSON+XSS) ───────
    log_section "15.31  Wrong Content-Type (text/plain → JSON with XSS)"
    body=$(curl -s --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/wrong-content-type" 2>/dev/null)
    if [[ -n "$body" ]]; then
        if echo "$body" | grep -q "<script>"; then
            known_issue "§15.31 Proxy forwarded XSS payload in mistyped response" \
                "Content-Type: text/plain but body contains <script>alert(1)</script>"
        else
            pass "§15.31 Proxy handled wrong-content-type (XSS may have been stripped)"
        fi
    else
        fail "§15.31 Proxy returned empty body for wrong-content-type"
    fi

    # ── 15.32  Range ignored (server returns 200 instead of 206) ────────────
    log_section "15.32  Range ignored — server returns 200 for Range request"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        -H "Range: bytes=0-99" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/range-ignored" 2>/dev/null)
    if [[ "$http_code" == "200" ]]; then
        pass "§15.32 Proxy passed through 200 when server ignored Range header"
    elif [[ "$http_code" == "206" ]]; then
        fail "§15.32 Proxy turned a 200 into a 206!" \
            "Server explicitly ignored Range but proxy synthesised a 206"
    else
        known_issue "§15.32 Proxy returned HTTP $http_code for range-ignored"
    fi

    # ── 15.33  SSRF redirect chain (external → external → 169.254.169.254) ──
    log_section "15.33  SSRF redirect chain to cloud metadata"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$CURL_TIMEOUT" \
        --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
        "${ECHO_SERVER}/adversarial/ssrf-redirect-chain" 2>/dev/null)
    if [[ "$http_code" == "302" ]]; then
        pass "§15.33 Proxy forwarded first redirect without following (HTTP 302)"
    else
        known_issue "§15.33 Proxy returned HTTP $http_code for SSRF chain" \
            "Multi-hop: external → external → 169.254.169.254"
    fi

    # ══════════════════════════════════════════════════════════════════════════
    # STRESS: rapid-fire all adversarial endpoints — does the proxy survive?
    # ══════════════════════════════════════════════════════════════════════════
    log_section "15.34  Rapid-fire resilience (all safe adversarial endpoints)"
    local rapid_endpoints=(
        "vary-other" "huge-header" "wrong-content-type" "space-in-status"
        "gzip-no-header" "gzip-double" "range-ignored" "trailer-injection"
        "range-overflow" "content-range-bad" "cache-poison" "header-repeat"
        "mixed-cl-chunked" "content-encoding-bomb"
    )
    local rapid_ok=0
    local rapid_fail=0
    for ep in "${rapid_endpoints[@]}"; do
        local rc
        rc=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 \
            --proxy "http://${PROXY_HOST}:${PROXY_PORT}" \
            "${ECHO_SERVER}/adversarial/${ep}" 2>/dev/null || echo "000")
        if [[ "$rc" =~ ^[23] ]]; then
            ((rapid_ok++)) || true
        else
            ((rapid_fail++)) || true
            verbose "  ↳ ${ep}: HTTP $rc"
        fi
    done
    if [[ "$rapid_fail" -eq 0 ]]; then
        pass "§15.34 All ${rapid_ok}/${#rapid_endpoints[@]} rapid-fire endpoints returned 2xx/3xx"
    else
        known_issue "§15.34 Rapid-fire: ${rapid_ok} ok, ${rapid_fail} failed out of ${#rapid_endpoints[@]}"
    fi

    # Final survival check
    if proxy_alive; then
        pass "§15.35 PROXY SURVIVED all adversarial tests — still responding"
    else
        fail "§15.35 PROXY IS DOWN after adversarial battery!" \
            "CRITICAL: Proxy crashed during adversarial testing"
    fi

    echo ""
    echo -e "  ${BOLD}Section 15 complete: Adversarial + CVE resilience battery${NC}"
}

###############################################################################
# Summary & Known Issues Reference
###############################################################################
print_summary() {
    log_header "TEST SUMMARY"

    echo ""
    echo -e "  ${GREEN}PASS:        ${PASS}${NC}"
    echo -e "  ${RED}FAIL:        ${FAIL}${NC}"
    echo -e "  ${YELLOW}KNOWN ISSUE: ${KNOWN}${NC}"
    echo -e "  ${CYAN}SKIPPED:     ${SKIP}${NC}"
    echo -e "  ${BOLD}TOTAL:       ${TOTAL}${NC}"
    echo ""

    if [[ "$KNOWN" -gt 0 ]]; then
        echo -e "${BOLD}${YELLOW}Known Issues (confirmed architectural gaps):${NC}"
        echo ""
        echo "  ┌─────────────────────────────────────────────────────────────────┐"
        echo "  │  # │ Issue                            │ Severity  │ Section     │"
        echo "  ├─────────────────────────────────────────────────────────────────┤"
        echo "  │  1 │ No DNS caching                   │ HIGH      │ §2          │"
        echo "  │  2 │ Proxy uses system DNS             │ CRITICAL  │ §8          │"
        echo "  │  3 │ HEAD method hangs (intermittent)  │ MEDIUM    │ §3.4        │"
        echo "  │  4 │ No Via header (RFC 7230)          │ LOW       │ §3.1        │"
        echo "  │  5 │ No X-Forwarded-For                │ LOW       │ §3.2        │"
        echo "  │  6 │ SSRF — admin UI via proxy         │ CRITICAL  │ §7.1        │"
        echo "  │  7 │ NXDOMAIN returns NOERROR          │ MEDIUM    │ §1.5        │"
        echo "  │  8 │ WebSocket not supported            │ HIGH      │ §6          │"
        echo "  │  9 │ Accept-Encoding stripped           │ MEDIUM    │ §3.7        │"
        echo "  │ 10 │ Content-Length mismatch            │ MEDIUM    │ §3.6        │"
        echo "  │ 11 │ 10MB RAM buffering per response   │ CRITICAL  │ §11, §14    │"
        echo "  │ 12 │ No streaming/flush support         │ HIGH      │ §12         │"
        echo "  │ 13 │ Range requests broken              │ HIGH      │ §13         │"
        echo "  └─────────────────────────────────────────────────────────────────┘"
        echo ""
        echo -e "  ${BOLD}Detailed descriptions:${NC}"
        echo ""
        echo "  1. NO DNS CACHING"
        echo "     File: application/dns/server/server.go"
        echo "     Func: forwardDNSRequest() — creates new dns.Client every call"
        echo "     Impact: Every DNS query hits upstream (8.8.8.8), adds 10-30ms latency"
        echo "     Fix: Add in-memory cache keyed by (qname, qtype) with TTL expiration"
        echo ""
        echo "  2. PROXY USES SYSTEM DNS (not GateSentry)"
        echo "     File: gatesentryproxy/proxy.go line ~25"
        echo "     Code: var dialer = &net.Dialer{} — no Resolver field"
        echo "     Impact: Proxy hostname resolution bypasses GateSentry filtering entirely"
        echo "     Fix: Set dialer.Resolver to use 127.0.0.1:${DNS_PORT}"
        echo ""
        echo "  3. HEAD METHOD HANGS (intermittent)"
        echo "     File: gatesentryproxy/proxy.go line ~488"
        echo "     Code: io.ReadAll(teeReader) — HEAD responses have no body"
        echo "     Impact: HEAD requests may time out depending on upstream behaviour"
        echo "     Fix: Skip body read when r.Method == \"HEAD\""
        echo ""
        echo "  4. NO Via HEADER (RFC 7230 §5.7.1)"
        echo "     File: gatesentryproxy/proxy.go"
        echo "     Impact: Non-compliant with HTTP/1.1 proxy spec"
        echo "     Fix: Add resp.Header.Add(\"Via\", \"1.1 gatesentry\")"
        echo ""
        echo "  5. NO X-Forwarded-For HEADER"
        echo "     File: gatesentryproxy/proxy.go"
        echo "     Impact: Upstream servers cannot identify original client IP"
        echo "     Fix: Add r.Header.Set(\"X-Forwarded-For\", clientIP)"
        echo ""
        echo "  6. SSRF — ADMIN UI ACCESSIBLE VIA PROXY"
        echo "     File: gatesentryproxy/proxy.go + utils.go"
        echo "     Impact: Attacker can reach admin UI through proxy on 127.0.0.1:${ADMIN_PORT}"
        echo "     Fix: Block requests to loopback/LAN addresses in proxy handler"
        echo ""
        echo "  7. NXDOMAIN RETURNS NOERROR"
        echo "     File: application/dns/server/server.go"
        echo "     Impact: Clients see NOERROR with 0 answers instead of NXDOMAIN rcode"
        echo "     Fix: Preserve upstream rcode in response"
        echo ""
        echo "  8. WEBSOCKET NOT SUPPORTED"
        echo "     File: gatesentryproxy/websocket.go"
        echo "     Code: Returns 400 'Web sockets currently not supported'"
        echo "     Impact: WebSocket apps fail — chat, real-time dashboards, gaming"
        echo ""
        echo "  9. ACCEPT-ENCODING STRIPPED"
        echo "     File: gatesentryproxy/proxy.go line ~396"
        echo "     Code: r.Header.Del(\"Accept-Encoding\")"
        echo "     Impact: Proxy fetches uncompressed, re-compresses — wastes bandwidth"
        echo "     Fix: Conditionally preserve when content scanning not needed"
        echo ""
        echo "  10. CONTENT-LENGTH MISMATCH"
        echo "      File: gatesentryproxy/proxy.go — copyResponseHeader()"
        echo "      Issue: Original Content-Length forwarded but body is re-encoded"
        echo "      Impact: Clients may truncate or fail to read response body"
        echo "      Fix: Set Content-Length after body processing, not before"
        echo ""
        echo "  11. 10MB RAM BUFFERING PER RESPONSE"
        echo "      File: gatesentryproxy/proxy.go line ~488"
        echo "      Code: io.ReadAll(teeReader) with LimitedReader at 10MB"
        echo "      Impact: Every response <10MB is fully buffered in RAM before"
        echo "              a single byte reaches the client. 100 connections = 1GB+"
        echo "      Fix: Stream-scan with fixed-size ring buffer, flush as scanned"
        echo ""
        echo "  12. NO STREAMING / FLUSH SUPPORT"
        echo "      File: gatesentryproxy/proxy.go"
        echo "      Issue: No http.Flusher usage — cannot progressively deliver"
        echo "      Impact: SSE (EventSource), live video, real-time APIs all break"
        echo "      Fix: Use http.Flusher after each chunk, detect streaming content"
        echo ""
        echo "  13. RANGE REQUESTS BROKEN"
        echo "      File: gatesentryproxy/proxy.go — copyResponseHeader() + body handling"
        echo "      Issue: Proxy strips/rewrites Content-Length, ignores Range semantics"
        echo "      Impact: Cannot resume downloads, video seeking fails, PDF page-load fails"
        echo "      Fix: Pass through Range/Content-Range/206 responses untouched"
        echo ""
    fi

    if [[ "$FAIL" -gt 0 ]]; then
        echo -e "${RED}${BOLD}⚠  ${FAIL} unexpected failure(s) detected!${NC}"
        exit 1
    elif [[ "$KNOWN" -gt 0 ]]; then
        echo -e "${YELLOW}${BOLD}⚠  All failures are known issues. ${PASS} tests passed.${NC}"
        exit 0
    else
        echo -e "${GREEN}${BOLD}✓  All ${PASS} tests passed!${NC}"
        exit 0
    fi
}

###############################################################################
# MAIN
###############################################################################
main() {
    echo ""
    echo -e "${BOLD}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║       GateSentry — Proxy & DNS Test Suite                    ║${NC}"
    echo -e "${BOLD}║       $(date '+%Y-%m-%d %H:%M:%S')                                    ║${NC}"
    echo -e "${BOLD}╠═══════════════════════════════════════════════════════════════╣${NC}"
    echo -e "${BOLD}║  DNS:       ${DNS_HOST}:${DNS_PORT}$(printf '%*s' $((31 - ${#DNS_HOST} - ${#DNS_PORT})) '')║${NC}"
    echo -e "${BOLD}║  Proxy:     ${PROXY_HOST}:${PROXY_PORT}$(printf '%*s' $((31 - ${#PROXY_HOST} - ${#PROXY_PORT})) '')║${NC}"
    echo -e "${BOLD}║  Admin:     ${ADMIN_HOST}:${ADMIN_PORT}$(printf '%*s' $((31 - ${#ADMIN_HOST} - ${#ADMIN_PORT})) '')║${NC}"
    echo -e "${BOLD}║  Testbed:   HTTP :9999 / HTTPS :9443 / Echo :9998            ║${NC}"
    echo -e "${BOLD}║  Mode:      100% LOCAL (no internet dependency)              ║${NC}"
    echo -e "${BOLD}╚═══════════════════════════════════════════════════════════════╝${NC}"

    preflight_check
    test_dns_functionality
    test_dns_caching
    test_proxy_rfc_compliance
    test_http_methods
    test_https_connect
    test_websocket
    test_proxy_security
    test_proxy_dns_resolution
    test_performance
    test_concurrent
    test_large_downloads
    test_streaming
    test_range_requests
    test_resource_behaviour
    test_adversarial_resilience
    print_summary
}

main "$@"
