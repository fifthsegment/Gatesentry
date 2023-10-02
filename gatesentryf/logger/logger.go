package gatesentry2logger

import (
	// "gatesentry2/utils"
	// "github.com/elazarl/goproxy"
	"encoding/json"
	"log"
	"strings"
	"time"

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
	Time int64  `json:"time"`
	IP   string `json:"ip"`
	URL  string `json:"url"`
	Type string `json:"type"`
	// Add more fields if needed
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
	var config buntdb.Config
	if err := db.ReadConfig(&config); err != nil {
		// log.Fatal(err)
	}
	config.SyncPolicy = buntdb.Never
	if err := db.SetConfig(config); err != nil {
		// log.Fatal(err)
	}
	if err := db.ReadConfig(&config); err != nil {
		// log.Fatal(err)
	}
	// fmt.Println( config );
	if err != nil {
		log.Println("GS-LOGGER ERROR" + err.Error())
	}
	db.CreateIndex("entries", "*", buntdb.IndexJSON("time"))
	// defer db.Close()

	l := &Log{}
	l.Database = db
	l.LogLocation = LogLocation
	l.LastCommitTime = time.Now()
	return l
}

func (L *Log) LogDNS(domain string, user string) {
	ip := user
	// url:=url;
	go func() {
		now := time.Now()
		secs := now.Unix()
		_ = secs
		// fmt.Println( gatesentry2utils.GetUserFromAuthHeader(ctx.Req) );
		// logitem := "[GS-Logger] " + ctx.Req.RemoteAddr + " - " + ctx.Req.URL.String();

		timestring := gatesentry2utils.Int64toString(secs)
		logJson := `{"time": ` + timestring + `, "ip":"` + ip + `","url":"` + domain + `","type":"dns"}`
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

func (L *Log) Log(url string, user string) {
	ip := user
	// url:=url;
	go func() {
		now := time.Now()
		secs := now.Unix()
		_ = secs
		// fmt.Println( gatesentry2utils.GetUserFromAuthHeader(ctx.Req) );
		// logitem := "[GS-Logger] " + ctx.Req.RemoteAddr + " - " + ctx.Req.URL.String();

		timestring := gatesentry2utils.Int64toString(secs)
		logJson := `{"time": ` + timestring + `, "ip":"` + ip + `","url":"` + url + `"}`
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

func (L *Log) GetLog(entries int64) string {
	result := ``
	now := time.Now()
	totime := now.Unix()
	fromtime := totime - 100

	from := gatesentry2utils.Int64toString(fromtime)
	to := gatesentry2utils.Int64toString(totime)
	// fmt.Println("Viewing from " + from  + " to " + to );
	// , `{"time":30}`, `{"time":50}`
	err := L.Database.View(func(tx *buntdb.Tx) error {
		err := tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			// fmt.Printf("key: %s, value: %s\n", key, value)
			result += value + ","

			return true
		})
		return err
	})
	_ = err

	result = strings.TrimSuffix(result, ",")
	return result
}

func (L *Log) GetDNSLogs(entries int64) ([]LogEntry, error) {
	var logs []LogEntry

	now := time.Now()
	totime := now.Unix()
	fromtime := totime - entries

	from := gatesentry2utils.Int64toString(fromtime)
	to := gatesentry2utils.Int64toString(totime)

	err := L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			var parsedValue map[string]interface{}
			if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
				return true // Continue iterating
			}

			if v, ok := parsedValue["type"]; ok && v == "dns" {
				var logEntry LogEntry
				if err := json.Unmarshal([]byte(value), &logEntry); err != nil {
					return true // Continue iterating
				}
				logs = append(logs, logEntry)
			}

			return true
		})
	})
	if err != nil {
		return nil, err
	}

	return logs, nil
}
