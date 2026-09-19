# CHANGELOG

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
