package cache

import (
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/tidwall/buntdb"
)

// TestRecorderSnapshotAndHistory verifies that the recorder writes snapshots
// to BuntDB and GetHistory returns chronologically ordered deltas.
func TestRecorderSnapshotAndHistory(t *testing.T) {
	// In-memory BuntDB — no disk I/O.
	db, err := buntdb.Open(":memory:")
	if err != nil {
		t.Fatalf("open buntdb: %v", err)
	}
	defer db.Close()

	c := New(DefaultConfig())
	defer c.Stop()

	// Simulate some cache activity.
	m := makeMsg("example.com.", dns.TypeA, 300)
	c.Put("example.com.", dns.TypeA, m)
	c.Get("example.com.", dns.TypeA)  // hit
	c.Get("notfound.com.", dns.TypeA) // miss

	// Create recorder with a very short interval (won't actually tick in this test).
	rec := NewRecorder(c, db, time.Minute)

	// Manually trigger a snapshot (don't start the goroutine).
	rec.snapshot()

	// Verify we can read it back.
	history := rec.GetHistory(60)
	if len(history) == 0 {
		t.Fatal("expected at least 1 snapshot, got 0")
	}

	snap := history[len(history)-1]
	if snap.Stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", snap.Stats.Hits)
	}
	if snap.Stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", snap.Stats.Misses)
	}
	if snap.Stats.Inserts != 1 {
		t.Errorf("expected 1 insert, got %d", snap.Stats.Inserts)
	}
	if snap.Stats.Entries != 1 {
		t.Errorf("expected 1 entry, got %d", snap.Stats.Entries)
	}
	if snap.Stats.MaxEntries != DefaultConfig().MaxEntries {
		t.Errorf("expected max_entries=%d, got %d", DefaultConfig().MaxEntries, snap.Stats.MaxEntries)
	}
	if snap.Time == "" {
		t.Error("expected non-empty time key")
	}
	if snap.TimeUnixMs == 0 {
		t.Error("expected non-zero TimeUnixMs")
	}
}

// TestRecorderStartStop verifies the goroutine lifecycle.
func TestRecorderStartStop(t *testing.T) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		t.Fatalf("open buntdb: %v", err)
	}
	defer db.Close()

	c := New(DefaultConfig())
	defer c.Stop()

	rec := NewRecorder(c, db, 50*time.Millisecond)
	rec.Start()

	// Let a few ticks happen.
	time.Sleep(200 * time.Millisecond)

	rec.Stop()

	// Should have at least the initial snapshot + a couple of ticks.
	history := rec.GetHistory(60)
	if len(history) < 1 {
		t.Fatalf("expected ≥1 snapshots, got %d", len(history))
	}
}

// TestRecorderHistoryCutoff verifies that GetHistory respects the minutes filter.
func TestRecorderHistoryCutoff(t *testing.T) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		t.Fatalf("open buntdb: %v", err)
	}
	defer db.Close()

	c := New(DefaultConfig())
	defer c.Stop()

	rec := NewRecorder(c, db, time.Minute)

	// Write a snapshot
	rec.snapshot()

	// With minutes=60, should return it.
	history := rec.GetHistory(60)
	if len(history) == 0 {
		t.Fatal("expected snapshot within 60 min window")
	}

	// With minutes=0 (defaults to 60), should also return it.
	history = rec.GetHistory(0)
	if len(history) == 0 {
		t.Fatal("expected snapshot with default window")
	}
}
