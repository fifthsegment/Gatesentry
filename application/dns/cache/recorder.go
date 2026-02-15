package cache

import (
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/buntdb"
)

// snapshotTTL controls how long each per-minute snapshot lives in BuntDB.
// 24 hours gives enough runway for the UI "past hour" view while bounding
// storage at ≤ 1,440 entries (~300 KB).
const snapshotTTL = 24 * time.Hour

// keyPrefix tags all snapshot keys in BuntDB so they can be scanned without
// colliding with the existing request-log keys.
const keyPrefix = "cache:stats:"

// TimestampedSnapshot is a StatsSnapshot with a wall-clock timestamp attached
// so the API consumer can build a time-series from the ordered slice.
type TimestampedSnapshot struct {
	Time       string        `json:"time"`         // "YYYY-MM-DDTHH:MM" local-time key
	TimeUnixMs int64         `json:"time_unix_ms"` // Unix millis for JS convenience
	Stats      StatsSnapshot `json:"stats"`
}

// Recorder periodically snapshots DNS cache counters into BuntDB.
//
// Why BuntDB rather than a separate file?  The existing logger already owns a
// BuntDB instance that persists across restarts.  Reusing it means zero extra
// file handles, zero extra fsync contention, and the snapshots survive binary
// restarts just like request logs.
//
// Write cost: one tiny JSON blob per minute → < 0.02 IOPS.
type Recorder struct {
	cache    *DNSCache
	db       *buntdb.DB
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewRecorder creates a recorder that snapshots the cache stats every
// `interval` into the given BuntDB database.  Call Start() to begin.
func NewRecorder(cache *DNSCache, db *buntdb.DB, interval time.Duration) *Recorder {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Recorder{
		cache:    cache,
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background recording goroutine.
// It is safe to call Start only once.
func (r *Recorder) Start() {
	r.wg.Add(1)
	go r.loop()
	log.Printf("[Cache Recorder] Started — interval=%s, TTL=%s", r.interval, snapshotTTL)
}

// Stop signals the recorder goroutine to exit and waits for it to finish.
func (r *Recorder) Stop() {
	close(r.stopCh)
	r.wg.Wait()
	log.Println("[Cache Recorder] Stopped")
}

// loop runs in a dedicated goroutine, snapshotting once per interval.
func (r *Recorder) loop() {
	defer r.wg.Done()

	// Take one snapshot immediately so the UI has data right after startup.
	r.snapshot()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.snapshot()
		}
	}
}

// snapshot takes a point-in-time copy of the cache stats and writes it
// into BuntDB with a local-time minute key and a 24-hour TTL.
func (r *Recorder) snapshot() {
	snap := r.cache.SnapshotAndReset()
	now := time.Now()

	// Key format matches the frontend's bucketKey for easy merging.
	key := keyPrefix + now.Local().Format("2006-01-02T15:04")

	data, err := json.Marshal(snap)
	if err != nil {
		log.Printf("[Cache Recorder] marshal error: %v", err)
		return
	}

	err = r.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(key, string(data), &buntdb.SetOptions{
			Expires: true,
			TTL:     snapshotTTL,
		})
		return err
	})
	if err != nil {
		log.Printf("[Cache Recorder] write error: %v", err)
	} else {
		log.Printf("[Cache Recorder] Snapshot written: key=%s, hits=%d, misses=%d", key, snap.Hits, snap.Misses)
	}
}

// GetHistory reads the last `minutes` worth of snapshots from BuntDB
// and returns them in chronological order.
//
// The returned slice contains raw cumulative counters.  The frontend
// computes deltas (snapshot[i] − snapshot[i-1]) to derive per-minute
// rates for the chart.
func (r *Recorder) GetHistory(minutes int) []TimestampedSnapshot {
	if minutes <= 0 {
		minutes = 60
	}

	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute)
	cutoffKey := keyPrefix + cutoff.Local().Format("2006-01-02T15:04")

	log.Printf("[Cache Recorder] GetHistory: looking for snapshots >= %s (last %d min)", cutoffKey, minutes)

	var results []TimestampedSnapshot

	err := r.db.View(func(tx *buntdb.Tx) error {
		count := 0
		return tx.AscendGreaterOrEqual("", cutoffKey, func(key, value string) bool {
			count++
			if !strings.HasPrefix(key, keyPrefix) {
				return true // skip non-snapshot keys
			}

			log.Printf("[Cache Recorder] Found snapshot key: %s", key)

			timeStr := strings.TrimPrefix(key, keyPrefix)

			var snap StatsSnapshot
			if err := json.Unmarshal([]byte(value), &snap); err != nil {
				log.Printf("[Cache Recorder] unmarshal error for key %s: %v", key, err)
				return true
			}

			// Parse the local-time key back to Unix millis for the frontend.
			t, err := time.ParseInLocation("2006-01-02T15:04", timeStr, time.Local)
			if err != nil {
				log.Printf("[Cache Recorder] time parse error for key %s: %v", key, err)
				return true
			}

			results = append(results, TimestampedSnapshot{
				Time:       timeStr,
				TimeUnixMs: t.UnixMilli(),
				Stats:      snap,
			})
			return true
		})
	})
	if err != nil {
		log.Printf("[Cache Recorder] read error: %v", err)
	}

	// Belt-and-suspenders: ensure chronological order.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time < results[j].Time
	})

	log.Printf("[Cache Recorder] GetHistory: returning %d snapshots for last %d minutes", len(results), minutes)
	return results
}
