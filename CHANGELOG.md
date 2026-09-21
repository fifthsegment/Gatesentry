# CHANGELOG

## v2.0.0-beta.21 (21 September 2026)

- Stats: 7-day chart is hourly; 24h/1h rebucket the same compact series in the browser
- Request stats no longer full-scan `log.db` on every page load (minute rollups)

## v2.0.0-beta.20 (10 September 2026)

- DNS page: TSIG shared-secret copy works on HTTP admin (clipboard fallback)

## v2.0.0-beta.19 (10 September 2026)

- AI page: editable Grok, ChatGPT, and Ollama model ids

## v2.0.0-beta.18 (10 September 2026)

- AI page status badges re-render when probes finish (Svelte reactivity)

## v2.0.0-beta.17 (10 September 2026)

- AI status uses server probe as source of truth; empty fields stay Not configured
- Radio save no longer posts the old value when the AI page re-renders

## v2.0.0-beta.16 (10 September 2026)

- AI status: empty fields show Not configured immediately; probes go direct (no HTTP proxy, no GateSentry DNS)
- Grok/OpenAI API hosts are not filtered if the probe is intercepted by the proxy

## v2.0.0-beta.15 (10 September 2026)

- AI page: reachability status on each config card (Grok, ChatGPT, Ollama, legacy scanner)

## v2.0.0-beta.14 (10 September 2026)

- AI page Alpha badge is red

## v2.0.0-beta.13 (10 September 2026)

- AI page (Alpha): Grok / ChatGPT / Local LLM radios, API keys, Ollama URL
- Local LLM URL stored as `ai_local_llm_url` (not used on the request path yet)

## v2.0.0-beta.12 (9 September 2026)

- Device details tabs keep a fixed height so the dialog does not resize when switching tabs

## v2.0.0-beta.11 (9 September 2026)

- Device details dialog: Reachability, DNS queries, Identity, and Discovery on tabs
- Compact field layout; Ping now updates online/offline in the open dialog

## v2.0.0-beta.10 (9 September 2026)

- Stats "Past 7 days" scans the full window (not the newest 20k log lines)
- DNS queries are logged again for charts; stats aggregate during the scan
- Add A/AAAA Save enables when name plus IPv4 or IPv6 is valid

## v2.0.0-beta.9 (9 September 2026)

- Custom A/AAAA records on the DNS Server tab (IPv4, IPv6, or both)
- DNS query path: custom records first, then filters, then cache/upstream
- Lock-free custom-record snapshot; AAAA answers packed as 16-byte IPv6
- Cache positive TTLs from answer/glue (not SOA); remaining TTL served on hits

## v2.0.0-beta.8 (9 September 2026)

- Admin HTTPS on `GS_ADMIN_PORT_SSL` (default 9877), enabled from Settings above MITM
- Upload server certificate (and key) plus a separate admin CA; not the MITM CA unless you paste the same PEM

## v2.0.0-beta.7 (9 September 2026)

- Ignore this process's own Bonjour advertisements so they are not applied as another device's name
- Strip those ads from persisted devices whose address is not local
- Devices API/list sorted by display name

## v2.0.0-beta.6 (9 September 2026)

- Hostname ownership prefers a bare DNS label over the same name with `.local`, so a stolen alias is not treated as the owner

## v2.0.0-beta.5 (9 September 2026)

- Device names: do not fuse two named devices when mDNS/DDNS mixes a name with another host's address
- Extra hostnames and mDNS aliases expire (7-day TTL); unstamped legacy aliases drop on load
- IPs are runtime values only — no site LAN in product defaults, UI samples, or tests (192.0.2.0/24, 2001:db8::/32)
- Default IPv6 resolver is empty; `run.sh` / `restart.sh` no longer force a site recursive DNS

## v2.0.0-beta.4 (9 September 2026)

- Devices list: query tag is last DNS request the device sent to GateSentry (`No queries` / `Queried …`), not a missing DNS record
- Ship pipeline: `./build.sh` (binaries), `./release.sh` (image + Nexus), `./deploy.sh` (NAS)
- `release.sh` logs into Nexus with `NEXUS_TOKEN` when set (needed for some IP endpoints), otherwise `NEXUS_USERNAME` / `NEXUS_PASSWORD`

## v2.0.0-beta.3 (9 September 2026)

- Devices page: online/offline is ICMP ping on page open (and refresh), not last DNS query
- Devices detail dialog: separate reachability (ping, RTT) and DNS activity; Ping now
- `POST /api/devices/probe` and `POST /api/devices/{id}/probe`; last DNS query is tracked separately
- Runtime image includes `iputils` so ping can fall back to the system ping binary
- Public install docs: README now matches how v2 is actually built and run
- Sample `docker-compose.yml` (bridged 8080/10053) and `docker-compose.host.yml` (Linux host network, DNS 53)
- Dockerfile/compose notes: quote `GATESENTRY_DNS_ADDR=0.0.0.0,::`; do not set `GATESENTRY_DNS_RESOLVER` unless you want it to overwrite stored settings

## v2.0.0-beta.2 (2 September 2026)

- Dual-stack DNS listen (`0.0.0.0` plus `::`); AAAA answers for appliance/WPAD/blocked names
- Separate IPv6 upstream resolver (`dns_resolver_ipv6`, default `[2001:db8::1]:53`); AAAA/HTTPS/ip6.arpa use it
- Generate fallback blocked image when `blocked.jpg` is missing
- Bonjour advertises using the LAN IPv4 instead of hostname lookup
- Timezone follows container `TZ` (Asia/Singapore on monster-jj)
- Quiet IPv6 "network is unreachable" proxy log spam

## v2.0.0-beta.1 (2 September 2026)

Control-plane availability when upstream DNS is down (monster-jj hang):

- Negative-cache SERVFAIL/timeouts; cap in-flight upstream queries; circuit-break a dead resolver
- `MapStore` no longer deadlocks on unmarshal failure; Get is locked; Init reloads in place
- DNS hot path no longer logs every query or spawns a buntdb writer per request
- Admin Host allowlist includes `hostname.<zone>` and `GS_ADMIN_HOSTS`; appliance names answered locally
- Admin HTTP server timeouts; authenticated `/api/logs/{id}`; real `Stop()`
- In-process HTTP clients ignore `HTTP_PROXY`; `InitProxy` no longer resets `GS_MAX_SCAN_SIZE_MB`
- DDNS defaults off for new installs; stats default window 24h; domain-list parent matching

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
