# GateSentry Proxy Service — Hardening & Architecture Update Plan

**Author:** @jbarwick
**Date:** February 10, 2026
**Branch:** `feature/proxy-hardening` (cumulative from `feature/docker-publish`)
**Scope:** `gatesentryproxy/` package (3,074 LOC, 18 files)

---

## Executive Summary

A comprehensive, automated test suite (96 tests across 15 sections) was built to
evaluate the GateSentry proxy against real-world HTTP semantics, RFC compliance,
and adversarial attack patterns inspired by 55 published Squid CVEs and 35
unfixed Squid 0-days.

**Pre-hardening: 75 PASS · 3 FAIL · 17 KNOWN ISSUES · 1 SKIP**
**After Phase 1: 81 PASS · 2 FAIL · 13 KNOWN ISSUES · 1 SKIP**
**After Phase 2: 84 PASS · 2 FAIL · 10 KNOWN ISSUES · 1 SKIP**

The good news: the proxy is fundamentally sound — it survived every CVE-inspired
attack pattern that killed Squid, including chunked-extension stack overflow
(CVE-2024-25111), Vary:Other assertion crash (CVE-2021-28662), unsolicited
100-Continue barrage (Squid unfixed 0day), and 5,000-entry X-Forwarded-For
overflow (CVE-2023-50269).

The issues found are **architectural** — they share a small number of root causes
and can be fixed in phases without rewriting the proxy. This document proposes a
5-phase plan where each phase is independently testable, deployable, and
mergeable.

---

## Table of Contents

1. [Test Infrastructure](#1-test-infrastructure)
2. [Full Test Results](#2-full-test-results)
3. [Architecture Analysis](#3-architecture-analysis)
4. [Root-Cause Clusters](#4-root-cause-clusters)
5. [Remediation Phases](#5-remediation-phases)
6. [Phase 1 — Response Header Sanitization](#6-phase-1--response-header-sanitization)
7. [Phase 2 — DNS & SSRF Hardening](#7-phase-2--dns--ssrf-hardening)
8. [Phase 3 — Streaming Response Pipeline](#8-phase-3--streaming-response-pipeline)
9. [Phase 4 — WebSocket & Protocol Support](#9-phase-4--websocket--protocol-support)
10. [Phase 5 — Content Scanning Hardening](#10-phase-5--content-scanning-hardening)
11. [Implementation Checklist](#11-implementation-checklist)
12. [Risk Assessment](#12-risk-assessment)
13. [Testing Strategy](#13-testing-strategy)
14. [Legacy Code Cleanup](#14-legacy-code-cleanup--applicationproxy)

---

## 1. Test Infrastructure

All testing is **100% local** with zero internet dependency. The testbed
simulates a hostile internet using protocol-level misbehaviour endpoints.

### Components

| Component | Port | Purpose |
|-----------|------|---------|
| GateSentry DNS | 10053 | DNS resolver under test |
| GateSentry Proxy | 10413 | HTTP proxy under test |
| GateSentry Admin | 8080 | Admin UI |
| nginx (HTTP) | 9999 | Static files, reverse proxy to echo server |
| nginx (HTTPS) | 9443 | TLS termination with internal CA |
| Echo Server | 9998 | Python HTTP server with 41 hostile endpoints |

### Echo Server Endpoint Categories

| Category | Count | Purpose |
|----------|-------|---------|
| Standard | 17 | /echo, /headers, /get, /post, /status/\<code\>, /delay, /stream, etc. |
| Adversarial | 25 | Protocol-level misbehaviour: lying Content-Length, mid-stream drops, response splitting, gzip bombs, slow bodies, HTTP/0.9, etc. |
| CVE-Inspired | 10 | Attack patterns from Squid CVEs: Vary:Other, 100-Continue, chunked extensions, range overflow, cache poisoning, TRACE reflection, etc. |

### Test Suite Structure

| Section | Tests | Description |
|---------|-------|-------------|
| §1 | 6 | DNS Functionality (A, AAAA, MX, TXT, NXDOMAIN, TTL) |
| §2 | 2 | DNS Caching |
| §3 | 7 | Proxy RFC Compliance (Via, XFF, hop-by-hop, HEAD, OPTIONS, C-L, Accept-Encoding) |
| §4 | 6 | HTTP Method Support (GET, POST, PUT, DELETE, PATCH, HEAD) |
| §5 | 2 | HTTPS / CONNECT Tunnel |
| §6 | 1 | WebSocket Support |
| §7 | 5 | Proxy Security (SSRF, host injection, loop detection, oversized headers) |
| §8 | 1 | Proxy DNS Resolution Path |
| §9 | 4 | Performance Benchmarks (DNS latency, DNS QPS, proxy throughput, large response) |
| §10 | 2 | Concurrent Requests (DNS 20-parallel, proxy 10-parallel) |
| §11 | 5 | Large File Downloads (1MB, 10MB, 100MB, TTFB, integrity) |
| §12 | 4 | Streaming & Chunked Transfer (chunked, SSE, drip timing, 1MB chunked) |
| §13 | 4 | HTTP Range Requests (partial, resume, multi-range) |
| §14 | 2 | Memory & Resource Behaviour |
| §15 | 35 | **Adversarial Resilience & CVE Tests** |
| | **96** | **Total** |

---

## 2. Full Test Results

*Run date: February 10, 2026 — GateSentry commit `3224cff`*

### Summary

```
  PASS:        84  (includes 3 handled by Go's net/http client)
  FAIL:        2   (§11.2 10MB truncation, §12.3 drip timing)
  KNOWN ISSUE: 10  (architectural limitations documented below)
  SKIPPED:     1
  TOTAL:       97
```

*Phase 1: §3.1 Via header, §7.4 loop detection, §3.6 Content-Length — moved from KNOWN/FAIL → PASS*
*Phase 2: §8.1 DNS resolution, §7.1 SSRF admin, §7.2 SSRF localhost — moved from KNOWN → PASS*

### All Results by Section

#### Pre-Flight (6/6 PASS)
| # | Test | Result |
|---|------|--------|
| 0.1 | DNS server reachable | ✅ PASS |
| 0.2 | HTTP proxy reachable | ✅ PASS |
| 0.3 | Admin UI reachable | ✅ PASS |
| 0.4 | Testbed HTTP ready | ✅ PASS |
| 0.5 | Testbed HTTPS ready | ✅ PASS |
| 0.6 | Echo server ready | ✅ PASS |

#### §1 DNS Functionality (5/6 PASS, 1 KNOWN)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 1.1 | A-record resolution | ✅ PASS | |
| 1.2 | AAAA-record resolution | ✅ PASS | |
| 1.3 | MX-record resolution | ✅ PASS | |
| 1.4 | TXT-record resolution | ✅ PASS | |
| 1.5 | NXDOMAIN handling | ⚠️ KNOWN | Returns NOERROR with 0 answers instead of NXDOMAIN rcode |
| 1.6 | TTL in DNS responses | ✅ PASS | |

#### §2 DNS Caching (1/2 PASS, 1 KNOWN)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 2.1 | Cache hit (repeated query faster) | ⚠️ KNOWN | No caching — every query hits upstream |
| 2.2 | TTL decrement | ✅ PASS | |

#### §3 Proxy RFC Compliance (3/7 PASS, 4 KNOWN)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 3.1 | Via header (RFC 7230 §5.7.1) | ⚠️ KNOWN | No Via header added |
| 3.2 | X-Forwarded-For | ✅ PASS | |
| 3.3 | Hop-by-hop header removal | ✅ PASS | |
| 3.4 | HEAD method (3s timeout) | ✅ PASS | 3/3 attempts |
| 3.5 | OPTIONS method | ✅ PASS | |
| 3.6 | Content-Length accuracy | ⚠️ KNOWN | C-L is 0 but body has 885 bytes |
| 3.7 | Accept-Encoding handling | ⚠️ KNOWN | Stripped unconditionally |

#### §4 HTTP Methods (6/6 PASS)
| # | Test | Result |
|---|------|--------|
| 4.1–4.6 | GET, POST, PUT, DELETE, PATCH, HEAD | ✅ PASS (all) |

#### §5 HTTPS / CONNECT (2/2 PASS)
| # | Test | Result |
|---|------|--------|
| 5.1 | CONNECT tunnel basic | ✅ PASS |
| 5.2 | CONNECT to non-standard port | ✅ PASS |

#### §6 WebSocket (0/1 PASS, 1 KNOWN)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 6.1 | WebSocket upgrade | ⚠️ KNOWN | Returns 400 "not supported" |

#### §7 Proxy Security (2/5 PASS, 2 KNOWN, 1 FAIL)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 7.1 | SSRF — admin UI via proxy | ⚠️ KNOWN | Proxy allows access to 127.0.0.1:8080 |
| 7.2 | SSRF — localhost by name | ⚠️ KNOWN | 'localhost:8080' accessible |
| 7.3 | Host header injection | ✅ PASS | |
| 7.4 | Proxy loop / self-request | ❌ FAIL | 5440ms hang — no loop detection |
| 7.5 | Oversized header handling | ✅ PASS | Returns 400 |

#### §8 Proxy DNS Resolution (0/1 PASS, 1 KNOWN)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 8.1 | Proxy uses GateSentry DNS | ⚠️ KNOWN | Uses system DNS, bypasses filtering |

#### §9 Performance (4/4 PASS)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 9.1 | DNS latency | ✅ PASS | avg 33ms |
| 9.2 | DNS throughput | ✅ PASS | 7,539 QPS |
| 9.3 | Proxy throughput | ✅ PASS | 1,656 req/s |
| 9.4 | Large response passthrough | ✅ PASS | 1MB in 19ms |

#### §10 Concurrent Requests (2/2 PASS)
| # | Test | Result |
|---|------|--------|
| 10.1 | 20 parallel DNS queries | ✅ PASS |
| 10.2 | 10 parallel proxy requests | ✅ PASS |

#### §11 Large Downloads (5/5 PASS)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 11.1 | 1MB download | ✅ PASS | 54.9 MB/s |
| 11.2 | 10MB download | ✅ PASS | 50.7 MB/s |
| 11.3 | 100MB download | ✅ PASS | 78.5 MB/s |
| 11.4 | TTFB (time-to-first-byte) | ✅ PASS | 73ms (79x direct) |
| 11.5 | Download integrity (checksum) | ✅ PASS | MD5 match |

#### §12 Streaming (2/4 PASS, 1 FAIL, 1 SKIP)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 12.1 | Chunked Transfer-Encoding | ✅ PASS | 17 lines received |
| 12.2 | SSE (time-to-first-event) | ⊘ SKIP | Inconclusive |
| 12.3 | Drip timing | ❌ FAIL | 2.2s (timing distortion on LAN) |
| 12.4 | Large chunked (100 chunks) | ✅ PASS | 1025KB in 19ms |

#### §13 Range Requests (4/4 PASS)
| # | Test | Result |
|---|------|--------|
| 13.1 | First 1024 bytes | ✅ PASS |
| 13.2 | Range body size | ✅ PASS |
| 13.3 | Mid-file resume | ✅ PASS |
| 13.4 | Multi-range | ✅ PASS |

#### §14 Memory & Resources (1/2 PASS, 1 KNOWN)
| # | Test | Result | Notes |
|---|------|--------|-------|
| 14.1 | MaxContentScanSize impact | ⚠️ KNOWN | 10MB RAM per response buffered |
| 14.2 | Connection cleanup | ✅ PASS | Connections cleaned after load |

#### §15 Adversarial Resilience & CVE Tests (29/35 PASS, 5 KNOWN, 1 FAIL)

| # | Test | Result | Notes |
|---|------|--------|-------|
| 15.1 | HEAD with illegal body | ✅ PASS | Body stripped |
| 15.2 | Lying C-L (under: claims 1000, sends 50) | ✅ PASS | No hang |
| 15.3 | Lying C-L (over: claims 10, sends 500) | ✅ PASS | Truncated to 10 bytes |
| 15.4 | Connection drop mid-stream | ⚠️ KNOWN | Returns 200 with partial data |
| 15.5 | Mixed C-L + chunked (smuggling vector) | ✅ PASS | |
| 15.6 | Gzip body, no C-E header | ✅ PASS | |
| 15.7 | Double-gzip, single C-E | ✅ PASS | |
| 15.8 | No framing (connection close) | ✅ PASS | 614 bytes delivered |
| 15.9 | SSRF redirect to localhost | ✅ PASS | 302 forwarded, not followed |
| 15.10 | Null bytes in headers | ✅ PASS | *Handled by Go HTTP client — rejects at RoundTrip level |
| 15.11 | Huge header (64KB) | ✅ PASS | |
| 15.12 | Double Content-Length | ✅ PASS | *Handled by Go HTTP client — rejects conflicting C-L per RFC 9110 §8.6 |
| 15.13 | Premature chunked EOF | ⚠️ KNOWN | Returns 200 for incomplete stream |
| 15.14 | Negative Content-Length (-1) | ✅ PASS | *Handled by Go HTTP client — rejects negative C-L at RoundTrip level |
| 15.15 | Non-standard status reason | ✅ PASS | |
| 15.16 | Chunked trailer injection | ✅ PASS | |
| 15.17 | Slow body (3s drip) | ✅ PASS | |
| 15.18 | Gzip bomb (1KB → 1MB) | ✅ PASS | |
| 15.19 | HTTP response splitting | ⚠️ KNOWN | Inherent HTTP-level proxy limitation — Go HTTP client parses \r\n as header delimiter; injected headers indistinguishable from legitimate |
| 15.20 | Keep-alive desync | ✅ PASS | Survived + recovered |
| 15.20b | Recovery after desync | ✅ PASS | |
| 15.21 | CVE-2021-28662 — Vary: Other | ✅ PASS | No crash (killed Squid) |
| 15.22 | Squid 0day — 100-Continue | ✅ PASS | |
| 15.23 | 10x 100-Continue barrage | ✅ PASS | |
| 15.24 | CVE-2024-25111 — chunked extensions | ✅ PASS | 41KB delivered (killed Squid) |
| 15.25 | CVE-2021-31808 — range overflow | ✅ PASS | |
| 15.26 | CVE-2021-33620 — bad Content-Range | ✅ PASS | |
| 15.27 | CVE-2023-50269 — XFF overflow | ✅ PASS | 5000-entry header handled |
| 15.28 | CVE-2023-5824 — cache poison | ✅ PASS | Not cached |
| 15.28b | Cache poison follow-up | ✅ PASS | Clean |
| 15.29 | CVE-2023-49288 — TRACE reflection | ⚠️ KNOWN | Cookies visible in response body |
| 15.30 | 1000x Set-Cookie headers | ✅ PASS | |
| 15.31 | Wrong Content-Type (XSS) | ⚠️ KNOWN | `<script>` forwarded in mistyped response |
| 15.32 | Range ignored (200 not 206) | ✅ PASS | |
| 15.33 | SSRF redirect chain | ✅ PASS | |
| 15.34 | Rapid-fire resilience (14 endpoints) | ✅ PASS | 14/14 |
| 15.35 | Proxy survived all adversarial tests | ✅ PASS | Still responding |

### CVE Survival Scorecard

The proxy was tested against attack patterns from published Squid
vulnerabilities. These are patterns that **crashed, hung, or compromised**
Squid — the most widely deployed HTTP proxy in the world.

| CVE | Squid Impact | GateSentry Result |
|-----|-------------|-------------------|
| CVE-2021-28662 | Assertion crash (DoS) | ✅ SURVIVED |
| CVE-2024-25111 | Stack overflow (DoS/RCE) | ✅ SURVIVED |
| CVE-2021-31808 | Integer overflow (DoS) | ✅ SURVIVED |
| CVE-2021-33620 | Crash (DoS) | ✅ SURVIVED |
| CVE-2023-50269 | Stack overflow (DoS) CVSS 8.6 | ✅ SURVIVED |
| CVE-2023-5824 | Cache poisoning (CVSS 7.5) | ✅ SURVIVED |
| CVE-2023-49288 | Credential leak (CVSS 8.6) | ⚠️ Reflects cookies |
| Unfixed 0day | 100-Continue crash | ✅ SURVIVED |

**GateSentry's Go runtime provides natural immunity to the memory corruption
class of bugs that plague C-based proxies.** The remaining issues are logical,
not memory-safety.

---

## 3. Architecture Analysis

### Current Request Pipeline

```
Client Request
     │
     ▼
ServeHTTP()                          ← proxy.go:165
     │
     ├── Transparent proxy detection  ← proxy.go:172-186
     ├── URL length check (>10000)    ← proxy.go:191
     ├── Scheme/Host inference        ← proxy.go:198-212
     ├── Auth check                   ← proxy.go:218-248
     ├── Time-based access filter     ← proxy.go:267
     ├── CONNECT host parsing         ← proxy.go:275-283
     ├── URL access filter            ← proxy.go:288-295
     ├── Content-Type filter (by ext) ← proxy.go:297-315
     ├── Rule matching                ← proxy.go:323-338
     │
     ├── CONNECT + SSL Bump?    ────→ HandleSSLBump()     ← ssl.go
     ├── CONNECT + Direct?      ────→ HandleSSLConnectDirect()
     ├── WebSocket?             ────→ HandleWebsocketConnection()  ← returns 400
     │
     ├── XFF loop check (≥10)        ← proxy.go:388
     ├── Accept-Encoding DELETE      ← proxy.go:396 ⚠️ unconditional
     │
     ▼
rt.RoundTrip(r)                      ← proxy.go:412
     │
     ├── Post-response URL regex     ← proxy.go:423-459
     ├── Content-Type filter (resp)  ← proxy.go:468-479
     │
     ▼
io.ReadAll(teeReader)                ← proxy.go:484-489 ⚠️ UP TO 10MB IN RAM
     │
     ├── limitedReader.N == 0?  ────→ Streaming passthrough (>10MB)
     │                                But: sets gzip BEFORE copyResponseHeader
     │                                     then sets gzip AGAIN after io.Copy ⚠️
     │
     ├── filetype.Match()            ← proxy.go:504 (needs first ~262 bytes)
     ├── ScanMedia()                 ← contentscanner.go (image/video/audio)
     ├── ScanText()                  ← contentscanner.go (HTML content)
     │
     ▼
Write to client                      ← proxy.go:521-531
     ├── gzipOK && len > 1000?  ────→ gzip.NewWriter(w), re-compress
     └── else                   ────→ Set Content-Length, write raw
```

### Key Files

| File | Lines | Responsibility |
|------|-------|----------------|
| `proxy.go` | 720 | Main request handler, header management, response pipeline |
| `ssl.go` | 531 | CONNECT tunnel, SSL bump (MITM), certificate handling |
| `contentscanner.go` | 126 | Media & text content scanning |
| `transparent.go` | 190 | Transparent proxy support |
| `transparent_listener.go` | 437 | Transparent proxy listener |
| `certificates.go` | 196 | Certificate generation/signing |
| `image.go` | 162 | Block page image generation |
| `utils.go` | 110 | LAN detection, MIME helpers |
| `types.go` | 97 | All type definitions |
| `bufferpool.go` | 71 | sync.Pool buffer management |
| `constants.go` | 50 | Action constants, hop-by-hop list |
| `auth.go` | 54 | Proxy authentication |
| `websocket.go` | 8 | Stub — returns 400 |

---

## 4. Root-Cause Clusters

Every test failure traces back to one of five architectural root causes. The
key insight is: **fixing at the cluster level eliminates multiple symptoms at
once, while patching symptoms individually creates fragile special cases.**

### Cluster A: Buffer-Everything Response Pipeline

**Root cause:** `proxy.go:484-489` — `io.ReadAll(teeReader)` buffers the entire
response body (up to 10MB) in RAM before any byte reaches the client.

**Why it exists:** The content scanner (`ScanMedia` / `ScanText`) needs the full
body to inspect. The scanner was designed as a batch operation, not a stream
processor.

**Symptoms (8 tests):**
| Test | Symptom |
|------|---------|
| §14.1 | 10MB RAM per response |
| §11.4 | High TTFB (73ms vs <1ms direct) |
| §12.2 | SSE/streaming broken (no flush) |
| §12.3 | Drip timing distorted (body held until complete) |
| §15.4 | 200 returned on mid-stream drop (partial data already buffered) |
| §15.13 | 200 for incomplete chunked (chunks already buffered) |
| §15.18 | Gzip bomb fully decompressed to 1MB in RAM |
| §3.7 | Accept-Encoding stripped (proxy re-compresses from scratch) |

**The fix:** Replace the single buffer path with a 3-way router:
1. **Stream-passthrough** — for content types that don't need scanning (JS, CSS, fonts, video, audio, binary)
2. **Peek-and-forward** — read first 4KB for `filetype.Match`, then stream the rest
3. **Buffer-and-scan** — only for `text/html` and image types that match content filter rules

### Cluster B: Response Header Handling

**Root cause:** `copyResponseHeader()` at `proxy.go:700-712` copies all response
headers verbatim (except `Content-Length`) with **zero validation or
sanitization**.

**Why it exists:** The original implementation trusts upstream servers to send
well-formed headers. In a home environment where the proxy faces the open
internet, this trust is misplaced.

**Symptoms (10 tests):**
| Test | Symptom | Severity |
|------|---------|----------|
| §15.19 | Response splitting — injected `Set-Cookie: evil=stolen` forwarded | **CRITICAL** |
| §15.10 | Null bytes in header values forwarded | HIGH |
| §15.12 | Double Content-Length forwarded (RFC says reject) | HIGH |
| §15.14 | Negative Content-Length accepted | HIGH |
| §15.29 | TRACE reflection — cookies visible in response body | MEDIUM |
| §15.31 | XSS in mistyped Content-Type forwarded | MEDIUM |
| §3.1 | No Via header added | LOW |
| §3.6 | Content-Length mismatch (set before body re-encoding) | MEDIUM |
| §7.4 | No loop detection (Via header would enable this) | MEDIUM |
| §3.7 | Accept-Encoding unconditionally deleted | MEDIUM |

### Cluster C: Connection & DNS Architecture

**Root cause:** `proxy.go:25-29` — the `dialer` has no `Resolver` field, so all
proxy HTTP requests resolve hostnames via the system's `/etc/resolv.conf`,
completely bypassing GateSentry's DNS filtering.

**Additional:** `isLanAddress()` in `utils.go` exists but is only used to **skip
filtering** for LAN clients — never to **block outbound requests** to
internal/private addresses (SSRF protection).

**Symptoms (4 tests):**
| Test | Symptom | Severity |
|------|---------|----------|
| §8.1 | Proxy DNS bypasses GateSentry | **CRITICAL** |
| §7.1 | SSRF — admin UI accessible via proxy | **CRITICAL** |
| §7.2 | SSRF — localhost accessible via proxy | CRITICAL |
| §7.4 | No loop detection on self-request | MEDIUM |

### Cluster D: Content Scanning Design

**Root cause:** `contentscanner.go` — `ScanMedia()` and `ScanText()` operate
only on the full buffered body, require `filetype.Match()` (which only needs
~262 bytes), and have no decompression limits or Content-Type validation.

**Symptoms (3 tests):**
| Test | Symptom |
|------|---------|
| §15.18 | No decompression bomb limit |
| §15.31 | No Content-Type vs actual-body validation |
| §14.1 | Full buffering exists to serve the scanner |

### Cluster E: Missing Protocol Support

**Root cause:** Features that were never implemented.

**Symptoms (3 tests):**
| Test | Symptom |
|------|---------|
| §6.1 | WebSocket returns 400 (8-line stub) |
| §1.5 | NXDOMAIN returns NOERROR (DNS server issue) |
| §2.1 | No DNS response caching |

---

## 5. Remediation Phases

| Phase | Cluster | Effort | Tests Fixed | Risk | Description |
|-------|---------|--------|-------------|------|-------------|
| **1** | B | **Small** (50-80 LOC) | 10 | Very Low | Response header sanitization |
| **2** | C | **Small** (30-50 LOC) | 4 | Low | DNS resolver wiring + SSRF block |
| **3** | A | **Large** (200-300 LOC) | 8 | Medium | Streaming response pipeline |
| **4** | E | **Medium** (100-150 LOC) | 3 | Low | WebSocket tunnel + DNS cache |
| **5** | D | **Medium** (80-120 LOC) | 3 | Low | Content scanning hardening |

**Total estimated new code: 460-700 lines**
**Total estimated tests fixed: 20 of 20 (3 FAIL + 17 KNOWN)**

### Dependency Graph

```
Phase 1 (headers) ──→ Phase 3 (streaming) ──→ Phase 5 (scanning)
Phase 2 (DNS/SSRF) ─┘                         Phase 4 (WebSocket) ─┘
```

Phases 1 and 2 are independent and can be done in parallel.
Phase 3 depends on Phase 1 (headers must be solid before refactoring the body pipeline).
Phases 4 and 5 are independent of each other but benefit from Phase 3.

---

## 6. Phase 1 — Response Header Sanitization

**Goal:** Fix the only CRITICAL security vulnerability and 9 other header issues.

### Changes Required

#### 6.1 New function: `sanitizeResponseHeaders()`

Add to `proxy.go` — called before `copyResponseHeader()`. This function:

1. **Validates Content-Length** — reject responses with multiple conflicting
   Content-Length values (RFC 9110 §8.6)
2. **Rejects negative Content-Length** — `Content-Length: -1` is invalid
3. **Strips null bytes from header values** — prevents header injection via
   C-parser truncation
4. **Adds Via header** — `Via: 1.1 gatesentry` per RFC 7230 §5.7.1
5. **Detects response splitting** — reject header values containing `\r` or `\n`

#### 6.2 Fix `copyResponseHeader()`

Current code skips `Content-Length` but copies everything else blindly:

```go
// CURRENT (proxy.go:700-712)
func copyResponseHeader(w http.ResponseWriter, resp *http.Response) {
    newHeader := w.Header()
    for key, values := range resp.Header {
        if key == "Content-Length" {
            continue
        }
        for _, v := range values {
            newHeader.Add(key, v)
        }
    }
    w.WriteHeader(resp.StatusCode)
}
```

Must be changed to:
- Call `sanitizeResponseHeaders()` first
- Skip hop-by-hop headers in the response direction
- Set Content-Length **after** body processing, not before

#### 6.3 Fix Content-Length lifecycle

Currently, Content-Length is:
1. Skipped in `copyResponseHeader()` (line 702)
2. Set in the gzip path (line 524) — but to `resp.ContentLength` which is the *original* length
3. Set in the non-gzip path (line 528) — correctly to `len(localCopyData)`
4. Set **again** in the >10MB path (line 499) — to `resp.ContentLength` before body is written

Fix: always set Content-Length as the **last step** before writing to the client,
based on actual bytes being written.

#### 6.4 Add Via header for loop detection

Adding `Via: 1.1 gatesentry` to every proxied response enables:
- RFC compliance (§5.7.1)
- Self-loop detection (check incoming `Via` for own identifier)
- §7.4 fix (proxy self-request detection)

### Tests Fixed by Phase 1

| Test | Before | After | Responsibility |
|------|--------|-------|----------------|
| §15.10 | ⚠️ KNOWN (null in headers) | ✅ PASS | Go `net/http` client — rejects at `RoundTrip()` |
| §15.12 | ⚠️ KNOWN (double C-L) | ✅ PASS | Go `net/http` client — rejects conflicting C-L |
| §15.14 | ⚠️ KNOWN (negative C-L) | ✅ PASS | Go `net/http` client — rejects C-L: -1 |
| §15.19 | ⚠️ KNOWN (response splitting) | ⚠️ KNOWN | Inherent HTTP-proxy limitation — Go parses `\r\n` as header delimiter; injected headers indistinguishable from legitimate |
| §3.1 | ⚠️ KNOWN (no Via header) | ✅ PASS | GateSentry — `copyResponseHeader()` adds `Via: 1.1 gatesentry` |
| §3.6 | ⚠️ KNOWN (C-L mismatch) | ✅ PASS | GateSentry — Content-Length lifecycle fixed |
| §7.4 | ❌ FAIL (loop detection) | ✅ PASS | GateSentry — Via-based loop detection in `ServeHTTP()` |
| §15.29 | ⚠️ KNOWN (TRACE reflection) | Improved | GateSentry — Via header aids detection |
| §15.31 | ⚠️ KNOWN (XSS in wrong C-T) | Improved | GateSentry — `X-Content-Type-Options: nosniff` added |
| §3.7 | ⚠️ KNOWN (Accept-Encoding) | Improved | GateSentry — conditional stripping |

**Go `net/http` protection:** §15.10, §15.12, and §15.14 are rejected by Go's
`http.Transport.RoundTrip()` before the proxy ever sees `resp.Header`. The
`sanitizeResponseHeaders()` function in the proxy provides defence-in-depth for
any edge cases the Go HTTP client might miss in future versions.

---

## 7. Phase 2 — DNS & SSRF Hardening

**Goal:** Make the proxy use GateSentry's own DNS for hostname resolution, and
block outbound requests to private/internal addresses.

### Changes Required

#### 7.1 Wire dialer to GateSentry DNS

```go
// CURRENT (proxy.go:25-29)
var dialer = &net.Dialer{
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,
}

// PROPOSED
var dialer = &net.Dialer{
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,
    Resolver: &net.Resolver{
        PreferGo: true,
        Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
            d := net.Dialer{Timeout: 5 * time.Second}
            return d.DialContext(ctx, "udp", "127.0.0.1:10053")
        },
    },
}
```

This ensures every hostname the proxy resolves goes through GateSentry's DNS,
enabling domain filtering to work for proxied HTTP requests.

#### 7.2 SSRF protection on outbound connections

Add a `DialContext` wrapper that resolves the hostname, then checks if the
resolved IP is a private/internal address:

- `127.0.0.0/8` (loopback)
- `10.0.0.0/8` (RFC 1918)
- `172.16.0.0/12` (RFC 1918)
- `192.168.0.0/16` (RFC 1918) — but allow if the *client* is also on the same LAN
- `169.254.0.0/16` (link-local / cloud metadata)
- `::1` (IPv6 loopback)
- `fc00::/7` (IPv6 ULA)

#### 7.3 Admin UI isolation

Block proxy requests to the GateSentry admin port (8080) regardless of address.

### Tests Fixed by Phase 2

| Test | Current | After Phase 2 |
|------|---------|---------------|
| §8.1 | ⚠️ KNOWN (system DNS) | ✅ PASS |
| §7.1 | ⚠️ KNOWN (SSRF admin) | ✅ PASS |
| §7.2 | ⚠️ KNOWN (SSRF localhost) | ✅ PASS |
| §15.33 | ✅ PASS (redirect chain) | ✅ PASS (stronger — blocks final hop) |

---

## 8. Phase 3 — Streaming Response Pipeline

**Goal:** Replace the buffer-everything architecture with a 3-path response
router that only buffers content that actually needs scanning.

### Design

```
Response from upstream
     │
     ▼
Content-Type check
     │
     ├── text/html, text/xml  ──────→ Path C: Buffer & Scan (≤10MB)
     ├── image/*, video/*, audio/*  ─→ Path B: Peek 4KB + Stream
     └── everything else  ──────────→ Path A: Stream Passthrough
```

**Path A: Stream Passthrough** — JS, CSS, fonts, JSON, binary, downloads, video.
No buffering. `io.Copy()` from upstream response body directly to
`http.ResponseWriter`, using `http.Flusher` for progressive delivery.

**Path B: Peek & Stream** — Media types. Read first 4KB for `filetype.Match()`,
run content filter. If allowed, write the 4KB peek + stream the rest. If
blocked, return block page.

**Path C: Buffer & Scan** — Only `text/html` (and perhaps XML). Buffer up to
10MB, run `ScanText()`, then deliver. This is the existing behaviour, preserved
only for the content type that needs it.

### Key Implementation Details

- Use `http.Flusher` interface to progressively deliver bytes
- Preserve `Transfer-Encoding: chunked` from upstream when possible
- Set `Content-Length` only when the proxy knows the final body size
- For Path A, do NOT strip `Accept-Encoding` — let upstream handle compression

### Tests Fixed by Phase 3

| Test | Current | After Phase 3 |
|------|---------|---------------|
| §14.1 | ⚠️ KNOWN (10MB buffer) | ✅ PASS (streaming for most content) |
| §12.2 | ⊘ SKIP (SSE broken) | ✅ PASS (Path A streams SSE) |
| §12.3 | ❌ FAIL (drip timing) | ✅ PASS (Path A streams drips) |
| §15.4 | ⚠️ KNOWN (200 on drop) | ✅ PASS (Path A: error before client sees 200) |
| §15.13 | ⚠️ KNOWN (200 on incomplete) | ✅ PASS (Path A: error on truncated stream) |
| §3.7 | ⚠️ KNOWN (Accept-Encoding) | ✅ PASS (Path A: preserved) |
| §15.18 | ✅ PASS (bomb decompressed) | ✅ PASS (with decompression limits) |
| §11.4 | ✅ PASS (73ms TTFB) | ✅ PASS (TTFB near 0 for Path A) |

---

## 9. Phase 4 — WebSocket & Protocol Support

**Goal:** Support WebSocket connections (transparent tunnel) and add DNS caching.

### 9.1 WebSocket Tunnel

Replace the 8-line stub with a bidirectional tunnel:

1. Detect `Upgrade: websocket` + `Connection: Upgrade` headers
2. Establish TCP connection to upstream
3. Forward the HTTP upgrade request
4. Read the 101 Switching Protocols response
5. `io.Copy` bidirectionally (same pattern as `ConnectDirect`)

No content inspection — WebSocket frames are opaque to the proxy, same as
CONNECT tunnel traffic.

### 9.2 DNS Response Cache

Add an in-memory cache in the DNS server keyed by `(qname, qtype)` with TTL
expiration. Implementation: `sync.Map` or a simple map with `sync.RWMutex`.

### Tests Fixed by Phase 4

| Test | Current | After Phase 4 |
|------|---------|---------------|
| §6.1 | ⚠️ KNOWN (returns 400) | ✅ PASS |
| §2.1 | ⚠️ KNOWN (no caching) | ✅ PASS |
| §1.5 | ⚠️ KNOWN (NOERROR for NXDOMAIN) | ✅ PASS |

---

## 10. Phase 5 — Content Scanning Hardening

**Goal:** Make the content scanner robust against adversarial responses.

### Changes

1. **Decompression bomb limit** — cap decompressed size at `MaxContentScanSize`
   during scanning, independent of compressed size
2. **Content-Type validation** — compare declared Content-Type against
   `filetype.Match()` result; flag mismatches
3. **Remove dead code** — the commented-out LazyLoad JS injection in
   `ScanText()` (lines 110-127 of contentscanner.go) should be removed
4. **Streaming HTML scan** — for Phase 3's Path C, consider a ring-buffer
   approach that can scan HTML as it arrives

### Tests Fixed by Phase 5

| Test | Current | After Phase 5 |
|------|---------|---------------|
| §15.18 | ✅ PASS (1MB bomb accepted) | ✅ PASS (with size limit) |
| §15.31 | ⚠️ KNOWN (XSS in wrong C-T) | ✅ PASS (C-T mismatch detected) |
| §15.29 | ⚠️ KNOWN (TRACE reflection) | ✅ PASS (sensitive header stripping) |

---

## 11. Implementation Checklist

### Phase 1 — Response Header Sanitization
- [x] Create `sanitizeResponseHeaders()` in `proxy.go`
- [x] Validate single Content-Length (reject conflicting multiples)
- [x] Reject negative Content-Length values
- [x] Strip null bytes (`\x00`) from header values
- [x] Strip `\r` and `\n` from header values (response splitting defense)
- [x] Add `Via: 1.1 gatesentry` to responses
- [x] Add incoming `Via` header check for loop detection
- [x] Fix Content-Length lifecycle (set after body processing)
- [x] Add `X-Content-Type-Options: nosniff` to proxied responses
- [ ] Conditionally preserve `Accept-Encoding` for non-scannable types (deferred to Phase 3)
- [x] Run test suite — 81 PASS, 2 FAIL, 13 KNOWN, 1 SKIP (§3.1, §3.6, §7.4 fixed)
- [x] Verify no regressions in §4 (HTTP methods), §5 (HTTPS), §11 (downloads)

### Phase 2 — DNS & SSRF Hardening
- [x] Wire `dialer.Resolver` to `127.0.0.1:10053` (GateSentry DNS)
- [x] Make DNS port configurable (`GATESENTRY_DNS_PORT` env var)
- [x] Add `safeDialContext()` — blocks DNS rebinding to admin port (loopback/link-local)
- [x] Block proxy requests targeting admin port (8080) in `ServeHTTP()`
- [x] Allow LAN-to-LAN requests — only admin-port rebinding is blocked
- [x] Run test suite — 84 PASS, 2 FAIL, 10 KNOWN, 1 SKIP (§8.1, §7.1, §7.2 fixed)
- [x] Verify HTTPS CONNECT still works — §5.1, §5.2 both PASS

### Phase 3 — Streaming Response Pipeline
- [ ] Define content-type classification: scannable vs passthrough
- [ ] Implement Path A: stream passthrough with `http.Flusher`
- [ ] Implement Path B: peek 4KB + stream for media
- [ ] Preserve Path C: buffer-and-scan for HTML only
- [ ] Remove unconditional `Accept-Encoding` stripping for Path A
- [ ] Add `Transfer-Encoding: chunked` passthrough
- [ ] Add decompression bomb limit
- [ ] Run test suite — verify §14.1, §12.2, §12.3, §15.4, §15.13 fixed
- [ ] Benchmark TTFB before/after (target: <5ms for Path A)
- [ ] Load test: 100 concurrent downloads, measure peak RSS

### Phase 4 — WebSocket & Protocol Support
- [ ] Implement WebSocket tunnel in `websocket.go`
- [ ] Detect upgrade headers
- [ ] Forward upgrade request, read 101 response
- [ ] Bidirectional `io.Copy`
- [ ] Add DNS response cache with TTL expiration
- [ ] Fix NXDOMAIN rcode preservation
- [ ] Run test suite — verify §6.1, §2.1, §1.5 fixed
- [ ] Test WebSocket with a real application (e.g., simple chat)

### Phase 5 — Content Scanning Hardening
- [ ] Add decompression size limit in scanner
- [ ] Add Content-Type vs filetype.Match() cross-check
- [ ] Remove dead LazyLoad code from `contentscanner.go`
- [ ] Consider ring-buffer streaming HTML scanner
- [ ] Run test suite — verify §15.18, §15.31, §15.29 improved
- [ ] Run full adversarial battery (§15) — zero regressions

---

## 12. Risk Assessment

### Phase 1 (Very Low Risk)
- Changes only touch `copyResponseHeader()` and add a new validation function
- All existing behaviour preserved for well-formed responses
- Only rejects/sanitises malformed responses that were already problematic
- Easy to test: response splitting is a binary pass/fail

### Phase 2 (Low Risk)
- DNS resolver change is a single struct field
- SSRF protection adds a check before `Dial` — if check fails, returns error
- Risk: could block legitimate LAN-to-LAN proxying. Mitigation: allow when
  client is on the same RFC 1918 subnet as the destination.

### Phase 3 (Medium Risk)
- Largest change — restructures the core response pipeline
- Must be carefully tested against all 96 tests + manual verification
- Risk: content scanning could miss blocked content if classification is wrong.
  Mitigation: default to Path C (buffer-and-scan) for unknown content types.
- Risk: streaming path could leak partial responses on error.
  Mitigation: only send status 200 after confirming upstream responded with 200.

### Phase 4 (Low Risk)
- WebSocket tunnel is additive (new code, no existing code changed)
- DNS cache is additive (caches responses, falls through to upstream on miss)
- Risk: stale cache entries. Mitigation: honour TTL, add manual flush endpoint.

### Phase 5 (Low Risk)
- Hardens existing scanner, doesn't change scanning logic
- Decompression limit is additive (cap, not change)
- Dead code removal is always safe

---

## 13. Testing Strategy

### Automated Test Suite

The existing 96-test suite (`tests/proxy_benchmark_suite.sh`) provides
comprehensive regression coverage. After each phase:

1. Run the full suite: `bash tests/proxy_benchmark_suite.sh`
2. Verify targeted tests flip from FAIL/KNOWN → PASS
3. Verify zero regressions (no new FAIL results)
4. Update test expectations for fixed issues

### Test Infrastructure

All tests run locally with zero internet dependency:
- **nginx** testbed: static files + HTTPS on ports 9999/9443
- **Echo server**: 41 hostile endpoints on port 9998
- **Internal CA**: "JVJ 28 Inc." — TLS chain verification

Setup: `sudo bash tests/testbed/setup.sh`

### Adversarial Testing Philosophy

> *"Don't configure nginx to make tests work — configure it to make tests fail."*

The testbed simulates a **hostile internet**. Every endpoint in the echo server
deliberately violates HTTP specifications in ways that real-world servers do.
The proxy must handle all of them gracefully:
- No crashes
- No hangs
- No data leaks
- Errors returned to the client with appropriate status codes

### Benchmark Targets

| Metric | Current | Target |
|--------|---------|--------|
| DNS latency | 33ms avg | <10ms (with cache) |
| DNS throughput | 7,539 QPS | >15,000 QPS (with cache) |
| Proxy throughput | 1,656 req/s | >2,000 req/s |
| TTFB (1MB file) | 73ms | <5ms (streaming) |
| TTFB (HTML page) | 73ms | 73ms (unchanged — still buffered) |
| Peak RSS (100 concurrent) | ~1GB (100×10MB) | ~100MB (streaming) |
| Test pass rate | 75/96 (78%) | 96/96 (100%) |

---

---

## 14. Legacy Code Cleanup — `application/proxy/`

### Discovery

During architecture review, two separate proxy packages were found:

| | `application/proxy/` | `gatesentryproxy/` |
|---|---|---|
| **Status** | 🪦 Dead / legacy | ✅ Active |
| **Package name** | `gatesentry2proxy` | `gatesentryproxy` |
| **Approach** | Wraps `gopkg.in/elazarl/goproxy.v1` (third-party) | Custom `net/http` implementation |
| **LOC** | ~300 (mostly commented out) | 3,074 |
| **Initialised by** | `application/start.go` (commented out) | `main.go` (active) |

### Evidence

**`application/start.go`** — the old proxy startup is commented out:
```go
// proxy := gatesentry2proxy.StartProxy();
R = &GSRuntime{
    // Proxy: proxy,
}
// proxy.Listen();
```

**`main.go:258-259`** — only the new proxy is initialised:
```go
gatesentryproxy.InitProxy()
ngp := gatesentryproxy.NewGSProxy()
```

### Issues Found

1. **Dead import in `application/runtime.go:13`:**
   ```go
   gatesentry2proxy "bitbucket.org/abdullah_irfan/gatesentryf/proxy"
   ```
   This import is only used for the `Proxy *gatesentry2proxy.GSProxy` field in
   `GSRuntime` (line 78), which is never assigned or read.

2. **Expired hardcoded CA certificate in `application/proxy/certs.go`:**
   - Organization: "ABDULLAHAINC"
   - Created: March 9, 2017
   - **Expired: March 9, 2018** — over 7 years ago
   - Contains both the CA certificate AND the private key in source code

3. **Two different goproxy forks referenced:**
   - `session.go` uses `gopkg.in/elazarl/goproxy.v1`
   - `ext/html.go` uses `github.com/abourget/goproxy` (a different fork)

4. **`application/filters.go:5`** still imports `gopkg.in/elazarl/goproxy.v1`

### Cleanup Plan

- [ ] Remove `application/proxy/` directory (4 files: `certs.go`, `session.go`,
      `structures.go`, `ext/html.go`)
- [ ] Remove `gatesentry2proxy` import from `application/runtime.go`
- [ ] Remove `Proxy *gatesentry2proxy.GSProxy` field from `GSRuntime` struct
- [ ] Remove goproxy import from `application/filters.go` if unused
- [ ] Remove `gopkg.in/elazarl/goproxy.v1` and `github.com/abourget/goproxy`
      from `go.mod` / `go.sum`
- [ ] Verify `go build` succeeds after cleanup
- [ ] Run full test suite to confirm no regressions

**Risk: Very Low** — this is dead code removal. The old proxy is never
initialised, never called, and never assigned. The only risk is if any other
file imports the old package's types, which the grep search confirms is limited
to `runtime.go` and `filters.go`.

---

*This document is maintained alongside the implementation. Each phase will be
committed and tested independently, with test results updated in this document
before the PR is submitted.*
