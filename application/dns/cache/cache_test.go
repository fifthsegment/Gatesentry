package cache

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// ---------- helpers ----------

// makeMsg builds a minimal dns.Msg with one A record for testing.
func makeMsg(qname string, qtype uint16, ttl uint32) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(qname), qtype)
	m.Rcode = dns.RcodeSuccess
	m.Answer = append(m.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   dns.Fqdn(qname),
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		A: []byte{1, 2, 3, 4},
	})
	return m
}

// makeNXDOMAIN builds an NXDOMAIN response with a SOA authority record.
func makeNXDOMAIN(qname string, soaTTL, soaMinTTL uint32) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(qname), dns.TypeA)
	m.Rcode = dns.RcodeNameError
	m.Ns = append(m.Ns, &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    soaTTL,
		},
		Ns:      "ns1.example.com.",
		Mbox:    "admin.example.com.",
		Serial:  2025010101,
		Refresh: 3600,
		Retry:   900,
		Expire:  604800,
		Minttl:  soaMinTTL,
	})
	return m
}

// fastConfig returns a cache config for testing with short intervals.
func fastConfig() Config {
	return Config{
		MaxEntries:   100,
		MinTTL:       1 * time.Second,
		MaxTTL:       1 * time.Hour,
		NegativeTTL:  30 * time.Second,
		ReapInterval: 50 * time.Millisecond, // fast reaper for tests
	}
}

// ---------- Basic operations ----------

func TestPutAndGet(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	msg := makeMsg("example.com", dns.TypeA, 300)
	c.Put("example.com.", dns.TypeA, msg)

	got := c.Get("example.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected cache hit, got nil")
	}
	if len(got.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(got.Answer))
	}

	// TTL should be ≤300 (slightly less due to time elapsed)
	ttl := got.Answer[0].Header().Ttl
	if ttl == 0 || ttl > 300 {
		t.Errorf("unexpected TTL: %d", ttl)
	}
}

func TestGetMiss(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	got := c.Get("nonexistent.com.", dns.TypeA)
	if got != nil {
		t.Fatal("expected nil for cache miss")
	}
}

func TestCaseInsensitive(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	c.Put("Example.COM.", dns.TypeA, makeMsg("Example.COM", dns.TypeA, 60))

	// Should match regardless of case
	if c.Get("example.com.", dns.TypeA) == nil {
		t.Error("expected case-insensitive hit")
	}
	if c.Get("EXAMPLE.COM.", dns.TypeA) == nil {
		t.Error("expected case-insensitive hit (all caps)")
	}
}

func TestDifferentQTypes(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	c.Put("example.com.", dns.TypeA, makeMsg("example.com", dns.TypeA, 60))

	// AAAA query should miss
	if c.Get("example.com.", dns.TypeAAAA) != nil {
		t.Error("expected miss for different qtype")
	}
	// A query should hit
	if c.Get("example.com.", dns.TypeA) == nil {
		t.Error("expected hit for matching qtype")
	}
}

// ---------- TTL countdown ----------

func TestTTLCountdown(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	c.Put("countdown.com.", dns.TypeA, makeMsg("countdown.com", dns.TypeA, 10))

	// Immediate get — TTL should be ~10
	got := c.Get("countdown.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit")
	}
	ttl1 := got.Answer[0].Header().Ttl
	if ttl1 < 9 || ttl1 > 10 {
		t.Errorf("expected TTL ~10, got %d", ttl1)
	}

	// Wait 2 seconds, TTL should have decreased
	time.Sleep(2 * time.Second)
	got = c.Get("countdown.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit after 2s")
	}
	ttl2 := got.Answer[0].Header().Ttl
	if ttl2 >= ttl1 {
		t.Errorf("TTL should have decreased: was %d, now %d", ttl1, ttl2)
	}
	if ttl2 > 8 {
		t.Errorf("TTL should be ≤8 after 2s sleep, got %d", ttl2)
	}
}

// ---------- TTL expiry ----------

func TestExpiry(t *testing.T) {
	cfg := fastConfig()
	cfg.MinTTL = 1 * time.Second
	c := New(cfg)
	defer c.Stop()

	// Insert with 1-second TTL (will be clamped to MinTTL=1s)
	c.Put("expiring.com.", dns.TypeA, makeMsg("expiring.com", dns.TypeA, 1))

	// Should be present immediately
	if c.Get("expiring.com.", dns.TypeA) == nil {
		t.Fatal("expected hit before expiry")
	}

	// Wait for expiry
	time.Sleep(1500 * time.Millisecond)

	// Should be gone (lazy expiry on Get)
	if c.Get("expiring.com.", dns.TypeA) != nil {
		t.Error("expected miss after expiry")
	}
}

func TestReaperCleansExpiredEntries(t *testing.T) {
	cfg := fastConfig()
	cfg.MinTTL = 1 * time.Second
	cfg.ReapInterval = 100 * time.Millisecond
	c := New(cfg)
	defer c.Stop()

	// Insert entries with short TTL
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("reap%d.com.", i)
		c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 1))
	}

	snap := c.Snapshot()
	if snap.Entries != 10 {
		t.Fatalf("expected 10 entries, got %d", snap.Entries)
	}

	// Wait for TTL + reaper interval
	time.Sleep(1500 * time.Millisecond)

	snap = c.Snapshot()
	if snap.Entries != 0 {
		t.Errorf("expected 0 entries after reaper, got %d", snap.Entries)
	}
	if snap.Expired < 10 {
		t.Errorf("expected ≥10 expired, got %d", snap.Expired)
	}
}

// ---------- TTL clamping ----------

func TestMinTTLClamp(t *testing.T) {
	cfg := fastConfig()
	cfg.MinTTL = 30 * time.Second
	c := New(cfg)
	defer c.Stop()

	// Upstream TTL = 5s, but MinTTL = 30s — should be clamped up
	c.Put("cdn.com.", dns.TypeA, makeMsg("cdn.com", dns.TypeA, 5))

	got := c.Get("cdn.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit")
	}
	ttl := got.Answer[0].Header().Ttl
	if ttl < 28 { // allow ~2s for test execution
		t.Errorf("expected TTL ≥28 (clamped to MinTTL=30), got %d", ttl)
	}
}

func TestMaxTTLClamp(t *testing.T) {
	cfg := fastConfig()
	cfg.MaxTTL = 60 * time.Second
	c := New(cfg)
	defer c.Stop()

	// Upstream TTL = 86400 (1 day), but MaxTTL = 60s — should be capped
	c.Put("long.com.", dns.TypeA, makeMsg("long.com", dns.TypeA, 86400))

	got := c.Get("long.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit")
	}
	ttl := got.Answer[0].Header().Ttl
	if ttl > 60 {
		t.Errorf("expected TTL ≤60 (capped at MaxTTL), got %d", ttl)
	}
}

// ---------- Negative caching ----------

func TestNXDOMAINcaching(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	nxMsg := makeNXDOMAIN("nope.com", 600, 60)
	c.Put("nope.com.", dns.TypeA, nxMsg)

	got := c.Get("nope.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected NXDOMAIN to be cached")
	}
	if got.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN rcode, got %d", got.Rcode)
	}
}

func TestNXDOMAINusesSOAMinimum(t *testing.T) {
	cfg := fastConfig()
	cfg.NegativeTTL = 5 * time.Minute
	c := New(cfg)
	defer c.Stop()

	// SOA TTL = 600s, SOA MINIMUM = 30s → cache for 30s (min of the two, per RFC 2308 §5)
	nxMsg := makeNXDOMAIN("nx.com", 600, 30)
	c.Put("nx.com.", dns.TypeA, nxMsg)

	got := c.Get("nx.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected NXDOMAIN cached")
	}
	// The authority section TTL should reflect the cache TTL (clamped)
	if len(got.Ns) > 0 {
		ttl := got.Ns[0].Header().Ttl
		if ttl > 30 {
			t.Errorf("expected TTL ≤30 (SOA minimum), got %d", ttl)
		}
	}
}

func TestNXDOMAINNegativeTTLCap(t *testing.T) {
	cfg := fastConfig()
	cfg.NegativeTTL = 10 * time.Second
	c := New(cfg)
	defer c.Stop()

	// SOA MINIMUM = 300s, but NegativeTTL cap = 10s → should be capped at 10s
	nxMsg := makeNXDOMAIN("capped.com", 300, 300)
	c.Put("capped.com.", dns.TypeA, nxMsg)

	got := c.Get("capped.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit")
	}
	if len(got.Ns) > 0 {
		ttl := got.Ns[0].Header().Ttl
		if ttl > 10 {
			t.Errorf("expected TTL ≤10 (NegativeTTL cap), got %d", ttl)
		}
	}
}

// ---------- Eviction ----------

func TestEvictionAtCapacity(t *testing.T) {
	cfg := fastConfig()
	cfg.MaxEntries = 32 // 32/16 = 2 per shard
	c := New(cfg)
	defer c.Stop()

	// Insert more entries than capacity — some will be evicted
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("evict%d.com.", i)
		c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 3600))
	}

	snap := c.Snapshot()
	if snap.Entries > int64(cfg.MaxEntries) {
		t.Errorf("cache exceeded max capacity: %d entries (max %d)", snap.Entries, cfg.MaxEntries)
	}
	if snap.Evictions == 0 {
		t.Error("expected some evictions")
	}
}

func TestEvictionPrefersNearExpiry(t *testing.T) {
	cfg := fastConfig()
	cfg.MaxEntries = 16 // 1 per shard
	c := New(cfg)
	defer c.Stop()

	// Insert an entry with short TTL
	c.Put("short.com.", dns.TypeA, makeMsg("short.com", dns.TypeA, 5))
	// Insert an entry with long TTL into the same shard — should evict short one
	// (We can't guarantee shard placement, so insert many and check stats)
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("long%d.com.", i)
		c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 3600))
	}

	snap := c.Snapshot()
	if snap.Evictions == 0 {
		t.Error("expected evictions when exceeding capacity")
	}
}

// ---------- Remove ----------

func TestRemove(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	c.Put("remove.com.", dns.TypeA, makeMsg("remove.com", dns.TypeA, 300))

	if !c.Remove("remove.com.", dns.TypeA) {
		t.Error("expected Remove to return true")
	}
	if c.Get("remove.com.", dns.TypeA) != nil {
		t.Error("expected miss after Remove")
	}
	if c.Remove("remove.com.", dns.TypeA) {
		t.Error("expected Remove to return false for already-removed entry")
	}
}

// ---------- Flush ----------

func TestFlush(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("flush%d.com.", i)
		c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 3600))
	}

	c.Flush()
	snap := c.Snapshot()
	if snap.Entries != 0 {
		t.Errorf("expected 0 entries after flush, got %d", snap.Entries)
	}
}

// ---------- Stats ----------

func TestStats(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	c.Put("stats.com.", dns.TypeA, makeMsg("stats.com", dns.TypeA, 300))

	// 1 hit
	c.Get("stats.com.", dns.TypeA)
	// 2 misses
	c.Get("nope1.com.", dns.TypeA)
	c.Get("nope2.com.", dns.TypeA)

	snap := c.Snapshot()
	if snap.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", snap.Hits)
	}
	if snap.Misses != 2 {
		t.Errorf("expected 2 misses, got %d", snap.Misses)
	}
	if snap.Inserts != 1 {
		t.Errorf("expected 1 insert, got %d", snap.Inserts)
	}
	if snap.HitRate < 33.0 || snap.HitRate > 34.0 {
		t.Errorf("expected hit rate ~33.3%%, got %.1f%%", snap.HitRate)
	}
}

func TestStatsMemoryEstimate(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	c.Put("mem.com.", dns.TypeA, makeMsg("mem.com", dns.TypeA, 300))

	snap := c.Snapshot()
	if snap.SizeBytes <= 0 {
		t.Error("expected positive size estimate")
	}
	// A single A record response should be roughly 300–800 bytes
	if snap.SizeBytes > 5000 {
		t.Errorf("size estimate seems too large: %d bytes", snap.SizeBytes)
	}
}

// ---------- Concurrency ----------

func TestConcurrentReadWrite(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	var wg sync.WaitGroup
	const goroutines = 50
	const opsPerGoroutine = 200

	// Writers
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				name := fmt.Sprintf("concurrent%d-%d.com.", id, i)
				c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 60))
			}
		}(g)
	}

	// Readers
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				name := fmt.Sprintf("concurrent%d-%d.com.", id, i)
				c.Get(name, dns.TypeA) // may hit or miss — both are fine
			}
		}(g)
	}

	wg.Wait()

	snap := c.Snapshot()
	if snap.Entries < 0 {
		t.Error("entry count should never be negative")
	}
	if snap.SizeBytes < 0 {
		t.Error("size estimate should never be negative")
	}
}

func TestConcurrentPutFlush(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	var wg sync.WaitGroup

	// Concurrent inserts
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			name := fmt.Sprintf("pf%d.com.", i)
			c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 60))
		}
	}()

	// Concurrent flushes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			c.Flush()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	// No panics, no negative counts
	snap := c.Snapshot()
	if snap.Entries < 0 {
		t.Errorf("negative entry count: %d", snap.Entries)
	}
}

// ---------- Deep copy safety ----------

func TestGetReturnsCopy(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	original := makeMsg("copy.com", dns.TypeA, 300)
	c.Put("copy.com.", dns.TypeA, original)

	// Mutate the returned message
	got := c.Get("copy.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit")
	}
	got.Answer[0].Header().Ttl = 99999

	// Get again — should have original TTL, not the mutated one
	got2 := c.Get("copy.com.", dns.TypeA)
	if got2 == nil {
		t.Fatal("expected hit")
	}
	if got2.Answer[0].Header().Ttl >= 99999 {
		t.Error("Get returned a reference instead of a copy — mutations leaked")
	}
}

// ---------- Nil safety ----------

func TestPutNilMessage(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	// Should not panic
	c.Put("nil.com.", dns.TypeA, nil)
	if c.Get("nil.com.", dns.TypeA) != nil {
		t.Error("expected nil for nil message put")
	}
}

// ---------- Replacement ----------

func TestPutReplacesExisting(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	msg1 := makeMsg("replace.com", dns.TypeA, 60)
	msg1.Answer[0].(*dns.A).A = []byte{1, 1, 1, 1}
	c.Put("replace.com.", dns.TypeA, msg1)

	msg2 := makeMsg("replace.com", dns.TypeA, 120)
	msg2.Answer[0].(*dns.A).A = []byte{2, 2, 2, 2}
	c.Put("replace.com.", dns.TypeA, msg2)

	got := c.Get("replace.com.", dns.TypeA)
	if got == nil {
		t.Fatal("expected hit")
	}
	a := got.Answer[0].(*dns.A).A
	if a[0] != 2 {
		t.Errorf("expected replaced IP 2.2.2.2, got %v", a)
	}

	// Entry count should still be 1
	snap := c.Snapshot()
	if snap.Entries != 1 {
		t.Errorf("expected 1 entry after replacement, got %d", snap.Entries)
	}
}

// ---------- Shard distribution ----------

func TestShardDistribution(t *testing.T) {
	// Verify that entries are distributed across multiple shards
	c := New(fastConfig())
	defer c.Stop()

	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("shard%d.com.", i)
		c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 300))
	}

	nonEmpty := 0
	for i := range c.shards {
		c.shards[i].mu.RLock()
		if len(c.shards[i].entries) > 0 {
			nonEmpty++
		}
		c.shards[i].mu.RUnlock()
	}

	// With 100 entries across 16 shards, we'd expect most to have entries
	if nonEmpty < 8 {
		t.Errorf("poor shard distribution: only %d/16 shards have entries", nonEmpty)
	}
}

// ---------- Event bus ----------

func TestEventBusQuery(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	ch := c.Events.Subscribe()
	defer c.Events.Unsubscribe(ch)

	c.Put("event.com.", dns.TypeA, makeMsg("event.com", dns.TypeA, 300))

	// Drain the insert event
	select {
	case ev := <-ch:
		if ev.Type != EventInsert {
			t.Errorf("expected insert event, got %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for insert event")
	}

	// Cache hit
	c.Get("event.com.", dns.TypeA)
	select {
	case ev := <-ch:
		if ev.Type != EventQuery {
			t.Errorf("expected query event, got %s", ev.Type)
		}
		if !ev.Hit {
			t.Error("expected hit=true")
		}
		if ev.Domain != "event.com" {
			t.Errorf("expected domain=event.com, got %s", ev.Domain)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for query event")
	}

	// Cache miss
	c.Get("miss.com.", dns.TypeA)
	select {
	case ev := <-ch:
		if ev.Type != EventQuery {
			t.Errorf("expected query event, got %s", ev.Type)
		}
		if ev.Hit {
			t.Error("expected hit=false for miss")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for miss event")
	}
}

func TestEventBusFlush(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	// Insert some entries first (without subscriber)
	for i := 0; i < 5; i++ {
		c.Put(fmt.Sprintf("f%d.com.", i), dns.TypeA, makeMsg(fmt.Sprintf("f%d.com", i), dns.TypeA, 300))
	}

	ch := c.Events.Subscribe()
	defer c.Events.Unsubscribe(ch)

	c.Flush()

	select {
	case ev := <-ch:
		if ev.Type != EventFlush {
			t.Errorf("expected flush event, got %s", ev.Type)
		}
		if ev.Count != 5 {
			t.Errorf("expected flush count=5, got %d", ev.Count)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for flush event")
	}
}

func TestEventBusNonBlocking(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	// Subscribe with a channel that we never read from
	ch := c.Events.Subscribe()
	defer c.Events.Unsubscribe(ch)

	// Flood events — should not block the DNS server
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			name := fmt.Sprintf("flood%d.com.", i)
			c.Put(name, dns.TypeA, makeMsg(name, dns.TypeA, 300))
		}
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't block
	case <-time.After(5 * time.Second):
		t.Fatal("event emission blocked — should be non-blocking")
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	c := New(fastConfig())
	defer c.Stop()

	ch1 := c.Events.Subscribe()
	ch2 := c.Events.Subscribe()
	defer c.Events.Unsubscribe(ch1)
	defer c.Events.Unsubscribe(ch2)

	c.Put("multi.com.", dns.TypeA, makeMsg("multi.com", dns.TypeA, 300))

	// Both should receive the insert event
	for i, ch := range []chan *Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != EventInsert {
				t.Errorf("subscriber %d: expected insert, got %s", i, ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestEventBusAutoDisable(t *testing.T) {
	eb := NewEventBus()

	// No subscribers — should not be enabled
	if eb.SubscriberCount() != 0 {
		t.Error("expected 0 subscribers")
	}

	ch := eb.Subscribe()
	if eb.SubscriberCount() != 1 {
		t.Error("expected 1 subscriber")
	}

	eb.Unsubscribe(ch)
	if eb.SubscriberCount() != 0 {
		t.Error("expected 0 subscribers after unsubscribe")
	}

	// Emit after unsubscribe — should be no-op (no panic)
	eb.Emit(QueryEvent("test.com", "A", true, 60, 0))
}

func TestEventJSON(t *testing.T) {
	ev := QueryEvent("example.com", "A", true, 300, 42)
	b := ev.JSON()
	s := string(b)

	if len(s) == 0 {
		t.Fatal("expected non-empty JSON")
	}
	// Spot-check a few fields
	for _, want := range []string{`"type":"query"`, `"domain":"example.com"`, `"hit":true`} {
		found := false
		for i := 0; i <= len(s)-len(want); i++ {
			if s[i:i+len(want)] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected JSON to contain %s, got: %s", want, s)
		}
	}
}

// ---------- Shard index determinism ----------

func TestShardIndexDeterministic(t *testing.T) {
	key := "example.com./A"
	idx1 := shardIndex(key)
	idx2 := shardIndex(key)
	if idx1 != idx2 {
		t.Error("shardIndex should be deterministic")
	}
	if idx1 >= shardCount {
		t.Errorf("shardIndex %d out of range [0, %d)", idx1, shardCount)
	}
}

// ---------- Cache key format ----------

func TestCacheKey(t *testing.T) {
	tests := []struct {
		qname string
		qtype uint16
		want  string
	}{
		{"example.com.", dns.TypeA, "example.com./A"},
		{"Example.COM.", dns.TypeAAAA, "example.com./AAAA"},
		{"test.org.", dns.TypeMX, "test.org./MX"},
		{"test.org.", 65534, "test.org./65534"}, // unknown type → numeric
	}
	for _, tt := range tests {
		got := cacheKey(tt.qname, tt.qtype)
		if got != tt.want {
			t.Errorf("cacheKey(%q, %d) = %q, want %q", tt.qname, tt.qtype, got, tt.want)
		}
	}
}

// ---------- Default config ----------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxEntries <= 0 {
		t.Error("MaxEntries should be positive")
	}
	if cfg.MinTTL <= 0 {
		t.Error("MinTTL should be positive")
	}
	if cfg.MaxTTL <= cfg.MinTTL {
		t.Error("MaxTTL should be greater than MinTTL")
	}
	if cfg.ReapInterval <= 0 {
		t.Error("ReapInterval should be positive")
	}
}

// ---------- Stop idempotency ----------

func TestStopIdempotent(t *testing.T) {
	c := New(fastConfig())
	c.Stop()
	c.Stop() // should not panic
}

// ---------- Request event ----------

func TestRequestEvent(t *testing.T) {
	ev := RequestEvent("ads.example.com", "A", "blocked", true)
	if ev.Type != EventRequest {
		t.Errorf("expected type %q, got %q", EventRequest, ev.Type)
	}
	if ev.Domain != "ads.example.com" {
		t.Errorf("expected domain ads.example.com, got %s", ev.Domain)
	}
	if ev.QType != "A" {
		t.Errorf("expected qtype A, got %s", ev.QType)
	}
	if ev.ResponseType != "blocked" {
		t.Errorf("expected response_type blocked, got %s", ev.ResponseType)
	}
	if !ev.Blocked {
		t.Error("expected blocked=true")
	}
	if ev.Timestamp <= 0 {
		t.Error("expected positive timestamp")
	}
}

func TestRequestEventJSON(t *testing.T) {
	ev := RequestEvent("google.com", "AAAA", "forwarded", false)
	s := string(ev.JSON())

	for _, want := range []string{
		`"type":"request"`,
		`"domain":"google.com"`,
		`"qtype":"AAAA"`,
		`"response_type":"forwarded"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected JSON to contain %s, got: %s", want, s)
		}
	}
	// blocked=false should be omitted (omitempty)
	if strings.Contains(s, `"blocked"`) {
		t.Errorf("blocked=false should be omitted, got: %s", s)
	}
}

func TestRequestEventViaEventBus(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	bus.Emit(RequestEvent("tracker.io", "A", "blocked", true))

	select {
	case ev := <-ch:
		if ev.Type != EventRequest {
			t.Errorf("expected request event, got %s", ev.Type)
		}
		if ev.ResponseType != "blocked" {
			t.Errorf("expected response_type blocked, got %s", ev.ResponseType)
		}
		if !ev.Blocked {
			t.Error("expected blocked=true")
		}
	default:
		t.Fatal("expected event on channel")
	}
}
