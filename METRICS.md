# GateSentry Prometheus Metrics

Comprehensive `/metrics` endpoint for diagnosing performance, correlating resource usage with system behaviour, and monitoring the health of the DNS server, proxy server, and supporting subsystems.

## Endpoint

```
GET http://<admin-host>:8080/metrics
```

- Registered on the **root router** (not the basePath subrouter).
- **No authentication** — Prometheus scrapers hit this path without JWT tokens.
- Standard Prometheus text exposition format.

## Architecture: Zero Hot-Path Impact

The metrics system is designed so that DNS query processing and proxy request handling are **never blocked or slowed** by instrumentation.

```
 DNS / Proxy hot path                    Prometheus scrape (cold path)
 ───────────────────                     ──────────────────────────────
 atomic.Int64.Add(1)  ──→ shared memory ──→  atomic.Int64.Load()
 AtomicHistogram.Observe() ─────────────→  CumulativeBuckets()
```

### Key design decisions

| Concern | Approach |
|---------|----------|
| Counter increments on hot path | `sync/atomic.Int64` — single CPU instruction, zero lock contention |
| Latency distribution | `AtomicHistogram` / `DNSHistogram` — fixed-bucket arrays with atomic increments, zero heap allocation per `Observe()` |
| Prometheus collection | Custom `prometheus.Collector` reads atomics **on scrape only** — no background goroutine, no ticker |
| Cross-module boundary | Proxy module (`gatesentryproxy`) has **no Prometheus dependency** — only `sync/atomic` and `time`. The application module's collector imports and reads proxy counters |
| Nil safety | Every `Collect*()` method checks for nil sources and skips gracefully |

## Source Files

| File | Role |
|------|------|
| `gatesentryproxy/metrics.go` | `ProxyMetrics` struct (30+ atomic counters) + `AtomicHistogram` type |
| `application/dns/server/metrics.go` | `DNSMetrics` struct (10 atomic counters) + `DNSHistogram` type |
| `application/webserver/metrics/metrics.go` | Prometheus collector — reads all atomics on scrape, emits labeled metrics |
| `application/webserver/webserver.go` | Registers `/metrics` handler with custom registry |

### Instrumented hot-path files

| File | Points | What's measured |
|------|--------|-----------------|
| `application/dns/server/server.go` | 10 | Query timing, per-result-type counters, upstream RTT |
| `gatesentryproxy/proxy.go` | ~15 | Request count/timing/active gauge, block reasons, upstream timing, bytes, pipeline path |
| `gatesentryproxy/ssl.go` | 7 | Active MITM/direct gauges, panic recovery, cert cache hit/miss, TLS errors |

## Metric Reference

### DNS Server

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_dns_queries_total` | counter | `result` | Total DNS queries. Labels: `blocked`, `cached`, `forwarded`, `device`, `exception`, `internal`, `error`, `wpad`, `ddns` |
| `gatesentry_dns_query_duration_seconds` | histogram | — | End-to-end `handleDNSRequest` processing time (14 buckets, 100µs–10s) |
| `gatesentry_dns_upstream_duration_seconds` | histogram | — | Upstream resolver round-trip time from `dns.Client.Exchange()` |

### DNS Cache

| Metric | Type | Description |
|--------|------|-------------|
| `gatesentry_dns_cache_hits_total` | counter | Cache hits |
| `gatesentry_dns_cache_misses_total` | counter | Cache misses |
| `gatesentry_dns_cache_inserts_total` | counter | New entries inserted |
| `gatesentry_dns_cache_evictions_total` | counter | Entries evicted due to capacity pressure |
| `gatesentry_dns_cache_expired_total` | counter | Entries removed by TTL expiry |
| `gatesentry_dns_cache_entries` | gauge | Current number of cached entries |
| `gatesentry_dns_cache_max_entries` | gauge | Maximum cache capacity |
| `gatesentry_dns_cache_size_bytes` | gauge | Estimated memory used by cache |
| `gatesentry_dns_cache_hit_rate_percent` | gauge | Hit rate as percentage (0–100) |

### Proxy Server — Requests

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_proxy_requests_total` | counter | `type` | Request count. Labels: `all`, `http`, `connect`, `websocket` |
| `gatesentry_proxy_connect_total` | counter | `type` | HTTPS CONNECT breakdown. Labels: `mitm`, `direct` |
| `gatesentry_proxy_request_duration_seconds` | histogram | — | End-to-end `ServeHTTP` processing time (14 buckets, 1ms–60s) |
| `gatesentry_proxy_upstream_duration_seconds` | histogram | — | Upstream `RoundTrip` time |

### Proxy Server — Active Connections (Gauges)

| Metric | Type | Description |
|--------|------|-------------|
| `gatesentry_proxy_active_requests` | gauge | Currently executing `ServeHTTP` handlers |
| `gatesentry_proxy_active_mitm_connections` | gauge | Active SSL-bump (MITM) tunnels |
| `gatesentry_proxy_active_direct_connections` | gauge | Active CONNECT passthrough tunnels |
| `gatesentry_proxy_active_websocket_connections` | gauge | Active WebSocket tunnels |

### Proxy Server — Blocks

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_proxy_blocks_total` | counter | `reason` | Blocked requests. Labels: `rule`, `url`, `time`, `user`, `ssrf`, `content_type`, `keyword`, `media` |

### Proxy Server — Errors

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_proxy_errors_total` | counter | `type` | Error count. Labels: `upstream` (RoundTrip failures), `hijack` (connection hijack), `tls` (TLS handshake), `panic` (recovered panics) |
| `gatesentry_proxy_auth_failures_total` | counter | — | Proxy authentication failures (407 responses) |

### Proxy Server — Pipeline & Transfer

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_proxy_pipeline_total` | counter | `path` | Response pipeline path. Labels: `stream` (passthrough), `peek` (peek + stream), `buffer` (full buffer + scan) |
| `gatesentry_proxy_bytes_written_total` | counter | — | Total response bytes written to clients |

### Proxy Server — TLS Certificate Cache

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_proxy_cert_cache_total` | counter | `result` | Cert cache operations. Labels: `hit`, `miss` |
| `gatesentry_proxy_cert_cache_entries` | gauge | — | Current entries in the cert cache |
| `gatesentry_proxy_user_cache_entries` | gauge | — | Current entries in the proxy auth user cache |

### SSE & Application

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gatesentry_sse_subscribers` | gauge | `stream` | Active SSE subscribers. Labels: `log_stream`, `dns_events` |
| `gatesentry_devices` | gauge | — | Discovered network devices |
| `gatesentry_rules` | gauge | — | Total configured proxy rules |
| `gatesentry_rules_enabled` | gauge | — | Enabled proxy rules |
| `gatesentry_domain_index_domains_total` | gauge | — | Total unique domains across all loaded domain lists |

### Go Runtime (automatic via `GoCollector`)

| Metric | What it tells you |
|--------|-------------------|
| `go_goroutines` | Current goroutine count — correlate with active connections |
| `go_threads` | OS threads in use |
| `go_gc_duration_seconds` | GC pause times (summary with quantiles) |
| `go_memstats_alloc_bytes` | Heap bytes currently allocated |
| `go_memstats_heap_inuse_bytes` | Heap memory in use |
| `go_memstats_sys_bytes` | Total memory obtained from OS |
| `go_memstats_heap_objects` | Live heap objects — rising count means allocation pressure |
| `go_memstats_stack_inuse_bytes` | Stack memory — grows with goroutine count |
| `go_sched_gomaxprocs_threads` | GOMAXPROCS value |
| `go_info` | Go version label |

### Process (automatic via `ProcessCollector`)

| Metric | What it tells you |
|--------|-------------------|
| `process_cpu_seconds_total` | Total CPU time consumed |
| `process_resident_memory_bytes` | RSS — actual physical memory |
| `process_virtual_memory_bytes` | Virtual memory size |
| `process_open_fds` | Open file descriptors — proxy connections consume FDs |
| `process_max_fds` | FD limit (`ulimit -n`) |
| `process_start_time_seconds` | Process start time (unix epoch) — calculate uptime |
| `process_network_receive_bytes_total` | Network bytes received |
| `process_network_transmit_bytes_total` | Network bytes transmitted |

## Histogram Bucket Boundaries

### Proxy (`AtomicHistogram`) — 14 buckets

Covers sub-millisecond cache hits through multi-second page loads:

```
1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s, 60s, +Inf
```

### DNS (`DNSHistogram`) — 14 buckets

Covers sub-millisecond cached responses through upstream timeouts:

```
100µs, 500µs, 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, +Inf
```

## Troubleshooting Correlation Guide

Use these metric combinations to diagnose common issues:

### High CPU

```promql
# Check request rate
rate(gatesentry_proxy_requests_total{type="all"}[5m])
rate(gatesentry_dns_queries_total[5m])

# Check goroutine count
go_goroutines

# Check GC pressure
rate(go_gc_duration_seconds_count[5m])     # GC cycles per second
go_memstats_heap_objects                    # live objects driving GC work
```

### High Memory

```promql
# Breakdown
go_memstats_heap_inuse_bytes               # heap
go_memstats_stack_inuse_bytes              # goroutine stacks
gatesentry_dns_cache_size_bytes            # DNS cache
gatesentry_proxy_cert_cache_entries        # TLS cert cache
gatesentry_domain_index_domains_total      # domain lists
process_resident_memory_bytes              # total RSS
```

### Slow Proxy Responses

```promql
# Where is time spent?
histogram_quantile(0.95, rate(gatesentry_proxy_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(gatesentry_proxy_upstream_duration_seconds_bucket[5m]))

# Pipeline path distribution — buffer path is slowest
rate(gatesentry_proxy_pipeline_total[5m])
```

### Connection / FD Exhaustion

```promql
# Active connections
gatesentry_proxy_active_requests
gatesentry_proxy_active_mitm_connections
gatesentry_proxy_active_direct_connections
gatesentry_proxy_active_websocket_connections

# FD headroom
process_open_fds / process_max_fds
```

### DNS Performance

```promql
# Cache effectiveness
gatesentry_dns_cache_hit_rate_percent
rate(gatesentry_dns_cache_evictions_total[5m])    # evictions mean cache too small

# Query latency
histogram_quantile(0.95, rate(gatesentry_dns_query_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(gatesentry_dns_upstream_duration_seconds_bucket[5m]))

# Error rate
rate(gatesentry_dns_queries_total{result="error"}[5m])
```

### Block Analysis

```promql
# What's being blocked and why
rate(gatesentry_proxy_blocks_total[5m])

# Auth issues
rate(gatesentry_proxy_auth_failures_total[5m])
```

### TLS / MITM Health

```promql
# TLS errors vs total MITM connections
rate(gatesentry_proxy_errors_total{type="tls"}[5m])
rate(gatesentry_proxy_connect_total{type="mitm"}[5m])

# Cert cache efficiency
rate(gatesentry_proxy_cert_cache_total{result="hit"}[5m])
/ (rate(gatesentry_proxy_cert_cache_total{result="hit"}[5m]) + rate(gatesentry_proxy_cert_cache_total{result="miss"}[5m]))

# Panics in SSL bump (should be zero)
gatesentry_proxy_errors_total{type="panic"}
```

## Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'gatesentry'
    scrape_interval: 15s
    static_configs:
      - targets: ['<host>:8080']
```

The endpoint is lightweight — scrape intervals of 10–15 seconds are fine. The collector reads ~35 atomic values and a few lightweight accessors (device count, rule list, cache snapshot) on each scrape.
