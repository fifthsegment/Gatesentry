# Agent Notes for GateSentry

Important context for AI agents working on this project.

## Project Structure

- **Go backend** — Multi-module workspace (`go.work`): root (`gatesentrybin`), `./application`, `./gatesentryproxy`
- **Svelte frontend** — In `ui/` directory (Svelte 4, Vite 4, Carbon Components Svelte)
- **Embedded UI** — The built UI is copied into `application/webserver/frontend/files/` and embedded in the Go binary

## Build & Run

- **Build everything** (UI + Go binary): `./build.sh`
- **Restart the running server** (does NOT rebuild): `./restart.sh`
- **Typical workflow**: Edit code → `./build.sh` → `./restart.sh`
- The Go binary is output to `bin/gatesentrybin`
- The server runs from the `bin/` directory (working dir matters for data paths)
- Log output goes to `log.txt`
- **Deep test DNS & proxy**: `scripts/dns_deep_test.sh` — fully tests and retests DNS services and proxy
- **Deep test proxy**: `scripts/proxy_deep_tests.sh` — comprehensive proxy filtering, MITM, and content pipeline tests
- **full unittest**: `make tests` -- run all unit tests

### Proxy Deep Tests — State Management

`scripts/proxy_deep_tests.sh` saves and restores the server's full state (rules, settings, keyword filters) around each test run. During setup, **all existing proxy rules are deleted** so only the test-created rules (`PT: ...` prefixed) are active — this ensures deterministic results regardless of what rules the admin has configured. On exit (including Ctrl-C), all rules are deleted and the original saved rules are re-created.

## Ports

| Service       | Default Port | Environment Variable       |
|---------------|-------------|---------------------------|
| Admin UI      | **8080**    | `GS_ADMIN_PORT`           |
| DNS server    | **10053**   | `GATESENTRY_DNS_PORT`     |
| Proxy server  | **10413**   | (see proxy config)        |

## Environment Variables

See `run.sh` and `restart.sh` for the full set of environment variables and their defaults:

- `GATESENTRY_DNS_ADDR` — DNS listen address (default: `::`) tcp6 stack
- `GATESENTRY_DNS_PORT` — DNS listen port (default: `10053`)
- `GATESENTRY_DNS_RESOLVER` — Upstream DNS resolver (default: `192.168.1.1:53`)
- `GS_ADMIN_PORT` — Admin web UI port (default: `8080`)
- `GS_MAX_SCAN_SIZE_MB` — Max content scan size (default: `2`)

## ⚠️ curl / HTTP Requests

**IMPORTANT**: The development machine has `http_proxy` set to the GateSentry proxy (`http://monster-jj:10413`). Any `curl` or HTTP request from the terminal will be routed through the proxy unless you bypass it.

**Always use `--noproxy '*'` with curl:**

```bash
# Correct
curl --noproxy '*' http://localhost:8080/api/about

# WRONG — will go through the proxy and fail
curl http://localhost:8080/api/about
```

Without `--noproxy '*'`, requests hit the GateSentry proxy on port 10413 instead of the admin UI on port 8080, producing misleading errors (400, 508, etc.).

## Authentication

- Admin UI requires JWT authentication
- Login endpoint: `POST /api/auth/token` with `{"username": "...", "pass": "..."}`
- Response: `{"Validated": true, "Jwtoken": "..."}` on success
- Use the JWT as `Authorization: Bearer <token>` header on subsequent requests
- Admin credentials are stored encrypted in `bin/gatesentry/GSSettings`

## Settings API

- **GET** `/api/settings/{key}` — Returns `{"Key": "...", "Value": "..."}` (uppercase, no JSON tags)
- **POST** `/api/settings/{key}` — Accepts `{"key": "...", "value": "..."}` (lowercase, uses `Datareceiver` struct JSON tags)
- Settings keys must be whitelisted in `application/webserver/endpoints/handler_settings.go` for both GET and POST

## Blocked Domain Middleware

The `blockedDomainMiddleware` in `webserver.go` intercepts requests where the HTTP `Host` header doesn't match a known GateSentry hostname. It serves a block page instead of the admin UI. Known hosts include `localhost`, `127.0.0.1`, `::1`, the machine's hostname, and all local network IPs.

## Data Storage

- Settings file: `bin/gatesentry/GSSettings` (encrypted JSON)
- Filter files: `bin/gatesentry/filterfiles/`
- The `MapStore` persists via `Update()` → `Set()` → `Persist()` → writes to disk

## Proxy Rule Architecture

### Overview

All filtering is scoped to individual rules. There are no global filtering pipelines. Rules are evaluated in **priority order** (lower number = higher priority). The first rule that fully matches a request is applied — subsequent rules are skipped.

### HTTPS Visibility

The proxy's ability to inspect traffic depends on whether SSL MITM (Man-in-the-Middle) inspection is active:

| What the proxy sees          | HTTP | HTTPS (no MITM) | HTTPS (MITM) |
|------------------------------|------|------------------|---------------|
| Domain / hostname            | ✅   | ✅               | ✅            |
| URL path & query string      | ✅   | ❌               | ✅            |
| Response Content-Type header | ✅   | ❌               | ✅            |
| Response body (for keywords) | ✅   | ❌               | ✅            |

Because virtually all sites are HTTPS, **MITM must be enabled** for URL patterns, content-type matching, and keyword scanning to function.

### MITM Setting Resolution

Each rule has a `mitm_action` field with three possible values:
- `"enable"` — Always MITM this traffic (decrypt HTTPS)
- `"disable"` — Never MITM (pass-through encrypted tunnel)
- `"default"` — Use the **global setting** (`enable_https_filtering` in GSSettings)

The resolved MITM state determines whether steps 5–7 below can execute.

### Rule Evaluation Flow (8-Step Pipeline)

For each incoming proxy request, rules are evaluated in priority order:

1. **Check rule status** — If the rule is disabled, or the current local time is outside the rule's active hours window, **skip this rule**.

2. **Check user list** — If the rule's user list is empty, it applies to all users. If non-empty and the requesting user is NOT in the list, **skip this rule**.

3. **Check domain match** — Compare the request hostname against the rule's Domain Patterns and Domain Lists. If both are empty (catch-all rule), the domain matches. If non-empty and the domain does NOT match any pattern or list, **skip this rule**.

4. **Resolve MITM** — Determine the effective MITM state for this rule: `"enable"` → MITM on, `"disable"` → MITM off, `"default"` → use global `enable_https_filtering` setting. If MITM is off AND the request is HTTPS, steps 5–7 are **skipped** (the proxy cannot see URL paths, content-types, or body content through an encrypted tunnel) — proceed directly to step 8. **HTTP requests always pass through steps 5–7** regardless of the MITM setting.

5. **Check URL patterns** *(always for HTTP; requires MITM for HTTPS)* — If the rule has `url_regex_patterns`, match them against the full request URL. If non-empty and NO pattern matches, **skip this rule** (fall through to next rule). If empty, this criterion is not evaluated (effective match).

6. **Check content-type** *(always for HTTP; requires MITM for HTTPS)* — If the rule has `blocked_content_types`, match them against the response `Content-Type` header. If non-empty and NO type matches, **skip this rule**. If empty, this criterion is not evaluated (effective match).

7. **Check keyword filter** *(always for HTTP; requires MITM for HTTPS)* — If `keyword_filter_enabled` is true, scan the response body for blocked keywords. If the keyword score exceeds the watermark threshold, **force a Block action** regardless of the rule's configured action. If below the watermark, continue to step 8.

8. **Apply rule action** — All match criteria are satisfied. Apply the rule's action:
   - `"allow"` → Proxy the request normally, deliver the response to the client.
   - `"block"` → Serve a block page. The response body (if any) is discarded.

If **no rule matches** after evaluating all rules, the request is allowed through (default-allow).

### Implementation Notes

- Steps 1–3 happen in `application/rules.go` → `MatchRule()` (pre-proxy, domain-level match).
- Step 4 is resolved partly in `rules.go` (`ShouldMITM` field) and partly in `proxy.go` (global fallback for `"default"`).
- Steps 5–7 happen in `gatesentryproxy/proxy.go` **after** the request has been proxied and the response headers/body are available. They are "post-response match criteria" — if they don't match, the rule is conceptually skipped (but since the request is already in flight, the proxy falls back to allowing it).
- Step 8's block action at the domain level (step 3 match + no MITM-dependent criteria) short-circuits in `proxy.go` before the request is proxied.

### UI Form Layout

The rule form (`ui/src/routes/rules/rform.svelte`) is organized to match this pipeline:

1. **Rule Definition** — Name, enabled toggle, active hours, MITM setting, description
2. **User Match Criteria** — User list (empty = all users)
3. **Rule Selection Criteria** — Domain patterns, domain lists, URL patterns, content-type. URL patterns and content-type show an informational "HTTPS requires MITM" badge when MITM is off (fields remain editable since they always work on HTTP).
4. **Matching Results** — Keyword filter toggle (shows "HTTPS requires MITM" badge when MITM is off, remains editable) and final action (Allow / Block).

## Code Quality & Common Pitfalls

These rules are derived from recurring issues caught during PR code reviews. **Always** check new code against these before committing.

### Linting

- **Run `make lint`** before committing Go changes. The project uses `golangci-lint` (config: `.golangci.yml`).
- **Run `shellcheck`** on any modified `.sh` files.
- Pre-commit hooks (`.pre-commit-config.yaml`) automate both — install with `pre-commit install`.

### Security — HTML/JS/Template Injection

- **Never interpolate user-controlled or config values directly into HTML or JavaScript strings** with `fmt.Sprintf`. This includes:
  - The `host` from `r.Host` in block pages → use `html.EscapeString(host)`
  - `basePath` injected into `<script>` tags → JSON-encode for JS, HTML-escape for `href`
  - `proxyHost`/`proxyPort` in PAC file JS → validate as hostname/IP and numeric port first
- When generating HTML, prefer `html/template` over `fmt.Sprintf` whenever possible.

### Security — Proxy Header Hygiene

- **Strip hop-by-hop and proxy-only headers** before forwarding requests upstream, especially in WebSocket tunnels. The following headers must NOT be forwarded:
  - `Proxy-Authorization`, `Proxy-Authenticate`, `Proxy-Connection`
  - Hop-by-hop headers listed in `Connection:` header values
  - `TE`, `Transfer-Encoding`, `Upgrade` (unless specifically needed for the tunnel)

### Security — Secrets & Credentials

- **Never commit real private keys or credentials**, even for tests. Use `tests/fixtures/gen_test_certs.sh` to generate ephemeral test certs.
- **Never pass passwords via CLI flags** (e.g., `docker login -p`). Use `--password-stdin` or environment variables.
- Test fixture keys in `.gitignore` are OK; anything in tracked files must be obviously synthetic.

### Correctness — Go-Specific

- **Range loop variable pointers**: In Go < 1.22, `&item` inside `for _, item := range` returns the address of the *reused* loop variable. Use `for i := range items` and `&items[i]` instead, or assign to a local variable first. The `gatesentryproxy` module is Go 1.17 — this is especially critical there.
- **DNS FQDN trailing dots**: DNS-derived hostnames may include a trailing `.` (e.g., `example.com.`). Always normalize with `strings.TrimRight(domain, ".")` before comparing against domain lists or patterns.
- **DNS cache keys**: `dns.TypeToString[qtype]` returns empty string for unknown qtypes. Always fall back to a numeric string (e.g., `strconv.Itoa(int(qtype))`) to prevent cache key collisions.
- **`http.Error` with JSON bodies**: `http.Error` sets `Content-Type: text/plain`. If the body is JSON, manually set `Content-Type: application/json` and use `w.WriteHeader()` + `json.NewEncoder(w).Encode()`.

### Correctness — Configuration

- **Never hardcode ports or addresses**. Always read from environment variables or settings:
  - Admin port → `GS_ADMIN_PORT` (default `8080`)
  - DNS port → `GATESENTRY_DNS_PORT` (default `10053`)
  - Proxy port → configured in settings
- **Normalize `basePath`**: Ensure it starts with `/` and does NOT end with `/` to avoid double-slash redirects.
- **Test scripts**: Default to `localhost` / `127.0.0.1`, not private LAN IPs. Use environment variables for non-default addresses.

### Code Quality

- **No verbose logging in hot paths**. Functions called on every request (e.g., `GetHistory`, proxy handlers) should not log per-invocation unless behind a debug flag. Use `log.Printf` sparingly in:
  - Request-level proxy handling
  - DNS query resolution
  - Cache operations called from API handlers
- **No no-op tests**. Every test function must contain at least one assertion. A test that only calls `_ = rm` gives false coverage.
- **Documentation consistency**: When changing default ports, paths, or URLs, grep for the old value across README.md, AGENTS.md, Makefile, Dockerfile, docker-compose.yml, and run.sh/restart.sh. Update all occurrences.

## Current Work In Progress

We are implementing the **Domain List & Rules Enhancement Plan** (`DOMAIN_LIST_RULES_PLAN.md`). This is a multi-phase effort to unify DNS blocklists, proxy filters, and per-user rules around reusable "Domain Lists."

### Completed So Far

- **Phase 1** — `DomainListManager` foundation (`application/domainlist/`): CRUD, index, loader, migration, API endpoints, tests (19 passing)
- **Phase 2** — DNS Server Migration: DNS server uses shared `DomainListIndex` instead of its own `blockedDomains` map
- **Phase 3** — Rule Struct Expansion: Rules can reference `DomainPatterns` (plural wildcards) and `DomainLists` (list IDs) for domain matching (18 rules tests passing)
- **Phase 4** — Content Filtering by Domain List: MITM content filtering can block embedded resources by domain list membership (8 new tests)
- **UI** — Domain Lists management page (`/domainlists`), DNS page rewritten with allow/block list assignment sections, menu cleanup (removed old Block List and Exception Hostnames items)
- **Settings persistence fix** — Added `dns_domain_lists` and `dns_whitelist_domain_lists` to the GET/POST whitelists in `handler_settings.go`

### Known Issue: DNS Page UI Not Loading/Saving Lists

The `/dns` page (`dnslists.svelte`) is supposed to let the admin add/remove Domain Lists to DNS blocklist and whitelist sets (stored as `dns_domain_lists` and `dns_whitelist_domain_lists` settings keys). **The DNS filtering itself works** — domains are being blocked correctly. However, **the UI is not loading or saving** the assigned list IDs when navigating to the page. This is still being debugged.
