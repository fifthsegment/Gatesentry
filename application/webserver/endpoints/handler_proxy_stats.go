package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryproxy "bitbucket.org/abdullah_irfan/gatesentryproxy"
)

// isBlockedAction returns true if the proxy action represents a blocked request.
func isBlockedAction(action string) bool {
	switch action {
	case string(gatesentryproxy.ProxyActionBlockedTextContent),
		string(gatesentryproxy.ProxyActionBlockedMediaContent),
		string(gatesentryproxy.ProxyActionBlockedFileType),
		string(gatesentryproxy.ProxyActionBlockedTime),
		string(gatesentryproxy.ProxyActionBlockedInternetForUser),
		string(gatesentryproxy.ProxyActionBlockedUrl):
		return true
	}
	return false
}

// isAllowedAction returns true if the proxy action represents allowed/passed traffic.
func isAllowedAction(action string) bool {
	switch action {
	case string(gatesentryproxy.ProxyActionSSLDirect),
		string(gatesentryproxy.ProxyActionSSLBump),
		string(gatesentryproxy.ProxyActionFilterNone):
		return true
	}
	return false
}

// ---------- Response types ----------

// ProxyStatsSummary contains aggregate proxy traffic counts.
type ProxyStatsSummary struct {
	TotalRequests int `json:"total_requests"`
	Allowed       int `json:"allowed"`
	Blocked       int `json:"blocked"`
	SSLBumped     int `json:"ssl_bumped"`
	SSLDirect     int `json:"ssl_direct"`
}

// ProxyBucket contains time-bucketed proxy counts.
type ProxyBucket struct {
	Allowed int `json:"allowed"`
	Blocked int `json:"blocked"`
}

// ProxyTopSite is a single entry in a top-N site list.
type ProxyTopSite struct {
	Host   string `json:"host"`
	Count  int    `json:"count"`
	Action string `json:"action,omitempty"` // primary action for context
}

// ProxyActionBreakdown shows counts per proxy action type.
type ProxyActionBreakdown struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
}

// ProxyUserSummary shows per-user traffic stats.
type ProxyUserSummary struct {
	User    string `json:"user"`
	Total   int    `json:"total"`
	Allowed int    `json:"allowed"`
	Blocked int    `json:"blocked"`
}

// ProxyStatsResponse is the full response for GET /api/stats/proxy.
type ProxyStatsResponse struct {
	Summary    ProxyStatsSummary      `json:"summary"`
	TimeSeries map[string]ProxyBucket `json:"time_series"`
	TopBlocked []ProxyTopSite         `json:"top_blocked"`
	TopAllowed []ProxyTopSite         `json:"top_allowed"`
	Actions    []ProxyActionBreakdown `json:"actions"`
	Users      []ProxyUserSummary     `json:"users"`
}

// actionLabel returns a human-friendly label for a proxy action string.
func actionLabel(action string) string {
	switch action {
	case string(gatesentryproxy.ProxyActionBlockedTextContent):
		return "Blocked (Content)"
	case string(gatesentryproxy.ProxyActionBlockedMediaContent):
		return "Blocked (Media)"
	case string(gatesentryproxy.ProxyActionBlockedFileType):
		return "Blocked (File Type)"
	case string(gatesentryproxy.ProxyActionBlockedTime):
		return "Blocked (Time)"
	case string(gatesentryproxy.ProxyActionBlockedInternetForUser):
		return "Blocked (User)"
	case string(gatesentryproxy.ProxyActionBlockedUrl):
		return "Blocked (URL/Domain)"
	case string(gatesentryproxy.ProxyActionSSLDirect):
		return "SSL Direct"
	case string(gatesentryproxy.ProxyActionSSLBump):
		return "SSL Bumped (MITM)"
	case string(gatesentryproxy.ProxyActionFilterNone):
		return "Allowed"
	case string(gatesentryproxy.ProxyActionFilterError):
		return "Filter Error"
	default:
		return action
	}
}

// ApiGetProxyStats returns proxy-only traffic statistics.
//
// Query parameters:
//
//	seconds – time window (default 604800 = 7 days)
//	group   – bucket granularity: "day" (default), "hour", "minute"
//	user    – filter by user/IP (optional, empty = all users)
//
// GET /api/stats/proxy
func ApiGetProxyStats(w http.ResponseWriter, r *http.Request, logger *gatesentryLogger.Log) {
	seconds, group := ParseStatsQuery(r)
	userFilter := strings.TrimSpace(r.URL.Query().Get("user"))

	var groupFormat string
	switch group {
	case "hour":
		groupFormat = "2006-01-02T15"
	case "minute":
		groupFormat = "2006-01-02T15:04"
	default:
		groupFormat = "2006-01-02"
	}

	logEntriesInterface, err := logger.GetLastXSecondsDNSLogs(int64(seconds), groupFormat)
	if err != nil {
		jsonError(w, "Failed to retrieve logs", http.StatusInternalServerError)
		return
	}

	// Extract the grouped entries
	var bucketedLogs map[string][]gatesentryLogger.LogEntry
	if logEntriesInterface == nil {
		bucketedLogs = make(map[string][]gatesentryLogger.LogEntry)
	} else {
		switch logs := logEntriesInterface.(type) {
		case map[string][]gatesentryLogger.LogEntry:
			bucketedLogs = logs
		default:
			bucketedLogs = make(map[string][]gatesentryLogger.LogEntry)
		}
	}

	// Aggregate proxy-only entries
	var summary ProxyStatsSummary
	timeSeries := make(map[string]ProxyBucket)
	blockedCounts := make(map[string]int) // host → count
	allowedCounts := make(map[string]int)
	actionCounts := make(map[string]int) // action → count
	userStats := make(map[string]*ProxyUserSummary)

	for bucket, entries := range bucketedLogs {
		var bucketAllowed, bucketBlocked int

		for _, entry := range entries {
			if entry.Type != "proxy" {
				continue
			}

			// Apply user filter if specified
			if userFilter != "" && !strings.EqualFold(entry.IP, userFilter) {
				continue
			}

			summary.TotalRequests++
			actionCounts[entry.ProxyResponseType]++

			// Per-user aggregation
			userKey := entry.IP
			if userKey == "" {
				userKey = "anonymous"
			}
			us, ok := userStats[userKey]
			if !ok {
				us = &ProxyUserSummary{User: userKey}
				userStats[userKey] = us
			}
			us.Total++

			if isBlockedAction(entry.ProxyResponseType) {
				summary.Blocked++
				bucketBlocked++
				blockedCounts[entry.URL]++
				us.Blocked++
			} else if isAllowedAction(entry.ProxyResponseType) {
				summary.Allowed++
				bucketAllowed++
				allowedCounts[entry.URL]++
				us.Allowed++
			}

			// Track SSL types
			if entry.ProxyResponseType == string(gatesentryproxy.ProxyActionSSLBump) {
				summary.SSLBumped++
			} else if entry.ProxyResponseType == string(gatesentryproxy.ProxyActionSSLDirect) {
				summary.SSLDirect++
			}
		}

		if bucketAllowed > 0 || bucketBlocked > 0 {
			timeSeries[bucket] = ProxyBucket{
				Allowed: bucketAllowed,
				Blocked: bucketBlocked,
			}
		}
	}

	// Build top blocked sites (top 10)
	topBlocked := buildTopSites(blockedCounts, 10)

	// Build top allowed sites (top 10)
	topAllowed := buildTopSites(allowedCounts, 10)

	// Build action breakdown
	actions := make([]ProxyActionBreakdown, 0, len(actionCounts))
	for action, count := range actionCounts {
		actions = append(actions, ProxyActionBreakdown{
			Action: action,
			Label:  actionLabel(action),
			Count:  count,
		})
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Count > actions[j].Count
	})

	// Build user summary list
	users := make([]ProxyUserSummary, 0, len(userStats))
	for _, us := range userStats {
		users = append(users, *us)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Total > users[j].Total
	})

	resp := ProxyStatsResponse{
		Summary:    summary,
		TimeSeries: timeSeries,
		TopBlocked: topBlocked,
		TopAllowed: topAllowed,
		Actions:    actions,
		Users:      users,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// buildTopSites converts a host→count map into a sorted top-N slice.
func buildTopSites(counts map[string]int, n int) []ProxyTopSite {
	sites := make([]ProxyTopSite, 0, len(counts))
	for host, count := range counts {
		sites = append(sites, ProxyTopSite{Host: host, Count: count})
	}
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].Count > sites[j].Count
	})
	if len(sites) > n {
		sites = sites[:n]
	}
	return sites
}
