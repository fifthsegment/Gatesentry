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
- **full unittest**: `make tests` -- run all unit tests

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
