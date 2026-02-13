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

### Rule Actions and Content Filtering

- **Action: "block"** — The matched domain is blocked outright. No content filtering applies because the connection is refused before any content is fetched.
- **Action: "allow"** — The matched domain is allowed through the proxy. Content filtering options (keyword scanning, content-type blocking, URL regex blocking, embedded resource blocking) are **only evaluated on allow rules**.

**Important**: Content Filtering Options in the UI are hidden when the rule action is "Block" because those filters never execute — a blocked domain never reaches the content pipeline. Always set action to "Allow" before configuring content filters.

### Per-Rule Filtering (No Global Filters)

All filtering is scoped to individual rules. There are no global filtering pipelines:

- **Content-type blocking** — A **request-level** filter (not a content filter). When the browser fetches a sub-resource (e.g. `<img src="photo.jpeg">`), that is a separate proxy request. The proxy checks the **response** Content-Type header against `blocked_content_types` on the matched rule. If it matches (e.g. `image/jpeg`), the response is blocked with a 403. In the UI, this is in the general rule definition area, not under Content Filtering Options. Enforcement is in `proxy.go` after response headers are read, using `sendInsecureBlockBytes()` for images.
- **Keyword scanning** — A **content filter**. Controlled by `keyword_filter_enabled` on each rule. The proxy only calls `ScanText` when the matched rule has this flag set (`isKeywordFilterEnabled()` in `proxy.go`). Requires MITM for HTTPS.
- **URL regex blocking** — Auto-derived from `url_regex_patterns` array on the rule.
- **Embedded resource blocking** — Auto-derived from `content_domain_lists` array on the rule.
- **SSL Inspection (MITM)** — Must be enabled (`mitm_action: "enable"`) for content filters (keyword, URL regex, domain list) to function on HTTPS traffic. Content-type blocking works without MITM since it only reads response headers.

The legacy `block_type` enum field still exists on the `Rule` struct for backward compatibility but is **no longer used** in matching logic. Filters are auto-derived from populated fields in `MatchRule()` (`application/rules.go`).

### Rule Evaluation Flow

1. Request arrives at proxy → `matchRuleForRequest()` finds first matching rule by priority
2. If rule action is "block" → serve block page, done
3. If rule action is "allow" → proxy the request, populate `RuleMatch` with filter config
4. Response arrives → content pipeline checks `RuleMatch` for active filters
5. Each filter (keyword, content-type, URL regex, domain list) only runs if the matched rule has data for it

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
