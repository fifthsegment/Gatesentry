// metrics.go provides lightweight, lock-free instrumentation counters for
// the DNS query hot path.
//
// All fields use sync/atomic — zero lock contention on query processing.
// The application's Prometheus collector reads these on each /metrics scrape.
package gatesentryDnsServer

import (
	"sync/atomic"
	"time"
)

// dnsMetrics is the package-level singleton.
var dnsMetrics = &DNSMetrics{}

// GetDNSMetrics returns the singleton metrics instance for reading by the
// Prometheus collector.  Never nil.
func GetDNSMetrics() *DNSMetrics {
	return dnsMetrics
}

// DNSMetrics holds atomic counters for DNS server instrumentation.
type DNSMetrics struct {
	// ── Query result counters ─────────────────────────────────────────
	QueriesTotal     atomic.Int64 // every call to handleDNSRequest
	QueriesBlocked   atomic.Int64 // resolved to local IP (blocked domain)
	QueriesCached    atomic.Int64 // served from cache
	QueriesForwarded atomic.Int64 // forwarded to upstream resolver
	QueriesDevice    atomic.Int64 // answered from device store
	QueriesException atomic.Int64 // exception domain (forwarded)
	QueriesInternal  atomic.Int64 // internal record match
	QueriesError     atomic.Int64 // upstream forward error (SERVFAIL)
	QueriesWPAD      atomic.Int64 // WPAD interception
	QueriesDDNS      atomic.Int64 // DDNS UPDATE messages

	// ── Latency histograms ────────────────────────────────────────────
	QueryDuration    DNSHistogram // end-to-end handleDNSRequest time
	UpstreamDuration DNSHistogram // upstream forwardDNSRequest round-trip
}

// ---------------------------------------------------------------------------
// DNSHistogram — identical to the proxy AtomicHistogram but avoids a
// cross-module dependency.
// ---------------------------------------------------------------------------

const dnsHistBucketCount = 14

// DNSHistBoundariesSec covers sub-millisecond cache hits through multi-second
// upstream timeouts.
var DNSHistBoundariesSec = [dnsHistBucketCount]float64{
	0.0001, // 100 µs
	0.0005, // 500 µs
	0.001,  // 1 ms
	0.005,  // 5 ms
	0.010,  // 10 ms
	0.025,  // 25 ms
	0.050,  // 50 ms
	0.100,  // 100 ms
	0.250,  // 250 ms
	0.500,  // 500 ms
	1.0,    // 1 s
	2.5,    // 2.5 s
	5.0,    // 5 s
	10.0,   // 10 s
}

// DNSHistogram records a distribution of durations using fixed buckets
// and atomic counters.
type DNSHistogram struct {
	Buckets [dnsHistBucketCount + 1]atomic.Int64
	SumUS   atomic.Int64
	Count   atomic.Int64
}

// Observe records a duration.
func (h *DNSHistogram) Observe(d time.Duration) {
	us := d.Microseconds()
	h.SumUS.Add(us)
	h.Count.Add(1)
	sec := d.Seconds()
	for i := 0; i < dnsHistBucketCount; i++ {
		if sec <= DNSHistBoundariesSec[i] {
			h.Buckets[i].Add(1)
			return
		}
	}
	h.Buckets[dnsHistBucketCount].Add(1)
}

// CumulativeBuckets returns histogram data formatted for
// prometheus.MustNewConstHistogram.
func (h *DNSHistogram) CumulativeBuckets() (buckets map[float64]uint64, totalCount uint64, totalSum float64) {
	buckets = make(map[float64]uint64, dnsHistBucketCount)
	var cumulative uint64
	for i := 0; i < dnsHistBucketCount; i++ {
		cumulative += uint64(h.Buckets[i].Load())
		buckets[DNSHistBoundariesSec[i]] = cumulative
	}
	totalCount = uint64(h.Count.Load())
	totalSum = float64(h.SumUS.Load()) / 1e6
	return
}
