// Package cache provides a high-performance, memory-bounded DNS response cache
// designed for home network appliances (including Raspberry Pi with ≤1 GB RAM).
//
// Design goals:
//   - Sharded locking (16 shards) to minimise contention under concurrent queries
//   - Per-entry TTL countdown with background reaper (no stale entries linger)
//   - Bounded memory: configurable max entries with nearest-to-expire eviction
//   - Negative caching: NXDOMAIN/NODATA cached per RFC 2308 §5 (SOA minimum TTL)
//   - Atomic hit/miss/eviction counters for zero-cost observability
//   - Safe for concurrent use by multiple goroutines
package cache

import (
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// shardCount is the number of independent cache shards.
// 16 was chosen because it's a power of two (fast modulo via bitmask)
// and provides good concurrency for typical home network query rates
// (hundreds of QPS) without excessive memory overhead from shard metadata.
const shardCount = 16

// Config holds tuneable cache parameters. All fields have sensible defaults
// if left at zero value — call DefaultConfig() for a ready-to-use config.
type Config struct {
	// MaxEntries is the total maximum number of cached DNS responses.
	// Distributed evenly across shards (MaxEntries/shardCount per shard).
	// Default: 10000 (~10–20 MB worst case for typical DNS responses).
	MaxEntries int

	// MinTTL is the minimum cache duration for any entry (floor).
	// Upstream records with TTL < MinTTL are cached for MinTTL instead.
	// This prevents cache churn from CDNs that set TTL=0 or TTL=1.
	// Default: 5 seconds. RFC 8767 recommends ≥5s.
	MinTTL time.Duration

	// MaxTTL is the maximum cache duration for any entry (ceiling).
	// Upstream records with TTL > MaxTTL are capped at MaxTTL.
	// Default: 1 hour. Prevents stale entries from authoritative servers
	// that set multi-day TTLs.
	MaxTTL time.Duration

	// NegativeTTL is the maximum cache duration for negative responses
	// (NXDOMAIN, NODATA). Clamped between MinTTL and NegativeTTL.
	// If the SOA minimum TTL is available and smaller, that is used instead.
	// Default: 300 seconds (5 minutes). RFC 2308 §5 recommends 1–3 hours,
	// but for a home proxy we prefer shorter to avoid stale NXDOMAIN.
	NegativeTTL time.Duration

	// ReapInterval is how often the background reaper sweeps for expired entries.
	// Default: 30 seconds. Lower values reclaim memory faster but cost more CPU.
	ReapInterval time.Duration
}

// DefaultConfig returns a configuration suitable for home network use on
// resource-constrained devices (Raspberry Pi 1 GB).
func DefaultConfig() Config {
	return Config{
		MaxEntries:   10000,
		MinTTL:       5 * time.Second,
		MaxTTL:       1 * time.Hour,
		NegativeTTL:  5 * time.Minute,
		ReapInterval: 30 * time.Second,
	}
}

// entry holds a single cached DNS response with its absolute expiry time
// and an estimate of its heap size for memory accounting.
type entry struct {
	msg       *dns.Msg  // deep-copied on insert; copied again on read
	expiresAt time.Time // absolute wall-clock expiry
	sizeBytes int       // estimated heap size (for memory reporting)
}

// shard is one independent slice of the cache, with its own lock and map.
type shard struct {
	mu       sync.RWMutex
	entries  map[string]*entry
	maxLocal int // per-shard capacity (Config.MaxEntries / shardCount)
}

// Stats holds atomic cache performance counters.
// All fields are read/written with atomic operations — no locking required.
type Stats struct {
	Hits      atomic.Int64 // cache hits (response served from cache)
	Misses    atomic.Int64 // cache misses (forwarded to upstream)
	Inserts   atomic.Int64 // entries added
	Evictions atomic.Int64 // entries evicted (capacity pressure)
	Expired   atomic.Int64 // entries removed by reaper or lazy expiry
	Entries   atomic.Int64 // current entry count (approximate)
	SizeBytes atomic.Int64 // estimated total cache memory (approximate)
}

// Snapshot returns a point-in-time copy of the stats for serialisation.
type StatsSnapshot struct {
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	Inserts   int64   `json:"inserts"`
	Evictions int64   `json:"evictions"`
	Expired   int64   `json:"expired"`
	Entries   int64   `json:"entries"`
	SizeBytes int64   `json:"size_bytes"`
	HitRate   float64 `json:"hit_rate_pct"` // hits / (hits + misses) * 100
}

// DNSCache is a sharded, TTL-aware DNS response cache.
type DNSCache struct {
	shards [shardCount]shard
	config Config
	stats  Stats
	Events *EventBus     // fan-out event bus for SSE consumers (nil-safe)
	stopCh chan struct{} // signals the reaper to stop
	wg     sync.WaitGroup
}

// New creates a new DNS cache with the given configuration and starts
// the background reaper goroutine. Call Stop() to release resources.
func New(cfg Config) *DNSCache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = DefaultConfig().MaxEntries
	}
	if cfg.MinTTL <= 0 {
		cfg.MinTTL = DefaultConfig().MinTTL
	}
	if cfg.MaxTTL <= 0 {
		cfg.MaxTTL = DefaultConfig().MaxTTL
	}
	if cfg.NegativeTTL <= 0 {
		cfg.NegativeTTL = DefaultConfig().NegativeTTL
	}
	if cfg.ReapInterval <= 0 {
		cfg.ReapInterval = DefaultConfig().ReapInterval
	}

	perShard := cfg.MaxEntries / shardCount
	if perShard < 1 {
		perShard = 1
	}

	c := &DNSCache{
		config: cfg,
		Events: NewEventBus(),
		stopCh: make(chan struct{}),
	}
	for i := range c.shards {
		c.shards[i] = shard{
			entries:  make(map[string]*entry),
			maxLocal: perShard,
		}
	}

	c.wg.Add(1)
	go c.reaper()

	return c
}

// Stop shuts down the background reaper. Safe to call multiple times.
func (c *DNSCache) Stop() {
	select {
	case <-c.stopCh:
		// already stopped
	default:
		close(c.stopCh)
	}
	c.wg.Wait()
}

// Get returns a cached response for the given question name and type,
// or nil if not found / expired. The returned message is a deep copy
// with TTLs adjusted to reflect remaining cache lifetime.
func (c *DNSCache) Get(qname string, qtype uint16) *dns.Msg {
	key := cacheKey(qname, qtype)
	s := &c.shards[shardIndex(key)]

	s.mu.RLock()
	e, ok := s.entries[key]
	s.mu.RUnlock()

	if !ok {
		c.stats.Misses.Add(1)
		c.emitQuery(qname, qtype, false, 0)
		return nil
	}

	remaining := time.Until(e.expiresAt)
	if remaining <= 0 {
		// Expired — lazy delete
		s.mu.Lock()
		if e2, ok := s.entries[key]; ok && e2 == e {
			delete(s.entries, key)
			c.stats.Entries.Add(-1)
			c.stats.SizeBytes.Add(-int64(e.sizeBytes))
			c.stats.Expired.Add(1)
		}
		s.mu.Unlock()
		c.stats.Misses.Add(1)
		c.emitQuery(qname, qtype, false, 0)
		return nil
	}

	c.stats.Hits.Add(1)

	// Deep-copy and adjust TTLs to reflect remaining lifetime
	msg := e.msg.Copy()
	ttlSec := uint32(remaining.Seconds())
	if ttlSec == 0 {
		ttlSec = 1 // never return TTL=0 for a valid entry
	}
	adjustTTLs(msg, ttlSec)
	c.emitQuery(qname, qtype, true, int(ttlSec))
	return msg
}

// Put stores a DNS response in the cache. The TTL is derived from the
// minimum TTL across all answer/authority records, clamped to [MinTTL, MaxTTL].
// For negative responses (NXDOMAIN, NODATA with no answers), the SOA minimum
// TTL from the authority section is used per RFC 2308 §5.
func (c *DNSCache) Put(qname string, qtype uint16, msg *dns.Msg) {
	if msg == nil {
		return
	}

	ttl := c.computeTTL(msg)
	if ttl <= 0 {
		return // don't cache zero-TTL entries
	}

	key := cacheKey(qname, qtype)
	s := &c.shards[shardIndex(key)]

	e := &entry{
		msg:       msg.Copy(),
		expiresAt: time.Now().Add(ttl),
		sizeBytes: estimateSize(msg),
	}

	s.mu.Lock()

	// If replacing an existing entry, adjust counters
	if old, ok := s.entries[key]; ok {
		c.stats.SizeBytes.Add(-int64(old.sizeBytes))
		c.stats.Entries.Add(-1)
	}

	// Evict if at capacity
	if len(s.entries) >= s.maxLocal {
		c.evictFromShard(s)
	}

	s.entries[key] = e
	c.stats.Entries.Add(1)
	c.stats.SizeBytes.Add(int64(e.sizeBytes))
	c.stats.Inserts.Add(1)

	s.mu.Unlock()

	// Emit insert event (after releasing the lock)
	c.Events.Emit(InsertEvent(
		qname, dns.TypeToString[qtype],
		int(ttl.Seconds()), c.stats.Entries.Load(),
	))
}

// Flush removes all entries from the cache.
func (c *DNSCache) Flush() {
	totalFlushed := 0
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		for k, e := range s.entries {
			c.stats.SizeBytes.Add(-int64(e.sizeBytes))
			delete(s.entries, k)
			totalFlushed++
		}
		s.mu.Unlock()
	}
	c.stats.Entries.Store(0)
	c.Events.Emit(FlushEvent(totalFlushed))
}

// Remove deletes a specific entry from the cache (e.g. after a DDNS update
// invalidates a record). Returns true if an entry was removed.
func (c *DNSCache) Remove(qname string, qtype uint16) bool {
	key := cacheKey(qname, qtype)
	s := &c.shards[shardIndex(key)]

	s.mu.Lock()
	e, ok := s.entries[key]
	if ok {
		delete(s.entries, key)
		c.stats.Entries.Add(-1)
		c.stats.SizeBytes.Add(-int64(e.sizeBytes))
	}
	s.mu.Unlock()
	return ok
}

// Snapshot returns a point-in-time copy of the cache statistics.
func (c *DNSCache) Snapshot() StatsSnapshot {
	hits := c.stats.Hits.Load()
	misses := c.stats.Misses.Load()
	total := hits + misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100.0
	}
	return StatsSnapshot{
		Hits:      hits,
		Misses:    misses,
		Inserts:   c.stats.Inserts.Load(),
		Evictions: c.stats.Evictions.Load(),
		Expired:   c.stats.Expired.Load(),
		Entries:   c.stats.Entries.Load(),
		SizeBytes: c.stats.SizeBytes.Load(),
		HitRate:   hitRate,
	}
}

// ---------- internal ----------

// cacheKey builds a lookup key from question name and type.
// Format: "lowercasename/TYPE" e.g. "example.com./A"
func cacheKey(qname string, qtype uint16) string {
	t := dns.TypeToString[qtype]
	if t == "" {
		return strings.ToLower(qname) + "/" + fmt.Sprint(qtype)
	}
	return strings.ToLower(qname) + "/" + t
}

// shardIndex maps a cache key to a shard index using FNV-1a hash.
func shardIndex(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32() & (shardCount - 1) // bitmask since shardCount is power of 2
}

// computeTTL determines how long to cache a DNS response.
// Positive responses: minimum TTL across answer + authority records.
// Negative responses (NXDOMAIN, NODATA): SOA minimum TTL per RFC 2308 §5.
// Result is clamped to [MinTTL, MaxTTL] (or [MinTTL, NegativeTTL] for negatives).
func (c *DNSCache) computeTTL(msg *dns.Msg) time.Duration {
	isNegative := msg.Rcode == dns.RcodeNameError || // NXDOMAIN
		(msg.Rcode == dns.RcodeSuccess && len(msg.Answer) == 0) // NODATA

	if isNegative {
		return c.negativeTTL(msg)
	}

	// Positive response — find minimum TTL across all records
	var minTTL uint32 = 0
	found := false
	for _, rr := range msg.Answer {
		ttl := rr.Header().Ttl
		if !found || ttl < minTTL {
			minTTL = ttl
			found = true
		}
	}
	for _, rr := range msg.Ns {
		ttl := rr.Header().Ttl
		if !found || ttl < minTTL {
			minTTL = ttl
			found = true
		}
	}
	if !found {
		// No records at all — use MinTTL as fallback
		return c.config.MinTTL
	}

	return c.clampTTL(time.Duration(minTTL)*time.Second, c.config.MaxTTL)
}

// negativeTTL computes cache duration for NXDOMAIN / NODATA responses.
// Per RFC 2308 §5, uses the minimum of:
//   - The SOA record's TTL (how long the authority says the SOA is valid)
//   - The SOA MINIMUM field (the negative cache TTL)
//
// Falls back to MinTTL if no SOA is present.
func (c *DNSCache) negativeTTL(msg *dns.Msg) time.Duration {
	for _, rr := range msg.Ns {
		if soa, ok := rr.(*dns.SOA); ok {
			// RFC 2308 §5: cache time = min(SOA TTL, SOA MINIMUM)
			soaTTL := rr.Header().Ttl
			soaMin := soa.Minttl
			ttl := soaTTL
			if soaMin < ttl {
				ttl = soaMin
			}
			return c.clampTTL(time.Duration(ttl)*time.Second, c.config.NegativeTTL)
		}
	}
	// No SOA — use MinTTL
	return c.config.MinTTL
}

// clampTTL enforces the [MinTTL, maxCap] range.
func (c *DNSCache) clampTTL(ttl, maxCap time.Duration) time.Duration {
	if ttl < c.config.MinTTL {
		ttl = c.config.MinTTL
	}
	if ttl > maxCap {
		ttl = maxCap
	}
	return ttl
}

// evictFromShard removes entries from a shard when it's at capacity.
// Strategy: first remove all expired entries, then if still at ≥90% capacity,
// evict the entries nearest to expiry (they're almost dead anyway).
// Called with s.mu already held (write lock).
func (c *DNSCache) evictFromShard(s *shard) {
	now := time.Now()
	expiredCount := 0

	// Pass 1: remove expired entries
	for k, e := range s.entries {
		if now.After(e.expiresAt) {
			c.stats.SizeBytes.Add(-int64(e.sizeBytes))
			delete(s.entries, k)
			expiredCount++
		}
	}
	c.stats.Expired.Add(int64(expiredCount))
	c.stats.Entries.Add(-int64(expiredCount))

	// If we've freed enough space, we're done
	if len(s.entries) < s.maxLocal*9/10 {
		return
	}

	// Pass 2: evict entries nearest to expiry (lowest remaining TTL).
	// To avoid sorting (O(n log n)), we do a simple selection:
	// find the entry with the earliest expiresAt, delete it, repeat.
	// We evict 10% of shard capacity at a time.
	evictTarget := s.maxLocal / 10
	if evictTarget < 1 {
		evictTarget = 1
	}
	for i := 0; i < evictTarget && len(s.entries) > 0; i++ {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, e := range s.entries {
			if first || e.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.expiresAt
				first = false
			}
		}
		if e, ok := s.entries[oldestKey]; ok {
			c.stats.SizeBytes.Add(-int64(e.sizeBytes))
			delete(s.entries, oldestKey)
			c.stats.Evictions.Add(1)
			c.stats.Entries.Add(-1)
		}
	}
}

// reaper runs in a background goroutine, periodically sweeping all shards
// to remove expired entries. This prevents memory from growing when the
// cache is below capacity (where Put's eviction logic wouldn't trigger).
func (c *DNSCache) reaper() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.config.ReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.reapExpired()
		}
	}
}

// reapExpired sweeps all shards and removes expired entries.
func (c *DNSCache) reapExpired() {
	now := time.Now()
	totalReaped := int64(0)
	totalBytes := int64(0)

	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		for k, e := range s.entries {
			if now.After(e.expiresAt) {
				totalBytes += int64(e.sizeBytes)
				delete(s.entries, k)
				totalReaped++
			}
		}
		s.mu.Unlock()
	}

	if totalReaped > 0 {
		c.stats.Expired.Add(totalReaped)
		c.stats.Entries.Add(-totalReaped)
		c.stats.SizeBytes.Add(-totalBytes)
		log.Printf("[DNS Cache] Reaper: removed %d expired entries, freed ~%d KB",
			totalReaped, totalBytes/1024)
		c.Events.Emit(ReaperEvent(int(totalReaped), c.stats.Entries.Load()))
	}
}

// adjustTTLs sets the TTL on all resource records in a message.
// OPT pseudo-records (EDNS) are skipped because their TTL field
// carries flags, not a cache lifetime.
func adjustTTLs(msg *dns.Msg, ttl uint32) {
	for _, rr := range msg.Answer {
		rr.Header().Ttl = ttl
	}
	for _, rr := range msg.Ns {
		rr.Header().Ttl = ttl
	}
	for _, rr := range msg.Extra {
		if rr.Header().Rrtype != dns.TypeOPT {
			rr.Header().Ttl = ttl
		}
	}
}

// estimateSize returns a rough estimate of the heap memory used by a dns.Msg.
// This isn't exact (Go doesn't expose object sizes), but it's close enough
// for capacity planning: each RR is ~100–200 bytes, plus the message overhead.
// We overcount slightly rather than undercount — better to evict a bit early
// than to OOM on a Pi.
func estimateSize(msg *dns.Msg) int {
	// Base message struct overhead
	size := 256

	// Each resource record: header (~80 bytes) + rdata (varies, ~40–200 bytes)
	rrCount := len(msg.Answer) + len(msg.Ns) + len(msg.Extra)
	size += rrCount * 200

	// Question section
	for _, q := range msg.Question {
		size += len(q.Name) + 16
	}

	// Rough estimate of rdata for common types
	for _, rr := range msg.Answer {
		size += len(rr.String())
	}

	return size
}

// emitQuery is a helper that emits a query event with domain/type info.
// Extracts a clean domain name from the FQDN (strips trailing dot).
func (c *DNSCache) emitQuery(qname string, qtype uint16, hit bool, ttl int) {
	domain := strings.TrimSuffix(strings.ToLower(qname), ".")
	c.Events.Emit(QueryEvent(domain, dns.TypeToString[qtype], hit, ttl, c.stats.Entries.Load()))
}
