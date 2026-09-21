# Agent Notes for GateSentry

Important context for AI agents working on this project.

## Branch

- **Active development is `v2`** (this checkout tracks `myfork/v2`). Do not treat origin/master or v1.20.x / "1.2" as the working tree.
- Upstream: `origin` = https://github.com/fifthsegment/Gatesentry.git
- Fork: `myfork` = https://github.com/jbarwick/Gatesentry.git
- Binary version is `GATESENTRY_VERSION` in `main.go` (current: `2.0.0-beta.21`). That string is the only release tag. Do not invent a different version for the image.

## Which tree to use for deployments

| Location | What it is | Use for |
|----------|------------|---------|
| **This repo (`Gatesentry`, branch `v2`)** | Application source, Dockerfile, tests, `build.sh` / `release.sh` / `deploy.sh` | All code, image builds, v2 features |
| **`../gatesentry-synology`** | One-file Synology compose overlay | NAS-specific volume/network/env. `deploy.sh` keeps its image tag in sync. |
| **`/volume1/docker/Gatesentry/` on monster-jj** | Live production compose + data volume | Actual run. `deploy.sh` writes the image tag here and recreates the container. |

**Use this v2 repo to build and publish. Deploy on monster-jj with the NAS compose** (host network, port 53, admin 9876, data on `/volume1/docker/Gatesentry/gatesentry`), **not** with this repo's public samples: `docker-compose.yml` (bridged 8080/10053) or `docker-compose.host.yml` (generic host-network, admin 8080).

## Ship pipeline (required)

Do **not** ad-hoc `scp` binaries, `ssh` a `docker build` on the NAS, or hand-edit the compose tag. Use these three scripts in order. Version is always `GATESENTRY_VERSION` from `main.go`.

```bash
./build.sh      # 1. binaries only (UI + Go → bin/gatesentrybin)
./release.sh    # 2. package + publish to Nexus (requires local Docker)
./deploy.sh     # 3. install that tag on the NAS
```

| Script | Does | Does not |
|--------|------|----------|
| **`./build.sh`** | Build Svelte UI (if `ui/node_modules` exists) and `bin/gatesentrybin` | Docker, Nexus, NAS |
| **`./release.sh`** | `docker build` a runtime image; tag `gatesentry:<ver>`, `<nexus>/gatesentry:<ver>`, and `:latest`; **push both tags to Nexus** | Run the container; touch NAS compose |
| **`./deploy.sh`** | SSH to monster-jj, set the compose image tag, `docker-compose pull && up -d`, wait for admin, sync `../gatesentry-synology` | Rebuild the binary or image |

**Local Docker is required for `release.sh`.** Start Docker Desktop (WSL integration) if `docker info` fails. The NAS still runs the production container; that is a different Docker daemon.

**Nexus publish (`release.sh`)** needs `NEXUS_SERVER` (default `https://monster-jj.jvj28.com:9092`) and credentials:

- Hostname (e.g. `monster-jj.jvj28.com:9092`): **`NEXUS_USERNAME` + `NEXUS_PASSWORD`**
- Nexus by **IP**: **`NEXUS_TOKEN`** as docker `-u` (account username is often rejected)
- If the first method fails, `release.sh` tries the other when both are set

Do not print these values. Image: `monster-jj.jvj28.com:9092/gatesentry:<GATESENTRY_VERSION>`.

**NAS install (`deploy.sh`)** uses Synology's `docker-compose` binary (not the `docker compose` plugin). Target: `root@monster-jj`, compose file `/volume1/docker/Gatesentry/docker-compose.yml`. After deploy, admin is `http://monster-jj:9876/gatesentry/` (`/api/about` reports `version`).

`docker-publish.sh` is the older Docker Hub / Nexus helper. Do not use it for the NAS pipeline; `release.sh` is the Nexus publish step.

## Deployment Target

- **Production/target server**: `monster-jj` — Synology DS1618+, SSH as root: `ssh root@monster-jj`
- **Image build**: local Docker Desktop via `./release.sh`
- **Production container**: Docker on monster-jj via `./deploy.sh`
- To check logs, containers, or service health: `ssh root@monster-jj` first, then `export PATH=$PATH:/usr/local/bin:/usr/bin` (docker is `/usr/local/bin/docker`; compose is `docker-compose`)
- The GateSentry **process** does not run on the workstation in the normal workflow (`./restart.sh` is local/dev only)
- Compose project: `/volume1/docker/Gatesentry/docker-compose.yml`
- Data: `/volume1/docker/Gatesentry/gatesentry/` (`GSSettings`, `log.db`, `devices.json`, `filterfiles/`)

## Project Structure

- **Go backend** — Multi-module workspace (`go.work`): root (`gatesentrybin`), `./application`, `./gatesentryproxy`
- **Svelte frontend** — In `ui/` directory (Svelte 4, Vite 4, Carbon Components Svelte)
- **Embedded UI** — The built UI is copied into `application/webserver/frontend/files/` and embedded in the Go binary
- **`gatesentryproxy` is Go 1.17** — range-loop variable pointers (`&item` in `for _, item := range`) are still a bug there

## Addresses are values, not identity

A device's IPv4/IPv6 is **whatever it has right now**. It can change on DHCP renew. Inspect it at runtime (device store, ARP, a DNS answer). Do not bake any site unicast address, LAN prefix, or box hostname into application code, product defaults, or UI copy.

- Identity is hostname / MAC / user-assigned name. IP is a field on the record.
- Do not merge two named devices because an mDNS packet put name A at address B.
- Tests and comments use documentation ranges only: **192.0.2.0/24** (RFC 5737) and **2001:db8::/32** (RFC 3849). Never copy the operator's LAN into fixtures.
- Product resolver default is `8.8.8.8:53` (IPv6 resolver default empty). Site DNS is settings or `GATESENTRY_DNS_RESOLVER` at deploy time.

## Build & Run (local/dev)

- **Build binaries**: `./build.sh` → `bin/gatesentrybin`
- **Restart a local binary** (does NOT rebuild or deploy): `./restart.sh`
- Ship to production: see **Ship pipeline** above (`build.sh` → `release.sh` → `deploy.sh`)
- The server runs from the `bin/` directory (working dir matters for data paths)
- Log output goes to `log.txt` locally; on the NAS use `docker logs gatesentry`
- **Deep test DNS & proxy**: `scripts/dns_deep_test.sh`
- **Deep test proxy**: `scripts/proxy_deep_tests.sh`
- **full unittest**: `make tests`

### Proxy Deep Tests — State Management

`scripts/proxy_deep_tests.sh` saves and restores the server's full state (rules, settings, keyword filters) around each test run. During setup, **all existing proxy rules are deleted** so only the test-created rules (`PT: ...` prefixed) are active. On exit (including Ctrl-C), all rules are deleted and the original saved rules are re-created.

## Ports

Defaults in this repo (local `docker-compose.yml` / `run.sh`):

| Service       | Default Port | Environment Variable       |
|---------------|-------------|---------------------------|
| Admin UI      | **8080**    | `GS_ADMIN_PORT`           |
| Admin HTTPS   | **9877**    | `GS_ADMIN_PORT_SSL`       |
| DNS server    | **10053**   | `GATESENTRY_DNS_PORT`     |
| Proxy server  | **10413**   | (see proxy config)        |
| Transparent   | **10414**   | `GS_TRANSPARENT_PROXY_PORT` |
| Metrics       | **same as admin** | `/metrics` on the admin HTTP server (no extra port) |

**Production on monster-jj** (`network_mode: host`):

| Service    | Port | Notes |
|------------|------|--------|
| Admin UI   | **9876** | **`http://monster-jj:9876/gatesentry/`** (`GS_BASE_PATH=/gatesentry`). Root `http://monster-jj:9876/` 302s there. **Do not use `monster-jj.jvj28.com`** — nginx on :80/:443 redirects that FQDN to HTTPS and never reaches GateSentry. |
| Admin HTTPS | **9877** | `GS_ADMIN_PORT_SSL` — same UI over TLS when enabled in Settings. |
| Metrics    | **9876** | `http://monster-jj:9876/metrics` (unauthenticated, same listener) |
| DNS        | **53**   | `GATESENTRY_DNS_PORT=53` — `dig @monster-jj -p 53 example.com` |
| Proxy      | **10413** | |
| Transparent| **10414** | |

Do **not** document metrics as a separate service on 9876. 9876 is the admin port in production; metrics share it.

## Environment Variables

See `run.sh` and `restart.sh` for the full set. Important ones:

- `GATESENTRY_DNS_ADDR` — DNS listen address (default: `::` locally; production `0.0.0.0`)
- `GATESENTRY_DNS_PORT` — DNS listen port (default: `10053`)
- `GATESENTRY_DNS_RESOLVER` — Upstream DNS resolver. **On `Init()`, a non-empty value overwrites stored `dns_resolver`.** Use this to recover when the saved resolver is unreachable.
- `GS_ADMIN_PORT` — Admin web UI port (default `8080`; production `9876`)
- `GS_ADMIN_PORT_SSL` — Admin HTTPS port (default `9877`; production `9877`). Serves the same UI when **Admin HTTPS Server** is enabled in Settings. Server cert/key and admin CA are stored separately from MITM `capem`/`keypem`.
- `GS_BASE_PATH` — URL prefix (default `/gatesentry`; production `/gatesentry`)
- `GS_MAX_SCAN_SIZE_MB` — Max content scan size (default: `2`)
- `GS_AI_GROK_API_KEY`, `GS_AI_OPENAI_API_KEY`, `GS_AI_OLLAMA_URL`, `GS_AI_OLLAMA_MODEL`, `GS_AI_GROK_MODEL`, `GS_AI_OPENAI_MODEL`, `GS_AI_IMAGE_FILTERING_MODE` — optional presets for `/gatesentry/ai`. **Seeded only when the stored setting is empty** (placeholders like `CHANGE_ME` are ignored). The AI page can change them; a later start does not overwrite a saved value. Same names are used in `.env` / docker compose and live vision tests.

## curl / HTTP Requests

**IMPORTANT**: The development machine has `http_proxy` set to the GateSentry proxy (`http://monster-jj:10413`). Any `curl` or HTTP request from the terminal will be routed through the proxy unless you bypass it.

**Always use `--noproxy '*'` with curl:**

```bash
# Correct
curl --noproxy '*' http://localhost:8080/api/about

# WRONG — will go through the proxy and fail
curl http://localhost:8080/api/about
```

Without `--noproxy '*'`, requests hit the GateSentry proxy on port 10413 instead of the admin UI, producing misleading errors (400, 508, etc.).

Admin URL on the LAN: **`http://monster-jj:9876/gatesentry/`**. Avoid the HTTPS vhost on :80/:443. When DNS or the proxy is unhealthy, use the NAS IPv4 and `--noproxy '*'`.

## Authentication

- Admin UI requires JWT authentication
- Login endpoint: `POST /api/auth/token` with `{"username": "...", "pass": "..."}`
- Response: `{"Validated": true, "Jwtoken": "..."}` on success
- Use the JWT as `Authorization: Bearer <token>` header on subsequent requests
- Admin credentials are stored encrypted in `bin/gatesentry/GSSettings` (production: `/volume1/docker/Gatesentry/gatesentry/GSSettings`)

## Settings API

- **GET** `/api/settings/{key}` — Returns `{"Key": "...", "Value": "..."}` (uppercase, no JSON tags)
- **POST** `/api/settings/{key}` — Accepts `{"key": "...", "value": "..."}` (lowercase, uses `Datareceiver` struct JSON tags)
- Settings keys must be whitelisted in `application/webserver/endpoints/handler_settings.go` for both GET and POST

## Blocked Domain Middleware

The `blockedDomainMiddleware` in `application/webserver/webserver.go` intercepts requests where the HTTP `Host` header doesn't match a known GateSentry hostname. It serves a block page instead of the admin UI. Known hosts include `localhost`, `127.0.0.1`, `::1`, the machine's hostname (`monster-jj`), `hostname.local`, and local interface IPs.

**Gap:** FQDNs such as `monster-jj.jvj28.com` are **not** allowlisted. A browser that uses the FQDN gets a DNS-block page, which looks like the admin is down. Access via hostname, `.local`, or IP.

## Data Storage

- Settings file: `GSSettings` (encrypted JSON). `MapStore` persists via `Update()` → `Set()` → `Persist()` → disk
- Filter files: `filterfiles/`
- Logs: BuntDB `log.db` (was 134MB in production — treat as a capacity/availability issue)
- Devices: `devices.json`

### MapStore locking (known footgun)

`MapStore.Update` holds `Mutex` and **returns without Unlock()** if JSON unmarshal fails (`application/storage/storage.go`). After that, every settings write deadlocks. `Get` / `SetDefault` do not take the mutex (data race with `Update`). Do not add more unlocked readers; fix the unlock path with `defer m.Mutex.Unlock()`.

## Proxy Rule Architecture

### Overview

All filtering is scoped to individual rules. There are no global filtering pipelines. Rules are evaluated in **priority order** (lower number = higher priority). The first rule that fully matches a request is applied — subsequent rules are skipped.

### HTTPS Visibility

The proxy's ability to inspect traffic depends on whether SSL MITM (Man-in-the-Middle) inspection is active:

| What the proxy sees          | HTTP | HTTPS (no MITM) | HTTPS (MITM) |
|------------------------------|------|------------------|---------------|
| Domain / hostname            | yes  | yes              | yes          |
| URL path & query string      | yes  | no               | yes          |
| Response Content-Type header | yes  | no               | yes          |
| Response body (for keywords) | yes  | no               | yes          |

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
4. **Resolve MITM** — `"enable"` → MITM on, `"disable"` → MITM off, `"default"` → global `enable_https_filtering`. If MITM is off AND the request is HTTPS, steps 5–7 are **skipped**. HTTP always passes through steps 5–7.
5. **Check URL patterns** *(always for HTTP; requires MITM for HTTPS)* — If `url_regex_patterns` is non-empty and NO pattern matches, skip this rule.
6. **Check content-type** *(always for HTTP; requires MITM for HTTPS)* — If `blocked_content_types` is non-empty and NO type matches, skip this rule.
7. **Check keyword filter** *(always for HTTP; requires MITM for HTTPS)* — If `keyword_filter_enabled` is true and the keyword score exceeds the watermark, **force a Block action**.
8. **Apply rule action** — `"allow"` or `"block"`.

If **no rule matches**, the request is allowed (default-allow).

### Implementation Notes

- Steps 1–3 happen in `application/rules.go` → `MatchRule()` (pre-proxy, domain-level match).
- Step 4 is resolved partly in `rules.go` (`ShouldMITM`) and partly in `proxy.go` (global fallback for `"default"`).
- Steps 5–7 happen in `gatesentryproxy/proxy.go` **after** the request has been proxied.
- Step 8's block action at the domain level short-circuits in `proxy.go` before the request is proxied.

### UI Form Layout

The rule form (`ui/src/routes/rules/rform.svelte`) matches this pipeline:

1. **Rule Definition** — Name, enabled toggle, active hours, MITM setting, description
2. **User Match Criteria** — User list (empty = all users)
3. **Rule Selection Criteria** — Domain patterns, domain lists, URL patterns, content-type
4. **Matching Results** — Keyword filter toggle and final action (Allow / Block)

## Availability — Admin must work when upstream DNS is down

This is a product requirement, not an ops inconvenience. The last outage was: site recursive DNS down → operator tried to open admin to change `dns_resolver` → admin unresponsive → process later SIGTERM'd.

Constraints the code must satisfy:

- Failed upstream forwards must be **negatively cached** (and/or circuit-broken). Do not re-dial a dead resolver on every client retry.
- Cap in-flight upstream queries (semaphore). Timeout is already 3s (`forwardDNSRequest`).
- Do not `log.Println` or spawn a buntdb writer (`LogDNS`) on every DNS query. Gate verbose DNS logs behind `GS_DEBUG_LOGGING`.
- Serve/resolve the admin host **locally** (device store, static A record, or Host allowlist including FQDN) so the UI does not depend on upstream DNS.
- Admin HTTP server needs timeouts. Stats/logs handlers must not full-scan a 100MB+ `log.db` on page load.
- When DNS is unhealthy, operators should use `http://<lan-ip>:9876/gatesentry/` with proxy bypass.

`GATESENTRY_DNS_RESOLVER` is the emergency override: `Init()` writes it into `dns_resolver` before the DNS server starts.

## Code Quality & Common Pitfalls

These rules are derived from recurring issues caught during PR code reviews. **Always** check new code against these before committing.

### Linting

- **Run `make lint`** before committing Go changes. The project uses `golangci-lint` (config: `.golangci.yml`).
- **Run `shellcheck`** on any modified `.sh` files.
- Pre-commit hooks (`.pre-commit-config.yaml`) automate both — install with `pre-commit install`.
- There is no such thing as pre-existing warnings and errors from the linter or unit tests. All tests must work, and all errors must be cleared.

### Security — HTML/JS/Template Injection

- **Never interpolate user-controlled or config values directly into HTML or JavaScript strings** with `fmt.Sprintf`. This includes:
  - The `host` from `r.Host` in block pages → use `html.EscapeString(host)`
  - `basePath` injected into `<script>` tags → JSON-encode for JS, HTML-escape for `href`
  - `proxyHost`/`proxyPort` in PAC file JS → validate as hostname/IP and numeric port first
- When generating HTML, prefer `html/template` over `fmt.Sprintf` whenever possible.

### Security — Proxy Header Hygiene

- **Strip hop-by-hop and proxy-only headers** before forwarding requests upstream, especially in WebSocket tunnels. Do not forward:
  - `Proxy-Authorization`, `Proxy-Authenticate`, `Proxy-Connection`
  - Hop-by-hop headers listed in `Connection:` header values
  - `TE`, `Transfer-Encoding`, `Upgrade` (unless specifically needed for the tunnel)

### Security — Secrets & Credentials

- **Never commit real private keys or credentials**, even for tests. Use `tests/fixtures/gen_test_certs.sh` to generate ephemeral test certs.
- **Never pass passwords via CLI flags** (e.g., `docker login -p`). Use `--password-stdin` or environment variables.
- Hardcoded JWT HMAC in `application/webserver/webserver.go` (`hmacSampleSecret`) is a known issue — do not copy that pattern.

### Correctness — Go-Specific

- **Range loop variable pointers**: In Go < 1.22, `&item` inside `for _, item := range` returns the address of the *reused* loop variable. Use `for i := range items` and `&items[i]`. Critical in `gatesentryproxy` (Go 1.17).
- **DNS FQDN trailing dots**: Normalize with `strings.TrimRight(domain, ".")` before comparing against domain lists or patterns.
- **DNS cache keys**: `dns.TypeToString[qtype]` returns empty string for unknown qtypes. Fall back to `strconv.Itoa(int(qtype))`.
- **`http.Error` with JSON bodies**: `http.Error` sets `Content-Type: text/plain`. For JSON, set `Content-Type: application/json` and use `w.WriteHeader()` + `json.NewEncoder`.
- **Mutex unlock on all paths**: never `return` between `Lock()` and `Unlock()` without `defer`.

### Correctness — Configuration

- **Never hardcode ports or addresses**. Always read from environment variables or settings.
- **Normalize `basePath`**: Ensure it starts with `/` and does NOT end with `/`.
- **Test scripts**: Default to `localhost` / `127.0.0.1`. Use environment variables for non-default addresses.

### Code Quality

- **No verbose logging in hot paths**. Functions called on every request (DNS handler, proxy handlers, `GetHistory`) should not log per-invocation unless behind a debug flag. The production DNS handler still logs every query — do not add more of that.
- **No no-op tests**. Every test function must contain at least one assertion.
- **All tests must pass.** Do not dismiss lint or unittest failures as pre-existing. Fix them.
- **Documentation consistency**: When changing default ports, paths, or URLs, grep README.md, AGENTS.md, Makefile, Dockerfile, docker-compose.yml, run.sh/restart.sh, and the Synology compose.

## Current Work In Progress

We are implementing the **Domain List & Rules Enhancement Plan** (`DOMAIN_LIST_RULES_PLAN.md`) on **v2**. This unifies DNS blocklists, proxy filters, and per-user rules around reusable Domain Lists.

### Completed So Far

- **Phase 1** — `DomainListManager` foundation (`application/domainlist/`): CRUD, index, loader, migration, API endpoints, tests
- **Phase 2** — DNS Server Migration: DNS server uses shared `DomainListIndex`
- **Phase 3** — Rule Struct Expansion: `DomainPatterns` and `DomainLists` (18 rules tests)
- **Phase 4** — Content Filtering by Domain List (8 new tests)
- **UI** — Domain Lists page (`/domainlists`), DNS page allow/block list assignment, menu cleanup
- **Settings persistence** — `dns_domain_lists` and `dns_whitelist_domain_lists` on the GET/POST whitelist

### Known issues

- **DNS page UI load/save of assigned list IDs** (`dnslists.svelte` / `dns.svelte`) was still being debugged. DNS filtering itself works.
- **Production hang when upstream DNS is down** — see Availability section. Highest priority before bringing monster-jj back.
- **`log.db` growth** (134MB) — raw logs still expire at 7 days; stats charts read minute rollups (2.0.0-beta.21).
- Production image on monster-jj is `GATESENTRY_VERSION` in `main.go`. Ship with `./build.sh && ./release.sh && ./deploy.sh`.
