package gatesentry2logger

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2utils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
	"github.com/tidwall/buntdb"
)

var Log_Entry_Expires = time.Second * 3600 * 24 * 7
var Commit_Interval = time.Second * 60

type Log struct {
	Database *buntdb.DB
	// DataCache
	LastCommitTime time.Time
	LogLocation    string
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
		log.Println("Gatesentry logger error" + err.Error())
		log.Fatal(err)
		return nil
	}
	var config buntdb.Config
	if err := db.ReadConfig(&config); err != nil {
		log.Println("Gatesentry logger error" + err.Error())
		log.Fatal(err)
		return nil
	}
	config.SyncPolicy = buntdb.Never
	if err := db.SetConfig(config); err != nil {
		log.Println("Gatesentry logger error" + err.Error())
		log.Fatal(err)
		return nil
	}
	if err := db.ReadConfig(&config); err != nil {
		log.Println("Gatesentry logger error" + err.Error())
		log.Fatal(err)
		return nil
	}
	// fmt.Println( config );
	// if err != nil {
	// 	log.Println("GS-LOGGER ERROR" + err.Error())
	// }
	db.CreateIndex("entries", "*", buntdb.IndexJSON("time"))
	// defer db.Close()

	l := &Log{}
	l.Database = db
	l.LogLocation = LogLocation
	l.LastCommitTime = time.Now()
	return l
}

func (L *Log) LogDNS(domain string, user string, responseType string) {
	ip := user
	// url:=url;
	go func() {
		now := time.Now()
		secs := now.Unix()
		_ = secs

		timestring := gatesentry2utils.Int64toString(secs)
		logJson := `{"time": ` + timestring + `, "ip":"` + ip + `","url":"` + domain + `","type":"dns", "dnsResponseType":"` + responseType + `"}`
		key := gatesentry2utils.RandomString(25) + timestring

		err := L.Database.Update(func(tx *buntdb.Tx) error {
			_, _, err := tx.Set(key, logJson, &buntdb.SetOptions{Expires: true, TTL: Log_Entry_Expires})
			L.Commit(tx)
			return err
		})
		// fmt.Println( err );
		_ = err
	}()
}

func (L *Log) LogProxy(url string, user string, actionType string) {
	ip := user
	// url:=url;
	go func() {
		now := time.Now()
		secs := now.Unix()
		_ = secs
		// logitem := "[GS-Logger] " + ctx.Req.RemoteAddr + " - " + ctx.Req.URL.String();

		timestring := gatesentry2utils.Int64toString(secs)
		logJson := `{"time": ` + timestring + `, "ip":"` + ip + `","url":"` + url + `", "type":"proxy", "proxyResponseType":"` + actionType + `"}`
		key := gatesentry2utils.RandomString(25) + timestring
		// fmt.Println( logJson );

		err := L.Database.Update(func(tx *buntdb.Tx) error {
			_, _, err := tx.Set(key, logJson, &buntdb.SetOptions{Expires: true, TTL: Log_Entry_Expires})
			L.Commit(tx)
			return err
		})
		// fmt.Println( err );
		_ = err
	}()

}

// LogDecision records a structured filtering decision. It is the canonical
// PER-36 path: every DNS, proxy, and content enforcement point builds a
// policy.Decision and hands it here. The decision is flattened into a
// LogEntry so the existing log, stats, and device-activity queries keep
// reading it, and stored asynchronously like LogDNS/LogProxy.
//
// Legacy compatibility: ResponseType is written to DNSResponseType (DNS) or
// ProxyResponseType (proxy/content) so viewers that filter on the native
// outcome label are unaffected. New viewers read Action/Layer/MatchedRule.
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
	case gatesentryPolicy.LayerExplicitProxy, gatesentryPolicy.LayerTransparentProxy, gatesentryPolicy.LayerContent:
		entry.Type = "proxy"
		entry.ProxyResponseType = d.ResponseType
	default:
		entry.Type = "proxy"
		entry.ProxyResponseType = d.ResponseType
	}
	go func() {
		logJson, err := json.Marshal(entry)
		if err != nil {
			log.Println("Gatesentry logger LogDecision marshal error: " + err.Error())
			return
		}
		key := gatesentry2utils.RandomString(25) + gatesentry2utils.Int64toString(entry.Time)
		err = L.Database.Update(func(tx *buntdb.Tx) error {
			_, _, err := tx.Set(key, string(logJson), &buntdb.SetOptions{Expires: true, TTL: Log_Entry_Expires})
			L.Commit(tx)
			return err
		})
		_ = err
	}()
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
