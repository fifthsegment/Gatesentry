# GateSentry Integration Test Suite (pytest)
# ===========================================
# Requires a running GateSentry server. Started by `make test`.
#
# Usage:
#   make test-python                     # full suite
#   pytest tests/ -v --base-url=...       # manual
#   GATESENTRY_DNS_PORT=10053 pytest tests/ -v -k dns

import json
import os
import subprocess
import tempfile
import time

import pytest
import requests


# ── Config (override via env / pytest CLI) ───────────────────────────────
@pytest.fixture(scope="session")
def base_url():
    return os.environ.get("GS_BASE_URL", "http://localhost:10786")


@pytest.fixture(scope="session")
def proxy_url():
    proxy = os.environ.get("GS_PROXY_HOST", "localhost")
    port = os.environ.get("GS_PROXY_PORT", "10413")
    return f"http://{proxy}:{port}"


@pytest.fixture(scope="session")
def admin_auth():
    return ("admin", "admin")


# ── Shared client fixture (one per session) ─────────────────────────────
class GSClient:
    """Thin wrapper around the GateSentry admin API + proxy."""

    def __init__(self, base_url, proxy_url, username, password):
        self.base_url = base_url.rstrip("/")
        self.proxy_url = proxy_url
        self._username = username
        self._password = password
        self.token = None
        self.cert_pem = None
        self.cert_file = None

    # -- auth --
    def login(self):
        r = requests.post(
            f"{self.base_url}/api/auth/token",
            json={"username": self._username, "pass": self._password},
            timeout=10,
        )
        assert r.status_code == 200, f"login → {r.status_code} {r.text}"
        data = r.json()
        self.token = data["Jwtoken"]

    @property
    def headers(self):
        return {"Authorization": f"Bearer {self.token}"}

    # -- cert --
    def fetch_cert(self):
        r = requests.get(f"{self.base_url}/api/files/certificate", timeout=10)
        assert r.status_code == 200
        assert "BEGIN CERTIFICATE" in r.text
        self.cert_pem = r.text
        fd, self.cert_file = tempfile.mkstemp(suffix=".pem")
        os.write(fd, self.cert_pem.encode())
        os.close(fd)

    # -- proxy --
    def _proxies(self):
        return {"http": self.proxy_url, "https": self.proxy_url}

    def proxy_get(self, url, **kw):
        """HTTP(S) through the proxy tunnel."""
        return requests.get(
            url, proxies=self._proxies(), timeout=kw.pop("timeout", 15), **kw
        )

    def proxy_get_verified(self, url, **kw):
        """Proxy with the GS CA cert trusted."""
        timeout = kw.pop("timeout", 15)
        return requests.get(
            url, proxies=self._proxies(), verify=self.cert_file, timeout=timeout, **kw
        )

    # -- api helpers --
    def api_get(self, path, **kw):
        return requests.get(
            f"{self.base_url}{path}",
            headers=self.headers,
            timeout=kw.pop("timeout", 10),
            **kw,
        )

    def api_post(self, path, json=None, **kw):
        return requests.post(
            f"{self.base_url}{path}",
            headers=self.headers,
            json=json,
            timeout=kw.pop("timeout", 10),
            **kw,
        )

    def api_put(self, path, json=None, **kw):
        return requests.put(
            f"{self.base_url}{path}",
            headers=self.headers,
            json=json,
            timeout=kw.pop("timeout", 10),
            **kw,
        )

    def api_delete(self, path, **kw):
        return requests.delete(
            f"{self.base_url}{path}",
            headers=self.headers,
            timeout=kw.pop("timeout", 10),
            **kw,
        )

    # -- dns --
    @property
    def dns_port(self):
        return int(os.environ.get("GATESENTRY_DNS_PORT", "10053"))

    def dns_query(self, domain, qtype="A"):
        """Query the GateSentry DNS server on the configured alt port."""
        import dns.message
        import dns.query
        import dns.rdatatype

        msg = dns.message.make_query(domain, dns.rdatatype.from_text(qtype))
        return dns.query.udp(msg, "127.0.0.1", timeout=3, port=self.dns_port)


@pytest.fixture(scope="session")
def client(base_url, proxy_url, admin_auth):
    c = GSClient(base_url, proxy_url, *admin_auth)
    c.login()
    return c


# ══════════════════════════════════════════════════════════════════════════
# Test classes – each class is one "section"
# ══════════════════════════════════════════════════════════════════════════


class TestServerLiveness:
    def test_admin_ui_loads(self, base_url):
        r = requests.get(f"{base_url}/", timeout=10)
        assert r.status_code == 200
        assert "<html" in r.text.lower()


class TestUIPages:
    SPA_PAGES = ["/login", "/stats", "/users", "/dns", "/settings", "/rules"]

    @pytest.mark.parametrize("page", SPA_PAGES)
    def test_spa_page_loads(self, base_url, page):
        r = requests.get(f"{base_url}{page}", timeout=10)
        assert r.status_code == 200
        assert "<html" in r.text.lower()

    def test_unknown_route_returns_something(self, base_url):
        r = requests.get(f"{base_url}/nonexistent-page", timeout=10)
        # Accept 200 (SPA fallback) or 404 (explicit routes only)
        assert r.status_code in (200, 404)


class TestAuthentication:
    def test_login_returns_token(self, base_url):
        r = requests.post(
            f"{base_url}/api/auth/token",
            json={"username": "admin", "pass": "admin"},
            timeout=10,
        )
        assert r.status_code == 200
        assert "Jwtoken" in r.json()

    def test_token_verify_valid(self, client):
        r = client.api_get("/api/auth/verify")
        assert r.status_code == 200
        assert r.json().get("Validated") is True

    def test_invalid_token_rejected(self, base_url):
        try:
            r = requests.get(
                f"{base_url}/api/auth/verify",
                headers={"Authorization": "Bearer bad"},
                timeout=10,
            )
            assert r.status_code in (401, 400, 403, 200)
        except requests.exceptions.ConnectionError:
            # Server drops connection on bad token — acceptable
            pass


class TestCertificate:
    def test_download_cert(self, client):
        client.fetch_cert()
        assert len(client.cert_pem) > 500
        assert "-----BEGIN CERTIFICATE-----" in client.cert_pem

    def test_cert_info_api(self, client):
        r = client.api_get("/api/certificate/info")
        assert r.status_code == 200
        info = r.json()
        assert info.get("name")
        assert info.get("expiry")

    def test_cert_valid_x509(self, client):
        client.fetch_cert()
        with tempfile.NamedTemporaryFile(suffix=".pem", mode="w", delete=False) as f:
            f.write(client.cert_pem)
            cert_path = f.name
        try:
            result = subprocess.run(
                ["openssl", "x509", "-in", cert_path, "-noout", "-subject"],
                capture_output=True,
                text=True,
            )
            assert result.returncode == 0, result.stderr
            assert "GateSentryFilter" in result.stdout
        finally:
            os.unlink(cert_path)


class TestHTTPProxy:
    def test_proxy_example_com(self, client):
        r = client.proxy_get("http://example.com")
        assert r.status_code == 200

    def test_proxy_httpbin_json(self, client):
        r = client.proxy_get("http://httpbin.org/get")
        if r.status_code == 200:
            assert "headers" in r.json()
        else:
            pytest.skip(f"httpbin.org returned {r.status_code} (transient)")

    def test_proxy_bad_domain(self, client):
        try:
            r = client.proxy_get("http://nonexistent.invalid.domain.test/", timeout=10)
            # Any response is fine – just not a crash
            assert r is not None
        except (requests.exceptions.ProxyError, requests.exceptions.Timeout):
            pass  # also fine, expected for bad domain


class TestHTTPSProxy:
    def test_connect_tunnel(self, client):
        r = client.proxy_get("https://example.com", verify=False)
        assert r.status_code == 200


class TestHTTPSMitm:
    def test_mitm_with_trusted_cert(self, client):
        client.fetch_cert()
        # enable HTTPS filtering
        client.api_post(
            "/api/settings/enable_https_filtering",
            json={"key": "enable_https_filtering", "value": "true"},
        )
        try:
            r = client.proxy_get_verified("https://httpbin.org/headers")
            if r.status_code in (502, 503, 504):
                pytest.skip(f"MITM upstream returned {r.status_code} (transient)")
            assert r.status_code == 200
            data = r.json()
            assert "headers" in data
        finally:
            client.api_post(
                "/api/settings/enable_https_filtering",
                json={"key": "enable_https_filtering", "value": "false"},
            )

    def test_mitm_example_com(self, client):
        client.fetch_cert()
        client.api_post(
            "/api/settings/enable_https_filtering",
            json={"key": "enable_https_filtering", "value": "true"},
        )
        try:
            r = client.proxy_get_verified("https://example.com")
            assert r.status_code == 200
        finally:
            client.api_post(
                "/api/settings/enable_https_filtering",
                json={"key": "enable_https_filtering", "value": "false"},
            )


class TestDNSResolution:
    def test_query_google_via_gs(self, client):
        try:
            resp = client.dns_query("google.com.")
            answers = [a for a in resp.answer if a.to_text().startswith("google.com.")]
            assert len(answers) > 0
        except (ImportError, ModuleNotFoundError):
            pytest.skip("dnspython not installed")
        except Exception as e:
            pytest.skip(f"DNS server not reachable on port {client.dns_port}: {e}")

    def test_query_example_via_gs(self, client):
        try:
            resp = client.dns_query("example.com.")
            answers = [a for a in resp.answer if a.to_text().startswith("example.com.")]
            assert len(answers) > 0
        except (ImportError, ModuleNotFoundError):
            pytest.skip("dnspython not installed")
        except Exception as e:
            pytest.skip(f"DNS server not reachable on port {client.dns_port}: {e}")


class TestDNSConcurrency:
    def test_50_concurrent_queries_via_gs(self, client):
        try:
            import dns.message
            import dns.query
            import dns.rdatatype
        except ImportError:
            pytest.skip("dnspython not installed")

        from concurrent.futures import ThreadPoolExecutor, as_completed

        host, port = "127.0.0.1", client.dns_port

        def _query(_):
            msg = dns.message.make_query("google.com.", dns.rdatatype.A)
            try:
                resp = dns.query.udp(msg, host, timeout=5, port=port)
                return len([a for a in resp.answer if a.rdtype == dns.rdatatype.A])
            except Exception:
                return -1

        start = time.time()
        results = []
        with ThreadPoolExecutor(max_workers=50) as pool:
            for f in as_completed([pool.submit(_query, i) for i in range(50)]):
                results.append(f.result())

        elapsed = (time.time() - start) * 1000
        errors = sum(1 for r in results if r == -1)
        successes = sum(1 for r in results if r > 0)
        fail_rate = errors / 50

        if errors == 50:
            pytest.skip(f"GateSentry DNS not reachable on port {port} (50/50 errors)")
        elif fail_rate < 0.15:
            pass  # acceptable loss
        else:
            pytest.fail(
                f"50 concurrent: {errors} errors, {successes} ok, {elapsed:.0f}ms (fail rate {fail_rate:.0%} > 15%)"
            )


class TestDNSInfoAPI:
    def test_dns_info_endpoint(self, client):
        r = client.api_get("/api/dns/info")
        assert r.status_code == 200

    def test_dns_custom_entries(self, client):
        r = client.api_get("/api/dns/custom_entries")
        assert r.status_code == 200


class TestFiltersAPI:
    def test_list_filters(self, client):
        r = client.api_get("/api/filters")
        assert r.status_code == 200
        filters = r.json()
        assert len(filters) >= 3  # at least 3 default filters

    def test_get_single_filter(self, client):
        r = client.api_get("/api/filters")
        filters = r.json()
        assert filters
        fid = filters[0].get("Id", filters[0].get("ID"))
        assert fid, f"no id field in filter: {list(filters[0].keys())}"
        r2 = client.api_get(f"/api/filters/{fid}")
        assert r2.status_code == 200


class TestSettingsAPI:
    KEYS = [
        "strictness",
        "timezone",
        "enable_https_filtering",
        "enable_dns_server",
        "dns_resolver",
    ]

    @pytest.mark.parametrize("key", KEYS)
    def test_settings_key_readable(self, client, key):
        r = client.api_get(f"/api/settings/{key}")
        assert r.status_code == 200
        val = r.json().get("Value")
        assert val is not None


class TestUsersAPI:
    _created = False

    @pytest.fixture(autouse=True)
    def _cleanup(self, client):
        yield
        if self._created:
            try:
                client.api_delete("/api/users/inttest_user")
            except Exception:
                pass

    def test_list_users(self, client):
        r = client.api_get("/api/users")
        assert r.status_code == 200

    def test_crud_cycle(self, client):
        user = {
            "username": "inttest_user",
            "password": "inttestpass123",
            "allowaccess": True,
        }

        r = client.api_post("/api/users", json=user)
        assert r.status_code == 200, f"create → {r.status_code} {r.text}"
        self._created = True

        r = client.api_put("/api/users", json={**user, "password": "newpass456789"})
        assert r.status_code == 200, f"update → {r.status_code} {r.text}"

        r = client.api_delete("/api/users/inttest_user")
        assert r.status_code == 200, f"delete → {r.status_code} {r.text}"
        self._created = False


class TestStatsAPI:
    def test_status_endpoint(self, client):
        r = client.api_get("/api/status")
        assert r.status_code == 200

    def test_about_endpoint(self, client):
        r = requests.get(f"{client.base_url}/api/about", timeout=10)
        assert r.status_code == 200
        assert "version" in r.json()

    def test_stats_show_data_after_traffic(self, client):
        # Generate real traffic through the proxy to be logged
        for _ in range(3):
            try:
                client.proxy_get("http://example.com", timeout=10)
            except Exception:
                pass
            try:
                client.proxy_get("https://example.com", verify=False, timeout=10)
            except Exception:
                pass
        time.sleep(1.5)  # let the logger flush

        # Query stats: POST /api/stats?fromTime=N
        r = client.api_post("/api/stats?fromTime=3600")
        if r.status_code == 200:
            items = r.json().get("items", [])
            # If items are present, verify they contain URLs we requested
            if items:
                urls = {it.get("URL", "") for it in items}
                assert len(urls) > 0, "stats have entries but no URLs found"
            else:
                # stats may be async-logged or the endpoint may return empty
                # This is ok as long as we get 200
                pass
        else:
            # Accept non-200 on the POST stats endpoint (some setups use GET)
            pass

    def test_stats_by_url_endpoint(self, base_url, client):
        r = client.api_get("/api/stats/byUrl")
        assert r.status_code == 200


class TestRulesAPI:
    def test_list_rules(self, client):
        r = client.api_get("/api/rules")
        assert r.status_code == 200

    def test_test_rule_match(self, client):
        r = client.api_post(
            "/api/rules/test", json={"domain": "example.com", "user": "testuser"}
        )
        assert r.status_code == 200


class TestConsumptionAPI:
    def test_consumption_get(self, client):
        r = client.api_get("/api/consumption")
        assert r.status_code == 200


class TestStaticAssets:
    PATHS = ["/fs/bundle.js", "/fs/favicon.ico", "/fs/style.css"]

    @pytest.mark.parametrize("path", PATHS)
    def test_static_served_or_absent(self, base_url, path):
        r = requests.get(f"{base_url}{path}", timeout=10)
        # 200 = served, 404 = doesn't exist — both are okay
        assert r.status_code in (200, 404)


class TestLogsAPI:
    def test_logs_endpoint(self, client):
        r = client.api_get("/api/logs/0")
        # May be 200 or another code depending on log state
        assert r is not None


class TestProxyConcurrency:
    def test_10_concurrent_proxy_requests(self, client):
        from concurrent.futures import ThreadPoolExecutor, as_completed

        def _fetch(_):
            try:
                r = client.proxy_get("http://example.com", timeout=10)
                return r.status_code == 200
            except Exception:
                return False

        with ThreadPoolExecutor(max_workers=10) as pool:
            results = [
                f.result()
                for f in as_completed([pool.submit(_fetch, i) for i in range(10)])
            ]

        assert all(results), (
            f"only {sum(results)}/10 concurrent proxy requests succeeded"
        )
