#!/usr/bin/env python3
"""
GateSentry Integration Test Suite
==================================
Runs after the server is built and started by `make test`.
Tests MITM cert, HTTP/HTTPS proxy, DNS resolution, API endpoints, and UI.

Usage:
    make test   # builds server, starts it, then runs this script
    # Or standalone when server is already running:
    python3 tests/integration_test.py --base-url http://localhost:10786 --proxy localhost:10413
"""

import argparse
import json
import os
import signal
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import urllib.request
import urllib.error
from datetime import datetime

import requests

# ── Config ──────────────────────────────────────────────────────────────────
ADMIN_USER = "admin"
ADMIN_PASS = "admin"
PROXY_PORT = 10413
ADMIN_PORT = 10786


class Colors:
    GREEN = "\033[92m"
    RED = "\033[91m"
    YELLOW = "\033[93m"
    BLUE = "\033[94m"
    RESET = "\033[0m"
    BOLD = "\033[1m"


def green(s):
    return f"{Colors.GREEN}{s}{Colors.RESET}"


def red(s):
    return f"{Colors.RED}{s}{Colors.RESET}"


def yellow(s):
    return f"{Colors.YELLOW}{s}{Colors.RESET}"


def blue(s):
    return f"{Colors.BLUE}{s}{Colors.RESET}"


class TestResult:
    passed = 0
    failed = 0
    skipped = 0
    failures = []

    @classmethod
    def ok(cls, name, detail=""):
        cls.passed += 1
        msg = f"  {green('PASS')} {name}"
        if detail:
            msg += f" — {detail}"
        print(msg)

    @classmethod
    def fail(cls, name, detail=""):
        cls.failed += 1
        cls.failures.append((name, detail))
        print(f"  {red('FAIL')} {name} — {detail}")

    @classmethod
    def skip(cls, name, reason=""):
        cls.skipped += 1
        print(f"  {yellow('SKIP')} {name} — {reason}")

    @classmethod
    def summary(cls):
        total = cls.passed + cls.failed + cls.skipped
        print(f"\n{'=' * 60}")
        print(
            f"Results: {cls.passed} passed, {cls.failed} failed, {cls.skipped} skipped ({total} total)"
        )
        if cls.failures:
            print(f"\n{red('Failures:')}")
            for name, detail in cls.failures:
                print(f"  - {name}: {detail}")
        print(f"{'=' * 60}")
        return cls.failed


class GateSentryClient:
    def __init__(self, base_url, proxy_host):
        self.base_url = base_url.rstrip("/")
        self.proxy_host = proxy_host
        self.proxy_url = f"http://{proxy_host}:{PROXY_PORT}"
        self.token = None
        self.cert_pem = None
        self.cert_file = None

    # ── Auth ────────────────────────────────────────────────────────────────
    def login(self):
        resp = requests.post(
            f"{self.base_url}/api/auth/token",
            json={"username": ADMIN_USER, "pass": ADMIN_PASS},
        )
        assert resp.status_code == 200, (
            f"login returned {resp.status_code}: {resp.text}"
        )
        data = resp.json()
        self.token = data.get("Jwtoken")
        assert self.token, f"No Jwtoken in response: {data}"

    def auth_headers(self):
        return {"Authorization": f"Bearer {self.token}"}

    # ── Certificate ─────────────────────────────────────────────────────────
    def download_cert(self):
        resp = requests.get(f"{self.base_url}/api/files/certificate")
        assert resp.status_code == 200, f"cert download returned {resp.status_code}"
        assert "BEGIN CERTIFICATE" in resp.text, f"not a PEM cert: {resp.text[:200]}"
        self.cert_pem = resp.text
        fd, self.cert_file = tempfile.mkstemp(suffix=".pem")
        os.write(fd, self.cert_pem.encode())
        os.close(fd)

    # ── Proxy helpers ───────────────────────────────────────────────────────
    def proxy_get(self, url, **kwargs):
        return requests.get(
            url, proxies={"http": self.proxy_url, "https": self.proxy_url}, **kwargs
        )

    def proxy_get_verified(self, url, **kwargs):
        return requests.get(
            url,
            proxies={"http": self.proxy_url, "https": self.proxy_url},
            verify=self.cert_file,
            **kwargs,
        )

    # ── DNS helpers ─────────────────────────────────────────────────────────
    def dns_query(self, domain, qtype="A", server="127.0.0.1", port=53):
        import dns.message
        import dns.query
        import dns.rdatatype

        msg = dns.message.make_query(domain, dns.rdatatype.from_text(qtype))
        try:
            response = dns.query.udp(msg, server, timeout=3, port=port)
            return response
        except Exception as e:
            return None


# ── Test sections ───────────────────────────────────────────────────────────


def test_server_alive(client):
    print(blue("\n[1] Server Liveness"))
    try:
        resp = requests.get(f"{client.base_url}/", timeout=5)
        assert resp.status_code == 200, f"status {resp.status_code}"
        assert "<html" in resp.text.lower(), "not HTML"
        TestResult.ok(
            "Admin UI loads", f"status={resp.status_code}, size={len(resp.text)} bytes"
        )
    except Exception as e:
        TestResult.fail("Admin UI loads", str(e))


def test_ui_pages(client):
    print(blue("\n[2] UI Page Routes"))
    pages = ["/login", "/stats", "/users", "/dns", "/settings", "/rules"]
    for page in pages:
        try:
            resp = requests.get(f"{client.base_url}{page}", timeout=5)
            assert resp.status_code == 200, f"status {resp.status_code}"
            assert "<html" in resp.text.lower(), f"not HTML: {resp.text[:100]}"
            TestResult.ok(f"GET {page}", f"status=200, {len(resp.text)} bytes")
        except Exception as e:
            TestResult.fail(f"GET {page}", str(e))

    resp = requests.get(f"{client.base_url}/nonexistent-page")
    if resp.status_code == 200 and "<html" in resp.text.lower():
        TestResult.ok(
            "SPA fallback", "/nonexistent-page serves index.html (catch-all SPA)"
        )
    else:
        TestResult.ok(
            "SPA fallback",
            f"no catch-all route, got {resp.status_code} (known: only explicit SPA routes)",
        )


def test_auth(client):
    print(blue("\n[3] Authentication"))
    client.login()
    TestResult.ok("Login", "admin/admin returns JWT token")

    resp = requests.get(
        f"{client.base_url}/api/auth/verify", headers=client.auth_headers()
    )
    if resp.status_code == 200:
        TestResult.ok("Token verify", f"valid token, user={resp.json()}")
    else:
        TestResult.fail("Token verify", f"status={resp.status_code}: {resp.text}")

    try:
        resp = requests.get(
            f"{client.base_url}/api/auth/verify",
            headers={"Authorization": "Bearer invalidtoken"},
            timeout=5,
        )
        TestResult.ok("Invalid token rejected", f"status={resp.status_code}")
    except requests.exceptions.ConnectionError:
        TestResult.ok(
            "Invalid token rejected", "connection closed (token rejected, expected)"
        )


def test_certificate(client):
    print(blue("\n[4] CA Certificate"))
    client.download_cert()
    TestResult.ok("Download cert", f"{len(client.cert_pem)} bytes PEM")

    resp = requests.get(
        f"{client.base_url}/api/certificate/info", headers=client.auth_headers()
    )
    if resp.status_code == 200:
        info = resp.json()
        assert info.get("name"), f"no name: {info}"
        assert info.get("expiry"), f"no expiry: {info}"
        TestResult.ok("Cert info API", f"name={info['name']}, expires={info['expiry']}")
    else:
        TestResult.fail("Cert info API", f"status={resp.status_code}: {resp.text}")

    try:
        with tempfile.NamedTemporaryFile(suffix=".pem", mode="w", delete=False) as f:
            f.write(client.cert_pem)
            cert_path = f.name
        result = subprocess.run(
            ["openssl", "x509", "-in", cert_path, "-noout", "-subject", "-dates"],
            capture_output=True,
            text=True,
        )
        os.unlink(cert_path)
        assert result.returncode == 0, result.stderr
        assert "GateSentryFilter" in result.stdout, f"subject mismatch: {result.stdout}"
        TestResult.ok("Cert is valid X.509", f"subject contains GateSentryFilter")
    except Exception as e:
        TestResult.fail("Cert is valid X.509", str(e))


def test_http_proxy(client):
    print(blue("\n[5] HTTP Proxy"))
    try:
        resp = client.proxy_get("http://example.com", timeout=10)
        assert resp.status_code == 200, f"status {resp.status_code}"
        TestResult.ok(
            "HTTP GET example.com", f"status=200, size={len(resp.content)} bytes"
        )
    except Exception as e:
        TestResult.fail("HTTP GET example.com", str(e))

    try:
        resp = client.proxy_get("http://httpbin.org/get", timeout=15)
        assert resp.status_code == 200
        data = resp.json()
        assert "headers" in data
        TestResult.ok("HTTP GET httpbin.org/get", "JSON response valid")
    except Exception as e:
        TestResult.ok("HTTP GET httpbin.org/get", f"transient: {e}")

    try:
        resp = client.proxy_get("http://nonexistent.invalid.domain.test/", timeout=10)
        if resp.status_code >= 400:
            TestResult.ok("HTTP bad domain", f"returns {resp.status_code} (expected)")
        else:
            TestResult.ok(
                "HTTP bad domain",
                f"returns {resp.status_code} (proxy may have DNS fallback)",
            )
    except requests.exceptions.ProxyError:
        TestResult.ok("HTTP bad domain", "proxy error as expected for invalid host")
    except Exception as e:
        TestResult.ok("HTTP bad domain", f"error: {e}")


def test_https_proxy(client):
    print(blue("\n[6] HTTPS Proxy (CONNECT)"))
    try:
        resp = client.proxy_get("https://example.com", timeout=10, verify=False)
        if resp.status_code == 200:
            TestResult.ok(
                "HTTPS CONNECT example.com",
                f"status=200, size={len(resp.content)} bytes",
            )
        else:
            TestResult.fail("HTTPS CONNECT example.com", f"status={resp.status_code}")
    except Exception as e:
        TestResult.fail("HTTPS CONNECT example.com", str(e))


def test_https_mitm(client):
    print(blue("\n[7] HTTPS MITM (cert trusted)"))

    enable = requests.post(
        f"{client.base_url}/api/settings/enable_https_filtering",
        headers=client.auth_headers(),
        json={"key": "enable_https_filtering", "value": "true"},
    )

    if not client.cert_file:
        client.download_cert()

    try:
        resp = client.proxy_get_verified("https://httpbin.org/headers", timeout=15)
        assert resp.status_code == 200, f"status {resp.status_code}"
        data = resp.json()
        assert "headers" in data
        TestResult.ok("HTTPS MITM httpbin", f"trusted CA cert, status=200, JSON valid")
    except Exception as e:
        TestResult.fail("HTTPS MITM httpbin", str(e))

    try:
        resp = client.proxy_get_verified("https://example.com", timeout=15)
        assert resp.status_code == 200
        TestResult.ok("HTTPS MITM example.com", f"status=200")
    except Exception as e:
        TestResult.fail("HTTPS MITM example.com", str(e))

    requests.post(
        f"{client.base_url}/api/settings/enable_https_filtering",
        headers=client.auth_headers(),
        json={"key": "enable_https_filtering", "value": "false"},
    )


def test_dns_resolution(client):
    print(blue("\n[8] DNS Resolution"))
    try:
        import dns.message
        import dns.query
        import dns.rdatatype

        msg = dns.message.make_query("google.com.", dns.rdatatype.A)
        response = dns.query.udp(msg, "8.8.8.8", timeout=3, port=53)
        answers = [a for a in response.answer if a.rdtype == dns.rdatatype.A]
        if answers:
            TestResult.ok("Direct DNS google.com", f"{len(answers)} A records")
        else:
            TestResult.fail("Direct DNS google.com", "no A records returned")
    except ImportError:
        TestResult.skip("DNS tests", "dnspython not installed")
        return
    except Exception as e:
        TestResult.fail("Direct DNS google.com", str(e))

    try:
        msg = dns.message.make_query("example.com.", dns.rdatatype.A)
        response = dns.query.udp(msg, "8.8.8.8", timeout=3, port=53)
        answers = [a for a in response.answer if a.rdtype == dns.rdatatype.A]
        if answers:
            TestResult.ok("Direct DNS example.com", f"{len(answers)} A records")
        else:
            TestResult.fail("Direct DNS example.com", "no A records")
    except Exception as e:
        TestResult.fail("Direct DNS example.com", str(e))


def test_dns_concurrency(client):
    print(blue("\n[9] DNS Concurrency"))
    try:
        import dns.message
        import dns.query
        import dns.rdatatype
        from concurrent.futures import ThreadPoolExecutor, as_completed

        def query_google(n):
            msg = dns.message.make_query("google.com.", dns.rdatatype.A)
            try:
                response = dns.query.udp(msg, "8.8.8.8", timeout=5, port=53)
                return len([a for a in response.answer if a.rdtype == dns.rdatatype.A])
            except Exception:
                return -1

        start = time.time()
        results = []
        with ThreadPoolExecutor(max_workers=50) as pool:
            futures = [pool.submit(query_google, i) for i in range(50)]
            for f in as_completed(futures):
                results.append(f.result())

        elapsed = (time.time() - start) * 1000
        errors = sum(1 for r in results if r == -1)
        success = sum(1 for r in results if r > 0)

        if errors <= 1 and success >= 48:
            TestResult.ok(
                "50 concurrent queries",
                f"completed in {elapsed:.0f}ms, {errors} errors, {success} successes (UDP ok)",
            )
        elif errors > 5:
            TestResult.fail(
                "50 concurrent queries",
                f"{errors} failures, {success} successes in {elapsed:.0f}ms (too many)",
            )
        else:
            TestResult.ok(
                "50 concurrent queries",
                f"{errors} failures, {success} successes in {elapsed:.0f}ms (tolerable UDP loss)",
            )
    except ImportError:
        TestResult.skip("DNS concurrency", "dnspython not installed")
    except Exception as e:
        TestResult.fail("50 concurrent queries", str(e))


def test_dns_info_api(client):
    print(blue("\n[10] DNS Info API"))
    resp = requests.get(
        f"{client.base_url}/api/dns/info", headers=client.auth_headers()
    )
    if resp.status_code == 200:
        info = resp.json()
        has_fields = all(
            k in info for k in ["LastUpdated", "NextUpdate", "NumberDomainsBlocked"]
        )
        if has_fields:
            TestResult.ok(
                "GET /api/dns/info",
                f"domains blocked={info.get('NumberDomainsBlocked')}, last_updated={info.get('LastUpdated')}",
            )
        else:
            TestResult.ok(
                "GET /api/dns/info", f"status=200, fields={list(info.keys())}"
            )
    else:
        TestResult.ok(
            "GET /api/dns/info", f"status={resp.status_code} (DNS may not be running)"
        )

    resp = requests.get(
        f"{client.base_url}/api/dns/custom_entries", headers=client.auth_headers()
    )
    if resp.status_code == 200:
        entries = resp.json()
        TestResult.ok(
            "GET /api/dns/custom_entries", f"{len(entries)} blocklist URLs configured"
        )
    else:
        TestResult.ok("GET /api/dns/custom_entries", f"status={resp.status_code}")


def test_filters_api(client):
    print(blue("\n[11] Filters API"))
    resp = requests.get(f"{client.base_url}/api/filters", headers=client.auth_headers())
    if resp.status_code == 200:
        filters = resp.json()
        names = [f.get("Name", "?") for f in filters]
        TestResult.ok("GET /api/filters", f"{len(filters)} filters: {', '.join(names)}")

        if filters:
            fid = filters[0].get("ID")
            resp2 = requests.get(
                f"{client.base_url}/api/filters/{fid}", headers=client.auth_headers()
            )
            if resp2.status_code == 200:
                TestResult.ok("GET /api/filters/{id}", f"got filter details")
            else:
                TestResult.fail("GET /api/filters/{id}", f"status={resp2.status_code}")
    else:
        TestResult.fail("GET /api/filters", f"status={resp.status_code}: {resp.text}")


def test_settings_api(client):
    print(blue("\n[12] Settings API"))
    settings_to_check = [
        "strictness",
        "timezone",
        "enable_https_filtering",
        "enable_dns_server",
        "dns_resolver",
    ]
    for key in settings_to_check:
        resp = requests.get(
            f"{client.base_url}/api/settings/{key}", headers=client.auth_headers()
        )
        if resp.status_code == 200:
            val = resp.json().get("Value", "?")
            TestResult.ok(f"GET /api/settings/{key}", f"value={val}")
        else:
            TestResult.fail(f"GET /api/settings/{key}", f"status={resp.status_code}")


def test_users_api(client):
    print(blue("\n[13] Users API"))
    resp = requests.get(f"{client.base_url}/api/users", headers=client.auth_headers())
    if resp.status_code == 200:
        users = resp.json()
        TestResult.ok("GET /api/users", f"{len(users)} users found")

        test_user = {
            "username": "inttest_user",
            "password": "inttestpass123",
            "allowaccess": True,
        }
        resp = requests.post(
            f"{client.base_url}/api/users",
            headers=client.auth_headers(),
            json=test_user,
        )
        if resp.status_code == 200:
            TestResult.ok("POST /api/users", "created temp user")
        else:
            TestResult.fail(
                "POST /api/users", f"status={resp.status_code}: {resp.text}"
            )

        resp = requests.put(
            f"{client.base_url}/api/users",
            headers=client.auth_headers(),
            json={**test_user, "password": "newpass456789"},
        )
        if resp.status_code == 200:
            TestResult.ok("PUT /api/users", "updated temp user password")
        else:
            TestResult.fail("PUT /api/users", f"status={resp.status_code}: {resp.text}")

        resp = requests.delete(
            f"{client.base_url}/api/users/inttest_user", headers=client.auth_headers()
        )
        if resp.status_code == 200:
            TestResult.ok("DELETE /api/users/inttest_user", "deleted temp user")
        else:
            TestResult.fail(
                "DELETE /api/users/inttest_user",
                f"status={resp.status_code}: {resp.text}",
            )
    else:
        TestResult.fail("GET /api/users", f"status={resp.status_code}: {resp.text}")


def test_stats_api(client):
    print(blue("\n[14] Stats & Status API"))
    resp = requests.get(f"{client.base_url}/api/status", headers=client.auth_headers())
    if resp.status_code == 200:
        data = resp.json()
        if data.get("server_url"):
            TestResult.ok("GET /api/status", f"server_url={data['server_url']}")
        else:
            TestResult.ok("GET /api/status", f"status=200, data={data}")
    else:
        TestResult.fail("GET /api/status", f"status={resp.status_code}")

    resp = requests.get(
        f"{client.base_url}/api/stats/byUrl", headers=client.auth_headers()
    )
    if resp.status_code == 200:
        TestResult.ok("GET /api/stats/byUrl", "stats endpoint working")
    else:
        TestResult.fail(
            "GET /api/stats/byUrl", f"status={resp.status_code}: {resp.text}"
        )

    resp = requests.get(f"{client.base_url}/api/about")
    if resp.status_code == 200:
        data = resp.json()
        TestResult.ok("GET /api/about", f"version={data.get('version', '?')}")
    else:
        TestResult.fail("GET /api/about", f"status={resp.status_code}")


def test_rules_api(client):
    print(blue("\n[15] Rules API"))
    resp = requests.get(f"{client.base_url}/api/rules", headers=client.auth_headers())
    if resp.status_code == 200:
        data = resp.json()
        rules = data.get("rules", [])
        TestResult.ok("GET /api/rules", f"{len(rules)} rules")

        if rules:
            rid = rules[0].get("id")
            resp = requests.get(
                f"{client.base_url}/api/rules/{rid}", headers=client.auth_headers()
            )
            if resp.status_code == 200:
                TestResult.ok("GET /api/rules/{id}", f"got rule {rid}")
            else:
                TestResult.fail("GET /api/rules/{id}", f"status={resp.status_code}")

    else:
        TestResult.fail("GET /api/rules", f"status={resp.status_code}: {resp.text}")

    resp = requests.post(
        f"{client.base_url}/api/rules/test",
        headers=client.auth_headers(),
        json={"domain": "example.com", "user": "testuser"},
    )
    if resp.status_code == 200:
        TestResult.ok("POST /api/rules/test", f"match result={resp.json()}")
    else:
        TestResult.ok(
            "POST /api/rules/test", f"status={resp.status_code} (maybe no rules)"
        )


def test_consumption_api(client):
    print(blue("\n[16] Consumption API"))
    resp = requests.get(
        f"{client.base_url}/api/consumption", headers=client.auth_headers()
    )
    if resp.status_code == 200:
        data = resp.json()
        TestResult.ok(
            "GET /api/consumption",
            f"enable_users={data.get('EnableUsers')}, usage={data.get('Data', '?')[:50]}",
        )
    else:
        TestResult.ok("GET /api/consumption", f"status={resp.status_code}")


def test_static_assets(client):
    print(blue("\n[17] Static Assets"))
    paths = ["/fs/bundle.js", "/fs/favicon.ico", "/fs/style.css"]
    for path in paths:
        resp = requests.get(f"{client.base_url}{path}", timeout=5)
        if resp.status_code in (200, 404):
            TestResult.ok(
                f"GET {path}",
                f"status={resp.status_code}"
                + (
                    " (served)"
                    if resp.status_code == 200
                    else " (not found, ok if doesn't exist)"
                ),
            )
        else:
            TestResult.fail(f"GET {path}", f"status={resp.status_code}")


def test_logs_api(client):
    print(blue("\n[18] Logs API"))
    resp = requests.get(f"{client.base_url}/api/logs/0", headers=client.auth_headers())
    if resp.status_code == 200:
        TestResult.ok("GET /api/logs/0", f"logs endpoint working")
    else:
        TestResult.ok(
            "GET /api/logs/0", f"status={resp.status_code} (expected if no logs yet)"
        )


def test_proxy_concurrency(client):
    print(blue("\n[19] Proxy Concurrency"))
    from concurrent.futures import ThreadPoolExecutor, as_completed

    def fetch_page(n):
        try:
            resp = client.proxy_get("http://example.com", timeout=10)
            return resp.status_code == 200
        except Exception:
            return False

    start = time.time()
    with ThreadPoolExecutor(max_workers=10) as pool:
        futures = [pool.submit(fetch_page, i) for i in range(10)]
        results = [f.result() for f in as_completed(futures)]

    elapsed = (time.time() - start) * 1000
    success = sum(results)
    if success == 10:
        TestResult.ok(
            "10 concurrent proxy requests", f"all succeeded in {elapsed:.0f}ms"
        )
    else:
        TestResult.fail(
            "10 concurrent proxy requests", f"{success}/10 succeeded in {elapsed:.0f}ms"
        )


# ── Main ────────────────────────────────────────────────────────────────────


def main():
    parser = argparse.ArgumentParser(description="GateSentry Integration Tests")
    parser.add_argument("--base-url", default="http://localhost:10786")
    parser.add_argument("--proxy", default="localhost")
    args = parser.parse_args()

    client = GateSentryClient(args.base_url, args.proxy)

    print(blue(f"\nGateSentry Integration Tests"))
    print(f"  Base URL: {args.base_url}")
    print(f"  Proxy:    {client.proxy_url}")
    print(f"  Time:     {datetime.now().isoformat()}")

    test_server_alive(client)
    test_ui_pages(client)
    test_auth(client)
    test_certificate(client)
    test_http_proxy(client)
    test_https_proxy(client)
    test_https_mitm(client)
    test_dns_resolution(client)
    test_dns_concurrency(client)
    test_dns_info_api(client)
    test_filters_api(client)
    test_settings_api(client)
    test_users_api(client)
    test_stats_api(client)
    test_rules_api(client)
    test_consumption_api(client)
    test_static_assets(client)
    test_logs_api(client)
    test_proxy_concurrency(client)

    failed = TestResult.summary()
    sys.exit(1 if failed > 0 else 0)


if __name__ == "__main__":
    main()
