package gatesentry2logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2utils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
	"github.com/tidwall/buntdb"
)

var Log_Entry_Expires = 7 * 24 * time.Hour

const (
	defaultQueueCapacity = 4096
	defaultBatchSize     = 128
	defaultFlushInterval = 100 * time.Millisecond
	defaultEnqueueWait   = 10 * time.Millisecond
	initialRetryDelay    = 10 * time.Millisecond
	maximumRetryDelay    = time.Second
)

var ErrLoggerClosed = errors.New("logger is closed")

// LoggerOptions controls the bounded asynchronous writer. Zero values use
// defaults chosen for low-resource gateways.
type LoggerOptions struct {
	QueueCapacity int
	BatchSize     int
	FlushInterval time.Duration
	EnqueueWait   time.Duration
}

// LoggerStats is a point-in-time health snapshot. It deliberately contains no
// decision data. QueueDepth counts accepted decision entries still buffered in
// the channel; it excludes control barriers and entries already in-flight.
type LoggerStats struct {
	Accepted    uint64 `json:"accepted"`
	Persisted   uint64 `json:"persisted"`
	Dropped     uint64 `json:"dropped"`
	WriteErrors uint64 `json:"write_errors"`
	QueueDepth  int64  `json:"queue_depth"`
	LastError   string `json:"last_error,omitempty"`
}

type queuedEntry struct {
	key   string
	value string
}

type queueItem struct {
	entry   *queuedEntry
	barrier chan error
}

type Log struct {
	Database    *buntdb.DB
	LogLocation string

	queue         chan queueItem
	batchSize     int
	flushInterval time.Duration
	enqueueWait   time.Duration
	update        func(func(*buntdb.Tx) error) error

	stateMu    sync.Mutex
	producers  sync.WaitGroup
	closed     bool
	closeOnce  sync.Once
	closeReady chan struct{}
	done       chan struct{}
	closeErr   error

	queueDepth atomic.Int64

	accepted     atomic.Uint64
	persisted    atomic.Uint64
	dropped      atomic.Uint64
	writeErrors  atomic.Uint64
	lastError    atomic.Value
	lastWarn     atomic.Int64
	lastDropWarn atomic.Int64
}

type LogEntry struct {
	Time              int64  `json:"time"`
	IP                string `json:"ip"`
	URL               string `json:"url"`
	Type              string `json:"type"`
	DNSResponseType   string `json:"dnsResponseType"`
	ProxyResponseType string `json:"proxyResponseType"`
	// Structured decision provenance (PER-36). Optional; legacy entries
	// logged via LogDNS/LogProxy unmarshal with zero values.
	Layer          string `json:"layer,omitempty"`
	Action         string `json:"action,omitempty"`
	MatchedRule    string `json:"matched_rule,omitempty"`
	Reason         string `json:"reason,omitempty"`
	GroupID        string `json:"group_id,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	Source         string `json:"source,omitempty"`
	PolicyRevision int    `json:"policy_revision,omitempty"`
}

// NewLogger preserves the historical constructor. New code that needs to
// handle initialization failure should use OpenLogger.
func NewLogger(logLocation string) *Log {
	l, err := OpenLogger(logLocation)
	if err != nil {
		log.Printf("Gatesentry logger initialization error: %v", err)
		return nil
	}
	return l
}

// OpenLogger creates a logger and reports initialization errors to its caller.
func OpenLogger(logLocation string) (*Log, error) {
	return OpenLoggerWithOptions(logLocation, LoggerOptions{})
}

// OpenLoggerWithOptions creates a logger with bounded writer settings.
func OpenLoggerWithOptions(logLocation string, options LoggerOptions) (*Log, error) {
	if options.QueueCapacity <= 0 {
		options.QueueCapacity = defaultQueueCapacity
	}
	if options.BatchSize <= 0 {
		options.BatchSize = defaultBatchSize
	}
	if options.FlushInterval <= 0 {
		options.FlushInterval = defaultFlushInterval
	}
	if options.EnqueueWait <= 0 {
		options.EnqueueWait = defaultEnqueueWait
	}

	db, err := buntdb.Open(logLocation)
	if err != nil {
		return nil, fmt.Errorf("open logger database: %w", err)
	}
	fail := func(err error) (*Log, error) {
		_ = db.Close()
		return nil, err
	}
	var config buntdb.Config
	if err := db.ReadConfig(&config); err != nil {
		return fail(fmt.Errorf("read logger database config: %w", err))
	}
	config.SyncPolicy = buntdb.EverySecond
	if err := db.SetConfig(config); err != nil {
		return fail(fmt.Errorf("set logger database config: %w", err))
	}
	if err := db.CreateIndex("entries", "*", buntdb.IndexJSON("time")); err != nil && !errors.Is(err, buntdb.ErrIndexExists) {
		return fail(fmt.Errorf("create logger entries index: %w", err))
	}

	l := &Log{
		Database:      db,
		LogLocation:   logLocation,
		queue:         make(chan queueItem, options.QueueCapacity),
		batchSize:     options.BatchSize,
		flushInterval: options.FlushInterval,
		enqueueWait:   options.EnqueueWait,
		update:        db.Update,
		closeReady:    make(chan struct{}),
		done:          make(chan struct{}),
	}
	go l.runWriter()
	return l, nil
}

func (L *Log) runWriter() {
	ticker := time.NewTicker(L.flushInterval)
	defer ticker.Stop()
	batch := make([]queuedEntry, 0, L.batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		delay := initialRetryDelay
		for {
			err := L.update(func(tx *buntdb.Tx) error {
				for i := range batch {
					if _, _, err := tx.Set(batch[i].key, batch[i].value, &buntdb.SetOptions{Expires: true, TTL: Log_Entry_Expires}); err != nil {
						return err
					}
				}
				return nil
			})
			if err == nil {
				L.persisted.Add(uint64(len(batch)))
				batch = batch[:0]
				return nil
			}
			L.recordWriteError(err)

			time.Sleep(delay)
			if delay < maximumRetryDelay {
				delay *= 2
				if delay > maximumRetryDelay {
					delay = maximumRetryDelay
				}
			}
		}
	}

	finish := func(writeErr error) {
		if writeErr != nil {
			// A terminal failure is only requested by Close. Wait until every
			// producer admitted before Close has finished enqueueing before the
			// final drain, so no accepted entry can land after the drain.
			<-L.closeReady
		}
		// Unblock any Flush calls left behind a terminal write failure.
		for {
			select {
			case item := <-L.queue:
				if item.entry != nil {
					L.decrementQueueDepth()
				}
				if item.barrier != nil {
					item.barrier <- writeErr
					close(item.barrier)
				}
			default:
				closeErr := L.Database.Close()
				if errors.Is(closeErr, buntdb.ErrDatabaseClosed) {
					closeErr = nil
				}
				L.closeErr = errors.Join(writeErr, closeErr)
				close(L.done)
				return
			}
		}
	}

	process := func(item queueItem) error {
		if item.entry != nil {
			L.decrementQueueDepth()
			batch = append(batch, *item.entry)
			if len(batch) >= L.batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if item.barrier != nil {
			err := flush()
			item.barrier <- err
			close(item.barrier)
			return err
		}
		return nil
	}

	for {
		select {
		case item := <-L.queue:
			if err := process(item); err != nil {
				finish(err)
				return
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				finish(err)
				return
			}
		case <-L.closeReady:
			// No producer can enqueue after closeReady. Drain in FIFO order before
			// persisting the final partial batch and closing the database.
			for {
				select {
				case item := <-L.queue:
					if err := process(item); err != nil {
						finish(err)
						return
					}
				default:
					finish(flush())
					return
				}
			}
		}
	}
}

func (L *Log) decrementQueueDepth() {
	L.queueDepth.Add(-1)
}

func (L *Log) recordWriteError(err error) {
	L.writeErrors.Add(1)
	L.lastError.Store(err.Error())
	now := time.Now().Unix()
	last := L.lastWarn.Load()
	if now > last && L.lastWarn.CompareAndSwap(last, now) {
		log.Printf("Gatesentry logger write failed; retrying (errors=%d): %v", L.writeErrors.Load(), err)
	}
}

func (L *Log) recordDrop(reason string) {
	dropped := L.dropped.Add(1)
	now := time.Now().Unix()
	last := L.lastDropWarn.Load()
	if now > last && L.lastDropWarn.CompareAndSwap(last, now) {
		log.Printf("Gatesentry logger dropped an entry (%s; total dropped=%d)", reason, dropped)
	}
}

func (L *Log) enqueue(entry LogEntry) bool {
	if L == nil || L.Database == nil {
		return false
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		L.recordDrop("encoding failed")
		L.lastError.Store(err.Error())
		return false
	}
	item := queueItem{entry: &queuedEntry{
		key:   gatesentry2utils.RandomString(25) + gatesentry2utils.Int64toString(entry.Time),
		value: string(encoded),
	}}

	L.stateMu.Lock()
	if L.closed {
		L.stateMu.Unlock()
		L.recordDrop("logger closed")
		return false
	}
	L.producers.Add(1)
	L.stateMu.Unlock()
	defer L.producers.Done()

	select {
	case L.queue <- item:
		L.queueDepth.Add(1)
		L.accepted.Add(1)
		return true
	default:
	}

	timer := time.NewTimer(L.enqueueWait)
	defer timer.Stop()
	select {
	case L.queue <- item:
		L.queueDepth.Add(1)
		L.accepted.Add(1)
		return true
	case <-timer.C:
		L.recordDrop("queue full")
		return false
	}
}

// Stats returns counters for queue health and durable writes.
func (L *Log) Stats() LoggerStats {
	if L == nil {
		return LoggerStats{}
	}
	queueDepth := L.queueDepth.Load()
	if queueDepth < 0 {
		queueDepth = 0
	}
	stats := LoggerStats{
		Accepted:    L.accepted.Load(),
		Persisted:   L.persisted.Load(),
		Dropped:     L.dropped.Load(),
		WriteErrors: L.writeErrors.Load(),
		QueueDepth:  queueDepth,
	}
	if value := L.lastError.Load(); value != nil {
		stats.LastError, _ = value.(string)
	}
	return stats
}

// Flush waits until every entry accepted before this call is durably handed to
// BuntDB. Entries accepted after the barrier may remain queued.
func (L *Log) Flush(ctx context.Context) error {
	if L == nil || L.Database == nil {
		return nil
	}
	barrier := make(chan error, 1)
	L.stateMu.Lock()
	if L.closed {
		L.stateMu.Unlock()
		select {
		case <-L.done:
			return L.closeErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case L.queue <- queueItem{barrier: barrier}:
		L.stateMu.Unlock()
	case <-ctx.Done():
		L.stateMu.Unlock()
		return ctx.Err()
	}
	select {
	case err := <-barrier:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops admission, drains accepted entries, closes BuntDB, and is safe
// to call repeatedly. If ctx expires, draining continues in the background and
// a later Close can wait for the same shutdown.
func (L *Log) Close(ctx context.Context) error {
	if L == nil || L.Database == nil {
		return nil
	}
	L.closeOnce.Do(func() {
		L.stateMu.Lock()
		L.closed = true
		L.stateMu.Unlock()
		go func() {
			L.producers.Wait()
			close(L.closeReady)
		}()
	})
	select {
	case <-L.done:
		return L.closeErr
	case <-ctx.Done():
		// The caller stops waiting, but the writer keeps every accepted entry
		// and continues draining in the background. A later Close can wait for
		// the same shutdown without turning a timeout into data loss.
		return ctx.Err()
	}
}

func (L *Log) LogDNS(domain string, user string, responseType string) {
	if L == nil {
		return
	}
	L.enqueue(LogEntry{
		Time:            time.Now().Unix(),
		IP:              user,
		URL:             domain,
		Type:            "dns",
		DNSResponseType: responseType,
	})
}

func (L *Log) LogProxy(url string, user string, actionType string) {
	if L == nil {
		return
	}
	L.enqueue(LogEntry{
		Time:              time.Now().Unix(),
		IP:                user,
		URL:               url,
		Type:              "proxy",
		ProxyResponseType: actionType,
	})
}

// LogDecision records a structured filtering decision. It preserves the
// existing JSON schema and native DNS/proxy response fields.
func (L *Log) LogDecision(d gatesentryPolicy.Decision) {
	if L == nil || L.Database == nil {
		return
	}
	url := d.URL
	if url == "" {
		url = d.Domain
	}
	entry := LogEntry{
		Time:           d.Timestamp.Unix(),
		IP:             d.ClientIP,
		URL:            url,
		Layer:          string(d.Layer),
		Action:         string(d.Action),
		MatchedRule:    d.MatchedRule,
		Reason:         d.Reason,
		GroupID:        d.GroupID,
		DeviceID:       d.DeviceID,
		Source:         string(d.Source),
		PolicyRevision: d.PolicyRevision,
	}
	switch d.Layer {
	case gatesentryPolicy.LayerDNS:
		entry.Type = "dns"
		entry.DNSResponseType = d.ResponseType
	default:
		entry.Type = "proxy"
		entry.ProxyResponseType = d.ResponseType
	}
	L.enqueue(entry)
}

func (L *Log) GetLog() string {
	outputs := []string{}
	now := time.Now()
	totime := now.Unix()
	fromtime := totime - 100

	from := gatesentry2utils.Int64toString(fromtime)
	to := gatesentry2utils.Int64toString(totime)
	// fmt.Println("Viewing from " + from  + " to " + to );
	// , `{"time":30}`, `{"time":50}`
	limitEntries := 100
	index := 0
	err := L.Database.View(func(tx *buntdb.Tx) error {
		err := tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			if index >= limitEntries {
				return false
			}
			// fmt.Printf("key: %s, value: %s\n", key, value)
			outputs = append(outputs, value)
			index++
			return true
		})
		return err
	})
	_ = err

	return strings.Join(outputs, ",")
}

func (L *Log) GetLogSearch(search string) string {
	now := time.Now()
	totime := now.Unix()
	fromtime := totime - 100

	from := gatesentry2utils.Int64toString(fromtime)
	to := gatesentry2utils.Int64toString(totime)
	// fmt.Println("Viewing from " + from  + " to " + to );
	// , `{"time":30}`, `{"time":50}`
	limitEntries := 100
	index := 0
	outputs := []string{}
	err := L.Database.View(func(tx *buntdb.Tx) error {
		err := tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			if index >= limitEntries {
				return false
			}

			var parsedValue map[string]interface{}
			if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
				return true // Continue iterating
			}

			index++
			if v, ok := parsedValue["url"]; ok && (strings.Contains(v.(string), search)) {
				// result += value + ","
				outputs = append(outputs, value)
			}

			if v, ok := parsedValue["ip"]; ok && (strings.Contains(v.(string), search)) {
				// result += value + ","
				outputs = append(outputs, value)
			}
			return true
		})
		return err
	})
	_ = err

	// result = strings.TrimSuffix(result, ",")
	return strings.Join(outputs, ",")
}

// GetDeviceActivity returns decision-history entries whose client address
// matches one of matchIPs, newest first, inside sinceSeconds, capped at
// limit. It is a read-only query over the same store the live log view uses;
// it never introduces a second decision record.
func (L *Log) GetDeviceActivity(matchIPs []string, sinceSeconds int64, limit int) ([]LogEntry, error) {
	ips := make(map[string]bool, len(matchIPs))
	for _, ip := range matchIPs {
		if ip != "" {
			ips[ip] = true
		}
	}
	if len(ips) == 0 {
		return []LogEntry{}, nil
	}
	if sinceSeconds <= 0 {
		sinceSeconds = int64((24 * time.Hour).Seconds())
	}
	if limit <= 0 {
		limit = 100
	}
	now := time.Now().Unix()
	from := gatesentry2utils.Int64toString(now - sinceSeconds)
	to := gatesentry2utils.Int64toString(now)
	entries := []LogEntry{}
	err := L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			var entry LogEntry
			if err := json.Unmarshal([]byte(value), &entry); err != nil {
				return true
			}
			if ips[entry.IP] {
				entries = append(entries, entry)
			}
			return len(entries) < limit
		})
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (L *Log) GetLastXSecondsDNSLogs(fromSeconds int64, groupByDate bool) (interface{}, error) {
	var logs interface{} // The return type can be either []LogEntry or map[string][]LogEntry

	now := time.Now()
	totime := now.Unix()
	fromtime := totime - fromSeconds

	from := gatesentry2utils.Int64toString(fromtime)
	to := gatesentry2utils.Int64toString(totime)

	log.Println("[LogViewer] Viewing from " + from + " to " + to)

	L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			var parsedValue map[string]interface{}
			if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
				return true // Continue iterating
			}

			if v, ok := parsedValue["type"]; ok && (v == "dns" || v == "proxy") {
				var logEntry LogEntry
				if err := json.Unmarshal([]byte(value), &logEntry); err != nil {
					log.Println("[LogViewer] Error parsing log entry: " + err.Error() + " - " + value)
					return true // Continue iterating
				}
				if logEntry.Type == "proxy" {
					logEntry.URL = strings.Replace(logEntry.URL, "http://", "", -1)
					logEntry.URL = strings.Replace(logEntry.URL, ":443", "", -1)
					// log.Println( logEntry );
					log.Println("[LogViewer] Proxy log entry : " + logEntry.URL)

				}
				if groupByDate {
					// Group entries by date
					if logs == nil {
						logs = make(map[string][]LogEntry)
					}
					date := time.Unix(logEntry.Time, 0).Format("2006-01-02")
					logs.(map[string][]LogEntry)[date] = append(logs.(map[string][]LogEntry)[date], logEntry)
				} else {
					// No grouping, add directly to the slice
					if logs == nil {
						logs = []LogEntry{}
					}
					logs = append(logs.([]LogEntry), logEntry)
				}

			}

			return true
		})
	})
	if logs == nil {
		logs = []LogEntry{}
	}

	return logs, nil
}
