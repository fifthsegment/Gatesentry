package gatesentry2logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tidwall/buntdb"
)

func openBatchTestLogger(t testing.TB, options LoggerOptions) *Log {
	t.Helper()
	l, err := OpenLoggerWithOptions(t.TempDir()+"/batch.db", options)
	if err != nil {
		t.Fatalf("open logger: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := l.Close(ctx); err != nil {
			t.Errorf("close logger: %v", err)
		}
	})
	return l
}

func flushTestLogger(t testing.TB, l *Log) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.Flush(ctx); err != nil {
		t.Fatalf("flush logger: %v", err)
	}
}

func TestBatchPreservesSchemaIndexAndRetention(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{FlushInterval: time.Hour})
	l.LogDNS("quoted\\\"domain.example", "192.0.2.1", "forward")
	flushTestLogger(t, l)

	indexes, err := l.Database.Indexes()
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 || indexes[0] != "entries" {
		t.Fatalf("indexes = %v, want entries", indexes)
	}
	var count int
	if err := l.Database.View(func(tx *buntdb.Tx) error {
		return tx.Ascend("entries", func(key, value string) bool {
			count++
			var entry LogEntry
			if err := json.Unmarshal([]byte(value), &entry); err != nil {
				t.Errorf("stored JSON: %v", err)
				return false
			}
			if entry.URL != "quoted\\\"domain.example" || entry.Type != "dns" || entry.DNSResponseType != "forward" {
				t.Errorf("stored entry = %+v", entry)
				return false
			}
			ttl, err := tx.TTL(key)
			if err != nil {
				t.Errorf("TTL: %v", err)
			} else if ttl <= Log_Entry_Expires-time.Minute || ttl > Log_Entry_Expires {
				t.Errorf("TTL = %v, want approximately %v", ttl, Log_Entry_Expires)
			}
			return true
		})
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored entries = %d, want 1", count)
	}
}

func TestWriterBatchesBySize(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{QueueCapacity: 16, BatchSize: 3, FlushInterval: time.Hour})
	originalUpdate := l.update
	var updates atomic.Int64
	l.update = func(fn func(*buntdb.Tx) error) error {
		updates.Add(1)
		return originalUpdate(fn)
	}

	for i := 0; i < 6; i++ {
		l.LogProxy(fmt.Sprintf("https://batch-%d.example", i), "192.0.2.1", "allowed")
	}
	flushTestLogger(t, l)

	if got := updates.Load(); got != 2 {
		t.Fatalf("database updates = %d, want 2 size-triggered batches", got)
	}
	stats := l.Stats()
	if stats.Accepted != 6 || stats.Persisted != 6 || stats.Dropped != 0 || stats.QueueDepth != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestWriterFlushesPartialBatchOnTimer(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{QueueCapacity: 8, BatchSize: 8, FlushInterval: 10 * time.Millisecond})
	l.LogProxy("https://timer.example", "192.0.2.1", "allowed")

	deadline := time.Now().Add(time.Second)
	for l.Stats().Persisted != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stats := l.Stats(); stats.Persisted != 1 {
		t.Fatalf("timer did not persist partial batch: %+v", stats)
	}
}

func TestFlushWaitsForInFlightBatch(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{QueueCapacity: 8, BatchSize: 1, FlushInterval: time.Hour})
	originalUpdate := l.update
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	l.update = func(fn func(*buntdb.Tx) error) error {
		once.Do(func() {
			close(started)
			<-release
		})
		return originalUpdate(fn)
	}
	l.LogProxy("https://blocked-writer.example", "192.0.2.1", "allowed")
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.Flush(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Flush error = %v, want deadline exceeded", err)
	}
	close(release)
	flushTestLogger(t, l)
	if got := l.Stats().Persisted; got != 1 {
		t.Fatalf("persisted = %d, want 1", got)
	}
}

func TestQueueSaturationUsesBoundedBackpressure(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{QueueCapacity: 1, BatchSize: 1, FlushInterval: time.Hour, EnqueueWait: 15 * time.Millisecond})
	originalUpdate := l.update
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	l.update = func(fn func(*buntdb.Tx) error) error {
		once.Do(func() {
			close(started)
			<-release
		})
		return originalUpdate(fn)
	}

	l.LogProxy("https://first.example", "192.0.2.1", "allowed")
	<-started
	if got := l.Stats().QueueDepth; got != 0 {
		t.Fatalf("queue depth with only an in-flight batch = %d, want 0", got)
	}
	l.LogProxy("https://queued.example", "192.0.2.1", "allowed")
	if got := l.Stats().QueueDepth; got != 1 {
		t.Fatalf("queue depth with one buffered entry = %d, want 1", got)
	}
	startedAt := time.Now()
	l.LogProxy("https://dropped.example", "192.0.2.1", "allowed")
	elapsed := time.Since(startedAt)
	if elapsed < 10*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("saturated enqueue waited %v, want brief bounded backpressure", elapsed)
	}
	if got := l.Stats().Dropped; got != 1 {
		t.Fatalf("dropped = %d, want 1", got)
	}
	close(release)
	flushTestLogger(t, l)
	if stats := l.Stats(); stats.Accepted != 2 || stats.Persisted != 2 {
		t.Fatalf("unexpected final stats: %+v", stats)
	}
}

func TestWriterRetriesAcceptedBatch(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{QueueCapacity: 8, BatchSize: 2, FlushInterval: time.Hour})
	originalUpdate := l.update
	var attempts atomic.Int64
	l.update = func(fn func(*buntdb.Tx) error) error {
		if attempts.Add(1) == 1 {
			return errors.New("transient write failure")
		}
		return originalUpdate(fn)
	}
	l.LogProxy("https://retry-1.example", "192.0.2.1", "allowed")
	l.LogProxy("https://retry-2.example", "192.0.2.1", "allowed")
	flushTestLogger(t, l)
	stats := l.Stats()
	if attempts.Load() != 2 || stats.WriteErrors != 1 || stats.Persisted != 2 || stats.LastError == "" {
		t.Fatalf("retry attempts=%d stats=%+v", attempts.Load(), stats)
	}
}

func TestCloseTimeoutPreservesFailedAcceptedBatch(t *testing.T) {
	l, err := OpenLoggerWithOptions(t.TempDir()+"/retry-after-timeout.db", LoggerOptions{QueueCapacity: 8, BatchSize: 1, FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	originalUpdate := l.update
	transientErr := errors.New("transient write failure")
	var attempts atomic.Int64
	attempted := make(chan struct{})
	recoverWrites := make(chan struct{})
	var once sync.Once
	l.update = func(fn func(*buntdb.Tx) error) error {
		attempts.Add(1)
		once.Do(func() { close(attempted) })
		select {
		case <-recoverWrites:
			return originalUpdate(fn)
		default:
			return transientErr
		}
	}
	l.LogProxy("https://retry-after-timeout.example", "192.0.2.1", "allowed")
	<-attempted

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := l.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	close(recoverWrites)

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := l.Close(ctx2); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := l.Database.View(func(*buntdb.Tx) error { return nil }); !errors.Is(err, buntdb.ErrDatabaseClosed) {
		t.Fatalf("database View error = %v, want database closed", err)
	}
	stats := l.Stats()
	if stats.Accepted != 1 || stats.Persisted != 1 || stats.WriteErrors == 0 || stats.LastError == "" {
		t.Fatalf("unexpected recovery stats: %+v", stats)
	}
}

func TestCloseDrainsAndIsIdempotent(t *testing.T) {
	l, err := OpenLoggerWithOptions(t.TempDir()+"/close.db", LoggerOptions{QueueCapacity: 8, BatchSize: 8, FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	l.LogProxy("https://close.example", "192.0.2.1", "allowed")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.Close(ctx); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := l.Close(ctx); err != nil {
		t.Fatalf("second close: %v", err)
	}
	before := l.Stats()
	l.LogProxy("https://after-close.example", "192.0.2.1", "allowed")
	after := l.Stats()
	if before.Persisted != 1 || after.Dropped != before.Dropped+1 || after.Accepted != before.Accepted {
		t.Fatalf("before=%+v after=%+v", before, after)
	}
}

func TestCloseTimeoutContinuesDraining(t *testing.T) {
	l, err := OpenLoggerWithOptions(t.TempDir()+"/close-timeout.db", LoggerOptions{QueueCapacity: 8, BatchSize: 1, FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	originalUpdate := l.update
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	l.update = func(fn func(*buntdb.Tx) error) error {
		once.Do(func() {
			close(started)
			<-release
		})
		return originalUpdate(fn)
	}
	l.LogProxy("https://close-timeout.example", "192.0.2.1", "allowed")
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := l.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want deadline exceeded", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := l.Close(ctx2); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if got := l.Stats().Persisted; got != 1 {
		t.Fatalf("persisted = %d, want 1", got)
	}
}

func TestConcurrentEnqueueAndClose(t *testing.T) {
	l, err := OpenLoggerWithOptions(t.TempDir()+"/concurrent-close.db", LoggerOptions{QueueCapacity: 32, BatchSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	var producers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		producers.Add(1)
		go func(worker int) {
			defer producers.Done()
			for i := 0; i < 100; i++ {
				l.LogProxy(fmt.Sprintf("https://concurrent-%d-%d.example", worker, i), "192.0.2.1", "allowed")
			}
		}(worker)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	producers.Wait()
	stats := l.Stats()
	if stats.Persisted != stats.Accepted || stats.Accepted+stats.Dropped != 800 {
		t.Fatalf("unexpected stats after concurrent close: %+v", stats)
	}
}

func TestOpenLoggerReportsInitializationError(t *testing.T) {
	if l, err := OpenLogger(t.TempDir()); err == nil || l != nil {
		t.Fatalf("OpenLogger(directory) = (%v, %v), want initialization error", l, err)
	}
}

func TestOpenLoggerUsesEverySecondSyncPolicy(t *testing.T) {
	l := openBatchTestLogger(t, LoggerOptions{})
	var config buntdb.Config
	if err := l.Database.ReadConfig(&config); err != nil {
		t.Fatal(err)
	}
	if config.SyncPolicy != buntdb.EverySecond {
		t.Fatalf("SyncPolicy = %v, want EverySecond", config.SyncPolicy)
	}
}

// BenchmarkLogProxyBatchedPersistence is directly comparable with the legacy
// 1,000-entry end-to-end benchmark, but Flush provides an honest persistence
// barrier before each iteration completes.
func BenchmarkLogProxyBatchedPersistence(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		l, err := OpenLoggerWithOptions(b.TempDir()+fmt.Sprintf("/persist-%d.db", i), LoggerOptions{})
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		for n := 0; n < 1000; n++ {
			l.LogProxy("https://benchmark.example/resource", "192.0.2.1", "allowed")
		}
		flushTestLogger(b, l)
		b.StopTimer()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := l.Close(ctx); err != nil {
			cancel()
			b.Fatal(err)
		}
		cancel()
	}
}

// BenchmarkLogProxyBatchedAdmission measures producer-side admission of the
// same 10,000 calls as the legacy benchmark and Flushes before ending the
// iteration so no persistence work escapes the measured operation.
func BenchmarkLogProxyBatchedAdmission(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		l, err := OpenLoggerWithOptions(b.TempDir()+fmt.Sprintf("/admit-%d.db", i), LoggerOptions{QueueCapacity: 16384})
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		for n := 0; n < 10000; n++ {
			l.LogProxy("https://benchmark.example/resource", "192.0.2.1", "allowed")
		}
		flushTestLogger(b, l)
		b.StopTimer()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := l.Close(ctx); err != nil {
			cancel()
			b.Fatal(err)
		}
		cancel()
	}
}

func TestConcurrentSaturatedProducersWaitIndependently(t *testing.T) {
	const producerCount = 12
	l := openBatchTestLogger(t, LoggerOptions{QueueCapacity: 1, BatchSize: 1, FlushInterval: time.Hour, EnqueueWait: 20 * time.Millisecond})
	originalUpdate := l.update
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	l.update = func(fn func(*buntdb.Tx) error) error {
		once.Do(func() {
			close(started)
			<-release
		})
		return originalUpdate(fn)
	}

	l.LogProxy("https://in-flight.example", "192.0.2.1", "allowed")
	<-started
	l.LogProxy("https://queued.example", "192.0.2.1", "allowed")

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(producerCount)
	startedAt := time.Now()
	for i := 0; i < producerCount; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			l.LogProxy(fmt.Sprintf("https://dropped-%d.example", i), "192.0.2.1", "allowed")
		}(i)
	}
	close(start)
	wg.Wait()
	elapsed := time.Since(startedAt)
	if elapsed > 150*time.Millisecond {
		t.Fatalf("concurrent saturated producers waited %v; waits appear serialized", elapsed)
	}
	if got := l.Stats().Dropped; got != producerCount {
		t.Fatalf("dropped = %d, want %d", got, producerCount)
	}
	close(release)
	flushTestLogger(t, l)
}
