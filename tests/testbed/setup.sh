#!/usr/bin/env bash
###############################################################################
# GateSentry Test Bed — Local Infrastructure Setup
#
# Creates a self-contained test environment on port 9999 that does NOT touch
# the existing nginx default server (vader on :80) or /var/www/html.
#
# What this sets up:
#   1. nginx vhost on port 9999 (HTTP) serving /var/www/gatesentry-testbed/
#   2. nginx vhost on port 9443 (HTTPS) with httpbin.org cert (internal CA)
#   3. Static test files of known sizes (1MB, 10MB, 100MB, 1GB) with checksums
#   4. Python echo server on port 9998 for dynamic tests (SSE, chunked, etc.)
#   5. /etc/hosts entry: httpbin.org → 127.0.0.1
#   6. Verification that everything works
#
# Usage:
#   sudo ./tests/testbed/setup.sh          # full setup
#   sudo ./tests/testbed/setup.sh teardown  # clean removal
#   sudo ./tests/testbed/setup.sh status    # check status
###############################################################################

set -euo pipefail

TESTBED_ROOT="/var/www/gatesentry-testbed"
NGINX_CONF="/etc/nginx/sites-available/gatesentry-testbed"
NGINX_LINK="/etc/nginx/sites-enabled/gatesentry-testbed"
ECHO_SERVER_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${ECHO_SERVER_DIR}/../.." && pwd)"
FIXTURES_DIR="${PROJECT_ROOT}/tests/fixtures"
ECHO_SERVER_PORT=9998
NGINX_PORT=9999
NGINX_SSL_PORT=9443
CHECKSUMS_FILE="${TESTBED_ROOT}/checksums.md5"

# SSL cert for httpbin.org (signed by internal CA "JVJ 28 Inc.")
SSL_CERT="${FIXTURES_DIR}/httpbin.org.crt"
SSL_KEY="${FIXTURES_DIR}/httpbin.org.key"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; }

###############################################################################
# Teardown
###############################################################################
do_teardown() {
    info "Tearing down GateSentry test bed..."

    # Stop echo server
    if pgrep -f "echo_server.py" > /dev/null 2>&1; then
        pkill -f "echo_server.py" && info "Echo server stopped" || true
    fi

    # Remove nginx config
    if [[ -L "$NGINX_LINK" ]]; then
        rm -f "$NGINX_LINK"
        info "Nginx site link removed"
    fi
    if [[ -f "$NGINX_CONF" ]]; then
        rm -f "$NGINX_CONF"
        info "Nginx config removed"
    fi

    # Reload nginx (vader stays untouched)
    if nginx -t 2>/dev/null; then
        systemctl reload nginx 2>/dev/null || nginx -s reload 2>/dev/null || true
        info "Nginx reloaded"
    fi

    # Remove test files (but NOT /var/www/html!)
    if [[ -d "$TESTBED_ROOT" ]]; then
        rm -rf "$TESTBED_ROOT"
        info "Test files removed: ${TESTBED_ROOT}"
    fi

    info "Teardown complete. Vader app untouched."
}

###############################################################################
# Status
###############################################################################
do_status() {
    echo -e "${BOLD}GateSentry Test Bed Status${NC}"
    echo ""

    # nginx vhost
    if [[ -L "$NGINX_LINK" ]]; then
        echo -e "  nginx config:  ${GREEN}enabled${NC} (${NGINX_LINK})"
    else
        echo -e "  nginx config:  ${RED}not found${NC}"
    fi

    # nginx port
    if curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${NGINX_PORT}/" 2>/dev/null | grep -q "200"; then
        echo -e "  nginx :${NGINX_PORT}:   ${GREEN}responding${NC}"
    else
        echo -e "  nginx :${NGINX_PORT}:   ${RED}not responding${NC}"
    fi

    # echo server
    if pgrep -f "echo_server.py" > /dev/null 2>&1; then
        echo -e "  echo server:   ${GREEN}running${NC} on :${ECHO_SERVER_PORT}"
    else
        echo -e "  echo server:   ${RED}not running${NC}"
    fi

    # test files
    if [[ -d "$TESTBED_ROOT" ]]; then
        local file_count
        file_count=$(find "$TESTBED_ROOT" -type f | wc -l)
        local total_size
        total_size=$(du -sh "$TESTBED_ROOT" 2>/dev/null | awk '{print $1}')
        echo -e "  test files:    ${GREEN}${file_count} files${NC} (${total_size})"
    else
        echo -e "  test files:    ${RED}not created${NC}"
    fi

    # vader check
    if curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:80/" 2>/dev/null | grep -qE "^[23]"; then
        echo -e "  vader (:80):   ${GREEN}still running${NC} ✓"
    else
        echo -e "  vader (:80):   ${YELLOW}not responding${NC}"
    fi
}

###############################################################################
# Create nginx config
###############################################################################
create_nginx_config() {
    info "Creating nginx config on port ${NGINX_PORT}..."

    cat > "$NGINX_CONF" << 'NGINX_EOF'
##
# GateSentry Test Bed — isolated on port 9999
# Does NOT interfere with default server (vader) on port 80
##

server {
    listen 9999;
    listen [::]:9999;

    server_name gatesentry-testbed localhost;

    root /var/www/gatesentry-testbed;
    index index.html;

    # ── Access log for test inspection ──
    access_log /var/log/nginx/gatesentry-testbed.access.log;
    error_log  /var/log/nginx/gatesentry-testbed.error.log;

    # ── Disable buffering for streaming tests ──
    proxy_buffering off;

    # ── Static files with proper headers ──
    location / {
        try_files $uri $uri/ =404;

        # Allow Range requests (critical for resume tests)
        add_header Accept-Ranges bytes always;
    }

    # ── Large file downloads with sendfile ──
    location /files/ {
        alias /var/www/gatesentry-testbed/files/;
        add_header Accept-Ranges bytes always;
        add_header X-Testbed "gatesentry" always;

        # Explicit Content-Type for binary files
        types {
            application/octet-stream bin;
        }
    }

    # ── Echo endpoint — returns request headers as response ──
    location /echo {
        proxy_pass http://127.0.0.1:9998;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Don't buffer — pass through immediately
        proxy_buffering off;
        proxy_cache off;
    }

    # ── SSE streaming endpoint ──
    location /sse {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
    }

    # ── Chunked transfer test ──
    location /chunked {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
        proxy_cache off;
        proxy_http_version 1.1;
    }

    # ── Drip / slow-byte endpoint ──
    location /drip {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 120s;
    }

    # ── WebSocket test ──
    location /ws {
        proxy_pass http://127.0.0.1:9998;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400s;
    }

    # ── HEAD method test — tiny known response ──
    location /head-test {
        alias /var/www/gatesentry-testbed/head-test.txt;
        add_header X-Head-Test "true" always;
    }

    # ── Health check ──
    location /health {
        return 200 '{"status":"ok","service":"gatesentry-testbed","port":9999}\n';
        add_header Content-Type application/json;
    }

    # ── EICAR test virus (safe test string) ──
    location /eicar {
        alias /var/www/gatesentry-testbed/eicar/;
        add_header X-Testbed "eicar-test" always;
    }

    # ── Simulated malicious responses ──
    location /malicious/ {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
    }
}

##
# GateSentry Test Bed — HTTPS on port 9443
# Uses httpbin.org certificate signed by internal CA (JVJ 28 Inc.)
# Requires: 127.0.0.1 httpbin.org in /etc/hosts
##

server {
    listen 9443 ssl;
    listen [::]:9443 ssl;

    server_name httpbin.org;

    ssl_certificate     SSL_CERT_PLACEHOLDER;
    ssl_certificate_key SSL_KEY_PLACEHOLDER;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    root /var/www/gatesentry-testbed;
    index index.html;

    access_log /var/log/nginx/gatesentry-testbed-ssl.access.log;
    error_log  /var/log/nginx/gatesentry-testbed-ssl.error.log;

    proxy_buffering off;

    # ── Static files ──
    location / {
        try_files $uri $uri/ =404;
        add_header Accept-Ranges bytes always;
    }

    location /files/ {
        alias /var/www/gatesentry-testbed/files/;
        add_header Accept-Ranges bytes always;
        add_header X-Testbed "gatesentry-ssl" always;
        types {
            application/octet-stream bin;
        }
    }

    # ── Echo / dynamic endpoints via echo server ──
    location /echo    { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /headers { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /get     { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /post    { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /put     { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /delete  { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /status/ { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }
    location /delay/  { proxy_pass http://127.0.0.1:9998; proxy_buffering off; }

    location /sse {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
    }

    location /chunked {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
        proxy_http_version 1.1;
    }

    location /drip {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
        proxy_read_timeout 120s;
    }

    location /stream {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
    }

    location /stream-bytes {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
    }

    location /bytes {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
    }

    location /redirect {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
    }

    location /ws {
        proxy_pass http://127.0.0.1:9998;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400s;
    }

    location /head-test {
        alias /var/www/gatesentry-testbed/head-test.txt;
        add_header X-Head-Test "true" always;
    }

    location /health {
        return 200 '{"status":"ok","service":"gatesentry-testbed-ssl","port":9443}\n';
        add_header Content-Type application/json;
    }

    location /eicar {
        alias /var/www/gatesentry-testbed/eicar/;
    }

    location /malicious/ {
        proxy_pass http://127.0.0.1:9998;
        proxy_buffering off;
    }
}
NGINX_EOF

    # Enable site
    ln -sf "$NGINX_CONF" "$NGINX_LINK"

    # ── Inject SSL cert paths (can't use variables inside heredoc) ──
    sed -i "s|SSL_CERT_PLACEHOLDER|${SSL_CERT}|g" "$NGINX_CONF"
    sed -i "s|SSL_KEY_PLACEHOLDER|${SSL_KEY}|g"   "$NGINX_CONF"

    # ── Verify cert files exist ──
    if [[ ! -f "$SSL_CERT" ]]; then
        error "SSL cert not found: ${SSL_CERT}"
        error "HTTPS server block will fail — place httpbin.org.crt in tests/fixtures/"
    fi
    if [[ ! -f "$SSL_KEY" ]]; then
        error "SSL key not found: ${SSL_KEY}"
        error "HTTPS server block will fail — place httpbin.org.key in tests/fixtures/"
    fi

    # ── /etc/hosts entry for httpbin.org → 127.0.0.1 ──
    if ! grep -q "127.0.0.1.*httpbin.org" /etc/hosts; then
        echo "127.0.0.1  httpbin.org" >> /etc/hosts
        info "Added httpbin.org → 127.0.0.1 in /etc/hosts"
    else
        info "httpbin.org already in /etc/hosts"
    fi

    info "Nginx config created and enabled (HTTP :${NGINX_PORT} + HTTPS :${NGINX_SSL_PORT})"
}

###############################################################################
# Create test files
###############################################################################
create_test_files() {
    info "Creating test files in ${TESTBED_ROOT}..."

    mkdir -p "${TESTBED_ROOT}/files"
    mkdir -p "${TESTBED_ROOT}/eicar"

    # ── Index page ──
    cat > "${TESTBED_ROOT}/index.html" << 'HTML_EOF'
<!DOCTYPE html>
<html>
<head><title>GateSentry Test Bed</title></head>
<body>
<h1>GateSentry Test Bed</h1>
<p>This is a local test server for GateSentry proxy testing.</p>
<h2>Endpoints</h2>
<ul>
  <li><a href="/files/">Static test files</a> (1MB, 10MB, 100MB, 1GB)</li>
  <li><a href="/echo">/echo</a> — Echo request headers</li>
  <li><a href="/sse">/sse</a> — Server-Sent Events stream</li>
  <li><a href="/chunked">/chunked</a> — Chunked transfer encoding</li>
  <li><a href="/drip">/drip</a> — Slow byte delivery</li>
  <li><a href="/ws">/ws</a> — WebSocket echo</li>
  <li><a href="/health">/health</a> — Health check (JSON)</li>
  <li><a href="/head-test">/head-test</a> — HEAD method test</li>
  <li><a href="/eicar/">/eicar/</a> — EICAR test virus files</li>
  <li><a href="/malicious/">/malicious/</a> — Simulated attack payloads</li>
</ul>
</body>
</html>
HTML_EOF

    # ── HEAD test file ──
    echo "HEAD-TEST-OK: This is exactly 69 bytes of content for HEAD testing." > "${TESTBED_ROOT}/head-test.txt"

    # ── Binary test files with deterministic content ──
    info "  Creating 1MB test file..."
    dd if=/dev/urandom of="${TESTBED_ROOT}/files/1MB.bin" bs=1M count=1 2>/dev/null
    info "  Creating 10MB test file..."
    dd if=/dev/urandom of="${TESTBED_ROOT}/files/10MB.bin" bs=1M count=10 2>/dev/null
    info "  Creating 100MB test file..."
    dd if=/dev/urandom of="${TESTBED_ROOT}/files/100MB.bin" bs=1M count=100 2>/dev/null

    # 1GB — only create if user explicitly wants it (takes time + space)
    if [[ "${CREATE_1GB:-0}" == "1" ]]; then
        info "  Creating 1GB test file (this takes a moment)..."
        dd if=/dev/urandom of="${TESTBED_ROOT}/files/1GB.bin" bs=1M count=1024 2>/dev/null
    else
        info "  Skipping 1GB file (set CREATE_1GB=1 to create)"
        # Create a small placeholder
        echo "Run setup with CREATE_1GB=1 to create 1GB test file" > "${TESTBED_ROOT}/files/1GB.bin.readme"
    fi

    # ── Zero-content file (edge case) ──
    touch "${TESTBED_ROOT}/files/0B.bin"

    # ── Text files for content scanning tests ──
    cat > "${TESTBED_ROOT}/files/clean.html" << 'CLEAN_EOF'
<!DOCTYPE html>
<html><head><title>Clean Page</title></head>
<body><h1>This is a clean page with no objectionable content.</h1>
<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p>
</body></html>
CLEAN_EOF

    # ── EICAR test virus string (industry-standard AV test) ──
    # This is NOT a real virus — it's a test string that AV software recognises.
    # See: https://www.eicar.org/download-anti-malware-testfile/
    echo 'X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*' > "${TESTBED_ROOT}/eicar/eicar.com"
    cp "${TESTBED_ROOT}/eicar/eicar.com" "${TESTBED_ROOT}/eicar/eicar.txt"

    # ── Generate MD5 checksums for integrity testing ──
    info "  Computing checksums..."
    (cd "${TESTBED_ROOT}/files" && md5sum *.bin 2>/dev/null > "${CHECKSUMS_FILE}" || true)
    cat "${CHECKSUMS_FILE}" 2>/dev/null || true

    # ── Files index ──
    cat > "${TESTBED_ROOT}/files/index.html" << 'FILES_EOF'
<!DOCTYPE html>
<html><head><title>Test Files</title></head>
<body>
<h1>Test Files</h1>
<ul>
  <li><a href="0B.bin">0B.bin</a> — Empty file</li>
  <li><a href="1MB.bin">1MB.bin</a> — 1 MB random data</li>
  <li><a href="10MB.bin">10MB.bin</a> — 10 MB random data</li>
  <li><a href="100MB.bin">100MB.bin</a> — 100 MB random data</li>
  <li><a href="clean.html">clean.html</a> — Clean HTML page</li>
</ul>
</body>
</html>
FILES_EOF

    # Set permissions
    chown -R www-data:www-data "${TESTBED_ROOT}" 2>/dev/null || true
    chmod -R 755 "${TESTBED_ROOT}"

    info "Test files created"
}

###############################################################################
# Start echo server
###############################################################################
start_echo_server() {
    info "Starting Python echo server on port ${ECHO_SERVER_PORT}..."

    # Kill any existing instance
    pkill -f "echo_server.py" 2>/dev/null || true
    sleep 0.5

    # Start in background
    nohup python3 "${ECHO_SERVER_DIR}/echo_server.py" \
        --port "${ECHO_SERVER_PORT}" \
        > /var/log/gatesentry-echo-server.log 2>&1 &

    local pid=$!
    sleep 1

    if kill -0 "$pid" 2>/dev/null; then
        info "Echo server started (PID: ${pid})"
    else
        error "Echo server failed to start — check /var/log/gatesentry-echo-server.log"
        return 1
    fi
}

###############################################################################
# Verify
###############################################################################
do_verify() {
    info "Verifying test bed..."
    local ok=0
    local fail=0

    # nginx config test
    if nginx -t 2>&1 | grep -q "successful"; then
        info "  nginx config: OK"
        ok=$((ok + 1))
    else
        error "  nginx config: FAILED"
        nginx -t 2>&1
        fail=$((fail + 1))
    fi

    # nginx port
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${NGINX_PORT}/health" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
        info "  nginx HTTP :${NGINX_PORT}: OK (HTTP ${code})"
        ok=$((ok + 1))
    else
        error "  nginx HTTP :${NGINX_PORT}: FAILED (HTTP ${code})"
        fail=$((fail + 1))
    fi

    # nginx HTTPS port
    code=$(curl -s -o /dev/null -w "%{http_code}" --cacert "${FIXTURES_DIR}/JVJCA.crt" \
        "https://httpbin.org:${NGINX_SSL_PORT}/health" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
        info "  nginx HTTPS :${NGINX_SSL_PORT}: OK (HTTP ${code})"
        ok=$((ok + 1))
    else
        error "  nginx HTTPS :${NGINX_SSL_PORT}: FAILED (HTTP ${code})"
        fail=$((fail + 1))
    fi

    # echo server
    code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${ECHO_SERVER_PORT}/echo" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
        info "  echo server :${ECHO_SERVER_PORT}: OK"
        ok=$((ok + 1))
    else
        error "  echo server :${ECHO_SERVER_PORT}: FAILED (HTTP ${code})"
        fail=$((fail + 1))
    fi

    # test files
    if [[ -f "${TESTBED_ROOT}/files/1MB.bin" ]]; then
        local fsize
        fsize=$(stat -c%s "${TESTBED_ROOT}/files/1MB.bin" 2>/dev/null || echo "0")
        if [[ "$fsize" -ge 1000000 ]]; then
            info "  test files: OK (1MB.bin = ${fsize} bytes)"
            ok=$((ok + 1))
        else
            error "  test files: 1MB.bin too small (${fsize})"
            fail=$((fail + 1))
        fi
    else
        error "  test files: NOT FOUND"
        fail=$((fail + 1))
    fi

    # Range request support
    code=$(curl -s -o /dev/null -w "%{http_code}" -H "Range: bytes=0-99" \
        "http://127.0.0.1:${NGINX_PORT}/files/1MB.bin" 2>/dev/null || echo "000")
    if [[ "$code" == "206" ]]; then
        info "  range requests: OK (HTTP 206)"
        ok=$((ok + 1))
    else
        error "  range requests: FAILED (HTTP ${code})"
        fail=$((fail + 1))
    fi

    # vader still alive?
    code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:80/" 2>/dev/null || echo "000")
    if [[ "$code" =~ ^[23] ]]; then
        info "  vader (:80): UNTOUCHED ✓ (HTTP ${code})"
        ok=$((ok + 1))
    else
        warn "  vader (:80): not responding (HTTP ${code}) — was it running?"
    fi

    echo ""
    if [[ "$fail" -eq 0 ]]; then
        info "All ${ok} checks passed! Test bed ready."
        echo ""
        echo "  HTTP static:   http://127.0.0.1:${NGINX_PORT}/files/"
        echo "  HTTPS static:  https://httpbin.org:${NGINX_SSL_PORT}/files/"
        echo "  Echo server:   http://127.0.0.1:${ECHO_SERVER_PORT}/echo"
        echo "  SSE stream:    http://127.0.0.1:${ECHO_SERVER_PORT}/sse"
        echo "  Health (HTTP): http://127.0.0.1:${NGINX_PORT}/health"
        echo "  Health (HTTPS):https://httpbin.org:${NGINX_SSL_PORT}/health"
        echo ""
        echo "  Test via proxy:"
        echo "    curl -x http://127.0.0.1:10413 http://127.0.0.1:${NGINX_PORT}/health"
        echo "    curl -x http://127.0.0.1:10413 https://httpbin.org:${NGINX_SSL_PORT}/health"
    else
        error "${fail} checks failed!"
        return 1
    fi
}

###############################################################################
# Main
###############################################################################
case "${1:-setup}" in
    teardown|remove|clean)
        do_teardown
        ;;
    status)
        do_status
        ;;
    verify)
        do_verify
        ;;
    setup|install)
        echo -e "${BOLD}══════════════════════════════════════════════════${NC}"
        echo -e "${BOLD}  GateSentry Test Bed Setup${NC}"
        echo -e "${BOLD}  nginx HTTP :${NGINX_PORT} + HTTPS :${NGINX_SSL_PORT}${NC}"
        echo -e "${BOLD}  echo server :${ECHO_SERVER_PORT}${NC}"
        echo -e "${BOLD}  Vader (:80 /var/www/html) will NOT be touched${NC}"
        echo -e "${BOLD}══════════════════════════════════════════════════${NC}"
        echo ""

        create_nginx_config
        create_test_files

        # Test nginx config before reload
        if nginx -t 2>&1 | grep -q "successful"; then
            systemctl reload nginx 2>/dev/null || nginx -s reload 2>/dev/null
            info "Nginx reloaded with test bed config"
        else
            error "Nginx config test failed!"
            nginx -t 2>&1
            exit 1
        fi

        start_echo_server
        sleep 1
        do_verify
        ;;
    *)
        echo "Usage: $0 {setup|teardown|status|verify}"
        exit 1
        ;;
esac
