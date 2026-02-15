#!/usr/bin/env python3
"""
GateSentry Test Bed — Echo & Dynamic Response Server

A lightweight HTTP server that replaces httpbin.org for local testing.
Provides deterministic, controllable endpoints for proxy testing.

Endpoints:
  /echo           — Echo request headers back as JSON
  /headers        — Return just the request headers (httpbin compat)
  /sse            — Server-Sent Events stream
  /chunked        — Chunked transfer encoding response
  /drip           — Slow byte-by-byte delivery
  /stream/<n>     — Stream n JSON lines
  /stream-bytes/n — Stream n random bytes in chunks
  /bytes/<n>      — Return n random bytes
  /status/<code>  — Return specific HTTP status code
  /delay/<secs>   — Delay then respond
  /redirect/<n>   — Redirect n times then return 200
  /ws             — WebSocket echo
  /get            — Echo GET request (httpbin compat)
  /post           — Echo POST request (httpbin compat)
  /put            — Echo PUT request (httpbin compat)
  /delete         — Echo DELETE request (httpbin compat)
  /patch          — Echo PATCH request (httpbin compat)
  /head           — Echo HEAD request (httpbin compat)
  /malicious/xss  — Response with XSS payload
  /malicious/sqli — Response with SQL injection patterns
  /malicious/headers — Response with malicious headers

  ADVERSARIAL (protocol-level misbehavior — the hostile internet):
  /adversarial/head-with-body     — HEAD response that illegally includes a body
  /adversarial/lying-content-length — Content-Length says X, body sends Y
  /adversarial/drop-mid-stream    — Close connection mid-response
  /adversarial/mixed-cl-chunked   — Both Content-Length AND chunked (RFC violation)
  /adversarial/gzip-no-header     — Gzip body with no Content-Encoding header
  /adversarial/no-framing         — Raw body, no Content-Length, no chunked
  /adversarial/range-ignored      — Ignores Range header, returns 200 + full body
  /adversarial/ssrf-redirect      — 302 to localhost/internal addresses
  /adversarial/null-in-headers    — Null bytes in header values
  /adversarial/huge-header        — Single header >64KB
  /adversarial/wrong-content-type — Says text/plain, sends JSON
  /adversarial/double-content-length — Two Content-Length headers with different values
  /adversarial/premature-eof-chunked — Chunked stream that ends without terminal chunk
  /adversarial/negative-content-length — Content-Length: -1
  /adversarial/space-in-status    — Non-standard status reason phrase
  /adversarial/trailer-injection  — Chunked with trailer headers

  CVE-INSPIRED (from Squid's 55-vulnerability audit — the internet fights back):
  /adversarial/vary-other         — Vary: Other header that crashed Squid (CVE-2021-28662)
  /adversarial/100-continue       — Unexpected 100 Continue (Squid unfixed 0day)
  /adversarial/chunked-extensions — Huge chunk extensions (CVE-2024-25111 pattern)
  /adversarial/range-overflow     — Range response with integer overflow values (CVE-2021-31808)
  /adversarial/content-range-bad  — Invalid Content-Range in response (CVE-2021-33620)
  /adversarial/xff-overflow       — Response echoing back giant X-Forwarded-For (CVE-2023-50269 pattern)
  /adversarial/cache-poison       — Response with conflicting cache headers + XSS (CVE-2023-5824)
  /adversarial/trace-reflect      — TRACE-like body reflection (CVE-2023-49288 pattern)

Usage:
  python3 echo_server.py --port 9998
"""

import argparse
import asyncio
import gzip
import hashlib
import json
import os
import random
import socket
import sys
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs
import threading
import struct


class EchoHandler(BaseHTTPRequestHandler):
    """Handles all test bed HTTP requests."""

    # Suppress default logging to stderr
    def log_message(self, format, *args):
        pass  # quiet unless VERBOSE

    def _send_json(self, data, status=200):
        """Send a JSON response."""
        body = json.dumps(data, indent=2).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(body)

    def _get_request_info(self):
        """Gather request metadata."""
        parsed = urlparse(self.path)
        return {
            "method": self.command,
            "url": self.path,
            "path": parsed.path,
            "args": parse_qs(parsed.query),
            "headers": dict(self.headers),
            "origin": self.client_address[0],
        }

    def _handle_any_method(self):
        """Route requests to the appropriate handler."""
        parsed = urlparse(self.path)
        path = parsed.path.rstrip("/")
        params = parse_qs(parsed.query)

        # ── /echo or /headers ──
        if path in ("/echo", "/headers"):
            info = self._get_request_info()
            if path == "/headers":
                self._send_json({"headers": info["headers"]})
            else:
                self._send_json(info)
            return

        # ── /get /post /put /delete /patch /head (httpbin compat) ──
        if path in ("/get", "/post", "/put", "/delete", "/patch", "/head"):
            info = self._get_request_info()
            # Read body for POST/PUT/PATCH
            if self.command in ("POST", "PUT", "PATCH"):
                content_length = int(self.headers.get("Content-Length", 0))
                if content_length > 0:
                    info["data"] = self.rfile.read(content_length).decode("utf-8", errors="replace")
            self._send_json(info)
            return

        # ── /health ──
        if path == "/health":
            self._send_json({"status": "ok", "service": "echo-server", "port": self.server.server_address[1]})
            return

        # ── /status/<code> ──
        if path.startswith("/status/"):
            try:
                code = int(path.split("/")[2])
                self.send_response(code)
                self.send_header("Content-Type", "text/plain")
                self.end_headers()
                self.wfile.write(f"Status: {code}\n".encode())
            except (ValueError, IndexError):
                self._send_json({"error": "Invalid status code"}, 400)
            return

        # ── /delay/<seconds> ──
        if path.startswith("/delay/"):
            try:
                delay = float(path.split("/")[2])
                delay = min(delay, 30)  # Cap at 30s
                time.sleep(delay)
                self._send_json({"delayed": delay})
            except (ValueError, IndexError):
                self._send_json({"error": "Invalid delay"}, 400)
            return

        # ── /bytes/<n> — return n random bytes ──
        if path.startswith("/bytes/"):
            try:
                n = int(path.split("/")[2])
                n = min(n, 100 * 1024 * 1024)  # Cap at 100MB
                data = os.urandom(n)
                self.send_response(200)
                self.send_header("Content-Type", "application/octet-stream")
                self.send_header("Content-Length", str(n))
                self.end_headers()
                self.wfile.write(data)
            except (ValueError, IndexError):
                self._send_json({"error": "Invalid byte count"}, 400)
            return

        # ── /stream/<n> — stream n JSON lines ──
        if path.startswith("/stream/"):
            try:
                n = int(path.split("/")[2])
                n = min(n, 1000)
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Transfer-Encoding", "chunked")
                self.end_headers()
                for i in range(n):
                    line = (
                        json.dumps(
                            {
                                "id": i,
                                "timestamp": time.time(),
                                "origin": self.client_address[0],
                            }
                        )
                        + "\n"
                    )
                    chunk = f"{len(line.encode()):x}\r\n{line}\r\n"
                    self.wfile.write(chunk.encode())
                    self.wfile.flush()
                # Final chunk
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
            except (ValueError, IndexError):
                self._send_json({"error": "Invalid stream count"}, 400)
            return

        # ── /stream-bytes/<n>?chunk_size=N — stream n bytes in chunks ──
        if path.startswith("/stream-bytes/"):
            try:
                n = int(path.split("/")[2])
                n = min(n, 100 * 1024 * 1024)
                chunk_size = int(params.get("chunk_size", [10240])[0])
                self.send_response(200)
                self.send_header("Content-Type", "application/octet-stream")
                self.send_header("Transfer-Encoding", "chunked")
                self.end_headers()
                sent = 0
                while sent < n:
                    to_send = min(chunk_size, n - sent)
                    data = os.urandom(to_send)
                    chunk_header = f"{to_send:x}\r\n".encode()
                    self.wfile.write(chunk_header + data + b"\r\n")
                    self.wfile.flush()
                    sent += to_send
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
            except (ValueError, IndexError):
                self._send_json({"error": "Invalid params"}, 400)
            return

        # ── /sse?count=N&delay=S — Server-Sent Events ──
        if path == "/sse":
            count = int(params.get("count", [10])[0])
            delay = float(params.get("delay", [1])[0])
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.send_header("X-Accel-Buffering", "no")  # Tell nginx not to buffer
            self.end_headers()
            try:
                for i in range(count):
                    event = f'id: {i}\nevent: tick\ndata: {{"seq": {i}, "time": {time.time()}}}\n\n'
                    self.wfile.write(event.encode())
                    self.wfile.flush()
                    if i < count - 1:
                        time.sleep(delay)
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        # ── /chunked?chunks=N&delay=S — chunked transfer with delays ──
        if path == "/chunked":
            chunks = int(params.get("chunks", [5])[0])
            delay = float(params.get("delay", [0.5])[0])
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            try:
                for i in range(chunks):
                    data = f"Chunk {i}: timestamp={time.time()}\n"
                    chunk = f"{len(data.encode()):x}\r\n{data}\r\n"
                    self.wfile.write(chunk.encode())
                    self.wfile.flush()
                    if i < chunks - 1:
                        time.sleep(delay)
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        # ── /drip?duration=S&numbytes=N — slow byte delivery ──
        if path == "/drip":
            duration = float(params.get("duration", [3])[0])
            numbytes = int(params.get("numbytes", [10])[0])
            delay_per = duration / max(numbytes, 1)
            code = int(params.get("code", [200])[0])
            initial_delay = float(params.get("delay", [0])[0])

            if initial_delay > 0:
                time.sleep(min(initial_delay, 10))

            self.send_response(code)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Length", str(numbytes))
            self.end_headers()
            try:
                for i in range(numbytes):
                    self.wfile.write(b"*")
                    self.wfile.flush()
                    if i < numbytes - 1:
                        time.sleep(delay_per)
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        # ── /redirect/<n> — redirect n times then 200 ──
        if path.startswith("/redirect/"):
            try:
                n = int(path.split("/")[2])
                if n > 0:
                    self.send_response(302)
                    self.send_header("Location", f"/redirect/{n - 1}")
                    self.end_headers()
                else:
                    self._send_json({"redirected": True})
            except (ValueError, IndexError):
                self._send_json({"error": "Invalid redirect count"}, 400)
            return

        # ── /ws — WebSocket echo ──
        if path == "/ws":
            # Check for WebSocket upgrade
            upgrade = self.headers.get("Upgrade", "").lower()
            if upgrade == "websocket":
                self._handle_websocket()
            else:
                self._send_json({"error": "WebSocket upgrade required"}, 400)
            return

        # ── /adversarial/* — protocol-level misbehavior (the hostile internet) ──
        if path.startswith("/adversarial/"):
            self._handle_adversarial(path, params)
            return

        # ── /malicious/* — attack simulation endpoints ──
        if path.startswith("/malicious/"):
            self._handle_malicious(path)
            return

        # ── Default: 404 ──
        self._send_json({"error": "Not found", "path": path}, 404)

    def _handle_websocket(self):
        """Minimal WebSocket handshake and echo."""
        import hashlib
        import base64

        key = self.headers.get("Sec-WebSocket-Key", "")
        if not key:
            self._send_json({"error": "Missing Sec-WebSocket-Key"}, 400)
            return

        # Compute accept key
        GUID = "258EAFA5-E914-47DA-95CA-5AB5DC11650E"
        accept = hashlib.sha1((key + GUID).encode()).digest()
        accept_b64 = __import__("base64").b64encode(accept).decode()

        # Send upgrade response
        self.send_response(101)
        self.send_header("Upgrade", "websocket")
        self.send_header("Connection", "Upgrade")
        self.send_header("Sec-WebSocket-Accept", accept_b64)
        self.end_headers()

        # Simple echo loop (read one frame, echo it back, close)
        try:
            # Read frame header
            header = self.rfile.read(2)
            if len(header) < 2:
                return

            opcode = header[0] & 0x0F
            masked = (header[1] & 0x80) != 0
            length = header[1] & 0x7F

            if length == 126:
                length = struct.unpack(">H", self.rfile.read(2))[0]
            elif length == 127:
                length = struct.unpack(">Q", self.rfile.read(8))[0]

            mask = self.rfile.read(4) if masked else b"\x00\x00\x00\x00"
            payload = bytearray(self.rfile.read(length))

            if masked:
                for i in range(len(payload)):
                    payload[i] ^= mask[i % 4]

            # Echo back (unmasked)
            response_header = bytes([0x81, min(length, 125)])  # FIN + text opcode
            self.wfile.write(response_header + bytes(payload))
            self.wfile.flush()

            # Send close frame
            self.wfile.write(b"\x88\x00")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError, struct.error):
            pass

    def _handle_adversarial(self, path, params):
        """Protocol-level misbehavior endpoints — simulate the hostile internet.

        These endpoints violate HTTP specs in ways that real-world servers do.
        A robust proxy must handle all of these gracefully without crashing,
        hanging, or passing garbage to the client.
        """
        attack = path.split("/adversarial/")[-1].rstrip("/")

        if attack == "head-with-body":
            # RFC 9110 §9.3.2: HEAD MUST NOT contain a body.
            # But broken servers do this. The proxy must NOT forward the body.
            body = b"THIS BODY SHOULD NOT BE HERE - proxy must strip it"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            # Intentionally write body even for HEAD
            try:
                self.wfile.write(body)
                self.wfile.flush()
            except BrokenPipeError:
                pass  # Client may rightfully close

        elif attack == "lying-content-length":
            # Content-Length says 1000 bytes, but we only send 50.
            # A proxy that trusts Content-Length will hang waiting for the rest.
            actual_body = b"Short body - only 50 bytes, not 1000 as claimed!"
            lie_size = int(params.get("claim", [1000])[0])
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(lie_size))
            self.end_headers()
            try:
                self.wfile.write(actual_body)
                self.wfile.flush()
            except BrokenPipeError:
                pass

        elif attack == "lying-content-length-over":
            # Inverse: Content-Length says 10, but we send 500 bytes.
            # Proxy should truncate or detect the mismatch.
            actual_body = b"X" * 500
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", "10")
            self.end_headers()
            try:
                self.wfile.write(actual_body)
                self.wfile.flush()
            except BrokenPipeError:
                pass

        elif attack == "drop-mid-stream":
            # Start sending a response, then abruptly close the connection.
            # Proxy must not crash and should relay whatever was received
            # (or return a 502/error to the client).
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", "10000")
            self.end_headers()
            try:
                # Send partial data
                self.wfile.write(b"Here is some data..." + b"." * 200)
                self.wfile.flush()
                time.sleep(0.1)
                # Abruptly close the socket
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            except Exception:
                pass

        elif attack == "mixed-cl-chunked":
            # RFC 9112 §6.1: If both Transfer-Encoding and Content-Length are
            # present, TE takes precedence. But this is a security risk —
            # HTTP request smuggling exploits this ambiguity.
            # A proxy MUST use chunked and ignore Content-Length.
            body = b"This uses chunked encoding but also has Content-Length"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", "9999")  # Lie
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            chunk = f"{len(body):x}\r\n".encode() + body + b"\r\n"
            self.wfile.write(chunk)
            self.wfile.write(b"0\r\n\r\n")
            self.wfile.flush()

        elif attack == "gzip-no-header":
            # Server sends gzip-compressed body but does NOT set
            # Content-Encoding: gzip. A naive proxy might pass the garbage
            # through; a smart proxy should not try to decompress it.
            raw_body = b"This text has been gzip compressed but no header says so"
            compressed = gzip.compress(raw_body)
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            # Intentionally NO Content-Encoding header
            self.send_header("Content-Length", str(len(compressed)))
            self.end_headers()
            self.wfile.write(compressed)

        elif attack == "gzip-double":
            # Double-compressed body with only one Content-Encoding: gzip.
            # Proxy should decompress once (or pass through), not loop.
            raw_body = b"Double compressed data - proxy should not infinite loop"
            once = gzip.compress(raw_body)
            twice = gzip.compress(once)
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Encoding", "gzip")
            self.send_header("Content-Length", str(len(twice)))
            self.end_headers()
            self.wfile.write(twice)

        elif attack == "no-framing":
            # No Content-Length, no Transfer-Encoding, no chunked.
            # HTTP/1.1 says server closes connection to signal end of body.
            # Proxy must handle this (read until EOF).
            body = b"This response has no Content-Length and no chunked encoding.\n"
            body += b"The server just closes the connection when done.\n"
            body += b"A" * 500 + b"\nEOF\n"
            # We need to write raw to the socket to avoid BaseHTTPRequestHandler
            # adding its own framing.
            try:
                raw = b"HTTP/1.1 200 OK\r\n"
                raw += b"Content-Type: text/plain\r\n"
                raw += b"Connection: close\r\n"
                raw += b"\r\n"
                raw += body
                self.wfile.write(raw)
                self.wfile.flush()
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            except Exception:
                pass
            # Tell the handler we already sent the response
            return

        elif attack == "range-ignored":
            # Client sends Range: bytes=0-99 but server ignores it,
            # returns 200 + full body. Proxy must pass the 200 through
            # and NOT try to splice/assemble ranges.
            full_body = b"A" * 10000
            range_header = self.headers.get("Range", "none")
            self.send_response(200)  # NOT 206
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Length", str(len(full_body)))
            self.send_header("X-Range-Requested", range_header)
            self.send_header("X-Range-Status", "ignored")
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(full_body)

        elif attack == "ssrf-redirect":
            # 302 redirect to an internal/private address.
            # A security-conscious proxy MUST NOT follow this redirect
            # (or at minimum, block internal IPs).
            target = params.get("target", ["http://127.0.0.1:8080/admin"])[0]
            self.send_response(302)
            self.send_header("Location", target)
            self.send_header("Content-Type", "text/plain")
            body = f"Redirecting to {target}".encode()
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "ssrf-redirect-chain":
            # Multi-hop redirect: external → external → internal
            # Even if proxy allows one redirect, it should catch the final hop.
            step = int(params.get("step", [0])[0])
            if step == 0:
                loc = f"http://{self.headers.get('Host', 'httpbin.org:9998')}/adversarial/ssrf-redirect-chain?step=1"
                self.send_response(302)
                self.send_header("Location", loc)
                self.end_headers()
            elif step == 1:
                # Final hop: redirect to internal
                self.send_response(302)
                self.send_header("Location", "http://169.254.169.254/latest/meta-data/")
                self.end_headers()
            else:
                self._send_json({"error": "unknown step"}, 400)

        elif attack == "null-in-headers":
            # Null bytes in header values. Can cause C-based parsers to
            # truncate strings, leading to header injection.
            try:
                # Write raw to bypass Python's header validation
                raw = b"HTTP/1.1 200 OK\r\n"
                raw += b"Content-Type: text/plain\r\n"
                raw += b"X-Null-Test: before\x00after\r\n"
                raw += b"X-Clean: clean-value\r\n"
                body = b"Null byte in header test"
                raw += f"Content-Length: {len(body)}\r\n".encode()
                raw += b"\r\n"
                raw += body
                self.wfile.write(raw)
                self.wfile.flush()
            except Exception:
                pass
            return

        elif attack == "huge-header":
            # Single header value >64KB. Proxy might have a header size limit.
            # Should either forward it or return 502 — not crash.
            size = int(params.get("size", [65536])[0])
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("X-Huge", "H" * size)
            body = b"Huge header test"
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "wrong-content-type":
            # Says text/plain but body is JSON.
            # Proxy that does content-based filtering should detect this.
            body = json.dumps(
                {
                    "sneaky": True,
                    "message": "I claim to be text/plain but I am JSON",
                    "script": "<script>alert(1)</script>",
                }
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "double-content-length":
            # Two Content-Length headers with different values.
            # RFC 9110 §8.6: "If multiple Content-Length header fields are
            # present with differing values, the server MUST reject."
            # But we're the server misbehaving. Proxy should 502 or pick one.
            try:
                raw = b"HTTP/1.1 200 OK\r\n"
                raw += b"Content-Type: text/plain\r\n"
                raw += b"Content-Length: 10\r\n"
                raw += b"Content-Length: 50\r\n"
                raw += b"\r\n"
                raw += b"Which Content-Length did the proxy believe? This is 50 bytes of body content!!"
                self.wfile.write(raw)
                self.wfile.flush()
            except Exception:
                pass
            return

        elif attack == "premature-eof-chunked":
            # Chunked response that ends without the terminal "0\r\n\r\n".
            # Proxy must detect the incomplete stream and handle gracefully.
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            try:
                # Send a few valid chunks
                for i in range(3):
                    chunk_data = f"Chunk {i}: {'X' * 100}\n".encode()
                    self.wfile.write(f"{len(chunk_data):x}\r\n".encode())
                    self.wfile.write(chunk_data)
                    self.wfile.write(b"\r\n")
                    self.wfile.flush()
                    time.sleep(0.05)
                # Now abruptly close — no terminal chunk
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            except Exception:
                pass
            return

        elif attack == "negative-content-length":
            # Content-Length: -1 — parsers might underflow.
            try:
                raw = b"HTTP/1.1 200 OK\r\n"
                raw += b"Content-Type: text/plain\r\n"
                raw += b"Content-Length: -1\r\n"
                raw += b"\r\n"
                raw += b"Negative content length body\n"
                self.wfile.write(raw)
                self.wfile.flush()
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            except Exception:
                pass
            return

        elif attack == "space-in-status":
            # Non-standard status reason phrase with extra characters.
            # e.g. "HTTP/1.1 200 OK COOL BEANS"
            try:
                raw = b"HTTP/1.1 200 OK COOL BEANS\r\n"
                raw += b"Content-Type: text/plain\r\n"
                body = b"Unusual status line test"
                raw += f"Content-Length: {len(body)}\r\n".encode()
                raw += b"\r\n"
                raw += body
                self.wfile.write(raw)
                self.wfile.flush()
            except Exception:
                pass
            return

        elif attack == "trailer-injection":
            # Chunked encoding with trailer headers.
            # RFC 9110 §6.5.1: Trailers can carry metadata after the body.
            # Some proxies strip or mangle these. Worse: trailer injection
            # can add headers the client trusts (like Content-Length again).
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Transfer-Encoding", "chunked")
            self.send_header("Trailer", "X-Checksum, X-Injected")
            self.end_headers()
            body = b"Body with trailer headers following"
            self.wfile.write(f"{len(body):x}\r\n".encode())
            self.wfile.write(body + b"\r\n")
            self.wfile.write(b"0\r\n")
            # Trailers
            self.wfile.write(b"X-Checksum: abc123\r\n")
            self.wfile.write(b"X-Injected: this-should-not-be-trusted\r\n")
            self.wfile.write(b"\r\n")
            self.wfile.flush()

        elif attack == "slow-body":
            # Send headers immediately, then dribble body out over 10 seconds.
            # Tests proxy timeout handling. Many proxies have a body read
            # timeout that's separate from connect/header timeout.
            duration = int(params.get("duration", [10])[0])
            duration = min(duration, 30)
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            try:
                for i in range(duration):
                    chunk = f"byte {i}\n".encode()
                    self.wfile.write(f"{len(chunk):x}\r\n".encode())
                    self.wfile.write(chunk + b"\r\n")
                    self.wfile.flush()
                    time.sleep(1)
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass

        elif attack == "slow-headers":
            # Slowloris-style: send response status, then headers one per second.
            # Tests proxy's header timeout separate from connect timeout.
            duration = int(params.get("duration", [10])[0])
            duration = min(duration, 20)
            try:
                self.wfile.write(b"HTTP/1.1 200 OK\r\n")
                self.wfile.flush()
                for i in range(duration):
                    self.wfile.write(f"X-Slow-Header-{i}: {'A' * 100}\r\n".encode())
                    self.wfile.flush()
                    time.sleep(1)
                body = b"Slowloris headers test complete"
                self.wfile.write(f"Content-Length: {len(body)}\r\n".encode())
                self.wfile.write(b"Content-Type: text/plain\r\n")
                self.wfile.write(b"\r\n")
                self.wfile.write(body)
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        elif attack == "http09":
            # HTTP/0.9 style response — no headers at all, just raw body.
            # Modern proxies should reject this or convert it.
            try:
                self.wfile.write(b"<html><body>HTTP/0.9 response - no headers!</body></html>\n")
                self.wfile.flush()
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            except Exception:
                pass
            return

        elif attack == "content-encoding-bomb":
            # Small compressed payload that decompresses to enormous size.
            # Tests proxy's decompression limits (zip bomb defense).
            # We'll create ~1MB of zeros that compresses to ~1KB.
            raw_data = b"\x00" * (1024 * 1024)  # 1MB of nulls
            compressed = gzip.compress(raw_data, compresslevel=9)
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Encoding", "gzip")
            self.send_header("Content-Length", str(len(compressed)))
            self.end_headers()
            self.wfile.write(compressed)

        elif attack == "response-splitting":
            # HTTP response splitting attempt via crafted header.
            # Server sends a header that contains \r\n to inject a second response.
            try:
                raw = b"HTTP/1.1 200 OK\r\n"
                raw += b"Content-Type: text/plain\r\n"
                raw += b"X-Split: first\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 44\r\n\r\n<html><body>INJECTED RESPONSE</body></html>"
                raw += b"\r\n"
                body = b"Response splitting test"
                raw = b"HTTP/1.1 200 OK\r\n"
                raw += b"Content-Type: text/plain\r\n"
                raw += b"Set-Cookie: legitimate=true\r\n"
                raw += b"X-Split: innocent\r\nSet-Cookie: evil=stolen\r\n"
                raw += f"Content-Length: {len(body)}\r\n".encode()
                raw += b"\r\n"
                raw += body
                self.wfile.write(raw)
                self.wfile.flush()
            except Exception:
                pass
            return

        elif attack == "keepalive-desync":
            # Tell client Connection: keep-alive, send body, then close.
            # Proxy that reuses the connection will get an error on the next request.
            body = b"I said keep-alive but I lied"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Connection", "keep-alive")
            self.send_header("Keep-Alive", "timeout=300, max=1000")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            self.wfile.flush()
            # Now close the connection despite saying keep-alive
            try:
                time.sleep(0.1)
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            except Exception:
                pass

        # ── CVE-INSPIRED ENDPOINTS (from Squid's 55-vulnerability audit) ──

        elif attack == "vary-other":
            # CVE-2021-28662: Squid assertion crash on "Vary: Other"
            # This is a VALID HTTP header. CDNs send Vary all day.
            # One weird value killed Squid. A proxy MUST pass it through.
            body = b"This response has Vary: Other - a perfectly legal header that crashed Squid"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Vary", "Other")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "100-continue":
            # Squid unfixed 0day: unexpected "HTTP/1.1 100 Continue" crashes proxy.
            # Load balancers and middleware legitimately send 100 before the real response.
            # The proxy MUST handle: 100 Continue + actual response in sequence.
            try:
                # Send an unsolicited 100 Continue first
                self.wfile.write(b"HTTP/1.1 100 Continue\r\n\r\n")
                self.wfile.flush()
                time.sleep(0.1)
                # Then the real response
                body = b"Response after unsolicited 100 Continue"
                self.wfile.write(b"HTTP/1.1 200 OK\r\n")
                self.wfile.write(b"Content-Type: text/plain\r\n")
                self.wfile.write(f"Content-Length: {len(body)}\r\n".encode())
                self.wfile.write(b"\r\n")
                self.wfile.write(body)
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        elif attack == "chunked-extensions":
            # CVE-2024-25111: Squid stack overflow from recursive chunked parsing.
            # Chunk extensions are legal: "1a;ext=value\r\n"
            # But huge/deeply nested extensions can crash parsers.
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            try:
                # Send chunks with large extensions (legal per RFC 9112 §7.1.1)
                for i in range(5):
                    data = f"Chunk {i} with big extensions\n".encode()
                    # Huge chunk extension — 8KB of extension data per chunk
                    ext = f";ext-{i}={'A' * 8192}"
                    chunk_header = f"{len(data):x}{ext}\r\n".encode()
                    self.wfile.write(chunk_header)
                    self.wfile.write(data + b"\r\n")
                    self.wfile.flush()
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass

        elif attack == "range-overflow":
            # CVE-2021-31808: Integer overflow in Range header processing.
            # Server sends back a 206 with Content-Range values near MAX_INT.
            # Proxy that does math on these may integer-overflow.
            body = b"A" * 100
            self.send_response(206)
            self.send_header("Content-Type", "application/octet-stream")
            # Values near 2^63 to trigger integer overflow in parsers
            self.send_header("Content-Range", "bytes 9223372036854775800-9223372036854775806/9223372036854775807")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(body)

        elif attack == "content-range-bad":
            # CVE-2021-33620: Crash in Content-Range response header logic.
            # "Can be expected in HTTP traffic WITHOUT malicious intent."
            # Malformed Content-Range where end > total.
            body = b"B" * 50
            self.send_response(206)
            self.send_header("Content-Type", "application/octet-stream")
            # end > total — clearly invalid but servers send this
            self.send_header("Content-Range", "bytes 0-999/100")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(body)

        elif attack == "xff-overflow":
            # CVE-2023-50269 pattern: Giant X-Forwarded-For header in response.
            # Server echoes back a huge XFF chain. Proxy that parses XFF from
            # responses (for logging, access control) may stack overflow.
            # Also tests: does the proxy blindly copy all response headers?
            xff_chain = ", ".join([f"10.{i % 256}.{(i // 256) % 256}.{i % 128}" for i in range(5000)])
            body = b"Response with huge X-Forwarded-For echo"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("X-Forwarded-For", xff_chain)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "cache-poison":
            # CVE-2023-5824: Cache poisoning by large stored response headers.
            # Response has conflicting cache directives + XSS in headers.
            # If proxy caches this, ALL household members get poisoned.
            body = b"<html><body><h1>Cached XSS</h1><script>document.cookie</script></body></html>"
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            # Conflicting cache directives — which does the proxy obey?
            self.send_header("Cache-Control", "public, max-age=31536000")
            self.send_header("Cache-Control", "no-store, no-cache")
            self.send_header("Pragma", "no-cache")
            self.send_header("Age", "0")
            self.send_header("ETag", '"poisoned-etag-xss"')
            # XSS in a header value — proxy should not trust header content
            self.send_header("X-Debug", '<script>alert("cache-poisoned")</script>')
            # Huge header to push past cache header size limits
            self.send_header("X-Padding", "P" * 32768)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "trace-reflect":
            # CVE-2023-49288 pattern: TRACE-like body reflection.
            # Server echoes the entire request back in the response body,
            # including cookies and auth headers. Proxy should strip sensitive
            # headers from reflected responses, or block TRACE entirely.
            info = self._get_request_info()
            # Include everything — cookies, auth, the lot
            reflection = json.dumps(
                {
                    "reflected_method": info["method"],
                    "reflected_headers": info["headers"],
                    "reflected_url": info["url"],
                    "warning": "This response reflects your full request including cookies and auth tokens",
                },
                indent=2,
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "message/http")  # TRACE content type
            self.send_header("Content-Length", str(len(reflection)))
            self.end_headers()
            self.wfile.write(reflection)

        elif attack == "multi-100-continue":
            # Send MULTIPLE 100 Continue before the real response.
            # Some proxies handle one 100 but not a barrage.
            try:
                for i in range(10):
                    self.wfile.write(b"HTTP/1.1 100 Continue\r\n\r\n")
                    self.wfile.flush()
                    time.sleep(0.05)
                body = b"After 10 unsolicited 100-Continue responses"
                self.wfile.write(b"HTTP/1.1 200 OK\r\n")
                self.wfile.write(b"Content-Type: text/plain\r\n")
                self.wfile.write(f"Content-Length: {len(body)}\r\n".encode())
                self.wfile.write(b"\r\n")
                self.wfile.write(body)
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        elif attack == "header-repeat":
            # Send the same header 1000 times. Tests proxy header table limits.
            # Some proxies allocate a map entry per header — 1000 entries of the
            # same key can cause performance issues or memory bloat.
            body = b"Response with 1000 repeated Set-Cookie headers"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            for i in range(1000):
                self.send_header("Set-Cookie", f"tracker_{i}=value_{i}; Path=/; HttpOnly")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        else:
            available = [
                "head-with-body",
                "lying-content-length",
                "lying-content-length-over",
                "drop-mid-stream",
                "mixed-cl-chunked",
                "gzip-no-header",
                "gzip-double",
                "no-framing",
                "range-ignored",
                "ssrf-redirect",
                "ssrf-redirect-chain",
                "null-in-headers",
                "huge-header",
                "wrong-content-type",
                "double-content-length",
                "premature-eof-chunked",
                "negative-content-length",
                "space-in-status",
                "trailer-injection",
                "slow-body",
                "slow-headers",
                "http09",
                "content-encoding-bomb",
                "response-splitting",
                "keepalive-desync",
                # CVE-inspired:
                "vary-other",
                "100-continue",
                "multi-100-continue",
                "chunked-extensions",
                "range-overflow",
                "content-range-bad",
                "xff-overflow",
                "cache-poison",
                "trace-reflect",
                "header-repeat",
            ]
            self._send_json(
                {
                    "error": f"Unknown adversarial endpoint: {attack}",
                    "available": available,
                },
                404,
            )

    def _handle_malicious(self, path):
        """Simulate malicious server responses for security testing."""
        attack = path.split("/")[-1]

        if attack == "xss":
            # Response body contains XSS payload
            body = '<html><body><script>alert("XSS")</script><h1>XSS Test</h1></body></html>'
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body.encode())

        elif attack == "sqli":
            # Response with SQL-like patterns
            body = json.dumps(
                {
                    "user": "admin' OR '1'='1",
                    "query": "SELECT * FROM users WHERE id=1; DROP TABLE users;--",
                    "data": "Robert'); DROP TABLE Students;--",
                }
            )
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body.encode())

        elif attack == "headers":
            # Malicious response headers
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("X-Injected", "value\r\nSet-Cookie: stolen=true")
            self.send_header("Set-Cookie", "tracking=evil; Domain=.evil.com; Path=/")
            self.send_header("X-Frame-Options", "ALLOW-FROM http://evil.com")
            body = b"Malicious headers test"
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "slow-headers":
            # Slowloris-style: send headers very slowly
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.wfile.write(b"HTTP/1.1 200 OK\r\n")
            self.wfile.flush()
            for i in range(10):
                self.wfile.write(f"X-Slow-{i}: {'A' * 100}\r\n".encode())
                self.wfile.flush()
                time.sleep(1)
            self.wfile.write(b"\r\nDone\n")
            self.wfile.flush()

        elif attack == "big-header":
            # Oversized header response
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("X-Huge", "A" * 65536)
            body = b"Big header test"
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        elif attack == "path-traversal":
            body = json.dumps(
                {
                    "file": "../../../etc/passwd",
                    "content": "root:x:0:0:root:/root:/bin/bash\n",
                }
            )
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body.encode())

        else:
            self._send_json(
                {"error": f"Unknown attack type: {attack}", "available": ["xss", "sqli", "headers", "slow-headers", "big-header", "path-traversal"]},
                404,
            )

    # ── Route all HTTP methods ──
    def do_GET(self):
        self._handle_any_method()

    def do_POST(self):
        self._handle_any_method()

    def do_PUT(self):
        self._handle_any_method()

    def do_DELETE(self):
        self._handle_any_method()

    def do_PATCH(self):
        self._handle_any_method()

    def do_HEAD(self):
        self._handle_any_method()

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header("Allow", "GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "*")
        self.end_headers()


class ThreadedHTTPServer(HTTPServer):
    """Handle requests in threads for concurrency within a single process."""

    allow_reuse_address = True

    def process_request(self, request, client_address):
        thread = threading.Thread(target=self._handle_request_thread, args=(request, client_address))
        thread.daemon = True
        thread.start()

    def _handle_request_thread(self, request, client_address):
        try:
            self.finish_request(request, client_address)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass  # Expected when adversarial tests drop connections
        except Exception:
            self.handle_error(request, client_address)
        finally:
            self.shutdown_request(request)


class PreForkingHTTPServer:
    """
    Pre-forking HTTP server: spawns N worker processes that share the same
    listening socket. Each worker is a ThreadedHTTPServer with its own GIL,
    so requests truly execute in parallel across workers.

    This is the same model as uvicorn --workers N, but preserves raw socket
    access needed by adversarial endpoints (which ASGI frameworks abstract away).
    """

    def __init__(self, bind: str, port: int, handler_class, num_workers: int = 4):
        self.bind = bind
        self.port = port
        self.handler_class = handler_class
        self.num_workers = num_workers
        self.worker_pids: list[int] = []

    def serve_forever(self):
        """Bind the socket once, then fork workers that share it."""
        import signal

        # Create and bind the listening socket in the parent process
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind((self.bind, self.port))
        sock.listen(128)

        print(f"[Echo Server] Pre-fork: {self.num_workers} workers on {self.bind}:{self.port}")
        print(f"[Echo Server] Endpoints: /echo /sse /chunked /drip /stream /ws /malicious/* /adversarial/*")

        # Fork worker processes
        for i in range(self.num_workers):
            pid = os.fork()
            if pid == 0:
                # Child worker process — each has its own GIL
                signal.signal(signal.SIGINT, signal.SIG_DFL)
                # Create the server object WITHOUT binding (bind_and_activate=False)
                server = ThreadedHTTPServer((self.bind, self.port), self.handler_class, bind_and_activate=False)
                server.socket = sock  # Use the parent's pre-bound socket
                print(f"[Echo Server] Worker {i + 1} (PID {os.getpid()}) ready")
                try:
                    server.serve_forever()
                except KeyboardInterrupt:
                    pass
                finally:
                    os._exit(0)
            else:
                self.worker_pids.append(pid)

        # Parent process — wait for signal, then clean up
        def shutdown_handler(signum, frame):
            print(f"\n[Echo Server] Shutting down {len(self.worker_pids)} workers...")
            for pid in self.worker_pids:
                try:
                    os.kill(pid, signal.SIGTERM)
                except OSError:
                    pass
            for pid in self.worker_pids:
                try:
                    os.waitpid(pid, 0)
                except ChildProcessError:
                    pass
            sock.close()
            sys.exit(0)

        signal.signal(signal.SIGINT, shutdown_handler)
        signal.signal(signal.SIGTERM, shutdown_handler)

        # Wait for any child to exit (shouldn't happen normally)
        try:
            while True:
                pid, status = os.wait()
                print(f"[Echo Server] Worker PID {pid} exited with status {status}")
                self.worker_pids.remove(pid)
                if not self.worker_pids:
                    break
        except ChildProcessError:
            pass


def main():
    parser = argparse.ArgumentParser(description="GateSentry Test Bed Echo Server")
    parser.add_argument("--port", type=int, default=9998, help="Port to listen on")
    parser.add_argument("--bind", default="0.0.0.0", help="Address to bind to")
    parser.add_argument("--workers", type=int, default=4, help="Number of pre-forked worker processes (default: 4)")
    parser.add_argument("--single", action="store_true", help="Run single-process threaded server (for debugging)")
    args = parser.parse_args()

    if args.single:
        server = ThreadedHTTPServer((args.bind, args.port), EchoHandler)
        print(f"[Echo Server] Single-process mode on {args.bind}:{args.port}")
        print(f"[Echo Server] Endpoints: /echo /sse /chunked /drip /stream /ws /malicious/* /adversarial/*")
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            print("\n[Echo Server] Shutting down...")
            server.shutdown()
    else:
        server = PreForkingHTTPServer(args.bind, args.port, EchoHandler, num_workers=args.workers)
        server.serve_forever()


if __name__ == "__main__":
    main()
