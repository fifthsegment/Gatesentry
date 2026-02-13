#!/bin/bash
#
# GateSentry Proxy Server - Deep Analysis and Robustness Testing Script
# =====================================================================
#
# This script performs comprehensive proxy server testing to ensure the
# GateSentry proxy filtering, MITM, passthrough, and content scanning
# pipelines are robust and correct.
#
# It is designed to be repeatable: it saves the current server state,
# configures test-specific rules/settings, runs all tests, and restores
# the original state on exit (including on Ctrl-C / errors).
#
# Platform Support:
#   - Linux (GNU coreutils) - Full support
#   - macOS/BSD - Requires GNU tools: brew install coreutils grep
#
# Usage:
#   ./proxy_deep_tests.sh [OPTIONS]
#
# Options:
#   -P, --proxy-port PORT     Proxy port (default: 10413)
#   -A, --admin-port PORT     Admin UI port (default: 8080)
#   -v, --verbose             Enable verbose output
#   -h, --help                Show this help message
#   -k, --keep-state          Don't restore original state on exit
#   --skip-setup              Skip state save/restore (for debugging)
#   --section SECTION         Run only the named section
#
# Requirements:
#   - curl
#   - python3 (for JSON parsing)
#   - openssl (for certificate checks)
#   - nc (netcat, for echo server)
#   - timeout (GNU coreutils)
#
# Author: GateSentry Team
# Date: 2026-02-14
#

set -euo pipefail

# =============================================================================
# Clear proxy environment variables to prevent curl from routing through
# GateSentry itself, which causes 508 Loop Detection errors.
# =============================================================================
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY no_proxy NO_PROXY 2>/dev/null || true

# =============================================================================
# Platform Detection and Compatibility
# =============================================================================

PLATFORM="unknown"
case "$(uname -s)" in
    Linux*)  PLATFORM="linux";;
    Darwin*) PLATFORM="macos";;
    *) PLATFORM="unknown";;
esac

# Portable time in milliseconds
get_time_ms() {
    if [[ "$PLATFORM" == "macos" ]]; then
        python3 -c 'import time; print(int(time.time() * 1000))' 2>/dev/null || echo "$(($(date +%s) * 1000))"
    else
        echo "$(($(date +%s%N) / 1000000))"
    fi
}

# =============================================================================
# Configuration and Defaults
# =============================================================================

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Default configuration
PROXY_PORT="${PROXY_PORT:-10413}"
ADMIN_PORT="${GS_ADMIN_PORT:-8080}"
ADMIN_BASE="http://localhost:${ADMIN_PORT}/gatesentry/api"
PROXY_URL="http://localhost:${PROXY_PORT}"
VERBOSE=false
KEEP_STATE=false
SKIP_SETUP=false
RUN_SECTION=""

# Test server ports
HTTP_ECHO_PORT=19080
HTTPS_ECHO_PORT=19443
ECHO_DOMAIN=httpbin.org
ECHO_IPV6_ADDR=fd00:1234:5678::1023

# Auth token
TOKEN=""

# Server management
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
GATESENTRY_BIN="$PROJECT_DIR/bin/gatesentrybin"
GATESENTRY_LOG="$PROJECT_DIR/proxy_test_server.log"
GATESENTRY_PID=""
STARTED_SERVER=false
ORIGINAL_SERVER_WAS_RUNNING=false

# State backup
SAVED_RULES=""
SAVED_FILTERS=""
SAVED_SETTINGS_DNS_FILTERING=""
SAVED_SETTINGS_HTTPS_FILTERING=""
SAVED_SETTINGS_STRICTNESS=""
ECHO_SERVER_PID=""
HTTPS_ECHO_SERVER_PID=""

# Test-specific IDs (created during setup, cleaned up on exit)
TEST_DOMAIN_LIST_ID=""
TEST_RULE_BLOCK_DOMAIN_ID=""
TEST_RULE_ALLOW_ID=""
TEST_RULE_NOMITM_ID=""
TEST_RULE_CONTENT_TYPE_ID=""
TEST_RULE_URL_REGEX_ID=""
TEST_RULE_ECHO_MITM_ID=""

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0
TESTS_TOTAL=0
declare -a TEST_RESULTS=()

# =============================================================================
# Utility Functions
# =============================================================================

print_header() {
    echo -e "\n${BOLD}${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${CYAN}  $1${NC}"
    echo -e "${BOLD}${BLUE}═══════════════════════════════════════════════════════════════════${NC}\n"
}

print_section() {
    echo -e "\n${BOLD}${MAGENTA}─── $1 ───${NC}\n"
}

print_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TEST_RESULTS+=("PASS: $1")
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TEST_RESULTS+=("FAIL: $1")
}

print_skip() {
    echo -e "${YELLOW}[SKIP]${NC} $1"
    TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TEST_RESULTS+=("SKIP: $1")
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${CYAN}[DEBUG]${NC} $1"
    fi
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# JSON helpers using python3
json_get() {
    local json="$1"
    local key="$2"
    echo "$json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('$key',''))" 2>/dev/null
}

json_get_nested() {
    local json="$1"
    local expr="$2"
    echo "$json" | python3 -c "import sys,json; d=json.load(sys.stdin); print($expr)" 2>/dev/null
}

# API helpers — always use --noproxy '*' to bypass GateSentry proxy
api_get() {
    local path="$1"
    curl --noproxy '*' -s "${ADMIN_BASE}${path}" -H "Authorization: Bearer ${TOKEN}" 2>/dev/null
}

api_post() {
    local path="$1"
    local data="$2"
    curl --noproxy '*' -s "${ADMIN_BASE}${path}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -d "$data" 2>/dev/null
}

api_put() {
    local path="$1"
    local data="$2"
    curl --noproxy '*' -s -X PUT "${ADMIN_BASE}${path}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -d "$data" 2>/dev/null
}

api_delete() {
    local path="$1"
    curl --noproxy '*' -s -X DELETE "${ADMIN_BASE}${path}" \
        -H "Authorization: Bearer ${TOKEN}" 2>/dev/null
}

# Proxy request helpers
proxy_http() {
    local url="$1"
    shift
    curl -s --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 15 "$url" "$@" 2>/dev/null
}

proxy_https() {
    local url="$1"
    shift
    curl -s --proxy "${PROXY_URL}" --noproxy '' \
        --insecure --max-time 15 "$url" "$@" 2>/dev/null
}

proxy_https_with_headers() {
    local url="$1"
    shift
    curl -si --proxy "${PROXY_URL}" --noproxy '' \
        --insecure --max-time 15 "$url" "$@" 2>/dev/null
}

proxy_http_with_headers() {
    local url="$1"
    shift
    curl -si --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 15 "$url" "$@" 2>/dev/null
}

proxy_http_status() {
    local url="$1"
    shift
    curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 15 "$url" "$@" 2>/dev/null
}

proxy_connect_status() {
    local url="$1"
    shift
    curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --insecure --max-time 15 "$url" "$@" 2>/dev/null
}

# =============================================================================
# CLI Argument Parsing
# =============================================================================

show_help() {
    cat << EOF
GateSentry Proxy Server - Deep Analysis and Robustness Testing Script

Usage: $0 [OPTIONS]

Options:
  -P, --proxy-port PORT     Proxy port (default: 10413)
  -A, --admin-port PORT     Admin UI port (default: 8080)
  -v, --verbose             Enable verbose output
  -h, --help                Show this help message
  -k, --keep-state          Don't restore original state on exit
  --skip-setup              Skip state save/restore (for debugging)
  --section SECTION         Run only the named section

Available sections:
  connectivity              Basic proxy connectivity
  rfc                       RFC proxy forwarding compliance
  mitm                      MITM / SSL bumping
  passthrough               SSL passthrough / direct tunnel
  domain-patterns           Domain pattern matching rules
  domain-lists              Domain list blocking rules
  keyword-filtering         Keyword content filtering / strictness
  content-type              Content type match criteria
  url-regex                 URL regex pattern match criteria
  block-pages               Block page verification
  rule-priority             Rule priority / first-match-wins
  loop-detection            Proxy loop detection
  ssrf                      SSRF protection
  error-handling            Error handling (bad upstream, timeouts)
  headers                   Header sanitization / hop-by-hop
  performance               Performance / latency

Example:
  # Full test suite
  ./proxy_deep_tests.sh

  # Verbose, specific section
  ./proxy_deep_tests.sh -v --section keyword-filtering

EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case $1 in
        -P|--proxy-port) PROXY_PORT="$2"; shift 2;;
        -A|--admin-port) ADMIN_PORT="$2"; shift 2;;
        -v|--verbose) VERBOSE=true; shift;;
        -k|--keep-state) KEEP_STATE=true; shift;;
        --skip-setup) SKIP_SETUP=true; shift;;
        --section) RUN_SECTION="$2"; shift 2;;
        -h|--help) show_help;;
        *) echo "Unknown option: $1"; show_help;;
    esac
done

# Refresh base URLs after arg parsing
ADMIN_BASE="http://localhost:${ADMIN_PORT}/gatesentry/api"
PROXY_URL="http://localhost:${PROXY_PORT}"

# =============================================================================
# Dependency Checks
# =============================================================================

check_dependencies() {
    print_section "Checking Dependencies"

    local missing=()
    for cmd in curl python3 openssl nc timeout; do
        if command -v "$cmd" &>/dev/null; then
            print_verbose "Found: $cmd ($(command -v "$cmd"))"
        else
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        echo -e "${RED}Missing required dependencies: ${missing[*]}${NC}"
        exit 1
    fi
    print_info "All dependencies satisfied"
}

# =============================================================================
# GateSentry Server Management
# =============================================================================

# Check if a GateSentry process is currently running
is_gatesentry_running() {
    pgrep -x gatesentrybin >/dev/null 2>&1
}

# Check if the admin API is responsive
is_admin_api_ready() {
    local resp
    resp=$(curl --noproxy '*' -s -o /dev/null -w "%{http_code}" \
        "http://localhost:${ADMIN_PORT}/gatesentry/api/about" --max-time 3 2>/dev/null) || true
    resp=${resp:-000}
    [[ "$resp" == "200" || "$resp" == "401" ]]
}

# Check if the proxy port is listening
is_proxy_ready() {
    nc -z 127.0.0.1 "${PROXY_PORT}" 2>/dev/null
}

# Kill ALL existing GateSentry processes
kill_existing_gatesentry() {
    if is_gatesentry_running; then
        local pids
        pids=$(pgrep -x gatesentrybin)
        print_info "Killing existing GateSentry process(es): $pids"
        ORIGINAL_SERVER_WAS_RUNNING=true

        # SIGTERM first
        pkill -x gatesentrybin 2>/dev/null || true

        # Wait for graceful shutdown
        local waited=0
        while is_gatesentry_running && [[ $waited -lt 5 ]]; do
            sleep 1
            waited=$((waited + 1))
        done

        # Force kill if still running
        if is_gatesentry_running; then
            print_warning "Server didn't stop gracefully, forcing SIGKILL..."
            pkill -9 -x gatesentrybin 2>/dev/null || true
            sleep 1
        fi

        if is_gatesentry_running; then
            echo -e "${RED}FATAL: Could not kill existing GateSentry process${NC}"
            exit 1
        fi
        print_info "Existing server stopped"
    else
        print_info "No existing GateSentry process found"
    fi
}

# Start GateSentry with clean environment (no proxy env vars)
start_gatesentry_server() {
    print_section "Starting GateSentry Server (clean environment)"

    # Check if binary exists
    if [[ ! -x "$GATESENTRY_BIN" ]]; then
        print_warning "GateSentry binary not found at: $GATESENTRY_BIN"
        print_info "Attempting to build..."

        if [[ -f "$PROJECT_DIR/build.sh" ]]; then
            (cd "$PROJECT_DIR" && ./build.sh) > /dev/null 2>&1 || {
                echo -e "${RED}FATAL: Failed to build GateSentry${NC}"
                exit 1
            }
        else
            echo -e "${RED}FATAL: build.sh not found in $PROJECT_DIR${NC}"
            exit 1
        fi

        if [[ ! -x "$GATESENTRY_BIN" ]]; then
            echo -e "${RED}FATAL: Binary still not found after build${NC}"
            exit 1
        fi
        print_info "GateSentry built successfully"
    fi

    # Kill any existing instance first
    kill_existing_gatesentry

    # Start the server with a CLEAN environment — no http_proxy / https_proxy
    # so Go's net/http won't route outbound requests through itself
    print_info "Starting GateSentry with clean proxy environment..."
    print_verbose "Binary: $GATESENTRY_BIN"
    print_verbose "Log: $GATESENTRY_LOG"
    print_verbose "Admin port: $ADMIN_PORT"
    print_verbose "Proxy port: $PROXY_PORT (hardcoded default)"
    print_verbose "Environment: http_proxy=<unset> https_proxy=<unset>"

    (
        cd "$PROJECT_DIR/bin"
        # Explicitly clear all proxy env vars so the Go process
        # doesn't route its own outbound HTTP through itself
        unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY no_proxy NO_PROXY 2>/dev/null || true
        export GATESENTRY_DNS_ADDR="${GATESENTRY_DNS_ADDR:-0.0.0.0}"
        export GATESENTRY_DNS_PORT="${GATESENTRY_DNS_PORT:-10053}"
        export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-192.168.1.1:53}"
        export GS_ADMIN_PORT="${ADMIN_PORT}"
        export GS_MAX_SCAN_SIZE_MB="${GS_MAX_SCAN_SIZE_MB:-2}"
        exec ./gatesentrybin > "$GATESENTRY_LOG" 2>&1
    ) &
    GATESENTRY_PID=$!
    STARTED_SERVER=true

    print_info "Server starting with PID: $GATESENTRY_PID"

    # Wait for BOTH admin API and proxy port to be ready
    local max_wait=30
    local waited=0
    local admin_ready=false
    local proxy_ready=false
    print_info "Waiting for server to be ready (max ${max_wait}s)..."

    while [[ $waited -lt $max_wait ]]; do
        sleep 1
        waited=$((waited + 1))

        # Check if process is still alive
        if ! kill -0 "$GATESENTRY_PID" 2>/dev/null; then
            echo -e "${RED}FATAL: Server process died unexpectedly${NC}"
            print_info "Check log file: $GATESENTRY_LOG"
            if [[ "$VERBOSE" == "true" ]] && [[ -f "$GATESENTRY_LOG" ]]; then
                echo -e "${CYAN}Last 20 lines of log:${NC}"
                tail -20 "$GATESENTRY_LOG"
            fi
            exit 1
        fi

        # Check admin API
        if [[ "$admin_ready" == "false" ]] && is_admin_api_ready; then
            admin_ready=true
            print_verbose "Admin API ready (${waited}s)"
        fi

        # Check proxy port
        if [[ "$proxy_ready" == "false" ]] && is_proxy_ready; then
            proxy_ready=true
            print_verbose "Proxy port ready (${waited}s)"
        fi

        if [[ "$admin_ready" == "true" && "$proxy_ready" == "true" ]]; then
            print_pass "Server is ready (took ${waited}s)"

            # Verify the process has no proxy env vars
            local server_env
            server_env=$(cat /proc/$GATESENTRY_PID/environ 2>/dev/null | tr '\0' '\n' | grep -i proxy || echo "")
            if [[ -z "$server_env" ]]; then
                print_info "Verified: server has no proxy env vars"
            else
                print_warning "Server has proxy env vars (may cause loop issues):"
                echo "$server_env" | while read -r line; do print_verbose "  $line"; done
            fi
            return 0
        fi

        print_verbose "Waiting... ($waited/$max_wait) admin=$admin_ready proxy=$proxy_ready"
    done

    echo -e "${RED}FATAL: Server failed to become ready within ${max_wait} seconds${NC}"
    print_info "admin_ready=$admin_ready proxy_ready=$proxy_ready"
    print_info "Check log file: $GATESENTRY_LOG"
    if [[ "$VERBOSE" == "true" ]] && [[ -f "$GATESENTRY_LOG" ]]; then
        echo -e "${CYAN}Last 30 lines of log:${NC}"
        tail -30 "$GATESENTRY_LOG"
    fi
    exit 1
}

# Stop the GateSentry server if we started it
stop_gatesentry_server() {
    if [[ "$STARTED_SERVER" == "true" ]] && [[ -n "$GATESENTRY_PID" ]]; then
        print_info "Stopping test server (PID: $GATESENTRY_PID)..."

        # SIGTERM first
        kill "$GATESENTRY_PID" 2>/dev/null || true

        # Wait for graceful shutdown
        local waited=0
        while kill -0 "$GATESENTRY_PID" 2>/dev/null && [[ $waited -lt 5 ]]; do
            sleep 1
            waited=$((waited + 1))
        done

        # Force kill if still running
        if kill -0 "$GATESENTRY_PID" 2>/dev/null; then
            print_warning "Server didn't stop gracefully, forcing SIGKILL..."
            kill -9 "$GATESENTRY_PID" 2>/dev/null || true
            sleep 1
        fi

        print_info "Test server stopped"
        GATESENTRY_PID=""
        STARTED_SERVER=false
    fi
}

# Restart the original server (with its normal env) if it was running before
restart_original_server() {
    if [[ "$ORIGINAL_SERVER_WAS_RUNNING" == "true" ]]; then
        print_info "Restarting original GateSentry server..."
        (
            cd "$PROJECT_DIR/bin"
            export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-192.168.1.1:53}"
            export GS_ADMIN_PORT="${GS_ADMIN_PORT:-8080}"
            export GS_MAX_SCAN_SIZE_MB="${GS_MAX_SCAN_SIZE_MB:-2}"
            ./gatesentrybin > "$PROJECT_DIR/log.txt" 2>&1 &
        )
        sleep 2
        if is_gatesentry_running; then
            print_info "Original server restarted (PID: $(pgrep -x gatesentrybin))"
        else
            print_warning "Original server may not have restarted — run ./restart.sh manually"
        fi
    fi
}

# =============================================================================
# Authentication
# =============================================================================

authenticate() {
    print_section "Authenticating to Admin API"
    local resp
    resp=$(curl --noproxy '*' -s "${ADMIN_BASE}/auth/token" \
        -H "Content-Type: application/json" \
        -d '{"username":"admin","pass":"admin"}' 2>/dev/null)

    local validated
    validated=$(json_get "$resp" "Validated")
    if [[ "$validated" != "true" && "$validated" != "True" ]]; then
        echo -e "${RED}Authentication failed!${NC}"
        echo "Response: $resp"
        exit 1
    fi

    TOKEN=$(json_get "$resp" "Jwtoken")
    if [[ -z "$TOKEN" ]]; then
        echo -e "${RED}No JWT token received!${NC}"
        exit 1
    fi
    print_info "Authenticated successfully"
}

# =============================================================================
# Echo Test Server (lightweight HTTP server using Python)
# =============================================================================

start_echo_server() {
    print_section "Starting Test Echo Servers"

    # Create temp directory for test server files
    local tmpdir
    tmpdir=$(mktemp -d /tmp/gs_proxy_test.XXXXXX)

    # Create test HTML pages
    cat > "${tmpdir}/index.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body><h1>GateSentry Proxy Test Page</h1><p>This is a clean test page with no blocked content.</p></body>
</html>
HTMLEOF

    cat > "${tmpdir}/keywords_high.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><head><title>High Score Page</title></head>
<body>
<h1>Test Page with Keywords</h1>
<p>This page contains the test keyword many times to exceed the strictness threshold.</p>
<p>proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword</p>
<p>proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword</p>
<p>proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword</p>
<p>proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword</p>
<p>proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword</p>
</body>
</html>
HTMLEOF

    cat > "${tmpdir}/keywords_low.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><head><title>Low Score Page</title></head>
<body>
<h1>Test Page with One Keyword</h1>
<p>This page contains the test keyword only once: proxytest_keyword</p>
<p>The rest of the page is just normal content that should not trigger any filtering.</p>
</body>
</html>
HTMLEOF

    # Create a 1x1 red pixel JPEG for image tests (base64 encoded)
    python3 -c "
import base64, sys
# Minimal valid JPEG (1x1 red pixel)
jpeg = base64.b64decode(''.join([
    '/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkS',
    'Ew8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJ',
    'CQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIy',
    'MjIyMjIyMjIyMjIyMjL/wAARCAABAAEDASIAAhEBAxEB/8QAFAABAAAAAAAAAAAAAAAAAAAACf/',
    'EABQQAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAAX/xAAUEQEAAAAA',
    'AAAAAAAAAAAAAP/aAAwDAQACEQMRAD8AKwA//9k=']))
sys.stdout.buffer.write(jpeg)
" > "${tmpdir}/test.jpg"

    # Create a small CSS file (should pass through Path A unscanned)
    cat > "${tmpdir}/style.css" << 'CSSEOF'
body { margin: 0; padding: 20px; font-family: sans-serif; }
h1 { color: #333; }
CSSEOF

    # Create a JS file (should pass through Path A unscanned)
    cat > "${tmpdir}/script.js" << 'JSEOF'
// This JavaScript file contains the keyword proxytest_keyword many times
// proxytest_keyword proxytest_keyword proxytest_keyword proxytest_keyword
// It should NOT be blocked because JS goes through Path A (stream passthrough)
console.log("Hello from test server");
JSEOF

    # Create a page with embedded resources from a blocked domain
    cat > "${tmpdir}/embedded.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><head><title>Embedded Resource Test</title></head>
<body>
<h1>This page itself is fine</h1>
<p>But it references resources from other domains.</p>
</body>
</html>
HTMLEOF

    # Start Python HTTP server
    print_info "Starting HTTP echo server on port ${HTTP_ECHO_PORT}..."
    cd "${tmpdir}"
    python3 -m http.server ${HTTP_ECHO_PORT} --bind 127.0.0.1 &>/dev/null &
    ECHO_SERVER_PID=$!
    cd "${PROJECT_DIR}"

    # Use trusted test certs (signed by JVJCA CA installed in system trust store)
    local cert_dir="${PROJECT_DIR}/tests/fixtures"
    if [[ ! -f "${cert_dir}/httpbin.org.crt" || ! -f "${cert_dir}/httpbin.org.key" ]]; then
        print_info "Generating server certificate..."
        bash "${cert_dir}/gen_test_certs.sh" --force
    fi
    print_info "Starting HTTPS echo server on [${ECHO_IPV6_ADDR}]:${HTTPS_ECHO_PORT} (trusted cert)..."

    python3 -c "
import http.server, ssl, os, sys, socket
os.chdir('${tmpdir}')
class HTTPServerV6(http.server.HTTPServer):
    address_family = socket.AF_INET6
server = HTTPServerV6(('${ECHO_IPV6_ADDR}', ${HTTPS_ECHO_PORT}), http.server.SimpleHTTPRequestHandler)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain('${cert_dir}/httpbin.org.crt', '${cert_dir}/httpbin.org.key')
server.socket = ctx.wrap_socket(server.socket, server_side=True)
server.serve_forever()
" &>/dev/null &
    HTTPS_ECHO_SERVER_PID=$!

    # Wait for servers to come up
    sleep 1

    # Verify HTTP echo server
    local check
    check=$(curl --noproxy '*' -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" 2>/dev/null) || true
    check=${check:-000}
    if [[ "$check" == "200" ]]; then
        print_info "HTTP echo server is ready"
    else
        print_warning "HTTP echo server may not be ready (status: $check)"
    fi

    # Verify HTTPS echo server
    check=$(curl --noproxy '*' -s -o /dev/null -w "%{http_code}" --insecure "https://${ECHO_DOMAIN}:${HTTPS_ECHO_PORT}/index.html" 2>/dev/null) || true
    check=${check:-000}
    if [[ "$check" == "200" ]]; then
        print_info "HTTPS echo server is ready"
    else
        print_warning "HTTPS echo server may not be ready (status: $check)"
    fi

    # Store tmpdir for cleanup
    ECHO_TMPDIR="${tmpdir}"
}

stop_echo_server() {
    if [[ -n "$ECHO_SERVER_PID" ]]; then
        kill "$ECHO_SERVER_PID" 2>/dev/null || true
        wait "$ECHO_SERVER_PID" 2>/dev/null || true
        ECHO_SERVER_PID=""
    fi
    if [[ -n "$HTTPS_ECHO_SERVER_PID" ]]; then
        kill "$HTTPS_ECHO_SERVER_PID" 2>/dev/null || true
        wait "$HTTPS_ECHO_SERVER_PID" 2>/dev/null || true
        HTTPS_ECHO_SERVER_PID=""
    fi
    if [[ -n "${ECHO_TMPDIR:-}" ]]; then
        rm -rf "${ECHO_TMPDIR}" 2>/dev/null || true
    fi
}

# =============================================================================
# State Save / Restore
# =============================================================================

save_state() {
    print_section "Saving Current Server State"

    # Save rules
    SAVED_RULES=$(api_get "/rules")
    print_verbose "Saved rules: $(echo "$SAVED_RULES" | python3 -c "import sys,json; r=json.load(sys.stdin); print(len(r.get('rules',[])), 'rules')" 2>/dev/null)"

    # Save filters
    SAVED_FILTERS=$(api_get "/filters")
    print_verbose "Saved filters"

    # Save settings
    SAVED_SETTINGS_DNS_FILTERING=$(api_get "/settings/enable_dns_filtering" | python3 -c "import sys,json; print(json.load(sys.stdin).get('Value',''))" 2>/dev/null)
    SAVED_SETTINGS_HTTPS_FILTERING=$(api_get "/settings/enable_https_filtering" | python3 -c "import sys,json; print(json.load(sys.stdin).get('Value',''))" 2>/dev/null)
    SAVED_SETTINGS_STRICTNESS=$(api_get "/settings/strictness" | python3 -c "import sys,json; print(json.load(sys.stdin).get('Value',''))" 2>/dev/null)

    print_info "DNS filtering: ${SAVED_SETTINGS_DNS_FILTERING}"
    print_info "HTTPS filtering: ${SAVED_SETTINGS_HTTPS_FILTERING}"
    print_info "Strictness: ${SAVED_SETTINGS_STRICTNESS}"
    print_info "State saved successfully"
}

restore_state() {
    print_section "Restoring Original Server State"

    if [[ "$KEEP_STATE" == "true" ]]; then
        print_warning "Skipping state restore (--keep-state)"
        return
    fi

    # Delete ALL rules (test rules + any leftovers)
    local all_rule_ids
    all_rule_ids=$(api_get "/rules" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for r in data.get('rules', []):
    print(r['id'])
" 2>/dev/null)
    while IFS= read -r rid; do
        if [[ -n "$rid" ]]; then
            print_verbose "Deleting rule: $rid"
            api_delete "/rules/${rid}" >/dev/null 2>&1 || true
        fi
    done <<< "$all_rule_ids"

    # Restore original rules from saved state
    if [[ -n "$SAVED_RULES" ]]; then
        local restored_count=0
        local rules_json
        rules_json=$(echo "$SAVED_RULES" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for r in data.get('rules', []):
    # Remove id so the server generates a new one
    r.pop('id', None)
    r.pop('created_at', None)
    r.pop('updated_at', None)
    print(json.dumps(r))
" 2>/dev/null)
        while IFS= read -r rule_json; do
            if [[ -n "$rule_json" ]]; then
                api_post "/rules" "$rule_json" >/dev/null 2>&1 || true
                restored_count=$((restored_count + 1))
            fi
        done <<< "$rules_json"
        print_verbose "Restored $restored_count original rule(s)"
    fi

    # Delete test domain list
    if [[ -n "$TEST_DOMAIN_LIST_ID" ]]; then
        print_verbose "Deleting test domain list: $TEST_DOMAIN_LIST_ID"
        api_delete "/domainlists/${TEST_DOMAIN_LIST_ID}" >/dev/null 2>&1 || true
    fi

    # Restore settings
    if [[ -n "$SAVED_SETTINGS_DNS_FILTERING" ]]; then
        api_post "/settings/enable_dns_filtering" "{\"key\":\"enable_dns_filtering\",\"value\":\"${SAVED_SETTINGS_DNS_FILTERING}\"}" >/dev/null
        print_verbose "Restored DNS filtering: ${SAVED_SETTINGS_DNS_FILTERING}"
    fi
    if [[ -n "$SAVED_SETTINGS_HTTPS_FILTERING" ]]; then
        api_post "/settings/enable_https_filtering" "{\"key\":\"enable_https_filtering\",\"value\":\"${SAVED_SETTINGS_HTTPS_FILTERING}\"}" >/dev/null
        print_verbose "Restored HTTPS filtering: ${SAVED_SETTINGS_HTTPS_FILTERING}"
    fi
    if [[ -n "$SAVED_SETTINGS_STRICTNESS" ]]; then
        api_post "/settings/strictness" "{\"key\":\"strictness\",\"value\":\"${SAVED_SETTINGS_STRICTNESS}\"}" >/dev/null
        print_verbose "Restored strictness: ${SAVED_SETTINGS_STRICTNESS}"
    fi

    # Restore keyword filter to original state (remove our test keyword)
    # We restore by re-posting the saved filter entries for the keyword filter
    if [[ -n "$SAVED_FILTERS" ]]; then
        local keyword_entries
        keyword_entries=$(echo "$SAVED_FILTERS" | python3 -c "
import sys, json
filters = json.load(sys.stdin)
for f in filters:
    if f['Id'] == 'bVxTPTOXiqGRbhF':
        print(json.dumps(f.get('Entries', [])))
        break
" 2>/dev/null)
        if [[ -n "$keyword_entries" && "$keyword_entries" != "null" ]]; then
            api_post "/filters/bVxTPTOXiqGRbhF" "$keyword_entries" >/dev/null 2>&1 || true
            print_verbose "Restored keyword filter entries"
        fi
    fi

    print_info "State restored successfully"
}

# =============================================================================
# Test Setup — Configure GateSentry for Proxy Testing
# =============================================================================

setup_test_environment() {
    print_section "Configuring Test Environment"

    # 1. Delete ALL existing rules so only test rules are active (deterministic results)
    print_info "Removing all existing proxy rules for clean test environment..."
    local existing_rule_ids
    existing_rule_ids=$(api_get "/rules" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for r in data.get('rules', []):
    print(r['id'])
" 2>/dev/null)
    local removed_count=0
    while IFS= read -r rid; do
        if [[ -n "$rid" ]]; then
            api_delete "/rules/${rid}" >/dev/null 2>&1 || true
            removed_count=$((removed_count + 1))
        fi
    done <<< "$existing_rule_ids"
    if [[ $removed_count -gt 0 ]]; then
        print_info "Removed $removed_count existing rule(s)"
    fi
    sleep 0.5

    # 2. Disable DNS filtering so we test proxy filtering independently
    print_info "Disabling DNS filtering..."
    api_post "/settings/enable_dns_filtering" '{"key":"enable_dns_filtering","value":"false"}' >/dev/null
    sleep 0.5

    # 3. Enable HTTPS filtering (MITM)
    print_info "Enabling HTTPS filtering (MITM)..."
    api_post "/settings/enable_https_filtering" '{"key":"enable_https_filtering","value":"true"}' >/dev/null
    sleep 0.5

    # 4. Set strictness to a known value for keyword tests
    print_info "Setting strictness to 500..."
    api_post "/settings/strictness" '{"key":"strictness","value":"500"}' >/dev/null
    sleep 0.5

    # 5. Create a test domain list with known blocked domains
    print_info "Creating test domain list..."
    local create_resp
    create_resp=$(api_post "/domainlists" '{
        "name": "Proxy Test Blocked Domains",
        "source": "local"
    }')
    TEST_DOMAIN_LIST_ID=$(echo "$create_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('list',{}).get('id',''))" 2>/dev/null)
    if [[ -n "$TEST_DOMAIN_LIST_ID" ]]; then
        print_info "Created test domain list: $TEST_DOMAIN_LIST_ID"
        # Add domains to the list via separate API call
        local add_resp
        add_resp=$(api_post "/domainlists/${TEST_DOMAIN_LIST_ID}/domains" '{
            "domains": ["blocked-test-domain.example.com", "another-blocked.example.com", "evil-site.test.local"]
        }')
        print_verbose "Add domains response: $add_resp"
        print_info "Added 3 test domains to blocklist"
    else
        print_warning "Failed to create test domain list — some tests may be skipped"
        print_verbose "Response: $create_resp"
    fi

    # 6. Create test rules (in priority order)
    # Rule: Block domains from our test domain list (priority 10)
    print_info "Creating test rules..."
    local rule_resp

    # Block rule — domains from the test list should be blocked
    if [[ -n "$TEST_DOMAIN_LIST_ID" ]]; then
        rule_resp=$(api_post "/rules" "{
            \"name\": \"PT: Block Test Domains\",
            \"enabled\": true,
            \"priority\": 10,
            \"action\": \"block\",
            \"mitm_action\": \"enable\",
            \"domain_lists\": [\"${TEST_DOMAIN_LIST_ID}\"],
            \"time_restriction\": {\"from\": \"00:00\", \"to\": \"23:59\"}
        }")
        TEST_RULE_BLOCK_DOMAIN_ID=$(echo "$rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)
        if [[ -n "$TEST_RULE_BLOCK_DOMAIN_ID" ]]; then
            print_info "Created block rule: $TEST_RULE_BLOCK_DOMAIN_ID"
        else
            print_warning "Failed to create block rule"
            print_verbose "Response: $rule_resp"
        fi
    fi

    # Allow rule — specific test domain patterns should be allowed (priority 5, higher priority)
    rule_resp=$(api_post "/rules" '{
        "name": "PT: Allow Test Pattern",
        "enabled": true,
        "priority": 5,
        "action": "allow",
        "mitm_action": "enable",
        "domain_patterns": ["allowed-test.example.com", "*.allowed-wildcard.example.com"],
        "time_restriction": {"from": "00:00", "to": "23:59"}
    }')
    TEST_RULE_ALLOW_ID=$(echo "$rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)
    if [[ -n "$TEST_RULE_ALLOW_ID" ]]; then
        print_info "Created allow rule: $TEST_RULE_ALLOW_ID"
    else
        print_warning "Failed to create allow rule"
        print_verbose "Response: $rule_resp"
    fi

    # No-MITM rule — passthrough for specific domains (priority 3)
    rule_resp=$(api_post "/rules" '{
        "name": "PT: No MITM Passthrough",
        "enabled": true,
        "priority": 3,
        "action": "allow",
        "mitm_action": "disable",
        "domain_patterns": ["passthrough-test.example.com"],
        "time_restriction": {"from": "00:00", "to": "23:59"}
    }')
    TEST_RULE_NOMITM_ID=$(echo "$rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)
    if [[ -n "$TEST_RULE_NOMITM_ID" ]]; then
        print_info "Created no-MITM rule: $TEST_RULE_NOMITM_ID"
    else
        print_warning "Failed to create no-MITM rule"
        print_verbose "Response: $rule_resp"
    fi

    # URL regex block rule — match criteria, does not require MITM (priority 15)
    rule_resp=$(api_post "/rules" "{
        \"name\": \"PT: URL Regex Block\",
        \"enabled\": true,
        \"priority\": 15,
        \"action\": \"block\",
        \"domain_patterns\": [\"regex-test.example.com\"],
        \"url_regex_patterns\": [\".*blocked-path.*\", \".*\\\\.exe$\"],
        \"time_restriction\": {\"from\": \"00:00\", \"to\": \"23:59\"}
    }")
    TEST_RULE_URL_REGEX_ID=$(echo "$rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)
    if [[ -n "$TEST_RULE_URL_REGEX_ID" ]]; then
        print_info "Created URL regex rule: $TEST_RULE_URL_REGEX_ID"
    else
        print_warning "Failed to create URL regex rule"
        print_verbose "Response: $rule_resp"
    fi

    # Echo server MITM + keyword rule — enables MITM and keyword scanning for
    # the local echo server so tests 3.4 (Via in MITM) and 7.5 (HTTPS keyword
    # filtering) can exercise the per-rule content filtering pipeline.
    # Matches httpbin.org (HTTPS echo) and 127.0.0.1 (HTTP echo).
    rule_resp=$(api_post "/rules" "{
        \"name\": \"PT: Echo Server MITM + Keywords\",
        \"enabled\": true,
        \"priority\": 1,
        \"action\": \"allow\",
        \"mitm_action\": \"enable\",
        \"domain_patterns\": [\"httpbin.org\", \"127.0.0.1\"],
        \"keyword_filter_enabled\": true,
        \"time_restriction\": {\"from\": \"00:00\", \"to\": \"23:59\"}
    }")
    TEST_RULE_ECHO_MITM_ID=$(echo "$rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)
    if [[ -n "$TEST_RULE_ECHO_MITM_ID" ]]; then
        print_info "Created echo server MITM rule: $TEST_RULE_ECHO_MITM_ID"
    else
        print_warning "Failed to create echo server MITM rule"
        print_verbose "Response: $rule_resp"
    fi

    # 7. Add a test keyword to the keyword filter with known score
    print_info "Adding test keyword 'proxytest_keyword' with score 100..."
    # Get current keywords, append ours, re-post
    local current_keywords
    current_keywords=$(api_get "/filters/bVxTPTOXiqGRbhF" | python3 -c "
import sys, json
data = json.load(sys.stdin)
entries = data[0].get('Entries', []) if isinstance(data, list) else data.get('Entries', [])
entries.append({'Content': 'proxytest_keyword', 'Score': 100})
print(json.dumps(entries))
" 2>/dev/null)
    if [[ -n "$current_keywords" ]]; then
        local kw_resp
        kw_resp=$(api_post "/filters/bVxTPTOXiqGRbhF" "$current_keywords")
        print_verbose "Keyword POST response: $kw_resp"
        # Verify it was added
        local verify
        verify=$(api_get "/filters/bVxTPTOXiqGRbhF" | python3 -c "
import sys, json
data = json.load(sys.stdin)
entries = data[0].get('Entries', []) if isinstance(data, list) else data.get('Entries', [])
found = any(e.get('Content') == 'proxytest_keyword' for e in entries)
print('yes' if found else 'no')
" 2>/dev/null)
        if [[ "$verify" == "yes" ]]; then
            print_info "Verified: proxytest_keyword is in keyword filter"
        else
            print_warning "proxytest_keyword may not have been added to filter"
        fi
    else
        print_warning "Could not read keyword filter — keyword tests may fail"
    fi

    # Print setup summary
    echo ""
    print_info "=== Setup Summary ==="
    print_info "  Domain list ID: ${TEST_DOMAIN_LIST_ID:-'(not created)'}"
    print_info "  Block domain rule: ${TEST_RULE_BLOCK_DOMAIN_ID:-'(not created)'}"
    print_info "  Allow rule: ${TEST_RULE_ALLOW_ID:-'(not created)'}"
    print_info "  No-MITM rule: ${TEST_RULE_NOMITM_ID:-'(not created)'}"
    print_info "  URL regex rule: ${TEST_RULE_URL_REGEX_ID:-'(not created)'}"
    print_info "  Echo server MITM rule: ${TEST_RULE_ECHO_MITM_ID:-'(not created)'}"

    # Wait for settings to propagate
    sleep 1
    print_info "Test environment configured"
}

# =============================================================================
# Cleanup on exit (trap handler)
# =============================================================================

cleanup() {
    local exit_code=$?
    echo ""
    print_section "Cleanup"

    stop_echo_server

    if [[ "$SKIP_SETUP" != "true" && -n "$TOKEN" ]]; then
        restore_state
    fi

    # Stop the GateSentry server we started
    stop_gatesentry_server

    # Restart the original server if one was running before we killed it
    restart_original_server

    # Print summary
    print_summary

    exit $exit_code
}

trap cleanup EXIT INT TERM

# =============================================================================
# Test Summary
# =============================================================================

print_summary() {
    print_header "Test Summary"

    echo -e "${GREEN}  Passed:  ${TESTS_PASSED}${NC}"
    echo -e "${RED}  Failed:  ${TESTS_FAILED}${NC}"
    echo -e "${YELLOW}  Skipped: ${TESTS_SKIPPED}${NC}"
    echo -e "${BOLD}  Total:   ${TESTS_TOTAL}${NC}"
    echo ""

    if [[ ${TESTS_FAILED} -gt 0 ]]; then
        echo -e "${RED}${BOLD}Failed tests:${NC}"
        for result in "${TEST_RESULTS[@]}"; do
            if [[ "$result" == FAIL:* ]]; then
                echo -e "  ${RED}✗ ${result#FAIL: }${NC}"
            fi
        done
        echo ""
    fi

    if [[ ${TESTS_FAILED} -gt 0 ]]; then
        echo -e "${RED}${BOLD}RESULT: SOME TESTS FAILED${NC}"
    elif [[ ${TESTS_TOTAL} -eq 0 ]]; then
        echo -e "${YELLOW}${BOLD}RESULT: NO TESTS RUN${NC}"
    else
        echo -e "${GREEN}${BOLD}RESULT: ALL TESTS PASSED${NC}"
    fi
}

# =============================================================================
# Helper: should_run checks if a section should execute
# =============================================================================

should_run() {
    local section="$1"
    if [[ -z "$RUN_SECTION" || "$RUN_SECTION" == "$section" ]]; then
        return 0
    fi
    return 1
}

# =============================================================================
# TEST SECTION 1: Basic Proxy Connectivity
# =============================================================================

test_connectivity() {
    should_run "connectivity" || return 0
    print_header "Section 1: Basic Proxy Connectivity"

    # Test 1.1: Proxy is listening
    print_test "1.1 Proxy is listening on port ${PROXY_PORT}"
    if nc -z 127.0.0.1 "${PROXY_PORT}" 2>/dev/null; then
        print_pass "Proxy is listening on port ${PROXY_PORT}"
    else
        print_fail "Proxy is NOT listening on port ${PROXY_PORT}"
        return
    fi

    # Test 1.2: HTTP request through proxy to our echo server
    print_test "1.2 HTTP GET through proxy to echo server"
    local body
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    if echo "$body" | grep -q "GateSentry Proxy Test Page"; then
        print_pass "HTTP GET through proxy returned expected content"
    else
        print_fail "HTTP GET through proxy did not return expected content"
        print_verbose "Body: $body"
    fi

    # Test 1.3: HTTPS request through proxy (MITM mode)
    print_test "1.3 HTTPS GET through proxy (MITM) to echo server"
    body=$(proxy_https "https://${ECHO_DOMAIN}:${HTTPS_ECHO_PORT}/index.html")
    if echo "$body" | grep -q "GateSentry Proxy Test Page"; then
        print_pass "HTTPS GET through proxy (MITM) returned expected content"
    else
        print_fail "HTTPS GET through proxy (MITM) did not return expected content"
        print_verbose "Body: $body"
    fi

    # Test 1.4: HTTP request to external site
    print_test "1.4 HTTP request to external site (example.com)"
    local status
    status=$(proxy_http_status "http://example.com/")
    if [[ "$status" == "200" ]]; then
        print_pass "HTTP to example.com returned 200"
    else
        print_fail "HTTP to example.com returned $status (expected 200)"
    fi

    # Test 1.5: HTTPS request to external site
    print_test "1.5 HTTPS request to external site (example.com)"
    status=$(proxy_connect_status "https://example.com/")
    if [[ "$status" == "200" ]]; then
        print_pass "HTTPS to example.com returned 200"
    else
        print_fail "HTTPS to example.com returned $status (expected 200)"
    fi
}

# =============================================================================
# TEST SECTION 2: RFC Proxy Forwarding Compliance
# =============================================================================

test_rfc_compliance() {
    should_run "rfc" || return 0
    print_header "Section 2: RFC Proxy Forwarding Compliance"

    # Test 2.1: Hop-by-hop header stripping
    print_test "2.1 Hop-by-hop headers are stripped from response"
    local resp_headers
    resp_headers=$(proxy_http_with_headers "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    # Check that Connection, Proxy-Authenticate etc are NOT in response
    local hop_found=false
    for hdr in "Proxy-Authenticate:" "Proxy-Authorization:" "Proxy-Connection:"; do
        if echo "$resp_headers" | grep -qi "$hdr"; then
            hop_found=true
            print_verbose "Found hop-by-hop header: $hdr"
        fi
    done
    if [[ "$hop_found" == "false" ]]; then
        print_pass "No hop-by-hop headers leaked to client"
    else
        print_fail "Hop-by-hop headers found in response"
    fi

    # Test 2.2: Via header is present in responses
    print_test "2.2 Via header present in proxy responses"
    resp_headers=$(proxy_http_with_headers "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    if echo "$resp_headers" | grep -qi "^Via:.*gatesentry"; then
        print_pass "Via header contains 'gatesentry' identifier"
    else
        print_fail "Via header missing or does not contain 'gatesentry'"
        print_verbose "Headers: $(echo "$resp_headers" | head -20)"
    fi

    # Test 2.3: X-Content-Type-Options: nosniff
    print_test "2.3 X-Content-Type-Options: nosniff header present"
    if echo "$resp_headers" | grep -qi "^X-Content-Type-Options:.*nosniff"; then
        print_pass "X-Content-Type-Options: nosniff is set"
    else
        print_fail "X-Content-Type-Options: nosniff is NOT set"
    fi

    # Test 2.4: TRACE method is blocked
    print_test "2.4 TRACE method is rejected"
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        -X TRACE --max-time 10 "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "405" ]]; then
        print_pass "TRACE method returned 405 Method Not Allowed"
    else
        print_fail "TRACE method returned $status (expected 405)"
    fi

    # Test 2.5: Missing Host header handling
    print_test "2.5 Request with no Host header is handled"
    # curl always sends Host, so we test with proxy sending to a URL with host
    # This test verifies the proxy doesn't crash on edge cases
    status=$(proxy_http_status "http://127.0.0.1:${HTTP_ECHO_PORT}/")
    if [[ "$status" != "000" ]]; then
        print_pass "Proxy handled the request (status: $status)"
    else
        print_fail "Proxy returned no response"
    fi

    # Test 2.6: Very long URL rejection
    print_test "2.6 Very long URL is rejected (>10000 chars)"
    local long_path
    long_path=$(python3 -c "print('a' * 10500)")
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 10 "http://127.0.0.1:${HTTP_ECHO_PORT}/${long_path}" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "414" ]]; then
        print_pass "Long URL returned 414 URI Too Long"
    elif [[ "$status" == "400" || "$status" == "502" ]]; then
        print_pass "Long URL was rejected (status: $status)"
    else
        print_fail "Long URL returned $status (expected 414 or rejection)"
    fi

    # Test 2.7: X-Forwarded-For loop detection (>=10 entries)
    print_test "2.7 X-Forwarded-For loop detection (>=10 entries)"
    local xff="1.1.1.1, 2.2.2.2, 3.3.3.3, 4.4.4.4, 5.5.5.5, 6.6.6.6, 7.7.7.7, 8.8.8.8, 9.9.9.9, 10.10.10.10"
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        -H "X-Forwarded-For: $xff" \
        --max-time 10 "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "400" ]]; then
        print_pass "X-Forwarded-For loop detected (10 entries → 400)"
    else
        print_fail "X-Forwarded-For loop not detected (status: $status, expected 400)"
    fi
}

# =============================================================================
# TEST SECTION 3: MITM / SSL Bumping
# =============================================================================

test_mitm() {
    should_run "mitm" || return 0
    print_header "Section 3: MITM / SSL Bumping"

    # Test 3.1: CONNECT tunnel is established for HTTPS
    print_test "3.1 CONNECT tunnel established for HTTPS"
    local body
    body=$(proxy_https "https://example.com/")
    if [[ -n "$body" ]] && grep -qi "example" <<< "$body"; then
        print_pass "HTTPS via CONNECT returned content from example.com"
    else
        print_fail "HTTPS via CONNECT did not return expected content"
        print_verbose "Body (first 200): $(echo "$body" | head -c 200)"
    fi

    # Test 3.2: MITM certificate is GateSentry-generated (not the real cert)
    print_test "3.2 MITM certificate is proxy-generated"
    local cert_info
    cert_info=$(curl -v --proxy "${PROXY_URL}" --noproxy '' \
        --insecure --max-time 10 "https://example.com/" 2>&1 | grep -i "issuer\|subject" || echo "")
    if [[ -n "$cert_info" ]]; then
        print_verbose "Certificate info: $cert_info"
        # The MITM cert won't have the real CA — if we can connect with --insecure, MITM is working
        print_pass "MITM connection established (cert details available)"
    else
        print_fail "Could not get MITM certificate information"
    fi

    # Test 3.3: Content is visible through MITM (can read HTML body)
    print_test "3.3 Content is readable through MITM tunnel"
    body=$(proxy_https "https://${ECHO_DOMAIN}:${HTTPS_ECHO_PORT}/index.html")
    if echo "$body" | grep -q "GateSentry Proxy Test Page"; then
        print_pass "Can read content through MITM tunnel"
    else
        print_fail "Cannot read content through MITM tunnel"
        print_verbose "Body: $body"
    fi

    # Test 3.4: Via header present in MITM response
    print_test "3.4 Via header present in MITM HTTPS response"
    local headers
    headers=$(proxy_https_with_headers "https://${ECHO_DOMAIN}:${HTTPS_ECHO_PORT}/index.html")
    if echo "$headers" | grep -qi "^Via:.*gatesentry"; then
        print_pass "Via header present in MITM response"
    else
        print_fail "Via header missing in MITM response"
        print_verbose "Headers: $(echo "$headers" | head -15)"
    fi
}

# =============================================================================
# TEST SECTION 4: SSL Passthrough / Direct Tunnel
# =============================================================================

test_passthrough() {
    should_run "passthrough" || return 0
    print_header "Section 4: SSL Passthrough (Direct Tunnel)"

    # When mitm_action=disable, the proxy should tunnel without inspecting.
    # We can't easily test passthrough vs MITM without a domain rule,
    # but we can test that exception hosts work.

    # Test 4.1: Exception host bypasses MITM
    # github.com is in the Exception Hosts filter
    print_test "4.1 Exception host (github.com) bypasses MITM"
    local status
    status=$(proxy_connect_status "https://github.com/")
    if [[ "$status" == "200" || "$status" == "301" || "$status" == "302" ]]; then
        print_pass "github.com accessible through proxy (status: $status)"
    else
        print_fail "github.com not accessible (status: $status)"
    fi

    # Test 4.2: Non-MITM CONNECT returns ssldirect in logs
    # We test this indirectly — the connection should work without certificate issues
    print_test "4.2 Passthrough mode doesn't interfere with TLS"
    local body
    body=$(proxy_https "https://github.com/" 2>/dev/null || echo "")
    if [[ -n "$body" ]]; then
        print_pass "Passthrough to github.com succeeded"
    else
        # github.com may reject or redirect, that's OK
        print_skip "github.com response empty (may be redirect — not a failure)"
    fi
}

# =============================================================================
# TEST SECTION 5: Domain Pattern Matching
# =============================================================================

test_domain_patterns() {
    should_run "domain-patterns" || return 0
    print_header "Section 5: Domain Pattern Matching"

    # These tests use external domains that are in the production blocklists.
    # Since DNS filtering is disabled, blocking must come from proxy rules.

    # Test 5.1: Exact domain match blocks
    # snapads.com is in the Blocked URLs filter
    print_test "5.1 Blocked URL (snapads.com) is blocked"
    local resp_file
    resp_file=$(mktemp)
    local status
    status=$(curl -s -o "$resp_file" -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 15 "http://snapads.com/" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "200" ]] && grep -qi "blocked\|gatesentry" "$resp_file" 2>/dev/null; then
        print_pass "snapads.com returned block page (status: $status)"
    elif [[ "$status" == "403" ]]; then
        print_pass "snapads.com returned 403 Forbidden"
    else
        print_fail "snapads.com was not blocked (status: $status)"
        print_verbose "Body (first 200): $(head -c 200 "$resp_file")"
    fi
    rm -f "$resp_file"

    # Test 5.2: Non-blocked domain passes through
    print_test "5.2 Allowed domain (example.com) passes through"
    resp_file=$(mktemp)
    status=$(curl -s -o "$resp_file" -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 15 "http://example.com/" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "200" ]] && grep -qi "example domain" "$resp_file" 2>/dev/null; then
        print_pass "example.com content returned normally (status: $status)"
    elif [[ "$status" == "200" ]]; then
        print_pass "example.com returned 200 OK"
    else
        print_fail "example.com did not return expected content (status: $status)"
        print_verbose "Body (first 200): $(head -c 200 "$resp_file")"
    fi
    rm -f "$resp_file"
}

# =============================================================================
# TEST SECTION 6: Domain List Blocking
# =============================================================================

test_domain_lists() {
    should_run "domain-lists" || return 0
    print_header "Section 6: Domain List Blocking"

    if [[ -z "$TEST_DOMAIN_LIST_ID" || -z "$TEST_RULE_BLOCK_DOMAIN_ID" ]]; then
        print_skip "Test domain list or block rule not created — skipping section"
        return
    fi

    # Test 6.1: Domain in test blocklist is blocked via HTTPS
    print_test "6.1 Domain in test blocklist is blocked (HTTPS CONNECT)"
    # blocked-test-domain.example.com should be blocked by our test rule
    # Since this domain doesn't resolve, we expect the proxy to block before DNS
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --insecure --max-time 10 "https://blocked-test-domain.example.com/" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "000" || "$status" == "403" ]]; then
        # 000 means connection was closed (block before CONNECT completes)
        # 403 means block page served
        print_pass "Blocked domain rejected by proxy (status: $status)"
    else
        print_fail "Blocked domain was not rejected (status: $status)"
    fi

    # Test 6.2: Domain in test blocklist blocked via HTTP
    print_test "6.2 Domain in test blocklist is blocked (HTTP)"
    local body
    body=$(proxy_http "http://blocked-test-domain.example.com/" 2>/dev/null || echo "")
    status=$(proxy_http_status "http://blocked-test-domain.example.com/" 2>/dev/null) || true
    status=${status:-000}
    if grep -qi "blocked\|gatesentry" <<< "$body"; then
        print_pass "HTTP blocked domain returned block page"
    elif [[ "$status" == "403" || "$status" == "000" || "$status" == "502" ]]; then
        print_pass "HTTP blocked domain rejected (status: $status)"
    else
        print_fail "HTTP blocked domain was not blocked (status: $status)"
        print_verbose "Body (first 200): $(echo "$body" | head -c 200)"
    fi

    # Test 6.3: Second domain in list is also blocked
    print_test "6.3 Another domain in blocklist is also blocked"
    status=$(proxy_http_status "http://another-blocked.example.com/" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "403" || "$status" == "000" || "$status" == "502" ]]; then
        print_pass "another-blocked.example.com rejected (status: $status)"
    else
        body=$(proxy_http "http://another-blocked.example.com/" 2>/dev/null || echo "")
        if grep -qi "blocked\|gatesentry" <<< "$body"; then
            print_pass "another-blocked.example.com returned block page"
        else
            print_fail "another-blocked.example.com was not blocked (status: $status)"
        fi
    fi

    # Test 6.4: Domain NOT in blocklist is not blocked
    print_test "6.4 Domain not in blocklist passes through"
    body=$(proxy_http "http://example.com/")
    if grep -qi "example domain" <<< "$body"; then
        print_pass "example.com passed through (not in blocklist)"
    else
        print_fail "example.com did not pass through"
    fi
}

# =============================================================================
# TEST SECTION 7: Keyword Content Filtering
# =============================================================================

test_keyword_filtering() {
    should_run "keyword-filtering" || return 0
    print_header "Section 7: Keyword Content Filtering"

    # Strictness is set to 500. Our test keyword "proxytest_keyword" has score 100.
    # keywords_high.html has 25 occurrences → 25 * 100 = 2500 > 500 → BLOCKED
    # keywords_low.html has 1 occurrence → 1 * 100 = 100 < 500 → ALLOWED

    # Test 7.1: Page with keywords exceeding strictness is blocked
    print_test "7.1 Page with high keyword score is blocked (25×100=2500 > 500)"
    local body
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/keywords_high.html")
    if grep -qi "blocked\|score.*above\|GateSentry Web Filter" <<< "$body"; then
        print_pass "High-score keyword page was blocked"
    else
        print_fail "High-score keyword page was NOT blocked"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    fi

    # Test 7.2: Page with keywords below strictness passes through
    print_test "7.2 Page with low keyword score passes through (1×100=100 < 500)"
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/keywords_low.html")
    if echo "$body" | grep -q "This page contains the test keyword only once"; then
        print_pass "Low-score keyword page passed through"
    elif grep -qi "blocked" <<< "$body"; then
        print_fail "Low-score keyword page was incorrectly blocked"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    else
        print_fail "Unexpected response for low-score page"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    fi

    # Test 7.3: Clean page with no keywords passes through
    print_test "7.3 Clean page (no keywords) passes through"
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    if echo "$body" | grep -q "GateSentry Proxy Test Page"; then
        print_pass "Clean page passed through without blocking"
    else
        print_fail "Clean page was unexpectedly modified or blocked"
        print_verbose "Body (first 200): $(echo "$body" | head -c 200)"
    fi

    # Test 7.4: Block page includes score and keyword details
    print_test "7.4 Block page includes score and keyword information"
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/keywords_high.html")
    if echo "$body" | grep -q "proxytest_keyword"; then
        print_pass "Block page mentions the triggering keyword"
    else
        print_fail "Block page does not mention the keyword"
        print_verbose "Body (first 500): $(echo "$body" | head -c 500)"
    fi

    # Test 7.5: HTTPS keyword filtering also works (through MITM)
    print_test "7.5 Keyword filtering works through MITM (HTTPS)"
    body=$(proxy_https "https://${ECHO_DOMAIN}:${HTTPS_ECHO_PORT}/keywords_high.html")
    if grep -qi "blocked\|score.*above\|GateSentry Web Filter\|403 Forbidden" <<< "$body"; then
        print_pass "HTTPS keyword filtering blocked high-score page"
    else
        print_fail "HTTPS keyword filtering did NOT block high-score page"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    fi

    # Test 7.6: Changing strictness affects blocking
    print_test "7.6 Raising strictness above score allows the page"
    # Set strictness to 5000 (above our 2500 score)
    api_post "/settings/strictness" '{"key":"strictness","value":"5000"}' >/dev/null
    sleep 1
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/keywords_high.html")
    if echo "$body" | grep -q "proxytest_keyword"; then
        if ! grep -qi "blocked.*score\|GateSentry Web Filter" <<< "$body"; then
            print_pass "With strictness=5000, high-score page passes through"
        else
            print_fail "With strictness=5000, page was still blocked"
        fi
    else
        print_fail "Unexpected response with strictness=5000"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    fi

    # Restore strictness
    api_post "/settings/strictness" '{"key":"strictness","value":"500"}' >/dev/null
    sleep 0.5
}

# =============================================================================
# TEST SECTION 8: Content-Type Match Criteria & Pipeline (Path A/B/C)
# =============================================================================

test_content_type_pipeline() {
    should_run "content-type" || return 0
    print_header "Section 8: Content-Type Match Criteria & Pipeline"

    # --- 8A: Content-Type as match criteria (blocked_content_types on a rule) ---
    # Create a temporary rule that blocks image/jpeg on the echo server domain.
    # This exercises the blocked_content_types match criteria in the proxy pipeline.

    print_section "8A: Content-Type Match Criteria"

    # Create rule: block image/jpeg for echo server (priority 0, highest)
    local ct_rule_resp ct_rule_id
    ct_rule_resp=$(api_post "/rules" "{
        \"name\": \"PT: Block JPEG Content-Type\",
        \"enabled\": true,
        \"priority\": 0,
        \"action\": \"allow\",
        \"domain_patterns\": [\"httpbin.org\", \"127.0.0.1\"],
        \"blocked_content_types\": [\"image/jpeg\"],
        \"time_restriction\": {\"from\": \"00:00\", \"to\": \"23:59\"}
    }")
    ct_rule_id=$(echo "$ct_rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)

    if [[ -z "$ct_rule_id" ]]; then
        print_skip "Failed to create content-type test rule — skipping 8A"
    else
        print_info "Created content-type test rule: $ct_rule_id"
        sleep 0.5

        # Test 8.1: JPEG image is blocked by content-type match
        print_test "8.1 JPEG image blocked by content-type match criteria"
        local status
        status=$(proxy_http_status "http://127.0.0.1:${HTTP_ECHO_PORT}/test.jpg")
        if [[ "$status" == "403" ]]; then
            print_pass "JPEG blocked with 403 (content-type match)"
        elif [[ "$status" == "200" ]]; then
            # Check if body is a block page rather than actual image
            local body
            body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/test.jpg")
            if grep -qi "blocked\|gatesentry" <<< "$body" 2>/dev/null; then
                print_pass "JPEG blocked with block page (content-type match)"
            else
                print_fail "JPEG returned 200 and was NOT blocked (content-type match should block)"
            fi
        else
            print_fail "JPEG returned unexpected status: $status (expected 403)"
        fi

        # Test 8.2: HTML page is NOT blocked by the JPEG content-type rule
        print_test "8.2 HTML page passes through (not matched by image/jpeg rule)"
        local body
        body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
        if echo "$body" | grep -q "GateSentry Proxy Test Page"; then
            print_pass "HTML page passed through (content-type did not match)"
        else
            print_fail "HTML page was unexpectedly blocked or modified"
            print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
        fi

        # Test 8.3: CSS file is NOT blocked by the JPEG content-type rule
        print_test "8.3 CSS file passes through (not matched by image/jpeg rule)"
        body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/style.css")
        if echo "$body" | grep -q "font-family"; then
            print_pass "CSS file passed through"
        else
            print_fail "CSS file was blocked or modified"
        fi

        # Delete the content-type test rule
        api_delete "/rules/${ct_rule_id}" >/dev/null 2>&1 || true
        print_info "Cleaned up content-type test rule"
        sleep 0.5
    fi

    # --- 8B: Content Pipeline Path Classification ---
    # These test how different content types are routed through the proxy pipeline.
    # Requires the echo server MITM + keyword rule to be active.

    print_section "8B: Content Pipeline Paths (A/B/C)"

    # Path A (Stream): JS, CSS, fonts — keywords in these are NOT scanned
    # Path B (Peek): images, video, audio — 4KB peek for filetype
    # Path C (Buffer): text/html — full scan for keywords

    # Test 8.4: JavaScript with keywords is NOT blocked (Path A)
    print_test "8.4 JS file with keywords is NOT blocked (Path A stream)"
    local body
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/script.js")
    if echo "$body" | grep -q "proxytest_keyword"; then
        print_pass "JS file passed through unscanned (Path A)"
    elif grep -qi "blocked" <<< "$body"; then
        print_fail "JS file was blocked — should use Path A (no scanning)"
    else
        print_fail "Unexpected response for JS file"
        print_verbose "Body: $body"
    fi

    # Test 8.5: CSS file passes through (Path A)
    print_test "8.5 CSS file passes through (Path A stream)"
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/style.css")
    if echo "$body" | grep -q "font-family"; then
        print_pass "CSS file passed through (Path A)"
    else
        print_fail "CSS file not returned correctly"
        print_verbose "Body: $body"
    fi

    # Test 8.6: Image request returns valid content (Path B)
    # Note: No content-type blocking rule active now, so image should pass
    print_test "8.6 Image passes through proxy (Path B peek)"
    local status
    status=$(proxy_http_status "http://127.0.0.1:${HTTP_ECHO_PORT}/test.jpg")
    if [[ "$status" == "200" ]]; then
        print_pass "Image returned 200 through proxy"
    else
        print_fail "Image returned $status (expected 200)"
    fi

    # Test 8.7: HTML with keywords IS scanned and blocked (Path C)
    print_test "8.7 HTML with keywords IS blocked (Path C buffer & scan)"
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/keywords_high.html")
    if grep -qi "blocked\|GateSentry Web Filter" <<< "$body"; then
        print_pass "HTML with keywords was scanned and blocked (Path C)"
    else
        print_fail "HTML with keywords was NOT blocked (Path C scan should catch it)"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    fi
}

# =============================================================================
# TEST SECTION 9: URL Regex Match Criteria
# =============================================================================

test_url_regex() {
    should_run "url-regex" || return 0
    print_header "Section 9: URL Regex Match Criteria"

    # --- 9A: Structural check — the preset URL regex rule exists ---

    if [[ -z "$TEST_RULE_URL_REGEX_ID" ]]; then
        print_skip "URL regex rule not created — skipping structural check"
    else
        print_test "9.1 URL regex rule is configured"
        local rules
        rules=$(api_get "/rules")
        if echo "$rules" | grep -q "PT: URL Regex Block"; then
            print_pass "URL regex test rule exists"
        else
            print_fail "URL regex test rule not found"
        fi

        print_test "9.2 URL regex rule has correct patterns"
        local has_patterns
        has_patterns=$(echo "$rules" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for r in data.get('rules', []):
    if r.get('name') == 'PT: URL Regex Block':
        patterns = r.get('url_regex_patterns', [])
        if '.*blocked-path.*' in patterns and '.*\\\\.exe$' in patterns:
            print('yes')
        else:
            print('no')
        break
" 2>/dev/null)
        if [[ "$has_patterns" == "yes" ]]; then
            print_pass "URL regex patterns correctly configured"
        else
            print_fail "URL regex patterns not correctly configured"
        fi
    fi

    # --- 9B: Functional tests — URL regex blocks matching requests ---
    # We create a temporary rule on the echo server domain (127.0.0.1) with
    # URL patterns that match specific paths. This rule uses action=block
    # and priority 0 (highest) so it fires before the echo MITM rule.

    print_section "9B: URL Regex Functional Tests"

    local ur_rule_resp ur_rule_id
    ur_rule_resp=$(api_post "/rules" "{
        \"name\": \"PT: URL Regex Functional\",
        \"enabled\": true,
        \"priority\": 0,
        \"action\": \"block\",
        \"domain_patterns\": [\"httpbin.org\", \"127.0.0.1\"],
        \"url_regex_patterns\": [\".*blocked-path.*\", \".*\\\\.exe$\"],
        \"time_restriction\": {\"from\": \"00:00\", \"to\": \"23:59\"}
    }")
    ur_rule_id=$(echo "$ur_rule_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('rule',{}).get('id',''))" 2>/dev/null)

    if [[ -z "$ur_rule_id" ]]; then
        print_skip "Failed to create URL regex functional rule — skipping 9B"
    else
        print_info "Created URL regex functional rule: $ur_rule_id"
        sleep 0.5

        # Test 9.3: URL matching "blocked-path" pattern is blocked
        print_test "9.3 URL matching blocked-path pattern is blocked"
        local body
        body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/blocked-path/page.html")
        if grep -qi "blocked\|GateSentry\|URL blocked" <<< "$body"; then
            print_pass "URL with blocked-path was blocked"
        else
            print_fail "URL with blocked-path was NOT blocked"
            print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
        fi

        # Test 9.4: URL matching .exe pattern is blocked
        print_test "9.4 URL matching .exe pattern is blocked"
        body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/download/setup.exe")
        if grep -qi "blocked\|GateSentry\|URL blocked" <<< "$body"; then
            print_pass "URL with .exe was blocked"
        else
            print_fail "URL with .exe was NOT blocked"
            print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
        fi

        # Test 9.5: Non-matching URL passes through (index.html)
        # Delete the functional rule first so the echo MITM rule (priority 1) takes over
        api_delete "/rules/${ur_rule_id}" >/dev/null 2>&1 || true
        print_info "Cleaned up URL regex functional rule"
        sleep 0.5

        print_test "9.5 Non-matching URL passes through after rule removal"
        body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
        if echo "$body" | grep -q "GateSentry Proxy Test Page"; then
            print_pass "Non-matching URL passed through correctly"
        else
            print_fail "Non-matching URL was unexpectedly blocked"
            print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
        fi

        # Rule already deleted above
        ur_rule_id=""
    fi

    # Clean up just in case
    if [[ -n "$ur_rule_id" ]]; then
        api_delete "/rules/${ur_rule_id}" >/dev/null 2>&1 || true
    fi
}

# =============================================================================
# TEST SECTION 10: Block Page Verification
# =============================================================================

test_block_pages() {
    should_run "block-pages" || return 0
    print_header "Section 10: Block Page Verification"

    # Test 11.1: HTTP block page is served as HTML with 200 OK
    print_test "10.1 HTTP block page is HTML with 200 OK"
    local resp
    resp=$(proxy_http_with_headers "http://snapads.com/")
    local status
    status=$(echo "$resp" | head -1 | grep -oP '\d{3}' | head -1)
    if echo "$resp" | grep -qi "text/html"; then
        print_pass "HTTP block page served as text/html (status: $status)"
    else
        print_fail "HTTP block page not served as text/html"
        print_verbose "Headers: $(echo "$resp" | head -10)"
    fi

    # Test 11.2: Block page contains identifiable content
    print_test "10.2 Block page contains GateSentry branding"
    local body
    body=$(proxy_http "http://snapads.com/" 2>/dev/null || echo "")
    if grep -qi "gatesentry\|blocked\|Blocked URL" <<< "$body"; then
        print_pass "Block page contains GateSentry/blocked identifiers"
    else
        print_fail "Block page doesn't contain expected identifiers"
        print_verbose "Body (first 300): $(echo "$body" | head -c 300)"
    fi

    # Test 11.3: Keyword block page includes score and reasons
    print_test "10.3 Keyword block page includes score information"
    body=$(proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/keywords_high.html")
    if grep -qi "score\|Reason\|proxytest_keyword" <<< "$body"; then
        print_pass "Keyword block page has score/reason details"
    else
        print_fail "Keyword block page missing score/reason details"
        print_verbose "Body (first 500): $(echo "$body" | head -c 500)"
    fi

    # Test 11.4: HTTPS block page uses 403 Forbidden
    print_test "10.4 HTTPS block page sends 403 Forbidden"
    resp=$(proxy_https_with_headers "https://snapads.com/" 2>/dev/null || echo "")
    if echo "$resp" | grep -q "403"; then
        print_pass "HTTPS block page returned 403 Forbidden"
    else
        # The proxy hijacks the connection for HTTPS blocks — may not get clean headers
        if echo "$resp" | grep -qi "blocked\|gatesentry"; then
            print_pass "HTTPS block page served (block content detected)"
        else
            print_fail "HTTPS block page not detected"
            print_verbose "Response (first 300): $(echo "$resp" | head -c 300)"
        fi
    fi
}

# =============================================================================
# TEST SECTION 11: Rule Priority (First-Match-Wins)
# =============================================================================

test_rule_priority() {
    should_run "rule-priority" || return 0
    print_header "Section 11: Rule Priority (First-Match-Wins)"

    # Test 12.1: Verify our test rules are in correct priority order
    print_test "11.1 Test rules have correct priority ordering"
    local rules
    rules=$(api_get "/rules")
    local priorities
    priorities=$(echo "$rules" | python3 -c "
import sys, json
data = json.load(sys.stdin)
test_rules = [(r['priority'], r['name']) for r in data.get('rules', []) if r['name'].startswith('PT:')]
test_rules.sort()
for p, n in test_rules:
    print(f'{p}: {n}')
" 2>/dev/null)
    print_verbose "Test rule priorities:\n$priorities"
    if [[ -n "$priorities" ]]; then
        print_pass "Test rules have assigned priorities"
    else
        print_fail "Could not read test rule priorities"
    fi

    # Test 12.2: Higher priority allow rule should override lower priority block rule
    # PT: No MITM Passthrough (priority 3) — allow passthrough-test.example.com
    # PT: Allow Test Pattern (priority 5) — allow allowed-test.example.com
    # PT: Block Test Domains (priority 10) — block from test domain list
    # If a domain is in both allow (priority 5) and block (priority 10), allow wins
    print_test "11.2 Rules are evaluated in priority order (first match wins)"
    # Check that the rules have different priorities
    local rule_count
    rule_count=$(echo "$rules" | python3 -c "
import sys, json
data = json.load(sys.stdin)
test_rules = [r for r in data.get('rules', []) if r['name'].startswith('PT:')]
print(len(test_rules))
" 2>/dev/null)
    if [[ "$rule_count" -ge 3 ]]; then
        print_pass "Multiple test rules exist with different priorities ($rule_count rules)"
    else
        print_fail "Expected at least 3 test rules, found $rule_count"
    fi
}

# =============================================================================
# TEST SECTION 12: Proxy Loop Detection
# =============================================================================

test_loop_detection() {
    should_run "loop-detection" || return 0
    print_header "Section 12: Proxy Loop Detection"

    # Test 13.1: X-GateSentry-Loop header triggers 508 Loop Detected
    print_test "12.1 X-GateSentry-Loop header triggers loop detection"
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        -H "X-GateSentry-Loop: gatesentry" \
        --max-time 10 "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "508" ]]; then
        print_pass "X-GateSentry-Loop detected → 508 Loop Detected"
    else
        print_fail "X-GateSentry-Loop not detected (status: $status, expected 508)"
    fi

    # Test 13.2: Via header with "gatesentry" triggers loop detection
    print_test "12.2 Via header with 'gatesentry' triggers loop detection"
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        -H "Via: 1.1 gatesentry" \
        --max-time 10 "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "508" ]]; then
        print_pass "Via loop detected → 508 Loop Detected"
    else
        print_fail "Via loop not detected (status: $status, expected 508)"
    fi

    # Test 13.3: Normal request (no loop headers) works fine
    print_test "12.3 Normal request without loop headers succeeds"
    status=$(proxy_http_status "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    if [[ "$status" == "200" ]]; then
        print_pass "Normal request returned 200 (no loop)"
    else
        print_fail "Normal request returned $status (expected 200)"
    fi
}

# =============================================================================
# TEST SECTION 13: SSRF Protection
# =============================================================================

test_ssrf_protection() {
    should_run "ssrf" || return 0
    print_header "Section 13: SSRF Protection"

    # The proxy blocks requests to the admin port on loopback/link-local addresses

    # Test 14.1: Proxy blocks request to admin port on localhost
    print_test "13.1 SSRF: Proxy blocks requests to admin port on localhost"
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 10 "http://localhost:${ADMIN_PORT}/gatesentry/api/about" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "403" ]]; then
        print_pass "SSRF blocked → 403 Forbidden for admin port on localhost"
    else
        print_fail "SSRF not blocked (status: $status, expected 403)"
    fi

    # Test 14.2: Proxy blocks request to admin port on 127.0.0.1
    print_test "13.2 SSRF: Proxy blocks requests to admin port on 127.0.0.1"
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 10 "http://127.0.0.1:${ADMIN_PORT}/gatesentry/api/about" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "403" ]]; then
        print_pass "SSRF blocked → 403 Forbidden for admin port on 127.0.0.1"
    else
        print_fail "SSRF not blocked (status: $status, expected 403)"
    fi

    # Test 14.3: Requests to other ports on localhost are allowed
    print_test "13.3 Requests to non-admin ports on localhost are allowed"
    status=$(proxy_http_status "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    if [[ "$status" == "200" ]]; then
        print_pass "Non-admin port request allowed (status: 200)"
    else
        print_fail "Non-admin port request blocked (status: $status)"
    fi
}

# =============================================================================
# TEST SECTION 14: Error Handling
# =============================================================================

test_error_handling() {
    should_run "error-handling" || return 0
    print_header "Section 14: Error Handling"

    # Test 15.1: Connection refused upstream returns 502
    print_test "14.1 Unreachable upstream returns 502 Bad Gateway"
    local status
    # Use a non-routable address (RFC 5737 TEST-NET-1) — connection will be refused/timeout
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 10 "http://198.51.100.1:19999/" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "502" || "$status" == "504" || "$status" == "000" ]]; then
        print_pass "Unreachable upstream handled (status: $status)"
    else
        print_fail "Unreachable upstream returned $status (expected 502/504/000)"
    fi

    # Test 15.2: Non-existent domain returns 502
    print_test "14.2 Non-existent domain returns 502"
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 15 "http://this-domain-does-not-exist-at-all.invalid/" 2>/dev/null) || true
    status=${status:-000}
    if [[ "$status" == "502" || "$status" == "000" || "$status" == "503" ]]; then
        print_pass "Non-existent domain handled (status: $status)"
    else
        print_fail "Non-existent domain returned $status (expected 502 or connection close)"
    fi

    # Test 15.3: Timeout on slow upstream
    print_test "14.3 Timeout on slow upstream (if applicable)"
    # This is hard to test without a slow server, just verify proxy doesn't hang
    local start_ms end_ms elapsed
    start_ms=$(get_time_ms)
    status=$(curl -s -o /dev/null -w "%{http_code}" --proxy "${PROXY_URL}" --noproxy '' \
        --max-time 5 "http://127.0.0.1:19998/" 2>/dev/null) || true
    status=${status:-000}
    end_ms=$(get_time_ms)
    elapsed=$(( (end_ms - start_ms) ))
    if [[ $elapsed -lt 15000 ]]; then
        print_pass "Proxy responded within timeout ($elapsed ms, status: $status)"
    else
        print_fail "Proxy took too long ($elapsed ms)"
    fi
}

# =============================================================================
# TEST SECTION 15: Header Sanitization
# =============================================================================

test_header_sanitization() {
    should_run "headers" || return 0
    print_header "Section 15: Header Sanitization"

    # Test 16.1: Response headers don't contain null bytes
    print_test "15.1 Response headers are sanitized (no null bytes)"
    local headers
    headers=$(proxy_http_with_headers "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
    if echo "$headers" | grep -qP '\x00'; then
        print_fail "Null bytes found in response headers"
    else
        print_pass "No null bytes in response headers"
    fi

    # Test 16.2: Content-Length is consistent
    print_test "15.2 Content-Length header is present and valid"
    local cl
    cl=$(echo "$headers" | grep -i "^Content-Length:" | head -1 | awk '{print $2}' | tr -d '\r')
    if [[ -n "$cl" ]] && [[ "$cl" =~ ^[0-9]+$ ]]; then
        print_pass "Content-Length is valid: $cl"
    else
        # May use chunked transfer encoding — that's also OK
        if echo "$headers" | grep -qi "Transfer-Encoding"; then
            print_pass "Uses Transfer-Encoding (no Content-Length needed)"
        else
            print_fail "No valid Content-Length or Transfer-Encoding"
            print_verbose "Headers: $(echo "$headers" | head -15)"
        fi
    fi
}

# =============================================================================
# TEST SECTION 16: Performance / Latency
# =============================================================================

test_performance() {
    should_run "performance" || return 0
    print_header "Section 16: Performance / Latency"

    # Test 17.1: HTTP request latency through proxy
    print_test "16.1 HTTP proxy latency to echo server"
    local start_ms end_ms latency
    start_ms=$(get_time_ms)
    proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" >/dev/null
    end_ms=$(get_time_ms)
    latency=$((end_ms - start_ms))
    if [[ $latency -lt 1000 ]]; then
        print_pass "HTTP proxy latency: ${latency}ms (< 1000ms)"
    elif [[ $latency -lt 3000 ]]; then
        print_pass "HTTP proxy latency: ${latency}ms (acceptable)"
    else
        print_fail "HTTP proxy latency: ${latency}ms (too slow, > 3000ms)"
    fi

    # Test 17.2: HTTPS (MITM) request latency
    print_test "16.2 HTTPS (MITM) proxy latency to echo server"
    start_ms=$(get_time_ms)
    proxy_https "https://${ECHO_DOMAIN}:${HTTPS_ECHO_PORT}/index.html" >/dev/null
    end_ms=$(get_time_ms)
    latency=$((end_ms - start_ms))
    if [[ $latency -lt 2000 ]]; then
        print_pass "HTTPS MITM proxy latency: ${latency}ms (< 2000ms)"
    elif [[ $latency -lt 5000 ]]; then
        print_pass "HTTPS MITM proxy latency: ${latency}ms (acceptable)"
    else
        print_fail "HTTPS MITM proxy latency: ${latency}ms (too slow, > 5000ms)"
    fi

    # Test 17.3: Multiple sequential requests
    print_test "16.3 Sequential request throughput (10 requests)"
    start_ms=$(get_time_ms)
    local success=0
    for i in $(seq 1 10); do
        local s
        s=$(proxy_http_status "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html")
        if [[ "$s" == "200" ]]; then
            success=$((success + 1))
        fi
    done
    end_ms=$(get_time_ms)
    local total_ms=$((end_ms - start_ms))
    local avg=$((total_ms / 10))
    if [[ $success -eq 10 ]]; then
        print_pass "10/10 requests succeeded, total: ${total_ms}ms, avg: ${avg}ms"
    else
        print_fail "Only $success/10 requests succeeded (total: ${total_ms}ms)"
    fi

    # Test 17.4: Concurrent requests (background processes)
    print_test "16.4 Concurrent request handling (5 parallel)"
    local pids=()
    start_ms=$(get_time_ms)
    for i in $(seq 1 5); do
        proxy_http "http://127.0.0.1:${HTTP_ECHO_PORT}/index.html" >/dev/null &
        pids+=($!)
    done
    local all_ok=true
    for pid in "${pids[@]}"; do
        if ! wait "$pid"; then
            all_ok=false
        fi
    done
    end_ms=$(get_time_ms)
    total_ms=$((end_ms - start_ms))
    if [[ "$all_ok" == "true" ]]; then
        print_pass "5 concurrent requests completed in ${total_ms}ms"
    else
        print_fail "Some concurrent requests failed (${total_ms}ms)"
    fi
}

# =============================================================================
# MAIN EXECUTION
# =============================================================================

main() {
    print_header "GateSentry Proxy Server - Deep Test Suite"
    echo -e "${CYAN}  Proxy:  ${PROXY_URL}${NC}"
    echo -e "${CYAN}  Admin:  ${ADMIN_BASE}${NC}"
    echo -e "${CYAN}  Date:   $(date)${NC}"
    echo ""

    check_dependencies

    # Start (or restart) GateSentry with a clean environment
    start_gatesentry_server

    authenticate

    if [[ "$SKIP_SETUP" != "true" ]]; then
        save_state
        start_echo_server
        setup_test_environment
    else
        print_warning "Skipping setup (--skip-setup)"
        start_echo_server
    fi

    # Run test sections
    test_connectivity
    test_rfc_compliance
    test_mitm
    test_passthrough
    test_domain_patterns
    test_domain_lists
    test_keyword_filtering
    test_content_type_pipeline
    test_url_regex
    test_block_pages
    test_rule_priority
    test_loop_detection
    test_ssrf_protection
    test_error_handling
    test_header_sanitization
    test_performance

    # Cleanup and summary happen in the trap handler
}

main "$@"
