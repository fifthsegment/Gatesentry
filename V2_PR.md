# PR Title

> `[RFC] v2: Per-user rule-based filtering, Domain List system, proxy hardening, and UI overhaul`

# PR Description

Hi @fifthsegment — I've been working on a substantial set of improvements in my fork and wanted to share them for feedback. This is **not** a "please merge now" PR — it's more of an RFC / showcase. The branch is `v2` and represents about 40 commits / ~44K insertions across 169 files.

## Architecture Shift: Global → Per-User Rule-Based Filtering

The most significant change in v2 is **moving away from global filter lists toward a fully rule-based filtering system.**

In v1, proxy rules could already be defined for a user, and the rule system provided per-user targeting — that foundation was solid. However, the primary filtering mechanisms — blocked sites, exception lists, keyword filters, and DNS blocklists — operated as **global pipelines** that applied to everyone on the network identically. The per-user rules and the global filters were essentially two separate systems. Additionally, rules in v1 were strictly block rules — they could only deny access, not explicitly grant it.

In v2, **those global filtering pipelines have been consolidated into the per-user rule system.** There are no longer separate global blocked-site lists or global keyword filters running outside of rules. Instead, every filtering decision flows through the rule pipeline:

- **Everything is a rule now** — Blocked domains, URL patterns, content-type filtering, and keyword scanning are all configured as properties of individual rules rather than global lists. What was previously a global "blocked sites" list becomes a rule referencing a Domain List with action "block."
- **Rules are match + action, not just block lists** — Rules are no longer strictly "block" rules. Each rule is a **matching rule** with a configurable action: **Allow** or **Block**. Combined with priority ordering (lower number = higher priority, first match wins), this enables patterns that weren't possible before. For example, a high-priority "Allow" rule can grant a specific user access to `educational-site.com` even when a lower-priority "Block" rule blocks the entire category for everyone else. Exception lists become simply higher-priority Allow rules.
- **Per-user filtering becomes natural** — Since all filtering is rule-scoped, targeting specific users is just a field on the rule. A "kids" rule can block adult content while an "adults" rule allows it. An empty user list means "all users."
- **DNS filtering is now optional** — Domain blocking no longer requires the DNS server. All filtering can be handled entirely by proxy rules. DNS blocking is still available for users who want network-wide domain-level filtering, but it's no longer the only mechanism.
- **Reusable Domain Lists** — Both DNS filtering and proxy rules share the same `DomainListManager` and in-memory index. A single curated list (StevenBlack, Hagezi, etc.) can be referenced by multiple rules and by DNS simultaneously.

### 8-Step Rule Pipeline

Each proxy request is evaluated through a well-defined pipeline, rule by rule in priority order. The first rule that fully matches is applied — subsequent rules are skipped:

1. **Rule status** — Enabled/disabled, active hours schedule
2. **User match** — Does the requesting user match the rule's user list?
3. **Domain match** — Hostname checked against domain patterns (glob wildcards) and domain lists (by list ID)
4. **MITM resolution** — Per-rule MITM setting: `enable`, `disable`, or `default` (fall back to global setting). Determines whether steps 5–7 can inspect HTTPS traffic.
5. **URL regex** — Match against the full request URL (requires MITM for HTTPS)
6. **Content-type** — Match against response Content-Type header (requires MITM for HTTPS)
7. **Keyword filter** — Scan response body for blocked keywords with score watermark (requires MITM for HTTPS)
8. **Action** — **Allow** or **Block**

If no rule matches after evaluating all rules, the request is allowed (default-allow). Because Allow rules participate in the same priority-ordered pipeline as Block rules, administrators can build layered policies: a broad Block rule at priority 100 blocking a category, with targeted Allow exceptions at priority 10 for specific users or domains.

### Domain List System

A new `DomainListManager` provides CRUD operations, an O(1) in-memory index (`map[domain] → set of list IDs`), and support for both local lists and URL-sourced blocklists (StevenBlack, Hagezi, AdGuard, Firebog, etc.). DNS and proxy filtering share the same index. Includes automatic migration from the old blocklist format. Full API endpoints and a management UI at `/domainlists`.

### Device Discovery (Foundation)

A new device discovery system using mDNS/Bonjour browsing and passive DNS observation lays the groundwork for **per-device filtering** in a future release. Devices are identified, tracked, and exposed via API. The data model and record store are in place (with 30+ tests), and the DNS handler already performs passive discovery. The next step would be allowing rules to target devices in addition to users.

### Server-Side Rule Tester

Two new API endpoints (`POST /api/test/rule-match`, `POST /api/test/domain-lookup`) simulate the full 8-step pipeline against a rule definition without affecting live traffic. The rule editor UI has a built-in tester panel with optional live fetch to verify that a rule behaves as expected before saving.

### Real-Time Stats and Logs via SSE

The stats page now uses **Server-Sent Events** for near-real-time updates, replacing the previous 5-second polling approach. DNS request events are emitted from the handler and streamed to the browser as they happen.

### Proxy Hardening

- Streaming 3-path content router (small buffer / scan / stream-through) with configurable `GS_MAX_SCAN_SIZE_MB`
- Graceful shutdown with in-flight request draining
- Proxy loop detection (Via header + X-GateSentry-Loop + XFF depth limit)
- SSRF protection blocking proxy requests to the admin UI
- HTTPS block page delivery (MITM-signed error page for blocked HTTPS requests)
- WebSocket tunnel support
- `TRACE` method blocking, response header sanitization

### DNS Improvements

- Sharded response cache with configurable TTL and negative caching
- TCP query support (large responses > 512 bytes)
- RFC 2136 dynamic DNS update handler
- WPAD/PAC auto-configuration endpoints
- IPv6 listener support
- All settings configurable via environment variables

### UI Overhaul

- Complete Svelte/Carbon rewrite of the rule form matching the 8-step pipeline layout
- New Domain Lists management page (`/domainlists`)
- DNS page with allow/block domain list assignment sections
- Configurable base path support for reverse proxy deployments
- Docker deployment support with `docker-compose.yml` and publish script

### Test Coverage

- 19 rule-tester endpoint unit tests, 19 domain list unit tests
- Comprehensive proxy deep test script (`scripts/proxy_deep_tests.sh`) covering MITM, content filtering, keyword scanning, loop detection, and more

## Breaking Changes from v1

- Admin UI default port: `10786` → `8080` (configurable via `GS_ADMIN_PORT`)
- Rule struct expanded: `domain_patterns`, `domain_lists`, `mitm_action`, `url_regex_patterns`, `blocked_content_types`, `keyword_filter_enabled`, `active_hours`, `users`
- Settings keys added: `dns_domain_lists`, `dns_whitelist_domain_lists`
- Old global `blockedDomains` map replaced by shared `DomainListIndex`

## How to Try It

```bash
git remote add jbarwick https://github.com/jbarwick/Gatesentry.git
git fetch jbarwick v2
git checkout jbarwick/v2
./build.sh && cd bin && ../run.sh
# Admin UI at http://localhost:8080
```

Happy to answer questions, break this into smaller PRs, or adjust anything based on your feedback. I'm continuing to evaluate and test the software and plan to add parental controls in the near future — per-user and per-device content filtering — so additional changes will be coming.

Also — AI filtering is an interesting idea you have, and I'd like to help out on that front. I have some ideas that may or may not work, but I'd be happy to discuss them with you.

Great project — I've really enjoyed working on it.
