package gatesentry2logger

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tidwall/buntdb"
)

func TestApplyRollupMinuteAndHour(t *testing.T) {
	l := NewLogger(filepath.Join(t.TempDir(), "log.db"))
	t.Cleanup(l.Close)
	l.WaitRollupsForTest()

	now := time.Now()
	min := now.Local().Format("2006-01-02T15:04")
	hour := now.Local().Format("2006-01-02T15")

	l.applyRollup(LogEntry{Time: now.Unix(), URL: "ads.example.com", Type: "dns", DNSResponseType: "blocked"})
	l.applyRollup(LogEntry{Time: now.Unix(), URL: "ok.example.com", Type: "dns", DNSResponseType: "forwarded"})
	l.applyRollup(LogEntry{Time: now.Unix(), URL: "ads.example.com:443", Type: "proxy", IP: "user1", ProxyResponseType: "blocked_url"})
	l.applyRollup(LogEntry{Time: now.Unix(), URL: "news.example.com:443", Type: "proxy", IP: "user1", ProxyResponseType: "ssl-bump"})

	series := l.GetTrafficSeries(3600)
	if len(series.Minutes) == 0 {
		t.Fatal("expected at least one minute sample")
	}
	var point *MinutePoint
	for i := range series.Minutes {
		if series.Minutes[i].T == min {
			point = &series.Minutes[i]
			break
		}
	}
	if point == nil {
		t.Fatalf("missing minute %s in %#v", min, series.Minutes)
	}
	if point.DNSAll != 2 || point.DNSBlocked != 1 {
		t.Errorf("dns counts all=%d blocked=%d, want 2/1", point.DNSAll, point.DNSBlocked)
	}
	if point.ProxyAllowed != 1 || point.ProxyBlocked != 1 {
		t.Errorf("proxy counts allowed=%d blocked=%d, want 1/1", point.ProxyAllowed, point.ProxyBlocked)
	}
	if point.ProxySSLBump != 1 {
		t.Errorf("ssl bump=%d, want 1", point.ProxySSLBump)
	}

	hosts, ok := series.HourlyHosts[hour]
	if !ok {
		t.Fatalf("missing hour %s", hour)
	}
	if len(hosts.Blocked) < 1 || hosts.Blocked[0].Host != "ads.example.com" {
		t.Errorf("hourly blocked = %#v, want ads.example.com", hosts.Blocked)
	}
	if len(hosts.ProxyBlocked) < 1 || hosts.ProxyBlocked[0].Host != "ads.example.com" {
		t.Errorf("proxy blocked host = %#v", hosts.ProxyBlocked)
	}
}

func TestGetTrafficSeriesBackfillFromLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.db")

	db, err := buntdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateIndex("entries", "*", buntdb.IndexJSON("time")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	entry := LogEntry{Time: now, URL: "fill.example.com", Type: "dns", DNSResponseType: "blocked"}
	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	key := "prefill" + strconv.FormatInt(now, 10)
	if err := db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(key, string(payload), nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	l := NewLogger(path)
	t.Cleanup(l.Close)
	l.WaitRollupsForTest()
	series := l.GetTrafficSeries(3600)
	found := false
	for _, p := range series.Minutes {
		if p.DNSBlocked >= 1 && p.DNSAll >= 1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("backfill did not include inserted dns row: %#v", series.Minutes)
	}
}

func TestHostMapPruneKeepsTop(t *testing.T) {
	m := map[string]int{}
	for i := 0; i < hostMapSoftCap+5; i++ {
		bumpHost(m, "h"+strconv.Itoa(i))
	}
	// one extra popular host
	for i := 0; i < 10; i++ {
		bumpHost(m, "popular.example")
	}
	if len(m) > hostMapSoftCap {
		t.Errorf("map grew to %d, want prune at %d", len(m), hostMapSoftCap)
	}
	if m["popular.example"] < 10 {
		t.Errorf("popular host count = %d", m["popular.example"])
	}
}
