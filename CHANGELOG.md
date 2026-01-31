# CHANGELOG

## v1.20.4 (31 January 2026)

- Fix transparent HTTPS mode showing IP addresses instead of domain names
- Fixed SNI extraction to enable domain-based rule matching in transparent proxy mode
- Fixed multiple SNI parsing bugs in TLS Client Hello parser (length calculation and data extraction)
- Fixed connection handling in transparent proxy to properly forward TLS handshake data
- Improved error handling: fall back to direct tunnel instead of closing connection when ClientHello parsing fails
- Added diagnostic logging for SNI extraction failures

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
