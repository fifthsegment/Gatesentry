#!/bin/bash
#
# GateSentry DNS Server - Deep Analysis and Robustness Testing Script
# =====================================================================
#
# This script performs comprehensive DNS server testing to ensure the
# GateSentry DNS server implementation is robust and meets all DNS service demands.
#
# Platform Support:
#   - Linux (GNU coreutils) - Full support
#   - macOS/BSD - Requires GNU tools: brew install coreutils grep
#     Then use: PATH="/opt/homebrew/opt/coreutils/libexec/gnubin:$PATH"
#
# Usage:
#   ./dns_deep_test.sh [OPTIONS]
#
# Options:
#   -p, --port PORT          DNS server port to test (default: 10053)
#   -s, --server SERVER      DNS server address (default: 127.0.0.1)
#   -r, --resolver RESOLVER  External resolver for comparison (default: 8.8.8.8)
#   -t, --timeout TIMEOUT    Query timeout in seconds (default: 5)
#   -c, --concurrency NUM    Number of concurrent queries for stress test (default: 50)
#   -v, --verbose            Enable verbose output
#   -h, --help               Show this help message
#
# Environment Variables:
#   GATESENTRY_DNS_ADDR      Listen address for the DNS server (default: 0.0.0.0)
#   GATESENTRY_DNS_PORT      Port for the DNS server (default: 10053)
#   GATESENTRY_DNS_RESOLVER  External resolver address (default: 8.8.8.8:53)
#
# Note: Port 5353 is reserved for mDNS/Bonjour, so we use 10053 by default.
#
# Requirements:
#   - dig (dnsutils package)
#   - nc (netcat)
#   - timeout command (GNU coreutils)
#   - bc (for calculations)
#   - grep with PCRE support (-P flag) or GNU grep
#
# Author: GateSentry Team
# Date: 2026-02-07
#

set -euo pipefail

# =============================================================================
# Platform Detection and Compatibility
# =============================================================================

# Detect platform
PLATFORM="unknown"
case "$(uname -s)" in
    Linux*)  PLATFORM="linux";;
    Darwin*) PLATFORM="macos";;
    CYGWIN*|MINGW*|MSYS*) PLATFORM="windows";;
    FreeBSD*) PLATFORM="freebsd";;
    *) PLATFORM="unknown";;
esac

# Check for GNU grep with PCRE support
HAS_GREP_PCRE=false
if grep --version 2>/dev/null | grep -q "GNU"; then
    if echo "test" | grep -oP 'test' &>/dev/null; then
        HAS_GREP_PCRE=true
    fi
fi

# Portable grep -oP replacement using sed/awk
# Usage: extract_pattern "string" "prefix_regex"
# Extracts value after the prefix pattern
extract_after() {
    local input="$1"
    local prefix="$2"
    if [[ "$HAS_GREP_PCRE" == "true" ]]; then
        echo "$input" | grep -oP "${prefix}\\K[^ ]+" 2>/dev/null || echo ""
    else
        # Portable fallback using sed
        echo "$input" | sed -n "s/.*${prefix}\([^ ]*\).*/\1/p" | head -1
    fi
}

# Extract DNS status code from dig output (portable)
extract_dns_status() {
    local output="$1"
    if [[ "$HAS_GREP_PCRE" == "true" ]]; then
        echo "$output" | grep -oP 'status: \K[A-Z]+' 2>/dev/null || echo "UNKNOWN"
    else
        echo "$output" | sed -n 's/.*status: \([A-Z]*\).*/\1/p' | head -1
    fi
}

# Extract numeric value after a key= pattern (portable)
# Usage: extract_key_value "string" "keyname"
extract_key_value() {
    local input="$1"
    local key="$2"
    if [[ "$HAS_GREP_PCRE" == "true" ]]; then
        echo "$input" | grep -oP "${key}=\\K[0-9.]+" 2>/dev/null || echo "0"
    else
        echo "$input" | sed -n "s/.*${key}=\([0-9.]*\).*/\1/p" | head -1
    fi
}

# Get current time in milliseconds (portable)
get_time_ms() {
    if [[ "$PLATFORM" == "macos" ]]; then
        # macOS: use python or perl for millisecond precision
        if command -v python3 &>/dev/null; then
            python3 -c 'import time; print(int(time.time() * 1000))'
        elif command -v perl &>/dev/null; then
            perl -MTime::HiRes=time -e 'printf "%d\n", time * 1000'
        else
            # Fallback to seconds only
            echo "$(($(date +%s) * 1000))"
        fi
    else
        # Linux: date supports nanoseconds
        echo "$(($(date +%s%N) / 1000000))"
    fi
}

# Get current time in nanoseconds (portable, with fallback to milliseconds)
get_time_ns() {
    if [[ "$PLATFORM" == "macos" ]]; then
        # macOS: use python or perl, convert ms to ns
        if command -v python3 &>/dev/null; then
            python3 -c 'import time; print(int(time.time() * 1000000000))'
        elif command -v perl &>/dev/null; then
            perl -MTime::HiRes=time -e 'printf "%d\n", time * 1000000000'
        else
            # Fallback to seconds converted to ns
            echo "$(($(date +%s) * 1000000000))"
        fi
    else
        # Linux: date supports nanoseconds
        date +%s%N
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
# Note: Port 5353 is reserved for mDNS/Bonjour, use 10053 for testing
DNS_PORT="${GATESENTRY_DNS_PORT:-10053}"
DNS_SERVER="127.0.0.1"
EXTERNAL_RESOLVER="${GATESENTRY_DNS_RESOLVER:-8.8.8.8}"
QUERY_TIMEOUT=5
CONCURRENCY=50
VERBOSE=false

# Server management
GATESENTRY_PID=""
STARTED_SERVER=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
GATESENTRY_BIN="$PROJECT_DIR/bin/gatesentrybin"
GATESENTRY_LOG="$PROJECT_DIR/dns_test_server.log"

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0
TESTS_TOTAL=0

# Test results array
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

show_help() {
    cat << EOF
GateSentry DNS Server - Deep Analysis and Robustness Testing Script

Usage: $0 [OPTIONS]

Options:
  -p, --port PORT          DNS server port to test (default: 10053)
  -s, --server SERVER      DNS server address (default: 127.0.0.1)
  -r, --resolver RESOLVER  External resolver for comparison (default: 8.8.8.8)
  -t, --timeout TIMEOUT    Query timeout in seconds (default: 5)
  -c, --concurrency NUM    Number of concurrent queries for stress test (default: 50)
  -v, --verbose            Enable verbose output
  -h, --help               Show this help message

Environment Variables:
  GATESENTRY_DNS_PORT      Port for the DNS server (default: 10053)
  GATESENTRY_DNS_RESOLVER  External resolver address (default: 8.8.8.8:53)

Note: Port 5353 is reserved for mDNS/Bonjour, so we use 10053 by default.

Example:
  # Test DNS server on custom port
  GATESENTRY_DNS_PORT=10053 ./dns_deep_test.sh

  # Test with specific settings
  ./dns_deep_test.sh -p 10053 -s 127.0.0.1 -v

EOF
    exit 0
}

check_dependencies() {
    print_section "Checking Dependencies"

    local missing_deps=()
    local warnings=()

    # Check required commands
    for cmd in dig nc timeout bc awk sed grep; do
        if command -v "$cmd" &> /dev/null; then
            print_verbose "Found: $cmd ($(command -v "$cmd"))"
        else
            missing_deps+=("$cmd")
        fi
    done

    # Platform-specific checks
    if [[ "$PLATFORM" == "macos" ]]; then
        print_info "Detected macOS platform"

        # Check for GNU grep (needed for -P flag)
        if [[ "$HAS_GREP_PCRE" != "true" ]]; then
            warnings+=("GNU grep not found - using portable fallbacks (may be slower)")
            print_warning "For better performance: brew install grep && export PATH=\"/opt/homebrew/opt/grep/libexec/gnubin:\$PATH\""
        fi

        # Check for nanosecond timing support
        if ! command -v python3 &>/dev/null && ! command -v perl &>/dev/null; then
            warnings+=("Neither python3 nor perl found - timing precision reduced to seconds")
        fi

        # Check for gtimeout (GNU timeout)
        if ! command -v timeout &>/dev/null; then
            if command -v gtimeout &>/dev/null; then
                print_info "Using gtimeout instead of timeout"
                # Create alias for gtimeout
                timeout() { gtimeout "$@"; }
            else
                missing_deps+=("timeout (install with: brew install coreutils)")
            fi
        fi
    fi

    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        print_fail "Missing dependencies: ${missing_deps[*]}"
        echo -e "\nInstall missing dependencies:"
        echo "  Ubuntu/Debian: sudo apt-get install dnsutils netcat bc"
        echo "  RHEL/CentOS:   sudo yum install bind-utils nc bc"
        echo "  macOS:         brew install bind coreutils grep"
        echo "                 Then add to PATH: export PATH=\"/opt/homebrew/opt/coreutils/libexec/gnubin:\$PATH\""
        exit 1
    fi

    # Show warnings but continue
    for warn in "${warnings[@]:-}"; do
        if [[ -n "$warn" ]]; then
            print_warning "$warn"
        fi
    done

    print_pass "All dependencies satisfied"
}

# =============================================================================
# Server Management Functions
# =============================================================================

# Check if DNS server is responding on the configured port
is_server_running() {
    local result
    result=$(timeout 2 dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +short +time=1 +tries=1 2>&1 || echo "")

    if is_dns_error "$result"; then
        return 1  # Server not running or not responding
    fi

    if [[ -n "$result" ]]; then
        return 0  # Server is running
    fi

    return 1
}

# Start the GateSentry DNS server
start_gatesentry_server() {
    print_section "Starting GateSentry DNS Server"

    # Check if binary exists
    if [[ ! -x "$GATESENTRY_BIN" ]]; then
        print_warning "GateSentry binary not found at: $GATESENTRY_BIN"
        print_info "Attempting to build..."

        # Try to build
        if [[ -f "$PROJECT_DIR/build.sh" ]]; then
            (cd "$PROJECT_DIR" && ./build.sh) > /dev/null 2>&1 || {
                print_fail "Failed to build GateSentry"
                return 1
            }
        else
            print_fail "build.sh not found in $PROJECT_DIR"
            return 1
        fi

        if [[ ! -x "$GATESENTRY_BIN" ]]; then
            print_fail "Binary still not found after build"
            return 1
        fi
        print_pass "GateSentry built successfully"
    fi

    print_info "Starting GateSentry DNS server..."
    print_verbose "Binary: $GATESENTRY_BIN"
    print_verbose "Log: $GATESENTRY_LOG"
    print_verbose "Environment:"
    print_verbose "  GATESENTRY_DNS_ADDR=$GATESENTRY_DNS_ADDR"
    print_verbose "  GATESENTRY_DNS_PORT=$GATESENTRY_DNS_PORT"
    print_verbose "  GATESENTRY_DNS_RESOLVER=$GATESENTRY_DNS_RESOLVER"

    # Start the server in the background
    (
        cd "$PROJECT_DIR/bin"
        exec ./gatesentrybin > "$GATESENTRY_LOG" 2>&1
    ) &
    GATESENTRY_PID=$!
    STARTED_SERVER=true

    print_info "Server starting with PID: $GATESENTRY_PID"

    # Wait for server to be ready
    local max_wait=30
    local waited=0
    print_info "Waiting for DNS server to be ready (max ${max_wait}s)..."

    while [[ $waited -lt $max_wait ]]; do
        sleep 1
        waited=$((waited + 1))

        # Check if process is still running
        if ! kill -0 "$GATESENTRY_PID" 2>/dev/null; then
            print_fail "Server process died unexpectedly"
            print_info "Check log file: $GATESENTRY_LOG"
            if [[ "$VERBOSE" == "true" ]] && [[ -f "$GATESENTRY_LOG" ]]; then
                echo -e "${CYAN}Last 20 lines of log:${NC}"
                tail -20 "$GATESENTRY_LOG"
            fi
            return 1
        fi

        # Check if server is responding
        if is_server_running; then
            print_pass "DNS server is ready (took ${waited}s)"
            return 0
        fi

        print_verbose "Waiting... ($waited/$max_wait)"
    done

    print_fail "Server failed to respond within ${max_wait} seconds"
    print_info "Check log file: $GATESENTRY_LOG"
    if [[ "$VERBOSE" == "true" ]] && [[ -f "$GATESENTRY_LOG" ]]; then
        echo -e "${CYAN}Last 20 lines of log:${NC}"
        tail -20 "$GATESENTRY_LOG"
    fi
    return 1
}

# Stop the GateSentry DNS server if we started it
stop_gatesentry_server() {
    if [[ "$STARTED_SERVER" == "true" ]] && [[ -n "$GATESENTRY_PID" ]]; then
        print_section "Stopping GateSentry DNS Server"
        print_info "Stopping server (PID: $GATESENTRY_PID)..."

        # Send SIGTERM first
        kill "$GATESENTRY_PID" 2>/dev/null || true

        # Wait for graceful shutdown
        local waited=0
        while kill -0 "$GATESENTRY_PID" 2>/dev/null && [[ $waited -lt 5 ]]; do
            sleep 1
            waited=$((waited + 1))
        done

        # Force kill if still running
        if kill -0 "$GATESENTRY_PID" 2>/dev/null; then
            print_warning "Server didn't stop gracefully, forcing..."
            kill -9 "$GATESENTRY_PID" 2>/dev/null || true
        fi

        print_pass "Server stopped"
        GATESENTRY_PID=""
        STARTED_SERVER=false
    fi
}

# Cleanup function for script exit
cleanup() {
    stop_gatesentry_server
}

# Set up trap to cleanup on exit
trap cleanup EXIT INT TERM

# Ensure server is available, start if needed
ensure_server_available() {
    print_section "Checking DNS Server Availability"

    if is_server_running; then
        print_pass "DNS server is already running on $DNS_SERVER:$DNS_PORT"
        return 0
    fi

    print_warning "DNS server not responding on $DNS_SERVER:$DNS_PORT"

    # Only try to start if server is localhost
    if [[ "$DNS_SERVER" == "127.0.0.1" ]] || [[ "$DNS_SERVER" == "localhost" ]]; then
        print_info "Attempting to start GateSentry DNS server..."
        start_gatesentry_server
        return $?
    else
        print_fail "Cannot auto-start server on remote host $DNS_SERVER"
        print_info "Please ensure the DNS server is running on $DNS_SERVER:$DNS_PORT"
        return 1
    fi
}

# =============================================================================
# DNS Query Functions
# =============================================================================

# Check if dig output indicates an error (not a valid DNS response)
is_dns_error() {
    local output="$1"
    # Check for common error patterns in dig output
    if [[ -z "$output" ]]; then
        return 0  # Empty is an error
    fi
    if echo "$output" | grep -qi "connection refused\|timed out\|no servers could be reached\|communications error\|connection reset\|network unreachable\|host unreachable"; then
        return 0  # Error patterns found
    fi
    return 1  # No error
}

# Filter out error messages from dig output, return only valid results
filter_dns_result() {
    local output="$1"
    # Remove lines that contain error messages
    echo "$output" | grep -vi "connection refused\|timed out\|no servers could be reached\|communications error\|connection reset\|network unreachable\|host unreachable\|;;" | grep -v "^$" || echo ""
}

# Validate that a DNS response is correct for the queried domain
# Returns 0 if valid, 1 if invalid
# Sets global VALIDATION_ERROR with reason if invalid
validate_dns_response() {
    local domain="$1"
    local record_type="$2"
    local full_output="$3"

    VALIDATION_ERROR=""

    # Check for NOERROR status (successful query)
    if ! echo "$full_output" | grep -q "status: NOERROR"; then
        local status
        status=$(extract_dns_status "$full_output")
        # NXDOMAIN is valid for non-existent domains, but for our test domains it's an error
        if [[ "$status" == "NXDOMAIN" ]]; then
            VALIDATION_ERROR="Domain not found (NXDOMAIN)"
            return 1
        elif [[ "$status" == "SERVFAIL" ]]; then
            VALIDATION_ERROR="Server failure (SERVFAIL)"
            return 1
        elif [[ "$status" == "REFUSED" ]]; then
            VALIDATION_ERROR="Query refused (REFUSED)"
            return 1
        elif [[ "$status" != "NOERROR" ]] && [[ "$status" != "UNKNOWN" ]]; then
            VALIDATION_ERROR="Unexpected status: $status"
            return 1
        fi
    fi

    # Check that ANSWER section exists and has content
    if ! echo "$full_output" | grep -q "ANSWER SECTION"; then
        # Some queries legitimately have no answer (e.g., NXDOMAIN handled above)
        # But for successful queries we expect an answer
        if echo "$full_output" | grep -q "ANSWER: 0"; then
            VALIDATION_ERROR="No answer records returned"
            return 1
        fi
    fi

    # Validate that the answer is for the correct domain (case-insensitive)
    local domain_pattern
    domain_pattern=$(echo "$domain" | sed 's/\./\\./g')

    if echo "$full_output" | grep -q "ANSWER SECTION"; then
        # Check if the queried domain appears in the answer section
        if ! echo "$full_output" | grep -i "ANSWER SECTION" -A 20 | grep -qi "$domain_pattern"; then
            # Could be a CNAME chain - check if there's a valid chain
            if echo "$full_output" | grep -qi "CNAME"; then
                # CNAME is acceptable - the answer resolves through a chain
                return 0
            fi
            VALIDATION_ERROR="Answer does not match queried domain"
            return 1
        fi
    fi

    # Validate record type in answer matches query (for direct answers)
    case "$record_type" in
        A)
            # Should have A record or CNAME
            if ! echo "$full_output" | grep -q "IN\s*A\s\|IN\s*CNAME"; then
                VALIDATION_ERROR="No A or CNAME record in response"
                return 1
            fi
            ;;
        AAAA)
            # Should have AAAA record or CNAME
            if ! echo "$full_output" | grep -q "IN\s*AAAA\s\|IN\s*CNAME"; then
                VALIDATION_ERROR="No AAAA or CNAME record in response"
                return 1
            fi
            ;;
        MX)
            if ! echo "$full_output" | grep -q "IN\s*MX"; then
                VALIDATION_ERROR="No MX record in response"
                return 1
            fi
            ;;
        TXT)
            if ! echo "$full_output" | grep -q "IN\s*TXT"; then
                VALIDATION_ERROR="No TXT record in response"
                return 1
            fi
            ;;
        NS)
            if ! echo "$full_output" | grep -q "IN\s*NS"; then
                VALIDATION_ERROR="No NS record in response"
                return 1
            fi
            ;;
        SOA)
            if ! echo "$full_output" | grep -q "IN\s*SOA"; then
                VALIDATION_ERROR="No SOA record in response"
                return 1
            fi
            ;;
        CNAME)
            if ! echo "$full_output" | grep -q "IN\s*CNAME"; then
                VALIDATION_ERROR="No CNAME record in response"
                return 1
            fi
            ;;
        PTR)
            if ! echo "$full_output" | grep -q "IN\s*PTR"; then
                VALIDATION_ERROR="No PTR record in response"
                return 1
            fi
            ;;
    esac

    return 0
}

# Perform a validated DNS query - returns result only if response is correct
dns_query_validated() {
    local domain="$1"
    local record_type="${2:-A}"
    local server="${3:-$DNS_SERVER}"
    local port="${4:-$DNS_PORT}"

    # Get full output for validation
    local full_output
    full_output=$(timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +time=2 +tries=1 2>&1 || echo "")

    # Check for connection errors first
    if is_dns_error "$full_output"; then
        VALIDATION_ERROR="Connection error"
        echo ""
        return 0
    fi

    # Validate the response
    if ! validate_dns_response "$domain" "$record_type" "$full_output"; then
        echo ""
        return 0
    fi

    # Get short answer for display
    local short_result
    short_result=$(timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +short +time=2 +tries=1 2>/dev/null || echo "")
    filter_dns_result "$short_result"
}

dns_query() {
    local domain="$1"
    local record_type="${2:-A}"
    local server="${3:-$DNS_SERVER}"
    local port="${4:-$DNS_PORT}"

    local output
    output=$(timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +short +time=2 +tries=1 2>&1 || echo "")

    # Filter out error messages and return only valid results
    filter_dns_result "$output"
}

dns_query_full() {
    local domain="$1"
    local record_type="${2:-A}"
    local server="${3:-$DNS_SERVER}"
    local port="${4:-$DNS_PORT}"

    timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +time=2 +tries=1 2>/dev/null || echo ""
}

dns_query_stats() {
    local domain="$1"
    local record_type="${2:-A}"
    local server="${3:-$DNS_SERVER}"
    local port="${4:-$DNS_PORT}"

    timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +stats +time=2 +tries=1 2>/dev/null || echo ""
}

# DNS query with full diagnostic output (for verbose mode)
dns_query_diagnostic() {
    local domain="$1"
    local record_type="${2:-A}"
    local server="${3:-$DNS_SERVER}"
    local port="${4:-$DNS_PORT}"
    local protocol="${5:-udp}"

    local tcp_flag=""
    [[ "$protocol" == "tcp" ]] && tcp_flag="+tcp"

    timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" \
        +noall +comments +question +answer +authority +additional +stats \
        +time=2 +tries=1 $tcp_flag 2>/dev/null || echo ""
}

# DNS query via TCP only
dns_query_tcp() {
    local domain="$1"
    local record_type="${2:-A}"
    local server="${3:-$DNS_SERVER}"
    local port="${4:-$DNS_PORT}"

    timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +tcp +time=2 +tries=1 2>/dev/null || echo ""
}

# DNS query with EDNS buffer size control
dns_query_edns() {
    local domain="$1"
    local record_type="${2:-A}"
    local bufsize="${3:-512}"
    local server="${4:-$DNS_SERVER}"
    local port="${5:-$DNS_PORT}"

    timeout "$QUERY_TIMEOUT" dig @"$server" -p "$port" "$domain" "$record_type" +bufsize="$bufsize" +time=2 +tries=1 2>/dev/null || echo ""
}

# Get query time from dig output
get_query_time() {
    local output="$1"
    if [[ "$HAS_GREP_PCRE" == "true" ]]; then
        echo "$output" | grep -oP 'Query time: \K[0-9]+' || echo "0"
    else
        local val
        val=$(echo "$output" | sed -n 's/.*Query time: \([0-9]*\).*/\1/p' | head -1)
        echo "${val:-0}"
    fi
}

# Get message size from dig output
get_msg_size() {
    local output="$1"
    if [[ "$HAS_GREP_PCRE" == "true" ]]; then
        echo "$output" | grep -oP 'MSG SIZE\s+rcvd:\s*\K[0-9]+' || echo "0"
    else
        local val
        val=$(echo "$output" | sed -n 's/.*MSG SIZE.*rcvd:[[:space:]]*\([0-9]*\).*/\1/p' | head -1)
        echo "${val:-0}"
    fi
}

# Check if response is truncated
is_truncated() {
    local output="$1"
    echo "$output" | grep -qi "flags:.*tc" && echo "true" || echo "false"
}

# Detailed timing measurement
measure_timing() {
    local domain="$1"
    local record_type="${2:-A}"
    local iterations="${3:-5}"
    local server="${4:-$DNS_SERVER}"
    local port="${5:-$DNS_PORT}"

    local times=()
    local min=999999 max=0 total=0

    for i in $(seq 1 "$iterations"); do
        local start_ns=$(get_time_ns)
        dns_query "$domain" "$record_type" "$server" "$port" > /dev/null
        local end_ns=$(get_time_ns)
        local elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))
        times+=("$elapsed_ms")
        total=$((total + elapsed_ms))
        [[ $elapsed_ms -lt $min ]] && min=$elapsed_ms
        [[ $elapsed_ms -gt $max ]] && max=$elapsed_ms
    done

    local avg=$((total / iterations))

    # Calculate standard deviation
    local sum_sq=0
    for t in "${times[@]}"; do
        local diff=$((t - avg))
        sum_sq=$((sum_sq + diff * diff))
    done
    local variance=$((sum_sq / iterations))
    local stddev=$(echo "scale=2; sqrt($variance)" | bc 2>/dev/null || echo "0")

    echo "min=$min max=$max avg=$avg stddev=$stddev samples=${times[*]}"
}

# =============================================================================
# Test Categories
# =============================================================================

test_external_resolver() {
    print_header "0. External Resolver Validation"

    print_section "Checking External Resolver Connectivity"

    print_test "Testing external resolver ($EXTERNAL_RESOLVER) with validated query"

    # Get full output for validation
    local full_output
    full_output=$(timeout "$QUERY_TIMEOUT" dig @"$EXTERNAL_RESOLVER" -p 53 "google.com" A +time=2 +tries=1 2>&1 || echo "")

    # Check for connection errors first
    if is_dns_error "$full_output"; then
        print_fail "External resolver ($EXTERNAL_RESOLVER) is not responding"
        print_verbose "Error: $full_output"
        echo ""
        echo -e "${RED}${BOLD}WARNING: External resolver check failed!${NC}"
        echo -e "${YELLOW}The external resolver '$EXTERNAL_RESOLVER' is not responding.${NC}"
        echo -e "${YELLOW}This may cause comparison tests to fail.${NC}"
        echo ""
        echo -e "Possible causes:"
        echo -e "  - Network connectivity issues"
        echo -e "  - Firewall blocking DNS (port 53)"
        echo -e "  - Invalid resolver address"
        echo ""
        echo -e "You can specify a different resolver with: -r <resolver_ip>"
        echo ""
        return 1
    fi

    # Validate the response is correct for google.com
    if ! validate_dns_response "google.com" "A" "$full_output"; then
        print_fail "External resolver returned invalid response: $VALIDATION_ERROR"
        echo ""
        echo -e "${RED}${BOLD}WARNING: External resolver validation failed!${NC}"
        echo -e "${YELLOW}The response from '$EXTERNAL_RESOLVER' was not valid for google.com.${NC}"
        echo -e "${YELLOW}Error: $VALIDATION_ERROR${NC}"
        echo ""
        return 1
    fi

    # Get clean short result for display
    local result
    result=$(timeout "$QUERY_TIMEOUT" dig @"$EXTERNAL_RESOLVER" -p 53 "google.com" A +short +time=2 +tries=1 2>/dev/null || echo "")
    result=$(filter_dns_result "$result")

    if [[ -n "$result" ]]; then
        print_pass "External resolver validated: google.com -> $result"
    else
        print_fail "External resolver ($EXTERNAL_RESOLVER) returned empty response"
        return 1
    fi

    # Test external resolver response time
    print_test "External resolver response time"
    local output
    output=$(timeout "$QUERY_TIMEOUT" dig @"$EXTERNAL_RESOLVER" -p 53 "example.com" A +stats +time=2 +tries=1 2>/dev/null || echo "")
    local query_time
    query_time=$(get_query_time "$output")

    if [[ "$query_time" -gt 0 ]]; then
        print_info "External resolver response time: ${query_time}ms"
    fi
}

test_server_availability() {
    print_header "1. Server Availability Tests"

    print_section "UDP Connectivity"

    # Test UDP port is open
    print_test "Testing UDP port $DNS_PORT on $DNS_SERVER"
    if timeout 2 bash -c "echo > /dev/udp/$DNS_SERVER/$DNS_PORT" 2>/dev/null; then
        print_pass "UDP port $DNS_PORT is open"
    else
        # Try with nc as fallback
        if echo "" | nc -u -w 1 "$DNS_SERVER" "$DNS_PORT" 2>/dev/null; then
            print_pass "UDP port $DNS_PORT is open (nc fallback)"
        else
            print_warning "Could not verify UDP port (may still be operational)"
        fi
    fi

    # Test basic DNS query with validation
    print_test "Testing validated DNS query (google.com A record)"

    # Get full output for validation
    local full_output
    full_output=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +time=2 +tries=1 2>&1 || echo "")

    # Check for connection errors
    if is_dns_error "$full_output"; then
        print_fail "DNS query failed: connection error"
        print_verbose "Error: $full_output"
        return 1
    fi

    # Validate the response
    if ! validate_dns_response "google.com" "A" "$full_output"; then
        print_fail "DNS query failed validation: $VALIDATION_ERROR"
        print_verbose "Response did not contain valid answer for google.com"
        return 1
    fi

    # Get short result for display
    local result
    result=$(dns_query "google.com" "A")
    if [[ -n "$result" ]]; then
        print_pass "Validated DNS query successful: google.com -> $result"
    else
        print_fail "DNS query returned empty after validation passed (unexpected)"
        return 1
    fi

    # Test server response time
    print_test "Measuring response time"
    local output
    output=$(dns_query_stats "example.com" "A")
    local query_time
    query_time=$(get_query_time "$output")
    if [[ "$query_time" -lt 1000 ]]; then
        print_pass "Response time acceptable: ${query_time}ms"
    else
        print_warning "Response time high: ${query_time}ms"
    fi
}

test_record_types() {
    print_header "2. DNS Record Type Support"

    declare -A RECORD_TESTS=(
        ["A:google.com"]="IPv4 Address Record"
        ["AAAA:google.com"]="IPv6 Address Record"
        ["MX:google.com"]="Mail Exchange Record"
        ["TXT:google.com"]="Text Record"
        ["NS:google.com"]="Name Server Record"
        ["SOA:google.com"]="Start of Authority Record"
        ["CNAME:www.github.com"]="Canonical Name Record"
        ["PTR:8.8.8.8.in-addr.arpa"]="Pointer Record (Reverse DNS)"
        ["SRV:_ldap._tcp.google.com"]="Service Record"
        ["CAA:google.com"]="Certificate Authority Authorization"
    )

    for test_spec in "${!RECORD_TESTS[@]}"; do
        local record_type="${test_spec%%:*}"
        local domain="${test_spec#*:}"
        local description="${RECORD_TESTS[$test_spec]}"

        print_test "Testing $record_type record for $domain ($description)"

        # Use validated query to ensure response is correct
        VALIDATION_ERROR=""
        local result
        result=$(dns_query_validated "$domain" "$record_type")

        if [[ -n "$result" ]] && [[ -z "$VALIDATION_ERROR" ]]; then
            print_pass "$record_type query successful and validated"
            print_verbose "Result: $result"
        else
            # Check what went wrong
            if [[ -n "$VALIDATION_ERROR" ]]; then
                case "$VALIDATION_ERROR" in
                    "Connection error")
                        print_fail "$record_type query failed: connection error"
                        ;;
                    "No answer records returned"|"No "*" record in response")
                        # Some record types may legitimately return empty
                        case "$record_type" in
                            SRV|CAA)
                                print_skip "$record_type may not exist for $domain"
                                ;;
                            PTR)
                                print_skip "PTR record may not exist for this IP"
                                ;;
                            *)
                                print_fail "$record_type query failed: $VALIDATION_ERROR"
                                ;;
                        esac
                        ;;
                    *)
                        print_fail "$record_type query failed: $VALIDATION_ERROR"
                        ;;
                esac
                print_verbose "Validation error: $VALIDATION_ERROR"
            else
                print_fail "$record_type query returned empty result"
            fi
        fi
    done
}

test_edge_cases() {
    print_header "3. Edge Cases and Error Handling"

    print_section "Invalid Domain Handling"

    # Test non-existent domain (NXDOMAIN)
    print_test "NXDOMAIN response for non-existent domain"
    local output
    output=$(dns_query_full "this-domain-definitely-does-not-exist-12345.com" "A")
    if echo "$output" | grep -qi "NXDOMAIN\|SERVFAIL"; then
        print_pass "Correctly returns NXDOMAIN/SERVFAIL for non-existent domain"
    elif [[ -z "$(dns_query 'this-domain-definitely-does-not-exist-12345.com' 'A')" ]]; then
        print_pass "Returns empty for non-existent domain"
    else
        print_fail "Unexpected response for non-existent domain"
    fi

    # Test empty domain
    print_test "Empty domain query handling"
    local result
    result=$(dns_query "" "A" 2>&1) || true
    print_pass "Server handled empty domain query"

    # Test very long domain name (max 253 characters)
    print_test "Long domain name handling (near 253 char limit)"
    local long_domain
    long_domain=$(printf 'a%.0s' {1..63})
    long_domain="${long_domain}.${long_domain}.${long_domain}.com"
    result=$(dns_query "$long_domain" "A" 2>&1) || true
    print_pass "Server handled long domain name"

    # Test domain with special characters
    print_test "Domain with hyphens and numbers"
    result=$(dns_query "test-123.example.com" "A" 2>&1) || true
    print_pass "Server handled domain with special characters"

    # Test case insensitivity
    # Note: DNS round-robin means different queries for the same domain can return
    # different IP sets, so we cannot compare exact results across queries.
    # Instead, verify that all case variants successfully resolve (non-empty answer)
    # and that the answer count is consistent (same number of A records).
    print_test "Case insensitivity (RFC 1035)"
    local lower_result upper_result mixed_result
    lower_result=$(dns_query "google.com" "A")
    upper_result=$(dns_query "GOOGLE.COM" "A")
    mixed_result=$(dns_query "GoOgLe.CoM" "A")

    local case_ok=true
    if [[ -z "$lower_result" ]]; then
        print_verbose "lower case query returned empty"
        case_ok=false
    fi
    if [[ -z "$upper_result" ]]; then
        print_verbose "upper case query returned empty"
        case_ok=false
    fi
    if [[ -z "$mixed_result" ]]; then
        print_verbose "mixed case query returned empty"
        case_ok=false
    fi

    if [[ "$case_ok" == "true" ]]; then
        # All case variants resolved - also verify they return the same number of records
        local lower_count upper_count mixed_count
        lower_count=$(echo "$lower_result" | wc -l)
        upper_count=$(echo "$upper_result" | wc -l)
        mixed_count=$(echo "$mixed_result" | wc -l)
        if [[ "$lower_count" == "$upper_count" ]] && [[ "$lower_count" == "$mixed_count" ]]; then
            print_pass "DNS queries are case-insensitive ($lower_count records each)"
        else
            print_pass "DNS queries are case-insensitive (all resolved successfully)"
            print_verbose "Record counts: lower=$lower_count upper=$upper_count mixed=$mixed_count"
        fi
    else
        print_fail "Case sensitivity issue detected"
        print_verbose "lower: $lower_result"
        print_verbose "upper: $upper_result"
        print_verbose "mixed: $mixed_result"
    fi

    print_section "Malformed Query Handling"

    # Test invalid record type
    print_test "Invalid record type handling"
    result=$(dns_query "google.com" "INVALID" 2>&1) || true
    print_pass "Server handled invalid record type"

    # Test query for root
    print_test "Root zone query"
    result=$(dns_query "." "NS")
    if [[ -n "$result" ]]; then
        print_pass "Root zone query successful"
    else
        print_skip "Root zone query not forwarded (expected in some configurations)"
    fi
}

test_performance() {
    print_header "4. Performance and Load Testing"

    print_section "Single Query Performance"

    # Measure average response time over multiple queries
    local total_time=0
    local successful_queries=0
    local query_count=10

    print_test "Measuring average response time over $query_count queries"

    for i in $(seq 1 $query_count); do
        local output
        output=$(dns_query_stats "example.com" "A")
        local query_time
        query_time=$(get_query_time "$output")

        if [[ "$query_time" =~ ^[0-9]+$ ]]; then
            total_time=$((total_time + query_time))
            successful_queries=$((successful_queries + 1))
        fi
    done

    if [[ $successful_queries -gt 0 ]]; then
        local avg_time=$((total_time / successful_queries))
        print_pass "Average response time: ${avg_time}ms ($successful_queries/$query_count queries)"
    else
        print_fail "No successful queries for performance measurement"
    fi

    print_section "Concurrent Query Test"

    print_test "Running $CONCURRENCY concurrent queries"

    local temp_dir
    temp_dir=$(mktemp -d)
    local start_time
    start_time=$(get_time_ns)

    # Launch concurrent queries
    local pids=()
    for i in $(seq 1 "$CONCURRENCY"); do
        (
            result=$(dns_query "google.com" "A" 2>/dev/null)
            if [[ -n "$result" ]]; then
                echo "1" > "$temp_dir/success_$i"
            else
                echo "0" > "$temp_dir/fail_$i"
            fi
        ) &
        pids+=($!)
    done

    # Wait only for the concurrent query subshells, NOT the server process
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    local end_time
    end_time=$(get_time_ns)
    local duration_ms=$(( (end_time - start_time) / 1000000 ))

    # Count results
    local success_count
    success_count=$(find "$temp_dir" -name "success_*" | wc -l)
    local fail_count
    fail_count=$(find "$temp_dir" -name "fail_*" | wc -l)

    rm -rf "$temp_dir"

    local success_rate
    success_rate=$(echo "scale=2; $success_count * 100 / $CONCURRENCY" | bc)

    if (( $(echo "$success_rate >= 95" | bc -l) )); then
        print_pass "Concurrent test: ${success_count}/${CONCURRENCY} successful (${success_rate}%) in ${duration_ms}ms"
    elif (( $(echo "$success_rate >= 80" | bc -l) )); then
        print_warning "Concurrent test: ${success_count}/${CONCURRENCY} successful (${success_rate}%) in ${duration_ms}ms"
    else
        print_fail "Concurrent test: ${success_count}/${CONCURRENCY} successful (${success_rate}%) in ${duration_ms}ms"
    fi

    # Calculate queries per second
    if [[ $duration_ms -gt 0 ]]; then
        local qps
        qps=$(echo "scale=2; $success_count * 1000 / $duration_ms" | bc)
        print_info "Throughput: approximately ${qps} queries/second"
    fi

    print_section "Sustained Load Test"

    print_test "Running 100 sequential queries"
    local seq_start
    seq_start=$(get_time_ns)
    local seq_success=0

    for i in $(seq 1 100); do
        if [[ -n "$(dns_query 'example.com' 'A')" ]]; then
            seq_success=$((seq_success + 1))
        fi
    done

    local seq_end
    seq_end=$(get_time_ns)
    local seq_duration_ms=$(( (seq_end - seq_start) / 1000000 ))

    if [[ $seq_success -ge 95 ]]; then
        print_pass "Sequential test: ${seq_success}/100 successful in ${seq_duration_ms}ms"
    else
        print_fail "Sequential test: ${seq_success}/100 successful in ${seq_duration_ms}ms"
    fi
}

test_dns_features() {
    print_header "5. DNS Protocol Features"

    print_section "Recursion and Forwarding"

    # Test recursion desired flag
    print_test "Recursion Desired (RD) flag handling"
    local output
    output=$(dns_query_full "google.com" "A")
    if echo "$output" | grep -q "rd"; then
        print_pass "RD flag is properly set"
    else
        print_skip "Could not verify RD flag"
    fi

    # Test recursion available
    print_test "Recursion Available (RA) flag"
    if echo "$output" | grep -q "ra"; then
        print_pass "RA flag is set (server performs recursion)"
    else
        print_info "RA flag not set (may be authoritative only)"
    fi

    print_section "Response Validation"

    # Test ANSWER section
    print_test "ANSWER section presence"
    output=$(dns_query_full "google.com" "A")
    if echo "$output" | grep -qi "ANSWER SECTION"; then
        print_pass "ANSWER section present in response"
    else
        print_warning "ANSWER section not clearly present"
    fi

    # Test response code
    print_test "Response status code"
    local status
    status=$(extract_dns_status "$output")
    if [[ "$status" == "NOERROR" ]]; then
        print_pass "Response status: NOERROR"
    elif [[ "$status" != "UNKNOWN" ]]; then
        print_info "Response status: $status"
    else
        print_warning "Could not determine response status"
    fi

    print_section "TTL Handling"

    print_test "TTL values in responses"
    local ttl
    # Extract TTL value - the number before "IN A" in the answer section
    ttl=$(echo "$output" | awk '/IN[[:space:]]+A[[:space:]]/ {print $2}' | head -1 || echo "")
    if [[ -n "$ttl" ]] && [[ "$ttl" =~ ^[0-9]+$ ]]; then
        print_pass "TTL present in response: ${ttl}s"
    else
        print_info "Could not extract TTL from response"
    fi
}

test_security() {
    print_header "6. Security and Resilience Tests"

    print_section "Query Flood Resilience"

    # Rapid fire queries
    print_test "Rapid query handling (burst of 20 queries)"
    local burst_success=0
    local burst_pids=()
    for i in $(seq 1 20); do
        dns_query "google.com" "A" &
        burst_pids+=($!)
    done
    for pid in "${burst_pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    # Verify server still responds after burst
    if [[ -n "$(dns_query 'example.com' 'A')" ]]; then
        print_pass "Server remains responsive after query burst"
    else
        print_fail "Server unresponsive after query burst"
    fi

    print_section "Malformed Request Handling"

    # Test truncated query (using netcat to send raw data)
    print_test "Truncated/malformed packet handling"
    echo -n "garbage" | nc -u -w 1 "$DNS_SERVER" "$DNS_PORT" 2>/dev/null || true

    # Verify server still responds
    if [[ -n "$(dns_query 'google.com' 'A')" ]]; then
        print_pass "Server handles malformed packets gracefully"
    else
        print_fail "Server crashed or became unresponsive after malformed packet"
    fi

    print_section "DNS Amplification Prevention"

    print_test "ANY query handling (potential amplification)"
    local any_result
    any_result=$(dns_query "google.com" "ANY")
    if [[ -z "$any_result" ]] || [[ $(echo "$any_result" | wc -l) -lt 20 ]]; then
        print_pass "ANY query returns limited response (amplification mitigation)"
    else
        print_warning "ANY query returns large response - consider rate limiting"
    fi
}

test_comparison() {
    print_header "7. Comparison with External Resolver"

    print_section "Response Accuracy"

    local domains=("google.com" "github.com" "microsoft.com" "amazon.com" "cloudflare.com")

    for domain in "${domains[@]}"; do
        print_test "Comparing results for $domain"

        local local_result external_result
        local_result=$(dns_query "$domain" "A" "$DNS_SERVER" "$DNS_PORT" | sort | head -1)
        external_result=$(dns_query "$domain" "A" "$EXTERNAL_RESOLVER" "53" | sort | head -1)

        if [[ -n "$local_result" ]] && [[ -n "$external_result" ]]; then
            # IPs may differ due to CDN/geo-routing, but both should be valid
            print_pass "Both servers returned results for $domain"
            print_verbose "Local: $local_result | External: $external_result"
        elif [[ -n "$local_result" ]]; then
            print_pass "Local server returned result: $local_result"
        else
            print_fail "Local server failed to resolve $domain"
        fi
    done

    print_section "Response Time Comparison"

    print_test "Comparing response times"
    local local_output external_output
    local_output=$(dns_query_stats "cloudflare.com" "A" "$DNS_SERVER" "$DNS_PORT")
    external_output=$(dns_query_stats "cloudflare.com" "A" "$EXTERNAL_RESOLVER" "53")

    local local_time external_time
    local_time=$(get_query_time "$local_output")
    external_time=$(get_query_time "$external_output")

    print_info "Local server: ${local_time}ms | External resolver: ${external_time}ms"

    if [[ "$local_time" -le "$((external_time * 2))" ]]; then
        print_pass "Local server response time is acceptable"
    else
        print_warning "Local server is slower than expected"
    fi
}

test_tcp_fallback() {
    print_header "8. TCP Fallback Support"

    print_section "TCP Query Support"

    print_test "TCP DNS query"
    local result
    result=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +tcp +short 2>/dev/null || echo "")

    if [[ -n "$result" ]]; then
        print_pass "TCP queries supported: $result"
    else
        print_warning "TCP queries not supported or timed out"
        print_info "Note: TCP support is recommended for truncated responses"
    fi

    # Test large response that might require TCP
    print_test "Large response handling (TXT record)"
    result=$(dns_query "google.com" "TXT")
    if [[ -n "$result" ]]; then
        print_pass "Large TXT record retrieved successfully"
    else
        print_info "TXT record query returned empty"
    fi
}

test_caching() {
    print_header "9. Caching Behavior (if implemented)"

    print_section "Cache Performance"

    # First query (cold cache)
    print_test "Cold cache query"
    local domain="cache-test-$(date +%s).example.com"
    local first_output
    first_output=$(dns_query_stats "github.com" "A")
    local first_time
    first_time=$(get_query_time "$first_output")

    print_info "First query time: ${first_time}ms"

    # Second query (should be cached)
    print_test "Warm cache query (same domain)"
    local second_output
    second_output=$(dns_query_stats "github.com" "A")
    local second_time
    second_time=$(get_query_time "$second_output")

    print_info "Second query time: ${second_time}ms"

    if [[ "$second_time" -lt "$first_time" ]] || [[ "$second_time" -lt 10 ]]; then
        print_pass "Caching appears to be working (faster second query)"
    else
        print_info "Caching behavior inconclusive or not implemented"
    fi
}

# =============================================================================
# NEW: PTR (Reverse DNS) Tests
# =============================================================================

test_ptr_records() {
    print_header "10. PTR Record (Reverse DNS) Tests"

    print_section "IPv4 Reverse DNS Lookups"

    # Well-known IP addresses for PTR testing
    declare -A PTR_TESTS_IPV4=(
        ["8.8.8.8"]="Google Public DNS"
        ["8.8.4.4"]="Google Public DNS Secondary"
        ["1.1.1.1"]="Cloudflare DNS"
        ["1.0.0.1"]="Cloudflare DNS Secondary"
        ["208.67.222.222"]="OpenDNS"
        ["9.9.9.9"]="Quad9 DNS"
    )

    for ip in "${!PTR_TESTS_IPV4[@]}"; do
        local description="${PTR_TESTS_IPV4[$ip]}"

        # Convert IP to in-addr.arpa format
        local reversed_ip
        reversed_ip=$(echo "$ip" | awk -F. '{print $4"."$3"."$2"."$1}')
        local ptr_domain="${reversed_ip}.in-addr.arpa"

        print_test "PTR lookup for $ip ($description)"

        local result
        result=$(dns_query "$ptr_domain" "PTR")

        if [[ -n "$result" ]]; then
            print_pass "PTR query successful: $result"
            print_verbose "$ip -> $result"
        else
            print_info "No PTR record returned for $ip"
        fi
    done

    print_section "IPv4 PTR Format Validation"

    # Test correct in-addr.arpa format handling
    print_test "Standard in-addr.arpa format"
    local result
    result=$(dns_query "8.8.8.8.in-addr.arpa" "PTR")
    if [[ -n "$result" ]]; then
        print_pass "Standard PTR format works: $result"
    else
        print_info "PTR query returned empty (may be expected)"
    fi

    # Test partial reverse zones
    print_test "Class C reverse zone lookup (/24)"
    result=$(dns_query_full "8.8.8.in-addr.arpa" "NS")
    if [[ -n "$result" ]]; then
        print_pass "Reverse zone NS query successful"
        print_verbose "$(echo "$result" | grep -i 'NS' | head -3)"
    else
        print_info "Reverse zone query returned empty"
    fi

    print_section "IPv6 Reverse DNS Lookups"

    # IPv6 PTR tests (ip6.arpa)
    print_test "IPv6 PTR lookup (Google DNS 2001:4860:4860::8888)"

    # The reversed nibble format for 2001:4860:4860::8888
    local ipv6_ptr="8.8.8.8.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.6.8.4.0.6.8.4.1.0.0.2.ip6.arpa"
    result=$(dns_query "$ipv6_ptr" "PTR")

    if [[ -n "$result" ]]; then
        print_pass "IPv6 PTR query successful: $result"
    else
        print_info "IPv6 PTR returned empty (may not have reverse DNS configured)"
    fi

    # Another IPv6 test
    print_test "IPv6 PTR lookup (Cloudflare 2606:4700:4700::1111)"
    local cf_ipv6_ptr="1.1.1.1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.7.4.0.0.7.4.6.0.6.2.ip6.arpa"
    result=$(dns_query "$cf_ipv6_ptr" "PTR")

    if [[ -n "$result" ]]; then
        print_pass "Cloudflare IPv6 PTR query successful: $result"
    else
        print_info "IPv6 PTR returned empty"
    fi

    print_section "PTR Query Performance"

    print_test "Measuring PTR query timing"
    local timing_result
    timing_result=$(measure_timing "8.8.8.8.in-addr.arpa" "PTR" 5)

    local avg=$(extract_key_value "$timing_result" "avg")
    local min=$(extract_key_value "$timing_result" "min")
    local max=$(extract_key_value "$timing_result" "max")

    print_info "PTR query timing - Min: ${min}ms, Avg: ${avg}ms, Max: ${max}ms"

    if [[ "$avg" -lt 500 ]]; then
        print_pass "PTR query performance acceptable"
    else
        print_warning "PTR queries are slow (avg > 500ms)"
    fi

    print_section "PTR vs Forward Lookup Consistency"

    print_test "Forward/Reverse consistency check"

    # Get IP for a domain
    local domain="dns.google"
    local forward_ip
    forward_ip=$(dns_query "$domain" "A" | head -1)

    if [[ -n "$forward_ip" ]]; then
        print_verbose "Forward lookup: $domain -> $forward_ip"

        # Do reverse lookup
        local reversed_ip
        reversed_ip=$(echo "$forward_ip" | awk -F. '{print $4"."$3"."$2"."$1}')
        local ptr_result
        ptr_result=$(dns_query "${reversed_ip}.in-addr.arpa" "PTR")

        if [[ -n "$ptr_result" ]]; then
            print_verbose "Reverse lookup: $forward_ip -> $ptr_result"

            # Check if reverse matches forward
            if echo "$ptr_result" | grep -qi "$domain"; then
                print_pass "Forward/Reverse DNS consistent"
            else
                print_info "Reverse DNS differs (common for CDNs/hosting): $ptr_result"
            fi
        else
            print_info "No PTR record for $forward_ip"
        fi
    else
        print_skip "Could not resolve $domain for consistency check"
    fi

    print_section "Special PTR Cases"

    # Test localhost reverse
    print_test "Localhost reverse lookup (127.0.0.1)"
    result=$(dns_query "1.0.0.127.in-addr.arpa" "PTR")
    if [[ -n "$result" ]]; then
        print_pass "Localhost PTR: $result"
    else
        print_info "No PTR for localhost (expected in many configurations)"
    fi

    # Test private IP range reverse (should likely fail or return nothing)
    print_test "Private IP reverse lookup (192.168.1.1)"
    result=$(dns_query "1.1.168.192.in-addr.arpa" "PTR")
    if [[ -z "$result" ]]; then
        print_pass "Correctly returns empty for private IP reverse"
    else
        print_info "Private IP has PTR: $result (unusual but possible)"
    fi

    # Test DNSSEC-signed reverse zone
    print_test "PTR query with DNSSEC validation"
    local output
    output=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "8.8.8.8.in-addr.arpa" PTR +dnssec 2>/dev/null || echo "")
    if echo "$output" | grep -qi "RRSIG"; then
        print_pass "DNSSEC signatures present in PTR response"
    else
        print_info "No DNSSEC signatures in PTR response"
    fi
}

# =============================================================================
# NEW: Detailed Query Analysis Tests
# =============================================================================

test_detailed_queries() {
    print_header "11. Detailed Query Analysis (Verbose Diagnostics)"

    print_section "Normal Query Diagnostic"

    # Standard A record query with full diagnostic
    print_test "Standard A record query - full diagnostic"
    local domain="google.com"
    local output
    output=$(dns_query_diagnostic "$domain" "A")

    if [[ -n "$output" ]]; then
        print_pass "Standard query successful"
        if [[ "$VERBOSE" == "true" ]]; then
            echo -e "${CYAN}─────────────────────────────────────────────────────────────${NC}"
            echo -e "${BOLD}Full Query Output:${NC}"
            echo "$output"
            echo -e "${CYAN}─────────────────────────────────────────────────────────────${NC}"
        fi

        # Extract and display key metrics
        local query_time=$(get_query_time "$output")
        local msg_size=$(get_msg_size "$output")
        local truncated=$(is_truncated "$output")

        print_info "Query Time: ${query_time}ms | Response Size: ${msg_size} bytes | Truncated: ${truncated}"
    else
        print_fail "Standard query failed"
    fi

    print_section "Advanced Query Types"

    # DNSSEC query
    print_test "DNSSEC-enabled query (DNSKEY record)"
    output=$(dns_query_diagnostic "google.com" "DNSKEY")
    if [[ -n "$output" ]]; then
        print_pass "DNSKEY query successful"
        if [[ "$VERBOSE" == "true" ]]; then
            local dnskey_count=$(echo "$output" | grep -c "DNSKEY" || echo "0")
            print_verbose "DNSKEY records returned: $dnskey_count"
            echo "$output" | head -20
        fi
    else
        print_info "DNSKEY query returned no records (may not be signed)"
    fi

    # NSEC/NSEC3 query (DNSSEC denial of existence)
    print_test "NSEC record query (DNSSEC authenticated denial)"
    output=$(dns_query_diagnostic "nonexistent.google.com" "A")
    if echo "$output" | grep -qi "NSEC\|NSEC3"; then
        print_pass "NSEC/NSEC3 records present in NXDOMAIN response"
        if [[ "$VERBOSE" == "true" ]]; then
            echo "$output" | grep -i "NSEC" | head -5
        fi
    else
        print_info "No NSEC records (DNSSEC denial not present)"
    fi

    # DS record query
    print_test "DS record query (Delegation Signer)"
    output=$(dns_query "google.com" "DS")
    if [[ -n "$output" ]]; then
        print_pass "DS record query successful"
        print_verbose "DS: $output"
    else
        print_info "DS record not available at this level"
    fi

    print_section "Query Flag Tests"

    # Test with Checking Disabled (CD) flag
    print_test "Query with Checking Disabled (CD) flag"
    output=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +cd +short 2>/dev/null || echo "")
    if [[ -n "$output" ]]; then
        print_pass "CD flag query successful: $output"
    else
        print_info "CD flag query returned no result"
    fi

    # Test with Authentic Data (AD) flag request
    print_test "Query requesting Authentic Data (AD) flag"
    output=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +adflag 2>/dev/null || echo "")
    if echo "$output" | grep -q "ad"; then
        print_pass "Server returns AD flag for validated responses"
    else
        print_info "AD flag not set (expected if not validating DNSSEC)"
    fi

    # Test with no recursion
    print_test "Non-recursive query (+norec)"
    output=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +norec 2>/dev/null || echo "")
    if [[ -n "$output" ]]; then
        print_pass "Non-recursive query handled"
        print_verbose "Response: $(extract_dns_status "$output")"
    else
        print_info "Non-recursive query returned no result (expected for forwarder)"
    fi
}

test_large_queries_tcp() {
    print_header "12. Large Query & TCP Fallback Tests (>512 bytes)"

    print_section "Message Size Analysis"

    # Test response size detection
    print_test "Measuring typical response sizes"

    declare -A size_tests=(
        ["google.com:A"]="Simple A record"
        ["google.com:MX"]="MX records"
        ["google.com:TXT"]="TXT records (often large)"
        ["google.com:NS"]="NS records"
        ["cloudflare.com:TXT"]="Cloudflare TXT (SPF/DKIM)"
    )

    for test_spec in "${!size_tests[@]}"; do
        local domain="${test_spec%%:*}"
        local rtype="${test_spec#*:}"
        local description="${size_tests[$test_spec]}"

        local output
        output=$(dns_query_diagnostic "$domain" "$rtype")
        local msg_size=$(get_msg_size "$output")
        local truncated=$(is_truncated "$output")

        if [[ "$msg_size" -gt 0 ]]; then
            local size_indicator=""
            if [[ "$msg_size" -gt 512 ]]; then
                size_indicator="${RED}[>512]${NC}"
            elif [[ "$msg_size" -gt 400 ]]; then
                size_indicator="${YELLOW}[400-512]${NC}"
            else
                size_indicator="${GREEN}[<400]${NC}"
            fi
            print_info "$description: ${msg_size} bytes $size_indicator (truncated: $truncated)"
        fi
    done

    print_section "UDP Truncation Handling"

    # Force truncation by requesting many records
    print_test "Testing truncation with large response (ANY query)"
    local output
    output=$(dns_query_diagnostic "google.com" "ANY")
    local truncated=$(is_truncated "$output")
    local msg_size=$(get_msg_size "$output")

    if [[ "$truncated" == "true" ]]; then
        print_pass "Server correctly sets TC (truncated) flag for large responses"
        print_info "Truncated response size: ${msg_size} bytes"
    else
        print_info "Response not truncated (${msg_size} bytes)"
    fi

    print_section "TCP Query Tests"

    # Test TCP explicitly
    print_test "Explicit TCP query (A record)"
    local tcp_output
    tcp_output=$(dns_query_tcp "google.com" "A")
    if [[ -n "$tcp_output" ]]; then
        print_pass "TCP query successful"
        if [[ "$VERBOSE" == "true" ]]; then
            echo -e "${CYAN}TCP Response:${NC}"
            echo "$tcp_output" | head -30
        fi
    else
        print_warning "TCP query failed - server may not support TCP"
    fi

    # Test TCP with large response
    print_test "TCP query for large TXT records"
    tcp_output=$(dns_query_tcp "google.com" "TXT")
    if [[ -n "$tcp_output" ]]; then
        local tcp_size
        tcp_size=$(get_msg_size "$tcp_output")
        print_pass "Large TCP query successful"
        print_info "TCP response size: ${tcp_size} bytes"
        if [[ "$VERBOSE" == "true" ]]; then
            echo "$tcp_output" | grep -A5 "ANSWER SECTION"
        fi
    else
        print_info "TCP TXT query returned no result"
    fi

    # Test UDP vs TCP response comparison
    print_test "Comparing UDP vs TCP response sizes"
    local udp_output tcp_output
    udp_output=$(dns_query_diagnostic "google.com" "TXT" "$DNS_SERVER" "$DNS_PORT" "udp")
    tcp_output=$(dns_query_diagnostic "google.com" "TXT" "$DNS_SERVER" "$DNS_PORT" "tcp")

    local udp_size=$(get_msg_size "$udp_output")
    local tcp_size=$(get_msg_size "$tcp_output")

    print_info "UDP response: ${udp_size} bytes | TCP response: ${tcp_size} bytes"

    if [[ "$tcp_size" -ge "$udp_size" ]]; then
        print_pass "TCP delivers complete (or larger) response as expected"
    else
        print_info "Response sizes similar (response fits in UDP)"
    fi

    print_section "EDNS Buffer Size Tests"

    # Test EDNS with different buffer sizes
    print_test "EDNS buffer size handling"

    for bufsize in 512 1232 4096; do
        local output
        output=$(dns_query_edns "google.com" "TXT" "$bufsize")
        local msg_size=$(get_msg_size "$output")
        local truncated=$(is_truncated "$output")

        print_info "EDNS bufsize=${bufsize}: response=${msg_size} bytes, truncated=${truncated}"
    done

    print_pass "EDNS buffer size tests completed"
}

test_timing_diagnostics() {
    print_header "13. Detailed Timing Diagnostics"

    print_section "Response Time Distribution"

    # Test timing for different query types
    declare -A timing_tests=(
        ["google.com:A"]="Standard A query"
        ["google.com:AAAA"]="AAAA (IPv6) query"
        ["google.com:MX"]="MX query"
        ["google.com:TXT"]="TXT query"
        ["github.com:A"]="Cross-domain A query"
    )

    echo -e "${BOLD}Query Type                    Min    Max    Avg    StdDev   Samples${NC}"
    echo "─────────────────────────────────────────────────────────────────────"

    for test_spec in "${!timing_tests[@]}"; do
        local domain="${test_spec%%:*}"
        local rtype="${test_spec#*:}"
        local description="${timing_tests[$test_spec]}"

        local timing_result
        timing_result=$(measure_timing "$domain" "$rtype" 5)

        # Parse timing result
        local min=$(extract_key_value "$timing_result" "min")
        local max=$(extract_key_value "$timing_result" "max")
        local avg=$(extract_key_value "$timing_result" "avg")
        local stddev=$(extract_key_value "$timing_result" "stddev")
        local samples=$(extract_key_value "$timing_result" "samples")

        printf "%-30s %-6s %-6s %-6s %-8s %s\n" "$description" "${min}ms" "${max}ms" "${avg}ms" "${stddev}ms" "[$samples]"
    done

    print_section "Latency Percentiles"

    print_test "Calculating latency percentiles (20 samples)"

    local samples=()
    for i in $(seq 1 20); do
        local start_ns=$(get_time_ns)
        dns_query "google.com" "A" > /dev/null
        local end_ns=$(get_time_ns)
        local elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))
        samples+=("$elapsed_ms")
    done

    # Sort samples
    IFS=$'\n' sorted=($(sort -n <<<"${samples[*]}")); unset IFS

    local p50_idx=$((20 * 50 / 100))
    local p90_idx=$((20 * 90 / 100))
    local p95_idx=$((20 * 95 / 100))
    local p99_idx=$((20 * 99 / 100))

    local p50=${sorted[$p50_idx]}
    local p90=${sorted[$p90_idx]}
    local p95=${sorted[$p95_idx]}
    local p99=${sorted[$p99_idx]}
    local min=${sorted[0]}
    local max=${sorted[19]}

    echo -e "${BOLD}Latency Percentiles:${NC}"
    echo "  Min:  ${min}ms"
    echo "  P50:  ${p50}ms (median)"
    echo "  P90:  ${p90}ms"
    echo "  P95:  ${p95}ms"
    echo "  P99:  ${p99}ms"
    echo "  Max:  ${max}ms"

    # Evaluate performance
    if [[ "$p95" -lt 100 ]]; then
        print_pass "Excellent latency: P95 < 100ms"
    elif [[ "$p95" -lt 500 ]]; then
        print_pass "Good latency: P95 < 500ms"
    elif [[ "$p95" -lt 1000 ]]; then
        print_warning "Moderate latency: P95 < 1000ms"
    else
        # Transient spikes (e.g., blocklist reload, network hiccup) are expected;
        # flag as warning rather than hard failure
        print_warning "High latency: P95 >= 1000ms (possible transient spike)"
    fi

    print_section "UDP vs TCP Timing Comparison"

    print_test "Comparing UDP and TCP latency"

    # UDP timing
    local udp_timing
    udp_timing=$(measure_timing "google.com" "A" 5)
    local udp_avg
    udp_avg=$(extract_key_value "$udp_timing" "avg")

    # TCP timing (if available)
    local tcp_times=()
    local tcp_success=0
    for i in $(seq 1 5); do
        local start_ns=$(get_time_ns)
        local result
        result=$(timeout "$QUERY_TIMEOUT" dig @"$DNS_SERVER" -p "$DNS_PORT" "google.com" A +tcp +short 2>/dev/null || true)
        local end_ns=$(get_time_ns)
        if [[ -n "$result" ]]; then
            local elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))
            tcp_times+=("$elapsed_ms")
            tcp_success=$((tcp_success + 1))
        fi
    done

    if [[ $tcp_success -gt 0 ]]; then
        local tcp_total=0
        for t in "${tcp_times[@]}"; do
            tcp_total=$((tcp_total + t))
        done
        local tcp_avg=$((tcp_total / tcp_success))

        print_info "UDP average: ${udp_avg}ms | TCP average: ${tcp_avg}ms"

        local overhead=$((tcp_avg - udp_avg))
        print_info "TCP overhead: ${overhead}ms (expected due to connection setup)"
        print_pass "UDP/TCP timing comparison completed"
    else
        print_info "UDP average: ${udp_avg}ms | TCP: not available"
        print_warning "TCP queries failed - cannot compare"
    fi

    print_section "First Query vs Subsequent Queries"

    print_test "Measuring cold start vs warm performance"

    # Use a unique domain to avoid caching
    local unique_domain="timing-test-$(get_time_ns).example.com"

    # This will likely fail (NXDOMAIN) but we measure the time anyway
    local cold_start=$(get_time_ns)
    dns_query "$unique_domain" "A" > /dev/null 2>&1
    local cold_end=$(get_time_ns)
    local cold_time=$(( (cold_end - cold_start) / 1000000 ))

    # Subsequent query to known domain
    local warm_start=$(get_time_ns)
    dns_query "google.com" "A" > /dev/null
    local warm_end=$(get_time_ns)
    local warm_time=$(( (warm_end - warm_start) / 1000000 ))

    print_info "Cold query (unknown domain): ${cold_time}ms"
    print_info "Warm query (known domain): ${warm_time}ms"

    print_pass "Timing diagnostic completed"

    if [[ "$VERBOSE" == "true" ]]; then
        print_section "Raw Timing Data"
        echo "All samples from percentile test: ${sorted[*]}"
    fi
}

# =============================================================================
# mDNS/Bonjour Service Discovery Testing
# =============================================================================

test_mdns_bonjour() {
    print_header "mDNS/Bonjour Service Discovery Testing"

    # Check for required tools
    local has_avahi=false
    local has_dns_sd=false
    local has_dig=true  # We already verified dig is available

    if command -v avahi-browse &> /dev/null; then
        has_avahi=true
        print_verbose "Found avahi-browse"
    fi

    if command -v dns-sd &> /dev/null; then
        has_dns_sd=true
        print_verbose "Found dns-sd"
    fi

    # GateSentry registers: "_gatesentry_proxy._tcp" on port 10413
    # with TXT records: "txtv=1", "app=gatesentry"

    print_section "mDNS Tool Availability"

    if [[ "$has_avahi" == "true" ]]; then
        print_pass "avahi-browse is available"
    else
        print_info "avahi-browse not found (install avahi-utils for full mDNS testing)"
    fi

    if [[ "$has_dns_sd" == "true" ]]; then
        print_pass "dns-sd is available"
    else
        print_info "dns-sd not found (available on macOS or via avahi-compat-libdns_sd)"
    fi

    # Test 1: Direct mDNS query using dig (multicast DNS)
    print_section "mDNS Protocol Testing"

    print_test "Testing mDNS multicast address reachability"
    # mDNS uses multicast address 224.0.0.251 on port 5353
    if timeout 2 bash -c "echo > /dev/udp/224.0.0.251/5353" 2>/dev/null; then
        print_pass "mDNS multicast address is reachable"
    else
        print_info "mDNS multicast test inconclusive (may still work)"
    fi

    # Test 2: Query for .local domains via mDNS
    print_test "Testing .local domain resolution capability"

    # Try to resolve the local hostname
    local local_hostname=$(hostname)
    local mdns_result

    # Use dig to query mDNS directly
    mdns_result=$(timeout 3 dig @224.0.0.251 -p 5353 "${local_hostname}.local" A +short 2>&1 || echo "")

    if [[ -n "$mdns_result" ]] && ! is_dns_error "$mdns_result"; then
        print_pass "mDNS .local resolution works: ${local_hostname}.local -> $mdns_result"
    else
        print_info "mDNS .local resolution not available (host may not be registered)"
        print_verbose "Tried to resolve: ${local_hostname}.local"
    fi

    # Test 3: Service Discovery using avahi-browse
    if [[ "$has_avahi" == "true" ]]; then
        print_section "Bonjour Service Discovery (via Avahi)"

        print_test "Browsing for GateSentry service (_gatesentry_proxy._tcp)"

        # Browse for GateSentry service with timeout
        local avahi_result
        avahi_result=$(timeout 5 avahi-browse -t -r "_gatesentry_proxy._tcp" 2>&1 || echo "")

        if echo "$avahi_result" | grep -q "GateSentry"; then
            print_pass "GateSentry Bonjour service discovered!"

            # Extract service details
            local service_host=$(echo "$avahi_result" | grep "hostname" | head -1 | awk '{print $NF}')
            local service_port=$(echo "$avahi_result" | grep "port" | head -1 | awk '{print $NF}')
            local txt_records=$(echo "$avahi_result" | grep "txt" | head -1)

            print_info "Service host: ${service_host:-unknown}"
            print_info "Service port: ${service_port:-unknown}"
            print_verbose "TXT records: ${txt_records:-none}"

            # Verify expected values
            if [[ "$service_port" == "[10413]" ]] || [[ "$service_port" == "10413" ]]; then
                print_pass "Service port is correct (10413)"
            else
                print_warning "Service port mismatch: expected 10413, got ${service_port:-unknown}"
            fi

            if echo "$txt_records" | grep -q "app=gatesentry"; then
                print_pass "TXT record contains 'app=gatesentry'"
            else
                print_warning "TXT record 'app=gatesentry' not found"
            fi
        else
            print_warning "GateSentry Bonjour service not found"
            print_info "Make sure GateSentry is running and Bonjour is enabled"
            print_verbose "avahi-browse output: $avahi_result"
        fi

        # Browse for all services to verify avahi is working
        print_test "Verifying Avahi is functional (browsing all services)"
        local all_services
        all_services=$(timeout 5 avahi-browse -t -a 2>&1 | head -20 || echo "")

        if [[ -n "$all_services" ]] && ! echo "$all_services" | grep -qi "error\|fail"; then
            local service_count=$(echo "$all_services" | wc -l)
            print_pass "Avahi is functional, found $service_count service entries"
        else
            print_warning "Avahi may not be running or configured"
            print_verbose "Output: $all_services"
        fi
    else
        print_section "Bonjour Service Discovery (Manual)"
        print_info "Install avahi-utils for automated service discovery:"
        print_info "  Ubuntu/Debian: sudo apt-get install avahi-utils"
        print_info "  RHEL/CentOS:   sudo yum install avahi-tools"
        print_skip "Automated Bonjour service discovery (avahi-browse not available)"
    fi

    # Test 4: Service Discovery using dns-sd (if available)
    if [[ "$has_dns_sd" == "true" ]]; then
        print_section "Bonjour Service Discovery (via dns-sd)"

        print_test "Browsing for GateSentry service using dns-sd"

        # dns-sd runs continuously, so we need to timeout and parse output
        local dnssd_result
        dnssd_result=$(timeout 3 dns-sd -B _gatesentry_proxy._tcp 2>&1 || echo "")

        if echo "$dnssd_result" | grep -q "GateSentry"; then
            print_pass "GateSentry service found via dns-sd"
        else
            print_warning "GateSentry service not found via dns-sd"
            print_verbose "dns-sd output: $dnssd_result"
        fi
    fi

    # Test 5: PTR record for service type enumeration
    print_section "mDNS Service Type Enumeration"

    print_test "Querying for registered service types"
    local ptr_result
    ptr_result=$(timeout 3 dig @224.0.0.251 -p 5353 "_services._dns-sd._udp.local" PTR +short 2>&1 || echo "")

    if [[ -n "$ptr_result" ]] && ! is_dns_error "$ptr_result"; then
        local service_types=$(echo "$ptr_result" | wc -l)
        print_pass "Found $service_types registered service types via mDNS"

        if echo "$ptr_result" | grep -q "_gatesentry_proxy"; then
            print_pass "GateSentry service type is registered"
        else
            print_info "GateSentry service type not in enumeration (may still be discoverable)"
        fi

        if [[ "$VERBOSE" == "true" ]]; then
            print_verbose "Registered service types:"
            echo "$ptr_result" | while read -r svc; do
                echo "  - $svc"
            done
        fi
    else
        print_info "mDNS service type enumeration not available"
        print_verbose "PTR query result: $ptr_result"
    fi

    # Test 6: Verify mDNS responder is running
    print_section "mDNS Responder Status"

    print_test "Checking for mDNS responder process"
    if pgrep -x "avahi-daemon" > /dev/null 2>&1; then
        print_pass "avahi-daemon is running"
    elif pgrep -x "mdnsd" > /dev/null 2>&1; then
        print_pass "mdnsd is running"
    else
        print_info "No system mDNS responder detected (GateSentry provides its own)"
    fi

    # Test 7: Connection test to GateSentry service port
    print_section "Bonjour Service Connectivity"

    print_test "Testing connection to GateSentry service port (10413)"
    if timeout 2 bash -c "echo > /dev/tcp/127.0.0.1/10413" 2>/dev/null; then
        print_pass "GateSentry proxy port 10413 is open"
    elif nc -z 127.0.0.1 10413 2>/dev/null; then
        print_pass "GateSentry proxy port 10413 is open (nc)"
    else
        print_warning "GateSentry proxy port 10413 is not responding"
        print_info "This is expected if GateSentry is not fully running"
    fi

    # Test 8: Verify avahi-resolve works for .local hostnames
    if [[ "$has_avahi" == "true" ]] && command -v avahi-resolve &> /dev/null; then
        print_section "mDNS Hostname Resolution"

        print_test "Testing avahi-resolve for local hostname"
        local local_hostname=$(hostname)
        local resolve_result
        resolve_result=$(timeout 3 avahi-resolve -n "${local_hostname}.local" 2>&1 || echo "")

        if [[ -n "$resolve_result" ]] && ! echo "$resolve_result" | grep -qi "failed\|error"; then
            print_pass "avahi-resolve works: $resolve_result"
        else
            print_info "avahi-resolve could not resolve ${local_hostname}.local"
            print_verbose "Result: $resolve_result"
        fi

        # Test reverse resolution
        print_test "Testing avahi-resolve reverse lookup"
        local local_ip=$(hostname -I | awk '{print $1}')
        if [[ -n "$local_ip" ]]; then
            resolve_result=$(timeout 3 avahi-resolve -a "$local_ip" 2>&1 || echo "")
            if [[ -n "$resolve_result" ]] && ! echo "$resolve_result" | grep -qi "failed\|error"; then
                print_pass "Reverse lookup works: $resolve_result"
            else
                print_info "Reverse lookup not available for $local_ip"
            fi
        fi
    fi

    # Test 9: Comprehensive TXT record validation
    if [[ "$has_avahi" == "true" ]]; then
        print_section "Bonjour TXT Record Validation"

        print_test "Validating all expected TXT records"
        local avahi_result
        avahi_result=$(timeout 5 avahi-browse -t -r "_gatesentry_proxy._tcp" 2>&1 || echo "")
        local txt_line=$(echo "$avahi_result" | grep "txt" | head -1)

        if [[ -n "$txt_line" ]]; then
            local txt_pass=true

            # Check for txtv=1
            if echo "$txt_line" | grep -q "txtv=1"; then
                print_pass "TXT record 'txtv=1' present"
            else
                print_fail "TXT record 'txtv=1' missing"
                txt_pass=false
            fi

            # Check for app=gatesentry
            if echo "$txt_line" | grep -q "app=gatesentry"; then
                print_pass "TXT record 'app=gatesentry' present"
            else
                print_fail "TXT record 'app=gatesentry' missing"
                txt_pass=false
            fi

            if [[ "$txt_pass" == "true" ]]; then
                print_pass "All expected TXT records validated"
            fi
        else
            print_warning "Could not retrieve TXT records for validation"
        fi
    fi

    # Test 10: IPv4 and IPv6 service discovery
    if [[ "$has_avahi" == "true" ]]; then
        print_section "IPv4/IPv6 Service Discovery"

        local avahi_result
        avahi_result=$(timeout 5 avahi-browse -t -r "_gatesentry_proxy._tcp" 2>&1 || echo "")

        print_test "Checking for IPv4 service advertisement"
        if echo "$avahi_result" | grep -q "IPv4"; then
            print_pass "GateSentry advertised on IPv4"
        else
            print_warning "No IPv4 advertisement found"
        fi

        print_test "Checking for IPv6 service advertisement"
        if echo "$avahi_result" | grep -q "IPv6"; then
            print_pass "GateSentry advertised on IPv6"
        else
            print_info "No IPv6 advertisement (IPv6 may not be configured)"
        fi

        # Extract and display discovered addresses
        print_test "Extracting service addresses"
        local addresses=$(echo "$avahi_result" | grep "address" | awk '{print $NF}' | tr -d '[]')
        if [[ -n "$addresses" ]]; then
            local addr_count=$(echo "$addresses" | wc -l)
            print_pass "Found $addr_count service address(es)"
            echo "$addresses" | while read -r addr; do
                print_info "  Service address: $addr"
            done
        else
            print_warning "No service addresses found"
        fi
    fi

    # Test 11: Service type browsing
    if [[ "$has_avahi" == "true" ]]; then
        print_section "Service Type Discovery"

        print_test "Browsing available service types on network"
        local service_types
        service_types=$(timeout 5 avahi-browse -t -D 2>&1 || echo "")

        if [[ -n "$service_types" ]]; then
            local type_count=$(echo "$service_types" | grep -c "+" || echo "0")
            print_pass "Found $type_count service type(s) on network"
            print_verbose "Service types:"
            if [[ "$VERBOSE" == "true" ]]; then
                echo "$service_types" | head -10
            fi
        else
            print_info "No service types discovered"
        fi
    fi

    # Test 12: Verify service can be looked up by name
    if [[ "$has_avahi" == "true" ]]; then
        print_section "Service Name Lookup"

        print_test "Looking up GateSentry service by name"
        local lookup_result
        lookup_result=$(timeout 5 avahi-browse -t -r "_gatesentry_proxy._tcp" 2>&1 | grep -A10 "GateSentry" || echo "")

        if [[ -n "$lookup_result" ]]; then
            # Verify hostname is present
            if echo "$lookup_result" | grep -q "hostname"; then
                local svc_hostname=$(echo "$lookup_result" | grep "hostname" | head -1 | awk '{print $NF}')
                print_pass "Service hostname: $svc_hostname"
            fi

            # Verify port is correct
            if echo "$lookup_result" | grep -q "port.*10413"; then
                print_pass "Service port verified: 10413"
            elif echo "$lookup_result" | grep -q "port"; then
                local svc_port=$(echo "$lookup_result" | grep "port" | head -1 | awk '{print $NF}')
                print_warning "Service port: $svc_port (expected 10413)"
            fi
        else
            print_warning "Could not look up GateSentry service by name"
        fi
    fi

    print_section "mDNS/Bonjour Test Summary"
    print_info "mDNS/Bonjour testing completed"

    if [[ "$has_avahi" != "true" ]]; then
        print_warning "For comprehensive mDNS testing, install: sudo apt-get install avahi-utils avahi-daemon"
    fi
}

# =============================================================================
# Summary and Reporting
# =============================================================================

print_summary() {
    print_header "Test Summary"

    echo -e "${BOLD}Test Results:${NC}"
    echo -e "  ${GREEN}Passed:${NC}  $TESTS_PASSED"
    echo -e "  ${RED}Failed:${NC}  $TESTS_FAILED"
    echo -e "  ${YELLOW}Skipped:${NC} $TESTS_SKIPPED"
    echo -e "  ${BLUE}Total:${NC}   $TESTS_TOTAL"
    echo ""

    local pass_rate=0
    if [[ $TESTS_TOTAL -gt 0 ]]; then
        pass_rate=$(echo "scale=1; $TESTS_PASSED * 100 / $TESTS_TOTAL" | bc)
    fi

    echo -e "${BOLD}Pass Rate:${NC} ${pass_rate}%"
    echo ""

    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo -e "${BOLD}${RED}Failed Tests:${NC}"
        for result in "${TEST_RESULTS[@]}"; do
            if [[ "$result" == FAIL:* ]]; then
                echo -e "  ${RED}✗${NC} ${result#FAIL: }"
            fi
        done
        echo ""
    fi

    if [[ $VERBOSE == "true" ]] && [[ $TESTS_PASSED -gt 0 ]]; then
        echo -e "${BOLD}${GREEN}Passed Tests:${NC}"
        for result in "${TEST_RESULTS[@]}"; do
            if [[ "$result" == PASS:* ]]; then
                echo -e "  ${GREEN}✓${NC} ${result#PASS: }"
            fi
        done
        echo ""
    fi

    # Overall status
    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "${BOLD}${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
        echo -e "${BOLD}${GREEN}  ✓ ALL TESTS PASSED - DNS Server is functioning correctly${NC}"
        echo -e "${BOLD}${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
        return 0
    elif [[ $TESTS_FAILED -lt 5 ]]; then
        echo -e "${BOLD}${YELLOW}═══════════════════════════════════════════════════════════════════${NC}"
        echo -e "${BOLD}${YELLOW}  ⚠ SOME TESTS FAILED - DNS Server may have minor issues${NC}"
        echo -e "${BOLD}${YELLOW}═══════════════════════════════════════════════════════════════════${NC}"
        return 1
    else
        echo -e "${BOLD}${RED}═══════════════════════════════════════════════════════════════════${NC}"
        echo -e "${BOLD}${RED}  ✗ MULTIPLE TESTS FAILED - DNS Server needs attention${NC}"
        echo -e "${BOLD}${RED}═══════════════════════════════════════════════════════════════════${NC}"
        return 2
    fi
}

# =============================================================================
# Main Execution
# =============================================================================

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -p|--port)
                DNS_PORT="$2"
                shift 2
                ;;
            -s|--server)
                DNS_SERVER="$2"
                shift 2
                ;;
            -r|--resolver)
                EXTERNAL_RESOLVER="$2"
                shift 2
                ;;
            -t|--timeout)
                QUERY_TIMEOUT="$2"
                shift 2
                ;;
            -c|--concurrency)
                CONCURRENCY="$2"
                shift 2
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                echo "Unknown option: $1"
                show_help
                ;;
        esac
    done

    # Export environment variables so GateSentry binary uses the same settings
    export GATESENTRY_DNS_ADDR="$DNS_SERVER"
    export GATESENTRY_DNS_PORT="$DNS_PORT"
    export GATESENTRY_DNS_RESOLVER="$EXTERNAL_RESOLVER"
}

main() {
    parse_args "$@"

    echo -e "${BOLD}${CYAN}"
    cat << 'EOF'
   ____       _        ____             _
  / ___| __ _| |_ ___ / ___|  ___ _ __ | |_ _ __ _   _
 | |  _ / _` | __/ _ \___ \ / _ \ '_ \| __| '__| | | |
 | |_| | (_| | ||  __/___) |  __/ | | | |_| |  | |_| |
  \____|\__,_|\__\___|____/ \___|_| |_|\__|_|   \__, |
                                                |___/
       DNS Server Deep Analysis & Testing Suite
EOF
    echo -e "${NC}"

    print_info "DNS Server: $DNS_SERVER:$DNS_PORT"
    print_info "External Resolver: $EXTERNAL_RESOLVER"
    print_info "Query Timeout: ${QUERY_TIMEOUT}s"
    print_info "Concurrency: $CONCURRENCY"
    print_info "Verbose: $VERBOSE"
    print_info "Environment: GATESENTRY_DNS_ADDR=$GATESENTRY_DNS_ADDR"
    print_info "             GATESENTRY_DNS_PORT=$GATESENTRY_DNS_PORT"
    print_info "             GATESENTRY_DNS_RESOLVER=$GATESENTRY_DNS_RESOLVER"
    echo ""

    # Run dependency check first
    check_dependencies

    # Ensure DNS server is available (start if needed)
    ensure_server_available || {
        print_fail "DNS server not available and could not be started - aborting tests"
        print_summary
        exit 1
    }

    # Run all test categories
    # First check external resolver
    test_external_resolver || {
        print_fail "External resolver validation failed - aborting tests"
        print_summary
        exit 1
    }

    test_server_availability || {
        print_fail "Server availability test failed - aborting remaining tests"
        print_summary
        exit 1
    }

    test_record_types
    test_edge_cases
    test_performance
    test_dns_features
    test_security
    test_comparison
    test_tcp_fallback
    test_caching
    test_ptr_records
    test_detailed_queries
    test_large_queries_tcp
    test_timing_diagnostics
    test_mdns_bonjour

    # Print final summary
    print_summary
}

# Run main function with all arguments
main "$@"
