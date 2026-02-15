#!/usr/bin/env bash
###############################################################################
# GateSentry Test Infrastructure — Setup Script
#
# Creates a LOCAL test environment for deterministic proxy/DNS testing:
#   • nginx vhost on port 9999 — static files, Range support, error codes
#   • Python echo server on port 9998 — header echo, SSE, chunked, drip
#   • Test fixture files: 1MB, 10MB, 100MB, 1GB, 2GB + EICAR + payloads
#
# Usage:
#   sudo ./tests/fixtures/setup_test_infra.sh          # full setup
#   sudo ./tests/fixtures/setup_test_infra.sh teardown  # remove everything
#   sudo ./tests/fixtures/setup_test_infra.sh status    # check status
#
# Requires: nginx, python3, fallocate/dd, sudo
###############################################################################

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="/var/www/gatesentry-test"
NGINX_CONF="/etc/nginx/sites-available/gatesentry-test"
NGINX_LINK="/etc/nginx/sites-enabled/gatesentry-test"
ECHO_SERVER="${SCRIPT_DIR}/echo_server.py"
ECHO_PID_FILE="/tmp/gatesentry-echo-server.pid"
NGINX_PORT=9999
ECHO_PORT=9998

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; }

###############################################################################
# Generate test fixture files
###############################################################################
generate_fixtures() {
    info "Creating test fixture directory: ${TEST_ROOT}"
    mkdir -p "${TEST_ROOT}/files"
    mkdir -p "${TEST_ROOT}/payloads"

    # ── Static binary test files (deterministic content for checksum testing) ──
    info "Generating test files..."

    # 1KB
    if [[ ! -f "${TEST_ROOT}/files/1kb.bin" ]]; then
        dd if=/dev/urandom of="${TEST_ROOT}/files/1kb.bin" bs=1024 count=1 2>/dev/null
        info "  Created 1kb.bin"
    fi

    # 1MB
    if [[ ! -f "${TEST_ROOT}/files/1mb.bin" ]]; then
        dd if=/dev/urandom of="${TEST_ROOT}/files/1mb.bin" bs=1M count=1 2>/dev/null
        info "  Created 1mb.bin"
    fi

    # 10MB (MaxContentScanSize boundary)
    if [[ ! -f "${TEST_ROOT}/files/10mb.bin" ]]; then
        dd if=/dev/urandom of="${TEST_ROOT}/files/10mb.bin" bs=1M count=10 2>/dev/null
        info "  Created 10mb.bin"
    fi

    # 100MB
    if [[ ! -f "${TEST_ROOT}/files/100mb.bin" ]]; then
        fallocate -l 100M "${TEST_ROOT}/files/100mb.bin" 2>/dev/null || \
            dd if=/dev/urandom of="${TEST_ROOT}/files/100mb.bin" bs=1M count=100 2>/dev/null
        info "  Created 100mb.bin"
    fi

    # 1GB
    if [[ ! -f "${TEST_ROOT}/files/1gb.bin" ]]; then
        fallocate -l 1G "${TEST_ROOT}/files/1gb.bin" 2>/dev/null || \
            dd if=/dev/zero of="${TEST_ROOT}/files/1gb.bin" bs=1M count=1024 2>/dev/null
        info "  Created 1gb.bin"
    fi

    # 2GB (tests >2GB boundary — int32 overflow risk)
    if [[ ! -f "${TEST_ROOT}/files/2gb.bin" ]]; then
        fallocate -l 2G "${TEST_ROOT}/files/2gb.bin" 2>/dev/null || \
            dd if=/dev/zero of="${TEST_ROOT}/files/2gb.bin" bs=1M count=2048 2>/dev/null
        info "  Created 2gb.bin"
    fi

    # ── Text files for content scanning tests ──
    cat > "${TEST_ROOT}/files/clean.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><head><title>GateSentry Test — Clean Page</title></head>
<body><h1>This is a clean test page</h1><p>No blocked content here.</p></body>
</html>
HTMLEOF

    cat > "${TEST_ROOT}/files/hello.txt" << 'EOF'
Hello from GateSentry local test server.
This is a simple text response for proxy testing.
EOF

    # ── EICAR test virus signature ──
    # Standard EICAR anti-malware test file (harmless but should trigger scanners)
    # See: https://www.eicar.org/download-anti-malware-testfile/
    echo 'X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*' \
        > "${TEST_ROOT}/payloads/eicar.com"
    info "  Created EICAR test virus file"

    # EICAR embedded in HTML (tests content scanning depth)
    cat > "${TEST_ROOT}/payloads/eicar-in-html.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><head><title>Hidden Malware</title></head>
<body><p>Normal looking page</p>
<!-- X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H* -->
<script>var x="X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*";</script>
</body></html>
HTMLEOF
    info "  Created EICAR-in-HTML payload"

    # ── XSS / injection payloads ──
    cat > "${TEST_ROOT}/payloads/xss-basic.html" << 'HTMLEOF'
<!DOCTYPE html>
<html><body>
<script>alert('XSS')</script>
<img src=x onerror="alert('XSS')">
<svg onload="alert('XSS')">
</body></html>
HTMLEOF

    # SQL injection strings (for URL parameter testing)
    cat > "${TEST_ROOT}/payloads/sqli-strings.txt" << 'EOF'
' OR '1'='1
'; DROP TABLE users; --
1 UNION SELECT * FROM passwords
" OR ""="
admin'--
1; EXEC xp_cmdshell('whoami')
EOF

    # Directory traversal payloads
    cat > "${TEST_ROOT}/payloads/traversal-paths.txt" << 'EOF'
../../../etc/passwd
..%2F..%2F..%2Fetc%2Fpasswd
....//....//....//etc/passwd
%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd
/etc/passwd
..\\..\\..\\windows\\system32\\config\\sam
EOF

    # ── Generate MD5 checksums for integrity testing ──
    info "Generating checksums..."
    (cd "${TEST_ROOT}/files" && md5sum *.bin > checksums.md5 2>/dev/null || true)
    info "  Checksums saved to ${TEST_ROOT}/files/checksums.md5"

    # Set permissions
    chmod -R 755 "${TEST_ROOT}"
    info "Fixture files ready in ${TEST_ROOT}"
}

###############################################################################
# Create nginx test vhost
###############################################################################
setup_nginx() {
    info "Creating nginx test vhost on port ${NGINX_PORT}..."

    cat > "${NGINX_CONF}" << 'NGINXEOF'
# GateSentry Test Server — deterministic local test endpoints
# Port 9999 — static file serving with full Range support
#
# Endpoints:
#   /files/*          — static test files (1kb, 1mb, 10mb, 100mb, 1gb, 2gb)
#   /payloads/*       — security test payloads (EICAR, XSS, etc.)
#   /status/{code}    — return specific HTTP status codes
#   /redirect/{n}     — redirect chain of n hops
#   /slow/{seconds}   — delayed response
#   /headers          — proxy to echo server (request header inspection)
#   /sse              — proxy to echo server (Server-Sent Events)
#   /chunked/{n}      — proxy to echo server (chunked n lines)
#   /drip             — proxy to echo server (timed byte delivery)
#   /large-headers    — response with many large headers

server {
    listen 9999 default_server;
    server_name gatesentry-test localhost;

    root /var/www/gatesentry-test;

    # Enable sendfile for efficient large file serving
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;

    # Disable access log for test server (reduce noise)
    access_log off;

    # ── Static files with full Range support (nginx default) ──
    location /files/ {
        alias /var/www/gatesentry-test/files/;
        add_header X-Test-Server "gatesentry-local";
        add_header Accept-Ranges bytes;

        # Allow any method
        if ($request_method = 'OPTIONS') {
            add_header Allow "GET, HEAD, OPTIONS, PUT, POST, DELETE, PATCH";
            return 200;
        }
    }

    # ── Security payloads ──
    location /payloads/ {
        alias /var/www/gatesentry-test/payloads/;
        add_header X-Test-Server "gatesentry-local";
        # Serve .com files as application/octet-stream
        types {
            application/octet-stream com;
        }
    }

    # ── Return specific HTTP status codes ──
    location ~ ^/status/(\d+)$ {
        add_header X-Test-Server "gatesentry-local";
        add_header Content-Type "text/plain";
        return $1 "Status: $1\n";
    }

    # ── Redirect chain ──
    location ~ ^/redirect/(\d+)$ {
        set $count $1;
        # Redirect to /redirect/(n-1) until 0
        if ($count = "0") {
            add_header X-Test-Server "gatesentry-local";
            return 200 "Final destination after redirect chain\n";
        }
        # nginx can't do arithmetic, so we handle a few levels
        return 302 /redirect-hop?from=$count;
    }
    location /redirect-hop {
        # Simple redirect that eventually terminates
        return 302 /files/hello.txt;
    }

    # ── Slow response — delay before sending body ──
    # We use proxy_pass to echo server which has proper delay support
    location ~ ^/slow/(\d+)$ {
        proxy_pass http://127.0.0.1:9998;
        proxy_set_header X-Delay-Seconds $1;
        proxy_read_timeout 120s;
    }

    # ── Response with large headers ──
    location /large-headers {
        add_header X-Test-Server "gatesentry-local";
        add_header X-Large-Header-1 "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
        add_header X-Large-Header-2 "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB";
        add_header X-Large-Header-3 "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC";
        return 200 "Response with large headers\n";
    }

    # ── Dynamic endpoints — proxy to Python echo server ──
    location /echo-headers {
        proxy_pass http://127.0.0.1:9998/echo-headers;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Original-URI $request_uri;
    }

    location /sse {
        proxy_pass http://127.0.0.1:9998/sse;
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
    }

    location ~ ^/chunked/(\d+)$ {
        proxy_pass http://127.0.0.1:9998/chunked/$1;
        proxy_buffering off;
        proxy_http_version 1.1;
    }

    location /drip {
        proxy_pass http://127.0.0.1:9998/drip;
        proxy_buffering off;
        proxy_read_timeout 120s;
    }

    location /websocket-echo {
        proxy_pass http://127.0.0.1:9998/websocket-echo;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # ── Catch-all ──
    location / {
        add_header X-Test-Server "gatesentry-local";
        add_header Content-Type "text/plain";
        return 200 "GateSentry Test Server\nPort: 9999\nUse /files/, /payloads/, /status/{code}, /echo-headers, /sse, /chunked/{n}, /drip\n";
    }
}
NGINXEOF

    # Enable the site
    ln -sf "${NGINX_CONF}" "${NGINX_LINK}" 2>/dev/null || true

    # Test and reload nginx
    if nginx -t 2>&1; then
        systemctl reload nginx
        info "nginx test vhost enabled on port ${NGINX_PORT}"
    else
        error "nginx config test FAILED — check ${NGINX_CONF}"
        exit 1
    fi
}

###############################################################################
# Create Python echo server
###############################################################################
create_echo_server() {
    info "Creating Python echo server script..."

    cat > "${ECHO_SERVER}" << 'PYEOF'
#!/usr/bin/env python3
"""
GateSentry Test Infrastructure — Dynamic Echo Server

Provides endpoints that nginx can't handle natively:
  /echo-headers   — returns all request headers as JSON
  /sse            — Server-Sent Events stream (5 events, 1/sec)
  /chunked/{n}    — chunked response with n lines
  /drip           — timed byte delivery (params: duration, numbytes)
  /slow/{seconds} — delayed response
  /websocket-echo — WebSocket echo (for future use)

Runs on port 9998 behind nginx reverse proxy on port 9999.
"""

import http.server
import json
import time
import sys
import os
import re
import signal
import threading
from urllib.parse import urlparse, parse_qs

PORT = 9998

class EchoHandler(http.server.BaseHTTPRequestHandler):
    """Handles dynamic test endpoints."""

    def log_message(self, format, *args):
        """Suppress default logging."""
        pass

    def do_GET(self):
        parsed = urlparse(self.path)
        path = parsed.path
        params = parse_qs(parsed.query)

        if path == '/echo-headers':
            self._echo_headers()
        elif path == '/sse':
            self._sse_stream(params)
        elif path.startswith('/chunked/'):
            n = int(path.split('/')[-1]) if path.split('/')[-1].isdigit() else 5
            self._chunked_response(n)
        elif path == '/drip':
            self._drip_response(params)
        elif path.startswith('/slow/'):
            seconds = int(path.split('/')[-1]) if path.split('/')[-1].isdigit() else 5
            self._slow_response(seconds)
        else:
            self.send_response(404)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()
            self.wfile.write(b'Unknown endpoint\n')

    def do_HEAD(self):
        """Handle HEAD — same as GET but no body."""
        parsed = urlparse(self.path)
        if parsed.path == '/echo-headers':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
        else:
            self.send_response(200)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()

    def do_POST(self):
        self.do_GET()

    def do_PUT(self):
        self.do_GET()

    def do_DELETE(self):
        self.do_GET()

    def do_PATCH(self):
        self.do_GET()

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header('Allow', 'GET, HEAD, POST, PUT, DELETE, PATCH, OPTIONS')
        self.send_header('Content-Length', '0')
        self.end_headers()

    def _echo_headers(self):
        """Return all request headers as JSON."""
        headers = {}
        for key, val in self.headers.items():
            if key in headers:
                if isinstance(headers[key], list):
                    headers[key].append(val)
                else:
                    headers[key] = [headers[key], val]
            else:
                headers[key] = val

        body = json.dumps({
            'method': self.command,
            'path': self.path,
            'headers': headers,
            'client': self.client_address[0],
        }, indent=2).encode('utf-8')

        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.send_header('X-Test-Server', 'gatesentry-echo')
        self.end_headers()
        self.wfile.write(body)

    def _sse_stream(self, params):
        """Server-Sent Events — sends n events with delay."""
        count = int(params.get('count', ['5'])[0])
        delay = float(params.get('delay', ['1.0'])[0])

        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Cache-Control', 'no-cache')
        self.send_header('Connection', 'keep-alive')
        self.send_header('X-Accel-Buffering', 'no')  # Tell nginx not to buffer
        self.end_headers()

        try:
            for i in range(count):
                event = f"id: {i}\nevent: message\ndata: {{\"seq\": {i}, \"time\": \"{time.time()}\"}}\n\n"
                self.wfile.write(event.encode('utf-8'))
                self.wfile.flush()
                if i < count - 1:
                    time.sleep(delay)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _chunked_response(self, n):
        """Send n lines as separate chunks."""
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain')
        self.send_header('Transfer-Encoding', 'chunked')
        self.send_header('X-Test-Server', 'gatesentry-echo')
        self.end_headers()

        try:
            for i in range(n):
                line = f"chunk {i}: timestamp={time.time()}\n"
                chunk = f"{len(line):x}\r\n{line}\r\n"
                self.wfile.write(chunk.encode('utf-8'))
                self.wfile.flush()
                time.sleep(0.1)  # Small delay between chunks

            # Final chunk
            self.wfile.write(b"0\r\n\r\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _drip_response(self, params):
        """Send bytes one at a time with delays — simulates slow streaming."""
        duration = float(params.get('duration', ['3'])[0])
        numbytes = int(params.get('numbytes', ['10'])[0])
        delay = duration / max(numbytes, 1)

        self.send_response(200)
        self.send_header('Content-Type', 'application/octet-stream')
        self.send_header('Content-Length', str(numbytes))
        self.send_header('X-Test-Server', 'gatesentry-echo')
        self.end_headers()

        try:
            for i in range(numbytes):
                self.wfile.write(b'*')
                self.wfile.flush()
                if i < numbytes - 1:
                    time.sleep(delay)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _slow_response(self, seconds):
        """Wait then send response — tests timeout handling."""
        time.sleep(seconds)
        body = f"Response after {seconds} second delay\n".encode('utf-8')
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain')
        self.send_header('Content-Length', str(len(body)))
        self.send_header('X-Test-Server', 'gatesentry-echo')
        self.end_headers()
        self.wfile.write(body)


class ThreadedHTTPServer(http.server.HTTPServer):
    """Handle requests in separate threads."""
    allow_reuse_address = True

    def process_request(self, request, client_address):
        thread = threading.Thread(target=self.process_request_thread,
                                  args=(request, client_address))
        thread.daemon = True
        thread.start()

    def process_request_thread(self, request, client_address):
        try:
            self.finish_request(request, client_address)
        except Exception:
            self.handle_error(request, client_address)
        finally:
            self.shutdown_request(request)


def main():
    server = ThreadedHTTPServer(('127.0.0.1', PORT), EchoHandler)
    print(f"Echo server running on http://127.0.0.1:{PORT}")

    # Write PID file
    pid_file = '/tmp/gatesentry-echo-server.pid'
    with open(pid_file, 'w') as f:
        f.write(str(os.getpid()))

    def shutdown(signum, frame):
        print("\nShutting down echo server...")
        server.shutdown()
        try:
            os.unlink(pid_file)
        except FileNotFoundError:
            pass
        sys.exit(0)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        shutdown(None, None)


if __name__ == '__main__':
    main()
PYEOF

    chmod +x "${ECHO_SERVER}"
    info "Echo server script created at ${ECHO_SERVER}"
}

###############################################################################
# Start/stop echo server
###############################################################################
start_echo_server() {
    # Kill existing if running
    stop_echo_server 2>/dev/null || true

    info "Starting Python echo server on port ${ECHO_PORT}..."
    nohup python3 "${ECHO_SERVER}" > /tmp/gatesentry-echo-server.log 2>&1 &
    sleep 1

    if curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${ECHO_PORT}/echo-headers" 2>/dev/null | grep -q "200"; then
        info "Echo server running (PID: $(cat ${ECHO_PID_FILE} 2>/dev/null))"
    else
        error "Echo server failed to start — check /tmp/gatesentry-echo-server.log"
        exit 1
    fi
}

stop_echo_server() {
    if [[ -f "${ECHO_PID_FILE}" ]]; then
        local pid
        pid=$(cat "${ECHO_PID_FILE}" 2>/dev/null)
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null
            info "Stopped echo server (PID: ${pid})"
        fi
        rm -f "${ECHO_PID_FILE}"
    fi
    # Also kill by port just in case
    fuser -k ${ECHO_PORT}/tcp 2>/dev/null || true
}

###############################################################################
# Status check
###############################################################################
check_status() {
    echo -e "${BOLD}GateSentry Test Infrastructure Status${NC}"
    echo ""

    # nginx test vhost
    if curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${NGINX_PORT}/" 2>/dev/null | grep -q "200"; then
        echo -e "  ${GREEN}✓${NC} nginx test server on port ${NGINX_PORT}"
    else
        echo -e "  ${RED}✗${NC} nginx test server on port ${NGINX_PORT}"
    fi

    # Echo server
    if curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${ECHO_PORT}/echo-headers" 2>/dev/null | grep -q "200"; then
        echo -e "  ${GREEN}✓${NC} Python echo server on port ${ECHO_PORT} (PID: $(cat ${ECHO_PID_FILE} 2>/dev/null || echo '?'))"
    else
        echo -e "  ${RED}✗${NC} Python echo server on port ${ECHO_PORT}"
    fi

    # Test files
    echo ""
    if [[ -d "${TEST_ROOT}/files" ]]; then
        echo -e "  ${GREEN}✓${NC} Test fixtures in ${TEST_ROOT}/files/:"
        ls -lhS "${TEST_ROOT}/files/"*.bin 2>/dev/null | awk '{print "       " $5 "  " $NF}'
    else
        echo -e "  ${RED}✗${NC} No test fixtures found"
    fi

    # GateSentry
    echo ""
    for port in 10053 10413 8080; do
        if ss -tlnp 2>/dev/null | grep -q ":${port}"; then
            echo -e "  ${GREEN}✓${NC} GateSentry port ${port}"
        else
            echo -e "  ${RED}✗${NC} GateSentry port ${port}"
        fi
    done
}

###############################################################################
# Teardown
###############################################################################
teardown() {
    warn "Tearing down test infrastructure..."
    stop_echo_server
    rm -f "${NGINX_LINK}"
    rm -f "${NGINX_CONF}"
    nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null
    info "nginx test vhost removed"

    read -p "Remove test fixture files in ${TEST_ROOT}? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf "${TEST_ROOT}"
        info "Test fixtures removed"
    else
        info "Test fixtures kept in ${TEST_ROOT}"
    fi
}

###############################################################################
# Main
###############################################################################
case "${1:-setup}" in
    setup)
        generate_fixtures
        create_echo_server
        setup_nginx
        start_echo_server
        echo ""
        check_status
        echo ""
        info "Test infrastructure ready!"
        info "Run tests with: ./tests/proxy_benchmark_suite.sh"
        ;;
    teardown)
        teardown
        ;;
    status)
        check_status
        ;;
    start-echo)
        start_echo_server
        ;;
    stop-echo)
        stop_echo_server
        ;;
    *)
        echo "Usage: $0 {setup|teardown|status|start-echo|stop-echo}"
        exit 1
        ;;
esac
