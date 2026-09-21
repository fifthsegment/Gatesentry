package gatesentry2logger

import (
	// "gatesentry2/utils"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gatesentry2utils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
	"github.com/tidwall/buntdb"
)

var Log_Entry_Expires = time.Second * 3600 * 24 * 7
var Commit_Interval = time.Second * 60

const (
	logQueueSize = 256
	maxLogScan   = 20000 // ungrouped live-log views only; stats scans the time window
)

type logWrite struct {
	key   string
	value string
	entry LogEntry
}

type Log struct {
	Database *buntdb.DB
	// DataCache
	LastCommitTime time.Time
	LogLocation    string

	// SSE subscriber support
	mu          sync.RWMutex
	subscribers map[chan LogEntry]struct{}

	writeCh chan logWrite
	stopCh  chan struct{}

	rollupMu     sync.Mutex
	minutes      map[string]*minuteSnap
	hours        map[string]*hourSnap
	dirtyMinutes map[string]struct{}
	dirtyHours   map[string]struct{}
	rollupOnce   sync.Once
	rollupsReady atomic.Bool
}

// Subscribe returns a channel that receives new log entries in real-time.
// The caller MUST call Unsubscribe when done to avoid goroutine leaks.
func (L *Log) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 64)
	L.mu.Lock()
	if L.subscribers == nil {
		L.subscribers = make(map[chan LogEntry]struct{})
	}
	L.subscribers[ch] = struct{}{}
	L.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (L *Log) Unsubscribe(ch chan LogEntry) {
	L.mu.Lock()
	delete(L.subscribers, ch)
	L.mu.Unlock()
	close(ch)
}

// SubscriberCount returns the current number of active log stream subscribers.
func (L *Log) SubscriberCount() int {
	L.mu.RLock()
	defer L.mu.RUnlock()
	return len(L.subscribers)
}

// broadcast sends a log entry to all active subscribers (non-blocking).
func (L *Log) broadcast(entry LogEntry) {
	L.mu.RLock()
	defer L.mu.RUnlock()
	for ch := range L.subscribers {
		select {
		case ch <- entry:
		default:
			// subscriber is slow, drop the entry
		}
	}
}

type LogEntry struct {
	Time              int64  `json:"time"`
	IP                string `json:"ip"`
	URL               string `json:"url"`
	Type              string `json:"type"`
	DNSResponseType   string `json:"dnsResponseType"`
	ProxyResponseType string `json:"proxyResponseType"`
	RuleName          string `json:"ruleName,omitempty"`
}

func (L *Log) Commit(tx *buntdb.Tx) {
	dur := time.Since(L.LastCommitTime)
	if dur > Commit_Interval {
		log.Println("Performing a commit")
		// tx.Commit();
		L.LastCommitTime = time.Now()
	}
}

func NewLogger(LogLocation string) *Log {
	log.Println("Creating a new log file = " + LogLocation)
	db, err := buntdb.Open(LogLocation)
	if err != nil {
		log.Printf("[Logger] open error: %v", err)
		l := &Log{writeCh: make(chan logWrite, 1), stopCh: make(chan struct{})}
		return l
	}
	var config buntdb.Config
	if err := db.ReadConfig(&config); err != nil {
		log.Printf("[Logger] read config error: %v", err)
	}
	config.SyncPolicy = buntdb.Never
	if err := db.SetConfig(config); err != nil {
		log.Printf("[Logger] set config error: %v", err)
	}
	// fmt.Println( config );
	// if err != nil {
	// 	log.Println("GS-LOGGER ERROR" + err.Error())
	// }
	db.CreateIndex("entries", "*", buntdb.IndexJSON("time"))

	l := &Log{}
	l.Database = db
	l.LogLocation = LogLocation
	l.LastCommitTime = time.Now()
	l.writeCh = make(chan logWrite, logQueueSize)
	l.stopCh = make(chan struct{})

	go l.writerLoop()
	l.initRollups()

	go func() {
		log.Println("[Logger] Running startup database shrink...")
		if err := db.Shrink(); err != nil {
			log.Println("[Logger] Startup shrink error:", err)
		} else {
			log.Println("[Logger] Startup shrink complete")
		}
	}()

	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-l.stopCh:
				return
			case <-ticker.C:
				log.Println("[Logger] Running periodic database shrink...")
				if err := db.Shrink(); err != nil {
					log.Println("[Logger] Periodic shrink error:", err)
				} else {
					log.Println("[Logger] Periodic shrink complete")
				}
			}
		}
	}()

	return l
}

func (L *Log) writerLoop() {
	for {
		select {
		case <-L.stopCh:
			return
		case w := <-L.writeCh:
			if L.Database == nil {
				continue
			}
			_ = L.Database.Update(func(tx *buntdb.Tx) error {
				_, _, err := tx.Set(w.key, w.value, &buntdb.SetOptions{Expires: true, TTL: Log_Entry_Expires})
				L.Commit(tx)
				return err
			})
			L.broadcast(w.entry)
		}
	}
}

func (L *Log) Close() {
	if L == nil {
		return
	}
	select {
	case <-L.stopCh:
	default:
		close(L.stopCh)
	}
	L.persistDirty()
	if L.Database != nil {
		_ = L.Database.Close()
		L.Database = nil
	}
}

func (L *Log) enqueue(entry LogEntry) {
	if L == nil || L.writeCh == nil {
		return
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}
	timestring := gatesentry2utils.Int64toString(entry.Time)
	w := logWrite{
		key:   gatesentry2utils.RandomString(25) + timestring,
		value: string(payload),
		entry: entry,
	}
	if L.rollupsReady.Load() {
		L.applyRollup(entry)
	}
	select {
	case L.writeCh <- w:
	default:
		// queue full — drop rather than spawning unbounded goroutines
	}
}

func (L *Log) LogDNS(domain string, user string, responseType string) {
	L.enqueue(LogEntry{
		Time:            time.Now().Unix(),
		IP:              user,
		URL:             domain,
		Type:            "dns",
		DNSResponseType: responseType,
	})
}

func (L *Log) LogProxy(url string, user string, actionType string, ruleName string) {
	L.enqueue(LogEntry{
		Time:              time.Now().Unix(),
		IP:                user,
		URL:               url,
		Type:              "proxy",
		ProxyResponseType: actionType,
		RuleName:          ruleName,
	})
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

// GetLastXSecondsDNSLogs retrieves DNS/proxy log entries from the last N seconds.
//
// groupByFormat controls how entries are bucketed:
//   - ""                   → no grouping, returns []LogEntry
//   - "2006-01-02"         → group by day   (local time)
//   - "2006-01-02T15"      → group by hour  (local time)
//   - "2006-01-02T15:04"   → group by minute (local time)
//
// When a groupByFormat is provided the return type is map[string][]LogEntry
// where the key is the formatted local-time bucket string.
func (L *Log) GetLastXSecondsDNSLogs(fromSeconds int64, groupByFormat string) (interface{}, error) {
	var logSlice []LogEntry
	var logMap map[string][]LogEntry

	useGrouping := groupByFormat != ""
	if useGrouping {
		logMap = make(map[string][]LogEntry)
	}

	now := time.Now()
	totime := now.Unix()
	fromtime := totime - fromSeconds

	from := gatesentry2utils.Int64toString(fromtime)
	to := gatesentry2utils.Int64toString(totime)

	scanned := 0

	if L.Database == nil {
		if useGrouping {
			return logMap, nil
		}
		return []LogEntry{}, nil
	}

	L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			scanned++
			// Ungrouped views (live log) stay bounded. Stats grouping
			// must cover the full requested window (up to log TTL, 7 days).
			if !useGrouping && scanned > maxLogScan {
				return false
			}
			var logEntry LogEntry
			if err := json.Unmarshal([]byte(value), &logEntry); err != nil {
				return true
			}

			if logEntry.Type != "dns" && logEntry.Type != "proxy" {
				return true
			}

			if logEntry.Type == "proxy" {
				logEntry.URL = strings.Replace(logEntry.URL, "http://", "", -1)
				logEntry.URL = strings.Replace(logEntry.URL, ":443", "", -1)
			}

			if useGrouping {
				bucket := time.Unix(logEntry.Time, 0).Local().Format(groupByFormat)
				logMap[bucket] = append(logMap[bucket], logEntry)
			} else {
				logSlice = append(logSlice, logEntry)
			}

			return true
		})
	})

	if useGrouping {
		return logMap, nil
	}
	if logSlice == nil {
		logSlice = []LogEntry{}
	}
	return logSlice, nil
}

func isProxyBlocked(action string) bool {
	switch action {
	case "blocked_text_content", "blocked_media_content", "blocked_file_type",
		"blocked_time", "blocked_internet_for_user", "blocked_url":
		return true
	default:
		return false
	}
}
