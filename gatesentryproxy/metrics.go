// Package gatesentryproxy — metrics.go provides lightweight, lock-free
// instrumentation counters for the proxy hot path.
//
// Design principles:
//   - All fields use sync/atomic — ZERO lock contention on the request path.
//   - No Prometheus dependency — the application module's collector reads these
//     atomic values on each scrape.
//   - AtomicHistogram gives latency distribution without heap allocations per
//     observation.
package gatesentryproxy

import (
	"sync/atomic"
	"time"
)

// Metrics is the singleton ProxyMetrics instance.  Initialised at package
// init so it is safe to use from the very first request.
var Metrics = &ProxyMetrics{}

// ProxyMetrics holds atomic counters for every proxy instrumentation point.
// The Prometheus collector reads these on each /metrics scrape.
type ProxyMetrics struct {
	// ── Request counters ──────────────────────────────────────────────
	RequestsTotal  atomic.Int64 // every call to ServeHTTP
	ConnectTotal   atomic.Int64 // CONNECT method requests (HTTPS)
	HTTPTotal      atomic.Int64 // non-CONNECT (plain HTTP & MITM'd HTTPS)
	MITMTotal      atomic.Int64 // SSL-bumped connections
	DirectTotal    atomic.Int64 // CONNECT pass-through (no MITM)
	WebSocketTotal atomic.Int64 // WebSocket upgrades

	// ── Active connection gauges (inc on start, dec on finish) ────────
	ActiveRequests  atomic.Int64 // currently executing ServeHTTP
	ActiveMITM      atomic.Int64 // active SSLBump tunnels
	ActiveDirect    atomic.Int64 // active direct-connect tunnels
	ActiveWebSocket atomic.Int64 // active WebSocket tunnels

	// ── Block counters by reason ──────────────────────────────────────
	BlocksRule        atomic.Int64 // blocked by proxy rule (domain + post-criteria)
	BlocksURL         atomic.Int64 // blocked URL pattern
	BlocksTime        atomic.Int64 // blocked by time restriction
	BlocksUser        atomic.Int64 // blocked user access
	BlocksSSRF        atomic.Int64 // SSRF protection block
	BlocksContentType atomic.Int64 // blocked content-type match
	BlocksKeyword     atomic.Int64 // blocked by keyword filter
	BlocksMedia       atomic.Int64 // blocked by media content filter

	// ── Error counters ────────────────────────────────────────────────
	ErrorsUpstream atomic.Int64 // RoundTrip / upstream errors
	ErrorsHijack   atomic.Int64 // connection hijack failures
	ErrorsTLS      atomic.Int64 // TLS handshake errors (client or server side)
	ErrorsPanic    atomic.Int64 // recovered panics in SSLBump

	// ── Auth ──────────────────────────────────────────────────────────
	AuthFailures atomic.Int64 // proxy auth required / denied

	// ── TLS certificate cache ─────────────────────────────────────────
	CertCacheHits   atomic.Int64
	CertCacheMisses atomic.Int64

	// ── Bytes transferred (approximate — counted in DataPassThru) ─────
	BytesWritten atomic.Int64 // response bytes sent to clients

	// ── Response pipeline path counters ───────────────────────────────
	PipelineStream atomic.Int64 // Path A: stream passthrough
	PipelinePeek   atomic.Int64 // Path B: peek + stream
	PipelineBuffer atomic.Int64 // Path C: buffer + scan

	// ── Latency histograms ────────────────────────────────────────────
	RequestDuration  AtomicHistogram // end-to-end ServeHTTP time
	UpstreamDuration AtomicHistogram // RoundTrip to upstream
}

// ---------------------------------------------------------------------------
// AtomicHistogram — lock-free latency distribution tracker
// ---------------------------------------------------------------------------

// histBucketCount is the number of finite upper-bound buckets.
const histBucketCount = 14

// HistBoundariesSec are the bucket upper bounds in seconds (float64),
// matching the Prometheus convention.  They are chosen to cover the range
// from sub-millisecond DNS lookups to multi-second page loads.
var HistBoundariesSec = [histBucketCount]float64{
	0.001, // 1 ms
	0.005, // 5 ms
	0.010, // 10 ms
	0.025, // 25 ms
	0.050, // 50 ms
	0.100, // 100 ms
	0.250, // 250 ms
	0.500, // 500 ms
	1.0,   // 1 s
	2.5,   // 2.5 s
	5.0,   // 5 s
	10.0,  // 10 s
	30.0,  // 30 s
	60.0,  // 60 s
}

// AtomicHistogram records a distribution of durations using fixed buckets
// and atomic counters.  It has zero allocation per Observe call and never
// blocks the caller.
type AtomicHistogram struct {
	// Buckets[0..histBucketCount-1] = finite buckets.
	// Buckets[histBucketCount]      = +Inf (overflow).
	Buckets [histBucketCount + 1]atomic.Int64
	SumUS   atomic.Int64 // cumulative sum in microseconds
	Count   atomic.Int64 // total observations
}

// Observe records a duration.
func (h *AtomicHistogram) Observe(d time.Duration) {
	us := d.Microseconds()
	h.SumUS.Add(us)
	h.Count.Add(1)
	sec := d.Seconds()
	for i := 0; i < histBucketCount; i++ {
		if sec <= HistBoundariesSec[i] {
			h.Buckets[i].Add(1)
			return
		}
	}
	h.Buckets[histBucketCount].Add(1) // +Inf
}

// Snapshot returns the current histogram state as plain values.
// Used by the Prometheus collector to build MustNewConstHistogram.
func (h *AtomicHistogram) Snapshot() (counts [histBucketCount + 1]int64, sumSec float64, count uint64) {
	for i := range h.Buckets {
		counts[i] = h.Buckets[i].Load()
	}
	sumSec = float64(h.SumUS.Load()) / 1e6
	count = uint64(h.Count.Load())
	return
}

// CumulativeBuckets returns the histogram data formatted for
// prometheus.MustNewConstHistogram: map[upperBound]cumulativeCount.
func (h *AtomicHistogram) CumulativeBuckets() (buckets map[float64]uint64, totalCount uint64, totalSum float64) {
	counts, sumSec, count := h.Snapshot()
	buckets = make(map[float64]uint64, histBucketCount)
	var cumulative uint64
	for i := 0; i < histBucketCount; i++ {
		cumulative += uint64(counts[i])
		buckets[HistBoundariesSec[i]] = cumulative
	}
	return buckets, count, sumSec
}

// CertCacheSize returns the current number of entries in the TLS certificate
// cache.  Thread-safe (acquires read lock).
func CertCacheSize() int {
	CertCache.lock.RLock()
	defer CertCache.lock.RUnlock()
	return len(CertCache.cache)
}

// UserCacheSize returns an approximate count of entries in the user
// authentication cache (sync.Map — no exact size without iteration).
func UserCacheSize() int {
	if IProxy == nil {
		return 0
	}
	count := 0
	IProxy.UsersCache.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
