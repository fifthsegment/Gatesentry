package gatesentryWebserverEndpoints

import (
	"net/http"
	"sort"
	"strconv"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryproxy "bitbucket.org/abdullah_irfan/gatesentryproxy"
)

// ParseStatsQuery extracts seconds and group from the request query string.
// Defaults: seconds=604800 (7 days), group="day".
func ParseStatsQuery(r *http.Request) (seconds int, group string) {
	seconds = 604800 // 7 days
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

type URLGroup struct {
	URL   string `json:"host"`
	Count int    `json:"count"`
}

type HostGroupWithTotal struct {
	Total int        `json:"total"`
	Hosts []URLGroup `json:"hosts"`
}

type HostGroupSet map[string]HostGroupWithTotal

type HostGroupResponse struct {
	ItemsBlocked HostGroupSet `json:"blocked"`
	All          HostGroupSet `json:"all"`
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

func SliceEntries(logs map[string][]gatesentryLogger.LogEntry, responseType string) map[string]HostGroupWithTotal {
	outputData := make(map[string]HostGroupWithTotal)
	for currentDate, entries := range logs {
		urlCounts := make(map[string]int)
		for _, entry := range entries {
			if (entry.Type == "dns" && responseType == "all") || (entry.Type == "proxy" && responseType == "all") {
				urlCounts[entry.URL]++
			} else if entry.Type == "dns" && entry.DNSResponseType == responseType {
				urlCounts[entry.URL]++
			} else if entry.Type == "proxy" && responseType == "blocked" {
				if entry.ProxyResponseType == string(gatesentryproxy.ProxyActionBlockedTextContent) ||
					entry.ProxyResponseType == string(gatesentryproxy.ProxyActionBlockedMediaContent) ||
					entry.ProxyResponseType == string(gatesentryproxy.ProxyActionBlockedFileType) ||
					entry.ProxyResponseType == string(gatesentryproxy.ProxyActionBlockedTime) ||
					entry.ProxyResponseType == string(gatesentryproxy.ProxyActionBlockedInternetForUser) {
					urlCounts[entry.URL]++
				}
			}
		}
		groupedURLs := make([]URLGroup, 0, len(urlCounts))
		hostCount := 0
		for url, count := range urlCounts {
			groupedURLs = append(groupedURLs, URLGroup{URL: url, Count: count})
			// sort groupedUIRLs by count
			sort.Slice(groupedURLs, func(i, j int) bool {
				return groupedURLs[i].Count > groupedURLs[j].Count
			})
			hostCount += count
		}
		outputData[currentDate] = HostGroupWithTotal{
			Total: hostCount,
			Hosts: groupedURLs,
		}
	}
	return outputData
}

// ApiGetStatsByURL returns DNS/proxy stats grouped by time bucket.
//
// Query parameters (all optional):
//
//	seconds  – time window in seconds (default 604800 = 7 days)
//	group    – bucket granularity: "day" (default), "hour", or "minute"
//
// The bucket keys are LOCAL-time strings so the frontend can display
// them without any UTC ↔ local conversion:
//
//	day    → "2006-01-02"        (matches the existing 7-day format)
//	hour   → "2006-01-02T15"     (for 24-hour view)
//	minute → "2006-01-02T15:04"  (for 1-hour view)
func ApiGetStatsByURL(logger *gatesentryLogger.Log, seconds int, group string) interface{} {
	// Map human-readable group names to Go time format strings.
	var groupFormat string
	switch group {
	case "hour":
		groupFormat = "2006-01-02T15"
	case "minute":
		groupFormat = "2006-01-02T15:04"
	default: // "day" or anything else
		groupFormat = "2006-01-02"
	}

	logEntriesInterface, err := logger.GetLastXSecondsDNSLogs(int64(seconds), groupFormat)
	if err != nil {
		return struct {
			Error string `json:"error"`
		}{Error: "Failed to retrieve logs"}
	}

	if logEntriesInterface == nil {
		return HostGroupResponse{
			ItemsBlocked: make(HostGroupSet),
			All:          make(HostGroupSet),
		}
	}

	switch logs := logEntriesInterface.(type) {
	case map[string][]gatesentryLogger.LogEntry:
		return HostGroupResponse{
			ItemsBlocked: SliceEntries(logs, "blocked"),
			All:          SliceEntries(logs, "all"),
		}
	default:
		return HostGroupResponse{
			ItemsBlocked: make(HostGroupSet),
			All:          make(HostGroupSet),
		}
	}
}
