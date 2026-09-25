# CHANGELOG

## v2.4.0 (25 September 2026)

### Administration UI

- Redesigned the sign-in page around the Carbon authentication pattern: a light G10 surface with a branded GateSentry header, extra-large labeled inputs, and one Clear / Sign in action row
- The inspection certificate download is now a de-emphasized ghost action inside the sign-in card instead of a standalone tertiary button, with the same download available in Settings
- Setup, session-checking, and unavailable screens share the same refreshed authentication layout

## v2.3.1 (25 September 2026)

### Fixes

- Login page shows the Clear and Sign in buttons in one row instead of stacked
- Logging out now renders the login page immediately instead of a page-not-found state
- DNS custom records show a single Add record action in the empty state
- Services page uses the standard page width like other routes

## v2.3.0 (25 September 2026)

### Administration UI

- Unified every Svelte administration route around shared Carbon page shells, section panels, resource states, authentication layouts, and confirmations
- Reorganized Settings into focused tabs and rebuilt Policies, Devices, DNS, HTTPS inspection, AI image filtering, Proxy users, Overview, Stats, Logs, and Services with consistent hierarchy and feedback
- Added explicit loading, empty, error, retry, save, and destructive-action states while removing native confirmations and legacy route-local layout patterns
- Made dense tables and editors own their overflow, improved tablet and phone layouts, and fixed the compact navigation drawer intercepting pointer or keyboard input after it closes
- Added a page-not-found recovery state, base-path-aware navigation and certificate downloads, and clearer guidance around HTTPS inspection, external AI processing, proxy users, and device-policy coverage

### Quality

- Expanded focused UI contracts for the application shell, connected settings, setup, policies, devices, DNS/HTTPS inspection, AI, and Proxy users
- Eliminated the accepted Svelte diagnostic baseline: the UI now checks with zero errors and zero warnings
- Rebuilt and validated the embedded administration assets

## v2.2.2 (24 September 2026)

### Performance

- Proxy forwarding now counts transferred bytes without retaining complete downloads in memory, including direct HTTPS tunnels
- Responses use one bounded inspection buffer; known oversized and non-inspectable responses stream directly, while oversized unknown-length responses preserve prefix and remainder order
- Decision logging now uses a bounded queue and batched database transactions with brief backpressure, observable drop and persistence counters, and substantially fewer allocations

### Reliability

- Graceful shutdown drains accepted decision logs and coordinates the web, DNS, transparent proxy, policy, and Bonjour lifecycles
- Response forwarding now preserves bodyless and range semantics, avoids scanning encoded payloads, strips stale representation and hop-by-hop headers, and handles upstream read failures before committing a response
- Added response-boundary, streaming, logger batching, saturation, retry, flush, close, and lifecycle regression coverage

## v2.2.1 (25 September 2026)

### Home

- Redesigned overview: status tiles for protection, devices, blocklist size and HTTPS inspection, a last-24-hours activity panel, setup progress and device connection details
- Device identity uses MAC address first, then DNS hostname, then IPv4 when no stronger identity is available
- Decision logs can show user-defined device names while retaining the client IP, and device-modal activity now refreshes live
- Removed outdated explanatory text and unused instruction components

### Stats

- Stats now come from the decision log, with a 24 hour / 7 day window selector
- New figures: block rate, active clients, distinct domains, inspection errors, requests over time, most requested domains, blocks by policy, and traffic by layer and outcome
- Live proxy traffic reports upload, download, and total bytes handled since GateSentry started
- `/decisions/summary` accepts `?days=N` and reports unique clients and domains, top domains, blocks by policy and an hourly/daily timeline

## v2.2.0 (24 September 2026)

### HTTPS inspection

- The Filters menu is now HTTPS inspection (Blocked keywords, Blocked content types, Sites not inspected), with a warning on each page when inspection is off
- The Block List page is removed; its entries are moved into every policy's blocked domains on upgrade

### Policies

- Policies, devices, and category feeds start with GateSentry rather than with the DNS server, so the proxy enforces a policy's blocked categories even when the DNS server is disabled
- Category lookups take constant time regardless of feed size (about 2µs per proxy decision with a million domains)

## v2.1.0 (24 September 2026)

### Policies page

- Each policy card shows a one-line summary of what it blocks, with rules and devices behind a "Details and devices" toggle
- The policy editor has a Bedtime switch (times and nights) alongside safe search; custom rules, proxy users, and the description are under Advanced, which opens automatically for policies that use them
- Starter policies sit under the page heading; "Test a site" and gateway-wide categories are collapsible

### Fixes

- Saving a device's name, owner, or category from the device details dialog did nothing

## v2.0.0 (24 September 2026)

**Breaking:** the policy document format changes from version 1 to version 2. Existing policies upgrade automatically on first read and keep enforcing what they enforced before; the next policy write persists the new format. The v1 API fields (`action`, `domains`, `categories` on a group, and `mitm_action` on a rule) are replaced by the fields below.

### Policy model

- **One policy per request:** every request resolves to the signed-in proxy user's policy, then the device's assigned policy, then an undeletable Default policy — so unassigned, unknown, and NATed clients always have a policy instead of falling through to nothing
- **Blocked and allowed lists:** a policy has `blocked_categories`, `blocked_domains`, and `allowed_domains` (which win over the blocked lists and the gateway-wide categories)
- **Targeted rules:** each rule names its own target (all traffic, specific domains, or specific categories) and can carry a schedule, a user scope, URL patterns, and response-type conditions — so "block TikTok but allow YouTube" or "no internet at bedtime" is one policy instead of two
- **First-match evaluation:** rules run top to bottom before the lists; the first active rule whose target covers the request decides
- **Safe search:** DNS-enforced safe search for Google (all country domains), Bing, DuckDuckGo, and YouTube restricted mode, using the providers' documented restricted-mode endpoints; fails closed on lookup error
- **Short block TTL:** DNS block answers use a 60-second TTL so schedules and pauses take effect within a minute
- **Proxy user over device:** a signed-in proxy user's policy wins over the device's, because the login is the more specific identity; the device stays recorded for logs and drilldown
- **User conflict detection:** a proxy user listed on two policies is rejected at save time
- **Default policy protection:** the default policy cannot be deleted or given proxy users

### DNS

- Safe search rewrites search engine and YouTube host names to their restricted-mode endpoints via CNAME + upstream resolution
- Gateway-wide categories are now evaluated inside the policy evaluator, so a policy's allowed domains can exempt a category hit on DNS the same way they do on the proxy

### Proxy

- Typed `PolicyDecision` replaces the reflection-based `interface{}` contract between the policy service and the proxy
- URL patterns are tested before the request leaves the gateway, not after fetching the response
- URL patterns skip CONNECT tunnels (which carry only host:port) and are tested per-request inside the decrypted connection instead
- A policy allow skips the gateway-wide URL blocklist and content-type rules
- The block page names the deciding policy and rule
- Transparent HTTP connections are handled by the full proxy handler (with block pages) instead of being silently dropped
- Transparent proxy connections feed the device inventory so per-device policies work on routed deployments where DNS bypasses GateSentry

### Templates

- Starters are useful out of the box: Young child and Teen block categories, force safe search, and add a bedtime rule; Guest blocks adult content and malware; Focused work blocks social media during working hours
- Schedule presets add a rule (block all traffic during the window) instead of setting a removed group-level schedule field

### UI

- Policies page rebuilt: each policy card shows its blocked categories, domains, rules as sentences, assigned devices with add/remove, and safe search status
- Device assignment lives on the policy card with an "Add a device" picker; removing a device tag moves it to the default policy
- "Test a site" panel: pick a device, type a domain or URL, choose DNS or proxy path, and see the verdict, the deciding rule, the evaluator trace, and safe-search status — uses the live evaluator and changes nothing
- Policy editor: blocked categories, blocked domains, always-allowed domains, safe search toggle, proxy users, and ordered rules with their own target (all traffic / categories / domains), schedule, user scope, and collapsible URL/response-type conditions with a proxy-only warning
- Domain input accepts pasted URLs and strips them to the domain
- Device detail shows the effective policy (including the default) instead of reporting no policy when unassigned
- `/policies` registered as a server-side SPA route (fixes 404 on reload)

### Upgrade notes

- The policy document upgrades from v1 to v2 on first read; the old `action`/`domains`/`categories` fields become `blocked_domains`/`blocked_categories`/`allowed_domains` and rules get explicit targets
- A v1 group with a schedule becomes a v2 group with a scheduled rule
- The per-rule TLS inspection setting (`mitm_action`) is removed; URL and response-type conditions turn inspection on by themselves
- API consumers using `action`, `domains`, or `categories` on a policy group must switch to the new fields

## v1.27.0 (20 September 2026)

- Policy rules now belong to a policy group: a group owns its categories, domains, and rules, and one evaluator decides for both DNS and the proxy. A rule is no longer a separate record that has to be matched to a group by hand
- Rule order is evaluation order, so the first matching rule decides; a group rule with no match falls back to the group's own action, and a disabled rule is ignored
- URL and response-type conditions on a rule are enforced by the proxy, which is what a rule that turns on TLS inspection reaches; the group's categories and domains stay the DNS-enforced part of the same policy
- Existing standalone rules migrate into one unassigned policy group each, so an upgrade keeps every rule and its conditions
- The `/api/rules` endpoints are removed; rules are read and written with their group through `/api/policy/groups`
- Self-updating domain categories: blocklist categories can be selected gateway-wide or per group, with the downloaded domain count shown for each category
- Policy groups are assigned from the policies page itself, and each group states the devices it is enforced on
- Policy template starters state their shared caveat once for the whole catalog instead of repeating the same text in every tile, and name the proxy path instead of a removed advanced rule editor
- Policies page rebuilt on Carbon's grid around a single group form; the standalone advanced rule editor is gone
- First-run setup no longer asks for a bootstrap authorization code
- Setup shows the password byte limit as the field's own error rather than as a separate message
- Dashboard rebuilt as a Carbon status page

## v1.26.1 (19 September 2026)

- Fixed GateSentry failing to start on Windows: Unix file permission checks on the installation key and bootstrap secret files always rejected files on Windows where NTFS ACLs don't map to Unix permission bits
- Fixed directory fsync failures on Windows: directory handle syncing is now skipped on Windows where it returns "Access is denied"
- Fixed restore endpoint returning invalid JSON on Windows: recovery point paths with backslashes are now properly JSON-escaped via json.Encoder instead of string concatenation
- Proxy smoke test now runs on Windows CI alongside Linux, catching Windows-specific failures before merge

## v1.26.0 (19 September 2026)

- Filtering explanations: block reasons surfaced with matched rule, layer, and policy provenance; device and policy drilldown views explain why a request was allowed or blocked
- Scoped domain exceptions and public access requests: per-device or per-group exceptions with an audit trail, plus time-bound public access requests
- Weekly schedules and temporary pauses: recurring weekly schedule windows for policy groups and on-demand filtering pauses with automatic resumption
- Policy preview: simulate policy decisions against proposed changes without affecting live traffic
- Validated backup and restore with rollback: export and restore configuration with integrity validation and automatic rollback on failure
- Gateway diagnostics and redacted support bundle: health checks for DNS, proxy, and storage plus a redacted support bundle that excludes credentials and private data
- Docs: moved the reproducible build section to the end of the README

## v1.25.0 (18 September 2026)

- Structured filtering decisions: one canonical Decision type emitted at every enforcement point (DNS, explicit proxy, transparent proxy, content inspection)
- Decision carries action (allow/block/inspect/bypass/error/unknown), matched rule, reason, layer, domain, URL, client IP, device/group context, source, policy revision, and timestamp
- Backward-compatible logging: legacy LogDNS/LogProxy still work; LogDecision writes adapter-native response types into legacy fields so stats and device-activity views are unchanged
- Editable policy starter templates: child, teen, adult/default, guest, work, IoT, and unrestricted profiles with honest protections and limitations preview
- Atomic per-group create, update, and delete operations; safe template reapplication returns conflict instead of overwriting edits
- Assignment cleanup when policy groups are deleted
- Rules page template cards and editable group management UI

## v1.24.0 (17 September 2026)

- Explicit policy groups with consistent DNS and proxy evaluation
- Per-device policy assignment, effective rules view, and activity history endpoints
- Device detail UI with effective rules, coverage confidence, caveats, identity, shared/stale address flags, and decision history
- Device-scoped activity history with client IPs recorded in DNS logs
- Commit identity protection: AGENTS.md rules, pre-commit hook, CI workflow, and make verify integration
- Reproducible release builds with multi-platform cross-compilation
- Docker quickstart smoke test as gated PR check


## v1.23.0 (17 May 2026)

- WebSocket proxy support: ws:// connections now properly proxied via TCP tunnel
- (was rejecting all WebSocket connections with "not supported" error)

## v1.22.0 (17 May 2026)

- Fixed static file serving: restored CSS/JS assets, added MIME types, fixed vite.svg path
- Restored block page CSS and JS deleted by frontend rebuild

## v1.21.0 (17 May 2026)

- Device discovery service: mDNS/Bonjour browser, passive DNS observation, RFC 2136 DDNS UPDATE support
- Device store with multi-zone DNS records (A/AAAA/PTR), thread-safe with sync.RWMutex
- Device inventory UI: devices list, device detail, naming and owner assignment
- Admin UI port configurable via GS_ADMIN_PORT env var
- Base path support for reverse proxy deployments (GS_BASE_PATH)

## v1.20.7 (17 May 2026)

- DNS server: replaced global `sync.Mutex` with `sync.RWMutex`, mutex released before upstream forwarding
- DNS server: added TCP support alongside UDP for queries exceeding 512 bytes
- DNS server: `GATESENTRY_DNS_RESOLVER` env var now overrides stored settings
- Fixed startup panic when DNS blocklists contain whitespace-only lines
- Added pytest integration test suite (46 tests covering proxy, MITM, DNS, API, UI)
- CI: integration tests run as gated PR check
- Fixed Codecov coverage collection (was reporting 0%)
- Added coverage-int target: instrumentation-based integration coverage
- Rewrote AI-generated repo description and README
- Removed stale AI-generated documentation files

## v1.20.6 (31 January 2026)

- Fix HTTP2 not working properly in transparent mode

## v1.20.5 (31 January 2026)

- Fixed HTTP/2 compatibility issue with MITM connections in transparent proxy mode
- MITM connections now bypass early SNI extraction to avoid connection wrapper issues

## v1.20.4 (31 January 2026)

- Improved error handling: fall back to direct tunnel instead of closing connection when ClientHello parsing fails
- Added diagnostic logging for SNI extraction failures

## v1.20.3 (31 January 2026)

- Fixed connection handling in transparent proxy to properly forward TLS handshake data

## v1.20.2 (31 January 2026)

- Fixed multiple SNI parsing bugs in TLS Client Hello parser (length calculation and data extraction)

## v1.20.1 (31 January 2026)

- Fix transparent HTTPS mode showing IP addresses instead of domain names
- Fixed SNI extraction to enable domain-based rule matching in transparent proxy mode

## v1.20.0 (31 January 2026)

- Fix transparent mode not picking up rules or logging traffic.

## v1.19.3 (31 January 2026)

- Added TPROXY support for transparent proxy mode (enables handling forwarded traffic from Tailscale exit nodes and routers)
- Added IP_TRANSPARENT socket option for transparent proxy listener
- Auto-detects Linux and enables transparent proxy by default with graceful fallback
- Platform-specific builds: transparent proxy code excluded on macOS/Windows

## v1.19.0 (30 January 2026)

- Add support for transparent proxying on linux.

## v1.18.1 (28 January 2026)

- Fix regex matching bug in rule based blocking system.

## v1.18.0 (26 January 2026)

- Added rule-based filtering system with domain-specific SSL inspection control
- Added ability to block specific content types or URL patterns per domain
- Added support for custom upstream DNS resolver configuration
- Performance enhancements and code optimizations

## v1.17.4 (25 February 2025)

- Updated expired MITM certificate with 2 year expiry. 
- Fixed bug causing a user created certificate not being saved + fixed restart after certificate update.

## v1.17.3 (22nd October 2023)

- Fix UI bug in the DNS page causing the user unable to modify domains

## v1.17.2 (22nd October 2023)

- Add support for running on Docker

## v1.17.1 (22nd October 2023)

- Fix: DNS Server blocklist in default settings

## v1.17 (22nd October 2023)

- Fix: DNS Server not updating list immediately after an update from the UI
- Added link to download certificate on the login screen
- Added DNS server info page which shows total blocked domains, last update time and next scheduled update for blocklists
- Added filtering strictness field to the UI
- Fixed bug : Missing mapping of exception hosts in the Web UI
- Fixed bug : AVIF files were not being properly displayed in Firefox
- Refactored code
- Now the content type filter also blocks content immediately by guessing the file type from the URL, this can be helpful in terms of saving bandwidth. Previously, we would send the request to the server and block content based upon the response MIME type
- Added tests for basic functionality and code coverage
- Fixed blocked responses for some cases, where if we detected blocked content we would simply terminate the connection, now we send a proper blocked page
- Added support for sending an image with the text BLOCKED for blocked images.
- Introduced the CHANGELOG
