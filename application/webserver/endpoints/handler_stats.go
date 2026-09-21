package gatesentryWebserverEndpoints

import (
	"net/http"
	"strconv"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

// ParseStatsQuery extracts seconds and group from the request query string.
// Defaults: seconds=604800 (7 days), group="day".
func ParseStatsQuery(r *http.Request) (seconds int, group string) {
	seconds = 604800 // 7 days — matches the stats "Past 7 days" filter
	group = "day"

	if s := r.URL.Query().Get("seconds"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			seconds = v
		}
	}
	if g := r.URL.Query().Get("group"); g != "" {
		group = g
	}
	return
}

func ApiGetStats(fromTimeParam string, logger *gatesentryLogger.Log) interface{} {

	// Parse the fromTimeParam to an integer
	fromTimeInt, err := strconv.Atoi(fromTimeParam)
	if err != nil {
		// ctx.StatusCode(iris.StatusBadRequest)
		// ctx.JSON(iris.Map{"error": "Invalid fromTime parameter"})
		return struct {
			Error string `json:"error"`
		}{Error: "Invalid fromTime parameter"}
	}

	logEntriesInterface, err := logger.GetLastXSecondsDNSLogs(int64(fromTimeInt), "")
	if err != nil {
		// ctx.StatusCode(iris.StatusInternalServerError)
		// ctx.JSON(iris.Map{"error": "Failed to retrieve logs"})
		return struct {
			Error string `json:"error"`
		}{Error: "Failed to retrieve logs"}
	}

	var logEntries []gatesentryLogger.LogEntry
	switch logs := logEntriesInterface.(type) {
	case []gatesentryLogger.LogEntry:
		logEntries = logs
	case map[string][]gatesentryLogger.LogEntry:
		for _, entries := range logs {
			logEntries = append(logEntries, entries...)
		}
	default:
		// ctx.StatusCode(iris.StatusInternalServerError)
		// ctx.JSON(iris.Map{"error": "Invalid logs format"})
		return struct {
			Error string `json:"error"`
		}{Error: "Invalid logs format"}
	}

	return struct {
		Items []gatesentryLogger.LogEntry `json:"items"`
	}{
		Items: logEntries,
	}

	// ctx.JSON(response)
}

// StatsSeriesResponse is the compact stats payload: per-minute totals plus
// hourly top-N hosts. The UI rebuckets this in the browser (7d hourly, 24h,
// 1h per-minute) without another server scan.
type StatsSeriesResponse struct {
	Minutes     []gatesentryLogger.MinutePoint          `json:"minutes"`
	HourlyHosts map[string]gatesentryLogger.HourHostSet `json:"hourly_hosts"`
}

// ApiGetStatsByURL returns a 7-day (or `seconds`) compact series.
//
// Query parameters (all optional):
//
//	seconds  – time window in seconds (default 604800 = 7 days)
//	group    – ignored; the client rebuckets the minute series
func ApiGetStatsByURL(logger *gatesentryLogger.Log, seconds int, group string) interface{} {
	_ = group
	if logger == nil {
		return StatsSeriesResponse{
			Minutes:     []gatesentryLogger.MinutePoint{},
			HourlyHosts: map[string]gatesentryLogger.HourHostSet{},
		}
	}
	series := logger.GetTrafficSeries(int64(seconds))
	if series.Minutes == nil {
		series.Minutes = []gatesentryLogger.MinutePoint{}
	}
	if series.HourlyHosts == nil {
		series.HourlyHosts = map[string]gatesentryLogger.HourHostSet{}
	}
	return StatsSeriesResponse{
		Minutes:     series.Minutes,
		HourlyHosts: series.HourlyHosts,
	}
}
