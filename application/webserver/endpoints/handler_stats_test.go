package gatesentryWebserverEndpoints

import (
	"testing"
	"time"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

func TestApiGetStatsByURLReturnsMinuteSeries(t *testing.T) {
	logger := setupTestLogger(t.TempDir())
	logger.WaitRollupsForTest()
	now := time.Now()
	logger.Observe(gatesentryLogger.LogEntry{
		Time:            now.Unix(),
		URL:             "ads.example.com",
		Type:            "dns",
		DNSResponseType: "blocked",
	})
	logger.Observe(gatesentryLogger.LogEntry{
		Time:            now.Unix(),
		URL:             "ok.example.com",
		Type:            "dns",
		DNSResponseType: "forwarded",
	})

	out := ApiGetStatsByURL(logger, 3600, "day")
	resp, ok := out.(StatsSeriesResponse)
	if !ok {
		t.Fatalf("got %T, want StatsSeriesResponse", out)
	}
	if len(resp.Minutes) == 0 {
		t.Fatal("expected minute samples")
	}
	var all, blocked int
	for _, p := range resp.Minutes {
		all += p.DNSAll
		blocked += p.DNSBlocked
	}
	if all != 2 || blocked != 1 {
		t.Errorf("dns all=%d blocked=%d, want 2/1", all, blocked)
	}
	if len(resp.HourlyHosts) == 0 {
		t.Fatal("expected hourly host lists")
	}
}
